package alert

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/orvice/neo-line/internal/store"
)

// channelSender delivers one rendered payload to a single AlertChannel of its
// kind. Each supported channel type contributes one adapter at this seam; the
// dispatcher picks an adapter by channel type and stays ignorant of transport
// details. Tests substitute a recording adapter to exercise the full dispatch
// flow without HTTP.
type channelSender interface {
	send(channel store.AlertChannel, payload Payload) error
}

// builtinSenders returns the production adapters keyed by channel type. The
// empty type is not listed here; the dispatcher maps it to "webhook" before
// lookup.
func builtinSenders(client *http.Client) map[string]channelSender {
	p := &poster{client: client}
	return map[string]channelSender{
		"webhook":  &webhookSender{p},
		"telegram": &telegramSender{p},
		"discord":  &discordSender{p},
		"mastodon": &mastodonSender{p},
	}
}

// poster carries the HTTP plumbing shared by the built-in senders.
type poster struct {
	client *http.Client
}

// webhookSender POSTs the raw Payload JSON to the target URL. Extra entries
// become request headers.
type webhookSender struct{ *poster }

func (s *webhookSender) send(channel store.AlertChannel, payload Payload) error {
	if channel.Target == "" {
		return errors.New("empty webhook target")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Extra entries are applied after Content-Type so a channel may override it.
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	for key, value := range channel.Extra {
		header.Set(key, value)
	}
	return s.postRaw(channel.Target, header, body, "webhook")
}

// telegramSender sends the alert as a Bot API message. Target is the chat_id
// and extra["bot_token"] holds the bot token.
type telegramSender struct{ *poster }

func (s *telegramSender) send(channel store.AlertChannel, payload Payload) error {
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
	return s.postJSON(endpoint, nil, body, "telegram")
}

// discordSender posts to a Discord webhook URL given in Target.
type discordSender struct{ *poster }

func (s *discordSender) send(channel store.AlertChannel, payload Payload) error {
	if strings.TrimSpace(channel.Target) == "" {
		return errors.New("empty discord webhook url (target)")
	}
	body, err := json.Marshal(map[string]string{
		"content": formatMessage(payload),
	})
	if err != nil {
		return err
	}
	return s.postJSON(channel.Target, nil, body, "discord")
}

// mastodonSender publishes a status to a Mastodon instance. Target is the
// instance base URL (e.g. https://mastodon.social) and extra["access_token"]
// holds the application access token. Optional extra["visibility"] sets the
// status visibility (defaults to "unlisted").
type mastodonSender struct{ *poster }

func (s *mastodonSender) send(channel store.AlertChannel, payload Payload) error {
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
	return s.postRaw(endpoint, header, []byte(form.Encode()), "mastodon")
}

// postJSON sends body as application/json to endpoint with optional extra
// headers. label names the channel for error messages.
func (p *poster) postJSON(endpoint string, header http.Header, body []byte, label string) error {
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")
	return p.postRaw(endpoint, header, body, label)
}

func (p *poster) postRaw(endpoint string, header http.Header, body []byte, label string) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, values := range header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned status %d", label, resp.StatusCode)
	}
	return nil
}
