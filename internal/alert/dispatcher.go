// Package alert delivers monitor status-change notifications based on the
// per-group AlertPolicy stored in MongoDB. Dispatch is best-effort: failures
// are logged and never block the scheduler or probe write path.
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

// Store is the slice of the monitoring store the dispatcher needs: resolving
// a MonitorGroup for its AlertPolicy and the NotifyGroups it references.
type Store interface {
	GetMonitorGroup(ctx context.Context, id string) (store.MonitorGroup, error)
	GetNotifyGroup(ctx context.Context, id string) (store.NotifyGroup, error)
}

// Dispatcher fans out per-group notifications for monitor status
// transitions. It is safe for concurrent use.
type Dispatcher struct {
	store    Store
	logger   *slog.Logger
	notifier *notify.Notifier

	// deliveries tracks in-flight background sends so tests (and a future
	// graceful shutdown) can wait for the fan-out to drain.
	deliveries sync.WaitGroup

	mu   sync.Mutex
	last map[string]time.Time // key: groupID + "|" + monitorID
}

// New builds a Dispatcher backed by st. logger may be nil; the package logger
// will then be used. The shared http.Client has a small timeout so a slow
// webhook receiver cannot stall the goroutine pool.
func New(st Store, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default().With("component", "alert")
	}
	return &Dispatcher{
		store:    st,
		logger:   logger,
		notifier: notify.New(&http.Client{Timeout: 5 * time.Second}, logger),
		last:     make(map[string]time.Time),
	}
}

// Payload is the JSON body delivered to each webhook target.
type Payload struct {
	MonitorID      string    `json:"monitor_id"`
	MonitorName    string    `json:"monitor_name"`
	ServerID       string    `json:"server_id"`
	PreviousStatus string    `json:"previous_status"`
	CurrentStatus  string    `json:"current_status"`
	OccurredAt     time.Time `json:"occurred_at"`
	GroupID        string    `json:"group_id"`
	GroupName      string    `json:"group_name"`
}

// OnMonitorStatusChange evaluates each of the monitor's groups against its
// AlertPolicy and dispatches notifications to the channels of every referenced
// NotifyGroup for any matching transition. The call returns immediately;
// deliveries happen in background goroutines so the scheduler is not blocked on
// slow receivers.
func (d *Dispatcher) OnMonitorStatusChange(ctx context.Context, monitor store.Monitor, prev, curr string, occurredAt time.Time) {
	if d == nil || len(monitor.GroupIDs) == 0 {
		return
	}
	if normalize(prev) == normalize(curr) {
		return
	}
	for _, groupID := range monitor.GroupIDs {
		group, err := d.store.GetMonitorGroup(ctx, groupID)
		if err != nil {
			d.logger.Warn("load group for alert", "group_id", groupID, "error", err.Error())
			continue
		}
		policy := group.AlertPolicy
		if !policy.Enabled || len(policy.NotifyGroupIDs) == 0 {
			continue
		}
		if !shouldFire(policy, prev, curr) {
			d.logger.Debug("alert not fired by policy",
				"group_id", groupID, "monitor_id", monitor.ID,
				"prev", normalize(prev), "curr", normalize(curr))
			continue
		}
		// Recovery notifications are never throttled: suppressing them would
		// leave receivers believing the service is still down. A recovery also
		// resets the throttle window so the next incident alerts immediately.
		if normalize(curr) == "Healthy" {
			d.resetThrottle(groupID, monitor.ID)
		} else if !d.allowThrottle(groupID, monitor.ID, policy.MinIntervalSeconds, occurredAt) {
			d.logger.Info("alert throttled",
				"group_id", groupID, "monitor_id", monitor.ID,
				"min_interval_seconds", policy.MinIntervalSeconds)
			continue
		}
		channels := d.resolveChannels(ctx, policy.NotifyGroupIDs)
		if len(channels) == 0 {
			d.logger.Warn("alert has no resolvable channels",
				"group_id", group.ID, "monitor_id", monitor.ID,
				"notify_group_ids", policy.NotifyGroupIDs)
			continue
		}
		d.logger.Info("dispatching alert",
			"group_id", group.ID, "monitor_id", monitor.ID,
			"prev", normalize(prev), "curr", normalize(curr),
			"channels", len(channels))
		payload := Payload{
			MonitorID:      monitor.ID,
			MonitorName:    monitor.Name,
			ServerID:       monitor.ServerID,
			PreviousStatus: normalize(prev),
			CurrentStatus:  normalize(curr),
			OccurredAt:     occurredAt.UTC(),
			GroupID:        group.ID,
			GroupName:      group.Name,
		}
		for _, channel := range channels {
			ch := channel
			d.deliveries.Go(func() {
				d.deliver(ch, payload)
			})
		}
	}
}

