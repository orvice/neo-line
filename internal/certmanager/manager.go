package certmanager

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/orvice/neo-line/internal/certnotify"
	"github.com/orvice/neo-line/internal/store"
)

// Store is the narrow persistence contract certmanager uses for DNS provider
// accounts and certificate issuers.
type Store interface {
	ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]store.DNSProviderAccount, string, error)
	CreateDNSProviderAccount(ctx context.Context, account store.DNSProviderAccount) (store.DNSProviderAccount, error)
	GetDNSProviderAccount(ctx context.Context, id string) (store.DNSProviderAccount, error)
	UpdateDNSProviderAccount(ctx context.Context, id string, account store.DNSProviderAccount) (store.DNSProviderAccount, error)
	DeleteDNSProviderAccount(ctx context.Context, id string) error

	ListCertificateIssuers(ctx context.Context, limit int64, pageToken string) ([]store.CertificateIssuer, string, error)
	CreateCertificateIssuer(ctx context.Context, issuer store.CertificateIssuer) (store.CertificateIssuer, error)
	GetCertificateIssuer(ctx context.Context, id string) (store.CertificateIssuer, error)
	UpdateCertificateIssuer(ctx context.Context, id string, issuer store.CertificateIssuer) (store.CertificateIssuer, error)
	DeleteCertificateIssuer(ctx context.Context, id string) error

	ListManagedCertificates(ctx context.Context, limit int64, pageToken string) ([]store.ManagedCertificate, string, error)
	ListManagedCertificatesByServer(ctx context.Context, serverID string) ([]store.ManagedCertificate, error)
	CreateManagedCertificate(ctx context.Context, cert store.ManagedCertificate) (store.ManagedCertificate, error)
	GetManagedCertificate(ctx context.Context, id string) (store.ManagedCertificate, error)
	UpdateManagedCertificate(ctx context.Context, id string, update store.ManagedCertificateUpdate) (store.ManagedCertificate, error)
	DeleteManagedCertificate(ctx context.Context, id string) error

	CreateCertificateOperation(ctx context.Context, op store.CertificateOperation) (store.CertificateOperation, error)
	GetCertificateOperation(ctx context.Context, id string) (store.CertificateOperation, error)
	FindRunningCertificateOperation(ctx context.Context, managedCertificateID, opType string) (store.CertificateOperation, error)
	ListCertificateOperationsByCertificate(ctx context.Context, managedCertificateID string, limit int64) ([]store.CertificateOperation, error)
	LatestCertificateOperation(ctx context.Context, managedCertificateID string) (store.CertificateOperation, error)
	ValidateNotifyGroupIDs(ctx context.Context, ids []string) error
	ValidateServerIDs(ctx context.Context, ids []string) error

	FindClaimableCertificateOperations(ctx context.Context, now time.Time, limit int64) ([]store.CertificateOperation, error)
	TryClaimCertificateOperation(ctx context.Context, p store.CertificateOperationClaimParams) (store.CertificateOperation, error)
	RenewCertificateOperationLease(ctx context.Context, opID, owner string, leaseExpires, now time.Time) error
	RecordCertificateOperationPendingTXT(ctx context.Context, opID, owner string, record store.DNSChallengeRecord) error
	ScheduleCertificateOperationRetry(ctx context.Context, opID, owner string, nextAttemptAt time.Time, errorSummary string, consecutiveFailures uint32) error
	MarkCertificateOperationFailed(ctx context.Context, opID, owner, errorSummary string) error
	FailExpiredCertificateOperations(ctx context.Context, now time.Time) (int64, error)
	ClearCertificateOperationPendingTXT(ctx context.Context, opID string) error
	HasRunningCertificateOperation(ctx context.Context, managedCertificateID string) (bool, error)

	ListAutoRenewManagedCertificates(ctx context.Context) ([]store.ManagedCertificate, error)
	ActivateFirstIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, opID, leaseOwner, warning string) error
	ActivateSubsequentIssueVersion(ctx context.Context, managedCertID string, version store.CertificateVersion, expectedActiveID, opID, leaseOwner, warning string) error
	ActivatePreviousVersion(ctx context.Context, managedCertID, versionID string) error

	ListManagedCertificatesForNotifications(ctx context.Context) ([]store.ManagedCertificate, error)
	TryRecordOperationFailureNotification(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationFailureReminder(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationRecovery(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordSevenDayReminder(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	TryRecordExpiredNotification(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	SetCertificateNotificationWarning(ctx context.Context, certID, warning string, at time.Time) error
	GetNotifyGroup(ctx context.Context, id string) (store.NotifyGroup, error)

	MarkVersionRevokePending(ctx context.Context, managedCertID, versionID string) error
	ClearVersionRevokePending(ctx context.Context, managedCertID, versionID string) error
	CompleteRevokeVersion(ctx context.Context, managedCertID, versionID, opID, leaseOwner string, revokedAt time.Time) error
	CountManagedCertificatesReferencingIssuer(ctx context.Context, issuerID string) (int64, error)
	CountManagedCertificatesReferencingDNSAccount(ctx context.Context, dnsID string) (int64, error)
}

// TokenVerifier validates DNS provider API tokens before persistence.
type TokenVerifier interface {
	VerifyCloudflareToken(ctx context.Context, token string) error
}

// Clock supplies the current time; tests inject a fixed clock.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// Manager owns certificate-management business rules. Connect handlers stay
// thin and delegate here.
type Manager struct {
	store        Store
	verifier     TokenVerifier
	acme         ACMEClient
	dnsFactory   DNSProviderFactory
	clock        Clock
	logger       *slog.Logger
	replicaID    string
	jitter       JitterFunc
	certNotifier *certnotify.Dispatcher

	claimingLeases       atomic.Bool
	runnerCtx            context.Context
	runnerCtxMu          sync.RWMutex
	opInflight           sync.Map
	registrationInflight sync.Map
	taskMu               sync.Mutex
	tasksClosed          bool
	taskWG               sync.WaitGroup
}

func NewManager(st Store, verifier TokenVerifier) *Manager {
	return NewManagerWithACME(st, verifier, NewLegoACMEClient(nil))
}

func NewManagerWithACME(st Store, verifier TokenVerifier, acme ACMEClient) *Manager {
	return NewManagerWithDeps(st, verifier, acme, NewCloudflareDNSFactory(nil))
}

func NewManagerWithDeps(st Store, verifier TokenVerifier, acme ACMEClient, dnsFactory DNSProviderFactory) *Manager {
	logger := slog.Default().With("component", "certmanager")
	manager := &Manager{
		store:      st,
		verifier:   verifier,
		acme:       acme,
		dnsFactory: dnsFactory,
		clock:      realClock{},
		logger:     logger,
	}
	manager.SetLogger(logger)
	return manager
}

// SetLogger configures structured certificate-manager logging.
func (m *Manager) SetLogger(logger *slog.Logger) {
	if m == nil || logger == nil {
		return
	}
	m.logger = logger
	for _, component := range []any{m.verifier, m.acme} {
		if setter, ok := component.(interface{ SetLogger(*slog.Logger) }); ok {
			setter.SetLogger(logger)
		}
	}
}

func (m *Manager) runnerContext() (context.Context, bool) {
	m.runnerCtxMu.RLock()
	defer m.runnerCtxMu.RUnlock()
	if m.runnerCtx == nil || m.runnerCtx.Err() != nil {
		return nil, false
	}
	return m.runnerCtx, true
}

func (m *Manager) backgroundContext() context.Context {
	m.runnerCtxMu.RLock()
	defer m.runnerCtxMu.RUnlock()
	if m.runnerCtx != nil {
		return m.runnerCtx
	}
	return context.Background()
}

func (m *Manager) launchBackgroundTask(run func()) bool {
	m.taskMu.Lock()
	defer m.taskMu.Unlock()
	if m.tasksClosed {
		return false
	}
	m.taskWG.Add(1)
	go func() {
		defer m.taskWG.Done()
		run()
	}()
	return true
}

// SetCertNotifier wires certificate lifecycle notifications (optional in tests).
func (m *Manager) SetCertNotifier(n *certnotify.Dispatcher) {
	if m != nil {
		m.certNotifier = n
	}
}

// DNSAccountInput is the mutable DNS provider account fields supplied by API
// callers. APIToken empty on update means keep the existing secret.
type DNSAccountInput struct {
	Name                      string
	Provider                  string
	PropagationTimeoutSeconds uint32
	APIToken                  string
}

// PublicAccount is a DNSProviderAccount safe for API responses (no secrets).
type PublicAccount struct {
	ID                        string
	Name                      string
	Provider                  string
	PropagationTimeoutSeconds uint32
	TokenConfigured           bool
	TokenLastVerifiedAt       *time.Time
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

func publicFromStore(a store.DNSProviderAccount) PublicAccount {
	return PublicAccount{
		ID:                        a.ID,
		Name:                      a.Name,
		Provider:                  a.Provider,
		PropagationTimeoutSeconds: a.PropagationTimeoutSeconds,
		TokenConfigured:           a.APIToken != "",
		TokenLastVerifiedAt:       a.TokenLastVerifiedAt,
		CreatedAt:                 a.CreatedAt,
		UpdatedAt:                 a.UpdatedAt,
	}
}

func (m *Manager) ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]PublicAccount, string, error) {
	accounts, next, err := m.store.ListDNSProviderAccounts(ctx, limit, pageToken)
	if err != nil {
		return nil, "", err
	}
	out := make([]PublicAccount, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, publicFromStore(a))
	}
	return out, next, nil
}

func (m *Manager) GetDNSProviderAccount(ctx context.Context, id string) (PublicAccount, error) {
	account, err := m.store.GetDNSProviderAccount(ctx, id)
	if err != nil {
		return PublicAccount{}, err
	}
	return publicFromStore(account), nil
}

func (m *Manager) verifyDNSProviderToken(ctx context.Context, action, accountID, token string) error {
	attrs := []any{"dns_account_action", action}
	if accountID != "" {
		attrs = append(attrs, "dns_provider_account_id", accountID)
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "cloudflare token verification started", attrs...)
	}
	if err := m.verifier.VerifyCloudflareToken(ctx, token); err != nil {
		if m.logger != nil {
			m.logger.ErrorContext(ctx, "cloudflare token verification failed", append(attrs, safeErrorAttrs(err)...)...)
		}
		return err
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "cloudflare token verification succeeded", attrs...)
	}
	return nil
}

