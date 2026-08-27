package certmanager

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Preset directory URLs for built-in CAs.
const (
	letsEncryptProductionDirectory = "https://acme-v02.api.letsencrypt.org/directory"
	letsEncryptStagingDirectory    = "https://acme-staging-v02.api.letsencrypt.org/directory"
	zeroSSLDirectory               = "https://acme.zerossl.com/v2/DV90"
	googlePublicCADirectory        = "https://dv.acme-v02.api.pki.goog/directory"

	issuerRegistrationFinalizeTimeout = 2 * time.Second
	issuerRegistrationInterrupted     = "certificate issuer registration interrupted"
)

// DirectoryPreview describes a resolved ACME directory for UI ToS agreement.
type DirectoryPreview struct {
	CAType            string
	DirectoryURL      string
	TermsOfServiceURL string
	StagingUntrusted  bool
	RequiresEAB       bool
}

// IssuerInput is the mutable issuer fields supplied by API callers.
type IssuerInput struct {
	Name                 string
	CAType               string
	CustomDirectoryURL   string
	Email                string
	AccountKeyPEM        string
	EABKid               string
	EABHMAC              string
	TermsOfServiceAgreed bool
}

// PublicIssuer is a CertificateIssuer safe for API responses.
type PublicIssuer struct {
	ID                     string
	Name                   string
	CAType                 string
	DirectoryURL           string
	Email                  string
	RegistrationStatus     string
	RegistrationError      string
	StagingUntrusted       bool
	TermsOfServiceURL      string
	TermsOfServiceAgreedAt *time.Time
	AccountKeyConfigured   bool
	EABConfigured          bool
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func resolvePreset(caType, customDirectoryURL string) (directoryURL string, stagingUntrusted bool, requiresEAB bool, err error) {
	switch caType {
	case store.CATypeLetsEncryptProduction:
		return letsEncryptProductionDirectory, false, false, nil
	case store.CATypeLetsEncryptStaging:
		return letsEncryptStagingDirectory, true, false, nil
	case store.CATypeZeroSSL:
		return zeroSSLDirectory, false, true, nil
	case store.CATypeGooglePublicCA:
		return googlePublicCADirectory, false, true, nil
	case store.CATypeCustom:
		customDirectoryURL = strings.TrimSpace(customDirectoryURL)
		if customDirectoryURL == "" {
			return "", false, false, ErrCustomDirectoryRequired
		}
		if err := validateCustomDirectoryURL(customDirectoryURL); err != nil {
			return "", false, false, err
		}
		return customDirectoryURL, false, false, nil
	default:
		return "", false, false, store.ErrInvalidCertificateIssuerCAType
	}
}

func validateCustomDirectoryURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: invalid directory URL", ErrInvalidDirectoryURL)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: directory URL must use HTTPS", ErrInvalidDirectoryURL)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: directory URL must include a host", ErrInvalidDirectoryURL)
	}
	return nil
}

func generateAccountKeyPEM() (string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
}

func publicFromIssuer(i store.CertificateIssuer) PublicIssuer {
	return PublicIssuer{
		ID:                     i.ID,
		Name:                   i.Name,
		CAType:                 i.CAType,
		DirectoryURL:           i.DirectoryURL,
		Email:                  i.Email,
		RegistrationStatus:     i.RegistrationStatus,
		RegistrationError:      i.RegistrationError,
		StagingUntrusted:       i.StagingUntrusted,
		TermsOfServiceURL:      i.TermsOfServiceURL,
		TermsOfServiceAgreedAt: i.TermsOfServiceAgreed,
		AccountKeyConfigured:   i.AccountKeyPEM != "",
		EABConfigured:          i.EABKid != "" && i.EABHMAC != "",
		CreatedAt:              i.CreatedAt,
		UpdatedAt:              i.UpdatedAt,
	}
}

func (m *Manager) GetCertificateIssuerDirectoryPreview(ctx context.Context, caType, customDirectoryURL string) (DirectoryPreview, error) {
	directoryURL, staging, requiresEAB, err := resolvePreset(caType, customDirectoryURL)
	if err != nil {
		return DirectoryPreview{}, err
	}
	meta, err := m.acme.FetchDirectory(ctx, directoryURL)
	if err != nil {
		return DirectoryPreview{}, err
	}
	return DirectoryPreview{
		CAType:            caType,
		DirectoryURL:      directoryURL,
		TermsOfServiceURL: meta.TermsOfService,
		StagingUntrusted:  staging,
		RequiresEAB:       requiresEAB,
	}, nil
}

