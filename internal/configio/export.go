package configio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Export serialises all monitoring configuration into a version-1 JSON
// document. Computed and runtime-managed fields are never included; channel
// secrets are included in plaintext, so the result must be handled as a
// sensitive file.
func Export(ctx context.Context, st Store, now time.Time) ([]byte, error) {
	settings, err := st.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	f := File{Version: Version, ExportedAt: now.UTC(), Settings: settingsFromStore(settings)}

	notify, err := listAll(func(limit int64, token string) ([]store.NotifyGroup, string, error) {
		return st.ListNotifyGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list notify groups: %w", err)
	}
	for _, g := range notify {
		f.NotifyGroups = append(f.NotifyGroups, notifyGroupFromStore(g))
	}

	groups, err := listAll(func(limit int64, token string) ([]store.MonitorGroup, string, error) {
		return st.ListMonitorGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list monitor groups: %w", err)
	}
	for _, g := range groups {
		f.MonitorGroups = append(f.MonitorGroups, monitorGroupFromStore(g))
	}

	servers, err := listAll(func(limit int64, token string) ([]store.Server, string, error) {
		return st.ListServers(ctx, "", nil, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	for _, srv := range servers {
		monitors, err := listAll(func(limit int64, token string) ([]store.Monitor, string, error) {
			return st.ListMonitors(ctx, srv.ID, limit, token)
		})
		if err != nil {
			return nil, fmt.Errorf("list monitors of server %s: %w", srv.ID, err)
		}
		f.Servers = append(f.Servers, serverFromStore(srv, monitors))
	}

	return json.MarshalIndent(f, "", "  ")
}
