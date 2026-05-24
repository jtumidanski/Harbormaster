package auth

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// usernameRegex enforces lowercase identifiers safe for URLs and logs.
var usernameRegex = regexp.MustCompile(`^[a-z0-9._-]+$`)

const (
	usernameMinLen = 3
	usernameMaxLen = 64
)

// AdminUserBuilder constructs validated AdminUser instances.
type AdminUserBuilder struct {
	id           uint
	username     string
	passwordHash string
	createdAt    time.Time
	updatedAt    time.Time
	disabledAt   *time.Time
}

// NewAdminUserBuilder returns an empty builder. All setters return the
// builder for fluent chaining; validation happens in Build().
func NewAdminUserBuilder() *AdminUserBuilder { return &AdminUserBuilder{} }

// ID sets the primary key (typically only after a successful insert).
func (b *AdminUserBuilder) ID(id uint) *AdminUserBuilder { b.id = id; return b }

// Username sets the username. Validation happens in Build.
func (b *AdminUserBuilder) Username(s string) *AdminUserBuilder { b.username = s; return b }

// PasswordHash sets the argon2id PHC-encoded hash.
func (b *AdminUserBuilder) PasswordHash(h string) *AdminUserBuilder { b.passwordHash = h; return b }

// CreatedAt sets the creation timestamp.
func (b *AdminUserBuilder) CreatedAt(t time.Time) *AdminUserBuilder { b.createdAt = t; return b }

// UpdatedAt sets the last-update timestamp.
func (b *AdminUserBuilder) UpdatedAt(t time.Time) *AdminUserBuilder { b.updatedAt = t; return b }

// DisabledAt marks the user as disabled at the given time. Pass nil to clear.
func (b *AdminUserBuilder) DisabledAt(t *time.Time) *AdminUserBuilder {
	if t == nil {
		b.disabledAt = nil
		return b
	}
	tt := *t
	b.disabledAt = &tt
	return b
}

// Build validates inputs and returns an immutable AdminUser.
func (b *AdminUserBuilder) Build() (AdminUser, error) {
	if l := len(b.username); l < usernameMinLen || l > usernameMaxLen {
		return AdminUser{}, fmt.Errorf("username length %d outside [%d, %d]",
			l, usernameMinLen, usernameMaxLen)
	}
	if !usernameRegex.MatchString(b.username) {
		return AdminUser{}, fmt.Errorf("username %q does not match %s",
			b.username, usernameRegex.String())
	}
	if b.passwordHash == "" {
		return AdminUser{}, errors.New("password hash is required")
	}
	if b.createdAt.IsZero() {
		b.createdAt = time.Now().UTC()
	}
	if b.updatedAt.IsZero() {
		b.updatedAt = b.createdAt
	}
	return AdminUser{
		id:           b.id,
		username:     b.username,
		passwordHash: b.passwordHash,
		createdAt:    b.createdAt.UTC(),
		updatedAt:    b.updatedAt.UTC(),
		disabledAt:   b.disabledAt,
	}, nil
}

// SessionBuilder constructs validated Session instances.
type SessionBuilder struct {
	id           string
	adminUserID  uint
	createdAt    time.Time
	expiresAt    time.Time
	lastActiveAt time.Time
	sourceIP     string
	userAgent    string
}

// NewSessionBuilder returns an empty builder.
func NewSessionBuilder() *SessionBuilder { return &SessionBuilder{} }

// ID sets the session identifier (typically a ULID).
func (b *SessionBuilder) ID(id string) *SessionBuilder { b.id = id; return b }

// AdminUserID sets the FK into admin_users.
func (b *SessionBuilder) AdminUserID(id uint) *SessionBuilder { b.adminUserID = id; return b }

// CreatedAt sets the creation timestamp.
func (b *SessionBuilder) CreatedAt(t time.Time) *SessionBuilder { b.createdAt = t; return b }

// ExpiresAt sets the absolute expiry timestamp.
func (b *SessionBuilder) ExpiresAt(t time.Time) *SessionBuilder { b.expiresAt = t; return b }

// LastActiveAt sets the most recent observed activity.
func (b *SessionBuilder) LastActiveAt(t time.Time) *SessionBuilder { b.lastActiveAt = t; return b }

// SourceIP sets the originating IP.
func (b *SessionBuilder) SourceIP(ip string) *SessionBuilder { b.sourceIP = ip; return b }

// UserAgent sets the User-Agent string.
func (b *SessionBuilder) UserAgent(ua string) *SessionBuilder { b.userAgent = ua; return b }

// Build validates inputs and returns an immutable Session.
func (b *SessionBuilder) Build() (Session, error) {
	if b.id == "" {
		return Session{}, errors.New("session id is required")
	}
	if b.adminUserID == 0 {
		return Session{}, errors.New("admin_user_id is required")
	}
	if b.createdAt.IsZero() {
		b.createdAt = time.Now().UTC()
	}
	if b.expiresAt.IsZero() {
		return Session{}, errors.New("expires_at is required")
	}
	if !b.expiresAt.After(b.createdAt) {
		return Session{}, errors.New("expires_at must be after created_at")
	}
	if b.lastActiveAt.IsZero() {
		b.lastActiveAt = b.createdAt
	}
	return Session{
		id:           b.id,
		adminUserID:  b.adminUserID,
		createdAt:    b.createdAt.UTC(),
		expiresAt:    b.expiresAt.UTC(),
		lastActiveAt: b.lastActiveAt.UTC(),
		sourceIP:     b.sourceIP,
		userAgent:    b.userAgent,
	}, nil
}
