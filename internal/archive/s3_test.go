package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/orvice/neo-line/internal/store"
)

type fakeUploader struct {
	mu   sync.Mutex
	puts [][]byte
	keys []string
}

func (f *fakeUploader) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(in.Body)
	f.puts = append(f.puts, body)
	f.keys = append(f.keys, *in.Key)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeUploader) snapshot() ([][]byte, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	puts := make([][]byte, len(f.puts))
	copy(puts, f.puts)
	keys := make([]string, len(f.keys))
	copy(keys, f.keys)
	return puts, keys
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestEncodeBatchNDJSON(t *testing.T) {
	batch := []store.CheckResult{
		{ID: "res_1", MonitorID: "mon_1", Status: store.StatusHealthy, DurationMS: 40},
		{ID: "res_2", MonitorID: "mon_1", Status: store.StatusDown, DurationMS: 5000},
	}
	body, err := encodeBatch(batch)
	if err != nil {
		t.Fatalf("encodeBatch: %v", err)
	}
	lines := bytes.Split(bytes.TrimRight(body, "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	var first store.CheckResult
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("line 0 not valid JSON: %v", err)
	}
	if first.ID != "res_1" {
		t.Fatalf("first.ID = %q, want res_1", first.ID)
	}
}

func TestObjectKeyLayout(t *testing.T) {
	now := time.Date(2026, 5, 30, 9, 15, 0, 0, time.UTC)
	key := objectKey("/monitor_results/", now, 12)
	if !strings.HasPrefix(key, "monitor_results/2026/05/30/09/") {
		t.Fatalf("key prefix wrong: %q", key)
	}
	if !strings.HasSuffix(key, ".jsonl") {
		t.Fatalf("key suffix wrong: %q", key)
	}
	if !strings.Contains(key, "-12-") {
		t.Fatalf("key missing batch count: %q", key)
	}
}

func TestRunFlushesOnSizeThreshold(t *testing.T) {
	fake := &fakeUploader{}
	a := newWithClient(fake, Config{Bucket: "b", Prefix: "p", BatchSize: 3, FlushInterval: time.Hour}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	for range 3 {
		a.Enqueue(store.CheckResult{ID: "res", MonitorID: "mon_1", Status: store.StatusHealthy})
	}

	deadline := time.After(2 * time.Second)
	for {
		puts, _ := fake.snapshot()
		if len(puts) >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected a size-triggered flush")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}

func TestRunDrainsAndFlushesOnShutdown(t *testing.T) {
	fake := &fakeUploader{}
	// Large batch + long interval: only the shutdown drain should flush.
	a := newWithClient(fake, Config{Bucket: "b", Prefix: "p", BatchSize: 1000, FlushInterval: time.Hour}, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	a.Enqueue(store.CheckResult{ID: "res_1", MonitorID: "mon_1", Status: store.StatusHealthy})
	a.Enqueue(store.CheckResult{ID: "res_2", MonitorID: "mon_1", Status: store.StatusDown})

	// Give Enqueue time to land in the channel before shutdown.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	puts, _ := fake.snapshot()
	if len(puts) != 1 {
		t.Fatalf("got %d uploads, want 1 on shutdown drain", len(puts))
	}
	lines := bytes.Split(bytes.TrimRight(puts[0], "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("shutdown flush wrote %d results, want 2", len(lines))
	}
}

func TestEnqueueDropsWhenBufferFull(t *testing.T) {
	a := newWithClient(&fakeUploader{}, Config{Bucket: "b"}, discardLogger())
	// Do not start Run, so nothing drains the channel.
	for range bufferCapacity + 100 {
		a.Enqueue(store.CheckResult{ID: "res", MonitorID: "mon_1"})
	}
	if len(a.ch) != bufferCapacity {
		t.Fatalf("buffered %d, want capped at %d", len(a.ch), bufferCapacity)
	}
}
