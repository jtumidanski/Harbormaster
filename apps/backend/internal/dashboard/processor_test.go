package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/dashboard"
)

func TestParseWindowValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want dashboard.Window
	}{
		{"", dashboard.Window7d}, // empty defaults to 7d
		{"24h", dashboard.Window24h},
		{"7d", dashboard.Window7d},
		{"30d", dashboard.Window30d},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := dashboard.Parse(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseWindowInvalid(t *testing.T) {
	t.Parallel()
	_, err := dashboard.Parse("foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, dashboard.ErrInvalidWindow)
}

func TestWindowDuration(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 24*time.Hour, dashboard.Window24h.Duration())
	assert.Equal(t, 7*24*time.Hour, dashboard.Window7d.Duration())
	assert.Equal(t, 30*24*time.Hour, dashboard.Window30d.Duration())
}

func TestBuild_TotalsAggregation(t *testing.T) {
	t.Parallel()
	// 12 buckets with varying sizes; assert View.Totals matches the sum.
	bks := make([]buckets.Bucket, 12)
	var wantBytes, wantObjects int64
	for i := 0; i < 12; i++ {
		bks[i] = buckets.Bucket{
			Name:           "b" + itoa(i),
			EstimatedBytes: int64((i + 1) * 1024 * 1024),
			ObjectCount:    int64((i + 1) * 100),
		}
		wantBytes += bks[i].EstimatedBytes
		wantObjects += bks[i].ObjectCount
	}

	auditP := newTestAuditProcessor(t)
	p := dashboard.NewProcessor(
		stubPool{
			info: dashboard.ServerInfo{
				Version:        "RELEASE.2026-04-30T12-00-00Z",
				DeploymentMode: "single-node-single-drive",
				UptimeSeconds:  84321,
			},
			nodes: []dashboard.NodeStatus{
				{Endpoint: "minio.lan:9000", State: "online", Drives: dashboard.DriveCount{Total: 1, Healthy: 1}},
			},
		},
		stubBuckets{out: bks},
		auditP,
	)

	view, err := p.Build(context.Background(), dashboard.Window7d)
	require.NoError(t, err)
	assert.Equal(t, int64(12), view.Totals.Buckets)
	assert.Equal(t, wantBytes, view.Totals.EstimatedBytes)
	assert.Equal(t, wantObjects, view.Totals.Objects)
	assert.Equal(t, "single-node-single-drive", view.Server.DeploymentMode)
	assert.Equal(t, int64(84321), view.Server.UptimeSeconds)
	require.Len(t, view.Nodes, 1)
	assert.Equal(t, "minio.lan:9000", view.Nodes[0].Endpoint)
	// No audit rows seeded: recent_activity and failures.entries are empty,
	// not nil — the SPA expects [] for these.
	assert.Equal(t, []dashboard.EventSummary{}, view.RecentActivity)
	assert.Equal(t, []dashboard.FailureSummary{}, view.RecentFailures.Entries)
	assert.Equal(t, dashboard.Window7d, view.RecentFailures.Window)
	assert.Equal(t, int64(0), view.RecentFailures.Count)
}

func TestBuild_RecentActivityAndFailuresPopulated(t *testing.T) {
	t.Parallel()
	auditP := newTestAuditProcessor(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// 3 successes inside the window (recent_activity will include all 5 rows).
	mustRecord(t, auditP, audit.Event{
		OccurredAt: now.Add(-10 * time.Minute),
		Actor:      "local-admin",
		Action:     audit.ActionBucketCreate,
		TargetType: "bucket",
		TargetID:   "photos",
		Outcome:    audit.OutcomeSuccess,
	})
	mustRecord(t, auditP, audit.Event{
		OccurredAt: now.Add(-5 * time.Minute),
		Actor:      "local-admin",
		Action:     audit.ActionBucketDelete,
		TargetType: "bucket",
		TargetID:   "old",
		Outcome:    audit.OutcomeSuccess,
	})
	// 2 failures inside the window.
	mustRecord(t, auditP, audit.Event{
		OccurredAt:   now.Add(-1 * time.Minute),
		Actor:        "local-admin",
		SourceIP:     "10.0.1.5",
		Action:       audit.ActionObjectUpload,
		TargetType:   "object",
		TargetID:     "backups/x.tar",
		Outcome:      audit.OutcomeFailure,
		ErrorMessage: "MinIO: NoSuchBucket",
	})
	mustRecord(t, auditP, audit.Event{
		OccurredAt: now.Add(-2 * time.Minute),
		Actor:      "local-admin",
		Action:     audit.ActionObjectDelete,
		TargetType: "object",
		TargetID:   "old/y.tar",
		Outcome:    audit.OutcomeFailure,
	})
	// 1 failure OUTSIDE the 24h window — must not appear in failures widget.
	mustRecord(t, auditP, audit.Event{
		OccurredAt: now.Add(-48 * time.Hour),
		Actor:      "local-admin",
		Action:     audit.ActionObjectUpload,
		TargetType: "object",
		TargetID:   "ancient",
		Outcome:    audit.OutcomeFailure,
	})

	p := dashboard.NewProcessor(stubPool{}, stubBuckets{}, auditP)
	view, err := p.Build(ctx, dashboard.Window24h)
	require.NoError(t, err)
	// 5 rows total in the activity feed (newest first).
	assert.Len(t, view.RecentActivity, 5)
	// 2 failures inside the 24h window.
	assert.Equal(t, int64(2), view.RecentFailures.Count)
	assert.Len(t, view.RecentFailures.Entries, 2)
	// First failure entry must include the source_ip and error_message
	// the contract documents.
	assert.Equal(t, "10.0.1.5", view.RecentFailures.Entries[0].SourceIP)
	assert.Equal(t, "MinIO: NoSuchBucket", view.RecentFailures.Entries[0].ErrorMessage)
}

func TestBuild_FanOutErrorPropagates(t *testing.T) {
	t.Parallel()
	auditP := newTestAuditProcessor(t)
	// PoolGetter errs — Build must surface that error and not return a
	// partial View. The other goroutines may complete or be cancelled.
	p := dashboard.NewProcessor(stubPool{err: errBoom}, stubBuckets{}, auditP)
	_, err := p.Build(context.Background(), dashboard.Window7d)
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

func TestBuild_BucketsErrorPropagates(t *testing.T) {
	t.Parallel()
	auditP := newTestAuditProcessor(t)
	p := dashboard.NewProcessor(stubPool{}, stubBuckets{err: errBoom}, auditP)
	_, err := p.Build(context.Background(), dashboard.Window7d)
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

func TestBuild_RejectsUnconfiguredProcessor(t *testing.T) {
	t.Parallel()
	// nil pool / buckets / audit — Build must fail fast, not panic.
	cases := []struct {
		name    string
		pool    dashboard.PoolGetter
		bks     dashboard.BucketsLister
		auditer dashboard.AuditQuerier
	}{
		{"nil-pool", nil, stubBuckets{}, newTestAuditProcessor(t)},
		{"nil-bks", stubPool{}, nil, newTestAuditProcessor(t)},
		{"nil-audit", stubPool{}, stubBuckets{}, nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := dashboard.NewProcessor(tc.pool, tc.bks, tc.auditer)
			_, err := p.Build(context.Background(), dashboard.Window7d)
			require.Error(t, err)
		})
	}
}

// mustRecord persists an audit event or fails the test.
func mustRecord(t *testing.T, p *audit.Processor, e audit.Event) {
	t.Helper()
	require.NoError(t, p.Record(context.Background(), e))
}

// itoa avoids pulling in strconv for the single test loop above.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
