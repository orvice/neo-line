package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListEnabledMonitors returns every enabled monitor that belongs to an enabled
// server. The scheduler uses this as the authoritative set of work to run.
func (s *Store) ListEnabledMonitors(ctx context.Context) ([]Monitor, error) {
	cursor, err := s.servers().Find(ctx, bson.M{"enabled": true})
	if err != nil {
		return nil, err
	}
	enabledServers := make([]string, 0)
	{
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var server Server
			if err := cursor.Decode(&server); err != nil {
				return nil, err
			}
			enabledServers = append(enabledServers, server.ID)
		}
		if err := cursor.Err(); err != nil {
			return nil, err
		}
	}
	if len(enabledServers) == 0 {
		return nil, nil
	}

	monCursor, err := s.monitors().Find(ctx, bson.M{"enabled": true, "server_id": bson.M{"$in": enabledServers}})
	if err != nil {
		return nil, err
	}
	defer monCursor.Close(ctx)
	var monitors []Monitor
	if err := monCursor.All(ctx, &monitors); err != nil {
		return nil, err
	}
	return monitors, nil
}

// SaveCheckResult persists a single probe outcome, updates the owning monitor's
// current status and timestamps, and recomputes the server's aggregated health.
func (s *Store) SaveCheckResult(ctx context.Context, result CheckResult) error {
	if result.ID == "" {
		result.ID = "res_" + uuid.NewString()
	}
	if _, err := s.results().InsertOne(ctx, result); err != nil {
		return err
	}

	now := time.Now().UTC()
	update := bson.M{"status": result.Status, "last_check_at": result.EndedAt, "updated_at": now}

	var current Monitor
	err := s.monitors().FindOne(ctx, bson.M{"id": result.MonitorID}).Decode(&current)
	if err == nil && normalizeStatus(current.Status) != normalizeStatus(result.Status) {
		update["last_status_change_at"] = now
	}
	if _, err := s.monitors().UpdateOne(ctx, bson.M{"id": result.MonitorID}, bson.M{"$set": update}); err != nil {
		return err
	}

	// Recompute and persist server-level aggregated health.
	_, err = s.GetServerHealth(ctx, result.ServerID)
	return err
}
