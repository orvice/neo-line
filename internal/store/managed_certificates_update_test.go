package store

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestManagedCertificateUpdateDocumentExcludesRuntimeState(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	change := managedCertificateUpdateDocument(ManagedCertificateUpdate{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		KeyType:              CertKeyTypeECP256,
		AutoRenewEnabled:     true,
		RenewBeforeDays:      30,
		NotifyGroupIDs:       []string{"notify_1"},
	}, now)

	set, ok := change["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set = %T, want bson.M", change["$set"])
	}
	for _, field := range []string{"id", "created_at", "active_version", "previous_version", "notification_state"} {
		if _, exists := set[field]; exists {
			t.Fatalf("runtime-owned field %q appears in $set", field)
		}
	}
	if got := set["updated_at"]; got != now {
		t.Fatalf("updated_at = %v, want %v", got, now)
	}
	if got := set["notify_group_ids"]; got == nil {
		t.Fatal("notify_group_ids was not set")
	}

	unset, ok := change["$unset"].(bson.M)
	if !ok {
		t.Fatalf("$unset = %T, want bson.M", change["$unset"])
	}
	if _, exists := unset["server_ids"]; !exists {
		t.Fatal("empty server_ids was not unset")
	}
}
