package audit

import (
	"fmt"

	"gorm.io/gorm"
)

// GetByID fetches a single Event by its ULID primary key.
func GetByID(db *gorm.DB, id string) (Event, error) {
	var entity auditEvent
	result := db.Where("id = ?", id).First(&entity)
	if result.Error != nil {
		return Event{}, fmt.Errorf("audit.GetByID(%s): %w", id, result.Error)
	}
	return entity.ToEvent(), nil
}

// List returns events matching the given Filter, ordered by occurred_at DESC.
// M5 will exercise pagination; M1 just needs the type to compile.
func List(db *gorm.DB, f Filter) ([]Event, error) {
	q := db.Model(&auditEvent{}).Order("occurred_at DESC")

	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		q = q.Where("target_id = ?", f.TargetID)
	}
	if f.Actor != "" {
		q = q.Where("actor = ?", f.Actor)
	}
	if !f.Since.IsZero() {
		q = q.Where("occurred_at >= ?", f.Since)
	}
	if !f.Until.IsZero() {
		q = q.Where("occurred_at <= ?", f.Until)
	}
	if f.PageSize > 0 {
		q = q.Limit(f.PageSize).Offset(f.PageOffset)
	}

	var entities []auditEvent
	if err := q.Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("audit.List: %w", err)
	}

	events := make([]Event, len(entities))
	for i, ae := range entities {
		events[i] = ae.ToEvent()
	}
	return events, nil
}
