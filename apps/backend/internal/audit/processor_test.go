package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

func TestProcessor_RecordSanitisesPayload(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	evt := audit.Event{
		OccurredAt: time.Now().UTC(),
		Actor:      "local-admin",
		SourceIP:   "10.0.0.1",
		Action:     audit.ActionBucketCreate,
		TargetType: "bucket",
		TargetID:   "test-bucket",
		Outcome:    audit.OutcomeSuccess,
		PayloadSummary: map[string]any{
			"secret_key":    "do-not-persist",
			"password":      "also-secret",
			"versioning":    true,
			"bucket_region": "us-east-1",
		},
	}

	require.NoError(t, p.Record(ctx, evt))

	// Fetch the event back by listing with the action filter; then read the
	// raw JSON to assert the stored content.
	events, err := audit.List(p.DB(), audit.Filter{Action: audit.ActionBucketCreate})
	require.NoError(t, err)
	require.Len(t, events, 1)

	stored := events[0]
	assert.Equal(t, audit.ActionBucketCreate, stored.Action)
	assert.Equal(t, "test-bucket", stored.TargetID)

	// The decoded PayloadSummary must not contain sensitive keys.
	assert.NotContains(t, stored.PayloadSummary, "secret_key")
	assert.NotContains(t, stored.PayloadSummary, "password")

	// Safe keys must be preserved.
	assert.Equal(t, true, stored.PayloadSummary["versioning"])
	assert.Equal(t, "us-east-1", stored.PayloadSummary["bucket_region"])

	// Also assert the raw JSON column excludes sensitive keys.
	raw := loadLatest(t, p, audit.ActionBucketCreate)
	assert.NotContains(t, raw, "secret_key")
	assert.NotContains(t, raw, "password")
	assert.Contains(t, raw, "versioning")
	assert.Contains(t, raw, "bucket_region")
}

func TestProcessor_DefaultsIDAndTimestamp(t *testing.T) {
	t.Parallel()
	p := newTestProcessor(t)
	ctx := context.Background()

	evt := audit.Event{
		Actor:      "local-admin",
		Action:     audit.ActionSessionLogin,
		TargetType: "session",
		Outcome:    audit.OutcomeSuccess,
	}

	require.NoError(t, p.Record(ctx, evt))

	events, err := audit.List(p.DB(), audit.Filter{Action: audit.ActionSessionLogin})
	require.NoError(t, err)
	require.Len(t, events, 1)

	stored := events[0]
	assert.NotEmpty(t, stored.ID, "ID should be auto-assigned")
	assert.False(t, stored.OccurredAt.IsZero(), "OccurredAt should be auto-assigned")
}
