package certmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/google/uuid"
	"github.com/orvice/neo-line/internal/store"
)

// ErrBundleNotAvailable is returned when no active certificate bundle exists.
var ErrBundleNotAvailable = errors.New("certificate bundle is not available")

// CertificateBundle is the admin-facing active bundle with PEM bytes and metadata.
type CertificateBundle struct {
	ManagedCertificateID string
	VersionID            string
	Domains              []string
	KeyType              string
	LeafFingerprint      string
	NotBefore            int64 // unix seconds for proto conversion in connect layer
	NotAfter             int64
	Validity             string
	StagingUntrusted     bool
	FullchainPEM         []byte
	PrivateKeyPEM        []byte
}

// PublicCertificateVersion exposes active/previous version metadata without secrets.
type PublicCertificateVersion struct {
	ID               string
	ConfigSnapshot   store.IssueConfigSnapshot
	LeafFingerprint  string
	SerialNumber     string
	IssuerCommonName string
	NotBefore        int64
	NotAfter         int64
	KeyType          string
	StagingUntrusted bool
	CreatedAt        int64
	RevokedAt        int64 // unix seconds; zero when not revoked
	RevokePending    bool
}

func (m *Manager) triggerIssueOperation(opID string) {
	m.triggerOperation(opID)
}

func (m *Manager) runIssueOperation(ctx context.Context, opID string) {
	m.runOperation(ctx, opID)
}

