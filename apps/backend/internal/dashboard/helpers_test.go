package dashboard_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/dashboard"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// stubPool returns canned ServerInfo / nodes / warnings.
// errOnCall, when non-nil, is returned to exercise the fan-out
// error-propagation path.
type stubPool struct {
	info     dashboard.ServerInfo
	nodes    []dashboard.NodeStatus
	warnings []string
	err      error
}

func (s stubPool) ServerInfo(_ context.Context) (dashboard.ServerInfo, []dashboard.NodeStatus, []string, error) {
	if s.err != nil {
		return dashboard.ServerInfo{}, nil, nil, s.err
	}
	return s.info, s.nodes, s.warnings, nil
}

// stubBuckets returns a canned slice of buckets and an optional error.
type stubBuckets struct {
	out []buckets.Bucket
	err error
}

func (s stubBuckets) List(_ context.Context) ([]buckets.Bucket, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.out, nil
}

// errBoom is reused across fan-out failure assertions.
var errBoom = errors.New("boom")

// newTestDB opens a fresh in-process SQLite database and runs all
// migrations so the live audit.Processor can satisfy AuditQuerier.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "dashboard_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newTestAuditProcessor returns a fresh audit.Processor backed by the
// in-memory test DB.
func newTestAuditProcessor(t *testing.T) *audit.Processor {
	t.Helper()
	return audit.NewProcessor(newTestDB(t), 90*24*time.Hour)
}
