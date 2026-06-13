package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ListEnabledMonitors returns every enabled monitor that belongs to an enabled
// server. Empty monitor hosts inherit the owning server host for runtime probes.
// The scheduler uses this as the authoritative set of work to run.
func (s *MongoStore) ListEnabledMonitors(ctx context.Context) ([]Monitor, error) {
	cursor, err := s.servers().Find(ctx, bson.M{"enabled": true})
	if err != nil {
		return nil, err
	}
	enabledServers := make([]string, 0)
	serverHosts := make(map[string]string)
	{
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var server Server
			if err := cursor.Decode(&server); err != nil {
				return nil, err
			}
			enabledServers = append(enabledServers, server.ID)
			serverHosts[server.ID] = strings.TrimSpace(server.Host)
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
	applyServerHostFallback(monitors, serverHosts)
	return monitors, nil
}

func applyServerHostFallback(monitors []Monitor, serverHosts map[string]string) {
	for i := range monitors {
		if strings.TrimSpace(monitors[i].Host) != "" {
			continue
		}
		monitors[i].Host = serverHosts[monitors[i].ServerID]
	}
}

// SaveCheckResult persists a single probe outcome, updates the owning monitor's
// current status and timestamps, and recomputes the server's aggregated health.
// It returns the monitor's previous status (before this result), enabling the
// caller to drive transition-based behavior such as alert dispatch. The
// returned previous status is empty when no prior status was recorded.
func (s *MongoStore) SaveCheckResult(ctx context.Context, result CheckResult) (string, error) {
	if result.ID == "" {
		result.ID = "res_" + uuid.NewString()
	}
	if _, err := s.results().InsertOne(ctx, result); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	update := bson.M{"status": result.Status, "last_check_at": result.EndedAt, "updated_at": now}
	if result.Certificate != nil {
		update["certificate"] = result.Certificate
	}

	var current Monitor
	var prevStatus string
	monitorFilter := bson.M{"server_id": result.ServerID, "id": result.MonitorID}
	err := s.monitors().FindOne(ctx, monitorFilter).Decode(&current)
	if err == nil {
		prevStatus = current.Status
		if normalizeStatus(current.Status) != normalizeStatus(result.Status) {
			update["last_status_change_at"] = now
		}
	}
	if _, err := s.monitors().UpdateOne(ctx, monitorFilter, bson.M{"$set": update}); err != nil {
		return prevStatus, err
	}

	// Recompute and persist server-level aggregated health.
	if _, err := s.GetServerHealth(ctx, result.ServerID); err != nil {
		return prevStatus, err
	}
	return prevStatus, nil
}
