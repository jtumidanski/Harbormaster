package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

// newHandlerServer builds a chi router that mirrors the production wiring:
// the full Routes() group is mounted twice with different middleware stacks
// so we can exercise both the public and the session-gated paths.
//
// In production, /auth/login and /csrf are public; /auth/me, /auth/logout,
// and /auth/password run behind RequireSession. The router below applies
// RequireSession to the privileged paths via a routed middleware filter on
// the per-request method+path, so a single Routes() registration suffices.
func newHandlerServer(t *testing.T, deps auth.HandlerDeps) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	// Apply RequireSession only when the path requires it.
	mw := auth.RequireSession(deps.SessionCookieName, deps.Processor, 0)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if isProtected(req.Method, req.URL.Path) {
				mw(next).ServeHTTP(w, req)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Route("/", auth.Routes(deps))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// isProtected reports whether the route requires a valid session cookie.
func isProtected(method, path string) bool {
	switch path {
	case "/auth/me":
		return method == http.MethodGet
	case "/auth/logout", "/auth/password":
		return method == http.MethodPost
	}
	return false
}

func newHandlerDeps(t *testing.T) (auth.HandlerDeps, func()) {
	t.Helper()
	gdb := newTestDB(t)
	seedAdmin(t, gdb, "admin", "supersecretpassword")
	p := auth.NewProcessor(gdb)
	rl := auth.NewLoginRateLimiter(5*time.Minute, 5)
	return auth.HandlerDeps{
		Processor:         p,
		RateLimiter:       rl,
		SessionCookieName: "harbormaster_session",
		CSRFCookieName:    "harbormaster_csrf",
		BasePath:          "",
		SessionTimeout:    8 * time.Hour,
		Secure:            false,
	}, func() {}
}

func postJSON(t *testing.T, url string, body any, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func getReq(t *testing.T, url string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ---- Test cases ---------------------------------------------------------

func TestLogin_Success_SetsBothCookies(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	resp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	sess := findCookie(resp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess, "session cookie missing")
	require.NotEmpty(t, sess.Value)
	require.True(t, sess.HttpOnly, "session cookie must be HttpOnly")
	require.Equal(t, http.SameSiteLaxMode, sess.SameSite)

	csrf := findCookie(resp.Cookies(), deps.CSRFCookieName)
	require.NotNil(t, csrf, "csrf cookie missing")
	require.NotEmpty(t, csrf.Value)
	require.False(t, csrf.HttpOnly, "csrf cookie must NOT be HttpOnly")
	require.Equal(t, http.SameSiteLaxMode, csrf.SameSite)
}

func TestLogin_BadCredentials_401(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	resp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong",
	})
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "invalid_credentials")
	require.Nil(t, findCookie(resp.Cookies(), deps.SessionCookieName))
}

func TestLogin_RateLimit_6thAttempt_429(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	// 5 bad attempts → each returns 401.
	for i := 0; i < 5; i++ {
		resp := postJSON(t, srv.URL+"/auth/login", map[string]string{
			"username": "admin",
			"password": "wrong",
		})
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "attempt %d", i+1)
		resp.Body.Close()
	}
	// 6th attempt within the window → 429.
	resp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "too_many_attempts")
}

func TestLogin_BadJSON_400(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/login", strings.NewReader("{not json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "bad_request")
}

func TestMe_WithSessionCookie_ReturnsUsername(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	loginResp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	loginResp.Body.Close()
	sess := findCookie(loginResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess)

	resp := getReq(t, srv.URL+"/auth/me", sess)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		Username         string    `json:"username"`
		SessionExpiresAt time.Time `json:"session_expires_at"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "admin", out.Username)
	require.True(t, out.SessionExpiresAt.After(time.Now().UTC()))
}

func TestMe_WithoutSession_401(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	resp := getReq(t, srv.URL+"/auth/me")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLogout_ClearsCookiesAndInvalidatesSession(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	loginResp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	loginResp.Body.Close()
	sess := findCookie(loginResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess)

	logoutResp := postJSON(t, srv.URL+"/auth/logout", struct{}{}, sess)
	defer logoutResp.Body.Close()
	require.Equal(t, http.StatusNoContent, logoutResp.StatusCode)

	clearedSess := findCookie(logoutResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, clearedSess, "logout must emit a clearing Set-Cookie")
	require.Equal(t, "", clearedSess.Value)
	require.True(t, clearedSess.MaxAge < 0, "session cookie must be deleted via MaxAge<0")

	clearedCSRF := findCookie(logoutResp.Cookies(), deps.CSRFCookieName)
	require.NotNil(t, clearedCSRF)
	require.True(t, clearedCSRF.MaxAge < 0)

	// The old session cookie should no longer authenticate /auth/me.
	resp := getReq(t, srv.URL+"/auth/me", sess)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestChangePassword_WrongCurrent_401(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	loginResp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	loginResp.Body.Close()
	sess := findCookie(loginResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess)

	resp := postJSON(t, srv.URL+"/auth/password", map[string]string{
		"current_password": "WRONG",
		"new_password":     "another-long-password",
	}, sess)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "invalid_credentials")
}

func TestChangePassword_WeakPassword_422(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	loginResp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	loginResp.Body.Close()
	sess := findCookie(loginResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess)

	resp := postJSON(t, srv.URL+"/auth/password", map[string]string{
		"current_password": "supersecretpassword",
		"new_password":     "short",
	}, sess)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "weak_password")
}

func TestChangePassword_HappyPath_204(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	loginResp := postJSON(t, srv.URL+"/auth/login", map[string]string{
		"username": "admin",
		"password": "supersecretpassword",
	})
	loginResp.Body.Close()
	sess := findCookie(loginResp.Cookies(), deps.SessionCookieName)
	require.NotNil(t, sess)

	resp := postJSON(t, srv.URL+"/auth/password", map[string]string{
		"current_password": "supersecretpassword",
		"new_password":     "another-long-password-99",
	}, sess)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestCSRF_IssuesCookieAndBody(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	resp := getReq(t, srv.URL+"/csrf")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	csrf := findCookie(resp.Cookies(), deps.CSRFCookieName)
	require.NotNil(t, csrf, "csrf cookie must be issued when none is present")
	require.NotEmpty(t, csrf.Value)

	var out struct {
		CSRFToken string `json:"csrf_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, csrf.Value, out.CSRFToken, "body token must match cookie token")
}

func TestCSRF_ReusesExistingCookie(t *testing.T) {
	deps, cleanup := newHandlerDeps(t)
	defer cleanup()
	srv := newHandlerServer(t, deps)

	existing := &http.Cookie{Name: deps.CSRFCookieName, Value: "preexisting-token-xyz"}
	resp := getReq(t, srv.URL+"/csrf", existing)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		CSRFToken string `json:"csrf_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "preexisting-token-xyz", out.CSRFToken)
}

// Sanity-check: WithSession (exported test helper) allows direct handler
// drive when middleware isn't in the chain.
func TestWithSession_HelperRoundTrip(t *testing.T) {
	ctx := auth.WithSession(context.Background(), auth.SessionInfo{
		SessionID:   "sid",
		AdminUserID: 7,
		Username:    "alice",
	})
	si, ok := auth.FromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "alice", si.Username)
}
