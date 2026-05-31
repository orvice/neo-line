package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/store"
)

// statusFakeStore embeds store.Store and implements only what getStatusOverview
// reads, leaving everything else unimplemented.
type statusFakeStore struct {
	store.Store
	groups          []store.MonitorGroup
	monitorsByGroup map[string][]store.Monitor
	serversByID     map[string]store.Server
	uptime          store.MonitorUptime
}

func (s *statusFakeStore) ListMonitorGroups(context.Context, int64, string) ([]store.MonitorGroup, string, error) {
	return s.groups, "", nil
}

func (s *statusFakeStore) ListMonitorsByGroup(_ context.Context, groupID string, _ int64, _ string) ([]store.Monitor, string, error) {
	return s.monitorsByGroup[groupID], "", nil
}

func (s *statusFakeStore) GetServer(_ context.Context, id string) (store.Server, error) {
	return s.serversByID[id], nil
}

func (s *statusFakeStore) GetMonitorUptime(context.Context, string, string) (store.MonitorUptime, error) {
	return s.uptime, nil
}

func TestGetStatusOverview(t *testing.T) {
	st := &statusFakeStore{
		groups: []store.MonitorGroup{{ID: "g1", Name: "Core", Description: "core services", SortOrder: 1}},
		monitorsByGroup: map[string][]store.Monitor{
			"g1": {
				{
					ID: "m1", ServerID: "s1", Name: "api", Kind: "url", Status: store.StatusHealthy, Enabled: true,
					Host: "10.0.0.5", Port: 8443, URL: "https://internal.example.com/health",
					Headers:     map[string]string{"Authorization": "Bearer secret"},
					SNIName:     "internal.example.com",
					Certificate: &store.CertificateInfo{Subject: "CN=internal", DNSNames: []string{"internal.example.com"}, DaysRemaining: 20},
				},
				{ID: "m2", ServerID: "s1", Name: "disabled", Kind: "tcp", Enabled: false},
			},
		},
		serversByID: map[string]store.Server{
			"s1": {ID: "s1", Name: "edge", Host: "10.0.0.5", Environment: "prod", SSH: &store.ServerSSH{Enabled: true, User: "root"}},
		},
		uptime: store.MonitorUptime{Windows: map[string]store.UptimeWindow{"24h": {Total: 10, Up: 10, Uptime: 1}}},
	}

	api := &API{store: st}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)

	api.getStatusOverview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	for _, leak := range []string{"10.0.0.5", "internal.example.com", "8443", "Bearer secret", "CN=internal", "root"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaked sensitive value %q: %s", leak, body)
		}
	}

	var got publicStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(got.Groups))
	}
	g := got.Groups[0]
	if g.Name != "Core" || len(g.Servers) != 1 {
		t.Fatalf("unexpected group: %#v", g)
	}
	srv := g.Servers[0]
	if srv.Name != "edge" || srv.Environment != "prod" {
		t.Fatalf("unexpected server: %#v", srv)
	}
	if len(srv.Monitors) != 1 {
		t.Fatalf("monitors = %d, want 1 (disabled filtered out)", len(srv.Monitors))
	}
	m := srv.Monitors[0]
	if m.Name != "api" || m.Status != store.StatusHealthy {
		t.Fatalf("unexpected monitor: %#v", m)
	}
	if m.Certificate == nil || m.Certificate.DaysRemaining != 20 {
		t.Fatalf("certificate timing missing: %#v", m.Certificate)
	}
	if m.Uptime.Windows["24h"].Uptime != 1 {
		t.Fatalf("uptime not attached: %#v", m.Uptime)
	}
}