func (m *Manager) executeCertificateIssuance(ctx context.Context, op store.CertificateOperation, leaseOwner string) (warning string, err error) {
	snap := op.ConfigSnapshot
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "key_type", snap.KeyType)
		m.logger.InfoContext(ctx, "certificate issuance started", attrs...)
	}
	issuer, err := m.store.GetCertificateIssuer(ctx, snap.CertificateIssuerID)
	if err != nil {
		return "", withOperationStage("resolve_issuer", fmt.Errorf("resolve issuer: %w", err))
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs,
			"issuer_registration_status", issuer.RegistrationStatus,
			"issuer_staging_untrusted", issuer.StagingUntrusted,
			"acme_host", safeURLHost(issuer.DirectoryURL),
		)
		m.logger.InfoContext(ctx, "certificate issuer resolved", attrs...)
	}
	if issuer.RegistrationStatus != store.IssuerRegistrationReady {
		return "", withOperationStage("issuer_readiness", ErrIssuerNotReady)
	}
	dnsAccount, err := m.store.GetDNSProviderAccount(ctx, snap.DNSProviderAccountID)
	if err != nil {
		return "", withOperationStage("resolve_dns_account", fmt.Errorf("resolve dns account: %w", err))
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs,
			"dns_provider", dnsAccount.Provider,
			"dns_token_configured", dnsAccount.APIToken != "",
			"dns_propagation_timeout_seconds", dnsAccount.PropagationTimeoutSeconds,
		)
		m.logger.InfoContext(ctx, "dns provider account resolved", attrs...)
	}
	if m.dnsFactory == nil {
		return "", withOperationStage("create_dns_provider", errors.New("dns provider factory is not configured"))
	}
	baseDNS, err := m.dnsFactory.NewProvider(dnsAccount)
	if err != nil {
		return "", withOperationStage("create_dns_provider", err)
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		m.logger.InfoContext(ctx, "dns challenge provider initialized", attrs...)
	}
	trackedDNS := &cleanupTrackingProvider{
		Provider:       baseDNS,
		logger:         m.logger,
		ctx:            ctx,
		operationAttrs: operationLogAttrs(op),
		recordPresented: func(record store.DNSChallengeRecord) error {
			return m.store.RecordCertificateOperationPendingTXT(ctx, op.ID, leaseOwner, record)
		},
	}

	acmeStarted := time.Now()
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "acme_host", safeURLHost(issuer.DirectoryURL))
		m.logger.InfoContext(ctx, "acme certificate obtain started", attrs...)
	}
	issueResult, issueErr := m.acme.IssueCertificate(ctx, IssueRequest{
		Issuer:               issuer,
		Domains:              snap.Domains,
		KeyType:              snap.KeyType,
		DNS:                  trackedDNS,
		OperationID:          op.ID,
		ManagedCertificateID: op.ManagedCertificateID,
		AttemptCount:         op.AttemptCount,
	})
	if issueErr != nil {
		return "", withOperationStage("acme_obtain", issueErr)
	}
	if m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "duration_ms", time.Since(acmeStarted).Milliseconds())
		m.logger.InfoContext(ctx, "acme certificate obtain succeeded", attrs...)
	}
	if cleanupWarning := trackedDNS.warning(); cleanupWarning != "" {
		warning = cleanupWarning
		if m.logger != nil {
			attrs := operationLogAttrs(op)
			m.logger.WarnContext(ctx, "certificate DNS cleanup completed with warning", attrs...)
		}
	}

	now := m.clock.Now()
	parsed, err := validateIssuedBundle(issueResult.FullchainPEM, issueResult.PrivateKeyPEM, snap.Domains, snap.KeyType, now)
	if err != nil {
		return "", withOperationStage("validate_issued_bundle", err)
	}

	version := store.CertificateVersion{
		ID:                  "cver_" + uuid.NewString(),
		ConfigSnapshot:      snap,
		FullchainPEM:        string(parsed.FullchainPEM),
		PrivateKeyPEM:       string(parsed.PrivateKeyPEM),
		LeafFingerprint:     parsed.LeafFingerprint,
		SerialNumber:        parsed.SerialNumber,
		IssuerCommonName:    parsed.IssuerCommonName,
		NotBefore:           parsed.NotBefore,
		NotAfter:            parsed.NotAfter,
		KeyType:             snap.KeyType,
		StagingUntrusted:    issuer.StagingUntrusted,
		CertificateIssuerID: issuer.ID,
		CreatedAt:           now,
	}
	cert, err := m.store.GetManagedCertificate(ctx, op.ManagedCertificateID)
	if err != nil {
		return "", withOperationStage("load_managed_certificate", err)
	}
	if cert.ActiveVersion == nil {
		if err := m.store.ActivateFirstIssueVersion(ctx, op.ManagedCertificateID, version, op.ID, leaseOwner, warning); err != nil {
			return "", withOperationStage("activate_first_version", err)
		}
	} else {
		if err := m.store.ActivateSubsequentIssueVersion(ctx, op.ManagedCertificateID, version, cert.ActiveVersion.ID, op.ID, leaseOwner, warning); err != nil {
			return "", withOperationStage("activate_subsequent_version", err)
		}
	}
	if err := m.store.ClearCertificateOperationPendingTXT(ctx, op.ID); err != nil && m.logger != nil {
		attrs := operationLogAttrs(op)
		attrs = append(attrs, "error_stage", "clear_pending_txt")
		attrs = append(attrs, safeErrorAttrs(err)...)
		m.logger.ErrorContext(ctx, "clear pending DNS challenge records failed", attrs...)
	}
	return warning, nil
}

