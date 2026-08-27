package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeleteManagedCertificateConstraints(t *testing.T) {
	st := newManagedCertTestStore()
	ctx := context.Background()
	cert := ManagedCertificate{
		ID:                   "mcert_del",
		Name:                 "del-test",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              CertKeyTypeECP256,
		ServerIDs:            []string{"srv_1"},
	}
	if _, err := st.CreateManagedCertificate(ctx, cert); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.DeleteManagedCertificate(ctx, "mcert_del"); !errors.Is(err, ErrManagedCertificateHasServerAssignments) {
		t.Fatalf("servers: %v", err)
	}

	cert.ServerIDs = nil
	if _, err := st.UpdateManagedCertificate(ctx, "mcert_del", cert); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := st.CreateCertificateOperation(ctx, CertificateOperation{
		ManagedCertificateID: "mcert_del",
		Type:                 CertOpTypeRevoke,
		Status:               CertOpStatusRunning,
	}); err != nil {
		t.Fatalf("create op: %v", err)
	}
	if err := st.DeleteManagedCertificate(ctx, "mcert_del"); !errors.Is(err, ErrManagedCertificateOperationInFlight) {
		t.Fatalf("running op: %v", err)
	}
}

func TestCountManagedCertificateReferences(t *testing.T) {
	st := newManagedCertTestStore()
	ctx := context.Background()
	now := time.Now().UTC()
	cert := ManagedCertificate{
		ID:                   "mcert_ref",
		Name:                 "ref-test",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_desired",
		DNSProviderAccountID: "dns_desired",
		KeyType:              CertKeyTypeECP256,
		ActiveVersion: &CertificateVersion{
			ID: "cver_a",
			ConfigSnapshot: IssueConfigSnapshot{
				CertificateIssuerID:  "iss_active",
				DNSProviderAccountID: "dns_active",
			},
			CertificateIssuerID: "iss_active",
		},
		PreviousVersion: &CertificateVersion{
			ID: "cver_p",
			ConfigSnapshot: IssueConfigSnapshot{
				CertificateIssuerID:  "iss_prev",
				DNSProviderAccountID: "dns_prev",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := st.CreateManagedCertificate(ctx, cert); err != nil {
		t.Fatalf("create: %v", err)
	}

	for issuer, want := range map[string]bool{
		"iss_desired": true,
		"iss_active":  true,
		"iss_prev":    true,
		"iss_other":   false,
	} {
		count, err := st.CountManagedCertificatesReferencingIssuer(ctx, issuer)
		if err != nil {
			t.Fatal(err)
		}
		if (count > 0) != want {
			t.Fatalf("issuer %s count=%d want referenced=%v", issuer, count, want)
		}
	}
	for dns, want := range map[string]bool{
		"dns_desired": true,
		"dns_active":  true,
		"dns_prev":    true,
		"dns_other":   false,
	} {
		count, err := st.CountManagedCertificatesReferencingDNSAccount(ctx, dns)
		if err != nil {
			t.Fatal(err)
		}
		if (count > 0) != want {
			t.Fatalf("dns %s count=%d want referenced=%v", dns, count, want)
		}
	}
}

type managedCertTestStore struct {
	certs map[string]ManagedCertificate
	ops   map[string]CertificateOperation
}

func newManagedCertTestStore() *managedCertTestStore {
	return &managedCertTestStore{
		certs: make(map[string]ManagedCertificate),
		ops:   make(map[string]CertificateOperation),
	}
}

func (s *managedCertTestStore) CreateManagedCertificate(_ context.Context, cert ManagedCertificate) (ManagedCertificate, error) {
	s.certs[cert.ID] = cert
	return cert, nil
}

func (s *managedCertTestStore) UpdateManagedCertificate(_ context.Context, id string, cert ManagedCertificate) (ManagedCertificate, error) {
	s.certs[id] = cert
	return cert, nil
}

func (s *managedCertTestStore) GetManagedCertificate(_ context.Context, id string) (ManagedCertificate, error) {
	c, ok := s.certs[id]
	if !ok {
		return ManagedCertificate{}, errors.New("not found")
	}
	return c, nil
}

func (s *managedCertTestStore) CreateCertificateOperation(_ context.Context, op CertificateOperation) (CertificateOperation, error) {
	if op.ID == "" {
		op.ID = "cop_test"
	}
	s.ops[op.ID] = op
	return op, nil
}

func (s *managedCertTestStore) HasRunningCertificateOperation(_ context.Context, managedCertificateID string) (bool, error) {
	for _, op := range s.ops {
		if op.ManagedCertificateID != managedCertificateID {
			continue
		}
		for _, st := range CertOpInFlightStatuses {
			if op.Status == st {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *managedCertTestStore) DeleteManagedCertificate(ctx context.Context, id string) error {
	cert, err := s.GetManagedCertificate(ctx, id)
	if err != nil {
		return err
	}
	if len(cert.ServerIDs) > 0 {
		return ErrManagedCertificateHasServerAssignments
	}
	running, err := s.HasRunningCertificateOperation(ctx, id)
	if err != nil {
		return err
	}
	if running {
		return ErrManagedCertificateOperationInFlight
	}
	for opID, op := range s.ops {
		if op.ManagedCertificateID == id {
			delete(s.ops, opID)
		}
	}
	delete(s.certs, id)
	return nil
}

func (s *managedCertTestStore) CountManagedCertificatesReferencingIssuer(_ context.Context, issuerID string) (int64, error) {
	var count int64
	for _, cert := range s.certs {
		if cert.CertificateIssuerID == issuerID {
			count++
			continue
		}
		if cert.ActiveVersion != nil {
			if cert.ActiveVersion.CertificateIssuerID == issuerID || cert.ActiveVersion.ConfigSnapshot.CertificateIssuerID == issuerID {
				count++
				continue
			}
		}
		if cert.PreviousVersion != nil {
			if cert.PreviousVersion.CertificateIssuerID == issuerID || cert.PreviousVersion.ConfigSnapshot.CertificateIssuerID == issuerID {
				count++
			}
		}
	}
	return count, nil
}

func (s *managedCertTestStore) CountManagedCertificatesReferencingDNSAccount(_ context.Context, dnsID string) (int64, error) {
	var count int64
	for _, cert := range s.certs {
		if cert.DNSProviderAccountID == dnsID {
			count++
			continue
		}
		if cert.ActiveVersion != nil && cert.ActiveVersion.ConfigSnapshot.DNSProviderAccountID == dnsID {
			count++
			continue
		}
		if cert.PreviousVersion != nil && cert.PreviousVersion.ConfigSnapshot.DNSProviderAccountID == dnsID {
			count++
		}
	}
	return count, nil
}