// resolveChannels loads the referenced NotifyGroups and flattens their channels
// into a single slice. Missing groups are logged and skipped.
func (d *Dispatcher) resolveChannels(ctx context.Context, notifyGroupIDs []string) []store.AlertChannel {
	var channels []store.AlertChannel
	for _, id := range notifyGroupIDs {
		ng, err := d.store.GetNotifyGroup(ctx, id)
		if err != nil {
			d.logger.Warn("load notify group for alert", "notify_group_id", id, "error", err.Error())
			continue
		}
		channels = append(channels, ng.Channels...)
	}
	return channels
}

func shouldFire(policy store.AlertPolicy, prev, curr string) bool {
	p := normalize(prev)
	c := normalize(curr)
	// recovery: any non-healthy -> Healthy. A previous status of "" or
	// "Unknown" means the monitor never had a definite state (new monitors are
	// created with StatusUnknown), so its first Healthy probe is not a recovery.
	if policy.OnRecover && p != "Healthy" && p != "" && p != "Unknown" && c == "Healthy" {
		return true
	}
	switch c {
	case "Down":
		return policy.OnDown
	case "Critical":
		return policy.OnCritical
	case "Warning":
		return policy.OnWarning
	}
	return false
}

func (d *Dispatcher) resetThrottle(groupID, monitorID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.last, groupID+"|"+monitorID)
}

func (d *Dispatcher) allowThrottle(groupID, monitorID string, minIntervalSeconds uint32, now time.Time) bool {
	if minIntervalSeconds == 0 {
		return true
	}
	key := groupID + "|" + monitorID
	window := time.Duration(minIntervalSeconds) * time.Second
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, ok := d.last[key]; ok && now.Sub(last) < window {
		return false
	}
	d.last[key] = now
	return true
}

// deliver renders the monitor payload and hands it to the generic notifier.
func (d *Dispatcher) deliver(channel store.AlertChannel, payload Payload) {
	webhookJSON, err := json.Marshal(payload)
	if err != nil {
		d.logger.Warn("alert payload marshal failed",
			"monitor_id", payload.MonitorID,
			"group_id", payload.GroupID,
			"error", err.Error(),
		)
		return
	}
	delivery := notify.Delivery{
		WebhookJSON: webhookJSON,
		HumanText:   formatMessage(payload),
	}
	if err := d.notifier.Deliver(channel, delivery); err != nil {
		if errors.Is(err, notify.ErrUnsupportedChannel) {
			d.logger.Warn("unsupported alert channel type", "type", channel.Type)
			return
		}
		d.logger.Warn("alert delivery failed",
			"type", channel.Type,
			"target", channel.Target,
			"monitor_id", payload.MonitorID,
			"group_id", payload.GroupID,
			"error", err.Error(),
		)
		return
	}
	d.logger.Debug("alert delivered",
		"type", channel.Type,
		"target", channel.Target,
		"monitor_id", payload.MonitorID,
		"group_id", payload.GroupID,
	)
}

// formatMessage renders a human-readable alert line shared by the chat-style
// channels (telegram, discord, mastodon).
func formatMessage(p Payload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[neo-line] %s: %s → %s", p.MonitorName, p.PreviousStatus, p.CurrentStatus)
	if p.GroupName != "" {
		fmt.Fprintf(&b, "\nGroup: %s", p.GroupName)
	}
	if p.ServerID != "" {
		fmt.Fprintf(&b, "\nServer: %s", p.ServerID)
	}
	fmt.Fprintf(&b, "\nTime: %s", p.OccurredAt.UTC().Format(time.RFC3339))
	return b.String()
}

func normalize(s string) string {
	switch s {
	case "Healthy", "healthy", "HEALTHY", "HEALTH_STATUS_HEALTHY":
		return "Healthy"
	case "Warning", "warning", "WARNING", "HEALTH_STATUS_WARNING":
		return "Warning"
	case "Critical", "critical", "CRITICAL", "HEALTH_STATUS_CRITICAL":
		return "Critical"
	case "Down", "down", "DOWN", "HEALTH_STATUS_DOWN":
		return "Down"
	case "":
		return ""
	default:
		return "Unknown"
	}
}
