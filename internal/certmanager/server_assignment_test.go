package certmanager

import (
	"context"
	"testing"

	"github.com/orvice/neo-line/internal/store"
)

func TestUpdateManagedCertificateServerAssignmentNoExtraIssue(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	st.servers["srv_1"] = store.Server{ID: "srv_1"}
	st.servers["srv_2"] = store.Server{ID: "srv_2"}
	m := NewManager(st, nil)

	created, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "shared",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(st.ops) != 1 {
		t.Fatalf("expected one issue op on create, got %d", len(st.ops))
	}

	updated, err := m.UpdateManagedCertificate(context.Background(), created.ID, ManagedCertificateInput{
		Name:                 "shared",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            []string{"srv_1", "srv_2"},
	})
	if err != nil {
		t.Fatalf("assign servers: %v", err)
	}
	if len(updated.ServerIDs) != 2 {
		t.Fatalf("server_ids = %v", updated.ServerIDs)
	}
	if len(st.ops) != 1 {
		t.Fatalf("assignment must not create new operation, got %d ops", len(st.ops))
	}

	updated, err = m.UpdateManagedCertificate(context.Background(), created.ID, ManagedCertificateInput{
		Name:                 "shared",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            []string{"srv_2"},
	})
	if err != nil {
		t.Fatalf("unassign server: %v", err)
	}
	if len(updated.ServerIDs) != 1 || updated.ServerIDs[0] != "srv_2" {
		t.Fatalf("server_ids = %v", updated.ServerIDs)
	}
}
