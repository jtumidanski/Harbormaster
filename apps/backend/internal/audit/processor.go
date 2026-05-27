package audit

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// Processor coordinates event recording and retention operations.
// Construct via NewProcessor; do not create the zero value directly.
type Processor struct {
	db        *gorm.DB
	retention time.Duration
}

// NewProcessor returns a Processor backed by db.
// retention is the age beyond which events are eligible for deletion.
func NewProcessor(db *gorm.DB, retention time.Duration) *Processor {
	return &Processor{db: db, retention: retention}
}

// Record sanitises the event payload and persists the event.
// If e.ID is empty a fresh ULID is assigned. If e.OccurredAt is zero it
// defaults to time.Now().UTC(). Sanitize is called unconditionally — the
// stored payload can never contain sensitive keys regardless of input.
func (p *Processor) Record(ctx context.Context, e Event) error {
	if e.ID == "" {
		e.ID = newULID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	// Sanitize is always called, even when PayloadSummary is nil.
	e.PayloadSummary = Sanitize(e.PayloadSummary)

	if err := insert(p.db.WithContext(ctx), e); err != nil {
		return fmt.Errorf("processor.Record: %w", err)
	}
	return nil
}

// RetentionSweep deletes events older than cutoff and returns the count removed.
func (p *Processor) RetentionSweep(cutoff time.Time) (int64, error) {
	return deleteOlderThan(p.db, cutoff)
}

// Recent returns the newest limit events ordered occurred_at DESC. A
// non-positive limit yields a nil slice without a query.
func (p *Processor) Recent(ctx context.Context, limit int) ([]Event, error) {
	return recent(p.db.WithContext(ctx), limit)
}

// FailuresSince returns the count of failure-outcome events since cutoff
// plus the most-recent limit entries. The count is the unfiltered total
// inside the window so callers can render "N failures since X" alongside
// a truncated entries[] slice.
func (p *Processor) FailuresSince(ctx context.Context, cutoff time.Time, limit int) (int64, []Event, error) {
	return failuresSince(p.db.WithContext(ctx), cutoff, limit)
}

// List returns events matching f, paginated per page, in occurred_at DESC
// order. The second return value is the unfiltered-by-page total so the
// caller can populate page meta (total_pages / total_records). Page
// number<1 / size<1 are normalised to (1, 50); sizes above 200 are capped.
func (p *Processor) List(ctx context.Context, f Filter, page Page) ([]Event, int64, error) {
	return listFiltered(p.db.WithContext(ctx), f, page)
}

// DB returns the underlying *gorm.DB (used by provider helpers in tests).
func (p *Processor) DB() *gorm.DB { return p.db }

// Retention returns the configured retention duration.
func (p *Processor) Retention() time.Duration { return p.retention }

// newULID returns a new monotonically-sortable ULID as a Crockford base32 string.
func newULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