func (m *Manager) ListCertificateIssuers(ctx context.Context, limit int64, pageToken string) ([]PublicIssuer, string, error) {
	issuers, next, err := m.store.ListCertificateIssuers(ctx, limit, pageToken)
	if err != nil {
		return nil, "", err
	}
	out := make([]PublicIssuer, 0, len(issuers))
	for _, i := range issuers {
		out = append(out, publicFromIssuer(i))
	}
	return out, next, nil
}

func (m *Manager) GetCertificateIssuer(ctx context.Context, id string) (PublicIssuer, error) {
	issuer, err := m.store.GetCertificateIssuer(ctx, id)
	if err != nil {
		return PublicIssuer{}, err
	}
	return publicFromIssuer(issuer), nil
}

func (m *Manager) CreateCertificateIssuer(ctx context.Context, input IssuerInput) (PublicIssuer, error) {
	if strings.TrimSpace(input.Name) == "" {
		return PublicIssuer{}, ErrIssuerNameRequired
	}
	if strings.TrimSpace(input.Email) == "" {
		return PublicIssuer{}, ErrIssuerEmailRequired
	}
	if !input.TermsOfServiceAgreed {
		return PublicIssuer{}, ErrTermsOfServiceRequired
	}
	directoryURL, staging, requiresEAB, err := resolvePreset(input.CAType, input.CustomDirectoryURL)
	if err != nil {
		return PublicIssuer{}, err
	}
	if requiresEAB && (strings.TrimSpace(input.EABKid) == "" || strings.TrimSpace(input.EABHMAC) == "") {
		return PublicIssuer{}, ErrEABRequired
	}
	meta, err := m.acme.FetchDirectory(ctx, directoryURL)
	if err != nil {
		if m.logger != nil {
			attrs := []any{
				"error_stage", "fetch_acme_directory",
				"acme_host", safeURLHost(directoryURL),
			}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "fetch ACME directory failed", attrs...)
		}
		return PublicIssuer{}, err
	}
	if meta.TermsOfService != "" && !input.TermsOfServiceAgreed {
		return PublicIssuer{}, ErrTermsOfServiceRequired
	}
	accountKey := strings.TrimSpace(input.AccountKeyPEM)
	if accountKey == "" {
		accountKey, err = generateAccountKeyPEM()
		if err != nil {
			return PublicIssuer{}, err
		}
	}
	now := m.clock.Now()
	issuer := store.CertificateIssuer{
		Name:                 strings.TrimSpace(input.Name),
		CAType:               input.CAType,
		DirectoryURL:         directoryURL,
		Email:                strings.TrimSpace(input.Email),
		RegistrationStatus:   store.IssuerRegistrationPending,
		StagingUntrusted:     staging,
		TermsOfServiceURL:    meta.TermsOfService,
		TermsOfServiceAgreed: &now,
		AccountKeyPEM:        accountKey,
		EABKid:               strings.TrimSpace(input.EABKid),
		EABHMAC:              strings.TrimSpace(input.EABHMAC),
	}
	created, err := m.store.CreateCertificateIssuer(ctx, issuer)
	if err != nil {
		if m.logger != nil {
			attrs := []any{
				"error_stage", "persist_certificate_issuer",
				"acme_host", safeURLHost(directoryURL),
			}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "persist certificate issuer failed", attrs...)
		}
		return PublicIssuer{}, err
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "certificate issuer created",
			"issuer_id", created.ID,
			"acme_host", safeURLHost(created.DirectoryURL),
			"registration_status", created.RegistrationStatus,
			"staging_untrusted", created.StagingUntrusted,
		)
	}
	m.startRegistration(created.ID)
	return publicFromIssuer(created), nil
}

func (m *Manager) UpdateCertificateIssuer(ctx context.Context, id string, input IssuerInput) (PublicIssuer, error) {
	existing, err := m.store.GetCertificateIssuer(ctx, id)
	if err != nil {
		return PublicIssuer{}, err
	}
	shouldStartRegistration := false
	switch existing.RegistrationStatus {
	case store.IssuerRegistrationReady:
		if identityChanged(existing, input) {
			return PublicIssuer{}, store.ErrCertificateIssuerImmutable
		}
		existing.Name = strings.TrimSpace(input.Name)
	case store.IssuerRegistrationFailed:
		updated, err := m.applyFailedIssuerUpdate(existing, input)
		if err != nil {
			return PublicIssuer{}, err
		}
		existing = updated
		shouldStartRegistration = true
	case store.IssuerRegistrationPending:
		return PublicIssuer{}, ErrIssuerRegistrationPending
	default:
		return PublicIssuer{}, errors.New("unsupported issuer registration status")
	}
	saved, err := m.store.UpdateCertificateIssuer(ctx, id, existing)
	if err != nil {
		return PublicIssuer{}, err
	}
	if shouldStartRegistration {
		m.startRegistration(id)
	}
	return publicFromIssuer(saved), nil
}

