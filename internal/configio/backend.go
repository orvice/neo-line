package configio

import (
	"context"

	"github.com/orvice/neo-line/internal/store"
)

// Store is the subset of store.Store that configio needs. store.Store (and so
// MongoStore) satisfies it; tests use a small in-memory fake.
type Store interface {
	GetSettings(ctx context.Context) (store.Settings, error)
	UpdateSettings(ctx context.Context, settings store.Settings) (store.Settings, error)

	ListNotifyGroups(ctx context.Context, limit int64, pageToken string) ([]store.NotifyGroup, string, error)
	CreateNotifyGroup(ctx context.Context, group store.NotifyGroup) (store.NotifyGroup, error)
	UpdateNotifyGroup(ctx context.Context, id string, group store.NotifyGroup) (store.NotifyGroup, error)

	ListMonitorGroups(ctx context.Context, limit int64, pageToken string) ([]store.MonitorGroup, string, error)
	CreateMonitorGroup(ctx context.Context, group store.MonitorGroup) (store.MonitorGroup, error)
	UpdateMonitorGroup(ctx context.Context, id string, group store.MonitorGroup) (store.MonitorGroup, error)

	ListServers(ctx context.Context, environment string, tags []string, limit int64, pageToken string) ([]store.Server, string, error)
	CreateServer(ctx context.Context, server store.Server) (store.Server, error)
	UpdateServer(ctx context.Context, id string, server store.Server) (store.Server, error)

	ListMonitors(ctx context.Context, serverID string, limit int64, pageToken string) ([]store.Monitor, string, error)
	CreateMonitor(ctx context.Context, serverID string, monitor store.Monitor) (store.Monitor, error)
	UpdateMonitor(ctx context.Context, serverID, monitorID string, monitor store.Monitor) (store.Monitor, error)
}

// Compile-time check that the full store satisfies the narrow interface.
var _ Store = store.Store(nil)

// listAll drains a paginated list endpoint using the maximum page size.
func listAll[T any](fetch func(limit int64, token string) ([]T, string, error)) ([]T, error) {
	var out []T
	token := ""
	for {
		items, next, err := fetch(200, token)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			return out, nil
		}
		token = next
	}
}
