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
	CATypeLetsEncryptProduction = "lets_encrypt_production"
	CATypeLetsEncryptStaging    = "lets_encrypt_staging"
	CATypeZeroSSL               = "zerossl"
	CATypeGooglePublicCA        = "google_public_ca"
	CATypeCustom                = "custom"

	IssuerRegistrationPending = "Pending"
	IssuerRegistrationReady   = "Ready"
	IssuerRegistrationFailed  = "Failed"
)

// ErrCertificateIssuerNameTaken is returned when an issuer name collides.
var ErrCertificateIssuerNameTaken = errors.New("certificate issuer name already exists")

// ErrInvalidCertificateIssuerCAType is returned for unsupported ca_type values.
var ErrInvalidCertificateIssuerCAType = errors.New("unsupported certificate issuer ca_type")

// ErrCertificateIssuerNotRetryable is returned when registration retry is not allowed.
var ErrCertificateIssuerNotRetryable = errors.New("certificate issuer registration is not in failed state")

// ErrCertificateIssuerImmutable is returned when identity fields are changed on a ready issuer.
var ErrCertificateIssuerImmutable = errors.New("ready certificate issuer identity fields are immutable")

// CertificateIssuer stores a named ACME account. Secret fields must never appear
// in API responses or audit logs.
type CertificateIssuer struct {
	ID                   string     `bson:"id" json:"id"`
	Name                 string     `bson:"name" json:"name"`
	CAType               string     `bson:"ca_type" json:"ca_type"`
	DirectoryURL         string     `bson:"directory_url" json:"directory_url"`
	Email                string     `bson:"email" json:"email"`
	RegistrationStatus   string     `bson:"registration_status" json:"registration_status"`
	RegistrationError    string     `bson:"registration_error,omitempty" json:"registration_error,omitempty"`
	StagingUntrusted     bool       `bson:"staging_untrusted" json:"staging_untrusted"`
	TermsOfServiceURL    string     `bson:"terms_of_service_url" json:"terms_of_service_url"`
	TermsOfServiceAgreed *time.Time `bson:"terms_of_service_agreed_at,omitempty" json:"terms_of_service_agreed_at,omitempty"`
	AccountKeyPEM        string     `bson:"account_key_pem,omitempty" json:"-"`
	EABKid               string     `bson:"eab_kid,omitempty" json:"-"`
	EABHMAC              string     `bson:"eab_hmac,omitempty" json:"-"`
	CreatedAt            time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time  `bson:"updated_at" json:"updated_at"`
}

func (s *MongoStore) certificateIssuers() *mongo.Collection {
	return s.database.Collection("certificate_issuers")
}

// EnsureCertificateIssuerIndexes creates indexes for certificate_issuers.
func (s *MongoStore) EnsureCertificateIssuerIndexes(ctx context.Context) error {
	if _, err := s.certificateIssuers().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_id"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateIssuers().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_name"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateIssuers().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("created_at_desc"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) ListCertificateIssuers(ctx context.Context, limit int64, pageToken string) ([]CertificateIssuer, string, error) {
	return findPage[CertificateIssuer](ctx, s.certificateIssuers(), bson.M{}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateCertificateIssuer(ctx context.Context, issuer CertificateIssuer) (CertificateIssuer, error) {
	now := time.Now().UTC()
	if issuer.ID == "" {
		issuer.ID = "iss_" + uuid.NewString()
	}
	issuer.CreatedAt = now
	issuer.UpdatedAt = now
	if _, err := s.certificateIssuers().InsertOne(ctx, issuer); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return CertificateIssuer{}, ErrCertificateIssuerNameTaken
		}
		return CertificateIssuer{}, err
	}
	return issuer, nil
}

func (s *MongoStore) GetCertificateIssuer(ctx context.Context, id string) (CertificateIssuer, error) {
	var issuer CertificateIssuer
	err := s.certificateIssuers().FindOne(ctx, bson.M{"id": id}).Decode(&issuer)
	return issuer, err
}

func (s *MongoStore) UpdateCertificateIssuer(ctx context.Context, id string, issuer CertificateIssuer) (CertificateIssuer, error) {
	existing, err := s.GetCertificateIssuer(ctx, id)
	if err != nil {
		return CertificateIssuer{}, err
	}
	issuer.ID = id
	issuer.CreatedAt = existing.CreatedAt
	issuer.UpdatedAt = time.Now().UTC()
	if _, err := s.certificateIssuers().ReplaceOne(ctx, bson.M{"id": id}, issuer); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return CertificateIssuer{}, ErrCertificateIssuerNameTaken
		}
		return CertificateIssuer{}, err
	}
	return issuer, nil
}

func (s *MongoStore) DeleteCertificateIssuer(ctx context.Context, id string) error {
	res, err := s.certificateIssuers().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
