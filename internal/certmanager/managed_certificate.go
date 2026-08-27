package certmanager

import (
	"context"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// ManagedCertificateInput is the mutable desired-config fields from API callers.
type ManagedCertificateInput struct {
	Name                 string
	Domains              []string
	CertificateIssuerID  string
	DNSProviderAccountID string
	KeyType              string
	AutoRenewEnabled     *bool
	RenewBeforeDays      uint32
	NotifyGroupIDs       []string
	ServerIDs            []string
}

// PublicOperation is a CertificateOperation safe for API responses.
type PublicOperation struct {
	ID                   string
	ManagedCertificateID string
	Type                 string
	Status               string
	AttemptCount         uint32
	ConfigSnapshot       store.IssueConfigSnapshot
	ErrorSummary         string
	Warning              string
	StartedAt            *time.Time
	FinishedAt           *time.Time
	NextAttemptAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// PublicManagedCertificate is a ManagedCertificate safe for API responses.
type PublicManagedCertificate struct {
	ID                   string
	Name                 string
	Domains              []string
	CertificateIssuerID  string
	DNSProviderAccountID string
	KeyType              string
	AutoRenewEnabled     bool
	RenewBeforeDays      uint32
	NotifyGroupIDs       []string
	ServerIDs            []string
	ActiveValidity       string
	BundleAvailable      bool
	ActiveVersion        *PublicCertificateVersion
	LatestOperation      *PublicOperation
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func publicOperationFromStore(op store.CertificateOperation) PublicOperation {
	return PublicOperation{
		ID:                   op.ID,
		ManagedCertificateID: op.ManagedCertificateID,
		Type:                 op.Type,
		Status:               op.Status,
		AttemptCount:         op.AttemptCount,
		ConfigSnapshot:       op.ConfigSnapshot,
		ErrorSummary:         op.ErrorSummary,
		Warning:              op.Warning,
		StartedAt:            op.StartedAt,
		FinishedAt:           op.FinishedAt,
		NextAttemptAt:        op.NextAttemptAt,
		CreatedAt:            op.CreatedAt,
		UpdatedAt:            op.UpdatedAt,
	}
}

func computeValidity(cert store.ManagedCertificate, now time.Time) (validity string, bundleAvailable bool) {
	if cert.ActiveVersion == nil {
		return store.CertValidityMissing, false
	}
	bundleAvailable = true
	v := cert.ActiveVersion
	if now.After(v.NotAfter) {
		return store.CertValidityExpired, bundleAvailable
	}
	return store.CertValidityValid, bundleAvailable
}

func publicCertFromStore(cert store.ManagedCertificate, latest *store.CertificateOperation, now time.Time) PublicManagedCertificate {
	validity, available := computeValidity(cert, now)
	out := PublicManagedCertificate{
		ID:                   cert.ID,
		Name:                 cert.Name,
		Domains:              append([]string(nil), cert.Domains...),
		CertificateIssuerID:  cert.CertificateIssuerID,
		DNSProviderAccountID: cert.DNSProviderAccountID,
		KeyType:              cert.KeyType,
		AutoRenewEnabled:     cert.AutoRenewEnabled,
		RenewBeforeDays:      cert.RenewBeforeDays,
		NotifyGroupIDs:       append([]string(nil), cert.NotifyGroupIDs...),
		ServerIDs:            append([]string(nil), cert.ServerIDs...),
		ActiveValidity:       validity,
		BundleAvailable:      available,
		CreatedAt:            cert.CreatedAt,
		UpdatedAt:            cert.UpdatedAt,
	}
	if latest != nil {
		op := publicOperationFromStore(*latest)
		out.LatestOperation = &op
	}
	out.ActiveVersion = publicVersionFromStore(cert.ActiveVersion)
	return out
}

func (m *Manager) ListManagedCertificates(ctx context.Context, limit int64, pageToken string) ([]PublicManagedCertificate, string, error) {
	certs, next, err := m.store.ListManagedCertificates(ctx, limit, pageToken)
	if err != nil {
		return nil, "", err
	}
	out := make([]PublicManagedCertificate, 0, len(certs))
	for _, cert := range certs {
		out = append(out, publicCertFromStore(cert, nil, m.clock.Now()))
	}
	return out, next, nil
}

func (m *Manager) GetManagedCertificate(ctx context.Context, id string) (PublicManagedCertificate, error) {
	cert, err := m.store.GetManagedCertificate(ctx, id)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	latest, err := m.store.LatestCertificateOperation(ctx, id)
	if err != nil && !store.IsNotFound(err) {
		return PublicManagedCertificate{}, err
	}
	var latestPtr *store.CertificateOperation
	if err == nil {
		latestPtr = &latest
	}
	return publicCertFromStore(cert, latestPtr, m.clock.Now()), nil
}

func (m *Manager) CreateManagedCertificate(ctx context.Context, input ManagedCertificateInput) (PublicManagedCertificate, error) {
	cert, err := m.buildManagedCertificate(input, store.ManagedCertificate{})
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	if err := m.validateManagedCertificateRefs(ctx, cert); err != nil {
		return PublicManagedCertificate{}, err
	}
	created, err := m.store.CreateManagedCertificate(ctx, cert)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	op, err := m.createPendingIssueOperation(ctx, created)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	m.triggerIssueOperation(op.ID)
	return publicCertFromStore(created, &op, m.clock.Now()), nil
}

func (m *Manager) UpdateManagedCertificate(ctx context.Context, id string, input ManagedCertificateInput) (PublicManagedCertificate, error) {
	existing, err := m.store.GetManagedCertificate(ctx, id)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	updated, err := m.buildManagedCertificate(input, existing)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	if err := m.validateManagedCertificateRefs(ctx, updated); err != nil {
		return PublicManagedCertificate{}, err
	}
	running, err := m.store.FindRunningCertificateOperation(ctx, id, store.CertOpTypeIssue)
	if err != nil && !store.IsNotFound(err) {
		return PublicManagedCertificate{}, err
	}
	if err == nil && issueFieldsChanged(existing, updated) {
		return PublicManagedCertificate{}, ErrIssueFieldsLocked
	}
	_ = running // running Issue op blocks issue-field changes only

	updated.ActiveVersion = existing.ActiveVersion
	updated.PreviousVersion = existing.PreviousVersion
	saved, err := m.store.UpdateManagedCertificate(ctx, id, updated)
	if err != nil {
		return PublicManagedCertificate{}, err
	}
	latest, err := m.store.LatestCertificateOperation(ctx, id)
	if err != nil && !store.IsNotFound(err) {
		return PublicManagedCertificate{}, err
	}
	var latestPtr *store.CertificateOperation
	if err == nil {
		latestPtr = &latest
	}
	return publicCertFromStore(saved, latestPtr, m.clock.Now()), nil
}

// SubmitIssueOperation returns an existing running Issue operation or creates a
// new Pending Issue operation with the current desired config snapshot.
func (m *Manager) SubmitIssueOperation(ctx context.Context, managedCertificateID string) (PublicOperation, error) {
	if existing, err := m.store.FindRunningCertificateOperation(ctx, managedCertificateID, store.CertOpTypeIssue); err == nil {
		return publicOperationFromStore(existing), nil
	} else if !store.IsNotFound(err) {
		return PublicOperation{}, err
	}
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		return PublicOperation{}, err
	}
	op, err := m.createPendingIssueOperation(ctx, cert)
	if err != nil {
		return PublicOperation{}, err
	}
	m.triggerIssueOperation(op.ID)
	return publicOperationFromStore(op), nil
}

func (m *Manager) buildManagedCertificate(input ManagedCertificateInput, existing store.ManagedCertificate) (store.ManagedCertificate, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return store.ManagedCertificate{}, ErrManagedCertificateNameRequired
	}
	domains, err := NormalizeDomains(input.Domains)
	if err != nil {
		return store.ManagedCertificate{}, err
	}
	issuerID := strings.TrimSpace(input.CertificateIssuerID)
	if issuerID == "" {
		return store.ManagedCertificate{}, ErrCertificateIssuerRequired
	}
	dnsID := strings.TrimSpace(input.DNSProviderAccountID)
	if dnsID == "" {
		return store.ManagedCertificate{}, ErrDNSAccountRequired
	}
	keyType, err := normalizeKeyType(input.KeyType)
	if err != nil {
		return store.ManagedCertificate{}, err
	}
	renewBefore := input.RenewBeforeDays
	if renewBefore == 0 {
		if existing.RenewBeforeDays != 0 {
			renewBefore = existing.RenewBeforeDays
		} else {
			renewBefore = store.DefaultRenewBeforeDays
		}
	}
	autoRenew := existing.AutoRenewEnabled
	if input.AutoRenewEnabled != nil {
		autoRenew = *input.AutoRenewEnabled
	} else if existing.ID == "" {
		autoRenew = true
	}
	return store.ManagedCertificate{
		ID:                   existing.ID,
		Name:                 name,
		Domains:              domains,
		CertificateIssuerID:  issuerID,
		DNSProviderAccountID: dnsID,
		KeyType:              keyType,
		AutoRenewEnabled:     autoRenew,
		RenewBeforeDays:      renewBefore,
		NotifyGroupIDs:       append([]string(nil), input.NotifyGroupIDs...),
		ServerIDs:            append([]string(nil), input.ServerIDs...),
		CreatedAt:            existing.CreatedAt,
	}, nil
}

func issueFieldsChanged(before, after store.ManagedCertificate) bool {
	if before.CertificateIssuerID != after.CertificateIssuerID ||
		before.DNSProviderAccountID != after.DNSProviderAccountID ||
		before.KeyType != after.KeyType {
		return true
	}
	if len(before.Domains) != len(after.Domains) {
		return true
	}
	for i := range before.Domains {
		if before.Domains[i] != after.Domains[i] {
			return true
		}
	}
	return false
}

func (m *Manager) validateManagedCertificateRefs(ctx context.Context, cert store.ManagedCertificate) error {
	issuer, err := m.store.GetCertificateIssuer(ctx, cert.CertificateIssuerID)
	if err != nil {
		if store.IsNotFound(err) {
			return ErrCertificateIssuerRequired
		}
		return err
	}
	if issuer.RegistrationStatus != store.IssuerRegistrationReady {
		return ErrIssuerNotReady
	}
	if _, err := m.store.GetDNSProviderAccount(ctx, cert.DNSProviderAccountID); err != nil {
		if store.IsNotFound(err) {
			return ErrDNSAccountRequired
		}
		return err
	}
	if err := m.store.ValidateNotifyGroupIDs(ctx, cert.NotifyGroupIDs); err != nil {
		return err
	}
	if err := m.store.ValidateServerIDs(ctx, cert.ServerIDs); err != nil {
		return err
	}
	return nil
}

func (m *Manager) createPendingIssueOperation(ctx context.Context, cert store.ManagedCertificate) (store.CertificateOperation, error) {
	if existing, err := m.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeIssue); err == nil {
		return existing, nil
	} else if !store.IsNotFound(err) {
		return store.CertificateOperation{}, err
	}
	snapshot := store.IssueConfigSnapshot{
		Domains:              append([]string(nil), cert.Domains...),
		CertificateIssuerID:  cert.CertificateIssuerID,
		DNSProviderAccountID: cert.DNSProviderAccountID,
		KeyType:              cert.KeyType,
	}
	return m.store.CreateCertificateOperation(ctx, store.CertificateOperation{
		ManagedCertificateID: cert.ID,
		Type:                 store.CertOpTypeIssue,
		Status:               store.CertOpStatusPending,
		AttemptCount:         0,
		ConfigSnapshot:       snapshot,
	})
}
