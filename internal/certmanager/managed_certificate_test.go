package certmanager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/orvice/neo-line/internal/store"
)

type managedCertFakeStore struct {
	noopIssueStore
	mu      sync.Mutex
	certs   map[string]store.ManagedCertificate
	certOrd []string
	ops     map[string]store.CertificateOperation
	opOrd   []string

	issuers map[string]store.CertificateIssuer
	dns     map[string]store.DNSProviderAccount
	notify  map[string]store.NotifyGroup
	servers map[string]store.Server
}

func newManagedCertFakeStore() *managedCertFakeStore {
	return &managedCertFakeStore{
		certs:   make(map[string]store.ManagedCertificate),
		ops:     make(map[string]store.CertificateOperation),
		issuers: make(map[string]store.CertificateIssuer),
		dns:     make(map[string]store.DNSProviderAccount),
		notify:  make(map[string]store.NotifyGroup),
		servers: make(map[string]store.Server),
	}
}

func (f *managedCertFakeStore) seedReadyIssuer(id string) {
	f.issuers[id] = store.CertificateIssuer{ID: id, RegistrationStatus: store.IssuerRegistrationReady}
}

func (f *managedCertFakeStore) seedDNS(id string) {
	f.dns[id] = store.DNSProviderAccount{ID: id}
}

func (f *managedCertFakeStore) ListDNSProviderAccounts(context.Context, int64, string) ([]store.DNSProviderAccount, string, error) {
	return nil, "", nil
}
func (f *managedCertFakeStore) CreateDNSProviderAccount(context.Context, store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return store.DNSProviderAccount{}, errors.New("not implemented")
}
func (f *managedCertFakeStore) GetDNSProviderAccount(_ context.Context, id string) (store.DNSProviderAccount, error) {
	a, ok := f.dns[id]
	if !ok {
		return store.DNSProviderAccount{}, mongo.ErrNoDocuments
	}
	return a, nil
}
func (f *managedCertFakeStore) UpdateDNSProviderAccount(context.Context, string, store.DNSProviderAccount) (store.DNSProviderAccount, error) {
	return store.DNSProviderAccount{}, errors.New("not implemented")
}
func (f *managedCertFakeStore) DeleteDNSProviderAccount(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *managedCertFakeStore) ListCertificateIssuers(context.Context, int64, string) ([]store.CertificateIssuer, string, error) {
	return nil, "", nil
}
func (f *managedCertFakeStore) CreateCertificateIssuer(context.Context, store.CertificateIssuer) (store.CertificateIssuer, error) {
	return store.CertificateIssuer{}, errors.New("not implemented")
}
func (f *managedCertFakeStore) GetCertificateIssuer(_ context.Context, id string) (store.CertificateIssuer, error) {
	i, ok := f.issuers[id]
	if !ok {
		return store.CertificateIssuer{}, mongo.ErrNoDocuments
	}
	return i, nil
}
func (f *managedCertFakeStore) UpdateCertificateIssuer(context.Context, string, store.CertificateIssuer) (store.CertificateIssuer, error) {
	return store.CertificateIssuer{}, errors.New("not implemented")
}
func (f *managedCertFakeStore) DeleteCertificateIssuer(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *managedCertFakeStore) ListManagedCertificates(_ context.Context, _ int64, _ string) ([]store.ManagedCertificate, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.ManagedCertificate, 0, len(f.certOrd))
	for _, id := range f.certOrd {
		out = append(out, f.certs[id])
	}
	return out, "", nil
}

func (f *managedCertFakeStore) CreateManagedCertificate(_ context.Context, cert store.ManagedCertificate) (store.ManagedCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.certs {
		if c.Name == cert.Name {
			return store.ManagedCertificate{}, store.ErrManagedCertificateNameTaken
		}
	}
	if cert.ID == "" {
		cert.ID = "mcert_test"
	}
	now := time.Now().UTC()
	cert.CreatedAt = now
	cert.UpdatedAt = now
	f.certs[cert.ID] = cert
	f.certOrd = append(f.certOrd, cert.ID)
	return cert, nil
}

func (f *managedCertFakeStore) GetManagedCertificate(_ context.Context, id string) (store.ManagedCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.certs[id]
	if !ok {
		return store.ManagedCertificate{}, mongo.ErrNoDocuments
	}
	return c, nil
}

