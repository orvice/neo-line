package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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

// Config holds the resolved S3 archive settings. The S3 client itself is
// supplied separately by the caller (typically from Butterfly's store/s3
// integration).
type Config struct {
	Bucket        string
	Prefix        string
	BatchSize     int
	FlushInterval time.Duration
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

// New builds an Archiver around the given S3 client and configuration. When
// either the client or the bucket is empty the archiver is disabled: it
// returns a Noop archiver and enabled=false. Callers should resolve the
// client and bucket from Butterfly's store/s3 integration:
//
//	client := bfs3.GetClient(key)
//	bucket := bfs3.GetBucket(key)
func New(client *s3.Client, cfg Config, logger *slog.Logger) (Archiver, bool, error) {
	if client == nil || cfg.Bucket == "" {
		return Noop{}, false, nil
	}
	return newWithClient(client, cfg, logger), true, nil
}

func newWithClient(client uploader, cfg Config, logger *slog.Logger) *S3Archiver {
	if cfg.Prefix == "" {
		cfg.Prefix = defaultPrefix
	}
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
