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
	CertKeyTypeECP256  = "ec_p256"
	CertKeyTypeRSA2048 = "rsa_2048"

	CertValidityMissing    = "Missing"
	CertValidityValid      = "Valid"
	CertValidityRenewalDue = "RenewalDue"
	CertValidityExpired    = "Expired"
	CertValidityRevoked    = "Revoked"

	DefaultRenewBeforeDays = uint32(30)
	MaxCertificateDomains  = 100
)

// ErrManagedCertificateNameTaken is returned when a certificate name collides.
var ErrManagedCertificateNameTaken = errors.New("managed certificate name already exists")

// ErrCertificateOperationConflict is returned when an operation state transition fails.
var ErrCertificateOperationConflict = errors.New("certificate operation state conflict")

// ErrActiveVersionConflict is returned when activating a version would violate invariants.
var ErrActiveVersionConflict = errors.New("active certificate version conflict")

// ErrVersionNotFound is returned when the requested certificate version does not exist.
var ErrVersionNotFound = errors.New("certificate version not found")

// ErrVersionRevoked is returned when activating a revoked certificate version.
var ErrVersionRevoked = errors.New("certificate version is revoked")

// ErrInvalidServerIDs is returned when server_ids reference missing servers.
var ErrInvalidServerIDs = errors.New("one or more server_ids do not exist")

// ErrIssuerNotReady is returned when the issuer is not Ready for issuance.
var ErrIssuerNotReady = errors.New("certificate issuer is not ready for issuance")

// IssueConfigSnapshot captures issuance parameters at operation creation time.
type IssueConfigSnapshot struct {
	Domains              []string `bson:"domains" json:"domains"`
	CertificateIssuerID  string   `bson:"certificate_issuer_id" json:"certificate_issuer_id"`
	DNSProviderAccountID string   `bson:"dns_provider_account_id" json:"dns_provider_account_id"`
	KeyType              string   `bson:"key_type" json:"key_type"`
}

