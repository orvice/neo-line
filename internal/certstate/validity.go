// Package certstate computes ManagedCertificate validity and renewal state.
package certstate

import (
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// VersionDistributable reports whether a certificate version can be served.
func VersionDistributable(v *store.CertificateVersion) bool {
	return v != nil && !v.RevokePending && v.RevokedAt == nil
}

// EffectiveRenewalWindow caps the configured window at one third of validity.
func EffectiveRenewalWindow(notBefore, notAfter time.Time, renewBeforeDays uint32) time.Duration {
	configured := time.Duration(renewBeforeDays) * 24 * time.Hour
	lifetime := notAfter.Sub(notBefore)
	if lifetime <= 0 {
		return configured
	}
	third := lifetime / 3
	if third < configured {
		return third
	}
	return configured
}

// EffectiveRenewalWindowDays returns the displayed whole-day renewal window.
func EffectiveRenewalWindowDays(notBefore, notAfter time.Time, renewBeforeDays uint32) uint32 {
	window := EffectiveRenewalWindow(notBefore, notAfter, renewBeforeDays)
	days := window / (24 * time.Hour)
	if days == 0 && window > 0 {
		return 1
	}
	return uint32(days)
}

// ComputeVersionValidity evaluates one certificate version at now.
func ComputeVersionValidity(v *store.CertificateVersion, renewBeforeDays uint32, now time.Time) (validity string, bundleAvailable bool) {
	if v == nil {
		return store.CertValidityMissing, false
	}
	if !VersionDistributable(v) {
		return store.CertValidityRevoked, false
	}
	bundleAvailable = true
	if now.After(v.NotAfter) {
		return store.CertValidityExpired, bundleAvailable
	}
	if !now.Before(v.NotBefore) {
		window := EffectiveRenewalWindow(v.NotBefore, v.NotAfter, renewBeforeDays)
		if !now.Before(v.NotAfter.Add(-window)) {
			return store.CertValidityRenewalDue, bundleAvailable
		}
	}
	return store.CertValidityValid, bundleAvailable
}

// ComputeValidity evaluates the active version of a ManagedCertificate.
func ComputeValidity(cert store.ManagedCertificate, now time.Time) (validity string, bundleAvailable bool) {
	return ComputeVersionValidity(cert.ActiveVersion, cert.RenewBeforeDays, now)
}
