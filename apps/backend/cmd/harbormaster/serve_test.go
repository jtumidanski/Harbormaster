package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

// TestDBReadiness verifies that readiness reflects only database reachability
// and takes no MinIO pool. This is the design half of the onboarding 503 fix:
// a MinIO outage must never pull the pod from the Service and lock operators
// out of the login page.
func TestDBReadiness(t *testing.T) {
	dir := t.TempDir()
	_, sdb, err := db.Open(config.Config{
		DataDir:      dir,
		DatabasePath: filepath.Join(dir, "ready.db"),
	})
	require.NoError(t, err)

	ready := dbReadiness(sdb)

	ok, _ := ready(context.Background())
	require.True(t, ok, "readiness is true when the database is reachable")

	require.NoError(t, sdb.Close())
	ok, reason := ready(context.Background())
	require.False(t, ok, "readiness is false when the database is unreachable")
	require.NotEmpty(t, reason, "an unready response must carry a reason")
}

func TestServeFailsOnKeyFingerprintMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARBORMASTER_DATA_DIR", dir)
	t.Setenv("HARBORMASTER_DATABASE_PATH", filepath.Join(dir, "h.db"))
	t.Setenv("HARBORMASTER_LISTEN_ADDR", "127.0.0.1:0")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), make([]byte, 32), 0o600))

	// First call records the fingerprint.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = runServe(ctx, os.Stdout) // it boots, then ctx times out

	// Swap the key file with a different one.
	newKey := make([]byte, 32)
	newKey[0] = 0xFF
	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), newKey, 0o600))

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	err := runServe(ctx2, os.Stdout)
	require.ErrorContains(t, err, "encryption key fingerprint mismatch")
}
