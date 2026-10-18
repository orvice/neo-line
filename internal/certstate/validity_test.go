package certstate

import (
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestEffectiveRenewalWindowUsesOneThirdForShortCertificate(t *testing.T) {
	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(30 * 24 * time.Hour)
	if got := EffectiveRenewalWindowDays(notBefore, notAfter, 30); got != 10 {
		t.Fatalf("effective days = %d, want 10", got)
	}
}

func TestComputeValidityTreatsAnyRevokedAtAsRevoked(t *testing.T) {
	zero := time.Time{}
	cert := store.ManagedCertificate{
		RenewBeforeDays: 30,
		ActiveVersion: &store.CertificateVersion{
			NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			NotAfter:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			RevokedAt: &zero,
		},
	}
	validity, available := ComputeValidity(cert, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if validity != store.CertValidityRevoked || available {
		t.Fatalf("validity=%q available=%v, want Revoked/false", validity, available)
	}
}
