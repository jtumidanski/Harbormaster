package server_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/httpx"
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

// clientIPProbe boots a server with the given trusted-proxy CIDRs and returns
// the client IP the middleware stack derives for a request carrying xff.
func clientIPProbe(t *testing.T, addr string, trusted []string, xff string) string {
	t.Helper()
	cfg := config.Config{
		ListenAddr: addr, LogLevel: "info", LogFormat: "json", BasePath: "/",
		TrustedProxies: trusted,
	}
	l, _ := log.New("info", "json")
	s := server.New(cfg, server.Deps{
		Logger: l,
		APIRoutes: []func(chi.Router){func(r chi.Router) {
			r.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, httpx.ClientIP(r))
			})
		}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("shutdown timed out")
		}
	})
	time.Sleep(100 * time.Millisecond)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/v1/whoami", nil)
	require.NoError(t, err)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
		req.Header.Set("X-Real-IP", "10.9.9.9")
		req.Header.Set("True-Client-IP", "10.9.9.9")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

// TestClientIP_NoTrustedProxies_IgnoresForwardedHeaders is the regression
// guard for the spoofing hole chi's deprecated RealIP left open: with
// HARBORMASTER_TRUSTED_PROXIES empty, forged forwarding headers must not
// influence the IP recorded in audit events or used as the login rate-limit
// key. The TCP peer address wins.
func TestClientIP_NoTrustedProxies_IgnoresForwardedHeaders(t *testing.T) {
	require.Equal(t, "127.0.0.1", clientIPProbe(t, "127.0.0.1:18083", nil, "203.0.113.9"))
}

// TestClientIP_TrustedProxy_HonoursForwardedFor verifies the behaviour the
// operator docs promise for HARBORMASTER_TRUSTED_PROXIES: the rightmost XFF
// entry outside the trusted CIDRs is the client.
func TestClientIP_TrustedProxy_HonoursForwardedFor(t *testing.T) {
	got := clientIPProbe(t, "127.0.0.1:18084", []string{"127.0.0.0/8"}, "203.0.113.9, 127.0.0.1")
	require.Equal(t, "203.0.113.9", got)
}

// TestClientIP_TrustedProxy_StopsAtFirstUntrustedHop verifies an attacker who
// prepends extra entries cannot reach past the hop their own proxy recorded.
func TestClientIP_TrustedProxy_StopsAtFirstUntrustedHop(t *testing.T) {
	got := clientIPProbe(t, "127.0.0.1:18085", []string{"127.0.0.0/8"}, "10.9.9.9, 203.0.113.9, 127.0.0.1")
	require.Equal(t, "203.0.113.9", got)
}
