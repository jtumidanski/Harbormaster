// Package bucketempty implements the persistent, single-flight, SSE-friendly
// empty-bucket worker. The repo layer translates between the GORM persistence
// row and the in-memory Job type, and enforces single-flight per bucket via
// the partial unique index defined in migration 0006.
package bucketempty

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// State values stored in bucket_empty_jobs.state.
const (
	StateRunning = "running"
	StateDone    = "done"
	StateError   = "error"
)

// ErrAlreadyRunning is returned by InsertRunning when the partial unique index
// rejects a second active row for the same bucket. Callers should surface this
// as a 409 Conflict at the HTTP layer.
var ErrAlreadyRunning = errors.New("bucketempty: a job is already running for this bucket")

// Job is the in-memory representation of one row in bucket_empty_jobs.
// It is the unit of exchange between the repo and service layers.
type Job struct {
	ID             string     `gorm:"column:id;primaryKey"`
	Bucket         string     `gorm:"column:bucket_name;not null"`
	StartedAt      time.Time  `gorm:"-"`
	LastProgressAt time.Time  `gorm:"-"`
	DeletedCount   int64      `gorm:"column:deleted_count;not null"`
	EstimatedTotal *int64     `gorm:"column:estimated_total"`
	State          string     `gorm:"column:state;not null"`
	Error          string     `gorm:"column:error_message"`
	FinishedAt     *time.Time `gorm:"-"`
	PurgeVersions  bool       `gorm:"-"`
}

// TableName satisfies gorm.Tabler.
func (Job) TableName() string { return "bucket_empty_jobs" }

// jobRow is the raw persistence form. Times are stored as RFC3339Nano strings
// (matching the convention used by audit_events) so SQLite remains comparable
// and human-readable. purge_versions is stored as 0/1 per the migration.
type jobRow struct {
	ID              string  `gorm:"column:id;primaryKey"`
	BucketName      string  `gorm:"column:bucket_name;not null"`
	StartedAt       string  `gorm:"column:started_at;not null"`
	LastProgressAt  string  `gorm:"column:last_progress_at;not null"`
	DeletedCount    int64   `gorm:"column:deleted_count;not null"`
	EstimatedTotal  *int64  `gorm:"column:estimated_total"`
	State           string  `gorm:"column:state;not null"`
	ErrorMessage    *string `gorm:"column:error_message"`
	FinishedAt      *string `gorm:"column:finished_at"`
	PurgeVersionsIN int     `gorm:"column:purge_versions;not null"`
}

func (jobRow) TableName() string { return "bucket_empty_jobs" }

func (r jobRow) toJob() Job {
	started, _ := time.Parse(time.RFC3339Nano, r.StartedAt)
	progress, _ := time.Parse(time.RFC3339Nano, r.LastProgressAt)
	j := Job{
		ID:             r.ID,
		Bucket:         r.BucketName,
		StartedAt:      started.UTC(),
		LastProgressAt: progress.UTC(),
		DeletedCount:   r.DeletedCount,
		EstimatedTotal: r.EstimatedTotal,
		State:          r.State,
		PurgeVersions:  r.PurgeVersionsIN == 1,
	}
	if r.ErrorMessage != nil {
		j.Error = *r.ErrorMessage
	}
	if r.FinishedAt != nil {
		t, _ := time.Parse(time.RFC3339Nano, *r.FinishedAt)
		tu := t.UTC()
		j.FinishedAt = &tu
	}
	return j
}

// newULID returns a fresh monotonically-sortable ULID string. The constructor
// matches the pattern used by audit.newULID so identifiers sort consistently
// across the database.
func newULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// InsertRunning inserts a new bucket_empty_jobs row with state='running'. The
// partial unique index bucket_empty_jobs_active_per_bucket rejects a duplicate
// active row for the same bucket; we translate that into ErrAlreadyRunning.
func InsertRunning(db *gorm.DB, bucket string, purgeVersions bool) (string, error) {
	id := newULID()
	now := time.Now().UTC()
	purge := 0
	if purgeVersions {
		purge = 1
	}
	row := jobRow{
		ID:              id,
		BucketName:      bucket,
		StartedAt:       fmtTime(now),
		LastProgressAt:  fmtTime(now),
		DeletedCount:    0,
		State:           StateRunning,
		PurgeVersionsIN: purge,
	}
	if err := db.Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return "", ErrAlreadyRunning
		}
		return "", fmt.Errorf("bucketempty: insert running: %w", err)
	}
	return id, nil
}

