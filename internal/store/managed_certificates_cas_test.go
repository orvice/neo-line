package store

import (
	"sync"
	"testing"
	"time"
)

// casManagedCertStore simulates Mongo single-document CAS for version switches.
type casManagedCertStore struct {
	mu    sync.Mutex
	certs map[string]ManagedCertificate
}

func newCASManagedCertStore() *casManagedCertStore {
	return &casManagedCertStore{certs: make(map[string]ManagedCertificate)}
}

func (s *casManagedCertStore) activateSubsequent(managedCertID string, version CertificateVersion, expectedActiveID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cert, ok := s.certs[managedCertID]
	if !ok || cert.ActiveVersion == nil || cert.ActiveVersion.ID != expectedActiveID {
		return ErrActiveVersionConflict
	}
	previous := *cert.ActiveVersion
	cert.PreviousVersion = &previous
	cert.ActiveVersion = &version
	cert.UpdatedAt = time.Now().UTC()
	s.certs[managedCertID] = cert
	return nil
}

func (s *casManagedCertStore) activatePrevious(managedCertID, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cert, ok := s.certs[managedCertID]
	if !ok || cert.PreviousVersion == nil || cert.PreviousVersion.ID != versionID {
		return ErrVersionNotFound
	}
	if cert.PreviousVersion.RevokedAt != nil {
		return ErrVersionRevoked
	}
	newActive := *cert.PreviousVersion
	if cert.ActiveVersion != nil {
		prev := *cert.ActiveVersion
		cert.PreviousVersion = &prev
	} else {
		cert.PreviousVersion = nil
	}
	cert.ActiveVersion = &newActive
	cert.UpdatedAt = time.Now().UTC()
	s.certs[managedCertID] = cert
	return nil
}

func TestCASSubsequentIssueRequiresMatchingActiveID(t *testing.T) {
	st := newCASManagedCertStore()
	active := CertificateVersion{ID: "cver_a", FullchainPEM: "A"}
	st.certs["mcert_1"] = ManagedCertificate{
		ID:            "mcert_1",
		ActiveVersion: &active,
	}
	newVersion := CertificateVersion{ID: "cver_b", FullchainPEM: "B"}

	if err := st.activateSubsequent("mcert_1", newVersion, "cver_a"); err != nil {
		t.Fatalf("first swap: %v", err)
	}
	cert := st.certs["mcert_1"]
	if cert.ActiveVersion.ID != "cver_b" || cert.PreviousVersion.ID != "cver_a" {
		t.Fatalf("after swap: active=%v previous=%v", cert.ActiveVersion, cert.PreviousVersion)
	}

	err := st.activateSubsequent("mcert_1", CertificateVersion{ID: "cver_c"}, "cver_a")
	if err != ErrActiveVersionConflict {
		t.Fatalf("stale active id: got %v", err)
	}
}

func TestCASPreviousActivationSwapsAndRejectsRevoked(t *testing.T) {
	st := newCASManagedCertStore()
	active := CertificateVersion{ID: "cver_a", FullchainPEM: "A"}
	revokedAt := time.Now().UTC()
	prev := CertificateVersion{ID: "cver_p", FullchainPEM: "P", RevokedAt: &revokedAt}
	st.certs["mcert_1"] = ManagedCertificate{
		ID:              "mcert_1",
		ActiveVersion:   &active,
		PreviousVersion: &prev,
	}
	if err := st.activatePrevious("mcert_1", "cver_p"); err != ErrVersionRevoked {
		t.Fatalf("revoked previous: got %v", err)
	}

	prev.RevokedAt = nil
	st.certs["mcert_1"] = ManagedCertificate{
		ID:              "mcert_1",
		ActiveVersion:   &active,
		PreviousVersion: &prev,
	}
	if err := st.activatePrevious("mcert_1", "cver_p"); err != nil {
		t.Fatalf("activate previous: %v", err)
	}
	cert := st.certs["mcert_1"]
	if cert.ActiveVersion.ID != "cver_p" || cert.PreviousVersion.ID != "cver_a" {
		t.Fatalf("rollback: active=%v previous=%v", cert.ActiveVersion, cert.PreviousVersion)
	}
}
