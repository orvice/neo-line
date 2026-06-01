package store

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureResultIndexes creates the index backing uptime aggregation. monitor_results
// is the fastest-growing collection (one document per probe), and GetMonitorUptime
// filters by (server_id, monitor_id) over a started_at window with a started_at sort.
// Without this index every uptime read is a full collection scan.
func (s *MongoStore) EnsureResultIndexes(ctx context.Context) error {
	if _, err := s.results().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "server_id", Value: 1},
			{Key: "monitor_id", Value: 1},
			{Key: "started_at", Value: -1},
		},
		Options: options.Index().SetName("by_server_monitor_started_at"),
	}); err != nil {
		return err
	}
	return nil
}

// uptimeMaxWindow bounds how far back uptime aggregation reads check results.
const uptimeMaxWindow = 24 * time.Hour

// uptimeBeatLimit caps how many recent heartbeats are returned for the bar.
const uptimeBeatLimit = 50

// UptimeWindow summarizes availability over a single rolling time window.
type UptimeWindow struct {
	WindowSeconds int64   `json:"window_seconds"`
	Total         int     `json:"total"`
	Up            int     `json:"up"`
	Down          int     `json:"down"`
	Uptime        float64 `json:"uptime"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
}

// Heartbeat is a compact view of one check, ordered oldest to newest for display.
type Heartbeat struct {
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
}

// MonitorUptime is the Kuma-style availability summary for one monitor: rolling
// uptime windows plus the most recent heartbeats.
type MonitorUptime struct {
	Windows    map[string]UptimeWindow `json:"windows"`
	Heartbeats []Heartbeat             `json:"heartbeats"`
}

// uptimeWindows defines the rolling windows reported for every monitor.
var uptimeWindows = []struct {
	key string
	dur time.Duration
}{
	{"1h", time.Hour},
	{"24h", 24 * time.Hour},
}

// GetMonitorUptime reads recent check results for a monitor and computes its
// availability windows and heartbeat history. monitor_results is the source of
// truth; nothing is denormalized onto the monitor document.
func (s *MongoStore) GetMonitorUptime(ctx context.Context, serverID, monitorID string) (MonitorUptime, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-uptimeMaxWindow)
	filter := bson.M{
		"server_id":  serverID,
		"monitor_id": monitorID,
		"started_at": bson.M{"$gte": cutoff},
	}
	opts := options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}})
	cursor, err := s.results().Find(ctx, filter, opts)
	if err != nil {
		return MonitorUptime{}, err
	}
	defer cursor.Close(ctx)
	var results []CheckResult
	if err := cursor.All(ctx, &results); err != nil {
		return MonitorUptime{}, err
	}
	return computeUptime(results, now, uptimeBeatLimit), nil
}

// computeUptime turns check results (newest first) into uptime windows and the
// most recent heartbeats. A check counts as down only when its status is Down;
// every other status means the target responded and is treated as up. Average
// latency is taken over up checks so timeouts do not skew the value.
func computeUptime(results []CheckResult, now time.Time, beatLimit int) MonitorUptime {
	out := MonitorUptime{Windows: make(map[string]UptimeWindow, len(uptimeWindows))}

	for _, w := range uptimeWindows {
		win := UptimeWindow{WindowSeconds: int64(w.dur / time.Second)}
		cutoff := now.Add(-w.dur)
		var latencySum float64
		for _, r := range results {
			if r.StartedAt.Before(cutoff) {
				continue
			}
			win.Total++
			if normalizeStatus(r.Status) == StatusDown {
				win.Down++
				continue
			}
			win.Up++
			latencySum += float64(r.DurationMS)
		}
		if win.Total > 0 {
			win.Uptime = float64(win.Up) / float64(win.Total)
		}
		if win.Up > 0 {
			win.AvgLatencyMS = latencySum / float64(win.Up)
		}
		out.Windows[w.key] = win
	}

	limit := min(beatLimit, len(results))
	beats := make([]Heartbeat, 0, limit)
	for i := range limit {
		r := results[i]
		beats = append(beats, Heartbeat{
			Status:     normalizeStatus(r.Status),
			StartedAt:  r.StartedAt,
			DurationMS: r.DurationMS,
		})
	}
	// results arrive newest-first; present oldest-first for a left-to-right bar.
	sort.SliceStable(beats, func(i, j int) bool {
		return beats[i].StartedAt.Before(beats[j].StartedAt)
	})
	out.Heartbeats = beats

	return out
}
