package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newAuditDB opens a fresh in-process SQLite test DB with all
// migrations applied.
func newAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "lifecycle_audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newAuditedProcessor builds a lifecycle Processor wired to an
// in-memory stub and a real audit.Processor sharing a fresh test DB.
func newAuditedProcessor(t *testing.T, s3 *stubS3) (*Processor, *audit.Processor, *stubS3) {
	t.Helper()
	p, s3 := newTestProcessor(t, s3)
	gdb := newAuditDB(t)
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p.Audit = a
	return p, a, s3
}

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

func requireNoSecrets(t *testing.T, payload string) {
	t.Helper()
	lower := strings.ToLower(payload)
	for _, banned := range []string{"password", "secret", "token", "signature", "presigned"} {
		require.NotContainsf(t, lower, banned,
			"payload leaked banned substring %q: %s", banned, payload)
	}
}

func TestAuditEvent_LifecycleRuleCreateSuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	_, err := p.Create(context.Background(), "ledger", 30, "logs/", "operator", "10.0.0.1")
	require.NoError(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionLifecycleRuleCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.1", ev.SourceIP)
	require.Equal(t, "bucket", ev.TargetType)
	require.Equal(t, "ledger", ev.TargetID)
	require.Contains(t, payload, "rule_id")
	require.Contains(t, payload, "days")
	require.Contains(t, payload, "prefix")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_LifecycleRuleCreateFailure_InvalidDays(t *testing.T) {
	p, a, _ := newAuditedProcessor(t, nil)
	_, err := p.Create(context.Background(), "ledger", 0, "", "operator", "10.0.0.1")
	require.Error(t, err)

	ev, _ := loadLatestPayload(t, a, audit.ActionLifecycleRuleCreate)
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
}

func TestAuditEvent_LifecycleRuleDeleteSuccess(t *testing.T) {
	cfg := mlifecycle.NewConfiguration()
	cfg.Rules = []mlifecycle.Rule{
		{ID: "drop-me", Status: "Enabled"},
	}
	p, a, _ := newAuditedProcessor(t, &stubS3{getCfg: cfg})
	require.NoError(t, p.Delete(context.Background(), "ledger", "drop-me", "operator", "10.0.0.2"))

	ev, payload := loadLatestPayload(t, a, audit.ActionLifecycleRuleDelete)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "ledger", ev.TargetID)
	require.Contains(t, payload, "drop-me")
	requireNoSecrets(t, payload)
}
