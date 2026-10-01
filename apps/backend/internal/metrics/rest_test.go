package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestToResponse_EmitsLegacySeriesKeys verifies that every objectstore_*
// series is also emitted under its pre-rename key with identical points, so
// browser bundles cached before the rename keep rendering.
func TestToResponse_EmitsLegacySeriesKeys(t *testing.T) {
	ts := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	v := View{
		Window:      Window1h,
		StepSeconds: 60,
		Collected:   true,
		Series: map[string][]Point{
			"objectstore_s3_requests_total": {{T: ts, V: 1.5}, {T: ts.Add(time.Minute), V: 2.5}},
		},
	}

	resp := toResponse(v)

	require.Len(t, resp.Series, 2)
	current, ok := resp.Series["objectstore_s3_requests_total"]
	require.True(t, ok, "current key missing")
	legacy, ok := resp.Series["minio_s3_requests_total"]
	require.True(t, ok, "legacy key missing")
	require.Equal(t, current, legacy)
	require.Equal(t, []pointWire{
		{T: "2026-10-01T12:00:00Z", V: 1.5},
		{T: "2026-10-01T12:01:00Z", V: 2.5},
	}, current)
}
