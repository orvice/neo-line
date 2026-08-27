package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	DNSProviderCloudflare            = "cloudflare"
	DefaultDNSPropagationTimeoutSecs = 120
	MinDNSPropagationTimeoutSecs     = 30
	MaxDNSPropagationTimeoutSecs     = 900
)

// ErrDNSProviderAccountNameTaken is returned when a DNSProviderAccount name
// collides with an existing account.
var ErrDNSProviderAccountNameTaken = errors.New("dns provider account name already exists")

// ErrInvalidDNSProvider is returned when an unsupported provider type is given.
var ErrInvalidDNSProvider = errors.New("unsupported dns provider")

// ErrInvalidPropagationTimeout is returned when propagation_timeout_seconds is
// outside the allowed 30–900 range.
var ErrInvalidPropagationTimeout = errors.New("propagation_timeout_seconds must be between 30 and 900")

// DNSProviderAccount stores reusable DNS-01 provider credentials. APIToken is
// persisted in MongoDB but must never appear in API responses or audit logs.
type DNSProviderAccount struct {
	ID                        string     `bson:"id" json:"id"`
	Name                      string     `bson:"name" json:"name"`
	Provider                  string     `bson:"provider" json:"provider"`
	PropagationTimeoutSeconds uint32     `bson:"propagation_timeout_seconds" json:"propagation_timeout_seconds"`
	APIToken                  string     `bson:"api_token,omitempty" json:"-"`
	TokenLastVerifiedAt       *time.Time `bson:"token_last_verified_at,omitempty" json:"token_last_verified_at,omitempty"`
	CreatedAt                 time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt                 time.Time  `bson:"updated_at" json:"updated_at"`
}

func (s *MongoStore) dnsProviderAccounts() *mongo.Collection {
	return s.database.Collection("dns_provider_accounts")
}

// EnsureDNSProviderAccountIndexes creates indexes for the dns_provider_accounts
// collection.
func (s *MongoStore) EnsureDNSProviderAccountIndexes(ctx context.Context) error {
	if _, err := s.dnsProviderAccounts().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_id"),
	}); err != nil {
		return err
	}
	if _, err := s.dnsProviderAccounts().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_name"),
	}); err != nil {
		return err
	}
	if _, err := s.dnsProviderAccounts().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("created_at_desc"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) ListDNSProviderAccounts(ctx context.Context, limit int64, pageToken string) ([]DNSProviderAccount, string, error) {
	return findPage[DNSProviderAccount](ctx, s.dnsProviderAccounts(), bson.M{}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateDNSProviderAccount(ctx context.Context, account DNSProviderAccount) (DNSProviderAccount, error) {
	now := time.Now().UTC()
	if account.ID == "" {
		account.ID = "dns_" + uuid.NewString()
	}
	account.CreatedAt = now
	account.UpdatedAt = now
	if _, err := s.dnsProviderAccounts().InsertOne(ctx, account); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return DNSProviderAccount{}, ErrDNSProviderAccountNameTaken
		}
		return DNSProviderAccount{}, err
	}
	return account, nil
}

func (s *MongoStore) GetDNSProviderAccount(ctx context.Context, id string) (DNSProviderAccount, error) {
	var account DNSProviderAccount
	err := s.dnsProviderAccounts().FindOne(ctx, bson.M{"id": id}).Decode(&account)
	return account, err
}

func (s *MongoStore) UpdateDNSProviderAccount(ctx context.Context, id string, account DNSProviderAccount) (DNSProviderAccount, error) {
	existing, err := s.GetDNSProviderAccount(ctx, id)
	if err != nil {
		return DNSProviderAccount{}, err
	}
	account.ID = id
	account.CreatedAt = existing.CreatedAt
	account.UpdatedAt = time.Now().UTC()
	if _, err := s.dnsProviderAccounts().ReplaceOne(ctx, bson.M{"id": id}, account); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return DNSProviderAccount{}, ErrDNSProviderAccountNameTaken
		}
		return DNSProviderAccount{}, err
	}
	return account, nil
}

func (s *MongoStore) DeleteDNSProviderAccount(ctx context.Context, id string) error {
	res, err := s.dnsProviderAccounts().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
