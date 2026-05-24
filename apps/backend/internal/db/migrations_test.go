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
		"admin_users", "sessions", "minio_connections", "app_settings",
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
