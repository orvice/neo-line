package notify

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func sampleDelivery() Delivery {
	payload := map[string]any{
		"monitor_id":      "mon1",
		"monitor_name":    "api-health",
		"server_id":       "srv1",
		"previous_status": "Healthy",
		"current_status":  "Down",
		"occurred_at":     time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"group_id":        "grp1",
		"group_name":      "prod",
	}
	body, _ := json.Marshal(payload)
	return Delivery{
		WebhookJSON: body,
		HumanText:   "[neo-line] api-health: Healthy → Down\nGroup: prod\nServer: srv1\nTime: 2026-05-30T10:00:00Z",
	}
}

func TestDeliverWebhookPostsRenderedJSON(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = append([]byte(nil), raw...)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.Client(), nil)
	d := sampleDelivery()
	if err := n.Deliver(storeAlertChannel("webhook", srv.URL, nil), d); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if !jsonEqual(gotBody, d.WebhookJSON) {
		t.Fatalf("webhook body mismatch:\ngot  %s\nwant %s", gotBody, d.WebhookJSON)
	}
}

func TestDeliverDiscordUsesHumanText(t *testing.T) {
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := New(srv.Client(), nil)
	d := sampleDelivery()
	if err := n.Deliver(storeAlertChannel("discord", srv.URL, nil), d); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if !strings.Contains(body["content"], "api-health") {
		t.Fatalf("discord content missing monitor name: %q", body["content"])
	}
}

func TestDeliverDiscordEmptyTarget(t *testing.T) {
	n := New(nil, nil)
	if err := n.Deliver(storeAlertChannel("discord", "", nil), sampleDelivery()); err == nil {
		t.Fatal("expected error for empty discord target")
	}
}

func TestDeliverMastodonUsesHumanText(t *testing.T) {
	var (
		status string
		auth   string
		path   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		status = form.Get("status")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.Client(), nil)
	d := sampleDelivery()
	ch := storeAlertChannel("mastodon", srv.URL, map[string]string{"access_token": "tok123"})
	if err := n.Deliver(ch, d); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if path != "/api/v1/statuses" {
		t.Errorf("path = %q, want /api/v1/statuses", path)
	}
	if auth != "Bearer tok123" {
		t.Errorf("authorization = %q, want Bearer tok123", auth)
	}
	if !strings.Contains(status, "api-health") {
		t.Errorf("status missing monitor name: %q", status)
	}
}

func TestDeliverMastodonMissingToken(t *testing.T) {
	n := New(nil, nil)
	ch := storeAlertChannel("mastodon", "https://example.social", nil)
	if err := n.Deliver(ch, sampleDelivery()); err == nil {
		t.Fatal("expected error for missing mastodon access_token")
	}
}

func TestDeliverTelegramValidation(t *testing.T) {
	n := New(nil, nil)
	d := sampleDelivery()
	if err := n.Deliver(storeAlertChannel("telegram", "", map[string]string{"bot_token": "t"}), d); err == nil {
		t.Fatal("expected error for empty telegram chat_id")
	}
	if err := n.Deliver(storeAlertChannel("telegram", "123", nil), d); err == nil {
		t.Fatal("expected error for missing telegram bot_token")
	}
}

func TestDeliverUnsupportedChannel(t *testing.T) {
	n := New(nil, nil)
	err := n.Deliver(storeAlertChannel("sms", "+123", nil), sampleDelivery())
	if err == nil {
		t.Fatal("expected error for unsupported channel")
	}
	if !errorsIsUnsupported(err) {
		t.Fatalf("error = %v, want ErrUnsupportedChannel", err)
	}
}

func TestDeliverEmptyTypeUsesWebhook(t *testing.T) {
	rec := &RecordingSender{}
	n := NewWithSenders(map[string]Sender{"webhook": rec}, nil)
	d := sampleDelivery()
	if err := n.Deliver(storeAlertChannel("", "http://example.test", nil), d); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if len(rec.Deliveries()) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(rec.Deliveries()))
	}
}

func TestRecordingSenderCapturesRenderedContent(t *testing.T) {
	rec := &RecordingSender{}
	n := NewWithSenders(map[string]Sender{"webhook": rec}, nil)
	d := Delivery{WebhookJSON: []byte(`{"event":"cert"}`), HumanText: "cert issued"}
	if err := n.Deliver(storeAlertChannel("webhook", "http://example.test", nil), d); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	got := rec.Deliveries()
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(got))
	}
	if string(got[0].WebhookJSON) != `{"event":"cert"}` {
		t.Errorf("webhook JSON = %q", got[0].WebhookJSON)
	}
	if got[0].HumanText != "cert issued" {
		t.Errorf("human text = %q", got[0].HumanText)
	}
}

func storeAlertChannel(typ, target string, extra map[string]string) store.AlertChannel {
	return store.AlertChannel{Type: typ, Target: target, Extra: extra}
}

func jsonEqual(a, b []byte) bool {
	var ja, jb any
	if json.Unmarshal(a, &ja) != nil || json.Unmarshal(b, &jb) != nil {
		return string(a) == string(b)
	}
	ab, _ := json.Marshal(ja)
	bb, _ := json.Marshal(jb)
	return string(ab) == string(bb)
}

func errorsIsUnsupported(err error) bool {
	return errors.Is(err, ErrUnsupportedChannel)
}
