package certmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	issuer, err := m.store.GetCertificateIssuer(ctx, snap.CertificateIssuerID)
	if err != nil {
		return "", fmt.Errorf("resolve issuer: %w", err)
	}
	if issuer.RegistrationStatus != store.IssuerRegistrationReady {
		return "", ErrIssuerNotReady
	}
	dnsAccount, err := m.store.GetDNSProviderAccount(ctx, snap.DNSProviderAccountID)
	if err != nil {
		return "", fmt.Errorf("resolve dns account: %w", err)
	}
	if m.dnsFactory == nil {
		return "", errors.New("dns provider factory is not configured")
	}
	baseDNS, err := m.dnsFactory.NewProvider(dnsAccount)
	if err != nil {
		return "", err
	}
	trackedDNS := &cleanupTrackingProvider{Provider: baseDNS}

	issueResult, issueErr := m.acme.IssueCertificate(ctx, IssueRequest{
		Issuer:  issuer,
		Domains: snap.Domains,
		KeyType: snap.KeyType,
		DNS:     trackedDNS,
	})
	m.persistPendingTXT(ctx, op.ID, leaseOwner, trackedDNS.presented)
	if issueErr != nil {
		return "", issueErr
	}
	if trackedDNS.cleanupWarning != "" {
		warning = trackedDNS.cleanupWarning
	}

	now := m.clock.Now()
	parsed, err := validateIssuedBundle(issueResult.FullchainPEM, issueResult.PrivateKeyPEM, snap.Domains, snap.KeyType, now)
	if err != nil {
		return "", err
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
		return "", err
	}
	if cert.ActiveVersion == nil {
		if err := m.store.ActivateFirstIssueVersion(ctx, op.ManagedCertificateID, version, op.ID, leaseOwner, warning); err != nil {
			return "", err
		}
	} else {
		if err := m.store.ActivateSubsequentIssueVersion(ctx, op.ManagedCertificateID, version, cert.ActiveVersion.ID, op.ID, leaseOwner, warning); err != nil {
			return "", err
		}
	}
	_ = m.store.ClearCertificateOperationPendingTXT(ctx, op.ID)
	return warning, nil
}

func sanitizeIssueError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	secretMarkers := []string{"token", "pem", "eab", "hmac", "api key", "private key", "account key", "authorization:", "order url", "acme-staging", "directory"}
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return "certificate issuance failed"
		}
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
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
	cleanupWarning string
	presented      []store.DNSChallengeRecord
}

func (p *cleanupTrackingProvider) Present(domain, token, keyAuth string) error {
	err := p.Provider.Present(domain, token, keyAuth)
	if err == nil {
		p.presented = append(p.presented, store.DNSChallengeRecord{
			Domain:  domain,
			Token:   token,
			KeyAuth: keyAuth,
		})
	}
	return err
}

func (p *cleanupTrackingProvider) CleanUp(domain, token, keyAuth string) error {
	err := p.Provider.CleanUp(domain, token, keyAuth)
	if err != nil {
		p.cleanupWarning = "dns txt cleanup failed: " + sanitizeIssueError(err)
		return nil
	}
	return nil
}

func (p *cleanupTrackingProvider) Timeout() (time.Duration, time.Duration) {
	if t, ok := p.Provider.(interface {
		Timeout() (time.Duration, time.Duration)
	}); ok {
		return t.Timeout()
	}
	return 0, 0
}
