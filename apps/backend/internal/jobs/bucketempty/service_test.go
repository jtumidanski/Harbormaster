package bucketempty

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// drainProgress blocks until the channel closes, returning the last Progress
// observed. It is the canonical way for tests to wait on a worker finishing
// without racing on the unbuffered subscriber sends.
func drainProgress(t *testing.T, ch <-chan Progress) Progress {
	t.Helper()
	var last Progress
	for p := range ch {
		last = p
	}
	return last
}

// waitForResult reads exactly one Result with a generous timeout to surface
// deadlocks as failures rather than test-runner hangs.
func waitForResult(t *testing.T, ch <-chan Result) Result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Result")
		return Result{}
	}
}

// TestStartOrAttach_NewJob exercises the happy path: a single caller starts a
// fresh job, the stub worker emits two batches, then completes. The DB row
// must end in state='done' with the final deleted_count, and exactly one
// success audit event must be recorded.
func TestStartOrAttach_NewJob(t *testing.T) {
	gdb := newTestDB(t)
	audit := &fakeAudit{}

	const totalKeys = 2500
	runFn := func(s *Service, ctx context.Context, sub *subscription, purgeVersions bool) {
		// Simulate two full batches plus a remainder; broadcast after each.
		s.broadcast(sub, Progress{Deleted: 1000})
		s.broadcast(sub, Progress{Deleted: 2000})
		s.broadcast(sub, Progress{Deleted: totalKeys})
		s.terminate(sub, Result{
			JobID:        sub.jobID,
			Bucket:       sub.bucket,
			DeletedTotal: totalKeys,
			DurationMS:   42,
		}, time.Now(), totalKeys, purgeVersions)
	}
	svc := newServiceWithRun(t, gdb, audit, runFn)

	progressCh, doneCh, err := svc.StartOrAttach(context.Background(), "bkt-1", false)
	require.NoError(t, err)

	last := drainProgress(t, progressCh)
	assert.EqualValues(t, totalKeys, last.Deleted)

	res := waitForResult(t, doneCh)
	assert.EqualValues(t, totalKeys, res.DeletedTotal)
	assert.Empty(t, res.ErrorMessage)
	// Channel must be closed after the single Result.
	_, ok := <-doneCh
	assert.False(t, ok, "done channel should be closed after Result")

	// DB row reflects the terminal state.
	job, err := findByID(gdb, res.JobID)
	require.NoError(t, err)
	assert.Equal(t, StateDone, job.State)
	assert.EqualValues(t, totalKeys, job.DeletedCount)
	require.NotNil(t, job.FinishedAt)

	// Exactly one success audit row was written for this bucket.
	calls := audit.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, ActionBucketEmpty, calls[0].Action)
	assert.Equal(t, "bkt-1", calls[0].Target)
	assert.Equal(t, OutcomeSuccess, calls[0].Outcome)
	assert.Empty(t, calls[0].ErrMsg)
}

// TestStartOrAttach_Concurrent verifies the single-flight fan-out: a second
// StartOrAttach for the same bucket returns the SAME done channel and a NEW
// progress channel. Only one MarkDone (one audit row) is written.
func TestStartOrAttach_Concurrent(t *testing.T) {
	gdb := newTestDB(t)
	audit := &fakeAudit{}

	// Block the worker until release is closed so we can attach a second
	// subscriber while the first job is still active.
	release := make(chan struct{})
	runFn := func(s *Service, ctx context.Context, sub *subscription, purgeVersions bool) {
		<-release
		s.broadcast(sub, Progress{Deleted: 7})
		s.terminate(sub, Result{
			JobID: sub.jobID, Bucket: sub.bucket, DeletedTotal: 7,
		}, time.Now(), 7, purgeVersions)
	}
	svc := newServiceWithRun(t, gdb, audit, runFn)

	pc1, done1, err := svc.StartOrAttach(context.Background(), "bkt-2", false)
	require.NoError(t, err)

	pc2, done2, err := svc.StartOrAttach(context.Background(), "bkt-2", false)
	require.NoError(t, err)

	// done is the SAME terminal channel for both subscribers; pc1 != pc2.
	assert.Equal(t, asInterface(done1), asInterface(done2), "done channels must be the same instance")
	assert.NotEqual(t, asInterface(pc1), asInterface(pc2), "progress channels must be independent")

	close(release)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = drainProgress(t, pc1) }()
	go func() { defer wg.Done(); _ = drainProgress(t, pc2) }()

	r1 := waitForResult(t, done1)
	wg.Wait()

	// Reading done2 must return immediately with the same Result already
	// observed on done1 (or a closed channel — both are acceptable since
	// the buffer is consumed by the first receiver).
	select {
	case _, ok := <-done2:
		// Either same Result or closed signal — both are valid.
		_ = ok
	case <-time.After(2 * time.Second):
		t.Fatal("done2 did not unblock")
	}

	assert.EqualValues(t, 7, r1.DeletedTotal)

	// Only one DB row exists for the bucket and audit recorded exactly once.
	var rows []jobRow
	require.NoError(t, gdb.Where("bucket_name = ?", "bkt-2").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, StateDone, rows[0].State)

	calls := audit.snapshot()
	assert.Len(t, calls, 1)
}

