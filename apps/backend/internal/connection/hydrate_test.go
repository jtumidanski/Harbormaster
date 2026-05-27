package connection_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/connection"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

// TestHydratePool_BindsFromPersistedRow verifies that a freshly-booted
// process binds its (initially empty) MinIO pool from the persisted
// connection row. This is the restart-safety half of the onboarding 503
// fix: the in-memory pool starts empty on every boot, so without hydration a
// restarted pod would never become ready.
func TestHydratePool_BindsFromPersistedRow(t *testing.T) {
	gdb := newTestDB(t)
	cipher := newTestCipher(t)
	ctx := context.Background()

	// Persist a connection row via the normal update path. Its own pool is
	// irrelevant here — we only need the encrypted row on disk.
	writer := connection.NewProcessor(gdb, cipher, hmminio.NewEmpty())
	writer.Probe = stubProbeOK
	require.NoError(t, writer.Update(ctx, connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIAEXAMPLE",
		SecretKey:   "topsecretvalue",
	}, "admin", "127.0.0.1"))

	// A second processor with a fresh empty pool simulates a process restart.
	pool := hmminio.NewEmpty()
	booted := connection.NewProcessor(gdb, cipher, pool)
	if _, _, err := pool.Get(ctx); err == nil {
		t.Fatal("pool must be empty before hydration")
	}

	require.NoError(t, booted.HydratePool(ctx))

	_, _, err := pool.Get(ctx)
	require.NoError(t, err, "pool must be bound after hydration from the persisted row")
}

// TestHydratePool_NoRowIsNoop verifies that hydration is a silent no-op when
// setup has not yet stored a connection — boot must not fail before first-run
// setup, and the pool stays unbound.
func TestHydratePool_NoRowIsNoop(t *testing.T) {
	gdb := newTestDB(t)
	cipher := newTestCipher(t)
	pool := hmminio.NewEmpty()
	p := connection.NewProcessor(gdb, cipher, pool)
	ctx := context.Background()

	require.NoError(t, p.HydratePool(ctx), "no persisted row must not error")

	if _, _, err := pool.Get(ctx); err == nil {
		t.Fatal("pool must stay unbound when nothing has been persisted")
	}
}
