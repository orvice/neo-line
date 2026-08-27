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

	ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]DNSProviderAccount, string, error)
	CreateDNSProviderAccount(ctx context.Context, account DNSProviderAccount) (DNSProviderAccount, error)
	GetDNSProviderAccount(ctx context.Context, id string) (DNSProviderAccount, error)
	UpdateDNSProviderAccount(ctx context.Context, id string, account DNSProviderAccount) (DNSProviderAccount, error)
	DeleteDNSProviderAccount(ctx context.Context, id string) error

	ListCertificateIssuers(ctx context.Context, limit int64, pageToken string) ([]CertificateIssuer, string, error)
	CreateCertificateIssuer(ctx context.Context, issuer CertificateIssuer) (CertificateIssuer, error)
	GetCertificateIssuer(ctx context.Context, id string) (CertificateIssuer, error)
	UpdateCertificateIssuer(ctx context.Context, id string, issuer CertificateIssuer) (CertificateIssuer, error)
	DeleteCertificateIssuer(ctx context.Context, id string) error

	ListManagedCertificates(ctx context.Context, limit int64, pageToken string) ([]ManagedCertificate, string, error)
	ListManagedCertificatesByServer(ctx context.Context, serverID string) ([]ManagedCertificate, error)
	CreateManagedCertificate(ctx context.Context, cert ManagedCertificate) (ManagedCertificate, error)
	GetManagedCertificate(ctx context.Context, id string) (ManagedCertificate, error)
	UpdateManagedCertificate(ctx context.Context, id string, update ManagedCertificateUpdate) (ManagedCertificate, error)
	DeleteManagedCertificate(ctx context.Context, id string) error
	MarkVersionRevokePending(ctx context.Context, managedCertID, versionID string) error
	ClearVersionRevokePending(ctx context.Context, managedCertID, versionID string) error
	CompleteRevokeVersion(ctx context.Context, managedCertID, versionID, opID, leaseOwner string, revokedAt time.Time) error
	CountManagedCertificatesReferencingIssuer(ctx context.Context, issuerID string) (int64, error)
	CountManagedCertificatesReferencingDNSAccount(ctx context.Context, dnsID string) (int64, error)

	CreateCertificateOperation(ctx context.Context, op CertificateOperation) (CertificateOperation, error)
	GetCertificateOperation(ctx context.Context, id string) (CertificateOperation, error)
	FindRunningCertificateOperation(ctx context.Context, managedCertificateID, opType string) (CertificateOperation, error)
	HasRunningCertificateOperation(ctx context.Context, managedCertificateID string) (bool, error)
	ListCertificateOperationsByCertificate(ctx context.Context, managedCertificateID string, limit int64) ([]CertificateOperation, error)
	LatestCertificateOperation(ctx context.Context, managedCertificateID string) (CertificateOperation, error)
	FindClaimableCertificateOperations(ctx context.Context, now time.Time, limit int64) ([]CertificateOperation, error)
	TryClaimCertificateOperation(ctx context.Context, p CertificateOperationClaimParams) (CertificateOperation, error)
	RenewCertificateOperationLease(ctx context.Context, opID, owner string, leaseExpires, now time.Time) error
	RecordCertificateOperationPendingTXT(ctx context.Context, opID, owner string, record DNSChallengeRecord) error
	ScheduleCertificateOperationRetry(ctx context.Context, opID, owner string, nextAttemptAt time.Time, errorSummary string, consecutiveFailures uint32) error
	MarkCertificateOperationFailed(ctx context.Context, opID, owner, errorSummary string) error
	FailExpiredCertificateOperations(ctx context.Context, now time.Time) (int64, error)
	ClearCertificateOperationPendingTXT(ctx context.Context, opID string) error
	ListAutoRenewManagedCertificates(ctx context.Context) ([]ManagedCertificate, error)
	ActivateFirstIssueVersion(ctx context.Context, managedCertID string, version CertificateVersion, opID, leaseOwner, warning string) error
	ActivateSubsequentIssueVersion(ctx context.Context, managedCertID string, version CertificateVersion, expectedActiveID, opID, leaseOwner, warning string) error
	ActivatePreviousVersion(ctx context.Context, managedCertID, versionID string) error
	ValidateNotifyGroupIDs(ctx context.Context, ids []string) error
	ValidateServerIDs(ctx context.Context, ids []string) error

	ListManagedCertificatesForNotifications(ctx context.Context) ([]ManagedCertificate, error)
	TryRecordOperationFailureNotification(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationFailureReminder(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationRecovery(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordSevenDayReminder(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	TryRecordExpiredNotification(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	SetCertificateNotificationWarning(ctx context.Context, certID, warning string, at time.Time) error

	ListCheckResults(ctx context.Context, serverID, monitorID string, limit int64, pageToken string, start, end *time.Time) ([]CheckResult, string, error)
	// SaveCheckResult persists a probe result and returns the monitor's prior
	// status (empty when no prior status existed).
	SaveCheckResult(ctx context.Context, result CheckResult) (string, error)
	GetMonitorUptime(ctx context.Context, serverID, monitorID string) (MonitorUptime, error)

	GetSettings(ctx context.Context) (Settings, error)
	UpdateSettings(ctx context.Context, settings Settings) (Settings, error)

	ListAuditLogs(ctx context.Context, filter AuditLogFilter, limit int64, pageToken string) ([]AuditLog, string, error)

	EnsureServerIndexes(ctx context.Context) error
	EnsureMonitorIndexes(ctx context.Context) error
	EnsureAuditIndexes(ctx context.Context) error
	EnsureAuthIndexes(ctx context.Context) error
	EnsureGroupIndexes(ctx context.Context) error
	EnsureNotifyGroupIndexes(ctx context.Context) error
	EnsureDNSProviderAccountIndexes(ctx context.Context) error
	EnsureCertificateIssuerIndexes(ctx context.Context) error
	EnsureManagedCertificateIndexes(ctx context.Context) error
	EnsureCertificateOperationIndexes(ctx context.Context) error
	EnsureMcpTokenIndexes(ctx context.Context) error
	EnsureCertificateAccessTokenIndexes(ctx context.Context) error
	EnsureResultIndexes(ctx context.Context) error

	CacheGet(ctx context.Context, key string) ([]byte, bool, error)
	CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error

	ListMcpTokens(ctx context.Context) ([]McpToken, error)
	CreateMcpToken(ctx context.Context, name string) (McpToken, string, error)
	DeleteMcpToken(ctx context.Context, id string) error
	CountMcpTokens(ctx context.Context) (int64, error)
	ValidateMcpToken(ctx context.Context, plaintext string) (bool, error)

	ListCertificateAccessTokensByServer(ctx context.Context, serverID string) ([]CertificateAccessToken, error)
	CreateCertificateAccessToken(ctx context.Context, serverID, name string, expiresAt *time.Time) (CertificateAccessToken, string, error)
	DeleteCertificateAccessToken(ctx context.Context, serverID, tokenID string) error
	LookupCertificateAccessToken(ctx context.Context, plaintext string) (CertificateAccessToken, error)
	ValidateCertificateAccessToken(ctx context.Context, serverID, plaintext string) (bool, error)

	RateLimitAllow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, err error)

	EnsureAdminUser(ctx context.Context, email, password string) error
	Authenticate(ctx context.Context, email, password string) (User, error)
	CreateSession(ctx context.Context, user User) (Session, error)
	GetSession(ctx context.Context, token string) (Session, error)
	DeleteSession(ctx context.Context, token string) error
}

var _ Store = (*MongoStore)(nil)
