package server_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/server"
)

func TestServerHealthzAndShutdown(t *testing.T) {
	cfg := config.Config{ListenAddr: "127.0.0.1:0", LogLevel: "info", LogFormat: "json", BasePath: "/"}
	cfg.ListenAddr = "127.0.0.1:18080"
	l, _ := log.New("info", "json")
	s := server.New(cfg, server.Deps{Logger: l})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18080/healthz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(body), `"status":"ok"`)

	resp2, err := http.Get("http://127.0.0.1:18080/api/v1/anything")
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, 404, resp2.StatusCode)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

// TestReadyz_NilReady_Returns200 verifies the M1 backwards-compatible path:
// when Deps.Ready is nil, /readyz always returns 200.
func TestReadyz_NilReady_Returns200(t *testing.T) {
	cfg := config.Config{ListenAddr: "127.0.0.1:18081", LogLevel: "info", LogFormat: "json", BasePath: "/"}
	l, _ := log.New("info", "json")
	s := server.New(cfg, server.Deps{Logger: l})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18081/readyz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), `"status":"ok"`)

	cancel()
	<-done
}

// TestReadyz_ReadyFalse_Returns503 verifies that a Ready probe returning
// (false, reason) produces a 503 with the apierror not_ready envelope.
func TestReadyz_ReadyFalse_Returns503(t *testing.T) {
	cfg := config.Config{ListenAddr: "127.0.0.1:18082", LogLevel: "info", LogFormat: "json", BasePath: "/"}
	l, _ := log.New("info", "json")
	s := server.New(cfg, server.Deps{
		Logger: l,
		Ready: func(_ context.Context) (bool, string) {
			return false, "minio probe stale"
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18082/readyz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Contains(t, string(body), `"not_ready"`)
	require.Contains(t, string(body), `"minio probe stale"`)

	cancel()
	<-done
}
