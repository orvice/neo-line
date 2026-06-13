package store

import (
	"context"
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

// downStatusVariants enumerates every persisted spelling of a "down" status so
// the aggregation pipeline can count outages without normalizing in Go. It must
// stay in sync with the down case of normalizeStatus.
var downStatusVariants = bson.A{StatusDown, "down", "DOWN", "HEALTH_STATUS_DOWN"}

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

// GetMonitorUptime computes a monitor's availability windows and heartbeat
// history from monitor_results. The windows are aggregated server-side via a
// single $facet pipeline so the whole result history never lands in memory;
// only the most recent uptimeBeatLimit heartbeats are fetched for rendering.
func (s *MongoStore) GetMonitorUptime(ctx context.Context, serverID, monitorID string) (MonitorUptime, error) {
	now := time.Now().UTC()
	windows, err := s.aggregateUptimeWindows(ctx, serverID, monitorID, now)
	if err != nil {
		return MonitorUptime{}, err
	}
	beats, err := s.recentHeartbeats(ctx, serverID, monitorID, now)
	if err != nil {
		return MonitorUptime{}, err
	}
	return MonitorUptime{Windows: windows, Heartbeats: beats}, nil
}

// windowAccumulator captures the raw aggregation output for one window.
type windowAccumulator struct {
	Total      int     `bson:"total"`
	Down       int     `bson:"down"`
	LatencySum float64 `bson:"latency_sum"`
}

// aggregateUptimeWindows runs one $facet pipeline that computes per-window totals,
// outage counts, and up-check latency sums entirely in MongoDB.
func (s *MongoStore) aggregateUptimeWindows(ctx context.Context, serverID, monitorID string, now time.Time) (map[string]UptimeWindow, error) {
	isDown := bson.D{{Key: "$in", Value: bson.A{"$status", downStatusVariants}}}
	facet := bson.D{}
	for _, w := range uptimeWindows {
		facet = append(facet, bson.E{Key: w.key, Value: bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "started_at", Value: bson.D{{Key: "$gte", Value: now.Add(-w.dur)}}}}}},
			bson.D{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: nil},
				{Key: "total", Value: bson.D{{Key: "$sum", Value: 1}}},
				{Key: "down", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{isDown, 1, 0}}}}}},
				{Key: "latency_sum", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{isDown, 0, "$duration_ms"}}}}}},
			}}},
		}})
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "server_id", Value: serverID},
			{Key: "monitor_id", Value: monitorID},
			{Key: "started_at", Value: bson.D{{Key: "$gte", Value: now.Add(-uptimeMaxWindow)}}},
		}}},
		bson.D{{Key: "$facet", Value: facet}},
	}

	cursor, err := s.results().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var facets []map[string][]windowAccumulator
	if err := cursor.All(ctx, &facets); err != nil {
		return nil, err
	}

	out := make(map[string]UptimeWindow, len(uptimeWindows))
	for _, w := range uptimeWindows {
		acc := windowAccumulator{}
		if len(facets) > 0 {
			if branch := facets[0][w.key]; len(branch) > 0 {
				acc = branch[0]
			}
		}
		out[w.key] = buildWindow(int64(w.dur/time.Second), acc)
	}
	return out, nil
}

// buildWindow turns the raw aggregation counts for one window into a populated
// UptimeWindow. A check counts as down only when its status is Down; every other
// status means the target responded and is treated as up. Average latency is
// taken over up checks so timeouts do not skew the value.
func buildWindow(windowSeconds int64, acc windowAccumulator) UptimeWindow {
	win := UptimeWindow{WindowSeconds: windowSeconds, Total: acc.Total, Down: acc.Down}
	win.Up = acc.Total - acc.Down
	if win.Total > 0 {
		win.Uptime = float64(win.Up) / float64(win.Total)
	}
	if win.Up > 0 {
		win.AvgLatencyMS = acc.LatencySum / float64(win.Up)
	}
	return win
}

// recentHeartbeats fetches the most recent checks (capped at uptimeBeatLimit)
// and returns them oldest-first for a left-to-right bar.
func (s *MongoStore) recentHeartbeats(ctx context.Context, serverID, monitorID string, now time.Time) ([]Heartbeat, error) {
	filter := bson.M{
		"server_id":  serverID,
		"monitor_id": monitorID,
		"started_at": bson.M{"$gte": now.Add(-uptimeMaxWindow)},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}}).
		SetLimit(uptimeBeatLimit).
		SetProjection(bson.D{{Key: "status", Value: 1}, {Key: "started_at", Value: 1}, {Key: "duration_ms", Value: 1}})
	cursor, err := s.results().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []CheckResult
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return toHeartbeats(results), nil
}

// toHeartbeats converts newest-first check results into oldest-first heartbeats.
func toHeartbeats(results []CheckResult) []Heartbeat {
	beats := make([]Heartbeat, 0, len(results))
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		beats = append(beats, Heartbeat{
			Status:     normalizeStatus(r.Status),
			StartedAt:  r.StartedAt,
			DurationMS: r.DurationMS,
		})
	}
	return beats
}
