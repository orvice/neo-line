// Package archive provides an optional, best-effort secondary sink for check
// results. MongoDB remains the source of truth; the archiver writes an
// append-only copy of every persisted result to durable object storage (S3 or
// any S3-compatible endpoint) for long-term history.
//
// Archiving never blocks or fails the primary MongoDB write: results are
// buffered and flushed in batches, and any error is logged and retried on the
// next flush rather than propagated to the caller.
package archive

import (
	"context"

	"github.com/orvice/neo-line/internal/store"
)

// Archiver receives persisted check results and durably stores them out of band.
type Archiver interface {
	// Enqueue hands a result to the archiver. It must be non-blocking and safe
	// for concurrent use; implementations may drop results under sustained
	// backpressure rather than stall the caller.
	Enqueue(result store.CheckResult)
	// Run drives any background flushing until ctx is cancelled, then flushes
	// whatever remains. Callers typically invoke it in a goroutine.
	Run(ctx context.Context)
}

// Noop is the archiver used when no archive backend is configured. Enqueue
// discards results and Run simply blocks until the context is cancelled so
// callers can treat the lifecycle uniformly.
type Noop struct{}

func (Noop) Enqueue(store.CheckResult) {}

func (Noop) Run(ctx context.Context) { <-ctx.Done() }
