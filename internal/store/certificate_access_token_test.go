package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type certAccessTokenMemStore struct {
	*MongoStore
	servers map[string]Server
	tokens  map[string]CertificateAccessToken
	certs   map[string]ManagedCertificate
}

func newCertAccessTokenMemStore() *certAccessTokenMemStore {
	return &certAccessTokenMemStore{
		servers: make(map[string]Server),
		tokens:  make(map[string]CertificateAccessToken),
		certs:   make(map[string]ManagedCertificate),
	}
}

func (m *certAccessTokenMemStore) GetServer(_ context.Context, id string) (Server, error) {
	s, ok := m.servers[id]
	if !ok {
		return Server{}, mongo.ErrNoDocuments
	}
	return s, nil
}

func (m *certAccessTokenMemStore) ListCertificateAccessTokensByServer(_ context.Context, serverID string) ([]CertificateAccessToken, error) {
	if serverID == "" {
		return nil, ErrInvalidServerIDs
	}
	out := make([]CertificateAccessToken, 0)
	for _, t := range m.tokens {
		if t.ServerID == serverID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *certAccessTokenMemStore) CreateCertificateAccessToken(_ context.Context, serverID, name string, expiresAt *time.Time) (CertificateAccessToken, string, error) {
	if serverID == "" {
		return CertificateAccessToken{}, "", ErrInvalidServerIDs
	}
	if _, err := m.GetServer(context.Background(), serverID); err != nil {
		return CertificateAccessToken{}, "", err
	}
	for _, t := range m.tokens {
		if t.ServerID == serverID && t.Name == name {
			return CertificateAccessToken{}, "", ErrCertificateAccessTokenNameTaken
		}
	}
	suffix := hex.EncodeToString([]byte(name + serverID))
	if len(suffix) < 64 {
		suffix = suffix + strings.Repeat("0", 64-len(suffix))
	}
	plaintext := certAccessTokenSecretPrefix + suffix[:64]
	token := CertificateAccessToken{
		ID:        "cat_" + serverID + "_" + name,
		ServerID:  serverID,
		Name:      name,
		TokenHash: hashCertificateAccessToken(plaintext),
		Prefix:    plaintext[:len(certAccessTokenSecretPrefix)+8],
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
	m.tokens[token.ID] = token
	return token, plaintext, nil
}

func (m *certAccessTokenMemStore) DeleteCertificateAccessToken(_ context.Context, serverID, tokenID string) error {
	t, ok := m.tokens[tokenID]
	if !ok || t.ServerID != serverID {
		return mongo.ErrNoDocuments
	}
	delete(m.tokens, tokenID)
	return nil
}

func (m *certAccessTokenMemStore) ValidateCertificateAccessToken(_ context.Context, serverID, plaintext string) (bool, error) {
	hash := hashCertificateAccessToken(plaintext)
	now := time.Now().UTC()
	for id, t := range m.tokens {
		if t.ServerID != serverID || t.TokenHash != hash {
			continue
		}
		if t.ExpiresAt != nil && !t.ExpiresAt.IsZero() && !t.ExpiresAt.After(now) {
			return false, nil
		}
		t.LastUsedAt = &now
		m.tokens[id] = t
		return true, nil
	}
	return false, nil
}

func TestCertificateAccessTokenSecretFormatAndHash(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.servers["srv_1"] = Server{ID: "srv_1", Name: "web-1"}

	token, plaintext, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "deploy", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(plaintext, certAccessTokenSecretPrefix) {
		t.Fatalf("secret prefix = %q", plaintext[:6])
	}
	if token.TokenHash == plaintext {
		t.Fatal("stored hash must not equal plaintext")
	}
	sum := sha256.Sum256([]byte(plaintext))
	wantHash := hex.EncodeToString(sum[:])
	if token.TokenHash != wantHash {
		t.Fatalf("hash mismatch")
	}
	if token.Prefix != plaintext[:len(certAccessTokenSecretPrefix)+8] {
		t.Fatalf("prefix = %q", token.Prefix)
	}
}

func TestCertificateAccessTokenNameUniquePerServer(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.servers["srv_1"] = Server{ID: "srv_1"}
	st.servers["srv_2"] = Server{ID: "srv_2"}

	if _, _, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "rotate", nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, _, err := st.CreateCertificateAccessToken(context.Background(), "srv_2", "rotate", nil); err != nil {
		t.Fatalf("same name other server: %v", err)
	}
	if _, _, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "rotate", nil); err == nil {
		t.Fatal("expected name taken on same server")
	} else if err != ErrCertificateAccessTokenNameTaken {
		t.Fatalf("expected ErrCertificateAccessTokenNameTaken, got %v", err)
	}
}

func TestCertificateAccessTokenMultipleAndDelete(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.servers["srv_1"] = Server{ID: "srv_1"}

	first, _, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "a", nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, _, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "b", nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	list, err := st.ListCertificateAccessTokensByServer(context.Background(), "srv_1")
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, err = %v", len(list), err)
	}
	if err := st.DeleteCertificateAccessToken(context.Background(), "srv_1", first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = st.ListCertificateAccessTokensByServer(context.Background(), "srv_1")
	if len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("after delete list = %#v", list)
	}
}

func TestCertificateAccessTokenExpiredValidation(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.servers["srv_1"] = Server{ID: "srv_1"}
	past := time.Now().UTC().Add(-time.Hour)
	_, plaintext, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "expired", &past)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ok, err := st.ValidateCertificateAccessToken(context.Background(), "srv_1", plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if ok {
		t.Fatal("expired token must not validate")
	}
}

func TestCertificateAccessTokenValidateUpdatesLastUsed(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.servers["srv_1"] = Server{ID: "srv_1"}
	_, plaintext, err := st.CreateCertificateAccessToken(context.Background(), "srv_1", "live", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ok, err := st.ValidateCertificateAccessToken(context.Background(), "srv_1", plaintext)
	if err != nil || !ok {
		t.Fatalf("validate = %v, %v", ok, err)
	}
	for _, tok := range st.tokens {
		if tok.LastUsedAt == nil {
			t.Fatal("expected last_used_at update")
		}
	}
}

func TestRemoveServerFromManagedCertificates(t *testing.T) {
	st := newCertAccessTokenMemStore()
	st.certs["mcert_1"] = ManagedCertificate{ID: "mcert_1", ServerIDs: []string{"srv_1", "srv_2"}}
	st.certs["mcert_2"] = ManagedCertificate{ID: "mcert_2", ServerIDs: []string{"srv_1"}}

	removeServerIDs := func(serverID string) {
		for id, cert := range st.certs {
			out := make([]string, 0, len(cert.ServerIDs))
			for _, sid := range cert.ServerIDs {
				if sid != serverID {
					out = append(out, sid)
				}
			}
			cert.ServerIDs = out
			st.certs[id] = cert
		}
	}
	removeServerIDs("srv_1")

	if got := st.certs["mcert_1"].ServerIDs; len(got) != 1 || got[0] != "srv_2" {
		t.Fatalf("mcert_1 server_ids = %v", got)
	}
	if len(st.certs["mcert_2"].ServerIDs) != 0 {
		t.Fatalf("mcert_2 should be empty")
	}
}
