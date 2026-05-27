package bucketempty

import (
	"context"
	"sync"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// Audit action / outcome constants mirror internal/audit so the package
// remains importable without a cyclic dependency.
const (
	ActionBucketEmpty = "bucket.empty"
	OutcomeSuccess    = "success"
	OutcomeFailure    = "failure"
)

// Progress is emitted per RemoveObjects batch to every attached SSE subscriber.
// EstimatedTotal is best-effort; the v1 worker leaves it zero and the UI
// renders a determinate-style bar based on deleted count alone.
type Progress struct {
	Deleted        int64 `json:"deleted"`
	EstimatedTotal int64 `json:"estimated_total"`
}

// Result terminates a stream. Exactly one Result is sent on each subscriber's
// done channel before the channel is closed.
type Result struct {
	JobID        string
	Bucket       string
	DeletedTotal int64
	DurationMS   int64
	ErrorMessage string
}

// PoolGetter is the subset of internal/minio.Pool the service uses. Keeping
// it as an interface lets tests inject a stub without importing the real
// pool package.
type PoolGetter interface {
	Get(ctx context.Context) (*madmin.AdminClient, *miniogo.Client, error)
}

// AuditRecorder is the subset of audit-recording the service uses. The
// signature intentionally returns no error; the worker cannot meaningfully
// recover from a missing audit write, so the adapter is expected to log and
// swallow.
type AuditRecorder interface {
	Record(ctx context.Context, action, target, outcome string, payload map[string]any, errMsg string)
}

// Service coordinates empty-bucket jobs. One Service per process; the
// in-memory active map is the authoritative single-flight gate for live
// requests, with the partial unique index as the cross-process backstop.
type Service struct {
	db    *gorm.DB
	pool  PoolGetter
	audit AuditRecorder

	mu     sync.Mutex
	active map[string]*subscription

	// runFn lets tests substitute the worker body without spinning up a real
	// MinIO. When nil the production run loop is used.
	runFn func(ctx context.Context, sub *subscription, purgeVersions bool)
}

// subscription tracks one running job and its fan-out subscriber channels.
type subscription struct {
	jobID  string
	bucket string

	// done is shared by all subscribers; buffered to 1 so the worker can
	// always send the terminal Result without coordination.
	done chan Result

	// subs holds every active progress channel. The slice is mutated under
	// Service.mu.
	subs []chan Progress
}

// New constructs a Service. Callers must call OrphanRunningAtStartup once at
// process boot before invoking StartOrAttach for the first time.
func New(db *gorm.DB, pool PoolGetter, audit AuditRecorder) *Service {
	return &Service{
		db:     db,
		pool:   pool,
		audit:  audit,
		active: map[string]*subscription{},
	}
}

// StartOrAttach starts a new job for bucket or attaches the caller to an
// in-flight job. progressCh emits per-batch updates; doneCh emits the terminal
// Result exactly once and is then closed.
//
// The returned progress channel belongs solely to this caller; cancelling the
// request context does NOT cancel the job (the worker uses a detached
// context.Background()). Closing the request just means future broadcasts
// will be dropped via the non-blocking select.
func (s *Service) StartOrAttach(ctx context.Context, bucket string, purgeVersions bool) (<-chan Progress, <-chan Result, error) {
	s.mu.Lock()
	if sub, ok := s.active[bucket]; ok {
		pc := make(chan Progress, 16)
		sub.subs = append(sub.subs, pc)
		s.mu.Unlock()
		return pc, sub.done, nil
	}

	id, err := InsertRunning(s.db, bucket, purgeVersions)
	if err != nil {
		s.mu.Unlock()
		return nil, nil, err
	}
	pc := make(chan Progress, 16)
	sub := &subscription{
		jobID:  id,
		bucket: bucket,
		done:   make(chan Result, 1),
		subs:   []chan Progress{pc},
	}
	s.active[bucket] = sub
	s.mu.Unlock()

	runFn := s.runFn
	if runFn == nil {
		runFn = s.run
	}
	//nolint:gosec // G118: the worker deliberately uses a detached context so cancelling the originating request does not abort an in-flight empty-bucket job (see StartOrAttach doc).
	go runFn(context.Background(), sub, purgeVersions)
	return pc, sub.done, nil
}

// run is the production worker body. It resolves the live MinIO client pair
// from the pool, checks versioning, drains all objects (and versions if
// requested) in 1000-key batches, broadcasts progress, and writes the
// terminal audit row.
func (s *Service) run(ctx context.Context, sub *subscription, purgeVersions bool) {
	start := time.Now()
	_, mc, err := s.pool.Get(ctx)
	if err != nil {
		s.terminate(sub, Result{JobID: sub.jobID, Bucket: sub.bucket, ErrorMessage: err.Error()}, start, 0, purgeVersions, false)
		return
	}

	versioned, _ := isVersioned(ctx, mc, sub.bucket)
	var (
		deleted int64
		batchN  = 1000
	)
	onBatch := func(n int64) {
		deleted += n
		s.broadcast(sub, Progress{Deleted: deleted})
	}
	if purgeVersions && versioned {
		err = drainVersions(ctx, mc, sub.bucket, batchN, onBatch)
	} else {
		err = drainObjects(ctx, mc, sub.bucket, batchN, onBatch)
	}
	res := Result{
		JobID:        sub.jobID,
		Bucket:       sub.bucket,
		DeletedTotal: deleted,
		DurationMS:   time.Since(start).Milliseconds(),
	}
	if err != nil {
		res.ErrorMessage = err.Error()
	}
	s.terminate(sub, res, start, deleted, purgeVersions, versioned)
}

// broadcast pushes a Progress event to every attached subscriber using a
// non-blocking send so a slow consumer can never stall the worker; dropped
// events are tolerated because each Progress carries the absolute deleted
// count (i.e. the stream is idempotent from the UI's perspective).
//
// It also persists the most recent counters so a restarted process can show
// the last known progress before the job is re-orphaned.
func (s *Service) broadcast(sub *subscription, p Progress) {
	s.mu.Lock()
	for _, ch := range sub.subs {
		select {
		case ch <- p:
		default:
		}
	}
	s.mu.Unlock()
	_ = UpdateProgress(s.db, sub.jobID, p.Deleted, p.EstimatedTotal)
}

// terminate closes every subscriber, sends the terminal Result, writes the
// final DB row, and emits the audit event. It is safe to call exactly once
// per subscription (it removes the entry from active under the lock).
//
// Note: r.DurationMS is the worker-measured wall-clock; the start parameter
// is retained as a signal for future telemetry hooks (latency histograms).
// versioningEnabledAtStart records the bucket's versioning posture as
// observed by the worker before the drain began; it surfaces in the audit
// payload so operators can correlate retention decisions with the actual
// purge-versions interaction (per T3.24).
func (s *Service) terminate(sub *subscription, r Result, _ time.Time, _ int64, purgeVersions, versioningEnabledAtStart bool) {
	s.mu.Lock()
	delete(s.active, sub.bucket)
	for _, ch := range sub.subs {
		close(ch)
	}
	sub.subs = nil
	s.mu.Unlock()

	// Persist the terminal DB row and emit the audit event BEFORE signalling
	// completion. The done send below is a happens-before edge for every
	// subscriber, so any consumer woken by the terminal Result must be able to
	// observe the fully-recorded state (final job row + exactly-once audit).
	// Signalling first would race these writes against the receiver.
	if r.ErrorMessage != "" {
		_ = MarkError(s.db, sub.jobID, r.ErrorMessage)
	} else {
		_ = MarkDone(s.db, sub.jobID, r.DeletedTotal)
	}

	if s.audit != nil {
		payload := map[string]any{
			"job_id":                      sub.jobID,
			"deleted_count":               r.DeletedTotal,
			"duration_ms":                 r.DurationMS,
			"purge_versions":              purgeVersions,
			"versioning_enabled_at_start": versioningEnabledAtStart,
		}
		out := OutcomeSuccess
		if r.ErrorMessage != "" {
			out = OutcomeFailure
		}
		s.audit.Record(context.Background(), ActionBucketEmpty, sub.bucket, out, payload, r.ErrorMessage)
	}

	// Send the terminal Result last. done is buffered (cap=1) so this never
	// blocks even when no subscriber is reading yet.
	sub.done <- r
	close(sub.done)
}
