package auth

import (
	"fmt"

	"gorm.io/gorm"
)

// getAdminUserByUsername returns a curried lookup over username.
// The returned function expects a context-scoped *gorm.DB from the processor.
func getAdminUserByUsername(username string) func(*gorm.DB) (AdminUser, error) {
	return func(db *gorm.DB) (AdminUser, error) {
		var e adminUserEntity
		if err := db.Where("username = ?", username).First(&e).Error; err != nil {
			return AdminUser{}, fmt.Errorf("auth.getAdminUserByUsername(%q): %w", username, err)
		}
		return MakeAdminUser(e)
	}
}

// getAdminUserByID returns a curried lookup over the primary key.
func getAdminUserByID(id uint) func(*gorm.DB) (AdminUser, error) {
	return func(db *gorm.DB) (AdminUser, error) {
		var e adminUserEntity
		if err := db.Where("id = ?", id).First(&e).Error; err != nil {
			return AdminUser{}, fmt.Errorf("auth.getAdminUserByID(%d): %w", id, err)
		}
		return MakeAdminUser(e)
	}
}

// getSessionByID returns a curried lookup over the session ULID.
func getSessionByID(id string) func(*gorm.DB) (Session, error) {
	return func(db *gorm.DB) (Session, error) {
		var e sessionEntity
		if err := db.Where("id = ?", id).First(&e).Error; err != nil {
			return Session{}, fmt.Errorf("auth.getSessionByID(%q): %w", id, err)
		}
		return MakeSession(e)
	}
}

// getActiveSessionsByAdmin lists every session owned by the given admin user
// regardless of expiry. The sweeper handles expiry removal separately.
func getActiveSessionsByAdmin(adminUserID uint) func(*gorm.DB) ([]Session, error) {
	return func(db *gorm.DB) ([]Session, error) {
		var entities []sessionEntity
		if err := db.Where("admin_user_id = ?", adminUserID).
			Order("created_at DESC").
			Find(&entities).Error; err != nil {
			return nil, fmt.Errorf("auth.getActiveSessionsByAdmin(%d): %w", adminUserID, err)
		}
		out := make([]Session, len(entities))
		for i, e := range entities {
			m, err := MakeSession(e)
			if err != nil {
				return nil, err
			}
			out[i] = m
		}
		return out, nil
	}
}
