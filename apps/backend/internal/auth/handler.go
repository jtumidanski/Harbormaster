package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// HandlerDeps wires the auth handlers with a processor, rate limiter, cookie
// configuration, and session lifetime. Construct one in the composition root
// (cmd/server) and pass it to Routes.
type HandlerDeps struct {
	Processor         *Processor
	RateLimiter       *LoginRateLimiter
	SessionCookieName string
	CSRFCookieName    string
	BasePath          string
	SessionTimeout    time.Duration
	Secure            bool
}

// Routes returns a chi sub-router function that mounts /auth/* and /csrf
// under whatever parent path the caller chooses. Callers are responsible for
// mounting RequireSession / RequireCSRF in front of the routes that need
// them (login/csrf are public; me/logout/password require a session).
//
// In production, prefer PublicRoutes() and ProtectedRoutes() so the
// composition root can apply RequireSession + RequireCSRF only to the
// protected subset. Routes() is retained for tests that mount a single
// registrar and filter middleware per-path.
func Routes(d HandlerDeps) func(chi.Router) {
	return func(r chi.Router) {
		d.PublicRoutes()(r)
		d.ProtectedRoutes()(r)
	}
}

// PublicRoutes mounts the endpoints that must be reachable without a
// session: POST /auth/login and GET /csrf. The composition root should NOT
// put RequireSession in front of these — login bootstraps the cookie, and
// /csrf is needed by the SPA before any credentials are entered.
func (d HandlerDeps) PublicRoutes() func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/auth/login", d.login)
		r.Get("/csrf", d.issueCSRF)
	}
}

// ProtectedRoutes mounts the endpoints that require a live session:
// POST /auth/logout, GET /auth/me, POST /auth/password. The composition
// root is responsible for wrapping these in RequireSession + RequireCSRF.
func (d HandlerDeps) ProtectedRoutes() func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/auth/logout", d.logout)
		r.Get("/auth/me", d.me)
		r.Post("/auth/password", d.changePassword)
	}
}

// login validates credentials, applies the per-IP rate limiter, and on
// success issues both the session and CSRF cookies. Returns 204 No Content
// — the session id is delivered exclusively in the HttpOnly cookie.
func (d HandlerDeps) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	ip := remoteIP(r)
	now := time.Now().UTC()
	if !d.RateLimiter.Allow(ip, now) {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusTooManyRequests,
			"too_many_attempts", "Too many failed attempts; try again in 5 minutes"))
		return
	}
	sid, csrf, err := d.Processor.Login(r.Context(), body.Username, body.Password, ip, r.UserAgent())
	if err != nil {
		d.RateLimiter.RecordFailure(ip, now)
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnauthorized,
			"invalid_credentials", "Invalid username or password"))
		return
	}
	d.RateLimiter.Reset(ip)
	d.setSessionCookie(w, sid)
	d.setCSRFCookie(w, csrf)
	w.WriteHeader(http.StatusNoContent)
}

// logout deletes the session and clears both cookies. Requires that the
// RequireSession middleware has run upstream so SessionInfo is in context.
func (d HandlerDeps) logout(w http.ResponseWriter, r *http.Request) {
	si, ok := FromContext(r.Context())
	if !ok {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	if err := d.Processor.Logout(r.Context(), si.SessionID); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal("logout failed"))
		return
	}
	d.clearCookie(w, d.SessionCookieName)
	d.clearCookie(w, d.CSRFCookieName)
	w.WriteHeader(http.StatusNoContent)
}

// me returns the caller's username and session expiry. Requires RequireSession.
func (d HandlerDeps) me(w http.ResponseWriter, r *http.Request) {
	si, ok := FromContext(r.Context())
	if !ok {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	sess, user, err := d.Processor.Me(r.Context(), si.SessionID)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	resp := struct {
		Username         string    `json:"username"`
		SessionExpiresAt time.Time `json:"session_expires_at"`
	}{
		Username:         user.Username(),
		SessionExpiresAt: sess.ExpiresAt(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// changePassword rotates the caller's password. Returns 204 on success.
// Requires RequireSession.
func (d HandlerDeps) changePassword(w http.ResponseWriter, r *http.Request) {
	si, ok := FromContext(r.Context())
	if !ok {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	if len(body.NewPassword) < 12 {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnprocessableEntity,
			"weak_password", "Password must be at least 12 characters"))
		return
	}
	if err := d.Processor.ChangePassword(r.Context(), si.SessionID, body.CurrentPassword, body.NewPassword); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnauthorized,
			"invalid_credentials", "Current password incorrect"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// issueCSRF mints (or reuses) the double-submit token cookie and echoes the
// value in the response body so SPAs can read it without parsing cookies.
// Safe to call without an authenticated session — it bootstraps the SPA.
func (d HandlerDeps) issueCSRF(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(d.CSRFCookieName); err != nil {
		token, err := NewCSRFToken()
		if err != nil {
			apierror.Write(w, apierror.StyleAction, apierror.Internal("failed to mint csrf token"))
			return
		}
		d.setCSRFCookie(w, token)
		resp := struct {
			CSRFToken string `json:"csrf_token"`
		}{CSRFToken: token}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	c, _ := r.Cookie(d.CSRFCookieName)
	resp := struct {
		CSRFToken string `json:"csrf_token"`
	}{CSRFToken: c.Value}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (d HandlerDeps) setSessionCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.SessionCookieName,
		Value:    sid,
		Path:     d.cookiePath(),
		Expires:  time.Now().UTC().Add(d.SessionTimeout),
		MaxAge:   int(d.SessionTimeout.Seconds()),
		HttpOnly: true,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (d HandlerDeps) setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.CSRFCookieName,
		Value:    token,
		Path:     d.cookiePath(),
		Expires:  time.Now().UTC().Add(d.SessionTimeout),
		MaxAge:   int(d.SessionTimeout.Seconds()),
		HttpOnly: false,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (d HandlerDeps) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     d.cookiePath(),
		MaxAge:   -1,
		HttpOnly: name == d.SessionCookieName,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// remoteIP returns the host portion of r.RemoteAddr, falling back to the
// raw value if it cannot be split (e.g. tests that set a bare IP).
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (d HandlerDeps) cookiePath() string {
	if d.BasePath == "" {
		return "/"
	}
	return d.BasePath
}
