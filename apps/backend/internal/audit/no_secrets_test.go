package audit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// TestNoSecretsInPayloadAnyAction verifies that for every defined action
// constant, Processor.Record never persists sensitive keys in
// payload_summary_json, regardless of what the caller supplies.
func TestNoSecretsInPayloadAnyAction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Verify AllActions() returns all 36 constants.
	allActions := audit.AllActions()
	require.Len(t, allActions, 36, "AllActions() should return exactly 36 actions")

	for _, a := range allActions {
		a := a // capture
		t.Run(a, func(t *testing.T) {
			t.Parallel()
			// Use a fresh processor per sub-test to avoid cross-contamination
			// in the loadLatest query (which filters by action).
			localP := newTestProcessor(t)

			err := localP.Record(ctx, audit.Event{
				OccurredAt: time.Now().UTC(),
				Actor:      "local-admin",
				Action:     a,
				TargetType: "bucket",
				TargetID:   "x",
				Outcome:    audit.OutcomeSuccess,
				PayloadSummary: map[string]any{
					"secret_key":    "AAA",
					"password":      "BBB",
					"presigned_url": "CCC",
					"safe_field":    "ok",
				},
			})
			require.NoError(t, err)

			got := loadLatest(t, localP, a)

			lower := strings.ToLower(got)
			require.NotContains(t, lower, "secret_key",
				"action %s leaked secret_key in payload_summary_json", a)
			require.NotContains(t, lower, "password",
				"action %s leaked password in payload_summary_json", a)
			require.NotContains(t, lower, "presigned_url",
				"action %s leaked presigned_url in payload_summary_json", a)
			require.Contains(t, got, "safe_field",
				"action %s dropped safe_field from payload_summary_json", a)
		})
	}
}
