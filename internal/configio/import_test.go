package configio

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func sampleFile() File {
	return File{
		Version:  Version,
		Settings: &Settings{SiteName: "Prod", StatusPageTitle: "Status"},
		NotifyGroups: []NotifyGroup{{
			ID: "ntf_1", Name: "ops",
			Channels: []Channel{{Type: "telegram", Target: "123", Extra: map[string]string{"bot_token": "s3cret"}}},
		}},
		MonitorGroups: []MonitorGroup{{
			ID: "grp_1", Name: "edge", SortOrder: 1,
			AlertPolicy: AlertPolicy{Enabled: true, NotifyGroupIDs: []string{"ntf_1"}, OnDown: true},
		}},
		Servers: []Server{
			{
				ID: "srv_1", Name: "web-1", Host: "10.0.0.1", Enabled: true, Tags: []string{"prod"},
				SSH: &SSH{Enabled: true, User: "ops"},
				Monitors: []Monitor{
					{ID: "mon_1", Name: "api", Kind: "url", Enabled: true, URL: "https://example.com/health", GroupIDs: []string{"grp_1"}, TLSVerify: true, IntervalSeconds: 60, TimeoutSeconds: 5, Retries: 3},
					{ID: "mon_2", Name: "ssh", Kind: "tcp", Enabled: true, Port: 22, IntervalSeconds: 60, TimeoutSeconds: 5, Retries: 3},
				},
			},
			{ID: "srv_2", Name: "web-2", Host: "10.0.0.2", Enabled: true},
		},
	}
}

