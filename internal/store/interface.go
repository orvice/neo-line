package store

import (
	"context"
	"time"
)

// Store is the persistence contract for neo-line. MongoStore is the default
// implementation; alternative backends only need to satisfy this interface.
type Store interface {
	Close(ctx context.Context) error

	ListServers(ctx context.Context, environment string, tags []string, limit int64, pageToken string) ([]Server, string, error)
	CreateServer(ctx context.Context, server Server) (Server, error)
	GetServer(ctx context.Context, id string) (Server, error)
	UpdateServer(ctx context.Context, id string, server Server) (Server, error)
	DeleteServer(ctx context.Context, id string) error
	GetServerHealth(ctx context.Context, serverID string) (ServerHealth, error)
	ListServerEvents(ctx context.Context, serverID string, limit int64, pageToken string) ([]ServerEvent, string, error)

	ListMonitors(ctx context.Context, serverID string, limit int64, pageToken string) ([]Monitor, string, error)
	CreateMonitor(ctx context.Context, serverID string, monitor Monitor) (Monitor, error)
	GetMonitor(ctx context.Context, serverID, monitorID string) (Monitor, error)
	UpdateMonitor(ctx context.Context, serverID, monitorID string, monitor Monitor) (Monitor, error)
	DeleteMonitor(ctx context.Context, serverID, monitorID string) error
	ListEnabledMonitors(ctx context.Context) ([]Monitor, error)

	ListCheckResults(ctx context.Context, serverID, monitorID string, limit int64, pageToken string, start, end *time.Time) ([]CheckResult, string, error)
	SaveCheckResult(ctx context.Context, result CheckResult) error
	GetMonitorUptime(ctx context.Context, serverID, monitorID string) (MonitorUptime, error)

	EnsureAuthIndexes(ctx context.Context) error
	EnsureAdminUser(ctx context.Context, email, password string) error
	Authenticate(ctx context.Context, email, password string) (User, error)
	CreateSession(ctx context.Context, user User) (Session, error)
	GetSession(ctx context.Context, token string) (Session, error)
	DeleteSession(ctx context.Context, token string) error
}

var _ Store = (*MongoStore)(nil)
