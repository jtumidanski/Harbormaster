package objects

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		DatabasePath: filepath.Join(dir, "objects_audit_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newAuditedProcessor wires an object Processor against in-memory stubs
// and a real audit.Processor sharing a fresh test DB.
func newAuditedProcessor(t *testing.T) (*Processor, *audit.Processor, *stubS3) {
	t.Helper()
	p, s3 := newTestProcessor(t, nil, ProcessorConfig{ShareLinkMaxTTL: time.Hour})
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
	for _, banned := range []string{"password", "secret", "token", "signature", "presigned", "://"} {
		require.NotContainsf(t, lower, banned,
			"payload leaked banned substring %q: %s", banned, payload)
	}
}

func TestAuditEvent_ObjectUploadSuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	_, err := p.Upload(context.Background(), "photos", "cat.jpg",
		bytes.NewBufferString("payload"), "image/jpeg", "operator", "10.0.0.1")
	require.NoError(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionObjectUpload)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.1", ev.SourceIP)
	require.Equal(t, "object", ev.TargetType)
	require.Equal(t, "photos/cat.jpg", ev.TargetID)
	require.Contains(t, payload, "size")
	require.Contains(t, payload, "cat.jpg")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ObjectUploadFailure_InvalidKey(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	_, err := p.Upload(context.Background(), "photos", "",
		bytes.NewBufferString("x"), "text/plain", "operator", "10.0.0.1")
	require.Error(t, err)

	ev, _ := loadLatestPayload(t, a, audit.ActionObjectUpload)
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
}

func TestAuditEvent_ObjectDeleteSuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	require.NoError(t, p.Delete(context.Background(), "photos", "cat.jpg", "operator", "10.0.0.2"))

	ev, payload := loadLatestPayload(t, a, audit.ActionObjectDelete)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "photos/cat.jpg", ev.TargetID)
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ObjectDownloadProxySuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	rc, _, err := p.Download(context.Background(), "photos", "cat.jpg", "operator", "10.0.0.3")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()

	ev, payload := loadLatestPayload(t, a, audit.ActionObjectDownloadProxy)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "photos/cat.jpg", ev.TargetID)
	requireNoSecrets(t, payload)
}

// TestAuditEvent_ObjectShareLinkCreate verifies the payload never
// leaks the minted URL (audit.Sanitize would drop a `url` key
// defensively, but the processor also avoids including it in the first
// place). The expires_seconds value comes from the clamped TTL.
func TestAuditEvent_ObjectShareLinkCreate(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	_, err := p.MintShareLink(context.Background(), "photos", "cat.jpg", 60, "operator", "10.0.0.4")
	require.NoError(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionObjectShareLinkCreate)
	require.NotEmpty(t, payload)
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "photos/cat.jpg", ev.TargetID)
	require.Contains(t, payload, "expires_seconds")
	// Spec: NEVER persist the URL.
	require.NotContains(t, strings.ToLower(payload), "minio.example")
	requireNoSecrets(t, payload)
}