func mustJSON(t *testing.T, f File) []byte {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func runImport(t *testing.T, fs *fakeStore, f File, opts Options) Report {
	t.Helper()
	report, err := Import(context.Background(), fs, mustJSON(t, f), opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return report
}

func TestImportThenExportRoundTrip(t *testing.T) {
	fs := &fakeStore{}
	want := sampleFile()

	report := runImport(t, fs, want, Options{Mode: ModeUpsert})
	wantSummary := Summary{
		Settings:      Counts{Updated: 1},
		NotifyGroups:  Counts{Created: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 2},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != wantSummary || len(report.Errors) != 0 {
		t.Fatalf("summary = %+v errors = %+v, want %+v", report.Summary, report.Errors, wantSummary)
	}

	data, err := Export(context.Background(), fs, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := decode(data)
	if err != nil {
		t.Fatalf("exported file must be importable: %v", err)
	}
	got.ExportedAt = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestImportMergeSkipsExistingAndCreatesMissing(t *testing.T) {
	fs := &fakeStore{
		notify:  []store.NotifyGroup{{ID: "ntf_1", Name: "ops-old"}},
		servers: []store.Server{{ID: "srv_1", Name: "old-name", Host: "old", HealthStatus: store.StatusHealthy}},
	}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeMerge})

	want := Summary{
		Settings:      Counts{Skipped: 1},
		NotifyGroups:  Counts{Skipped: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 1, Skipped: 1},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != want || len(report.Errors) != 0 {
		t.Fatalf("summary = %+v errors = %+v, want %+v", report.Summary, report.Errors, want)
	}
	if fs.servers[0].Name != "old-name" || fs.notify[0].Name != "ops-old" {
		t.Fatalf("MERGE must not overwrite existing resources: %+v %+v", fs.servers[0], fs.notify[0])
	}
	if fs.settings.SiteName != "" {
		t.Fatalf("MERGE must not overwrite settings: %+v", fs.settings)
	}
}

func TestImportUpsertOverwritesButKeepsComputedFields(t *testing.T) {
	fs := &fakeStore{
		servers:  []store.Server{{ID: "srv_1", Name: "old-name", Host: "old", HealthStatus: store.StatusDown}},
		monitors: []store.Monitor{{ID: "mon_1", ServerID: "srv_1", Name: "old", Kind: "url", Status: store.StatusDown}},
		groups:   []store.MonitorGroup{{ID: "grp_1", Name: "edge"}},
		notify:   []store.NotifyGroup{{ID: "ntf_1", Name: "ops"}},
	}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert})

	if report.Summary.Servers.Updated != 1 || report.Summary.Monitors.Updated != 1 || report.Summary.Monitors.Created != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	if fs.servers[0].Name != "web-1" || fs.servers[0].HealthStatus != store.StatusDown {
		t.Fatalf("server must be overwritten but keep health status: %+v", fs.servers[0])
	}
	if fs.monitors[0].Name != "api" || fs.monitors[0].Status != store.StatusDown {
		t.Fatalf("monitor must be overwritten but keep status: %+v", fs.monitors[0])
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	fs := &fakeStore{}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert, DryRun: true})

	if fs.writes != 0 {
		t.Fatalf("dry run performed %d writes", fs.writes)
	}
	if !report.DryRun {
		t.Fatal("report.DryRun must be true")
	}
	want := Summary{
		Settings:      Counts{Updated: 1},
		NotifyGroups:  Counts{Created: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 2},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
}

func TestImportNameConflict(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        Mode
		wantSkipped uint32
		wantFailed  uint32
	}{
		{"merge skips and reports", ModeMerge, 1, 0},
		{"upsert fails", ModeUpsert, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{notify: []store.NotifyGroup{{ID: "ntf_other", Name: "ops"}}}

			report := runImport(t, fs, sampleFile(), Options{Mode: tc.mode})

			nc := report.Summary.NotifyGroups
			if nc.Skipped != tc.wantSkipped || nc.Failed != tc.wantFailed || nc.Created != 0 {
				t.Fatalf("notify groups = %+v", nc)
			}
			if len(fs.notify) != 1 {
				t.Fatalf("conflicting notify group must not be written: %+v", fs.notify)
			}
			found := false
			for _, e := range report.Errors {
				if e.ResourceType == ResourceNotifyGroup && e.ID == "ntf_1" && strings.Contains(e.Message, "ntf_other") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a conflict issue naming the owner, got %+v", report.Errors)
			}
			// The group referencing the unavailable notify group, and the
			// monitor referencing that group, cascade into failures.
			if report.Summary.MonitorGroups.Failed != 1 {
				t.Fatalf("monitor groups = %+v", report.Summary.MonitorGroups)
			}
			if report.Summary.Monitors.Failed != 1 || report.Summary.Monitors.Created != 1 {
				t.Fatalf("monitors = %+v", report.Summary.Monitors)
			}
		})
	}
}

func TestImportItemValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*File)
		check  func(*testing.T, Summary)
	}{
		{
			name: "duplicate server id fails the server and its monitors",
			mutate: func(f *File) {
				f.Servers = append(f.Servers, Server{ID: "srv_1", Name: "dup", Host: "h", Monitors: []Monitor{{ID: "mon_9", Name: "x", Kind: "tcp"}}})
			},
			check: func(t *testing.T, s Summary) {
				if s.Servers.Failed != 1 || s.Servers.Created != 2 || s.Monitors.Failed != 1 || s.Monitors.Created != 2 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "missing monitor id",
			mutate: func(f *File) { f.Servers[0].Monitors[0].ID = "" },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "unsupported monitor kind",
			mutate: func(f *File) { f.Servers[0].Monitors[1].Kind = "icmp" },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "monitor references unknown group",
			mutate: func(f *File) { f.Servers[0].Monitors[1].GroupIDs = []string{"grp_missing"} },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name: "monitor group references unknown notify group",
			mutate: func(f *File) {
				f.MonitorGroups[0].AlertPolicy.NotifyGroupIDs = []string{"ntf_missing"}
			},
			check: func(t *testing.T, s Summary) {
				if s.MonitorGroups.Failed != 1 || s.Monitors.Failed != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name: "duplicate group name in file",
			mutate: func(f *File) {
				f.MonitorGroups = append(f.MonitorGroups, MonitorGroup{ID: "grp_2", Name: "edge"})
			},
			check: func(t *testing.T, s Summary) {
				if s.MonitorGroups.Created != 1 || s.MonitorGroups.Failed != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := sampleFile()
			tc.mutate(&f)
			report := runImport(t, &fakeStore{}, f, Options{Mode: ModeUpsert})
			tc.check(t, report.Summary)
			if len(report.Errors) == 0 {
				t.Fatal("expected at least one issue in the report")
			}
		})
	}
}

func TestImportWriteFailureIsReportedAndContinues(t *testing.T) {
	fs := &fakeStore{failIDs: map[string]error{"mon_1": errors.New("boom")}}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert})

	if report.Summary.Monitors.Failed != 1 || report.Summary.Monitors.Created != 1 {
		t.Fatalf("monitors = %+v", report.Summary.Monitors)
	}
	for _, e := range report.Errors {
		if strings.Contains(e.Message, "boom") {
			t.Fatalf("raw store errors must not leak into the report: %+v", e)
		}
	}
}

