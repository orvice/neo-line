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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Dispatcher fans out per-group notifications for monitor status
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
			d.logger.Debug("alert not fired by policy",
				"group_id", groupID, "monitor_id", monitor.ID,
				"prev", normalize(prev), "curr", normalize(curr))
			continue
		}
		if !d.allowThrottle(groupID, monitor.ID, policy.MinIntervalSeconds, occurredAt) {
			d.logger.Info("alert throttled",
				"group_id", groupID, "monitor_id", monitor.ID,
				"min_interval_seconds", policy.MinIntervalSeconds)
			continue
		}
		d.logger.Info("dispatching alert",
			"group_id", group.ID, "monitor_id", monitor.ID,
			"prev", normalize(prev), "curr", normalize(curr),
			"channels", len(policy.Channels))
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
	var err error
	switch channel.Type {
	case "", "webhook":
		err = d.postWebhook(channel, payload)
	case "telegram":
		err = d.postTelegram(channel, payload)
	case "discord":
		err = d.postDiscord(channel, payload)
	case "mastodon":
		err = d.postMastodon(channel, payload)
	default:
		d.logger.Warn("unsupported alert channel type", "type", channel.Type)
		return
	}
	if err != nil {
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

// postTelegram sends the alert as a Bot API message. Target is the chat_id and
// extra["bot_token"] holds the bot token.
func (d *Dispatcher) postTelegram(channel store.AlertChannel, payload Payload) error {
	chatID := strings.TrimSpace(channel.Target)
	if chatID == "" {
		return errors.New("empty telegram chat_id (target)")
	}
	token := strings.TrimSpace(channel.Extra["bot_token"])
	if token == "" {
		return errors.New("missing telegram bot_token in extra")
	}
	body, err := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    formatMessage(payload),
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	return d.postJSON(endpoint, nil, body, "telegram")
}

// postDiscord posts to a Discord webhook URL given in Target.
func (d *Dispatcher) postDiscord(channel store.AlertChannel, payload Payload) error {
	if strings.TrimSpace(channel.Target) == "" {
		return errors.New("empty discord webhook url (target)")
	}
	body, err := json.Marshal(map[string]string{
		"content": formatMessage(payload),
	})
	if err != nil {
		return err
	}
	return d.postJSON(channel.Target, nil, body, "discord")
}

// postMastodon publishes a status to a Mastodon instance. Target is the
// instance base URL (e.g. https://mastodon.social) and extra["access_token"]
// holds the application access token. Optional extra["visibility"] sets the
// status visibility (defaults to "unlisted").
func (d *Dispatcher) postMastodon(channel store.AlertChannel, payload Payload) error {
	base := strings.TrimRight(strings.TrimSpace(channel.Target), "/")
	if base == "" {
		return errors.New("empty mastodon instance url (target)")
	}
	token := strings.TrimSpace(channel.Extra["access_token"])
	if token == "" {
		return errors.New("missing mastodon access_token in extra")
	}
	visibility := strings.TrimSpace(channel.Extra["visibility"])
	if visibility == "" {
		visibility = "unlisted"
	}
	form := url.Values{}
	form.Set("status", formatMessage(payload))
	form.Set("visibility", visibility)
	endpoint := base + "/api/v1/statuses"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	return d.postRaw(endpoint, header, []byte(form.Encode()), "mastodon")
}

// postJSON sends body as application/json to endpoint with optional extra
// headers. label names the channel for error messages.
func (d *Dispatcher) postJSON(endpoint string, header http.Header, body []byte, label string) error {
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")
	return d.postRaw(endpoint, header, body, label)
}

func (d *Dispatcher) postRaw(endpoint string, header http.Header, body []byte, label string) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, values := range header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned status %d", label, resp.StatusCode)
	}
	return nil
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
