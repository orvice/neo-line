package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const certAccessTokenSecretPrefix = "nlct_"

// ErrCertificateAccessTokenNameTaken is returned when a token name collides within a server.
var ErrCertificateAccessTokenNameTaken = errors.New("certificate access token name already exists for this server")

// CertificateAccessToken is a long-lived credential bound to one Server for
// certificate bundle distribution. Plaintext secrets are only returned at
// creation; storage keeps a SHA-256 hash and display prefix.
type CertificateAccessToken struct {
	ID         string     `bson:"id" json:"id"`
	ServerID   string     `bson:"server_id" json:"server_id"`
	Name       string     `bson:"name" json:"name"`
	TokenHash  string     `bson:"token_hash" json:"-"`
	Prefix     string     `bson:"prefix" json:"prefix"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
	LastUsedAt *time.Time `bson:"last_used_at,omitempty" json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
}

func (s *MongoStore) certificateAccessTokens() *mongo.Collection {
	return s.database.Collection("certificate_access_tokens")
}

// EnsureCertificateAccessTokenIndexes creates indexes for certificate access tokens.
func (s *MongoStore) EnsureCertificateAccessTokenIndexes(ctx context.Context) error {
	if _, err := s.certificateAccessTokens().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "token_hash", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_token_hash"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateAccessTokens().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "server_id", Value: 1},
			{Key: "name", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("uniq_server_name"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateAccessTokens().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "server_id", Value: 1}, {Key: "created_at", Value: -1}},
		Options: options.Index().SetName("server_created_at_desc"),
	}); err != nil {
		return err
	}
	return nil
}

// ListCertificateAccessTokensByServer returns tokens for a server, most recent first.
func (s *MongoStore) ListCertificateAccessTokensByServer(ctx context.Context, serverID string) ([]CertificateAccessToken, error) {
	if serverID == "" {
		return nil, ErrInvalidServerIDs
	}
	cursor, err := s.certificateAccessTokens().Find(ctx, bson.M{"server_id": serverID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	tokens := make([]CertificateAccessToken, 0)
	if err := cursor.All(ctx, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// CreateCertificateAccessToken generates a token, stores its hash, and returns the
// record with the one-time plaintext secret.
func (s *MongoStore) CreateCertificateAccessToken(ctx context.Context, serverID, name string, expiresAt *time.Time) (CertificateAccessToken, string, error) {
	if serverID == "" {
		return CertificateAccessToken{}, "", ErrInvalidServerIDs
	}
	if _, err := s.GetServer(ctx, serverID); err != nil {
		return CertificateAccessToken{}, "", err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return CertificateAccessToken{}, "", err
	}
	plaintext := certAccessTokenSecretPrefix + hex.EncodeToString(secretBytes)
	now := time.Now().UTC()
	token := CertificateAccessToken{
		ID:        "cat_" + uuid.NewString(),
		ServerID:  serverID,
		Name:      name,
		TokenHash: hashCertificateAccessToken(plaintext),
		Prefix:    plaintext[:len(certAccessTokenSecretPrefix)+8],
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	if _, err := s.certificateAccessTokens().InsertOne(ctx, token); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return CertificateAccessToken{}, "", ErrCertificateAccessTokenNameTaken
		}
		return CertificateAccessToken{}, "", err
	}
	return token, plaintext, nil
}

// DeleteCertificateAccessToken revokes a token scoped to the given server.
func (s *MongoStore) DeleteCertificateAccessToken(ctx context.Context, serverID, tokenID string) error {
	if serverID == "" || tokenID == "" {
		return mongo.ErrNoDocuments
	}
	res, err := s.certificateAccessTokens().DeleteOne(ctx, bson.M{"id": tokenID, "server_id": serverID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// DeleteCertificateAccessTokensByServer removes all tokens for a server.
func (s *MongoStore) DeleteCertificateAccessTokensByServer(ctx context.Context, serverID string) error {
	_, err := s.certificateAccessTokens().DeleteMany(ctx, bson.M{"server_id": serverID})
	return err
}

// RemoveServerFromManagedCertificates pulls serverID from every managed certificate assignment.
func (s *MongoStore) RemoveServerFromManagedCertificates(ctx context.Context, serverID string) error {
	_, err := s.managedCertificates().UpdateMany(ctx, bson.M{}, bson.M{
		"$pull": bson.M{"server_ids": serverID},
	})
	return err
}

// LookupCertificateAccessToken resolves a plaintext nlct_ secret to its stored
// token record. Invalid, expired, or deleted tokens return not found. On match
// last_used_at is updated on a best-effort basis. No caching is applied.
func (s *MongoStore) LookupCertificateAccessToken(ctx context.Context, plaintext string) (CertificateAccessToken, error) {
	if plaintext == "" || !strings.HasPrefix(plaintext, certAccessTokenSecretPrefix) {
		return CertificateAccessToken{}, mongo.ErrNoDocuments
	}
	now := time.Now().UTC()
	filter := bson.M{
		"token_hash": hashCertificateAccessToken(plaintext),
		"$or": []bson.M{
			{"expires_at": bson.M{"$exists": false}},
			{"expires_at": nil},
			{"expires_at": bson.M{"$gt": now}},
		},
	}
	var token CertificateAccessToken
	res := s.certificateAccessTokens().FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": bson.M{"last_used_at": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	if err := res.Decode(&token); err != nil {
		return CertificateAccessToken{}, err
	}
	return token, nil
}

// ValidateCertificateAccessToken reports whether the plaintext matches a stored,
// non-expired token for the given server. On match it updates last_used_at on a
// best-effort basis. No caching is applied.
func (s *MongoStore) ValidateCertificateAccessToken(ctx context.Context, serverID, plaintext string) (bool, error) {
	if serverID == "" || plaintext == "" {
		return false, nil
	}
	now := time.Now().UTC()
	filter := bson.M{
		"server_id":  serverID,
		"token_hash": hashCertificateAccessToken(plaintext),
		"$or": []bson.M{
			{"expires_at": bson.M{"$exists": false}},
			{"expires_at": nil},
			{"expires_at": bson.M{"$gt": now}},
		},
	}
	res := s.certificateAccessTokens().FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": bson.M{"last_used_at": now}},
	)
	if err := res.Err(); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func hashCertificateAccessToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
