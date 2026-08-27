package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	CertOpTypeIssue  = "Issue"
	CertOpTypeRenew  = "Renew"
	CertOpTypeRevoke = "Revoke"

	CertOpStatusPending   = "Pending"
	CertOpStatusRunning   = "Running"
	CertOpStatusSucceeded = "Succeeded"
	CertOpStatusFailed    = "Failed"
)

// CertificateOperation tracks one Issue, Renew, or Revoke business operation.
type CertificateOperation struct {
	ID                   string              `bson:"id" json:"id"`
	ManagedCertificateID string              `bson:"managed_certificate_id" json:"managed_certificate_id"`
	Type                 string              `bson:"type" json:"type"`
	Status               string              `bson:"status" json:"status"`
	AttemptCount         uint32              `bson:"attempt_count" json:"attempt_count"`
	ConfigSnapshot       IssueConfigSnapshot `bson:"config_snapshot" json:"config_snapshot"`
	ErrorSummary         string              `bson:"error_summary,omitempty" json:"error_summary,omitempty"`
	Warning              string              `bson:"warning,omitempty" json:"warning,omitempty"`
	StartedAt            *time.Time          `bson:"started_at,omitempty" json:"started_at,omitempty"`
	FinishedAt           *time.Time          `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
	NextAttemptAt        *time.Time          `bson:"next_attempt_at,omitempty" json:"next_attempt_at,omitempty"`
	CreatedAt            time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time           `bson:"updated_at" json:"updated_at"`
}

func (s *MongoStore) certificateOperations() *mongo.Collection {
	return s.database.Collection("certificate_operations")
}

// EnsureCertificateOperationIndexes creates indexes for certificate_operations.
func (s *MongoStore) EnsureCertificateOperationIndexes(ctx context.Context) error {
	if _, err := s.certificateOperations().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_id"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateOperations().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "managed_certificate_id", Value: 1},
			{Key: "status", Value: 1},
			{Key: "type", Value: 1},
		},
		Options: options.Index().SetName("cert_status_type"),
	}); err != nil {
		return err
	}
	if _, err := s.certificateOperations().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("created_at_desc"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) CreateCertificateOperation(ctx context.Context, op CertificateOperation) (CertificateOperation, error) {
	now := time.Now().UTC()
	if op.ID == "" {
		op.ID = "cop_" + uuid.NewString()
	}
	op.CreatedAt = now
	op.UpdatedAt = now
	if _, err := s.certificateOperations().InsertOne(ctx, op); err != nil {
		return CertificateOperation{}, err
	}
	return op, nil
}

func (s *MongoStore) GetCertificateOperation(ctx context.Context, id string) (CertificateOperation, error) {
	var op CertificateOperation
	err := s.certificateOperations().FindOne(ctx, bson.M{"id": id}).Decode(&op)
	return op, err
}

// CertOpInFlightStatuses are operation statuses that block issue-field edits and
// make duplicate Issue submits idempotent.
var CertOpInFlightStatuses = []string{CertOpStatusPending, CertOpStatusRunning}

func (s *MongoStore) FindRunningCertificateOperation(ctx context.Context, managedCertificateID, opType string) (CertificateOperation, error) {
	var op CertificateOperation
	err := s.certificateOperations().FindOne(ctx, bson.M{
		"managed_certificate_id": managedCertificateID,
		"type":                   opType,
		"status":                 bson.M{"$in": CertOpInFlightStatuses},
	}).Decode(&op)
	return op, err
}

func (s *MongoStore) ListCertificateOperationsByCertificate(ctx context.Context, managedCertificateID string, limit int64) ([]CertificateOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := s.certificateOperations().Find(ctx,
		bson.M{"managed_certificate_id": managedCertificateID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []CertificateOperation
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LatestCertificateOperation returns the most recent operation for a certificate.
func (s *MongoStore) LatestCertificateOperation(ctx context.Context, managedCertificateID string) (CertificateOperation, error) {
	var op CertificateOperation
	err := s.certificateOperations().FindOne(ctx,
		bson.M{"managed_certificate_id": managedCertificateID},
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}}),
	).Decode(&op)
	return op, err
}

// ClaimPendingIssueOperation atomically transitions a Pending Issue operation to Running.
func (s *MongoStore) ClaimPendingIssueOperation(ctx context.Context, opID string) (CertificateOperation, error) {
	now := time.Now().UTC()
	var op CertificateOperation
	err := s.certificateOperations().FindOneAndUpdate(ctx, bson.M{
		"id":     opID,
		"type":   CertOpTypeIssue,
		"status": CertOpStatusPending,
	}, bson.M{
		"$set": bson.M{
			"status":     CertOpStatusRunning,
			"started_at": now,
			"updated_at": now,
		},
		"$inc": bson.M{"attempt_count": 1},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&op)
	if err != nil {
		return CertificateOperation{}, err
	}
	return op, nil
}

// FailIssueOperation marks a Running Issue operation as Failed with a sanitized summary.
func (s *MongoStore) FailIssueOperation(ctx context.Context, opID, errorSummary string) error {
	now := time.Now().UTC()
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":     opID,
		"type":   CertOpTypeIssue,
		"status": CertOpStatusRunning,
	}, bson.M{
		"$set": bson.M{
			"status":        CertOpStatusFailed,
			"error_summary": errorSummary,
			"finished_at":   now,
			"updated_at":    now,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrCertificateOperationConflict
	}
	return nil
}

// FindPendingRenewOperations returns Pending Renew operations oldest first.
func (s *MongoStore) FindPendingRenewOperations(ctx context.Context, limit int64) ([]CertificateOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := s.certificateOperations().Find(ctx, bson.M{
		"type":   CertOpTypeRenew,
		"status": CertOpStatusPending,
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []CertificateOperation
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ClaimPendingRenewOperation atomically transitions a Pending Renew operation to Running.
func (s *MongoStore) ClaimPendingRenewOperation(ctx context.Context, opID string) (CertificateOperation, error) {
	now := time.Now().UTC()
	var op CertificateOperation
	err := s.certificateOperations().FindOneAndUpdate(ctx, bson.M{
		"id":     opID,
		"type":   CertOpTypeRenew,
		"status": CertOpStatusPending,
	}, bson.M{
		"$set": bson.M{
			"status":     CertOpStatusRunning,
			"started_at": now,
			"updated_at": now,
		},
		"$inc": bson.M{"attempt_count": 1},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&op)
	if err != nil {
		return CertificateOperation{}, err
	}
	return op, nil
}

// FailRenewOperation marks a Running Renew operation as Failed with a sanitized summary.
func (s *MongoStore) FailRenewOperation(ctx context.Context, opID, errorSummary string) error {
	now := time.Now().UTC()
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":     opID,
		"type":   CertOpTypeRenew,
		"status": CertOpStatusRunning,
	}, bson.M{
		"$set": bson.M{
			"status":        CertOpStatusFailed,
			"error_summary": errorSummary,
			"finished_at":   now,
			"updated_at":    now,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrCertificateOperationConflict
	}
	return nil
}

// FindPendingIssueOperations returns Pending Issue operations oldest first.
func (s *MongoStore) FindPendingIssueOperations(ctx context.Context, limit int64) ([]CertificateOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := s.certificateOperations().Find(ctx, bson.M{
		"type":   CertOpTypeIssue,
		"status": CertOpStatusPending,
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []CertificateOperation
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateCertificateOperation replaces an operation document by id.
func (s *MongoStore) UpdateCertificateOperation(ctx context.Context, id string, op CertificateOperation) (CertificateOperation, error) {
	op.ID = id
	op.UpdatedAt = time.Now().UTC()
	res, err := s.certificateOperations().ReplaceOne(ctx, bson.M{"id": id}, op)
	if err != nil {
		return CertificateOperation{}, err
	}
	if res.MatchedCount == 0 {
		return CertificateOperation{}, mongo.ErrNoDocuments
	}
	return op, nil
}

// ValidateNotifyGroupIDs ensures notify group references exist.
func (s *MongoStore) ValidateNotifyGroupIDs(ctx context.Context, ids []string) error {
	return s.validateNotifyGroupIDs(ctx, ids)
}

// ValidateServerIDs ensures server references exist.
func (s *MongoStore) ValidateServerIDs(ctx context.Context, ids []string) error {
	return s.validateServerIDs(ctx, ids)
}
