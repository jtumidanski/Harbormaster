package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
)

func TestMigrateCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sdb, err := Open(cfg)
	require.NoError(t, err)
	defer sdb.Close()

	require.NoError(t, Migrate(gdb))

	expected := []string{
		"admin_users", "sessions", "connections", "app_settings",
		"audit_events", "bucket_empty_jobs",
	}
	for _, table := range expected {
		var name string
		require.NoError(t,
			gdb.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name=?;`, table).Scan(&name).Error,
		)
		require.Equal(t, table, name, "table %q must exist after migration", table)
	}
}

func TestMigration0008RenamesConnectionsTable(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sdb, err := Open(cfg)
	require.NoError(t, err)
	defer sdb.Close()

	require.NoError(t, Migrate(gdb))

	var n int
	if err := gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='minio_connections'`).Scan(&n).Error; err != nil || n != 0 {
		t.Fatalf("old table must be gone: n=%d err=%v", n, err)
	}
	if err := gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='connections_singleton'`).Scan(&n).Error; err != nil || n != 1 {
		t.Fatalf("new unique index missing: n=%d err=%v", n, err)
	}
}
