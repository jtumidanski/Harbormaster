package auth

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// DefaultSessionTTL is the lifetime of a freshly issued session.
// T2.3 may override this via constructor option; for T2.1 the value is fixed.
const DefaultSessionTTL = 8 * time.Hour

// invalidCredentialsError is returned for both unknown-user and bad-password
// failures so that no oracle is leaked from the response.
func invalidCredentialsError() *apierror.Error {
	return apierror.New(http.StatusUnauthorized, "invalid_credentials",
		"Invalid username or password.")
}

// Processor coordinates the auth domain's reads, writes, and business rules.
// Construct via NewProcessor; do not zero-construct.
type Processor struct {
	db         *gorm.DB
	ttl        time.Duration
	now        func() time.Time
	newID      func() string
	entropyMu  sync.Mutex
	entropy    *ulid.MonotonicEntropy
	csrfTokens map[string]string // sessionID -> csrf token (in-memory; rebuilt at restart)
	csrfMu     sync.RWMutex
}

// NewProcessor returns a Processor backed by db.
func NewProcessor(db *gorm.DB) *Processor {
	p := &Processor{
		db:         db,
		ttl:        DefaultSessionTTL,
		now:        func() time.Time { return time.Now().UTC() },
		csrfTokens: map[string]string{},
	}
	// Seed monotonic ULID entropy. The math/rand.Source is fine here because
	// ULIDs are not security tokens; the CSRF token uses crypto/rand.
	p.entropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	p.newID = p.newULID
	return p
}

// DB exposes the underlying *gorm.DB (used by tests and middleware).
func (p *Processor) DB() *gorm.DB { return p.db }

// newULID returns a fresh monotonic ULID string. Synchronised on entropyMu
// because ulid.MonotonicEntropy is not safe for concurrent use.
func (p *Processor) newULID() string {
	p.entropyMu.Lock()
	defer p.entropyMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(p.now()), p.entropy).String()
}

// CSRFTokenFor returns the CSRF token bound to the given session, if any.
// T2.3 will replace this in-memory map with a persisted cookie-bound token.
func (p *Processor) CSRFTokenFor(sessionID string) (string, bool) {
	p.csrfMu.RLock()
	defer p.csrfMu.RUnlock()
	tok, ok := p.csrfTokens[sessionID]
	return tok, ok
}

func (p *Processor) bindCSRF(sessionID, token string) {
	p.csrfMu.Lock()
	defer p.csrfMu.Unlock()
	p.csrfTokens[sessionID] = token
}

func (p *Processor) clearCSRF(sessionID string) {
	p.csrfMu.Lock()
	defer p.csrfMu.Unlock()
	delete(p.csrfTokens, sessionID)
}

// Login validates credentials and, on success, persists a fresh session row.
// On any failure (unknown user, bad password, disabled account) it returns
// the same opaque "invalid_credentials" apierror to avoid a username oracle.
//
// Returns (sessionID, csrfToken, error). Both strings are empty on error.
func (p *Processor) Login(ctx context.Context, username, password, ip, ua string) (string, string, error) {
	db := p.db.WithContext(ctx)

	user, err := getAdminUserByUsername(username)(db)
	if err != nil {
		// Still spend cycles to make timing closer to the success path; we
		// don't bother running argon2 on a placeholder because that's the
		// dominant cost and the rate limiter (applied at the HTTP layer)
		// blunts the timing oracle far better than a constant-time stub.
		return "", "", invalidCredentialsError()
	}
	if user.DisabledAt() != nil {
		return "", "", invalidCredentialsError()
	}
	if err := VerifyPassword(user.PasswordHash(), password); err != nil {
		return "", "", invalidCredentialsError()
	}

	now := p.now()
	sess, err := NewSessionBuilder().
		ID(p.newID()).
		AdminUserID(user.ID()).
		CreatedAt(now).
		ExpiresAt(now.Add(p.ttl)).
		LastActiveAt(now).
		SourceIP(ip).
		UserAgent(ua).
		Build()
	if err != nil {
		return "", "", apierror.Internal("failed to build session")
	}
	if err := createSession(db, sess); err != nil {
		return "", "", apierror.Internal("failed to persist session")
	}
	token, err := NewCSRFToken()
	if err != nil {
		// Best-effort cleanup; if this fails we surface the original error.
		_ = deleteSession(db, sess.ID())
		return "", "", apierror.Internal("failed to issue csrf token")
	}
	p.bindCSRF(sess.ID(), token)
	return sess.ID(), token, nil
}

// Logout removes the session row and forgets the bound CSRF token.
// Logout is idempotent — deleting an absent session is not an error.
func (p *Processor) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := deleteSession(p.db.WithContext(ctx), sessionID); err != nil {
		return err
	}
	p.clearCSRF(sessionID)
	return nil
}

// Me resolves a session ID into its (Session, AdminUser) pair. Returns an
// Unauthenticated apierror if the session is missing or expired.
func (p *Processor) Me(ctx context.Context, sessionID string) (Session, AdminUser, error) {
	if sessionID == "" {
		return Session{}, AdminUser{}, apierror.Unauthenticated()
	}
	db := p.db.WithContext(ctx)
	sess, err := getSessionByID(sessionID)(db)
	if err != nil {
		return Session{}, AdminUser{}, apierror.Unauthenticated()
	}
	if !sess.ExpiresAt().After(p.now()) {
		// Drop the expired row eagerly; the sweeper would catch it eventually.
		_ = deleteSession(db, sessionID)
		p.clearCSRF(sessionID)
		return Session{}, AdminUser{}, apierror.Unauthenticated()
	}
	user, err := getAdminUserByID(sess.AdminUserID())(db)
	if err != nil {
		return Session{}, AdminUser{}, apierror.Unauthenticated()
	}
	return sess, user, nil
}

// ChangePassword verifies the current password and writes a new argon2id hash.
// Returns an apierror with code "invalid_credentials" if the current password
// is wrong; that mirrors the login contract.
func (p *Processor) ChangePassword(ctx context.Context, sessionID, current, next string) error {
	if next == "" {
		return apierror.New(http.StatusBadRequest, "invalid_password",
			"New password must not be empty.")
	}
	db := p.db.WithContext(ctx)
	sess, user, err := p.Me(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := VerifyPassword(user.PasswordHash(), current); err != nil {
		return invalidCredentialsError()
	}
	hash, err := HashPassword(next)
	if err != nil {
		return apierror.Internal("failed to hash password")
	}
	if err := updateAdminUserPassword(db, user.ID(), hash); err != nil {
		return apierror.Internal("failed to update password")
	}
	// Keep the current session valid; cookie/CSRF rotation is T2.3 territory.
	_ = sess
	return nil
}

// SweepExpired removes every session row past its expiry.
func (p *Processor) SweepExpired(ctx context.Context) error {
	if _, err := deleteExpiredSessions(p.db.WithContext(ctx), p.now()); err != nil {
		return err
	}
	return nil
}

// IsInvalidCredentials reports whether err is the public credentials error.
// Handy in tests and rare places where the HTTP layer needs to branch.
func IsInvalidCredentials(err error) bool {
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == "invalid_credentials"
}
