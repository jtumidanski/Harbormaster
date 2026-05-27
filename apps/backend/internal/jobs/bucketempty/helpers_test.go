package bucketempty

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newTestDB opens an in-process SQLite database (file-backed, in t.TempDir)
// and runs all migrations. The underlying *sql.DB closer is registered with
// t.Cleanup so each test gets an isolated schema.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "bucketempty_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// auditCall records one Recorder invocation for later assertion.
type auditCall struct {
	Action  string
	Target  string
	Outcome string
	Payload map[string]any
	ErrMsg  string
}

// fakeAudit is a thread-safe AuditRecorder that captures every call.
type fakeAudit struct {
	mu    sync.Mutex
	calls []auditCall
}

func (f *fakeAudit) Record(_ context.Context, action, target, outcome string, payload map[string]any, errMsg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, auditCall{Action: action, Target: target, Outcome: outcome, Payload: payload, ErrMsg: errMsg})
}

func (f *fakeAudit) snapshot() []auditCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]auditCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// fakePool returns nil clients; tests that exercise StartOrAttach replace
// Service.runFn so the worker body is never invoked and pool.Get is never
// called. The interface is satisfied for compile-time wiring.
type fakePool struct{}

func (fakePool) Get(_ context.Context) (*madmin.AdminClient, *miniogo.Client, error) {
	return nil, nil, nil
}

// newServiceWithRun builds a Service with a substituted runFn so tests can
// drive the worker deterministically without a real MinIO. The default
// production runFn (s.run) needs a live pool + S3 endpoint.
func newServiceWithRun(t *testing.T, gdb *gorm.DB, audit AuditRecorder, runFn func(s *Service, ctx context.Context, sub *subscription, purgeVersions bool)) *Service {
	t.Helper()
	s := New(gdb, fakePool{}, audit)
	s.runFn = func(ctx context.Context, sub *subscription, purgeVersions bool) {
		runFn(s, ctx, sub, purgeVersions)
	}
	return s
}
