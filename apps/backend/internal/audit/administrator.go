package audit

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// insert persists a single audit event entity to the database.
// The caller is responsible for ensuring the payload has been sanitised before
// the entity was constructed.
func insert(db *gorm.DB, e Event) error {
	entity := Make(e)
	if result := db.Create(&entity); result.Error != nil {
		return fmt.Errorf("audit insert: %w", result.Error)
	}
	return nil
}

// deleteOlderThan removes all audit events with occurred_at before cutoff
// (exclusive) and returns the number of rows deleted.
func deleteOlderThan(db *gorm.DB, cutoff time.Time) (int64, error) {
	result := db.
		Where("occurred_at < ?", cutoff.UTC().Format(time.RFC3339Nano)).
		Delete(&auditEvent{})
	if result.Error != nil {
		return 0, fmt.Errorf("audit deleteOlderThan: %w", result.Error)
	}
	return result.RowsAffected, nil
}
