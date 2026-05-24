package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

func TestRetentionSweep_DeletesOldEvents(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour) // 100 days ago — beyond 90-day window
	recent := now.Add(-1 * 24 * time.Hour) // 1 day ago  — within window

	// Insert an old event.
	require.NoError(t, p.Record(ctx, audit.Event{
		OccurredAt: old,
		Actor:      "local-admin",
		Action:     audit.ActionBucketCreate,
		TargetType: "bucket",
		TargetID:   "old-bucket",
		Outcome:    audit.OutcomeSuccess,
	}))

	// Insert a recent event.
	require.NoError(t, p.Record(ctx, audit.Event{
		OccurredAt: recent,
		Actor:      "local-admin",
		Action:     audit.ActionBucketCreate,
		TargetType: "bucket",
		TargetID:   "new-bucket",
		Outcome:    audit.OutcomeSuccess,
	}))

	// Sweep with cutoff = 90 days ago.
	cutoff := now.Add(-90 * 24 * time.Hour)
	deleted, err := p.RetentionSweep(cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "exactly one old event should be deleted")

	// Verify only the recent event remains.
	remaining, err := audit.List(p.DB(), audit.Filter{Action: audit.ActionBucketCreate})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "new-bucket", remaining[0].TargetID)
}

func TestRetentionSweep_NoRowsWhenAllRecent(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	require.NoError(t, p.Record(ctx, audit.Event{
		OccurredAt: time.Now().UTC().Add(-1 * 24 * time.Hour),
		Actor:      "local-admin",
		Action:     audit.ActionSessionLogin,
		TargetType: "session",
		Outcome:    audit.OutcomeSuccess,
	}))

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	deleted, err := p.RetentionSweep(cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}
