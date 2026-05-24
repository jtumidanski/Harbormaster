package connection_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// newTestDB opens an isolated on-disk SQLite database (the in-process
// driver requires a file path) in a temp directory and runs every
// up-migration. The sql.DB closer is registered with t.Cleanup.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "connection_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newTestCipher constructs a deterministic 32-byte-keyed cipher. The key
// is constant so failures can be reproduced from the test output; it
// never touches a real master-key file.
func newTestCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.New(key)
	require.NoError(t, err)
	return c
}
