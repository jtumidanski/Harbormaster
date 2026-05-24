package audit

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// maxPageSize is the upper bound applied to Page.Size when the audit
// list endpoint translates a query string into a Page literal. A
// homelab UI never needs more than 200 rows per page; the cap also
// protects the SQLite ORM from accidental large scans.
const maxPageSize = 200

// recent returns the most-recent limit events ordered occurred_at DESC.
func recent(db *gorm.DB, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	var entities []auditEvent
	if err := db.Model(&auditEvent{}).
		Order("occurred_at DESC").
		Limit(limit).
		Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("audit.recent: %w", err)
	}
	out := make([]Event, len(entities))
	for i, ae := range entities {
		out[i] = ae.ToEvent()
	}
	return out, nil
}

// failuresSince returns the total count of failure outcomes since cutoff
// (inclusive) plus the most-recent limit failure entries. The count
// reflects the full set inside the window so the dashboard can render a
// "N failures in the last 7d" headline alongside the truncated entries.
func failuresSince(db *gorm.DB, cutoff time.Time, limit int) (int64, []Event, error) {
	cutoffStr := cutoff.UTC().Format(time.RFC3339Nano)
	var count int64
	if err := db.Model(&auditEvent{}).
		Where("outcome = ? AND occurred_at >= ?", OutcomeFailure, cutoffStr).
		Count(&count).Error; err != nil {
		return 0, nil, fmt.Errorf("audit.failuresSince count: %w", err)
	}
	if count == 0 || limit <= 0 {
		return count, nil, nil
	}
	var entities []auditEvent
	if err := db.Model(&auditEvent{}).
		Where("outcome = ? AND occurred_at >= ?", OutcomeFailure, cutoffStr).
		Order("occurred_at DESC").
		Limit(limit).
		Find(&entities).Error; err != nil {
		return 0, nil, fmt.Errorf("audit.failuresSince list: %w", err)
	}
	out := make([]Event, len(entities))
	for i, ae := range entities {
		out[i] = ae.ToEvent()
	}
	return count, out, nil
}

// listFiltered returns a filtered + paginated slice of events plus the
// unfiltered total matching the filter (used for page-meta).
func listFiltered(db *gorm.DB, f Filter, page Page) ([]Event, int64, error) {
	if page.Number < 1 {
		page.Number = 1
	}
	if page.Size < 1 {
		page.Size = 50
	}
	if page.Size > maxPageSize {
		page.Size = maxPageSize
	}

	q := db.Model(&auditEvent{})
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
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if !f.From.IsZero() {
		q = q.Where("occurred_at >= ?", f.From.UTC().Format(time.RFC3339Nano))
	}
	if !f.To.IsZero() {
		q = q.Where("occurred_at <= ?", f.To.UTC().Format(time.RFC3339Nano))
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("audit.listFiltered count: %w", err)
	}

	var entities []auditEvent
	if err := q.Order("occurred_at DESC").
		Offset((page.Number - 1) * page.Size).
		Limit(page.Size).
		Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("audit.listFiltered: %w", err)
	}
	out := make([]Event, len(entities))
	for i, ae := range entities {
		out[i] = ae.ToEvent()
	}
	return out, total, nil
}

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
