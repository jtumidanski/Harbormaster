package auth_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/auth"
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
		DatabasePath: filepath.Join(dir, "auth_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// seedAdmin inserts an admin row with the given username and plaintext
// password (hashed via auth.HashPassword) and returns the resulting ID.
func seedAdmin(t *testing.T, gdb *gorm.DB, username, password string) uint {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res := gdb.Exec(
		`INSERT INTO admin_users (username, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		username, hash, now, now,
	)
	require.NoError(t, res.Error)
	var id uint
	require.NoError(t,
		gdb.Raw("SELECT id FROM admin_users WHERE username = ?", username).Scan(&id).Error,
	)
	require.NotZero(t, id)
	return id
}