// TestStartOrAttach_MidFlightError simulates a worker failure after the first
// batch. The DB row must transition to state='error' with the message, and
// the audit event must be a failure outcome.
func TestStartOrAttach_MidFlightError(t *testing.T) {
	gdb := newTestDB(t)
	audit := &fakeAudit{}

	runFn := func(s *Service, ctx context.Context, sub *subscription, purgeVersions bool) {
		s.broadcast(sub, Progress{Deleted: 100})
		s.terminate(sub, Result{
			JobID: sub.jobID, Bucket: sub.bucket,
			DeletedTotal: 100, ErrorMessage: "boom: connection reset",
		}, time.Now(), 100, purgeVersions)
	}
	svc := newServiceWithRun(t, gdb, audit, runFn)

	progressCh, doneCh, err := svc.StartOrAttach(context.Background(), "bkt-err", true)
	require.NoError(t, err)

	_ = drainProgress(t, progressCh)
	res := waitForResult(t, doneCh)
	assert.Contains(t, res.ErrorMessage, "boom")
	assert.EqualValues(t, 100, res.DeletedTotal)

	job, err := findByID(gdb, res.JobID)
	require.NoError(t, err)
	assert.Equal(t, StateError, job.State)
	assert.Contains(t, job.Error, "boom")

	calls := audit.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, OutcomeFailure, calls[0].Outcome)
	assert.Contains(t, calls[0].ErrMsg, "boom")
}

// TestOrphanRunningAtStartup pre-seeds the table with one state='running'
// row, then runs OrphanRunningAtStartup and asserts the row was flipped to
// state='error' with the orphaned-by-restart message. Exactly one failure
// audit row must be recorded for the affected bucket.
func TestOrphanRunningAtStartup(t *testing.T) {
	gdb := newTestDB(t)
	audit := &fakeAudit{}

	// Insert one running row via the repo helper so the partial unique index
	// is exercised exactly as production does.
	id, err := InsertRunning(gdb, "stuck-bucket", false)
	require.NoError(t, err)

	orphaned, err := OrphanRunningAtStartup(gdb, audit)
	require.NoError(t, err)
	require.Len(t, orphaned, 1)
	assert.Equal(t, "stuck-bucket", orphaned[0].Bucket)

	job, err := findByID(gdb, id)
	require.NoError(t, err)
	assert.Equal(t, StateError, job.State)
	assert.True(t, strings.Contains(strings.ToLower(job.Error), "orphan"), "expected error to mention orphan, got %q", job.Error)

	calls := audit.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, ActionBucketEmpty, calls[0].Action)
	assert.Equal(t, "stuck-bucket", calls[0].Target)
	assert.Equal(t, OutcomeFailure, calls[0].Outcome)

	// Subsequent InsertRunning for the same bucket succeeds because the
	// partial unique index now sees the row as 'error'.
	_, err = InsertRunning(gdb, "stuck-bucket", false)
	require.NoError(t, err)
}

// TestInsertRunning_DuplicateReturnsSentinel verifies the partial unique
// index is translated into the exported sentinel error.
func TestInsertRunning_DuplicateReturnsSentinel(t *testing.T) {
	gdb := newTestDB(t)

	_, err := InsertRunning(gdb, "dup-bucket", false)
	require.NoError(t, err)

	_, err = InsertRunning(gdb, "dup-bucket", true)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRunning)
}

// findByID is a tiny test helper that loads one row by primary key and maps
// it back to the public Job type without going through FindRunning (so we
// can inspect terminal states).
func findByID(db *gorm.DB, id string) (Job, error) {
	var row jobRow
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Job{}, err
		}
		return Job{}, err
	}
	return row.toJob(), nil
}

// asInterface returns the dynamic address of a channel as an interface so two
// channel handles can be compared via assert.Equal. Comparing the channels
// directly with assert.Equal would deep-compare buffers; we want pointer
// identity.
func asInterface[T any](ch <-chan T) any { return ch }
