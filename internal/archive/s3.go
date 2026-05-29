package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/orvice/neo-line/internal/store"
)

const (
	defaultPrefix        = "monitor_results"
	defaultBatchSize     = 100
	defaultFlushInterval = 60 * time.Second
	// bufferCapacity bounds in-memory results awaiting flush. When the buffer is
	// full (e.g. S3 is unreachable) the oldest enqueued results are dropped.
	bufferCapacity = 10000
	// flushTimeout caps a single upload, including the final flush at shutdown.
	flushTimeout = 30 * time.Second
)

// Config holds the resolved S3 archive settings.
type Config struct {
	Bucket        string
	Prefix        string
	Region        string
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UsePathStyle  bool
	BatchSize     int
	FlushInterval time.Duration
}

// configFromEnv reads archive settings from the environment. The second return
// value is false when archiving is disabled (no bucket configured).
func configFromEnv() (Config, bool) {
	bucket := os.Getenv("ARCHIVE_S3_BUCKET")
	if bucket == "" {
		return Config{}, false
	}
	cfg := Config{
		Bucket:        bucket,
		Prefix:        envOr("ARCHIVE_S3_PREFIX", defaultPrefix),
		Region:        firstNonEmpty(os.Getenv("ARCHIVE_S3_REGION"), os.Getenv("AWS_REGION"), "us-east-1"),
		Endpoint:      os.Getenv("ARCHIVE_S3_ENDPOINT"),
		AccessKey:     os.Getenv("ARCHIVE_S3_ACCESS_KEY"),
		SecretKey:     os.Getenv("ARCHIVE_S3_SECRET_KEY"),
		UsePathStyle:  os.Getenv("ARCHIVE_S3_ENDPOINT") != "",
		BatchSize:     envInt("ARCHIVE_S3_BATCH_SIZE", defaultBatchSize),
		FlushInterval: time.Duration(envInt("ARCHIVE_S3_FLUSH_SECONDS", int(defaultFlushInterval/time.Second))) * time.Second,
	}
	return cfg, true
}

// uploader is the subset of the S3 client the archiver needs, narrowed so the
// flush logic can be unit tested with a fake.
type uploader interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Archiver buffers check results and flushes them to S3 as NDJSON batches.
type S3Archiver struct {
	client uploader
	cfg    Config
	logger *slog.Logger
	ch     chan store.CheckResult
}

// New builds an Archiver from the environment. When ARCHIVE_S3_BUCKET is unset
// it returns a Noop archiver and enabled=false. Otherwise it initializes an S3
// client (honoring a custom endpoint and static credentials for S3-compatible
// stores such as MinIO) and returns an *S3Archiver.
func New(ctx context.Context, logger *slog.Logger) (Archiver, bool, error) {
	cfg, ok := configFromEnv()
	if !ok {
		return Noop{}, false, nil
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, false, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	return newWithClient(client, cfg, logger), true, nil
}

func newWithClient(client uploader, cfg Config, logger *slog.Logger) *S3Archiver {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultFlushInterval
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &S3Archiver{
		client: client,
		cfg:    cfg,
		logger: logger.With("component", "archive"),
		ch:     make(chan store.CheckResult, bufferCapacity),
	}
}

// Enqueue hands a result to the flush loop. It never blocks: if the buffer is
// full the result is dropped and a warning is logged so the primary write path
// is never stalled by a slow or unavailable S3 backend.
func (a *S3Archiver) Enqueue(result store.CheckResult) {
	select {
	case a.ch <- result:
	default:
		a.logger.Warn("archive buffer full, dropping result", "monitor_id", result.MonitorID)
	}
}

// Run batches enqueued results and flushes them to S3 by size or interval,
// whichever comes first. On shutdown it drains the buffer and performs a final
// flush so buffered results are not lost.
func (a *S3Archiver) Run(ctx context.Context) {
	a.logger.Info("archive started",
		"bucket", a.cfg.Bucket,
		"prefix", a.cfg.Prefix,
		"batch_size", a.cfg.BatchSize,
		"flush_interval", a.cfg.FlushInterval.String(),
	)
	ticker := time.NewTicker(a.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]store.CheckResult, 0, a.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		uploadCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		if err := a.flush(uploadCtx, batch); err != nil {
			a.logger.Error("archive flush failed", "count", len(batch), "error", err.Error())
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case r := <-a.ch:
					batch = append(batch, r)
					if len(batch) >= a.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					a.logger.Info("archive stopped")
					return
				}
			}
		case r := <-a.ch:
			batch = append(batch, r)
			if len(batch) >= a.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// flush uploads one NDJSON object containing the batch.
func (a *S3Archiver) flush(ctx context.Context, batch []store.CheckResult) error {
	body, err := encodeBatch(batch)
	if err != nil {
		return err
	}
	key := objectKey(a.cfg.Prefix, time.Now().UTC(), len(batch))
	_, err = a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/x-ndjson"),
	})
	if err != nil {
		return err
	}
	a.logger.Debug("archive flushed", "key", key, "count", len(batch), "bytes", len(body))
	return nil
}

// encodeBatch serializes results as newline-delimited JSON (one result per line).
func encodeBatch(batch []store.CheckResult) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range batch {
		if err := enc.Encode(r); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// objectKey builds a time-partitioned, collision-resistant key for one batch.
func objectKey(prefix string, now time.Time, count int) string {
	prefix = strings.Trim(prefix, "/")
	return fmt.Sprintf("%s/%s/%d-%d-%s.jsonl",
		prefix,
		now.Format("2006/01/02/15"),
		now.UnixMilli(),
		count,
		uuid.NewString()[:8],
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
