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

	ListMonitorGroups(ctx context.Context, limit int64, pageToken string) ([]MonitorGroup, string, error)
	CreateMonitorGroup(ctx context.Context, group MonitorGroup) (MonitorGroup, error)
	GetMonitorGroup(ctx context.Context, id string) (MonitorGroup, error)
	UpdateMonitorGroup(ctx context.Context, id string, group MonitorGroup) (MonitorGroup, error)
	DeleteMonitorGroup(ctx context.Context, id string) error
	ListMonitorsByGroup(ctx context.Context, groupID string, limit int64, pageToken string) ([]Monitor, string, error)
	ListGroupsForMonitor(ctx context.Context, monitorID string) ([]MonitorGroup, error)

	ListNotifyGroups(ctx context.Context, limit int64, pageToken string) ([]NotifyGroup, string, error)
	CreateNotifyGroup(ctx context.Context, group NotifyGroup) (NotifyGroup, error)
	GetNotifyGroup(ctx context.Context, id string) (NotifyGroup, error)
	UpdateNotifyGroup(ctx context.Context, id string, group NotifyGroup) (NotifyGroup, error)
	DeleteNotifyGroup(ctx context.Context, id string) error

	ListCheckResults(ctx context.Context, serverID, monitorID string, limit int64, pageToken string, start, end *time.Time) ([]CheckResult, string, error)
	// SaveCheckResult persists a probe result and returns the monitor's prior
	// status (empty when no prior status existed).
	SaveCheckResult(ctx context.Context, result CheckResult) (string, error)
	GetMonitorUptime(ctx context.Context, serverID, monitorID string) (MonitorUptime, error)

	GetSettings(ctx context.Context) (Settings, error)
	UpdateSettings(ctx context.Context, settings Settings) (Settings, error)

	EnsureServerIndexes(ctx context.Context) error
	EnsureMonitorIndexes(ctx context.Context) error
	EnsureAuditIndexes(ctx context.Context) error
	EnsureAuthIndexes(ctx context.Context) error
	EnsureGroupIndexes(ctx context.Context) error
	EnsureNotifyGroupIndexes(ctx context.Context) error
	EnsureMcpTokenIndexes(ctx context.Context) error
	EnsureResultIndexes(ctx context.Context) error

	CacheGet(ctx context.Context, key string) ([]byte, bool, error)
	CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error

	ListMcpTokens(ctx context.Context) ([]McpToken, error)
	CreateMcpToken(ctx context.Context, name string) (McpToken, string, error)
	DeleteMcpToken(ctx context.Context, id string) error
	CountMcpTokens(ctx context.Context) (int64, error)
	ValidateMcpToken(ctx context.Context, plaintext string) (bool, error)
	EnsureAdminUser(ctx context.Context, email, password string) error
	Authenticate(ctx context.Context, email, password string) (User, error)
	CreateSession(ctx context.Context, user User) (Session, error)
	GetSession(ctx context.Context, token string) (Session, error)
	DeleteSession(ctx context.Context, token string) error
}

var _ Store = (*MongoStore)(nil)
