package configio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestExportOmitsComputedFieldsAndKeepsSecrets(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	fs := &fakeStore{
		notify: []store.NotifyGroup{{
			ID: "ntf_1", Name: "ops",
			Channels: []store.AlertChannel{{Type: "telegram", Target: "123", Extra: map[string]string{"bot_token": "s3cret"}}},
		}},
		servers: []store.Server{{
			ID: "srv_1", Name: "web-1", Host: "10.0.0.1", Enabled: true,
			HealthStatus: store.StatusDown, LastCheckAt: now, LastStatusChangeAt: now, CreatedAt: now, UpdatedAt: now,
		}},
		monitors: []store.Monitor{{
			ID: "mon_1", ServerID: "srv_1", Name: "api", Kind: "url", Enabled: true, URL: "https://example.com",
			Status: store.StatusDown, LastCheckAt: now, LastStatusChangeAt: now, CreatedAt: now, UpdatedAt: now,
			Certificate: &store.CertificateInfo{Subject: "CN=example.com"},
		}},
	}

	data, err := Export(context.Background(), fs, now)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	out := string(data)

	for _, banned := range []string{"health_status", `"status"`, "certificate", "created_at", "updated_at", "last_check_at", "last_status_change_at", "server_id"} {
		if strings.Contains(out, banned) {
			t.Errorf("export must not contain %q:\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "s3cret") {
		t.Errorf("export must include channel secrets in plaintext:\n%s", out)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("exported JSON invalid: %v", err)
	}
	if f.Version != Version || !f.ExportedAt.Equal(now) {
		t.Fatalf("header = version %d exported_at %v", f.Version, f.ExportedAt)
	}
	if len(f.Servers) != 1 || len(f.Servers[0].Monitors) != 1 || f.Servers[0].Monitors[0].ID != "mon_1" {
		t.Fatalf("monitors must be nested under their server: %+v", f.Servers)
	}
}

func TestExportPaginatesAllResources(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("srv_%d", i)
		fs.servers = append(fs.servers, store.Server{ID: id, Name: id, Host: "h"})
		for j := 0; j < 3; j++ {
			fs.monitors = append(fs.monitors, store.Monitor{ID: fmt.Sprintf("mon_%d_%d", i, j), ServerID: id, Name: "m", Kind: "tcp"})
		}
		fs.notify = append(fs.notify, store.NotifyGroup{ID: fmt.Sprintf("ntf_%d", i), Name: fmt.Sprintf("n%d", i)})
		fs.groups = append(fs.groups, store.MonitorGroup{ID: fmt.Sprintf("grp_%d", i), Name: fmt.Sprintf("g%d", i)})
	}

	data, err := Export(context.Background(), fs, time.Now())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Servers) != 5 || len(f.NotifyGroups) != 5 || len(f.MonitorGroups) != 5 {
		t.Fatalf("got %d servers, %d notify groups, %d groups; want 5 each", len(f.Servers), len(f.NotifyGroups), len(f.MonitorGroups))
	}
	for _, s := range f.Servers {
		if len(s.Monitors) != 3 {
			t.Fatalf("server %s has %d monitors, want 3", s.ID, len(s.Monitors))
		}
	}
}
