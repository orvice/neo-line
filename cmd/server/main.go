package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"butterfly.orx.me/core/app"
	bfs3 "butterfly.orx.me/core/store/s3"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/alert"
	"github.com/orvice/neo-line/internal/archive"
	"github.com/orvice/neo-line/internal/connectapi"
	"github.com/orvice/neo-line/internal/mcpserver"
	"github.com/orvice/neo-line/internal/scheduler"
	nlssh "github.com/orvice/neo-line/internal/ssh"
	"github.com/orvice/neo-line/internal/store"
)

type runtimeConfig struct {
	Mongo   mongoConfig   `yaml:"mongo"`
	Redis   redisConfig   `yaml:"redis"`
	Archive archiveConfig `yaml:"archive"`
	SSH     sshConfig     `yaml:"ssh"`
}

type sshConfig struct {
	// KeyPath is the local private key used for all SSH connections. Empty
	// disables SSH remote execution APIs and MCP tools.
	KeyPath string `yaml:"key_path"`
	// User is the default SSH user when a server does not override it.
	User string `yaml:"user"`
	// Port is the default SSH port when a server does not override it.
	Port int `yaml:"port"`
	// KnownHostsPath enables host key verification. Required when SSH is
	// enabled unless InsecureSkipHostKey is set.
	KnownHostsPath string `yaml:"known_hosts_path"`
	// InsecureSkipHostKey explicitly disables host key verification. Only for
	// trusted networks or local development.
	InsecureSkipHostKey bool `yaml:"insecure_skip_host_key"`
}

type mongoConfig struct {
	// ClientKey selects the Butterfly Mongo client configured at store.mongo.<key>.
	ClientKey string `yaml:"client_key"`
	// Database is the MongoDB database used by neo-line collections.
	Database string `yaml:"database"`
}

type redisConfig struct {
	// SessionClientKey selects the Butterfly Redis client configured at store.redis.<key>.
	SessionClientKey string `yaml:"session_client_key"`
}

type archiveConfig struct {
	// ClientKey selects the Butterfly S3 client configured at store.s3.<key>.
	// Empty disables check-result archiving.
	ClientKey string `yaml:"client_key"`
	// Prefix is the object-key prefix for archived NDJSON batches.
	Prefix string `yaml:"prefix"`
	// BatchSize is the max number of results per uploaded object.
	BatchSize int `yaml:"batch_size"`
	// FlushIntervalSeconds is the upper bound between flushes.
	FlushIntervalSeconds int `yaml:"flush_interval_seconds"`
}

