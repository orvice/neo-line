// Package alert delivers monitor status-change notifications based on the
// per-group AlertPolicy stored in MongoDB. Dispatch is best-effort: failures
// are logged and never block the scheduler or probe write path.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Dispatcher fans out per-group webhook notifications for monitor status
// transitions. It is safe for concurrent use.
type Dispatcher struct {
	store  store.Store
	logger *slog.Logger
	client *http.Client

	mu   sync.Mutex
	last map[string]time.Time // key: groupID + "|" + monitorID
}

// New builds a Dispatcher backed by st. logger may be nil; the package logger
// will then be used. The shared http.Client has a small timeout so a slow
// webhook receiver cannot stall the goroutine pool.
func New(st store.Store, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default().With("component", "alert")
	}
	return &Dispatcher{
		store:  st,
		logger: logger,
		client: &http.Client{Timeout: 5 * time.Second},
		last:   make(map[string]time.Time),
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
// AlertPolicy and dispatches webhook notifications for any matching transition.
// The call returns immediately; webhook deliveries happen in background
// goroutines so the scheduler is not blocked on slow receivers.
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
		if !policy.Enabled || len(policy.Channels) == 0 {
			continue
		}
		if !shouldFire(policy, prev, curr) {
			continue
		}
		if !d.allowThrottle(groupID, monitor.ID, policy.MinIntervalSeconds, occurredAt) {
			continue
		}
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
		for _, channel := range policy.Channels {
			ch := channel
			go d.deliver(ch, payload)
		}
	}
}

func shouldFire(policy store.AlertPolicy, prev, curr string) bool {
	p := normalize(prev)
	c := normalize(curr)
	// recovery: any non-healthy -> Healthy
	if policy.OnRecover && p != "Healthy" && p != "" && c == "Healthy" {
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

func (d *Dispatcher) deliver(channel store.AlertChannel, payload Payload) {
	switch channel.Type {
	case "webhook":
		if err := d.postWebhook(channel, payload); err != nil {
			d.logger.Warn("webhook delivery failed",
				"target", channel.Target,
				"monitor_id", payload.MonitorID,
				"group_id", payload.GroupID,
				"error", err.Error(),
			)
		}
	default:
		d.logger.Warn("unsupported alert channel type", "type", channel.Type)
	}
}

func (d *Dispatcher) postWebhook(channel store.AlertChannel, payload Payload) error {
	if channel.Target == "" {
		return errors.New("empty webhook target")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, channel.Target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range channel.Extra {
		req.Header.Set(key, value)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
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