func (m *Manager) GetCertificateBundle(ctx context.Context, managedCertificateID, versionSlot string) (CertificateBundle, error) {
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		return CertificateBundle{}, err
	}
	v := versionFromSlot(cert, versionSlot)
	if v == nil {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	if !versionDistributable(v) {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	validity, available := computeVersionValidity(v, cert.RenewBeforeDays, m.clock.Now())
	if !available {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	return CertificateBundle{
		ManagedCertificateID: cert.ID,
		VersionID:            v.ID,
		Domains:              append([]string(nil), snapDomains(v)...),
		KeyType:              v.KeyType,
		LeafFingerprint:      v.LeafFingerprint,
		NotBefore:            v.NotBefore.Unix(),
		NotAfter:             v.NotAfter.Unix(),
		Validity:             validity,
		StagingUntrusted:     v.StagingUntrusted,
		FullchainPEM:         []byte(v.FullchainPEM),
		PrivateKeyPEM:        []byte(v.PrivateKeyPEM),
	}, nil
}

func snapDomains(v *store.CertificateVersion) []string {
	if v == nil {
		return nil
	}
	return v.ConfigSnapshot.Domains
}

type cleanupTrackingProvider struct {
	challenge.Provider
	recordPresented func(store.DNSChallengeRecord) error
	logger          *slog.Logger
	ctx             context.Context
	operationAttrs  []any

	mu             sync.Mutex
	cleanupWarning string
}

func (p *cleanupTrackingProvider) Present(domain, token, keyAuth string) error {
	started := time.Now()
	if err := p.Provider.Present(domain, token, keyAuth); err != nil {
		if p.logger != nil {
			attrs := p.logAttrs(
				"domain", domain,
				"duration_ms", time.Since(started).Milliseconds(),
				"error_stage", "dns_present",
				"dns_provider_step", dnsProviderStep(err),
			)
			attrs = append(attrs, safeErrorAttrs(err)...)
			p.logger.ErrorContext(p.logContext(), "certificate DNS challenge present failed", attrs...)
		}
		return withOperationStage("dns_present", err)
	}
	if p.logger != nil {
		attrs := p.logAttrs("domain", domain, "duration_ms", time.Since(started).Milliseconds())
		p.logger.InfoContext(p.logContext(), "certificate DNS challenge presented", attrs...)
	}
	record := store.DNSChallengeRecord{
		Domain:  domain,
		Token:   token,
		KeyAuth: keyAuth,
	}
	if p.recordPresented == nil {
		return nil
	}
	if err := p.recordPresented(record); err != nil {
		persistErr := withOperationStage("persist_dns_challenge", err)
		if p.logger != nil {
			attrs := p.logAttrs("domain", domain, "error_stage", "persist_dns_challenge")
			attrs = append(attrs, safeErrorAttrs(err)...)
			p.logger.ErrorContext(p.logContext(), "persist DNS challenge record failed", attrs...)
		}
		if cleanupErr := p.Provider.CleanUp(domain, token, keyAuth); cleanupErr != nil {
			cleanupErr = withOperationStage("dns_cleanup_after_persist_failure", cleanupErr)
			if p.logger != nil {
				attrs := p.logAttrs("domain", domain, "error_stage", "dns_cleanup_after_persist_failure")
				attrs = append(attrs, safeErrorAttrs(cleanupErr)...)
				p.logger.WarnContext(p.logContext(), "cleanup DNS challenge after persistence failure failed", attrs...)
			}
			p.setCleanupWarning(cleanupErr)
		}
		return fmt.Errorf("persist DNS challenge record: %w", persistErr)
	}
	if p.logger != nil {
		attrs := p.logAttrs("domain", domain)
		p.logger.InfoContext(p.logContext(), "certificate DNS challenge record persisted", attrs...)
	}
	return nil
}

func (p *cleanupTrackingProvider) CleanUp(domain, token, keyAuth string) error {
	started := time.Now()
	if err := p.Provider.CleanUp(domain, token, keyAuth); err != nil {
		err = withOperationStage("dns_cleanup", err)
		if p.logger != nil {
			attrs := p.logAttrs(
				"domain", domain,
				"duration_ms", time.Since(started).Milliseconds(),
				"error_stage", "dns_cleanup",
				"dns_provider_step", dnsProviderStep(err),
			)
			attrs = append(attrs, safeErrorAttrs(err)...)
			p.logger.WarnContext(p.logContext(), "certificate DNS challenge cleanup failed", attrs...)
		}
		p.setCleanupWarning(err)
		return nil
	}
	if p.logger != nil {
		attrs := p.logAttrs("domain", domain, "duration_ms", time.Since(started).Milliseconds())
		p.logger.InfoContext(p.logContext(), "certificate DNS challenge cleaned up", attrs...)
	}
	return nil
}

func (p *cleanupTrackingProvider) logContext() context.Context {
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

func (p *cleanupTrackingProvider) logAttrs(extra ...any) []any {
	attrs := append([]any(nil), p.operationAttrs...)
	return append(attrs, extra...)
}

func (p *cleanupTrackingProvider) setCleanupWarning(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupWarning = "dns txt cleanup failed: " + sanitizeIssueError(err)
}

func (p *cleanupTrackingProvider) warning() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cleanupWarning
}

func (p *cleanupTrackingProvider) Timeout() (time.Duration, time.Duration) {
	if t, ok := p.Provider.(interface {
		Timeout() (time.Duration, time.Duration)
	}); ok {
		return t.Timeout()
	}
	return 0, 0
}
