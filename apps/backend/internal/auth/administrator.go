package auth

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// createAdminUser persists a new admin user and returns the populated entity
// (including the auto-incremented ID).
func createAdminUser(db *gorm.DB, u AdminUser) (adminUserEntity, error) {
	e := u.ToEntity()
	if err := db.Create(&e).Error; err != nil {
		return adminUserEntity{}, fmt.Errorf("auth.createAdminUser: %w", err)
	}
	return e, nil
}

// updateAdminUserPassword writes a new password hash and bumps updated_at.
func updateAdminUserPassword(db *gorm.DB, id uint, hash string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res := db.Model(&adminUserEntity{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password_hash": hash,
			"updated_at":    now,
		})
	if res.Error != nil {
		return fmt.Errorf("auth.updateAdminUserPassword(%d): %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("auth.updateAdminUserPassword(%d): no rows affected", id)
	}
	return nil
}

// createSession persists a new session row.
func createSession(db *gorm.DB, s Session) error {
	e := s.ToEntity()
	if err := db.Create(&e).Error; err != nil {
		return fmt.Errorf("auth.createSession: %w", err)
	}
	return nil
}

// deleteSession removes a single session row by ULID. Deleting a missing row
// is not an error — logout is idempotent.
func deleteSession(db *gorm.DB, id string) error {
	if err := db.Where("id = ?", id).Delete(&sessionEntity{}).Error; err != nil {
		return fmt.Errorf("auth.deleteSession(%q): %w", id, err)
	}
	return nil
}

// deleteExpiredSessions removes every session with expires_at strictly before
// cutoff and returns the count purged.
func deleteExpiredSessions(db *gorm.DB, cutoff time.Time) (int64, error) {
	res := db.Where("expires_at < ?", cutoff.UTC().Format(time.RFC3339Nano)).
		Delete(&sessionEntity{})
	if res.Error != nil {
		return 0, fmt.Errorf("auth.deleteExpiredSessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// rotateSession deletes oldID and persists newSess inside a single
// transaction. Used after a successful re-login to invalidate the prior
// cookie before the new one is issued.
func rotateSession(db *gorm.DB, oldID string, newSess Session) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", oldID).Delete(&sessionEntity{}).Error; err != nil {
			return fmt.Errorf("auth.rotateSession.delete: %w", err)
		}
		e := newSess.ToEntity()
		if err := tx.Create(&e).Error; err != nil {
			return fmt.Errorf("auth.rotateSession.create: %w", err)
		}
		return nil
	})
}

// touchSession updates last_active_at for a session.
func touchSession(db *gorm.DB, id string, when time.Time) error {
	res := db.Model(&sessionEntity{}).
		Where("id = ?", id).
		Update("last_active_at", when.UTC().Format(time.RFC3339Nano))
	if res.Error != nil {
		return fmt.Errorf("auth.touchSession(%q): %w", id, res.Error)
	}
	return nil
}