func TestImportIgnoresComputedFields(t *testing.T) {
	withComputed := []byte(`{
  "version": 1,
  "servers": [{
    "id": "srv_1",
    "name": "web-1",
    "host": "10.0.0.1",
    "enabled": true,
    "health_status": "Down",
    "last_status_change_at": "2026-01-01T00:00:00Z",
    "last_check_at": "2026-01-01T00:00:00Z",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
    "monitors": [{
      "id": "mon_1",
      "name": "ssh",
      "kind": "tcp",
      "port": 22,
      "interval_seconds": 60,
      "timeout_seconds": 5,
      "retries": 3,
      "status": "Down",
      "last_check_at": "2026-01-01T00:00:00Z",
      "last_status_change_at": "2026-01-01T00:00:00Z",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z",
      "certificate": {"subject": "CN=x"}
    }]
  }]
}`)
	fs := &fakeStore{}
	report, err := Import(context.Background(), fs, withComputed, Options{Mode: ModeUpsert})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Summary.Servers.Created != 1 || report.Summary.Monitors.Created != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	if len(fs.servers) != 1 {
		t.Fatalf("servers = %+v", fs.servers)
	}
	if fs.servers[0].HealthStatus != store.StatusUnknown {
		t.Fatalf("HealthStatus = %q, want %q", fs.servers[0].HealthStatus, store.StatusUnknown)
	}
	if len(fs.monitors) != 1 {
		t.Fatalf("monitors = %+v", fs.monitors)
	}
	if fs.monitors[0].Status != store.StatusUnknown || fs.monitors[0].Certificate != nil {
		t.Fatalf("monitor status/cert = %q %v, want Unknown nil", fs.monitors[0].Status, fs.monitors[0].Certificate)
	}

	withBogus := []byte(`{"version":1,"servers":[{"id":"srv_1","name":"n","host":"h","enabled":true,"bogus":1}]}`)
	_, err = Import(context.Background(), fs, withBogus, Options{Mode: ModeUpsert})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bogus field err = %v, want ErrInvalidInput", err)
	}
}

func TestImportRejectsInvalidInput(t *testing.T) {
	valid := mustJSON(t, File{Version: Version})
	for _, tc := range []struct {
		name string
		data []byte
		mode Mode
	}{
		{"mode unspecified", valid, 0},
		{"malformed json", []byte("{"), ModeMerge},
		{"missing version", []byte(`{}`), ModeMerge},
		{"future version", []byte(`{"version":2}`), ModeMerge},
		{"unknown field", []byte(`{"version":1,"bogus":true}`), ModeMerge},
		{"trailing data", append(append([]byte{}, valid...), []byte(`{}`)...), ModeMerge},
		{"oversize", make([]byte, MaxImportBytes+1), ModeMerge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Import(context.Background(), &fakeStore{}, tc.data, Options{Mode: tc.mode})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
