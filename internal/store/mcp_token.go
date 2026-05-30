package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mcpTokenPrefix is prepended to every generated MCP token so secrets are
// recognizable in logs and client configuration.
const mcpTokenPrefix = "mcp_"

// McpToken is a named API token for the MCP endpoint. The plaintext secret is
// only returned once at creation time; storage keeps a SHA-256 hash plus a
// short display prefix so tokens can be listed and revoked without exposing the
// secret.
type McpToken struct {
	ID         string    `bson:"id" json:"id"`
	Name       string    `bson:"name" json:"name"`
	TokenHash  string    `bson:"token_hash" json:"-"`
	Prefix     string    `bson:"prefix" json:"prefix"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	LastUsedAt time.Time `bson:"last_used_at,omitempty" json:"last_used_at,omitempty"`
}

// EnsureMcpTokenIndexes creates the unique index on token_hash used to look up
// and authenticate MCP tokens.
func (s *MongoStore) EnsureMcpTokenIndexes(ctx context.Context) error {
	_, err := s.mcpTokens().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "token_hash", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_token_hash"),
	})
	return err
}

// ListMcpTokens returns all MCP tokens, most recent first. Secrets are never
// included.
func (s *MongoStore) ListMcpTokens(ctx context.Context) ([]McpToken, error) {
	cursor, err := s.mcpTokens().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	tokens := make([]McpToken, 0)
	if err := cursor.All(ctx, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// CreateMcpToken generates a new random token, stores its hash, and returns the
// stored record together with the plaintext secret. The plaintext is only
// available from this call and cannot be recovered later.
func (s *MongoStore) CreateMcpToken(ctx context.Context, name string) (McpToken, string, error) {
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return McpToken{}, "", err
	}
	plaintext := mcpTokenPrefix + hex.EncodeToString(secretBytes)
	now := time.Now().UTC()
	token := McpToken{
		ID:        "mcp_" + uuid.NewString(),
		Name:      name,
		TokenHash: hashMcpToken(plaintext),
		Prefix:    plaintext[:len(mcpTokenPrefix)+8],
		CreatedAt: now,
	}
	if _, err := s.mcpTokens().InsertOne(ctx, token); err != nil {
		return McpToken{}, "", err
	}
	return token, plaintext, nil
}

// DeleteMcpToken revokes a token by id.
func (s *MongoStore) DeleteMcpToken(ctx context.Context, id string) error {
	res, err := s.mcpTokens().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// CountMcpTokens returns the number of stored MCP tokens. The MCP endpoint uses
// this to decide whether token auth is enforced.
func (s *MongoStore) CountMcpTokens(ctx context.Context) (int64, error) {
	return s.mcpTokens().CountDocuments(ctx, bson.M{})
}

// ValidateMcpToken reports whether the plaintext token matches a stored token.
// On a match it updates last_used_at on a best-effort basis.
func (s *MongoStore) ValidateMcpToken(ctx context.Context, plaintext string) (bool, error) {
	if plaintext == "" {
		return false, nil
	}
	filter := bson.M{"token_hash": hashMcpToken(plaintext)}
	res := s.mcpTokens().FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": bson.M{"last_used_at": time.Now().UTC()}},
	)
	if err := res.Err(); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func hashMcpToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func (s *MongoStore) mcpTokens() *mongo.Collection { return s.database.Collection("mcp_tokens") }
