package notify

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

// BuiltinSenders returns production adapters keyed by channel type. The empty
// type is not listed; callers map it to "webhook" before lookup.
func BuiltinSenders(client *http.Client) map[string]Sender {
	p := &poster{client: client}
	return map[string]Sender{
		"webhook":  &webhookSender{p},
		"telegram": &telegramSender{p},
		"discord":  &discordSender{p},
		"mastodon": &mastodonSender{p},
	}
}

type poster struct {
	client *http.Client
}

type webhookSender struct{ *poster }

func (s *webhookSender) Send(channel store.AlertChannel, d Delivery) error {
	if channel.Target == "" {
		return errors.New("empty webhook target")
	}
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	for key, value := range channel.Extra {
		header.Set(key, value)
	}
	return s.postRaw(channel.Target, header, d.WebhookJSON, "webhook")
}

type telegramSender struct{ *poster }

func (s *telegramSender) Send(channel store.AlertChannel, d Delivery) error {
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
		"text":    d.HumanText,
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	return s.postJSON(endpoint, nil, body, "telegram")
}

type discordSender struct{ *poster }

func (s *discordSender) Send(channel store.AlertChannel, d Delivery) error {
	if strings.TrimSpace(channel.Target) == "" {
		return errors.New("empty discord webhook url (target)")
	}
	body, err := json.Marshal(map[string]string{
		"content": d.HumanText,
	})
	if err != nil {
		return err
	}
	return s.postJSON(channel.Target, nil, body, "discord")
}

type mastodonSender struct{ *poster }

func (s *mastodonSender) Send(channel store.AlertChannel, d Delivery) error {
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
	form.Set("status", d.HumanText)
	form.Set("visibility", visibility)
	endpoint := base + "/api/v1/statuses"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.postRaw(endpoint, header, []byte(form.Encode()), "mastodon")
}

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
