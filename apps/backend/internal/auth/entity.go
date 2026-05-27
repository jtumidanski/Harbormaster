package auth

import "time"

// adminUserEntity is the GORM persistence struct for admin_users.
// Timestamps are stored as ISO-8601 strings per the project's invariants.
type adminUserEntity struct {
	ID           uint   `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string `gorm:"column:username;uniqueIndex;not null"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	CreatedAt    string `gorm:"column:created_at;not null"`
	UpdatedAt    string `gorm:"column:updated_at;not null"`
	DisabledAt   *string `gorm:"column:disabled_at"`
}

// TableName satisfies gorm.Tabler.
func (adminUserEntity) TableName() string { return "admin_users" }

// MakeAdminUser converts a persistence entity into a domain AdminUser.
// Times are parsed from RFC 3339 nano strings; malformed values produce a
// zero-valued time.
func MakeAdminUser(e adminUserEntity) (AdminUser, error) {
	created, _ := time.Parse(time.RFC3339Nano, e.CreatedAt)
	updated, _ := time.Parse(time.RFC3339Nano, e.UpdatedAt)
	var disabled *time.Time
	if e.DisabledAt != nil && *e.DisabledAt != "" {
		t, _ := time.Parse(time.RFC3339Nano, *e.DisabledAt)
		tt := t.UTC()
		disabled = &tt
	}
	return AdminUser{
		id:           e.ID,
		username:     e.Username,
		passwordHash: e.PasswordHash,
		createdAt:    created.UTC(),
		updatedAt:    updated.UTC(),
		disabledAt:   disabled,
	}, nil
}

// ToEntity converts a domain AdminUser back into a persistence entity.
func (u AdminUser) ToEntity() adminUserEntity {
	var disabled *string
	if u.disabledAt != nil {
		s := u.disabledAt.UTC().Format(time.RFC3339Nano)
		disabled = &s
	}
	return adminUserEntity{
		ID:           u.id,
		Username:     u.username,
		PasswordHash: u.passwordHash,
		CreatedAt:    u.createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:    u.updatedAt.UTC().Format(time.RFC3339Nano),
		DisabledAt:   disabled,
	}
}

// sessionEntity is the GORM persistence struct for sessions.
type sessionEntity struct {
	ID           string `gorm:"column:id;primaryKey"`
	AdminUserID  uint   `gorm:"column:admin_user_id;not null;index"`
	CreatedAt    string `gorm:"column:created_at;not null"`
	ExpiresAt    string `gorm:"column:expires_at;not null;index"`
	LastActiveAt string `gorm:"column:last_active_at;not null"`
	SourceIP     string `gorm:"column:source_ip"`
	UserAgent    string `gorm:"column:user_agent"`
}

// TableName satisfies gorm.Tabler.
func (sessionEntity) TableName() string { return "sessions" }

// MakeSession converts a persistence entity into a domain Session.
func MakeSession(e sessionEntity) (Session, error) {
	created, _ := time.Parse(time.RFC3339Nano, e.CreatedAt)
	expires, _ := time.Parse(time.RFC3339Nano, e.ExpiresAt)
	lastActive, _ := time.Parse(time.RFC3339Nano, e.LastActiveAt)
	return Session{
		id:           e.ID,
		adminUserID:  e.AdminUserID,
		createdAt:    created.UTC(),
		expiresAt:    expires.UTC(),
		lastActiveAt: lastActive.UTC(),
		sourceIP:     e.SourceIP,
		userAgent:    e.UserAgent,
	}, nil
}

// ToEntity converts a domain Session back into a persistence entity.
func (s Session) ToEntity() sessionEntity {
	return sessionEntity{
		ID:           s.id,
		AdminUserID:  s.adminUserID,
		CreatedAt:    s.createdAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:    s.expiresAt.UTC().Format(time.RFC3339Nano),
		LastActiveAt: s.lastActiveAt.UTC().Format(time.RFC3339Nano),
		SourceIP:     s.sourceIP,
		UserAgent:    s.userAgent,
	}
}
