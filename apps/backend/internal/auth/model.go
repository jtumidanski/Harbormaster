package auth

import "time"

// AdminUser is the immutable domain representation of a row in admin_users.
// All fields are private; callers must read state via accessors and mutate
// only through builders that return new instances.
type AdminUser struct {
	id           uint
	username     string
	passwordHash string
	createdAt    time.Time
	updatedAt    time.Time
	disabledAt   *time.Time
}

// ID returns the database primary key.
func (u AdminUser) ID() uint { return u.id }

// Username returns the lowercase username.
func (u AdminUser) Username() string { return u.username }

// PasswordHash returns the argon2id PHC-encoded hash.
func (u AdminUser) PasswordHash() string { return u.passwordHash }

// CreatedAt returns when the user was created.
func (u AdminUser) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns when the user was last updated.
func (u AdminUser) UpdatedAt() time.Time { return u.updatedAt }

// DisabledAt returns the disable timestamp, or nil if the user is active.
func (u AdminUser) DisabledAt() *time.Time {
	if u.disabledAt == nil {
		return nil
	}
	t := *u.disabledAt
	return &t
}

// Session is the immutable domain representation of a row in sessions.
type Session struct {
	id           string
	adminUserID  uint
	createdAt    time.Time
	expiresAt    time.Time
	lastActiveAt time.Time
	sourceIP     string
	userAgent    string
}

// ID returns the session ULID.
func (s Session) ID() string { return s.id }

// AdminUserID returns the FK into admin_users.
func (s Session) AdminUserID() uint { return s.adminUserID }

// CreatedAt returns when the session was created.
func (s Session) CreatedAt() time.Time { return s.createdAt }

// ExpiresAt returns the session's absolute expiry.
func (s Session) ExpiresAt() time.Time { return s.expiresAt }

// LastActiveAt returns the most recent observed activity.
func (s Session) LastActiveAt() time.Time { return s.lastActiveAt }

// SourceIP returns the originating IP captured at login.
func (s Session) SourceIP() string { return s.sourceIP }

// UserAgent returns the User-Agent header captured at login.
func (s Session) UserAgent() string { return s.userAgent }
