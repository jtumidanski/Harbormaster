package metrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newTestDB opens an isolated SQLite database in a temp directory and runs
// every up-migration. The sql.DB closer is registered with t.Cleanup.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "metrics_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newTestStore returns a Store backed by a fresh migrated DB.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(newTestDB(t))
}

func TestStoreInsertAndQueryWindow(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = st.Insert(context.Background(), base, map[string]float64{"minio_s3_requests_total": 100})
	_ = st.Insert(context.Background(), base.Add(time.Minute), map[string]float64{"minio_s3_requests_total": 110})
	pts, err := st.Query(context.Background(), []string{"minio_s3_requests_total"}, base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts["minio_s3_requests_total"]) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(pts["minio_s3_requests_total"]))
	}
}

func TestStoreRetentionSweep(t *testing.T) {
	st := newTestStore(t)
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = st.Insert(context.Background(), old, map[string]float64{"m": 1})
	n, err := st.RetentionSweep(old.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted, got %d", n)
	}
}