// UpdateProgress writes deleted_count and last_progress_at. estimatedTotal is
// stored when > 0, otherwise the column is left untouched. The migration
// schema permits NULL for estimated_total.
func UpdateProgress(db *gorm.DB, id string, deleted int64, estimatedTotal int64) error {
	updates := map[string]any{
		"deleted_count":    deleted,
		"last_progress_at": fmtTime(time.Now().UTC()),
	}
	if estimatedTotal > 0 {
		updates["estimated_total"] = estimatedTotal
	}
	res := db.Model(&jobRow{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("bucketempty: update progress: %w", res.Error)
	}
	return nil
}

// MarkDone sets state='done', finished_at=now, and stores the final count.
func MarkDone(db *gorm.DB, id string, deletedTotal int64) error {
	now := fmtTime(time.Now().UTC())
	res := db.Model(&jobRow{}).Where("id = ?", id).Updates(map[string]any{
		"state":         StateDone,
		"finished_at":   now,
		"deleted_count": deletedTotal,
	})
	if res.Error != nil {
		return fmt.Errorf("bucketempty: mark done: %w", res.Error)
	}
	return nil
}

// MarkError sets state='error', finished_at=now, error_message=msg.
func MarkError(db *gorm.DB, id, msg string) error {
	now := fmtTime(time.Now().UTC())
	res := db.Model(&jobRow{}).Where("id = ?", id).Updates(map[string]any{
		"state":         StateError,
		"finished_at":   now,
		"error_message": msg,
	})
	if res.Error != nil {
		return fmt.Errorf("bucketempty: mark error: %w", res.Error)
	}
	return nil
}

// FindRunning returns the unique active row for bucket. When no active row
// exists it returns gorm.ErrRecordNotFound so callers can match with
// errors.Is.
func FindRunning(db *gorm.DB, bucket string) (Job, error) {
	var row jobRow
	res := db.Where("bucket_name = ? AND state = ?", bucket, StateRunning).First(&row)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return Job{}, gorm.ErrRecordNotFound
		}
		return Job{}, fmt.Errorf("bucketempty: find running: %w", res.Error)
	}
	return row.toJob(), nil
}

// OrphanedErrorMessage is the canonical text stored on rows flipped by
// OrphanRunningAtStartup. Tests pattern-match against it.
const OrphanedErrorMessage = "orphaned by restart"

// OrphanRunningAtStartup flips every state='running' row to state='error'
// with error_message set to OrphanedErrorMessage. For each orphaned row it
// emits a bucket.empty failure event via audit. Called once at process boot
// before the server begins accepting requests; without this, a crash leaves
// the partial unique index permanently blocking new empty-bucket jobs.
func OrphanRunningAtStartup(db *gorm.DB, recorder AuditRecorder) ([]Job, error) {
	var rows []jobRow
	if err := db.Where("state = ?", StateRunning).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("bucketempty: scan running: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	now := fmtTime(time.Now().UTC())
	orphaned := make([]Job, 0, len(rows))
	for _, r := range rows {
		res := db.Model(&jobRow{}).Where("id = ?", r.ID).Updates(map[string]any{
			"state":         StateError,
			"finished_at":   now,
			"error_message": OrphanedErrorMessage,
		})
		if res.Error != nil {
			return orphaned, fmt.Errorf("bucketempty: orphan %s: %w", r.ID, res.Error)
		}
		j := r.toJob()
		j.State = StateError
		j.Error = OrphanedErrorMessage
		orphaned = append(orphaned, j)
		if recorder != nil {
			payload := map[string]any{
				"job_id":         r.ID,
				"deleted_count":  r.DeletedCount,
				"purge_versions": r.PurgeVersionsIN == 1,
			}
			recorder.Record(context.Background(), ActionBucketEmpty, j.Bucket,
				OutcomeFailure, payload, OrphanedErrorMessage)
		}
	}
	return orphaned, nil
}