func (f *managedCertFakeStore) UpdateManagedCertificate(_ context.Context, id string, cert store.ManagedCertificate) (store.ManagedCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.certs[id]; !ok {
		return store.ManagedCertificate{}, mongo.ErrNoDocuments
	}
	for otherID, c := range f.certs {
		if otherID != id && c.Name == cert.Name {
			return store.ManagedCertificate{}, store.ErrManagedCertificateNameTaken
		}
	}
	cert.ID = id
	cert.UpdatedAt = time.Now().UTC()
	f.certs[id] = cert
	return cert, nil
}

func (f *managedCertFakeStore) CreateCertificateOperation(_ context.Context, op store.CertificateOperation) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if op.ID == "" {
		op.ID = "cop_test"
	}
	now := time.Now().UTC()
	op.CreatedAt = now
	op.UpdatedAt = now
	f.ops[op.ID] = op
	f.opOrd = append(f.opOrd, op.ID)
	return op, nil
}

func (f *managedCertFakeStore) GetCertificateOperation(_ context.Context, id string) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok {
		return store.CertificateOperation{}, mongo.ErrNoDocuments
	}
	return op, nil
}

func (f *managedCertFakeStore) FindRunningCertificateOperation(_ context.Context, managedCertificateID, opType string) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.opOrd {
		op := f.ops[id]
		if op.ManagedCertificateID != managedCertificateID || op.Type != opType {
			continue
		}
		if op.Status == store.CertOpStatusPending || op.Status == store.CertOpStatusRunning {
			return op, nil
		}
	}
	return store.CertificateOperation{}, mongo.ErrNoDocuments
}

func (f *managedCertFakeStore) ListCertificateOperationsByCertificate(_ context.Context, managedCertificateID string, _ int64) ([]store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.CertificateOperation
	for i := len(f.opOrd) - 1; i >= 0; i-- {
		op := f.ops[f.opOrd[i]]
		if op.ManagedCertificateID == managedCertificateID {
			out = append(out, op)
		}
	}
	return out, nil
}

func (f *managedCertFakeStore) LatestCertificateOperation(_ context.Context, managedCertificateID string) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.opOrd) - 1; i >= 0; i-- {
		op := f.ops[f.opOrd[i]]
		if op.ManagedCertificateID == managedCertificateID {
			return op, nil
		}
	}
	return store.CertificateOperation{}, mongo.ErrNoDocuments
}

func (f *managedCertFakeStore) ValidateNotifyGroupIDs(_ context.Context, ids []string) error {
	for _, id := range ids {
		if id == "" {
			return store.ErrInvalidNotifyGroupIDs
		}
		if _, ok := f.notify[id]; !ok {
			return store.ErrInvalidNotifyGroupIDs
		}
	}
	return nil
}

func (f *managedCertFakeStore) ValidateServerIDs(_ context.Context, ids []string) error {
	for _, id := range ids {
		if id == "" {
			return store.ErrInvalidServerIDs
		}
		if _, ok := f.servers[id]; !ok {
			return store.ErrInvalidServerIDs
		}
	}
	return nil
}

func (f *managedCertFakeStore) ClaimPendingIssueOperation(_ context.Context, opID string) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusPending || op.Type != store.CertOpTypeIssue {
		return store.CertificateOperation{}, errors.New("not claimable")
	}
	now := time.Now().UTC()
	op.Status = store.CertOpStatusRunning
	op.AttemptCount++
	op.StartedAt = &now
	op.UpdatedAt = now
	f.ops[opID] = op
	return op, nil
}

func (f *managedCertFakeStore) FailIssueOperation(_ context.Context, opID, summary string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[opID]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now().UTC()
	op.Status = store.CertOpStatusFailed
	op.ErrorSummary = summary
	op.FinishedAt = &now
	op.UpdatedAt = now
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) FindPendingIssueOperations(_ context.Context, _ int64) ([]store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.CertificateOperation
	for _, id := range f.opOrd {
		op := f.ops[id]
		if op.Status == store.CertOpStatusPending && op.Type == store.CertOpTypeIssue {
			out = append(out, op)
		}
	}
	return out, nil
}

