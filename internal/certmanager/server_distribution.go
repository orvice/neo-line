package certmanager

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// ErrCertificateNotAuthorized is returned when a certificate is missing or not
// assigned to the calling Server. Callers map this to not_found for anti-enumeration.
var ErrCertificateNotAuthorized = errors.New("certificate not found")

// ServerCertificate is the Server-facing view of an authorized ManagedCertificate.
type ServerCertificate struct {
	ManagedCertificateID string
	Name                 string
	Domains              []string
	ActiveVersionID      string
	Available            bool
	Validity             string
	KeyType              string
	LeafFingerprint      string
	NotBefore            int64
	NotAfter             int64
	StagingUntrusted     bool
	ErrorSummary         string
}

func (m *Manager) ListServerCertificates(ctx context.Context, serverID string) ([]ServerCertificate, error) {
	certs, err := m.store.ListManagedCertificatesByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	out := make([]ServerCertificate, 0, len(certs))
	now := m.clock.Now()
	for _, cert := range certs {
		entry, err := m.serverCertificateEntry(ctx, cert, now)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (m *Manager) GetServerCertificateBundle(ctx context.Context, serverID, managedCertificateID string) (CertificateBundle, error) {
	cert, err := m.store.GetManagedCertificate(ctx, managedCertificateID)
	if err != nil {
		if store.IsNotFound(err) {
			return CertificateBundle{}, ErrCertificateNotAuthorized
		}
		return CertificateBundle{}, err
	}
	if !serverAssigned(cert, serverID) {
		return CertificateBundle{}, ErrCertificateNotAuthorized
	}
	if cert.ActiveVersion == nil {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	if !versionDistributable(cert.ActiveVersion) {
		return CertificateBundle{}, ErrBundleNotAvailable
	}
	return m.bundleFromActive(cert, m.clock.Now())
}

func (m *Manager) serverCertificateEntry(ctx context.Context, cert store.ManagedCertificate, now time.Time) (ServerCertificate, error) {
	entry := ServerCertificate{
		ManagedCertificateID: cert.ID,
	}
	if cert.ActiveVersion == nil {
		entry.Name = cert.Name
		entry.Domains = append([]string(nil), cert.Domains...)
		entry.Available = false
		entry.Validity = store.CertValidityMissing
		if latest, err := m.store.LatestCertificateOperation(ctx, cert.ID); err == nil {
			if latest.Status == store.CertOpStatusFailed && latest.ErrorSummary != "" {
				entry.ErrorSummary = latest.ErrorSummary
			}
		} else if !store.IsNotFound(err) {
			return ServerCertificate{}, err
		}
		return entry, nil
	}

	v := cert.ActiveVersion
	validity, available := computeValidity(cert, now)
	entry.ActiveVersionID = v.ID
	entry.Domains = append([]string(nil), snapDomains(v)...)
	entry.Available = available
	entry.Validity = validity
	entry.KeyType = v.KeyType
	entry.LeafFingerprint = v.LeafFingerprint
	entry.NotBefore = v.NotBefore.Unix()
	entry.NotAfter = v.NotAfter.Unix()
	entry.StagingUntrusted = v.StagingUntrusted
	return entry, nil
}

func (m *Manager) bundleFromActive(cert store.ManagedCertificate, now time.Time) (CertificateBundle, error) {
	v := cert.ActiveVersion
	validity, _ := computeValidity(cert, now)
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

func serverAssigned(cert store.ManagedCertificate, serverID string) bool {
	return slices.Contains(cert.ServerIDs, serverID)
}

func versionDistributable(v *store.CertificateVersion) bool {
	if v == nil {
		return false
	}
	if v.RevokePending {
		return false
	}
	if v.RevokedAt != nil && !v.RevokedAt.IsZero() {
		return false
	}
	return true
}
