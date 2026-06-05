package metrics

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// Store persists and queries metric samples.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// Insert writes one sample row per (metric, value) at collectedAt.
func (s *Store) Insert(ctx context.Context, collectedAt time.Time, values map[string]float64) error {
	ts := collectedAt.UTC().Format(time.RFC3339Nano)
	rows := make([]metricsSample, 0, len(values))
	for metric, v := range values {
		rows = append(rows, metricsSample{ID: newULID(), CollectedAt: ts, Metric: metric, Value: v})
	}
	if len(rows) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("metrics.Insert: %w", err)
	}
	return nil
}

// Query returns all samples for the given metrics at or after cutoff,
// grouped by metric and ordered by collected_at ascending.
func (s *Store) Query(ctx context.Context, metrics []string, cutoff time.Time) (map[string][]Point, error) {
	var rows []metricsSample
	if err := s.db.WithContext(ctx).
		Where("metric IN ? AND collected_at >= ?", metrics, cutoff.UTC().Format(time.RFC3339Nano)).
		Order("collected_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("metrics.Query: %w", err)
	}
	out := map[string][]Point{}
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339Nano, r.CollectedAt)
		if err != nil {
			continue
		}
		out[r.Metric] = append(out[r.Metric], Point{T: t, V: r.Value})
	}
	return out, nil
}

// RetentionSweep deletes samples older than cutoff and returns the count.
func (s *Store) RetentionSweep(cutoff time.Time) (int64, error) {
	res := s.db.Where("collected_at < ?", cutoff.UTC().Format(time.RFC3339Nano)).Delete(&metricsSample{})
	if res.Error != nil {
		return 0, fmt.Errorf("metrics.RetentionSweep: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// newULID returns a new monotonic ULID string (mirror audit.newULID).
func newULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