func (f *managedCertFakeStore) UpdateCertificateOperation(_ context.Context, id string, op store.CertificateOperation) (store.CertificateOperation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.ops[id]; !ok {
		return store.CertificateOperation{}, errors.New("not found")
	}
	op.ID = id
	op.UpdatedAt = time.Now().UTC()
	f.ops[id] = op
	return op, nil
}

func (f *managedCertFakeStore) ActivateFirstIssueVersion(_ context.Context, managedCertID string, version store.CertificateVersion, opID, warning string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[managedCertID]
	if !ok || cert.ActiveVersion != nil {
		return store.ErrActiveVersionConflict
	}
	cert.ActiveVersion = &version
	cert.UpdatedAt = time.Now().UTC()
	f.certs[managedCertID] = cert
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning {
		return store.ErrCertificateOperationConflict
	}
	now := time.Now().UTC()
	op.Status = store.CertOpStatusSucceeded
	op.FinishedAt = &now
	op.Warning = warning
	op.UpdatedAt = now
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) ActivateSubsequentIssueVersion(_ context.Context, managedCertID string, version store.CertificateVersion, expectedActiveID, opID, warning string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[managedCertID]
	if !ok || cert.ActiveVersion == nil || cert.ActiveVersion.ID != expectedActiveID {
		return store.ErrActiveVersionConflict
	}
	previous := *cert.ActiveVersion
	cert.PreviousVersion = &previous
	cert.ActiveVersion = &version
	cert.UpdatedAt = time.Now().UTC()
	f.certs[managedCertID] = cert
	op, ok := f.ops[opID]
	if !ok || op.Status != store.CertOpStatusRunning {
		return store.ErrCertificateOperationConflict
	}
	now := time.Now().UTC()
	op.Status = store.CertOpStatusSucceeded
	op.FinishedAt = &now
	op.Warning = warning
	op.UpdatedAt = now
	f.ops[opID] = op
	return nil
}

func (f *managedCertFakeStore) ActivatePreviousVersion(_ context.Context, managedCertID, versionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[managedCertID]
	if !ok || cert.PreviousVersion == nil || cert.PreviousVersion.ID != versionID {
		return store.ErrVersionNotFound
	}
	if cert.PreviousVersion.RevokedAt != nil {
		return store.ErrVersionRevoked
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
	f.certs[managedCertID] = cert
	return nil
}

func boolPtr(v bool) *bool { return &v }

func TestNormalizeDomains(t *testing.T) {
	got, err := NormalizeDomains([]string{" Example.COM.", "example.com", "  EXAMPLE.com  ", "München.de"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := []string{"example.com", "xn--mnchen-3ya.de"}
	if len(got) != len(want) {
		t.Fatalf("domains = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("domains[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}

	if _, err := NormalizeDomains([]string{"*.example.com", "api.example.com"}); err != nil {
		t.Fatalf("wildcard ok: %v", err)
	}
	if _, err := NormalizeDomains([]string{"*.*.example.com"}); err == nil {
		t.Fatal("expected wildcard error")
	}
}

func TestCreateManagedCertificatePendingIssueAndMissingValidity(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	got, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ActiveValidity != store.CertValidityMissing {
		t.Fatalf("validity = %q, want Missing", got.ActiveValidity)
	}
	if got.BundleAvailable {
		t.Fatal("expected bundle unavailable")
	}
	if got.LatestOperation == nil {
		t.Fatal("expected pending operation")
	}
	if got.LatestOperation.Status != store.CertOpStatusPending {
		t.Fatalf("op status = %q, want Pending", got.LatestOperation.Status)
	}
	if got.LatestOperation.Type != store.CertOpTypeIssue {
		t.Fatalf("op type = %q, want Issue", got.LatestOperation.Type)
	}
	if got.KeyType != store.CertKeyTypeECP256 {
		t.Fatalf("key type = %q, want ec_p256", got.KeyType)
	}
	if got.RenewBeforeDays != store.DefaultRenewBeforeDays {
		t.Fatalf("renew_before = %d, want %d", got.RenewBeforeDays, store.DefaultRenewBeforeDays)
	}
}

func TestCreateRejectsNotReadyIssuer(t *testing.T) {
	st := newManagedCertFakeStore()
	st.issuers["iss_1"] = store.CertificateIssuer{ID: "iss_1", RegistrationStatus: store.IssuerRegistrationPending}
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if !errors.Is(err, ErrIssuerNotReady) {
		t.Fatalf("expected ErrIssuerNotReady, got %v", err)
	}
}

func TestCreateUniqueName(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)
	input := ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"a.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	}
	if _, err := m.CreateManagedCertificate(context.Background(), input); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := m.CreateManagedCertificate(context.Background(), input)
	if !errors.Is(err, store.ErrManagedCertificateNameTaken) {
		t.Fatalf("expected name taken, got %v", err)
	}
}

func TestSubmitIssueOperationIdempotentWhenPending(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	created, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	first, err := m.SubmitIssueOperation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	second, err := m.SubmitIssueOperation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same pending op, got %q and %q", first.ID, second.ID)
	}
	if len(st.ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(st.ops))
	}
}

func TestUpdateBlocksIssueFieldsWhilePending(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	created, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = m.UpdateManagedCertificate(context.Background(), created.ID, ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"other.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if !errors.Is(err, ErrIssueFieldsLocked) {
		t.Fatalf("expected ErrIssueFieldsLocked during pending, got %v", err)
	}
}

func TestSubmitIssueOperationIdempotentWhenRunning(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	created, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	st.mu.Lock()
	for id, op := range st.ops {
		op.Status = store.CertOpStatusRunning
		st.ops[id] = op
	}
	st.mu.Unlock()

	first, err := m.SubmitIssueOperation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	second, err := m.SubmitIssueOperation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same running op, got %q and %q", first.ID, second.ID)
	}
	if len(st.ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(st.ops))
	}
}

