package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	bfmongo "butterfly.orx.me/core/store/mongo"
	bfredis "butterfly.orx.me/core/store/redis"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	StatusHealthy  = "Healthy"
	StatusWarning  = "Warning"
	StatusCritical = "Critical"
	StatusDown     = "Down"
	StatusUnknown  = "Unknown"
)

// Server is a monitored host. MongoDB is the source of truth for these fields.
type Server struct {
	ID                 string    `bson:"id" json:"id"`
	Name               string    `bson:"name" json:"name"`
	Host               string    `bson:"host" json:"host"`
	Environment        string    `bson:"environment,omitempty" json:"environment,omitempty"`
	Region             string    `bson:"region,omitempty" json:"region,omitempty"`
	Tags               []string  `bson:"tags,omitempty" json:"tags,omitempty"`
	SortOrder          uint32    `bson:"sort_order" json:"sort_order"`
	Enabled            bool      `bson:"enabled" json:"enabled"`
	HealthStatus       string    `bson:"health_status" json:"health_status"`
	LastStatusChangeAt time.Time `bson:"last_status_change_at,omitempty" json:"last_status_change_at,omitempty"`
	LastCheckAt        time.Time `bson:"last_check_at,omitempty" json:"last_check_at,omitempty"`
	CreatedAt          time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt          time.Time `bson:"updated_at" json:"updated_at"`
}

// Monitor describes one configured check attached to a server.
type Monitor struct {
	ID                  string            `bson:"id" json:"id"`
	ServerID            string            `bson:"server_id" json:"server_id"`
	GroupIDs            []string          `bson:"group_ids,omitempty" json:"group_ids,omitempty"`
	Name                string            `bson:"name" json:"name"`
	Kind                string            `bson:"kind" json:"kind"`
	Enabled             bool              `bson:"enabled" json:"enabled"`
	Host                string            `bson:"host,omitempty" json:"host,omitempty"`
	Port                uint32            `bson:"port,omitempty" json:"port,omitempty"`
	URL                 string            `bson:"url,omitempty" json:"url,omitempty"`
	Method              string            `bson:"method,omitempty" json:"method,omitempty"`
	Path                string            `bson:"path,omitempty" json:"path,omitempty"`
	Headers             map[string]string `bson:"headers,omitempty" json:"headers,omitempty"`
	ExpectedStatusCodes string            `bson:"expected_status_codes,omitempty" json:"expected_status_codes,omitempty"`
	TLSVerify           bool              `bson:"tls_verify" json:"tls_verify"`
	SNIName             string            `bson:"sni_name,omitempty" json:"sni_name,omitempty"`
	WarningDays         uint32            `bson:"warning_days,omitempty" json:"warning_days,omitempty"`
	CriticalDays        uint32            `bson:"critical_days,omitempty" json:"critical_days,omitempty"`
	IntervalSeconds     uint32            `bson:"interval_seconds" json:"interval_seconds"`
	TimeoutSeconds      uint32            `bson:"timeout_seconds" json:"timeout_seconds"`
	Retries             uint32            `bson:"retries" json:"retries"`
	Status              string            `bson:"status" json:"status"`
	LastCheckAt         time.Time         `bson:"last_check_at,omitempty" json:"last_check_at,omitempty"`
	LastStatusChangeAt  time.Time         `bson:"last_status_change_at,omitempty" json:"last_status_change_at,omitempty"`
	CreatedAt           time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt           time.Time         `bson:"updated_at" json:"updated_at"`
}

type CertificateInfo struct {
	Subject       string    `bson:"subject,omitempty" json:"subject,omitempty"`
	Issuer        string    `bson:"issuer,omitempty" json:"issuer,omitempty"`
	DNSNames      []string  `bson:"dns_names,omitempty" json:"dns_names,omitempty"`
	SerialNumber  string    `bson:"serial_number,omitempty" json:"serial_number,omitempty"`
	NotBefore     time.Time `bson:"not_before,omitempty" json:"not_before,omitempty"`
	NotAfter      time.Time `bson:"not_after,omitempty" json:"not_after,omitempty"`
	DaysRemaining int32     `bson:"days_remaining,omitempty" json:"days_remaining,omitempty"`
}

