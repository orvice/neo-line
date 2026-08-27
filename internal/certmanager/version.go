package certmanager

import (
	"context"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

const (
	VersionSlotActive   = "active"
	VersionSlotPrevious = "previous"
)

func effectiveRenewalWindow(notBefore, notAfter time.Time, renewBeforeDays uint32) time.Duration {
	cfg := time.Duration(renewBeforeDays) * 24 * time.Hour
	lifetime := notAfter.Sub(notBefore)
	if lifetime <= 0 {
		return cfg
	}
	third := lifetime / 3
	if third < cfg {
		return third
	}
	return cfg
}

func computeVersionValidity(v *store.CertificateVersion, renewBeforeDays uint32, now time.Time) (validity string, bundleAvailable bool) {
	if v == nil {
		return store.CertValidityMissing, false
	}
	if v.RevokedAt != nil {
		return store.CertValidityRevoked, false
	}
	bundleAvailable = true
	if now.After(v.NotAfter) {
		return store.CertValidityExpired, bundleAvailable
	}
	if !now.Before(v.NotBefore) {
		window := effectiveRenewalWindow(v.NotBefore, v.NotAfter, renewBeforeDays)
		if now.After(v.NotAfter.Add(-window)) {
			return store.CertValidityRenewalDue, bundleAvailable
		}
	}
	return store.CertValidityValid, bundleAvailable
}

func computeValidity(cert store.ManagedCertificate, now time.Time) (validity string, bundleAvailable bool) {
	return computeVersionValidity(cert.ActiveVersion, cert.RenewBeforeDays, now)
}

func desiredDiffersFromActive(cert store.ManagedCertificate) bool {
	if cert.ActiveVersion == nil {
		return false
	}
	snap := cert.ActiveVersion.ConfigSnapshot
	if cert.CertificateIssuerID != snap.CertificateIssuerID ||
		cert.DNSProviderAccountID != snap.DNSProviderAccountID ||
		cert.KeyType != snap.KeyType {
		return true
	}
	if len(cert.Domains) != len(snap.Domains) {
		return true
	}
	for i := range cert.Domains {
		if cert.Domains[i] != snap.Domains[i] {
			return true
		}
	}
	return false
}

func versionFromSlot(cert store.ManagedCertificate, slot string) *store.CertificateVersion {
	switch slot {
	case VersionSlotPrevious:
		return cert.PreviousVersion
	default:
		return cert.ActiveVersion
	}
}

func (m *Manager) ActivatePreviousVersion(ctx context.Context, managedCertificateID, versionID string) (PublicManagedCertificate, error) {
	if err := m.store.ActivatePreviousVersion(ctx, managedCertificateID, versionID); err != nil {
		return PublicManagedCertificate{}, err
	}
	return m.GetManagedCertificate(ctx, managedCertificateID)
}
