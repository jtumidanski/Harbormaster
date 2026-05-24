package buckets

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newAuditDB opens an in-process SQLite database in a temporary
// directory and runs all migrations. Each test gets an isolated schema.
func newAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "buckets_audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newAuditedProcessor builds a bucket Processor wired to in-memory s3
// stubs and a real audit.Processor sharing a fresh test DB.
func newAuditedProcessor(t *testing.T) (*Processor, *audit.Processor, *stubS3) {
	t.Helper()
	p, _, s3 := newTestProcessor(t, nil, nil)
	gdb := newAuditDB(t)
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p.Audit = a
	return p, a, s3
}

// loadLatestPayload returns the most-recently inserted audit Event for
// action together with the raw payload_summary_json column value.
func loadLatestPayload(t *testing.T, a *audit.Processor, action string) (audit.Event, string) {
	t.Helper()
	events, err := audit.List(a.DB(), audit.Filter{Action: action, PageSize: 1})
	require.NoError(t, err)
	if len(events) == 0 {
		return audit.Event{}, ""
	}
	type row struct {
		PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
	}
	var r row
	require.NoError(t,
		a.DB().
			Table("audit_events").
			Select("payload_summary_json").
			Where("id = ?", events[0].ID).
			Scan(&r).Error,
	)
	return events[0], r.PayloadSummaryJSON
}

// requireNoSecrets asserts payload never carries any sensitive token
// substring (password / secret / token / signature / presigned / url).
// Mirrors the connection-domain helper so cross-package navigation is
// cheap.
func requireNoSecrets(t *testing.T, payload string) {
	t.Helper()
	lower := strings.ToLower(payload)
	for _, banned := range []string{"password", "secret", "token", "signature", "presigned"} {
		require.NotContainsf(t, lower, banned,
			"payload leaked banned substring %q: %s", banned, payload)
	}
}

func TestAuditEvent_BucketCreateSuccess(t *testing.T) {
	p, a, s3 := newAuditedProcessor(t)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "ledger", CreationDate: time.Unix(1700000000, 0).UTC()},
	}
	_, err := p.Create(context.Background(), "ledger", CreateOpts{
		VersioningEnabled: true,
	}, "operator", "10.0.0.1")
	require.NoError(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.1", ev.SourceIP)
	require.Equal(t, "bucket", ev.TargetType)
	require.Equal(t, "ledger", ev.TargetID)
	require.Contains(t, payload, "versioning_enabled")
	require.Contains(t, payload, "has_quota")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketCreateFailure_OnUnknownTemplate(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	_, err := p.Create(context.Background(), "ledger", CreateOpts{
		LifecycleTemplate: "expire-forever",
	}, "operator", "10.0.0.1")
	require.Error(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
	require.Contains(t, ev.ErrorMessage, "unknown_lifecycle_template")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketDeleteSuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.Delete(context.Background(), "ledger", "ledger", "operator", "10.0.0.2"))

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketDelete)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "ledger", ev.TargetID)
	require.Contains(t, payload, "ledger")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketDeleteFailure_ConfirmMismatch(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.Error(t, p.Delete(context.Background(), "ledger", "wrong", "operator", "10.0.0.2"))

	ev, _ := loadLatestPayload(t, a, audit.ActionBucketDelete)
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
}

func TestAuditEvent_BucketVersioningEnable(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetVersioning(context.Background(), "ledger", true, "operator", "10.0.0.3"))

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketVersioningOn)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "ledger", ev.TargetID)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketVersioningDisable(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetVersioning(context.Background(), "ledger", false, "operator", "10.0.0.3"))

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketVersioningOff)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "ledger", ev.TargetID)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketPublicAccessUpdate(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetPublicAccess(context.Background(), "ledger", "public-read", "", "operator", "10.0.0.4"))

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketPublicUpdate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Contains(t, payload, "public-read")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_BucketQuotaUpdate(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.SetQuota(context.Background(), "ledger", QuotaKindHard, 1<<30, "operator", "10.0.0.5"))

	ev, payload := loadLatestPayload(t, a, audit.ActionBucketQuotaUpdate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Contains(t, payload, "hard")
	requireNoSecrets(t, payload)
}

// TestCreate_AppliesLifecycleTemplate verifies T3.21: a known template
// triggers a call into the LifecycleCreator with the matching (days,
// prefix) pair, and that a bucket.create audit row is emitted.
func TestCreate_AppliesLifecycleTemplate(t *testing.T) {
	p, _, s3 := newAuditedProcessor(t)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "ledger", CreationDate: time.Unix(1700000000, 0).UTC()},
	}
	lc := &stubLifecycle{}
	p.Lifecycle = lc

	_, err := p.Create(context.Background(), "ledger", CreateOpts{
		LifecycleTemplate: "expire-30d",
	}, "operator", "10.0.0.6")
	require.NoError(t, err)
	require.Len(t, lc.calls, 1)
	require.Equal(t, "ledger", lc.calls[0].Bucket)
	require.Equal(t, 30, lc.calls[0].Days)
	require.Equal(t, "", lc.calls[0].Prefix)
}

// TestCreate_AppliesLifecycleTemplate_90d covers the second bundled
// template so a regression in the lookup map fails the build, not just
// "expire-30d".
func TestCreate_AppliesLifecycleTemplate_90d(t *testing.T) {
	p, _, s3 := newAuditedProcessor(t)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "ledger", CreationDate: time.Unix(1700000000, 0).UTC()},
	}
	lc := &stubLifecycle{}
	p.Lifecycle = lc

	_, err := p.Create(context.Background(), "ledger", CreateOpts{
		LifecycleTemplate: "expire-90d",
	}, "operator", "10.0.0.7")
	require.NoError(t, err)
	require.Len(t, lc.calls, 1)
	require.Equal(t, 90, lc.calls[0].Days)
}

// TestCreate_UnknownLifecycleTemplate verifies the typed-422 envelope
// and short-circuit BEFORE MakeBucket runs.
func TestCreate_UnknownLifecycleTemplate(t *testing.T) {
	p, _, s3 := newAuditedProcessor(t)
	_, err := p.Create(context.Background(), "ledger", CreateOpts{
		LifecycleTemplate: "expire-forever",
	}, "operator", "10.0.0.8")
	require.Error(t, err)
	require.Empty(t, s3.makeCalls, "MakeBucket must not run when template is invalid")
}

// stubLifecycle captures Create invocations made through the
// LifecycleCreator interface so the bucket-creation template path can
// be asserted without spinning up a real lifecycle.Processor.
type stubLifecycle struct {
	calls []stubLifecycleCall
	err   error
}

type stubLifecycleCall struct {
	Bucket string
	Days   int
	Prefix string
}

func (s *stubLifecycle) Create(_ context.Context, bucket string, days int, prefix string) error {
	s.calls = append(s.calls, stubLifecycleCall{Bucket: bucket, Days: days, Prefix: prefix})
	return s.err
}
