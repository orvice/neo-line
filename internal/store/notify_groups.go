package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrNotifyGroupNameTaken is returned when a NotifyGroup name collides with an
// existing group (enforced by a unique index on notify_groups.name).
var ErrNotifyGroupNameTaken = errors.New("notify group name already exists")

// ErrInvalidNotifyGroupIDs is returned when an AlertPolicy references notify
// group IDs that do not exist in the notify_groups collection.
var ErrInvalidNotifyGroupIDs = errors.New("one or more notify_group_ids do not exist")

func (s *MongoStore) notifyGroups() *mongo.Collection {
	return s.database.Collection("notify_groups")
}

// EnsureNotifyGroupIndexes creates the indexes used by the notify_groups
// collection.
func (s *MongoStore) EnsureNotifyGroupIndexes(ctx context.Context) error {
	if _, err := s.notifyGroups().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_name"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) ListNotifyGroups(ctx context.Context, limit int64, pageToken string) ([]NotifyGroup, string, error) {
	return findPage[NotifyGroup](ctx, s.notifyGroups(), bson.M{}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateNotifyGroup(ctx context.Context, group NotifyGroup) (NotifyGroup, error) {
	now := time.Now().UTC()
	if group.ID == "" {
		group.ID = "ntf_" + uuid.NewString()
	}
	group.CreatedAt = now
	group.UpdatedAt = now
	if _, err := s.notifyGroups().InsertOne(ctx, group); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return NotifyGroup{}, ErrNotifyGroupNameTaken
		}
		return NotifyGroup{}, err
	}
	return group, nil
}

func (s *MongoStore) GetNotifyGroup(ctx context.Context, id string) (NotifyGroup, error) {
	var group NotifyGroup
	err := s.notifyGroups().FindOne(ctx, bson.M{"id": id}).Decode(&group)
	return group, err
}

func (s *MongoStore) UpdateNotifyGroup(ctx context.Context, id string, group NotifyGroup) (NotifyGroup, error) {
	existing, err := s.GetNotifyGroup(ctx, id)
	if err != nil {
		return NotifyGroup{}, err
	}
	group.ID = id
	group.CreatedAt = existing.CreatedAt
	group.UpdatedAt = time.Now().UTC()
	if _, err := s.notifyGroups().ReplaceOne(ctx, bson.M{"id": id}, group); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return NotifyGroup{}, ErrNotifyGroupNameTaken
		}
		return NotifyGroup{}, err
	}
	return group, nil
}

// DeleteNotifyGroup removes the group and pulls its ID from every monitor
// group's alert_policy.notify_group_ids array.
func (s *MongoStore) DeleteNotifyGroup(ctx context.Context, id string) error {
	res, err := s.notifyGroups().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	_, err = s.groups().UpdateMany(ctx,
		bson.M{"alert_policy.notify_group_ids": id},
		bson.M{"$pull": bson.M{"alert_policy.notify_group_ids": id}},
	)
	return err
}

// validateNotifyGroupIDs ensures every ID exists in notify_groups. An empty
// slice is allowed. Returns ErrInvalidNotifyGroupIDs (wrapped) when any ID is
// missing.
func (s *MongoStore) validateNotifyGroupIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("%w: empty id", ErrInvalidNotifyGroupIDs)
		}
		unique[id] = struct{}{}
	}
	distinct := make([]string, 0, len(unique))
	for id := range unique {
		distinct = append(distinct, id)
	}
	count, err := s.notifyGroups().CountDocuments(ctx, bson.M{"id": bson.M{"$in": distinct}})
	if err != nil {
		return err
	}
	if int(count) != len(distinct) {
		return fmt.Errorf("%w: expected %d, found %d", ErrInvalidNotifyGroupIDs, len(distinct), count)
	}
	return nil
}