// CertificateVersion holds an immutable issued bundle and issuance snapshot.
type CertificateVersion struct {
	ID                  string              `bson:"id" json:"id"`
	ConfigSnapshot      IssueConfigSnapshot `bson:"config_snapshot" json:"config_snapshot"`
	FullchainPEM        string              `bson:"fullchain_pem" json:"-"`
	PrivateKeyPEM       string              `bson:"private_key_pem" json:"-"`
	LeafFingerprint     string              `bson:"leaf_fingerprint" json:"leaf_fingerprint"`
	SerialNumber        string              `bson:"serial_number" json:"serial_number"`
	IssuerCommonName    string              `bson:"issuer_common_name" json:"issuer_common_name"`
	NotBefore           time.Time           `bson:"not_before" json:"not_before"`
	NotAfter            time.Time           `bson:"not_after" json:"not_after"`
	KeyType             string              `bson:"key_type" json:"key_type"`
	StagingUntrusted    bool                `bson:"staging_untrusted" json:"staging_untrusted"`
	CertificateIssuerID string              `bson:"certificate_issuer_id" json:"certificate_issuer_id"`
	CreatedAt           time.Time           `bson:"created_at" json:"created_at"`
	RevokedAt           *time.Time          `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
}

// ManagedCertificate is the admin-managed desired config and version container.
type ManagedCertificate struct {
	ID                   string              `bson:"id" json:"id"`
	Name                 string              `bson:"name" json:"name"`
	Domains              []string            `bson:"domains" json:"domains"`
	CertificateIssuerID  string              `bson:"certificate_issuer_id" json:"certificate_issuer_id"`
	DNSProviderAccountID string              `bson:"dns_provider_account_id" json:"dns_provider_account_id"`
	KeyType              string              `bson:"key_type" json:"key_type"`
	AutoRenewEnabled     bool                `bson:"auto_renew_enabled" json:"auto_renew_enabled"`
	RenewBeforeDays      uint32              `bson:"renew_before_days" json:"renew_before_days"`
	NotifyGroupIDs       []string            `bson:"notify_group_ids,omitempty" json:"notify_group_ids,omitempty"`
	ServerIDs            []string            `bson:"server_ids,omitempty" json:"server_ids,omitempty"`
	ActiveVersion        *CertificateVersion `bson:"active_version,omitempty" json:"active_version,omitempty"`
	PreviousVersion      *CertificateVersion `bson:"previous_version,omitempty" json:"previous_version,omitempty"`
	CreatedAt            time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time           `bson:"updated_at" json:"updated_at"`
}

func (s *MongoStore) managedCertificates() *mongo.Collection {
	return s.database.Collection("managed_certificates")
}

// EnsureManagedCertificateIndexes creates indexes for managed_certificates.
func (s *MongoStore) EnsureManagedCertificateIndexes(ctx context.Context) error {
	if _, err := s.managedCertificates().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_id"),
	}); err != nil {
		return err
	}
	if _, err := s.managedCertificates().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_name"),
	}); err != nil {
		return err
	}
	if _, err := s.managedCertificates().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("created_at_desc"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) ListManagedCertificates(ctx context.Context, limit int64, pageToken string) ([]ManagedCertificate, string, error) {
	return findPage[ManagedCertificate](ctx, s.managedCertificates(), bson.M{}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateManagedCertificate(ctx context.Context, cert ManagedCertificate) (ManagedCertificate, error) {
	now := time.Now().UTC()
	if cert.ID == "" {
		cert.ID = "mcert_" + uuid.NewString()
	}
	cert.CreatedAt = now
	cert.UpdatedAt = now
	if _, err := s.managedCertificates().InsertOne(ctx, cert); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ManagedCertificate{}, ErrManagedCertificateNameTaken
		}
		return ManagedCertificate{}, err
	}
	return cert, nil
}

func (s *MongoStore) GetManagedCertificate(ctx context.Context, id string) (ManagedCertificate, error) {
	var cert ManagedCertificate
	err := s.managedCertificates().FindOne(ctx, bson.M{"id": id}).Decode(&cert)
	return cert, err
}

func (s *MongoStore) UpdateManagedCertificate(ctx context.Context, id string, cert ManagedCertificate) (ManagedCertificate, error) {
	existing, err := s.GetManagedCertificate(ctx, id)
	if err != nil {
		return ManagedCertificate{}, err
	}
	cert.ID = id
	cert.CreatedAt = existing.CreatedAt
	cert.UpdatedAt = time.Now().UTC()
	if _, err := s.managedCertificates().ReplaceOne(ctx, bson.M{"id": id}, cert); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ManagedCertificate{}, ErrManagedCertificateNameTaken
		}
		return ManagedCertificate{}, err
	}
	return cert, nil
}

// ActivateFirstIssueVersion atomically sets active_version when none exists and marks the
// Issue operation succeeded. Used for the first successful ACME issue (#18).
func (s *MongoStore) ActivateFirstIssueVersion(ctx context.Context, managedCertID string, version CertificateVersion, opID, warning string) error {
	now := time.Now().UTC()
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id": managedCertID,
		"$or": []bson.M{
			{"active_version": nil},
			{"active_version": bson.M{"$exists": false}},
		},
	}, bson.M{
		"$set": bson.M{
			"active_version": version,
			"updated_at":     now,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrActiveVersionConflict
	}
	opSet := bson.M{
		"status":      CertOpStatusSucceeded,
		"finished_at": now,
		"updated_at":  now,
	}
	if warning != "" {
		opSet["warning"] = warning
	}
	opRes, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":     opID,
		"status": CertOpStatusRunning,
	}, bson.M{"$set": opSet})
	if err != nil {
		return err
	}
	if opRes.MatchedCount == 0 {
		return ErrCertificateOperationConflict
	}
	return nil
}

// ActivateSubsequentIssueVersion atomically moves the current active version to
// previous, sets the new version as active, and drops any older previous PEM.
func (s *MongoStore) ActivateSubsequentIssueVersion(ctx context.Context, managedCertID string, version CertificateVersion, expectedActiveID, opID, warning string) error {
	now := time.Now().UTC()
	cert, err := s.GetManagedCertificate(ctx, managedCertID)
	if err != nil {
		return err
	}
	if cert.ActiveVersion == nil {
		return ErrActiveVersionConflict
	}
	if cert.ActiveVersion.ID != expectedActiveID {
		return ErrActiveVersionConflict
	}
	previous := *cert.ActiveVersion
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id":                managedCertID,
		"active_version.id": expectedActiveID,
	}, bson.M{
		"$set": bson.M{
			"active_version":   version,
			"previous_version": previous,
			"updated_at":       now,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrActiveVersionConflict
	}
	opSet := bson.M{
		"status":      CertOpStatusSucceeded,
		"finished_at": now,
		"updated_at":  now,
	}
	if warning != "" {
		opSet["warning"] = warning
	}
	opRes, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":     opID,
		"status": CertOpStatusRunning,
	}, bson.M{"$set": opSet})
	if err != nil {
		return err
	}
	if opRes.MatchedCount == 0 {
		return ErrCertificateOperationConflict
	}
	return nil
}

// ActivatePreviousVersion atomically swaps active and previous when the target
// version matches previous and is not revoked. Desired config is unchanged.
func (s *MongoStore) ActivatePreviousVersion(ctx context.Context, managedCertID, versionID string) error {
	now := time.Now().UTC()
	cert, err := s.GetManagedCertificate(ctx, managedCertID)
	if err != nil {
		return err
	}
	if cert.PreviousVersion == nil || cert.PreviousVersion.ID != versionID {
		return ErrVersionNotFound
	}
	if cert.PreviousVersion.RevokedAt != nil {
		return ErrVersionRevoked
	}
	newActive := *cert.PreviousVersion
	var newPrevious *CertificateVersion
	if cert.ActiveVersion != nil {
		prev := *cert.ActiveVersion
		newPrevious = &prev
	}
	set := bson.M{
		"active_version": newActive,
		"updated_at":     now,
	}
	if newPrevious != nil {
		set["previous_version"] = newPrevious
	} else {
		set["previous_version"] = nil
	}
	filter := bson.M{
		"id":                          managedCertID,
		"previous_version.id":         versionID,
		"previous_version.revoked_at": bson.M{"$exists": false},
	}
	res, err := s.managedCertificates().UpdateOne(ctx, filter, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if cert.PreviousVersion.RevokedAt != nil {
			return ErrVersionRevoked
		}
		return ErrActiveVersionConflict
	}
	return nil
}

// validateServerIDs ensures every ID exists in servers. An empty slice is allowed.
func (s *MongoStore) validateServerIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	distinct := uniqueStrings(ids)
	for _, id := range distinct {
		if id == "" {
			return ErrInvalidServerIDs
		}
	}
	count, err := s.servers().CountDocuments(ctx, bson.M{"id": bson.M{"$in": distinct}})
	if err != nil {
		return err
	}
	if count != int64(len(distinct)) {
		return ErrInvalidServerIDs
	}
	return nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
