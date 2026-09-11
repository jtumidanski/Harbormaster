package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/httpx"
)

// TestClientIP_FallsBackToRemoteAddr covers handlers driven directly in unit
// tests, where no ClientIPFrom* middleware has run.
func TestClientIP_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:54321"
	require.Equal(t, "203.0.113.7", httpx.ClientIP(r))
}

// TestClientIP_FallsBackToBareRemoteAddr covers the httptest default and any
// other RemoteAddr that carries no port.
func TestClientIP_FallsBackToBareRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7"
	require.Equal(t, "203.0.113.7", httpx.ClientIP(r))
}

// TestClientIP_IgnoresForgedHeadersWithoutMiddleware is the regression guard
// for the spoofing hole chi's deprecated RealIP left open: a client-supplied
// header must never reach the audit trail on its own.
func TestClientIP_IgnoresForgedHeadersWithoutMiddleware(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:54321"
	r.Header.Set("X-Forwarded-For", "10.9.9.9")
	r.Header.Set("X-Real-IP", "10.9.9.9")
	r.Header.Set("True-Client-IP", "10.9.9.9")
	require.Equal(t, "203.0.113.7", httpx.ClientIP(r))
}

// TestClientIP_PrefersMiddlewareValue verifies ClientIP reads what the
// ClientIPFrom* middleware stored rather than re-deriving it.
func TestClientIP_PrefersMiddlewareValue(t *testing.T) {
	var got string
	h := chimw.ClientIPFromXFF("192.168.0.0/16")(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) { got = httpx.ClientIP(r) }))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:54321"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 192.168.1.1")
	h.ServeHTTP(httptest.NewRecorder(), r)

	require.Equal(t, "203.0.113.7", got)
}
