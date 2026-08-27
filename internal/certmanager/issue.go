package certmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
}

var issueInflight sync.Map // opID -> struct{}

func (m *Manager) triggerIssueOperation(opID string) {
	go m.runIssueOperation(context.Background(), opID)
}

func (m *Manager) runIssueOperation(ctx context.Context, opID string) {
	if _, loaded := issueInflight.LoadOrStore(opID, struct{}{}); loaded {
		return
	}
	defer issueInflight.Delete(opID)

	op, err := m.store.ClaimPendingIssueOperation(ctx, opID)
	if err != nil {
		if store.IsNotFound(err) {
			return
		}
		return
	}

	warning, runErr := m.executeIssue(ctx, op)
	if runErr == nil {
		return
	}
	_ = warning
	_ = m.store.FailIssueOperation(ctx, opID, sanitizeIssueError(runErr))
}

func (m *Manager) executeIssue(ctx context.Context, op store.CertificateOperation) (warning string, err error) {
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
	if err := m.store.ActivateFirstIssueVersion(ctx, op.ManagedCertificateID, version, op.ID, warning); err != nil {
		return "", err
	}
	return warning, nil
}

func sanitizeIssueError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	secretMarkers := []string{"token", "pem", "eab", "hmac", "api key", "private key", "account key", "authorization:"}
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

func (m *Manager) GetCertificateBundle(ctx context.Context, managedCertificateID string) (CertificateBundle, error) {
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		return CertificateBundle{}, err
	}
	if cert.ActiveVersion == nil {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	v := cert.ActiveVersion
	validity, _ := computeValidity(cert, m.clock.Now())
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

func publicVersionFromStore(v *store.CertificateVersion) *PublicCertificateVersion {
	if v == nil {
		return nil
	}
	return &PublicCertificateVersion{
		ID:               v.ID,
		ConfigSnapshot:   v.ConfigSnapshot,
		LeafFingerprint:  v.LeafFingerprint,
		SerialNumber:     v.SerialNumber,
		IssuerCommonName: v.IssuerCommonName,
		NotBefore:        v.NotBefore.Unix(),
		NotAfter:         v.NotAfter.Unix(),
		KeyType:          v.KeyType,
		StagingUntrusted: v.StagingUntrusted,
		CreatedAt:        v.CreatedAt.Unix(),
	}
}

type cleanupTrackingProvider struct {
	challenge.Provider
	cleanupWarning string
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
