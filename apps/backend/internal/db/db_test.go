package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
)

func TestOpenCreatesWALMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sql, err := Open(cfg)
	require.NoError(t, err)
	defer sql.Close()
	var mode string
	require.NoError(t, gdb.Raw("PRAGMA journal_mode;").Scan(&mode).Error)
	require.Equal(t, "wal", mode)
	var fk int
	require.NoError(t, gdb.Raw("PRAGMA foreign_keys;").Scan(&fk).Error)
	require.Equal(t, 1, fk)
}

func TestSingleConnectionLimits(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	_, sql, err := Open(cfg)
	require.NoError(t, err)
	defer sql.Close()
	stats := sql.Stats()
	require.Equal(t, 1, stats.MaxOpenConnections)
}