type CheckResult struct {
	ID             string           `bson:"id" json:"id"`
	ServerID       string           `bson:"server_id" json:"server_id"`
	MonitorID      string           `bson:"monitor_id" json:"monitor_id"`
	Status         string           `bson:"status" json:"status"`
	StartedAt      time.Time        `bson:"started_at" json:"started_at"`
	EndedAt        time.Time        `bson:"ended_at" json:"ended_at"`
	DurationMS     int64            `bson:"duration_ms" json:"duration_ms"`
	ErrorStage     string           `bson:"error_stage,omitempty" json:"error_stage,omitempty"`
	ErrorMessage   string           `bson:"error_message,omitempty" json:"error_message,omitempty"`
	RemoteAddress  string           `bson:"remote_address,omitempty" json:"remote_address,omitempty"`
	Port           uint32           `bson:"port,omitempty" json:"port,omitempty"`
	HTTPStatusCode uint32           `bson:"http_status_code,omitempty" json:"http_status_code,omitempty"`
	Certificate    *CertificateInfo `bson:"certificate,omitempty" json:"certificate,omitempty"`
}

type ServerEvent struct {
	ID             string    `bson:"id" json:"id"`
	ServerID       string    `bson:"server_id" json:"server_id"`
	PreviousStatus string    `bson:"previous_status" json:"previous_status"`
	CurrentStatus  string    `bson:"current_status" json:"current_status"`
	Reason         string    `bson:"reason,omitempty" json:"reason,omitempty"`
	OccurredAt     time.Time `bson:"occurred_at" json:"occurred_at"`
}

type ServerHealth struct {
	ServerID           string    `json:"server_id"`
	Status             string    `json:"status"`
	LastStatusChangeAt time.Time `json:"last_status_change_at,omitempty"`
	LastCheckAt        time.Time `json:"last_check_at,omitempty"`
	TotalMonitors      uint32    `json:"total_monitors"`
	HealthyMonitors    uint32    `json:"healthy_monitors"`
	WarningMonitors    uint32    `json:"warning_monitors"`
	CriticalMonitors   uint32    `json:"critical_monitors"`
	DownMonitors       uint32    `json:"down_monitors"`
	UnknownMonitors    uint32    `json:"unknown_monitors"`
}

// AlertChannel is one delivery target inside a NotifyGroup. Supported Types are
// "webhook", "telegram", "discord", and "mastodon"; the meaning of Target and
// Extra depends on the type.
type AlertChannel struct {
	Type   string            `bson:"type" json:"type"`
	Target string            `bson:"target" json:"target"`
	Extra  map[string]string `bson:"extra,omitempty" json:"extra,omitempty"`
}

