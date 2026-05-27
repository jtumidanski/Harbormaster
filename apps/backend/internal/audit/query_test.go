package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// seedEvent records a single Event with the supplied occurred_at and
// returns the assigned ID so the test can assert ordering.
func seedEvent(t *testing.T, p *audit.Processor, occurredAt time.Time, action, outcome string) string {
	t.Helper()
	e := audit.Event{
		OccurredAt: occurredAt,
		Actor:      "local-admin",
		SourceIP:   "10.0.0.1",
		Action:     action,
		TargetType: "bucket",
		TargetID:   "t",
		Outcome:    outcome,
	}
	require.NoError(t, p.Record(context.Background(), e))

	// Pull back the most recent event for this action+outcome to learn its ID.
	got, _, err := p.List(context.Background(),
		audit.Filter{Action: action, Outcome: outcome},
		audit.Page{Number: 1, Size: 1})
	require.NoError(t, err)
	require.NotEmpty(t, got)
	return got[0].ID
}

func TestRecent_ReturnsNewestNInDescOrder(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	base := time.Now().UTC().Add(-1 * time.Hour)
	// Insert 5 events 10 minutes apart, newest last.
	for i := 0; i < 5; i++ {
		seedEvent(t, p, base.Add(time.Duration(i)*10*time.Minute), audit.ActionBucketCreate, audit.OutcomeSuccess)
	}

	events, err := p.Recent(ctx, 3)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Newest first.
	assert.True(t, events[0].OccurredAt.After(events[1].OccurredAt))
	assert.True(t, events[1].OccurredAt.After(events[2].OccurredAt))
}

func TestRecent_LimitZeroReturnsNil(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	seedEvent(t, p, time.Now().UTC(), audit.ActionBucketCreate, audit.OutcomeSuccess)

	events, err := p.Recent(context.Background(), 0)
	require.NoError(t, err)
	assert.Nil(t, events)
}

func TestFailuresSince_CountsOnlyFailuresInWindow(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// 2 success rows inside the window — must not be counted.
	seedEvent(t, p, now.Add(-1*time.Hour), audit.ActionBucketCreate, audit.OutcomeSuccess)
	seedEvent(t, p, now.Add(-2*time.Hour), audit.ActionBucketDelete, audit.OutcomeSuccess)
	// 3 failure rows inside the window — must be counted.
	seedEvent(t, p, now.Add(-30*time.Minute), audit.ActionObjectUpload, audit.OutcomeFailure)
	seedEvent(t, p, now.Add(-45*time.Minute), audit.ActionObjectUpload, audit.OutcomeFailure)
	seedEvent(t, p, now.Add(-1*time.Hour), audit.ActionObjectDelete, audit.OutcomeFailure)
	// 1 failure row OUTSIDE the window — must not be counted.
	seedEvent(t, p, now.Add(-10*time.Hour), audit.ActionObjectUpload, audit.OutcomeFailure)

	cutoff := now.Add(-3 * time.Hour)
	count, entries, err := p.FailuresSince(ctx, cutoff, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	require.Len(t, entries, 3)
	for _, e := range entries {
		assert.Equal(t, audit.OutcomeFailure, e.Outcome)
		assert.True(t, !e.OccurredAt.Before(cutoff), "entry %s before cutoff", e.ID)
	}
}

func TestFailuresSince_LimitsEntriesButReturnsFullCount(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		seedEvent(t, p, now.Add(-time.Duration(i+1)*time.Minute), audit.ActionObjectUpload, audit.OutcomeFailure)
	}

	count, entries, err := p.FailuresSince(ctx, now.Add(-1*time.Hour), 3)
	require.NoError(t, err)
	assert.Equal(t, int64(7), count)
	assert.Len(t, entries, 3)
}

func TestList_FiltersByEachField(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	seedEvent(t, p, now.Add(-1*time.Minute), audit.ActionBucketCreate, audit.OutcomeSuccess)
	seedEvent(t, p, now.Add(-2*time.Minute), audit.ActionBucketDelete, audit.OutcomeSuccess)
	seedEvent(t, p, now.Add(-3*time.Minute), audit.ActionBucketCreate, audit.OutcomeFailure)
	seedEvent(t, p, now.Add(-4*time.Minute), audit.ActionObjectUpload, audit.OutcomeFailure)

	// Action filter.
	events, total, err := p.List(ctx, audit.Filter{Action: audit.ActionBucketCreate}, audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, events, 2)

	// Outcome filter.
	events, total, err = p.List(ctx, audit.Filter{Outcome: audit.OutcomeFailure}, audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, events, 2)

	// TargetType filter (all rows are bucket, so 4).
	events, total, err = p.List(ctx, audit.Filter{TargetType: "bucket"}, audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, events, 4)

	// Action + Outcome composite filter.
	events, total, err = p.List(ctx,
		audit.Filter{Action: audit.ActionBucketCreate, Outcome: audit.OutcomeFailure},
		audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, events, 1)
}

func TestList_FiltersByTimeRange(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := now.Add(-10 * time.Hour)
	mid := now.Add(-3 * time.Hour)
	recent := now.Add(-30 * time.Minute)

	seedEvent(t, p, old, audit.ActionBucketCreate, audit.OutcomeSuccess)
	seedEvent(t, p, mid, audit.ActionBucketCreate, audit.OutcomeSuccess)
	seedEvent(t, p, recent, audit.ActionBucketCreate, audit.OutcomeSuccess)

	// From cutoff excludes "old".
	events, total, err := p.List(ctx,
		audit.Filter{From: now.Add(-5 * time.Hour)},
		audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, events, 2)

	// To cutoff excludes "recent".
	events, total, err = p.List(ctx,
		audit.Filter{To: now.Add(-1 * time.Hour)},
		audit.Page{Number: 1, Size: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, events, 2)
}

func TestList_Paginates(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		seedEvent(t, p, now.Add(-time.Duration(i)*time.Minute), audit.ActionBucketCreate, audit.OutcomeSuccess)
	}

	// Page 1, size 4 — newest 4.
	page1, total, err := p.List(ctx, audit.Filter{}, audit.Page{Number: 1, Size: 4})
	require.NoError(t, err)
	assert.Equal(t, int64(10), total)
	require.Len(t, page1, 4)

	// Page 2, size 4 — next 4.
	page2, _, err := p.List(ctx, audit.Filter{}, audit.Page{Number: 2, Size: 4})
	require.NoError(t, err)
	require.Len(t, page2, 4)

	// Page 3, size 4 — remaining 2.
	page3, _, err := p.List(ctx, audit.Filter{}, audit.Page{Number: 3, Size: 4})
	require.NoError(t, err)
	require.Len(t, page3, 2)

	// IDs must not repeat across pages.
	seen := map[string]bool{}
	for _, e := range append(append([]audit.Event{}, page1...), append(page2, page3...)...) {
		assert.False(t, seen[e.ID], "id %s repeated across pages", e.ID)
		seen[e.ID] = true
	}
	assert.Len(t, seen, 10)
}

func TestList_NormalisesPageSize(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	seedEvent(t, p, now, audit.ActionBucketCreate, audit.OutcomeSuccess)

	// Number<1, Size<1 → defaults (1, 50).
	events, total, err := p.List(ctx, audit.Filter{}, audit.Page{Number: 0, Size: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, events, 1)
}
