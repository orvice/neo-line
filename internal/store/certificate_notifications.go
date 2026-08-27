package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const CertNotificationFailReminderInterval = 24 * time.Hour

const CertNotificationSevenDayThreshold = 7 * 24 * time.Hour

// CertificateNotificationState tracks persisted throttle and delivery markers for
// certificate lifecycle notifications. Stored on the ManagedCertificate document.
type CertificateNotificationState struct {
	HadOperationFailure       bool       `bson:"had_operation_failure,omitempty" json:"had_operation_failure,omitempty"`
	LastFailNotifiedAt        *time.Time `bson:"last_fail_notified_at,omitempty" json:"last_fail_notified_at,omitempty"`
	SevenDayReminderVersionID string     `bson:"seven_day_reminder_version_id,omitempty" json:"seven_day_reminder_version_id,omitempty"`
	ExpiredNotifiedVersionID  string     `bson:"expired_notified_version_id,omitempty" json:"expired_notified_version_id,omitempty"`
	LastNotificationWarning   string     `bson:"last_notification_warning,omitempty" json:"last_notification_warning,omitempty"`
	LastNotificationWarningAt *time.Time `bson:"last_notification_warning_at,omitempty" json:"last_notification_warning_at,omitempty"`
}

// ListManagedCertificatesForNotifications returns certificates that reference at
// least one NotifyGroup and have an active version (validity scan candidates).
func (s *MongoStore) ListManagedCertificatesForNotifications(ctx context.Context) ([]ManagedCertificate, error) {
	cursor, err := s.managedCertificates().Find(ctx, bson.M{
		"notify_group_ids.0": bson.M{"$exists": true},
		"active_version":     bson.M{"$exists": true, "$ne": nil},
	}, nil)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	certs := make([]ManagedCertificate, 0)
	if err := cursor.All(ctx, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

// TryRecordOperationFailureNotification claims the first failure notification
// slot for a certificate (immediate on episode start).
func (s *MongoStore) TryRecordOperationFailureNotification(ctx context.Context, certID string, now time.Time) (bool, error) {
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id": certID,
		"$or": []bson.M{
			{"notification_state.had_operation_failure": bson.M{"$ne": true}},
			{"notification_state": bson.M{"$exists": false}},
			{"notification_state.had_operation_failure": bson.M{"$exists": false}},
		},
	}, bson.M{
		"$set": bson.M{
			"notification_state.had_operation_failure": true,
			"notification_state.last_fail_notified_at": now,
			"updated_at": now,
		},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// TryRecordOperationFailureReminder claims a sustained-failure reminder when at
// least CertNotificationFailReminderInterval has elapsed since the last fail notify.
func (s *MongoStore) TryRecordOperationFailureReminder(ctx context.Context, certID string, now time.Time) (bool, error) {
	cutoff := now.Add(-CertNotificationFailReminderInterval)
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id": certID,
		"notification_state.had_operation_failure": true,
		"notification_state.last_fail_notified_at": bson.M{"$lte": cutoff},
	}, bson.M{
		"$set": bson.M{
			"notification_state.last_fail_notified_at": now,
			"updated_at": now,
		},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// TryRecordOperationRecovery clears failure episode markers after a successful
// Issue/Renew when a prior failure existed.
func (s *MongoStore) TryRecordOperationRecovery(ctx context.Context, certID string, now time.Time) (bool, error) {
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id": certID,
		"notification_state.had_operation_failure": true,
	}, bson.M{
		"$set": bson.M{
			"notification_state.had_operation_failure": false,
			"notification_state.last_fail_notified_at": nil,
			"updated_at": now,
		},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// TryRecordSevenDayReminder marks the seven-day remaining reminder as sent for
// the given active version.
func (s *MongoStore) TryRecordSevenDayReminder(ctx context.Context, certID, versionID string, now time.Time) (bool, error) {
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id":                certID,
		"active_version.id": versionID,
		"$or": []bson.M{
			{"notification_state.seven_day_reminder_version_id": bson.M{"$exists": false}},
			{"notification_state.seven_day_reminder_version_id": bson.M{"$ne": versionID}},
			{"notification_state": bson.M{"$exists": false}},
		},
	}, bson.M{
		"$set": bson.M{
			"notification_state.seven_day_reminder_version_id": versionID,
			"updated_at": now,
		},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// TryRecordExpiredNotification marks the expired notification as sent for the
// given active version.
func (s *MongoStore) TryRecordExpiredNotification(ctx context.Context, certID, versionID string, now time.Time) (bool, error) {
	res, err := s.managedCertificates().UpdateOne(ctx, bson.M{
		"id":                certID,
		"active_version.id": versionID,
		"$or": []bson.M{
			{"notification_state.expired_notified_version_id": bson.M{"$exists": false}},
			{"notification_state.expired_notified_version_id": bson.M{"$ne": versionID}},
			{"notification_state": bson.M{"$exists": false}},
		},
	}, bson.M{
		"$set": bson.M{
			"notification_state.expired_notified_version_id": versionID,
			"updated_at": now,
		},
	})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// SetCertificateNotificationWarning records the most recent channel delivery
// warning on the managed certificate without changing operations or versions.
func (s *MongoStore) SetCertificateNotificationWarning(ctx context.Context, certID, warning string, at time.Time) error {
	_, err := s.managedCertificates().UpdateOne(ctx, bson.M{"id": certID}, bson.M{
		"$set": bson.M{
			"notification_state.last_notification_warning":    warning,
			"notification_state.last_notification_warning_at": at,
			"updated_at": at,
		},
	})
	return err
}

// ClearCertificateNotificationState removes notification markers when a managed
// certificate is deleted locally.
func (s *MongoStore) ClearCertificateNotificationState(ctx context.Context, certID string) error {
	_, err := s.managedCertificates().UpdateOne(ctx, bson.M{"id": certID}, bson.M{
		"$unset": bson.M{"notification_state": ""},
	})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	return nil
}