// NotifyGroup is a reusable, named bucket of delivery channels. A MonitorGroup's
// AlertPolicy references NotifyGroups by ID; dispatch fans out to every channel
// of every referenced group.
type NotifyGroup struct {
	ID          string         `bson:"id" json:"id"`
	Name        string         `bson:"name" json:"name"`
	Description string         `bson:"description,omitempty" json:"description,omitempty"`
	Channels    []AlertChannel `bson:"channels,omitempty" json:"channels,omitempty"`
	CreatedAt   time.Time      `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `bson:"updated_at" json:"updated_at"`
}

// AlertPolicy holds the group-level alert configuration applied to every
// monitor in the group.
type AlertPolicy struct {
	Enabled            bool     `bson:"enabled" json:"enabled"`
	NotifyGroupIDs     []string `bson:"notify_group_ids,omitempty" json:"notify_group_ids,omitempty"`
	OnDown             bool     `bson:"on_down" json:"on_down"`
	OnRecover          bool     `bson:"on_recover" json:"on_recover"`
	OnWarning          bool     `bson:"on_warning" json:"on_warning"`
	OnCritical         bool     `bson:"on_critical" json:"on_critical"`
	MinIntervalSeconds uint32   `bson:"min_interval_seconds,omitempty" json:"min_interval_seconds,omitempty"`
}

// MonitorGroup is a named, flat bucket of monitors. Monitors may belong to
// multiple groups, and each group carries its own AlertPolicy.
type MonitorGroup struct {
	ID          string      `bson:"id" json:"id"`
	Name        string      `bson:"name" json:"name"`
	Description string      `bson:"description,omitempty" json:"description,omitempty"`
	SortOrder   uint32      `bson:"sort_order" json:"sort_order"`
	AlertPolicy AlertPolicy `bson:"alert_policy" json:"alert_policy"`
	CreatedAt   time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time   `bson:"updated_at" json:"updated_at"`
}

type MongoStore struct {
	client        *mongo.Client
	database      *mongo.Database
	sessionClient *bfredis.Client
}

func New(ctx context.Context, clientKey, database, sessionClientKey string) (*MongoStore, error) {
	if clientKey == "" {
		clientKey = "primary"
	}
	if database == "" {
		database = "neo_line"
	}
	if sessionClientKey == "" {
		sessionClientKey = "session"
	}
	client := bfmongo.GetClient(clientKey)
	if client == nil {
		return nil, fmt.Errorf("mongodb client %q is not configured; set store.mongo.%s.uri in Butterfly configuration", clientKey, clientKey)
	}
	sessionClient := bfredis.GetClient(sessionClientKey)
	if sessionClient == nil {
		return nil, fmt.Errorf("redis session client %q is not configured; set store.redis.%s in Butterfly configuration", sessionClientKey, sessionClientKey)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, err
	}
	if err := sessionClient.Ping(pingCtx).Err(); err != nil {
		return nil, err
	}
	return &MongoStore{client: client, database: client.Database(database), sessionClient: sessionClient}, nil
}

// EnsureServerIndexes creates the indexes used by the servers collection and
// backfills fields introduced after initial deployments.
func (s *MongoStore) EnsureServerIndexes(ctx context.Context) error {
	if _, err := s.servers().UpdateMany(ctx,
		bson.M{"sort_order": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"sort_order": 0}},
	); err != nil {
		return err
	}
	if _, err := s.servers().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "sort_order", Value: 1},
			{Key: "created_at", Value: -1},
		},
		Options: options.Index().SetName("by_sort_order_created_at"),
	}); err != nil {
		return err
	}
	return nil
}

func (s *MongoStore) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var err error
	if s.client != nil {
		err = s.client.Disconnect(ctx)
	}
	if s.sessionClient != nil {
		if redisErr := s.sessionClient.Close(); err == nil {
			err = redisErr
		}
	}
	return err
}

func (s *MongoStore) ListServers(ctx context.Context, environment string, tags []string, limit int64, pageToken string) ([]Server, string, error) {
	filter := bson.M{}
	if environment != "" {
		filter["environment"] = environment
	}
	if len(tags) > 0 {
		filter["tags"] = bson.M{"$all": tags}
	}
	return findPage[Server](ctx, s.servers(), filter, limit, pageToken, bson.D{{Key: "sort_order", Value: 1}, {Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateServer(ctx context.Context, server Server) (Server, error) {
	now := time.Now().UTC()
	if server.ID == "" {
		server.ID = "srv_" + uuid.NewString()
	}
	if server.HealthStatus == "" {
		server.HealthStatus = StatusUnknown
	}
	server.CreatedAt = now
	server.UpdatedAt = now
	server.LastStatusChangeAt = now
	_, err := s.servers().InsertOne(ctx, server)
	return server, err
}

func (s *MongoStore) GetServer(ctx context.Context, id string) (Server, error) {
	var server Server
	err := s.servers().FindOne(ctx, bson.M{"id": id}).Decode(&server)
	return server, err
}

func (s *MongoStore) UpdateServer(ctx context.Context, id string, server Server) (Server, error) {
	existing, err := s.GetServer(ctx, id)
	if err != nil {
		return Server{}, err
	}
	server.ID = id
	server.CreatedAt = existing.CreatedAt
	server.HealthStatus = valueOr(server.HealthStatus, existing.HealthStatus)
	server.LastStatusChangeAt = existing.LastStatusChangeAt
	server.LastCheckAt = existing.LastCheckAt
	server.UpdatedAt = time.Now().UTC()
	_, err = s.servers().ReplaceOne(ctx, bson.M{"id": id}, server)
	return server, err
}

func (s *MongoStore) DeleteServer(ctx context.Context, id string) error {
	res, err := s.servers().DeleteOne(ctx, bson.M{"id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	_, err = s.monitors().DeleteMany(ctx, bson.M{"server_id": id})
	return err
}

func (s *MongoStore) ListMonitors(ctx context.Context, serverID string, limit int64, pageToken string) ([]Monitor, string, error) {
	return findPage[Monitor](ctx, s.monitors(), bson.M{"server_id": serverID}, limit, pageToken, bson.D{{Key: "created_at", Value: -1}})
}

func (s *MongoStore) CreateMonitor(ctx context.Context, serverID string, monitor Monitor) (Monitor, error) {
	if _, err := s.GetServer(ctx, serverID); err != nil {
		return Monitor{}, err
	}
	if err := s.validateGroupIDs(ctx, monitor.GroupIDs); err != nil {
		return Monitor{}, err
	}
	now := time.Now().UTC()
	if monitor.ID == "" {
		monitor.ID = "mon_" + uuid.NewString()
	}
	monitor.ServerID = serverID
	applyMonitorDefaults(&monitor)
	monitor.CreatedAt = now
	monitor.UpdatedAt = now
	monitor.LastStatusChangeAt = now
	_, err := s.monitors().InsertOne(ctx, monitor)
	return monitor, err
}

func (s *MongoStore) GetMonitor(ctx context.Context, serverID, monitorID string) (Monitor, error) {
	var monitor Monitor
	err := s.monitors().FindOne(ctx, bson.M{"server_id": serverID, "id": monitorID}).Decode(&monitor)
	return monitor, err
}

func (s *MongoStore) UpdateMonitor(ctx context.Context, serverID, monitorID string, monitor Monitor) (Monitor, error) {
	existing, err := s.GetMonitor(ctx, serverID, monitorID)
	if err != nil {
		return Monitor{}, err
	}
	if err := s.validateGroupIDs(ctx, monitor.GroupIDs); err != nil {
		return Monitor{}, err
	}
	monitor.ID = monitorID
	monitor.ServerID = serverID
	monitor.CreatedAt = existing.CreatedAt
	monitor.Status = valueOr(monitor.Status, existing.Status)
	monitor.LastCheckAt = existing.LastCheckAt
	monitor.LastStatusChangeAt = existing.LastStatusChangeAt
	monitor.UpdatedAt = time.Now().UTC()
	applyMonitorDefaults(&monitor)
	_, err = s.monitors().ReplaceOne(ctx, bson.M{"server_id": serverID, "id": monitorID}, monitor)
	return monitor, err
}

func (s *MongoStore) DeleteMonitor(ctx context.Context, serverID, monitorID string) error {
	res, err := s.monitors().DeleteOne(ctx, bson.M{"server_id": serverID, "id": monitorID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (s *MongoStore) ListCheckResults(ctx context.Context, serverID, monitorID string, limit int64, pageToken string, start, end *time.Time) ([]CheckResult, string, error) {
	filter := bson.M{"server_id": serverID, "monitor_id": monitorID}
	if start != nil || end != nil {
		timeFilter := bson.M{}
		if start != nil {
			timeFilter["$gte"] = *start
		}
		if end != nil {
			timeFilter["$lte"] = *end
		}
		filter["started_at"] = timeFilter
	}
	return findPage[CheckResult](ctx, s.results(), filter, limit, pageToken, bson.D{{Key: "started_at", Value: -1}})
}

func (s *MongoStore) ListServerEvents(ctx context.Context, serverID string, limit int64, pageToken string) ([]ServerEvent, string, error) {
	return findPage[ServerEvent](ctx, s.events(), bson.M{"server_id": serverID}, limit, pageToken, bson.D{{Key: "occurred_at", Value: -1}})
}

func (s *MongoStore) GetServerHealth(ctx context.Context, serverID string) (ServerHealth, error) {
	server, err := s.GetServer(ctx, serverID)
	if err != nil {
		return ServerHealth{}, err
	}
	cursor, err := s.monitors().Find(ctx, bson.M{"server_id": serverID, "enabled": true})
	if err != nil {
		return ServerHealth{}, err
	}
	defer cursor.Close(ctx)
	health := ServerHealth{ServerID: serverID, Status: StatusUnknown, LastStatusChangeAt: server.LastStatusChangeAt, LastCheckAt: server.LastCheckAt}
	for cursor.Next(ctx) {
		var monitor Monitor
		if err := cursor.Decode(&monitor); err != nil {
			return ServerHealth{}, err
		}
		health.TotalMonitors++
		switch normalizeStatus(monitor.Status) {
		case StatusHealthy:
			health.HealthyMonitors++
		case StatusWarning:
			health.WarningMonitors++
		case StatusCritical:
			health.CriticalMonitors++
		case StatusDown:
			health.DownMonitors++
		default:
			health.UnknownMonitors++
		}
		if monitor.LastCheckAt.After(health.LastCheckAt) {
			health.LastCheckAt = monitor.LastCheckAt
		}
	}
	if err := cursor.Err(); err != nil {
		return ServerHealth{}, err
	}
	health.Status = aggregateHealth(health)
	if health.Status != server.HealthStatus {
		now := time.Now().UTC()
		_, _ = s.servers().UpdateOne(ctx, bson.M{"id": serverID}, bson.M{"$set": bson.M{"health_status": health.Status, "last_status_change_at": now, "last_check_at": health.LastCheckAt, "updated_at": now}})
		_, _ = s.events().InsertOne(ctx, ServerEvent{ID: "evt_" + uuid.NewString(), ServerID: serverID, PreviousStatus: server.HealthStatus, CurrentStatus: health.Status, Reason: "health aggregation", OccurredAt: now})
		health.LastStatusChangeAt = now
	}
	return health, nil
}

func findPage[T any](ctx context.Context, collection *mongo.Collection, filter bson.M, limit int64, pageToken string, sort bson.D) ([]T, string, error) {
	limit = normalizeLimit(limit)
	skip, err := parsePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}
	opts := options.Find().SetLimit(limit + 1).SetSkip(skip).SetSort(sort)
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, "", err
	}
	defer cursor.Close(ctx)
	items := make([]T, 0, limit)
	var scanned int64
	for cursor.Next(ctx) {
		scanned++
		if scanned > limit {
			return items, strconv.FormatInt(skip+limit, 10), nil
		}
		var item T
		if err := cursor.Decode(&item); err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := cursor.Err(); err != nil {
		return nil, "", err
	}
	return items, "", nil
}

func applyMonitorDefaults(monitor *Monitor) {
	if monitor.Kind == "" {
		monitor.Kind = "tcp"
	}
	if monitor.IntervalSeconds == 0 {
		monitor.IntervalSeconds = 60
	}
	if monitor.TimeoutSeconds == 0 {
		monitor.TimeoutSeconds = 5
	}
	if monitor.Retries == 0 {
		monitor.Retries = 3
	}
	if monitor.Status == "" {
		monitor.Status = StatusUnknown
	}
	if monitor.Method == "" && (monitor.Kind == "url" || monitor.Kind == "http" || monitor.Kind == "https") {
		monitor.Method = "GET"
	}
	if strings.TrimSpace(monitor.ExpectedStatusCodes) == "" && (monitor.Kind == "url" || monitor.Kind == "http" || monitor.Kind == "https") {
		monitor.ExpectedStatusCodes = "200"
	}
	if monitor.Kind == "tls_port" || monitor.Kind == "tls_certificate" {
		if monitor.Port == 0 {
			monitor.Port = 443
		}
		if monitor.WarningDays == 0 {
			monitor.WarningDays = 30
		}
		if monitor.CriticalDays == 0 {
			monitor.CriticalDays = 7
		}
	}
}

func aggregateHealth(health ServerHealth) string {
	if health.TotalMonitors == 0 {
		return StatusUnknown
	}
	if health.DownMonitors > 0 {
		return StatusDown
	}
	if health.CriticalMonitors > 0 {
		return StatusCritical
	}
	if health.WarningMonitors > 0 {
		return StatusWarning
	}
	if health.UnknownMonitors > 0 {
		return StatusUnknown
	}
	return StatusHealthy
}

func normalizeStatus(status string) string {
	switch status {
	case StatusHealthy, "healthy", "HEALTHY", "HEALTH_STATUS_HEALTHY":
		return StatusHealthy
	case StatusWarning, "warning", "WARNING", "HEALTH_STATUS_WARNING":
		return StatusWarning
	case StatusCritical, "critical", "CRITICAL", "HEALTH_STATUS_CRITICAL":
		return StatusCritical
	case StatusDown, "down", "DOWN", "HEALTH_STATUS_DOWN":
		return StatusDown
	default:
		return StatusUnknown
	}
}

func normalizeLimit(limit int64) int64 {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func parsePageToken(token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	skip, err := strconv.ParseInt(token, 10, 64)
	if err != nil || skip < 0 {
		return 0, fmt.Errorf("invalid page_token")
	}
	return skip, nil
}

func valueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func IsNotFound(err error) bool {
	return errors.Is(err, mongo.ErrNoDocuments)
}

func (s *MongoStore) servers() *mongo.Collection  { return s.database.Collection("servers") }
func (s *MongoStore) monitors() *mongo.Collection { return s.database.Collection("monitors") }
func (s *MongoStore) results() *mongo.Collection  { return s.database.Collection("monitor_results") }
func (s *MongoStore) events() *mongo.Collection   { return s.database.Collection("server_events") }
func (s *MongoStore) settingsColl() *mongo.Collection {
	return s.database.Collection("settings")
}