func identityChanged(existing store.CertificateIssuer, input IssuerInput) bool {
	if input.CAType != "" && input.CAType != existing.CAType {
		return true
	}
	if input.Email != "" && strings.TrimSpace(input.Email) != existing.Email {
		return true
	}
	if input.CustomDirectoryURL != "" && input.CustomDirectoryURL != existing.DirectoryURL {
		return true
	}
	if input.AccountKeyPEM != "" {
		return true
	}
	if input.EABKid != "" || input.EABHMAC != "" {
		return true
	}
	return false
}

func (m *Manager) applyFailedIssuerUpdate(existing store.CertificateIssuer, input IssuerInput) (store.CertificateIssuer, error) {
	if strings.TrimSpace(input.Name) != "" {
		existing.Name = strings.TrimSpace(input.Name)
	}
	caType := input.CAType
	if caType == "" {
		caType = existing.CAType
	}
	customURL := input.CustomDirectoryURL
	if customURL == "" && caType == existing.CAType {
		customURL = existing.DirectoryURL
	}
	directoryURL, staging, requiresEAB, err := resolvePreset(caType, customURL)
	if err != nil {
		return store.CertificateIssuer{}, err
	}
	email := strings.TrimSpace(input.Email)
	if email == "" {
		email = existing.Email
	}
	if email == "" {
		return store.CertificateIssuer{}, ErrIssuerEmailRequired
	}
	if requiresEAB {
		kid := strings.TrimSpace(input.EABKid)
		hmacKey := strings.TrimSpace(input.EABHMAC)
		if kid == "" {
			kid = existing.EABKid
		}
		if hmacKey == "" {
			hmacKey = existing.EABHMAC
		}
		if kid == "" || hmacKey == "" {
			return store.CertificateIssuer{}, ErrEABRequired
		}
		existing.EABKid = kid
		existing.EABHMAC = hmacKey
	}
	if pemKey := strings.TrimSpace(input.AccountKeyPEM); pemKey != "" {
		existing.AccountKeyPEM = pemKey
	}
	existing.CAType = caType
	existing.DirectoryURL = directoryURL
	existing.Email = email
	existing.StagingUntrusted = staging
	existing.RegistrationStatus = store.IssuerRegistrationPending
	existing.RegistrationError = ""
	return existing, nil
}

func (m *Manager) RetryCertificateIssuerRegistration(ctx context.Context, id string) (PublicIssuer, error) {
	existing, err := m.store.GetCertificateIssuer(ctx, id)
	if err != nil {
		return PublicIssuer{}, err
	}
	if existing.RegistrationStatus != store.IssuerRegistrationFailed {
		return PublicIssuer{}, store.ErrCertificateIssuerNotRetryable
	}
	existing.RegistrationStatus = store.IssuerRegistrationPending
	existing.RegistrationError = ""
	saved, err := m.store.UpdateCertificateIssuer(ctx, id, existing)
	if err != nil {
		return PublicIssuer{}, err
	}
	m.startRegistration(id)
	return publicFromIssuer(saved), nil
}

func (m *Manager) DeleteCertificateIssuer(ctx context.Context, id string) error {
	if err := m.ensureIssuerDeletable(ctx, id); err != nil {
		return err
	}
	return m.store.DeleteCertificateIssuer(ctx, id)
}

func (m *Manager) startRegistration(id string) {
	if _, loaded := m.registrationInflight.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	ctx := m.backgroundContext()
	if m.logger != nil {
		m.logger.InfoContext(ctx, "certificate issuer registration scheduled", "issuer_id", id)
	}
	if !m.launchBackgroundTask(func() {
		defer m.registrationInflight.Delete(id)
		m.runRegistration(ctx, id)
	}) {
		m.registrationInflight.Delete(id)
		if m.logger != nil {
			m.logger.ErrorContext(ctx, "certificate issuer registration task rejected", "issuer_id", id, "error_stage", "background_task_rejected")
		}
	}
}

