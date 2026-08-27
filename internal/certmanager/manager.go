package certmanager

import (
	"context"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Store is the narrow persistence contract certmanager uses for DNS provider
// accounts and future certificate resources.
type Store interface {
	ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]store.DNSProviderAccount, string, error)
	CreateDNSProviderAccount(ctx context.Context, account store.DNSProviderAccount) (store.DNSProviderAccount, error)
	GetDNSProviderAccount(ctx context.Context, id string) (store.DNSProviderAccount, error)
	UpdateDNSProviderAccount(ctx context.Context, id string, account store.DNSProviderAccount) (store.DNSProviderAccount, error)
	DeleteDNSProviderAccount(ctx context.Context, id string) error
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
	store    Store
	verifier TokenVerifier
	clock    Clock
}

func NewManager(st Store, verifier TokenVerifier) *Manager {
	return &Manager{store: st, verifier: verifier, clock: realClock{}}
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

func (m *Manager) CreateDNSProviderAccount(ctx context.Context, input DNSAccountInput) (PublicAccount, error) {
	account, err := m.buildAccount(input, store.DNSProviderAccount{})
	if err != nil {
		return PublicAccount{}, err
	}
	if input.APIToken == "" {
		return PublicAccount{}, ErrTokenRequired
	}
	if err := m.verifier.VerifyCloudflareToken(ctx, input.APIToken); err != nil {
		return PublicAccount{}, err
	}
	now := m.clock.Now()
	account.APIToken = input.APIToken
	account.TokenLastVerifiedAt = &now
	created, err := m.store.CreateDNSProviderAccount(ctx, account)
	if err != nil {
		return PublicAccount{}, err
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
		if err := m.verifier.VerifyCloudflareToken(ctx, input.APIToken); err != nil {
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
		return PublicAccount{}, err
	}
	return publicFromStore(updated), nil
}

func (m *Manager) DeleteDNSProviderAccount(ctx context.Context, id string) error {
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
