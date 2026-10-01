package db

import (
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
	migrations "github.com/jtumidanski/Harbormaster/migrations"
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

// newTestMigrator builds a golang-migrate instance over the embedded
// migrations, the same wiring Migrate uses, so a test can step to a specific
// schema version.
func newTestMigrator(t *testing.T, gdb *gorm.DB) *migrate.Migrate {
	t.Helper()
	sdb, err := gdb.DB()
	require.NoError(t, err)
	src, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	driver, err := newSQLiteMigrateDriver(sdb)
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	require.NoError(t, err)
	return m
}

// TestMigration0009RenamesMetricNames migrates a fresh database to version
// 8, stores samples under the pre-rename names, then applies 0009 and checks
// every minio_ series moved to objectstore_ (and back again on down).
func TestMigration0009RenamesMetricNames(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sdb, err := Open(cfg)
	require.NoError(t, err)
	defer sdb.Close()

	m := newTestMigrator(t, gdb)
	require.NoError(t, m.Migrate(8))

	insert := `INSERT INTO metrics_samples (id, collected_at, metric, value) VALUES (?, ?, ?, ?)`
	require.NoError(t, gdb.Exec(insert, "a", "2026-10-01T00:00:00Z", "minio_s3_requests_total", 1.0).Error)
	require.NoError(t, gdb.Exec(insert, "b", "2026-10-01T00:00:00Z", "minio_cluster_drive_online_total", 4.0).Error)
	// An underscore-free prefix lookalike must not be rewritten: LIKE's "_"
	// wildcard is escaped in the migration.
	require.NoError(t, gdb.Exec(insert, "c", "2026-10-01T00:00:00Z", "minioXs3", 2.0).Error)

	require.NoError(t, m.Migrate(9))

	metricOf := func(id string) string {
		var metric string
		require.NoError(t, gdb.Raw(`SELECT metric FROM metrics_samples WHERE id = ?`, id).Scan(&metric).Error)
		return metric
	}
	require.Equal(t, "objectstore_s3_requests_total", metricOf("a"))
	require.Equal(t, "objectstore_cluster_drive_online_total", metricOf("b"))
	require.Equal(t, "minioXs3", metricOf("c"))

	var n int
	require.NoError(t, gdb.Raw(`SELECT count(*) FROM metrics_samples WHERE metric LIKE 'minio\_%' ESCAPE '\'`).Scan(&n).Error)
	require.Equal(t, 0, n, "no minio_ series may remain after 0009")

	require.NoError(t, m.Migrate(8))
	require.Equal(t, "minio_s3_requests_total", metricOf("a"))
	require.Equal(t, "minio_cluster_drive_online_total", metricOf("b"))
	require.Equal(t, "minioXs3", metricOf("c"))
}
