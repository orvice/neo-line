package certmanager

import (
	"context"
	"time"

	"github.com/orvice/neo-line/internal/certstate"
	"github.com/orvice/neo-line/internal/store"
)

const (
	VersionSlotActive   = "active"
	VersionSlotPrevious = "previous"
)

func effectiveRenewalWindow(notBefore, notAfter time.Time, renewBeforeDays uint32) time.Duration {
	return certstate.EffectiveRenewalWindow(notBefore, notAfter, renewBeforeDays)
}

func effectiveRenewalWindowDays(notBefore, notAfter time.Time, renewBeforeDays uint32) uint32 {
	return certstate.EffectiveRenewalWindowDays(notBefore, notAfter, renewBeforeDays)
}

func nextRenewalAt(v *store.CertificateVersion, renewBeforeDays uint32) *time.Time {
	if v == nil || !versionDistributable(v) {
		return nil
	}
	window := effectiveRenewalWindow(v.NotBefore, v.NotAfter, renewBeforeDays)
	t := v.NotAfter.Add(-window)
	return &t
}

func renewalMetadata(cert store.ManagedCertificate, now time.Time) (effectiveDays uint32, nextAt *time.Time) {
	if cert.ActiveVersion == nil {
		return 0, nil
	}
	v := cert.ActiveVersion
	effectiveDays = effectiveRenewalWindowDays(v.NotBefore, v.NotAfter, cert.RenewBeforeDays)
	nextAt = nextRenewalAt(v, cert.RenewBeforeDays)
	if nextAt != nil && now.After(v.NotAfter) {
		return effectiveDays, nil
	}
	return effectiveDays, nextAt
}

func computeVersionValidity(v *store.CertificateVersion, renewBeforeDays uint32, now time.Time) (validity string, bundleAvailable bool) {
	return certstate.ComputeVersionValidity(v, renewBeforeDays, now)
}

func computeValidity(cert store.ManagedCertificate, now time.Time) (validity string, bundleAvailable bool) {
	return certstate.ComputeValidity(cert, now)
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
