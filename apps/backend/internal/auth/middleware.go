package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// ctxKey is a package-private type used as the request-context key so that
// SessionInfo cannot be retrieved (or planted) by code outside this package.
type ctxKey int

const (
	ctxSession ctxKey = iota
)

// SessionInfo is the read-only snapshot of an authenticated request attached
// to the request context by RequireSession.
type SessionInfo struct {
	SessionID   string
	AdminUserID uint
	Username    string
	SourceIP    string
}

// FromContext extracts SessionInfo from ctx. Returns false if no session is
// attached (e.g. RequireSession did not run for this request).
func FromContext(ctx context.Context) (SessionInfo, bool) {
	si, ok := ctx.Value(ctxSession).(SessionInfo)
	return si, ok
}

// WithSession returns a copy of ctx that carries si. Exported for test code
// that wants to drive handlers directly without mounting RequireSession.
func WithSession(ctx context.Context, si SessionInfo) context.Context {
	return context.WithValue(ctx, ctxSession, si)
}

// ErrNoSession is returned by helpers that require an authenticated context.
var ErrNoSession = errors.New("auth: no session in context")

// RequireSession reads the session cookie, looks it up via the Processor, and
// rejects the request with 401 when the cookie is missing, the session is
// unknown, or the session has expired. On success it attaches a SessionInfo
// to the request context.
func RequireSession(cookieName string, p *Processor, style apierror.Style) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			sess, user, err := p.Me(r.Context(), c.Value)
			if err != nil {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			// Belt and braces: Processor.Me already rejects expired rows, but
			// the contract here is "401 on expiry" so re-check the timestamp
			// directly to keep the middleware self-contained.
			if !sess.ExpiresAt().After(time.Now().UTC()) {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			ctx := WithSession(r.Context(), SessionInfo{
				SessionID:   sess.ID(),
				AdminUserID: user.ID(),
				Username:    user.Username(),
				SourceIP:    sess.SourceIP(),
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireCSRF enforces the double-submit token pattern on unsafe methods.
// GET/HEAD/OPTIONS requests pass through unchecked; all other methods must
// present matching cookie + X-CSRF-Token header values.
func RequireCSRF(cookieName string, style apierror.Style) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				apierror.Write(w, style, apierror.CSRFInvalid())
				return
			}
			h := r.Header.Get("X-CSRF-Token")
			if h == "" || h != c.Value {
				apierror.Write(w, style, apierror.CSRFInvalid())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
