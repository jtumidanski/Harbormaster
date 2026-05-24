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

// DB returns the underlying *gorm.DB (used by provider helpers in tests).
func (p *Processor) DB() *gorm.DB { return p.db }

// Retention returns the configured retention duration.
func (p *Processor) Retention() time.Duration { return p.retention }

// newULID returns a new monotonically-sortable ULID as a Crockford base32 string.
func newULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
