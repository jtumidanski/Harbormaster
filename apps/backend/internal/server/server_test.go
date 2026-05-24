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