func (m *Manager) CreateDNSProviderAccount(ctx context.Context, input DNSAccountInput) (PublicAccount, error) {
	account, err := m.buildAccount(input, store.DNSProviderAccount{})
	if err != nil {
		return PublicAccount{}, err
	}
	if input.APIToken == "" {
		return PublicAccount{}, ErrTokenRequired
	}
	if err := m.verifyDNSProviderToken(ctx, "create", "", input.APIToken); err != nil {
		return PublicAccount{}, err
	}
	now := m.clock.Now()
	account.APIToken = input.APIToken
	account.TokenLastVerifiedAt = &now
	created, err := m.store.CreateDNSProviderAccount(ctx, account)
	if err != nil {
		if m.logger != nil {
			attrs := []any{"dns_account_action", "create", "error_stage", "persist_dns_provider_account"}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "persist DNS provider account failed", attrs...)
		}
		return PublicAccount{}, err
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "DNS provider account created",
			"dns_provider_account_id", created.ID,
			"dns_provider", created.Provider,
			"token_verified", created.TokenLastVerifiedAt != nil,
			"propagation_timeout_seconds", created.PropagationTimeoutSeconds,
		)
	}
	return publicFromStore(created), nil
}

func (m *Manager) UpdateDNSProviderAccount(ctx context.Context, id string, input DNSAccountInput) (PublicAccount, error) {
	existing, err := m.store.GetDNSProviderAccount(ctx, id)
	if err != nil {
		return PublicAccount{}, err
	}
	account, err := m.buildAccount(input, existing)
	if err != nil {
		return PublicAccount{}, err
	}
	rotateToken := input.APIToken != ""
	if rotateToken {
		if err := m.verifyDNSProviderToken(ctx, "rotate", id, input.APIToken); err != nil {
			return PublicAccount{}, err
		}
		now := m.clock.Now()
		account.APIToken = input.APIToken
		account.TokenLastVerifiedAt = &now
	} else {
		account.APIToken = existing.APIToken
		account.TokenLastVerifiedAt = existing.TokenLastVerifiedAt
	}
	updated, err := m.store.UpdateDNSProviderAccount(ctx, id, account)
	if err != nil {
		if m.logger != nil {
			attrs := []any{
				"dns_provider_account_id", id,
				"dns_account_action", "update",
				"error_stage", "persist_dns_provider_account",
			}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "persist DNS provider account update failed", attrs...)
		}
		return PublicAccount{}, err
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "DNS provider account updated",
			"dns_provider_account_id", updated.ID,
			"dns_provider", updated.Provider,
			"token_rotated", rotateToken,
			"token_verified", updated.TokenLastVerifiedAt != nil,
			"propagation_timeout_seconds", updated.PropagationTimeoutSeconds,
		)
	}
	return publicFromStore(updated), nil
}

