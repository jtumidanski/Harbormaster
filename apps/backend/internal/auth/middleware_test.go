package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
)

const (
	sessionCookieName = "harbormaster_session"
	csrfCookieName    = "harbormaster_csrf"
)

// markerHandler is the inner handler used by middleware tests. It writes a
// short marker so callers can confirm "next" ran, and (when a session is
// present in context) it echoes the username.
func markerHandler(t *testing.T, expectUsername string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectUsername != "" {
			si, ok := auth.FromContext(r.Context())
			require.True(t, ok, "expected SessionInfo in context")
			require.Equal(t, expectUsername, si.Username)
			require.NotEmpty(t, si.SessionID)
			require.NotZero(t, si.AdminUserID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func readErrorCode(t *testing.T, body string) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	return env.Error.Code
}

func TestRequireSession_NoCookie_401(t *testing.T) {
	gdb := newTestDB(t)
	p := auth.NewProcessor(gdb)

	h := auth.RequireSession(sessionCookieName, p, apierror.StyleAction)(markerHandler(t, ""))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, "unauthenticated", readErrorCode(t, rr.Body.String()))
}

func TestRequireSession_UnknownCookie_401(t *testing.T) {
	gdb := newTestDB(t)
	p := auth.NewProcessor(gdb)

	h := auth.RequireSession(sessionCookieName, p, apierror.StyleAction)(markerHandler(t, ""))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "01NONEXISTENT"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, "unauthenticated", readErrorCode(t, rr.Body.String()))
}

func TestRequireSession_Expired_401(t *testing.T) {
	gdb := newTestDB(t)
	adminID := seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	// Insert an expired session row directly so the cookie value resolves
	// to a known, but stale, row in the database.
	pastCreated := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	pastExpiry := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	res := gdb.Exec(
		`INSERT INTO sessions (id, admin_user_id, created_at, expires_at, last_active_at, source_ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"01EXPIREDMW", adminID, pastCreated, pastExpiry, pastCreated, "1.2.3.4", "ua",
	)
	require.NoError(t, res.Error)

	h := auth.RequireSession(sessionCookieName, p, apierror.StyleAction)(markerHandler(t, ""))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "01EXPIREDMW"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, "unauthenticated", readErrorCode(t, rr.Body.String()))
}

func TestRequireSession_Valid_PassesContext(t *testing.T) {
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "pw")
	p := auth.NewProcessor(gdb)

	sid, _, err := p.Login(context.Background(), "admin", "pw", "1.2.3.4", "ua")
	require.NoError(t, err)

	h := auth.RequireSession(sessionCookieName, p, apierror.StyleAction)(markerHandler(t, "admin"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())
}

func TestRequireCSRF_SafeMethodsPass(t *testing.T) {
	h := auth.RequireCSRF(csrfCookieName, apierror.StyleAction)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			require.Equal(t, http.StatusOK, rr.Code)
		})
	}
}

func TestRequireCSRF_PostWithoutHeader_403(t *testing.T) {
	h := auth.RequireCSRF(csrfCookieName, apierror.StyleAction)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Equal(t, "csrf_token_invalid", readErrorCode(t, rr.Body.String()))
}

func TestRequireCSRF_PostWithoutCookie_403(t *testing.T) {
	h := auth.RequireCSRF(csrfCookieName, apierror.StyleAction)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set("X-CSRF-Token", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Equal(t, "csrf_token_invalid", readErrorCode(t, rr.Body.String()))
}

func TestRequireCSRF_PostMismatched_403(t *testing.T) {
	h := auth.RequireCSRF(csrfCookieName, apierror.StyleAction)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc"})
	req.Header.Set("X-CSRF-Token", "xyz")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	require.Equal(t, "csrf_token_invalid", readErrorCode(t, rr.Body.String()))
}

func TestRequireCSRF_PostMatched_OK(t *testing.T) {
	h := auth.RequireCSRF(csrfCookieName, apierror.StyleAction)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "match"})
	req.Header.Set("X-CSRF-Token", "match")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", rr.Body.String())
}

func TestFromContext_AbsentReturnsFalse(t *testing.T) {
	_, ok := auth.FromContext(context.Background())
	require.False(t, ok)
}