func (c *runtimeConfig) Print() {}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mongoStore *store.MongoStore
	var sshRunner *nlssh.Runner
	var archiver archive.Archiver = archive.Noop{}
	archiveEnabled := false
	appCfg := &runtimeConfig{}

	schedCtx, cancelSched := context.WithCancel(context.Background())
	archiveDone := make(chan struct{})

	config := &app.Config{
		Service: "neo-line",
		Config:  appCfg,
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "pong"})
			})
			connectapi.Register(r, mongoStore, sshRunner)
			mcpserver.Register(r, mongoStore, sshRunner)
		},
		InitFunc: []func() error{
			func() error {
				var err error
				mongoStore, err = store.New(ctx, appCfg.Mongo.ClientKey, appCfg.Mongo.Database, appCfg.Redis.SessionClientKey)
				if err != nil {
					return fmt.Errorf("connect MongoDB: %w", err)
				}
				if err := mongoStore.EnsureAuthIndexes(ctx); err != nil {
					return fmt.Errorf("ensure auth indexes: %w", err)
				}
				if err := mongoStore.EnsureServerIndexes(ctx); err != nil {
					return fmt.Errorf("ensure server indexes: %w", err)
				}
				if err := mongoStore.EnsureMonitorIndexes(ctx); err != nil {
					return fmt.Errorf("ensure monitor indexes: %w", err)
				}
				if err := mongoStore.EnsureAuditIndexes(ctx); err != nil {
					return fmt.Errorf("ensure audit indexes: %w", err)
				}
				if err := mongoStore.EnsureGroupIndexes(ctx); err != nil {
					return fmt.Errorf("ensure group indexes: %w", err)
				}
				if err := mongoStore.EnsureNotifyGroupIndexes(ctx); err != nil {
					return fmt.Errorf("ensure notify group indexes: %w", err)
				}
				if err := mongoStore.EnsureMcpTokenIndexes(ctx); err != nil {
					return fmt.Errorf("ensure mcp token indexes: %w", err)
				}
				if err := mongoStore.EnsureResultIndexes(ctx); err != nil {
					return fmt.Errorf("ensure result indexes: %w", err)
				}
				if err := bootstrapAdmin(ctx, mongoStore); err != nil {
					return fmt.Errorf("bootstrap admin user: %w", err)
				}

				archiver, archiveEnabled, err = newArchiver(appCfg.Archive)
				if err != nil {
					return fmt.Errorf("init archive: %w", err)
				}
				if archiveEnabled {
					log.Println("S3 check-result archiving enabled")
				}

				sshRunner, err = nlssh.New(nlssh.Config{
					KeyPath:             appCfg.SSH.KeyPath,
					User:                appCfg.SSH.User,
					Port:                appCfg.SSH.Port,
					KnownHostsPath:      appCfg.SSH.KnownHostsPath,
					InsecureSkipHostKey: appCfg.SSH.InsecureSkipHostKey,
				})
				if err != nil {
					return fmt.Errorf("init ssh: %w", err)
				}
				if sshRunner != nil {
					log.Println("SSH remote execution enabled")
				}
				return nil
			},
			func() error {
				go func() {
					archiver.Run(schedCtx)
					close(archiveDone)
				}()
				alerter := alert.New(mongoStore, slog.Default().With("component", "alert"))
				go scheduler.New(mongoStore, archiver).WithAlerter(alerter).Start(schedCtx)
				return nil
			},
		},
		TeardownFunc: []func() error{
			func() error {
				cancelSched()
				// Wait for the archiver to drain and flush any buffered
				// results before the process exits.
				select {
				case <-archiveDone:
				case <-time.After(35 * time.Second):
					log.Println("archive flush timed out on shutdown")
				}
				if mongoStore != nil {
					shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer shutdownCancel()
					if err := mongoStore.Close(shutdownCtx); err != nil {
						log.Printf("close MongoDB: %v", err)
					}
				}
				return nil
			},
		},
	}

	application := app.New(config)
	application.Run()
}

// newArchiver wires the S3 archiver against a Butterfly-managed S3 client.
// When ArchiveConfig.ClientKey is empty or the client/bucket are missing from
// Butterfly configuration, archiving is disabled and a Noop archiver is
// returned.
func newArchiver(cfg archiveConfig) (archive.Archiver, bool, error) {
	if cfg.ClientKey == "" {
		return archive.Noop{}, false, nil
	}
	client := bfs3.GetClient(cfg.ClientKey)
	bucket := bfs3.GetBucket(cfg.ClientKey)
	if client == nil || bucket == "" {
		return nil, false, fmt.Errorf("archive.client_key %q is not configured; set store.s3.%s.bucket in Butterfly configuration", cfg.ClientKey, cfg.ClientKey)
	}
	return archive.New(client, archive.Config{
		Bucket:        bucket,
		Prefix:        cfg.Prefix,
		BatchSize:     cfg.BatchSize,
		FlushInterval: time.Duration(cfg.FlushIntervalSeconds) * time.Second,
	}, slog.Default())
}

// bootstrapAdmin initializes the admin account from the environment.
// ADMIN_PASSWORD is required to (re)set the admin credentials; ADMIN_EMAIL is
// optional and defaults to admin@neo-line.local. When ADMIN_PASSWORD is unset
// the admin account is left untouched.
func bootstrapAdmin(ctx context.Context, st store.Store) error {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		log.Println("ADMIN_PASSWORD not set, skipping admin user initialization")
		return nil
	}
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@neo-line.local"
	}
	if err := st.EnsureAdminUser(ctx, email, password); err != nil {
		return err
	}
	log.Printf("admin user ensured: %s", email)
	return nil
}