func (m *Manager) DeleteDNSProviderAccount(ctx context.Context, id string) error {
	if err := m.ensureDNSAccountDeletable(ctx, id); err != nil {
		return err
	}
	return m.store.DeleteDNSProviderAccount(ctx, id)
}

func (m *Manager) buildAccount(input DNSAccountInput, existing store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	provider := input.Provider
	if provider == "" && existing.Provider != "" {
		provider = existing.Provider
	}
	if provider == "" {
		provider = store.DNSProviderCloudflare
	}
	if provider != store.DNSProviderCloudflare {
		return store.DNSProviderAccount{}, store.ErrInvalidDNSProvider
	}
	timeout := input.PropagationTimeoutSeconds
	if timeout == 0 {
		if existing.PropagationTimeoutSeconds != 0 {
			timeout = existing.PropagationTimeoutSeconds
		} else {
			timeout = store.DefaultDNSPropagationTimeoutSecs
		}
	}
	if timeout < store.MinDNSPropagationTimeoutSecs || timeout > store.MaxDNSPropagationTimeoutSecs {
		return store.DNSProviderAccount{}, store.ErrInvalidPropagationTimeout
	}
	return store.DNSProviderAccount{
		ID:                        existing.ID,
		Name:                      input.Name,
		Provider:                  provider,
		PropagationTimeoutSeconds: timeout,
		CreatedAt:                 existing.CreatedAt,
	}, nil
}