func (m *Manager) runRegistration(ctx context.Context, id string) {
	if ctx.Err() != nil {
		if m.logger != nil {
			m.logger.WarnContext(ctx, "certificate issuer registration interrupted before start", "issuer_id", id)
		}
		m.failInterruptedRegistration(ctx, id)
		return
	}
	issuer, err := m.store.GetCertificateIssuer(ctx, id)
	if err != nil {
		if m.logger != nil {
			attrs := []any{"issuer_id", id, "error_stage", "load_certificate_issuer"}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "load certificate issuer for registration failed", attrs...)
		}
		return
	}
	if issuer.RegistrationStatus != store.IssuerRegistrationPending {
		if m.logger != nil {
			m.logger.InfoContext(ctx, "certificate issuer registration skipped", "issuer_id", id, "registration_status", issuer.RegistrationStatus)
		}
		return
	}
	started := time.Now()
	if m.logger != nil {
		m.logger.InfoContext(ctx, "certificate issuer registration started",
			"issuer_id", id,
			"acme_host", safeURLHost(issuer.DirectoryURL),
			"staging_untrusted", issuer.StagingUntrusted,
		)
	}
	regErr := m.acme.RegisterAccount(ctx, issuer)
	if ctx.Err() != nil {
		if m.logger != nil {
			m.logger.WarnContext(ctx, "certificate issuer registration interrupted", "issuer_id", id, "duration_ms", time.Since(started).Milliseconds())
		}
		m.failInterruptedRegistration(ctx, id)
		return
	}
	issuer, err = m.store.GetCertificateIssuer(ctx, id)
	if err != nil {
		if m.logger != nil {
			attrs := []any{"issuer_id", id, "error_stage", "reload_certificate_issuer", "duration_ms", time.Since(started).Milliseconds()}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "reload certificate issuer after registration failed", attrs...)
		}
		return
	}
	if regErr != nil {
		issuer.RegistrationStatus = store.IssuerRegistrationFailed
		issuer.RegistrationError = sanitizeRegistrationError(regErr, issuer.EABKid, issuer.EABHMAC, issuer.AccountKeyPEM)
		if m.logger != nil {
			attrs := []any{
				"issuer_id", id,
				"acme_host", safeURLHost(issuer.DirectoryURL),
				"duration_ms", time.Since(started).Milliseconds(),
			}
			attrs = append(attrs, safeErrorAttrs(regErr)...)
			m.logger.ErrorContext(ctx, "certificate issuer registration failed", attrs...)
		}
	} else {
		issuer.RegistrationStatus = store.IssuerRegistrationReady
		issuer.RegistrationError = ""
		if m.logger != nil {
			m.logger.InfoContext(ctx, "certificate issuer registration succeeded",
				"issuer_id", id,
				"acme_host", safeURLHost(issuer.DirectoryURL),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}
	}
	if _, err := m.store.UpdateCertificateIssuer(ctx, id, issuer); err != nil {
		if m.logger != nil {
			attrs := []any{"issuer_id", id, "error_stage", "persist_registration_status"}
			attrs = append(attrs, safeErrorAttrs(err)...)
			m.logger.ErrorContext(ctx, "persist certificate issuer registration status failed", attrs...)
		}
		return
	}
	if m.logger != nil {
		m.logger.InfoContext(ctx, "certificate issuer registration status persisted",
			"issuer_id", id,
			"registration_status", issuer.RegistrationStatus,
		)
	}
}

func (m *Manager) failInterruptedRegistration(ctx context.Context, id string) {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), issuerRegistrationFinalizeTimeout)
	defer cancel()
	issuer, err := m.store.GetCertificateIssuer(finalizeCtx, id)
	if err != nil || issuer.RegistrationStatus != store.IssuerRegistrationPending {
		return
	}
	issuer.RegistrationStatus = store.IssuerRegistrationFailed
	issuer.RegistrationError = issuerRegistrationInterrupted
	if _, err := m.store.UpdateCertificateIssuer(finalizeCtx, id, issuer); err != nil && m.logger != nil {
		attrs := []any{"issuer_id", id, "error_stage", "persist_interrupted_registration"}
		attrs = append(attrs, safeErrorAttrs(err)...)
		m.logger.ErrorContext(finalizeCtx, "finalize interrupted issuer registration failed", attrs...)
	}
}