func TestUpdateBlocksIssueFieldsWhileRunning(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	created, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	st.mu.Lock()
	for id, op := range st.ops {
		op.Status = store.CertOpStatusRunning
		st.ops[id] = op
	}
	st.mu.Unlock()

	_, err = m.UpdateManagedCertificate(context.Background(), created.ID, ManagedCertificateInput{
		Name:                 "prod",
		Domains:              []string{"other.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if !errors.Is(err, ErrIssueFieldsLocked) {
		t.Fatalf("expected ErrIssueFieldsLocked, got %v", err)
	}

	updated, err := m.UpdateManagedCertificate(context.Background(), created.ID, ManagedCertificateInput{
		Name:                 "prod-renamed",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            []string{},
	})
	if err != nil {
		t.Fatalf("update allowed fields: %v", err)
	}
	if updated.Name != "prod-renamed" {
		t.Fatalf("name = %q", updated.Name)
	}
}

func TestValidateRefs(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	st.notify["ntf_1"] = store.NotifyGroup{ID: "ntf_1"}
	st.servers["srv_1"] = store.Server{ID: "srv_1"}
	m := NewManager(st, nil)

	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "with-refs",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"ntf_1"},
		ServerIDs:            []string{"srv_1"},
	})
	if err != nil {
		t.Fatalf("create with refs: %v", err)
	}

	_, err = m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "bad-notify",
		Domains:              []string{"b.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		NotifyGroupIDs:       []string{"missing"},
	})
	if !errors.Is(err, store.ErrInvalidNotifyGroupIDs) {
		t.Fatalf("expected invalid notify, got %v", err)
	}

	_, err = m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "bad-server",
		Domains:              []string{"c.example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            []string{"missing"},
	})
	if !errors.Is(err, store.ErrInvalidServerIDs) {
		t.Fatalf("expected invalid server, got %v", err)
	}
}

func TestCreateAllowsZeroServers(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	_, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "no-servers",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		ServerIDs:            nil,
	})
	if err != nil {
		t.Fatalf("create zero servers: %v", err)
	}
}

func TestAutoRenewDefaultTrue(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	got, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "auto",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !got.AutoRenewEnabled {
		t.Fatal("expected auto_renew default true")
	}
}

func TestAutoRenewExplicitFalse(t *testing.T) {
	st := newManagedCertFakeStore()
	st.seedReadyIssuer("iss_1")
	st.seedDNS("dns_1")
	m := NewManager(st, nil)

	got, err := m.CreateManagedCertificate(context.Background(), ManagedCertificateInput{
		Name:                 "manual",
		Domains:              []string{"example.com"},
		CertificateIssuerID:  "iss_1",
		DNSProviderAccountID: "dns_1",
		AutoRenewEnabled:     boolPtr(false),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.AutoRenewEnabled {
		t.Fatal("expected auto_renew false")
	}
}
