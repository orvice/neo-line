package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FindClaimableCertificateOperations returns operations ready for lease claim,
// oldest first.
func (s *MongoStore) FindClaimableCertificateOperations(ctx context.Context, now time.Time, limit int64) ([]CertificateOperation, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := s.certificateOperations().Find(ctx, claimableOperationFilter(now),
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(limit),
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

// TryClaimCertificateOperation atomically claims a Pending or expired-lease Running
// operation. At most one replica holds a valid lease per operation.
func (s *MongoStore) TryClaimCertificateOperation(ctx context.Context, p CertificateOperationClaimParams) (CertificateOperation, error) {
	target, err := s.GetCertificateOperation(ctx, p.OpID)
	if err != nil {
		return CertificateOperation{}, err
	}
	conflictCount, err := s.certificateOperations().CountDocuments(ctx, bson.M{
		"managed_certificate_id": target.ManagedCertificateID,
		"id":                     bson.M{"$ne": p.OpID},
		"$and":                   []bson.M{runningLeaseHeldFilter(p.Now)},
	})
	if err != nil {
		return CertificateOperation{}, err
	}
	if conflictCount > 0 {
		return CertificateOperation{}, ErrCertificateOperationConflict
	}

	filter := bson.M{
		"id": p.OpID,
		"$and": []bson.M{
			claimableOperationFilter(p.Now),
		},
	}
	pipeline := mongo.Pipeline{
		{{Key: "$set", Value: bson.M{
			"status":           CertOpStatusRunning,
			"lease_owner":      p.Owner,
			"lease_expires_at": p.LeaseExpires,
			"updated_at":       p.Now,
			"finished_at":      nil,
			"started_at":       bson.M{"$ifNull": []any{"$started_at", p.Now}},
			"attempt_count":    bson.M{"$add": []any{"$attempt_count", 1}},
		}}},
	}
	var op CertificateOperation
	err = s.certificateOperations().FindOneAndUpdate(ctx, filter, pipeline,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&op)
	if err != nil {
		return CertificateOperation{}, err
	}
	return op, nil
}

// RenewCertificateOperationLease extends the lease when the caller still owns it.
func (s *MongoStore) RenewCertificateOperationLease(ctx context.Context, opID, owner string, leaseExpires, now time.Time) error {
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":               opID,
		"status":           CertOpStatusRunning,
		"lease_owner":      owner,
		"lease_expires_at": bson.M{"$gt": now},
	}, bson.M{
		"$set": bson.M{
			"lease_expires_at": leaseExpires,
			"updated_at":       now,
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

// UpdateCertificateOperationPendingTXT stores DNS records for takeover cleanup.
func (s *MongoStore) UpdateCertificateOperationPendingTXT(ctx context.Context, opID, owner string, records []DNSChallengeRecord) error {
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":          opID,
		"status":      CertOpStatusRunning,
		"lease_owner": owner,
	}, bson.M{
		"$set": bson.M{
			"pending_txt_records": records,
			"updated_at":          time.Now().UTC(),
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

// ScheduleCertificateOperationRetry moves a failed attempt back to Pending with
// the next automatic retry time while releasing the lease.
func (s *MongoStore) ScheduleCertificateOperationRetry(ctx context.Context, opID, owner string, nextAttemptAt time.Time, errorSummary string, consecutiveFailures uint32) error {
	now := time.Now().UTC()
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":          opID,
		"status":      CertOpStatusRunning,
		"lease_owner": owner,
	}, bson.M{
		"$set": bson.M{
			"status":               CertOpStatusPending,
			"error_summary":        errorSummary,
			"next_attempt_at":      nextAttemptAt,
			"consecutive_failures": consecutiveFailures,
			"lease_owner":          "",
			"lease_expires_at":     nil,
			"updated_at":           now,
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

// MarkCertificateOperationFailed marks a terminal failure; lease must still be held.
func (s *MongoStore) MarkCertificateOperationFailed(ctx context.Context, opID, owner, errorSummary string) error {
	now := time.Now().UTC()
	res, err := s.certificateOperations().UpdateOne(ctx, bson.M{
		"id":          opID,
		"status":      CertOpStatusRunning,
		"lease_owner": owner,
	}, bson.M{
		"$set": bson.M{
			"status":           CertOpStatusFailed,
			"error_summary":    errorSummary,
			"finished_at":      now,
			"lease_owner":      "",
			"lease_expires_at": nil,
			"updated_at":       now,
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

// ClearCertificateOperationPendingTXT removes stored TXT records after cleanup.
func (s *MongoStore) ClearCertificateOperationPendingTXT(ctx context.Context, opID string) error {
	_, err := s.certificateOperations().UpdateOne(ctx, bson.M{"id": opID}, bson.M{
		"$set": bson.M{
			"pending_txt_records": nil,
			"updated_at":          time.Now().UTC(),
		},
	})
	return err
}

// HasRunningCertificateOperation reports whether the certificate has a Pending or
// Running operation (used to block delete).
func (s *MongoStore) HasRunningCertificateOperation(ctx context.Context, managedCertificateID string) (bool, error) {
	count, err := s.certificateOperations().CountDocuments(ctx, bson.M{
		"managed_certificate_id": managedCertificateID,
		"status":                 bson.M{"$in": CertOpInFlightStatuses},
	})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
