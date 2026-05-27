package setup_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
	"github.com/jtumidanski/Harbormaster/internal/setup"
)

// newTestDB opens an isolated on-disk SQLite database in a temp directory
// and runs every up-migration. The sql.DB closer is registered with
// t.Cleanup.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "setup_test.db"),
	}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sdb.Close() })
	require.NoError(t, db.Migrate(gdb))
	return gdb
}

// newTestCipher constructs a deterministic 32-byte-keyed cipher.
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

// stubProbeOK is a Prober that always reports success, used to bypass the
// network probe during Submit tests.
func stubProbeOK(_ context.Context, _ connection.SubmitInput) (connection.TestResult, *apierror.Error) {
	return connection.TestResult{
		TCPConnect:   "ok",
		ListBuckets:  "ok",
		AdminPing:    "ok",
		MinIOVersion: "RELEASE.2026-01-01T00-00-00Z",
	}, nil
}

// newProcessor builds a setup.Processor wired to a fresh test DB, a
// deterministic cipher, a connection.Processor with the success-stub probe,
// and an mc-config path under a temp dir (which the caller may overwrite).
func newProcessor(t *testing.T, mcPath string) (*setup.Processor, *gorm.DB) {
	t.Helper()
	gdb := newTestDB(t)
	cipher := newTestCipher(t)
	pool := hmminio.NewEmpty()
	connProc := connection.NewProcessor(gdb, cipher, pool)
	connProc.Probe = stubProbeOK
	authProc := auth.NewProcessor(gdb)
	p := &setup.Processor{
		DB:       gdb,
		Cipher:   cipher,
		AuthProc: authProc,
		ConnProc: connProc,
		McPath:   mcPath,
	}
	return p, gdb
}
