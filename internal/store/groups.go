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

// ErrGroupNameTaken is returned when a MonitorGroup name collides with an
// existing group (enforced by a unique index on monitor_groups.name).
var ErrGroupNameTaken = errors.New("monitor group name already exists")

// ErrInvalidGroupIDs is returned when a monitor references group IDs that do
// not exist in the monitor_groups collection.
var ErrInvalidGroupIDs = errors.New("one or more group_ids do not exist")

// EnsureGroupIndexes creates the indexes used by the monitor_groups collection
// and the group_ids field on monitors.
func (s *MongoStore) EnsureGroupIndexes(ctx context.Context) error {
	if _, err := s.groups().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_name"),
	}); err != nil {
		return err
	}
	if _, err := s.monitors().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "group_ids", Value: 1}},
		Options: options.Index().SetName("by_group_ids"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) ListMonitorGroups(ctx context.Context, limit int64, pageToken string) ([]MonitorGroup, string, error) {
	return findPage[MonitorGroup](ctx, s.groups(), bson.M{}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateMonitorGroup(ctx context.Context, group MonitorGroup) (MonitorGroup, error) {
	now := time.Now().UTC()
	if group.ID == "" {
		group.ID = "grp_" + uuid.NewString()
	}
	group.CreatedAt = now
	group.UpdatedAt = now
	if _, err := s.groups().InsertOne(ctx, group); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return MonitorGroup{}, ErrGroupNameTaken
		}
		return MonitorGroup{}, err
	}
	return group, nil
}

func (s *MongoStore) GetMonitorGroup(ctx context.Context, id string) (MonitorGroup, error) {
	var group MonitorGroup
	err := s.groups().FindOne(ctx, bson.M{"id": id}).Decode(&group)
	return group, err
}

func (s *MongoStore) UpdateMonitorGroup(ctx context.Context, id string, group MonitorGroup) (MonitorGroup, error) {
	existing, err := s.GetMonitorGroup(ctx, id)
	if err != nil {
		return MonitorGroup{}, err
	}
	group.ID = id
	group.CreatedAt = existing.CreatedAt
	group.UpdatedAt = time.Now().UTC()
	if _, err := s.groups().ReplaceOne(ctx, bson.M{"id": id}, group); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return MonitorGroup{}, ErrGroupNameTaken
		}
		return MonitorGroup{}, err
	}
	return group, nil
}

// DeleteMonitorGroup removes the group and pulls its ID from every monitor's
// group_ids array. The pull runs after the group is removed; if the group does
// not exist the call fails with mongo.ErrNoDocuments.
func (s *MongoStore) DeleteMonitorGroup(ctx context.Context, id string) error {
	res, err := s.groups().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	_, err = s.monitors().UpdateMany(ctx,
		bson.M{"group_ids": id},
		bson.M{"$pull": bson.M{"group_ids": id}},
	)
	return err
}

func (s *MongoStore) ListMonitorsByGroup(ctx context.Context, groupID string, limit int64, pageToken string) ([]Monitor, string, error) {
	return findPage[Monitor](ctx, s.monitors(), bson.M{"group_ids": groupID}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) ListGroupsForMonitor(ctx context.Context, monitorID string) ([]MonitorGroup, error) {
	var monitor Monitor
	if err := s.monitors().FindOne(ctx, bson.M{"id": monitorID}).Decode(&monitor); err != nil {
		return nil, err
	}
	if len(monitor.GroupIDs) == 0 {
		return nil, nil
	}
	cursor, err := s.groups().Find(ctx, bson.M{"id": bson.M{"$in": monitor.GroupIDs}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var groups []MonitorGroup
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// validateGroupIDs ensures every ID exists in monitor_groups. An empty slice
// is allowed. Returns ErrInvalidGroupIDs (wrapped) when any ID is missing.
func (s *MongoStore) validateGroupIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("%w: empty id", ErrInvalidGroupIDs)
		}
		unique[id] = struct{}{}
	}
	distinct := make([]string, 0, len(unique))
	for id := range unique {
		distinct = append(distinct, id)
	}
	count, err := s.groups().CountDocuments(ctx, bson.M{"id": bson.M{"$in": distinct}})
	if err != nil {
		return err
	}
	if int(count) != len(distinct) {
		return fmt.Errorf("%w: expected %d, found %d", ErrInvalidGroupIDs, len(distinct), count)
	}
	return nil
}

func (s *MongoStore) groups() *mongo.Collection { return s.database.Collection("monitor_groups") }
