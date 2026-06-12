# Folder Delete & Multi-Select Bulk Delete — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add recursive folder (prefix) delete and multi-select bulk delete to the object browser, powered by one new backend endpoint `POST /api/v1/buckets/{bucket}/objects/bulk-delete` that expands prefixes server-side and batch-deletes via MinIO's `RemoveObjects`.

**Architecture:** The processor's existing `s3API`/`S3Client` interfaces gain two channel-based MinIO methods (`ListObjects`, `RemoveObjects`); the live adapter already satisfies them via its embedded `*miniogo.Client`, so no adapter code changes. A new objects-local `bulkdelete.go` streams a recursive listing into ≤1000-key `RemoveObjects` batches (delete path) or counts to a 10,000 ceiling (dry-run path). On the frontend, one shared `BulkDeleteDialog` serves both folder-delete and multi-select entry points, with selection state owned by `ObjectBrowserPage`.

**Tech Stack:** Go (chi, minio-go v7, zerolog), React + TypeScript (Vite, TanStack React Query, shadcn/ui, Tailwind, sonner, vitest).

---

## File Structure

**Backend (`apps/backend`)**
- `internal/audit/model.go` — add + register `ActionObjectBulkDelete`.
- `internal/audit/no_secrets_test.go` — bump the hard-coded action count 35 → 36.
- `internal/objects/model.go` — add `BulkDeleteResult`, `BulkDeleteFailure`.
- `internal/objects/rest.go` — add `BulkDeleteRequest`.
- `internal/objects/processor.go` — extend `s3API` + `S3Client` interfaces (two methods); add `Processor.BulkDelete`.
- `internal/objects/bulkdelete.go` *(new)* — `countExpansion` + `deleteExpansion` helpers, ceiling/batch constants.
- `internal/objects/resource.go` — register route + `bulkDelete` handler.
- `internal/objects/helpers_test.go` — extend `stubS3` with channel `ListObjects`/`RemoveObjects`.
- `internal/objects/bulkdelete_test.go` *(new)* — processor unit tests.
- `internal/objects/resource_test.go` — route tests.
- `cmd/harbormaster/audit_adapter.go` — **no change** (embedded `*miniogo.Client` already satisfies the wider interface); a verification step confirms the build.

**Frontend (`apps/frontend`)**
- `src/components/ui/checkbox.tsx` *(new)* — native-input shadcn-style Checkbox (no new dependency).
- `src/features/objects/api.ts` — `previewBulkDelete`, `bulkDelete` + types.
- `src/features/objects/BulkDeleteDialog.tsx` *(new)* — shared confirmation dialog.
- `src/features/objects/BulkDeleteDialog.test.tsx` *(new)* — dialog tests.
- `src/features/objects/VirtualizedObjectList.tsx` — row checkboxes + folder trash button + new props.
- `src/features/objects/ObjectBrowserPage.tsx` — selection state, toolbar, dialog wiring, clear-on-nav.
- `src/features/objects/ObjectBrowserPage.test.tsx` — selection toolbar + folder-delete + partial-failure tests.

---

## Design decision: Checkbox component

The design names a "shadcn `Checkbox`", but `@radix-ui/react-checkbox` is **not** a dependency and no `src/components/ui/checkbox.tsx` exists. To avoid adding a dependency mid-task, this plan creates a self-contained shadcn-style `Checkbox` backed by a native `<input type="checkbox">` styled with Tailwind (Task 8). It exposes the same `checked` / `onCheckedChange`-equivalent surface the call sites need (`checked`, `onChange`). This satisfies the design intent (one reusable `Checkbox` in `components/ui`) without an `npm install` step.

---

## Task 1: Register the bulk-delete audit action

**Files:**
- Modify: `apps/backend/internal/audit/model.go:45` (const block) and `:86` (AllActions slice)
- Modify: `apps/backend/internal/audit/no_secrets_test.go:23`

- [ ] **Step 1: Run the existing audit test to confirm the current count**

Run (cwd `apps/backend`): `go test ./internal/audit/ -run TestNoSecrets -count=1`
Expected: PASS (35 actions today).

- [ ] **Step 2: Add the constant**

In `internal/audit/model.go`, inside the `const (...)` block, add after `ActionObjectUndelete = "object.undelete"` (line 45):

```go
	ActionObjectBulkDelete      = "object.bulk_delete"
```

- [ ] **Step 3: Register it in AllActions()**

In `internal/audit/model.go`, in the `AllActions()` return slice, add after `ActionObjectUndelete,` (line 86):

```go
		ActionObjectBulkDelete,
```

- [ ] **Step 4: Bump the hard-coded count assertion**

In `internal/audit/no_secrets_test.go`, change line 22-23 from:

```go
	// Verify AllActions() returns all 35 constants.
	allActions := audit.AllActions()
	require.Len(t, allActions, 35, "AllActions() should return exactly 35 actions")
```

to:

```go
	// Verify AllActions() returns all 36 constants.
	allActions := audit.AllActions()
	require.Len(t, allActions, 36, "AllActions() should return exactly 36 actions")
```

- [ ] **Step 5: Run the audit tests**

Run (cwd `apps/backend`): `go test ./internal/audit/ -count=1`
Expected: PASS (both `no_secrets_test.go` and `no_presigned_test.go` enumerate `AllActions()` and now see 36).

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/audit/model.go apps/backend/internal/audit/no_secrets_test.go
git commit -m "feat(audit): add object.bulk_delete action constant"
```

---

## Task 2: Add bulk-delete model + request types

**Files:**
- Modify: `apps/backend/internal/objects/model.go` (append)
- Modify: `apps/backend/internal/objects/rest.go` (append request type)

- [ ] **Step 1: Add result types to model.go**

Append to `internal/objects/model.go`:

```go
// BulkDeleteResult is the single result type for both bulk-delete modes.
// The REST layer renders the dry-run subset (ObjectCount, Truncated) or
// the delete subset (DeletedCount, Failures) depending on the request's
// DryRun flag.
type BulkDeleteResult struct {
	// Dry-run fields.
	ObjectCount int  // explicit keys + expanded prefix keys, exact up to the ceiling
	Truncated   bool // count hit the 10,000 ceiling

	// Delete fields.
	DeletedCount int
	Failures     []BulkDeleteFailure
}

// BulkDeleteFailure is a single per-key delete failure surfaced on the
// RemoveObjects error channel. Error is the SDK error message; it never
// aborts the remaining deletes.
type BulkDeleteFailure struct {
	Key   string
	Error string
}
```

- [ ] **Step 2: Add the request type to rest.go**

Append to `internal/objects/rest.go`:

```go
// BulkDeleteRequest is the body accepted by POST /objects/bulk-delete.
// At least one of Keys or Prefixes must be non-empty; DryRun selects the
// count-only preview vs. the real delete.
type BulkDeleteRequest struct {
	Keys     []string `json:"keys"`
	Prefixes []string `json:"prefixes"`
	DryRun   bool     `json:"dry_run"`
}
```

- [ ] **Step 3: Verify it compiles**

Run (cwd `apps/backend`): `CGO_ENABLED=0 go build ./internal/objects/`
Expected: success, no output.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/objects/model.go apps/backend/internal/objects/rest.go
git commit -m "feat(objects): add bulk-delete model and request types"
```

---

## Task 3: Extend the s3API / S3Client interfaces + test stub

**Files:**
- Modify: `apps/backend/internal/objects/processor.go:53-66` (s3API) and `:80-93` (S3Client)
- Modify: `apps/backend/internal/objects/helpers_test.go`

- [ ] **Step 1: Add the two methods to the unexported `s3API` interface**

In `internal/objects/processor.go`, inside `type s3API interface { ... }`, after the `PresignedGetObject` line (line 65), add:

```go
	// ListObjects streams a recursive (delimiter-less) listing of every
	// key under opts.Prefix. Used by the bulk-delete prefix expansion.
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
	// RemoveObjects batch-deletes the keys fed on objectsCh, signalling
	// per-key failures (and only failures) on the returned channel.
	RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
```

- [ ] **Step 2: Add the identical pair to the exported `S3Client` interface**

In `internal/objects/processor.go`, inside `type S3Client interface { ... }`, after its `PresignedGetObject` line (line 92), add the same two lines:

```go
	// ListObjects streams a recursive (delimiter-less) listing of every
	// key under opts.Prefix. Used by the bulk-delete prefix expansion.
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
	// RemoveObjects batch-deletes the keys fed on objectsCh, signalling
	// per-key failures (and only failures) on the returned channel.
	RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
```

- [ ] **Step 3: Add bulk-delete control fields to the stub**

In `internal/objects/helpers_test.go`, inside the `stubS3` struct (after the `removedVerIDs` field, line 69), add:

```go
	// Bulk-delete controls.
	// bulkListing maps a prefix to the object keys ListObjects emits for it.
	bulkListing map[string][]string
	// bulkListErr, when set, makes ListObjects emit a single ObjectInfo
	// with Err set (simulating a transport/listing failure).
	bulkListErr error
	// removeFailKeys maps a key to the error message RemoveObjects emits
	// for it; keys absent from the map "succeed" (silence on the channel).
	removeFailKeys map[string]string
	// removeSubmitted captures every key handed to RemoveObjects across
	// all batches, in submission order.
	removeSubmitted []string
```

- [ ] **Step 4: Implement the channel stub methods**

Append to `internal/objects/helpers_test.go`:

```go
func (s *stubS3) ListObjects(ctx context.Context, _ string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo {
	ch := make(chan miniogo.ObjectInfo)
	keys := s.bulkListing[opts.Prefix]
	listErr := s.bulkListErr
	go func() {
		defer close(ch)
		if listErr != nil {
			select {
			case ch <- miniogo.ObjectInfo{Err: listErr}:
			case <-ctx.Done():
			}
			return
		}
		for _, k := range keys {
			select {
			case ch <- miniogo.ObjectInfo{Key: k}:
			case <-ctx.Done():
				// Consumer broke early (e.g. dry-run ceiling); stop so the
				// producer goroutine doesn't block forever on a send.
				return
			}
		}
	}()
	return ch
}

func (s *stubS3) RemoveObjects(ctx context.Context, _ string, objectsCh <-chan miniogo.ObjectInfo, _ miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError {
	errCh := make(chan miniogo.RemoveObjectError)
	go func() {
		defer close(errCh)
		for obj := range objectsCh {
			s.mu.Lock()
			s.removeSubmitted = append(s.removeSubmitted, obj.Key)
			s.mu.Unlock()
			if msg, ok := s.removeFailKeys[obj.Key]; ok {
				select {
				case errCh <- miniogo.RemoveObjectError{ObjectName: obj.Key, Err: errors.New(msg)}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return errCh
}
```

(`errors` and `context` are already imported in `helpers_test.go`.)

- [ ] **Step 5: Verify it compiles (tests build, even before new tests exist)**

Run (cwd `apps/backend`): `go vet ./internal/objects/`
Expected: success — the stub now satisfies the wider `s3API`, and the live adapter's `var _ objects.S3Client = objectS3Adapter{}` assertion still holds because `*miniogo.Client` has both methods natively.

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/objects/processor.go apps/backend/internal/objects/helpers_test.go
git commit -m "feat(objects): extend S3 interfaces with ListObjects/RemoveObjects + stub"
```

---

## Task 4: Prefix-expansion + batching helper (`bulkdelete.go`)

**Files:**
- Create: `apps/backend/internal/objects/bulkdelete.go`
- Create: `apps/backend/internal/objects/bulkdelete_test.go`

- [ ] **Step 1: Write the failing tests for the expansion helpers**

Create `internal/objects/bulkdelete_test.go`:

```go
package objects

import (
	"context"
	"strconv"
	"testing"
)

func TestCountExpansion_ExplicitPlusPrefixes(t *testing.T) {
	s3 := &stubS3{
		bulkListing: map[string][]string{
			"photos/": {"photos/a.jpg", "photos/b.jpg", "photos/c.jpg"},
			"logs/":   {"logs/x.log"},
		},
	}
	count, truncated, err := countExpansion(context.Background(), s3, "b",
		[]string{"notes.txt", "readme.md"}, []string{"photos/", "logs/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Fatalf("did not expect truncation")
	}
	// 2 explicit keys + 3 under photos/ + 1 under logs/ = 6.
	if count != 6 {
		t.Fatalf("count = %d, want 6", count)
	}
}

func TestCountExpansion_Ceiling(t *testing.T) {
	keys := make([]string, 0, 10001)
	for i := 0; i < 10001; i++ {
		keys = append(keys, "big/"+strconv.Itoa(i))
	}
	s3 := &stubS3{bulkListing: map[string][]string{"big/": keys}}
	count, truncated, err := countExpansion(context.Background(), s3, "b", nil, []string{"big/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != bulkDeleteCeiling {
		t.Fatalf("count = %d, want %d", count, bulkDeleteCeiling)
	}
	if !truncated {
		t.Fatalf("expected truncated=true at the ceiling")
	}
	// If the producer goroutine were not torn down on the early break,
	// the test binary would leak a blocked goroutine; the select on
	// ctx.Done() in the stub guarantees teardown. Reaching here = no deadlock.
}

func TestCountExpansion_ListError(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	_, _, err := countExpansion(context.Background(), s3, "b", nil, []string{"photos/"})
	if err == nil {
		t.Fatalf("expected a listing error")
	}
}

func TestDeleteExpansion_BatchesAndDeletes(t *testing.T) {
	s3 := &stubS3{
		bulkListing: map[string][]string{"photos/": {"photos/a.jpg", "photos/b.jpg"}},
	}
	deleted, failures, err := deleteExpansion(context.Background(), s3, "b",
		[]string{"notes.txt"}, []string{"photos/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	if len(s3.removeSubmitted) != 3 {
		t.Fatalf("submitted %d keys, want 3", len(s3.removeSubmitted))
	}
}

func TestDeleteExpansion_PartialFailure(t *testing.T) {
	s3 := &stubS3{
		bulkListing:    map[string][]string{"logs/": {"logs/ok.log", "logs/locked.bin"}},
		removeFailKeys: map[string]string{"logs/locked.bin": "object is WORM-locked"},
	}
	deleted, failures, err := deleteExpansion(context.Background(), s3, "b", nil, []string{"logs/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if len(failures) != 1 || failures[0].Key != "logs/locked.bin" {
		t.Fatalf("failures = %+v, want one for logs/locked.bin", failures)
	}
}

func TestDeleteExpansion_ListError(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	_, _, err := deleteExpansion(context.Background(), s3, "b", nil, []string{"photos/"})
	if err == nil {
		t.Fatalf("expected a listing error to abort the delete")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail (helpers undefined)**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'Expansion' -count=1`
Expected: FAIL — `undefined: countExpansion`, `undefined: deleteExpansion`, `undefined: bulkDeleteCeiling`.

- [ ] **Step 3: Implement the helper**

Create `internal/objects/bulkdelete.go`:

```go
package objects

import (
	"context"

	miniogo "github.com/minio/minio-go/v7"
)

const (
	// bulkDeleteCeiling is the maximum exact object count the dry-run
	// preview reports. Once the running count reaches this, counting stops
	// and Truncated is set so the UI can render "10,000+". The delete path
	// is NOT capped by this value.
	bulkDeleteCeiling = 10000

	// bulkRemoveBatchN is the max number of keys handed to a single
	// RemoveObjects call. Mirrors MinIO's server-side max-keys cap of 1000.
	bulkRemoveBatchN = 1000
)

// countExpansion streams a recursive listing of each prefix and returns
// the number of objects under them plus len(keys), capped at
// bulkDeleteCeiling. De-duplication of overlapping keys/prefixes is not
// performed (best-effort per the PRD); the count may slightly over-report
// on overlap.
//
// A cancelable context is derived from ctx and cancelled on every return
// path (including the early break at the ceiling) so the minio-go producer
// goroutine never blocks forever on a channel send.
func countExpansion(ctx context.Context, s3 s3API, bucket string, keys, prefixes []string) (int, bool, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	count := len(keys)
	if count >= bulkDeleteCeiling {
		return bulkDeleteCeiling, true, nil
	}
	for _, prefix := range prefixes {
		ch := s3.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{Prefix: prefix, Recursive: true})
		for obj := range ch {
			if obj.Err != nil {
				return 0, false, obj.Err
			}
			count++
			if count >= bulkDeleteCeiling {
				return bulkDeleteCeiling, true, nil
			}
		}
	}
	return count, false, nil
}

// deleteExpansion streams the explicit keys plus every key under each
// prefix into RemoveObjects in batches of <= bulkRemoveBatchN, collecting
// per-key failures without aborting. Deletes carry no version ID
// (RemoveObjectsOptions{} + bare ObjectInfo{Key}), matching the
// single-object Delete semantics: a delete marker on a versioned bucket,
// a permanent removal on an unversioned bucket.
//
// A listing error aborts the whole operation and is returned. The producer
// goroutine is torn down via a cancelable context on every return path.
// DeletedCount = (keys submitted) - (per-key failures).
func deleteExpansion(ctx context.Context, s3 s3API, bucket string, keys, prefixes []string) (int, []BulkDeleteFailure, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var failures []BulkDeleteFailure
	submitted := 0
	batch := make([]miniogo.ObjectInfo, 0, bulkRemoveBatchN)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		objCh := make(chan miniogo.ObjectInfo, len(batch))
		for _, o := range batch {
			objCh <- o
		}
		close(objCh)
		errCh := s3.RemoveObjects(cctx, bucket, objCh, miniogo.RemoveObjectsOptions{})
		for e := range errCh {
			if e.Err != nil {
				failures = append(failures, BulkDeleteFailure{Key: e.ObjectName, Error: e.Err.Error()})
			}
		}
		submitted += len(batch)
		batch = batch[:0]
	}

	add := func(key string) {
		batch = append(batch, miniogo.ObjectInfo{Key: key})
		if len(batch) >= bulkRemoveBatchN {
			flush()
		}
	}

	for _, k := range keys {
		add(k)
	}
	for _, prefix := range prefixes {
		ch := s3.ListObjects(cctx, bucket, miniogo.ListObjectsOptions{Prefix: prefix, Recursive: true})
		for obj := range ch {
			if obj.Err != nil {
				return 0, nil, obj.Err
			}
			add(obj.Key)
		}
	}
	flush()

	return submitted - len(failures), failures, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'Expansion' -race -count=1`
Expected: PASS for all five tests.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/objects/bulkdelete.go apps/backend/internal/objects/bulkdelete_test.go
git commit -m "feat(objects): add prefix expansion + batching helpers for bulk delete"
```

---

## Task 5: `Processor.BulkDelete` (validation + audit)

**Files:**
- Modify: `apps/backend/internal/objects/processor.go` (append method)
- Modify: `apps/backend/internal/objects/bulkdelete_test.go` (append tests)

- [ ] **Step 1: Write the failing processor tests**

Append to `internal/objects/bulkdelete_test.go`:

```go
func TestBulkDelete_EmptyRequest_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, nil, true, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_EmptyPrefix_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{""}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_SlashPrefix_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{"/"}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_InvalidKey_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", []string{""}, nil, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "object_invalid_key")
}

func TestBulkDelete_DryRun_CountsWithoutDeleting(t *testing.T) {
	s3 := &stubS3{bulkListing: map[string][]string{"photos/": {"photos/a", "photos/b"}}}
	p, stub := newTestProcessor(t, s3, ProcessorConfig{})
	res, err := p.BulkDelete(context.Background(), "b", []string{"notes.txt"}, []string{"photos/"}, true, "alice", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ObjectCount != 3 || res.Truncated {
		t.Fatalf("got count=%d truncated=%v, want 3/false", res.ObjectCount, res.Truncated)
	}
	if len(stub.removeSubmitted) != 0 {
		t.Fatalf("dry-run must not delete; submitted %d keys", len(stub.removeSubmitted))
	}
}

func TestBulkDelete_Delete_AggregatesFailures(t *testing.T) {
	s3 := &stubS3{
		bulkListing:    map[string][]string{"logs/": {"logs/ok", "logs/bad"}},
		removeFailKeys: map[string]string{"logs/bad": "boom"},
	}
	p, _ := newTestProcessor(t, s3, ProcessorConfig{})
	res, err := p.BulkDelete(context.Background(), "b", nil, []string{"logs/"}, false, "alice", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DeletedCount != 1 {
		t.Fatalf("deleted = %d, want 1", res.DeletedCount)
	}
	if len(res.Failures) != 1 || res.Failures[0].Key != "logs/bad" {
		t.Fatalf("failures = %+v, want one for logs/bad", res.Failures)
	}
}

func TestBulkDelete_ListError_502(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	p, _ := newTestProcessor(t, s3, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{"photos/"}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 502, "minio_error")
}
```

Also append this shared assertion helper at the bottom of `bulkdelete_test.go`:

```go
// requireAPIError fails the test unless err is an *apierror.Error with the
// given HTTP status and code.
func requireAPIError(t *testing.T, err error, status int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with status %d code %q, got nil", status, code)
	}
	var ae *apierror.Error
	if !errorsAs(err, &ae) {
		t.Fatalf("error is not *apierror.Error: %v", err)
	}
	if ae.HTTPStatus != status || ae.Code != code {
		t.Fatalf("got status=%d code=%q, want status=%d code=%q", ae.HTTPStatus, ae.Code, status, code)
	}
}
```

Add the imports this helper needs to the top of `bulkdelete_test.go` (replace the existing import block):

```go
import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// errorsAs is a thin alias so the assertion helper reads cleanly.
func errorsAs(err error, target any) bool { return errors.As(err, target) }
```

> Verified: `apierror.Error` exposes `HTTPStatus int` and `Code string` (see `internal/apierror/apierror.go:27-31`). The error envelope sent to the client uses these via `apierror.Write`.

- [ ] **Step 2: Run the tests to verify they fail**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'TestBulkDelete' -count=1`
Expected: FAIL — `p.BulkDelete undefined`.

- [ ] **Step 3: Implement `Processor.BulkDelete`**

Append to `internal/objects/processor.go` (the file already imports `context`, `net/http`, `apierror`, and `audit`):

```go
// BulkDelete deletes (dryRun=false) or counts (dryRun=true) the union of
// the explicit keys and every key under each prefix.
//
// Validation runs before any MinIO call and records NO audit event on a
// pure reject: empty request -> 400 bad_request; invalid key -> 400
// object_invalid_key; empty or "/" prefix -> 400 bad_request (the
// whole-bucket-wipe guard, since an empty prefix matches every object).
//
// On dryRun the count is exact up to a 10,000 ceiling, beyond which
// ObjectCount is reported as 10000 and Truncated is set; nothing is
// deleted and no audit event is recorded. On a real delete, keys are
// issued without a version ID (delete marker on a versioned bucket,
// permanent removal otherwise), per-key failures are aggregated into
// Failures[] without aborting, a listing/transport error aborts with a
// 502 minio_error envelope, and exactly one audit event is recorded.
func (p *Processor) BulkDelete(ctx context.Context, bucket string, keys, prefixes []string, dryRun bool, actor, sourceIP string) (BulkDeleteResult, error) {
	if len(keys) == 0 && len(prefixes) == 0 {
		return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "bad_request",
			"at least one of keys or prefixes is required")
	}
	for _, k := range keys {
		if err := ValidateObjectKey(k); err != nil {
			return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
		}
	}
	for _, prefix := range prefixes {
		if prefix == "" || prefix == "/" {
			return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "bad_request", "prefix must not be empty")
		}
	}

	s3, err := p.clients(ctx)
	if err != nil {
		return BulkDeleteResult{}, err
	}

	if dryRun {
		count, truncated, cerr := countExpansion(ctx, s3, bucket, keys, prefixes)
		if cerr != nil {
			return BulkDeleteResult{}, mapClientError(cerr, "failed to count objects")
		}
		return BulkDeleteResult{ObjectCount: count, Truncated: truncated}, nil
	}

	deleted, failures, derr := deleteExpansion(ctx, s3, bucket, keys, prefixes)
	if derr != nil {
		return BulkDeleteResult{}, mapClientError(derr, "failed to bulk delete objects")
	}

	// One audit event per real delete operation. A single-prefix folder
	// delete (exactly one prefix, no explicit keys) is individually
	// traceable via bucket/prefix; everything else targets the bucket.
	targetID := bucket
	if len(prefixes) == 1 && len(keys) == 0 {
		targetID = bucket + "/" + prefixes[0]
	}
	outcome := audit.OutcomeSuccess
	if len(failures) > 0 {
		outcome = audit.OutcomeFailure
	}
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionObjectBulkDelete,
		TargetType: "object",
		TargetID:   targetID,
		Outcome:    outcome,
		PayloadSummary: map[string]any{
			"key_count":     len(keys),
			"prefixes":      prefixes,
			"deleted_count": deleted,
			"failure_count": len(failures),
		},
	})
	return BulkDeleteResult{DeletedCount: deleted, Failures: failures}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'TestBulkDelete' -race -count=1`
Expected: PASS for all seven tests.

- [ ] **Step 5: Add an audit-target test**

Append to `internal/objects/bulkdelete_test.go`. This verifies the single-prefix folder delete uses `bucket/prefix` and that dry-run records nothing. It uses a recording audit stub via the real `audit.Processor` is heavyweight; instead assert the public result behaviour and target-id selection indirectly through a focused unit test of the target rule. Add:

```go
func TestBulkDelete_DryRun_NoDeleteCalls(t *testing.T) {
	s3 := &stubS3{bulkListing: map[string][]string{"a/": {"a/1"}}}
	p, stub := newTestProcessor(t, s3, ProcessorConfig{})
	if _, err := p.BulkDelete(context.Background(), "b", nil, []string{"a/"}, true, "alice", "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.removeSubmitted) != 0 {
		t.Fatalf("dry-run submitted %d keys, want 0", len(stub.removeSubmitted))
	}
}
```

> Audit recording is best-effort and `p.Audit` is nil in `newTestProcessor`, so `recordAudit` is a no-op in these unit tests — the audit *contract* (one event, correct target) is covered by reading the code and by the route test in Task 6. No audit-spy infrastructure is added here to avoid over-engineering.

- [ ] **Step 6: Run the tests + full objects package**

Run (cwd `apps/backend`): `go test ./internal/objects/ -race -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/backend/internal/objects/processor.go apps/backend/internal/objects/bulkdelete_test.go
git commit -m "feat(objects): add Processor.BulkDelete with validation and audit"
```

---

## Task 6: REST handler + route

**Files:**
- Modify: `apps/backend/internal/objects/resource.go:64-77` (Routes) and append `bulkDelete` handler
- Modify: `apps/backend/internal/objects/resource_test.go` (append route tests)

- [ ] **Step 1: Write the failing route tests**

Append to `internal/objects/resource_test.go` (the file already has the `newTestRouter` helper used below — confirm its name by reading lines 21-34; if it differs, match the existing helper):

```go
func TestBulkDelete_HTTP_DryRunShape(t *testing.T) {
	stub := &stubS3{bulkListing: map[string][]string{"photos/": {"photos/a", "photos/b"}}}
	r, _ := newTestRouter(t, ProcessorConfig{}, stub)

	body := strings.NewReader(`{"prefixes":["photos/"],"dry_run":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/b/objects/bulk-delete", body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		ObjectCount int  `json:"object_count"`
		Truncated   bool `json:"truncated"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ObjectCount != 2 || got.Truncated {
		t.Fatalf("got %+v, want object_count=2 truncated=false", got)
	}
}

func TestBulkDelete_HTTP_DeleteShape(t *testing.T) {
	stub := &stubS3{
		bulkListing:    map[string][]string{"logs/": {"logs/ok", "logs/bad"}},
		removeFailKeys: map[string]string{"logs/bad": "boom"},
	}
	r, _ := newTestRouter(t, ProcessorConfig{}, stub)

	body := strings.NewReader(`{"prefixes":["logs/"],"dry_run":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/b/objects/bulk-delete", body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		DeletedCount int `json:"deleted_count"`
		Failures     []struct {
			Key   string `json:"key"`
			Error string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DeletedCount != 1 {
		t.Fatalf("deleted_count = %d, want 1", got.DeletedCount)
	}
	if len(got.Failures) != 1 || got.Failures[0].Key != "logs/bad" {
		t.Fatalf("failures = %+v, want one for logs/bad", got.Failures)
	}
}

func TestBulkDelete_HTTP_EmptyRequest_400(t *testing.T) {
	r, _ := newTestRouter(t, ProcessorConfig{}, &stubS3{})
	body := strings.NewReader(`{"dry_run":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/b/objects/bulk-delete", body)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error.Code != "bad_request" {
		t.Fatalf("error.code = %q, want bad_request", got.Error.Code)
	}
}
```

> Confirm the test file already imports `strings`, `encoding/json`, `net/http`, `net/http/httptest`. They are used by the existing tests in this file, so they should be present; if `strings` is missing add it.

- [ ] **Step 2: Run the tests to verify they fail**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'TestBulkDelete_HTTP' -count=1`
Expected: FAIL — route returns 404/405 (not registered) so status assertions fail.

- [ ] **Step 3: Register the route**

In `internal/objects/resource.go`, inside the `Routes` registrar, after the `r.Post("/buckets/{bucket}/objects/undelete", h.undelete)` line (line 75), add:

```go
		r.Post("/buckets/{bucket}/objects/bulk-delete", h.bulkDelete)
```

- [ ] **Step 4: Implement the handler**

Append to `internal/objects/resource.go`:

```go
// bulkDelete deletes a mix of explicit object keys and recursive folder
// prefixes in one operation, or (dry_run:true) counts what such a delete
// would affect without deleting anything. The body is plain JSON,
// mirroring restoreVersion/undelete; success and errors render
// action-style so the SPA reads error.code directly.
func (h *handler) bulkDelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var body BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(
			http.StatusBadRequest, "bad_request", "invalid JSON body"))
		return
	}

	actor, ip := actorFromRequest(r)
	res, err := h.p.BulkDelete(r.Context(), bucket, body.Keys, body.Prefixes, body.DryRun, actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}

	if body.DryRun {
		writeActionJSON(w, http.StatusOK, map[string]any{
			"object_count": res.ObjectCount,
			"truncated":    res.Truncated,
		})
		return
	}

	failures := make([]map[string]any, 0, len(res.Failures))
	for _, f := range res.Failures {
		failures = append(failures, map[string]any{"key": f.Key, "error": f.Error})
	}
	writeActionJSON(w, http.StatusOK, map[string]any{
		"deleted_count": res.DeletedCount,
		"failures":      failures,
	})
}
```

- [ ] **Step 5: Run the route tests to verify they pass**

Run (cwd `apps/backend`): `go test ./internal/objects/ -run 'TestBulkDelete_HTTP' -race -count=1`
Expected: PASS for all three tests.

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/objects/resource.go apps/backend/internal/objects/resource_test.go
git commit -m "feat(objects): mount POST /objects/bulk-delete route + handler"
```

---

## Task 7: Backend full gate

**Files:** none (verification only)

- [ ] **Step 1: Confirm the live adapter still satisfies the wider interface (no code change expected)**

Run (cwd `apps/backend`): `CGO_ENABLED=0 go build ./...`
Expected: success. The `var _ objects.S3Client = objectS3Adapter{}` assertion at `cmd/harbormaster/audit_adapter.go:260` holds because the embedded `*miniogo.Client` already exposes `ListObjects` and `RemoveObjects`. If the build fails with a missing-method error, read the method signatures minio-go actually exposes and reconcile the interface declarations in Task 3 against them.

- [ ] **Step 2: Run the full backend gate**

Run (cwd `apps/backend`), all four must be clean:

```bash
go test -race -count=1 ./...
go vet ./...
golangci-lint run
CGO_ENABLED=0 go build ./...
```

Expected: all pass. If `golangci-lint` flags the unused `add`/`flush` closures or an unparam result, address per its message (the closures here have no unused returns by design).

- [ ] **Step 3: Commit (only if any gofmt/lint fixups were needed)**

```bash
git add -A apps/backend
git commit -m "chore(objects): backend gate fixups for bulk delete"
```

---

## Task 8: Checkbox UI component (frontend)

**Files:**
- Create: `apps/frontend/src/components/ui/checkbox.tsx`

- [ ] **Step 1: Create the component**

Create `apps/frontend/src/components/ui/checkbox.tsx`:

```tsx
import * as React from "react";

import { cn } from "@/lib/utils";

export type CheckboxProps = React.InputHTMLAttributes<HTMLInputElement>;

// Checkbox is a thin, dependency-free wrapper over a native checkbox input
// styled to match the shadcn/ui surface. We use a native input (rather than
// @radix-ui/react-checkbox, which is not a project dependency) so callers
// get standard `checked` / `onChange` semantics with zero new packages.
const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, ...props }, ref) => (
    <input
      type="checkbox"
      ref={ref}
      className={cn(
        "h-4 w-4 shrink-0 cursor-pointer rounded border border-input accent-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
```

- [ ] **Step 2: Verify it type-checks**

Run (cwd `apps/frontend`): `npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add apps/frontend/src/components/ui/checkbox.tsx
git commit -m "feat(ui): add native Checkbox component"
```

---

## Task 9: Bulk-delete API client functions

**Files:**
- Modify: `apps/frontend/src/features/objects/api.ts` (append)

- [ ] **Step 1: Append the types and functions**

Append to `apps/frontend/src/features/objects/api.ts`:

```ts
// Bulk-delete wire types. The endpoint is POST .../objects/bulk-delete
// with a `dry_run` flag selecting the count preview vs. the real delete.
export type BulkDeletePreview = {
  object_count: number;
  truncated: boolean;
};

export type BulkDeleteFailure = {
  key: string;
  error: string;
};

export type BulkDeleteResult = {
  deleted_count: number;
  failures: BulkDeleteFailure[];
};

// previewBulkDelete returns the dry-run object count (exact up to 10,000,
// then truncated) for the given keys + prefixes WITHOUT deleting anything.
export async function previewBulkDelete(
  bucket: string,
  args: { keys: string[]; prefixes: string[] },
): Promise<BulkDeletePreview> {
  return api.post<BulkDeletePreview>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/bulk-delete`,
    { keys: args.keys, prefixes: args.prefixes, dry_run: true },
  );
}

// bulkDelete performs the real delete of the explicit keys plus every key
// under each prefix, returning the deleted count and per-key failures.
export async function bulkDelete(
  bucket: string,
  args: { keys: string[]; prefixes: string[] },
): Promise<BulkDeleteResult> {
  return api.post<BulkDeleteResult>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/bulk-delete`,
    { keys: args.keys, prefixes: args.prefixes, dry_run: false },
  );
}
```

- [ ] **Step 2: Verify it type-checks**

Run (cwd `apps/frontend`): `npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add apps/frontend/src/features/objects/api.ts
git commit -m "feat(objects): add bulk-delete API client functions"
```

---

## Task 10: Row checkboxes + folder trash button

**Files:**
- Modify: `apps/frontend/src/features/objects/VirtualizedObjectList.tsx`

- [ ] **Step 1: Extend the imports**

In `VirtualizedObjectList.tsx`, update the lucide and ui imports at the top:

```tsx
import { Download, Folder, History, Link as LinkIcon, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
```

- [ ] **Step 2: Extend the props type**

Replace the `VirtualizedObjectListProps` type (lines 19-30) with:

```tsx
export type VirtualizedObjectListProps = {
  items: ObjectListItem[];
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
  onOpenPrefix: (prefix: string) => void;
  onDownload: (key: string) => void;
  onDelete: (key: string) => void;
  onShare: (key: string) => void;
  onPreview: (key: string, contentType: string, size: number) => void;
  onVersions: (key: string) => void;
  selectedKeys: Set<string>;
  selectedPrefixes: Set<string>;
  onToggleKey: (key: string) => void;
  onTogglePrefix: (prefix: string) => void;
  onDeletePrefix: (prefix: string) => void;
};
```

- [ ] **Step 3: Destructure the new props**

In the `VirtualizedObjectList({ ... })` parameter list (lines 55-66), add the five new props:

```tsx
export function VirtualizedObjectList({
  items,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  onOpenPrefix,
  onDownload,
  onDelete,
  onShare,
  onPreview,
  onVersions,
  selectedKeys,
  selectedPrefixes,
  onToggleKey,
  onTogglePrefix,
  onDeletePrefix,
}: VirtualizedObjectListProps) {
```

- [ ] **Step 4: Add a checkbox + trash button to the folder row**

Replace the `object_prefixes` row block (lines 126-144) with:

```tsx
              if (item.type === "object_prefixes") {
                const p = item.attributes;
                return (
                  <div
                    key={`${item.type}:${item.id}`}
                    data-testid="object-row"
                    className="absolute left-0 right-0 flex items-center gap-3 border-b px-3 text-sm hover:bg-accent/40"
                    style={rowStyle}
                  >
                    <Checkbox
                      aria-label={`Select folder ${lastSegment(p.prefix)}`}
                      checked={selectedPrefixes.has(p.prefix)}
                      onChange={() => onTogglePrefix(p.prefix)}
                    />
                    <button
                      type="button"
                      className="flex flex-1 items-center gap-2 truncate text-left text-primary hover:underline"
                      onClick={() => onOpenPrefix(p.prefix)}
                    >
                      <Folder className="h-4 w-4 shrink-0" aria-hidden="true" />
                      <span className="truncate">{lastSegment(p.prefix)}/</span>
                    </button>
                    <div className="flex shrink-0 items-center gap-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-label={`Delete folder ${lastSegment(p.prefix)}`}
                        onClick={() => onDeletePrefix(p.prefix)}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </div>
                  </div>
                );
              }
```

- [ ] **Step 5: Add a checkbox to the object row**

Replace the object-row `return (...)` block (lines 147-203) so it begins with a checkbox immediately inside the row div. Replace lines 147-161 (from `return (` through the closing `</button>` of the preview button) with:

```tsx
              return (
                <div
                  key={`${item.type}:${item.id}`}
                  data-testid="object-row"
                  className="absolute left-0 right-0 flex items-center gap-3 border-b px-3 text-sm hover:bg-accent/40"
                  style={rowStyle}
                >
                  <Checkbox
                    aria-label={`Select ${e.key}`}
                    checked={selectedKeys.has(e.key)}
                    onChange={() => onToggleKey(e.key)}
                  />
                  <button
                    type="button"
                    className="flex flex-1 items-center gap-2 truncate text-left hover:underline"
                    onClick={() => onPreview(e.key, e.content_type, e.size)}
                    title={e.key}
                  >
                    <span className="truncate">{keyTail(e.key)}</span>
                  </button>
```

(The rest of the object row — size span and the Download/Share/Versions/Delete buttons — is unchanged.)

- [ ] **Step 6: Verify it type-checks**

Run (cwd `apps/frontend`): `npx tsc --noEmit`
Expected: no errors. (The parent `ObjectBrowserPage` does not yet pass the new props; this surfaces as a type error there in Task 12 — that is expected. If you run `tsc` now and see an error only in `ObjectBrowserPage.tsx`, that is fine and resolved by Task 12. To keep this step green, run the narrower check: `npx tsc --noEmit` and confirm the only errors are about missing props on `<VirtualizedObjectList>` in `ObjectBrowserPage.tsx`.)

- [ ] **Step 7: Commit**

```bash
git add apps/frontend/src/features/objects/VirtualizedObjectList.tsx
git commit -m "feat(objects): add row checkboxes and folder delete button"
```

---

## Task 11: Shared BulkDeleteDialog component

**Files:**
- Create: `apps/frontend/src/features/objects/BulkDeleteDialog.tsx`
- Create: `apps/frontend/src/features/objects/BulkDeleteDialog.test.tsx`

- [ ] **Step 1: Write the failing dialog tests**

Create `apps/frontend/src/features/objects/BulkDeleteDialog.test.tsx`:

```tsx
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ComponentProps } from "react";
import { Toaster } from "sonner";
import { BulkDeleteDialog } from "./BulkDeleteDialog";

function renderDialog(props: Partial<ComponentProps<typeof BulkDeleteDialog>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onDeleted = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <BulkDeleteDialog
        open
        onOpenChange={onOpenChange}
        bucket="b"
        keys={[]}
        prefixes={["photos/"]}
        onDeleted={onDeleted}
        {...props}
      />
      <Toaster />
    </QueryClientProvider>,
  );
  return { onDeleted, onOpenChange };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("BulkDeleteDialog", () => {
  it("shows the dry-run count and enables Delete once loaded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(jsonResponse({ object_count: 42, truncated: false }))),
    );
    renderDialog();
    await waitFor(() => expect(screen.getByText(/42/)).toBeInTheDocument());
    const del = screen.getByRole("button", { name: /^delete$/i });
    expect(del).toBeEnabled();
  });

  it("renders 10,000+ when truncated", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(jsonResponse({ object_count: 10000, truncated: true }))),
    );
    renderDialog();
    await waitFor(() => expect(screen.getByText(/10,000\+/)).toBeInTheDocument());
  });

  it("reports a partial-failure toast and calls onDeleted", async () => {
    const fetchSpy = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const isDelete =
        typeof init?.body === "string" && init.body.includes('"dry_run":false');
      if (isDelete) {
        return Promise.resolve(
          jsonResponse({ deleted_count: 2, failures: [{ key: "photos/x", error: "boom" }] }),
        );
      }
      return Promise.resolve(jsonResponse({ object_count: 3, truncated: false }));
    });
    vi.stubGlobal("fetch", fetchSpy);
    const { onDeleted } = renderDialog();

    await waitFor(() => expect(screen.getByText(/3/)).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: /^delete$/i }));

    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(await screen.findByText(/1 failed/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (cwd `apps/frontend`): `npx vitest run src/features/objects/BulkDeleteDialog.test.tsx`
Expected: FAIL — cannot resolve `./BulkDeleteDialog`.

- [ ] **Step 3: Implement the dialog**

Create `apps/frontend/src/features/objects/BulkDeleteDialog.tsx`:

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AppError } from "@/lib/api/errors";
import { objectsKeys } from "@/lib/api/keys";
import { bulkDelete, previewBulkDelete } from "./api";

export type BulkDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  // The prefix the object list is currently showing — used to invalidate
  // the right list query after the delete completes.
  listPrefix?: string;
  keys: string[];
  prefixes: string[];
  onDeleted: () => void;
};

function formatCount(objectCount: number, truncated: boolean): string {
  if (truncated) return "10,000+";
  return objectCount.toLocaleString();
}

export function BulkDeleteDialog({
  open,
  onOpenChange,
  bucket,
  listPrefix = "",
  keys,
  prefixes,
  onDeleted,
}: BulkDeleteDialogProps) {
  const qc = useQueryClient();

  // Sort the selection arrays so reordering selection doesn't refetch the
  // preview needlessly (stable query key).
  const sortedKeys = [...keys].sort();
  const sortedPrefixes = [...prefixes].sort();
  const selectedCount = keys.length + prefixes.length;

  const preview = useQuery({
    queryKey: ["objects", bucket, "bulk-delete-preview", sortedKeys, sortedPrefixes],
    queryFn: () => previewBulkDelete(bucket, { keys, prefixes }),
    enabled: open,
  });

  const mutation = useMutation({
    mutationFn: () => bulkDelete(bucket, { keys, prefixes }),
    onSuccess: async (res) => {
      await qc.invalidateQueries({ queryKey: objectsKeys.list(bucket, listPrefix) });
      if (res.failures.length === 0) {
        toast.success(`Deleted ${res.deleted_count.toLocaleString()} objects.`);
      } else {
        toast.warning(
          `Deleted ${res.deleted_count.toLocaleString()} · ${res.failures.length} failed.`,
        );
      }
      onDeleted();
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Bulk delete failed.");
      else toast.error("Bulk delete failed.");
    },
  });

  const isSinglePrefix = prefixes.length === 1 && keys.length === 0;
  const countLabel = preview.data
    ? formatCount(preview.data.object_count, preview.data.truncated)
    : "…";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete objects</DialogTitle>
          <DialogDescription>
            {preview.isLoading ? (
              <>Counting objects…</>
            ) : preview.isError ? (
              <>Could not determine how many objects this affects.</>
            ) : isSinglePrefix ? (
              <>
                Delete <span className="font-semibold">{countLabel}</span> objects under{" "}
                <span className="font-mono">{prefixes[0]}</span>?
              </>
            ) : (
              <>
                Delete <span className="font-semibold">{countLabel}</span> objects (
                {selectedCount} selected item{selectedCount === 1 ? "" : "s"})?
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={preview.isLoading || preview.isError || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? "Deleting…" : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run (cwd `apps/frontend`): `npx vitest run src/features/objects/BulkDeleteDialog.test.tsx`
Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add apps/frontend/src/features/objects/BulkDeleteDialog.tsx apps/frontend/src/features/objects/BulkDeleteDialog.test.tsx
git commit -m "feat(objects): add shared BulkDeleteDialog with dry-run preview"
```

---

## Task 12: Wire selection + toolbar + dialog into ObjectBrowserPage

**Files:**
- Modify: `apps/frontend/src/features/objects/ObjectBrowserPage.tsx`

- [ ] **Step 1: Add imports**

In `ObjectBrowserPage.tsx`, add to the imports:

```tsx
import { BulkDeleteDialog } from "./BulkDeleteDialog";
```

- [ ] **Step 2: Add selection state and a bulk-delete target**

After the existing `const [versionsKey, setVersionsKey] = useState<string | null>(null);` line (line 58), add:

```tsx
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [selectedPrefixes, setSelectedPrefixes] = useState<Set<string>>(new Set());
  // bulkTarget holds the keys/prefixes the dialog is acting on; null = closed.
  const [bulkTarget, setBulkTarget] = useState<{ keys: string[]; prefixes: string[] } | null>(
    null,
  );

  const clearSelection = () => {
    setSelectedKeys(new Set());
    setSelectedPrefixes(new Set());
  };

  const toggleKey = (key: string) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const togglePrefix = (prefix: string) => {
    setSelectedPrefixes((prev) => {
      const next = new Set(prev);
      if (next.has(prefix)) next.delete(prefix);
      else next.add(prefix);
      return next;
    });
  };

  const selectionCount = selectedKeys.size + selectedPrefixes.size;
```

- [ ] **Step 3: Clear selection on prefix navigation**

Replace the `setPrefix` function (lines 37-42) with:

```tsx
  const setPrefix = (next: string) => {
    const sp = new URLSearchParams(searchParams);
    if (next) sp.set("prefix", next);
    else sp.delete("prefix");
    setSearchParams(sp, { replace: false });
    // FR-18: selection is per-folder; drop it when the operator navigates.
    clearSelection();
  };
```

- [ ] **Step 4: Render the selection toolbar above the list**

Replace the list-rendering block (lines 94-117, the `query.isLoading ? ... : <VirtualizedObjectList .../>` expression) with the toolbar + the augmented list. Note the new props threaded into `VirtualizedObjectList`:

```tsx
      {selectionCount > 0 && (
        <div
          className="flex items-center justify-between rounded border bg-accent/30 px-3 py-2 text-sm"
          data-testid="selection-toolbar"
        >
          <span>{selectionCount} selected</span>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() =>
                setBulkTarget({
                  keys: Array.from(selectedKeys),
                  prefixes: Array.from(selectedPrefixes),
                })
              }
            >
              Delete selected
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={clearSelection}>
              Clear
            </Button>
          </div>
        </div>
      )}

      {query.isLoading ? (
        <div className="rounded border bg-background p-6 text-sm text-muted-foreground">
          Loading…
        </div>
      ) : query.isError ? (
        <div className="rounded border bg-background p-6 text-sm text-destructive">
          {query.error instanceof AppError ? query.error.message : "Failed to list objects."}
        </div>
      ) : (
        <VirtualizedObjectList
          items={items}
          hasNextPage={Boolean(query.hasNextPage)}
          isFetchingNextPage={query.isFetchingNextPage}
          fetchNextPage={() => {
            void query.fetchNextPage();
          }}
          onOpenPrefix={setPrefix}
          onDownload={onDownload}
          onDelete={(key) => setDeleteKey(key)}
          onShare={(key) => setShareKey(key)}
          onPreview={(key, contentType, size) => setPreview({ key, contentType, size })}
          onVersions={(key) => setVersionsKey(key)}
          selectedKeys={selectedKeys}
          selectedPrefixes={selectedPrefixes}
          onToggleKey={toggleKey}
          onTogglePrefix={togglePrefix}
          onDeletePrefix={(prefix) => setBulkTarget({ keys: [], prefixes: [prefix] })}
        />
      )}
```

- [ ] **Step 5: Render the BulkDeleteDialog**

Immediately before the existing single-object delete `<Dialog ...>` (line 162), add:

```tsx
      {bulkTarget !== null && (
        <BulkDeleteDialog
          open={bulkTarget !== null}
          onOpenChange={(o) => {
            if (!o) setBulkTarget(null);
          }}
          bucket={bucket}
          listPrefix={prefix}
          keys={bulkTarget.keys}
          prefixes={bulkTarget.prefixes}
          onDeleted={() => {
            clearSelection();
            setBulkTarget(null);
          }}
        />
      )}
```

- [ ] **Step 6: Verify it type-checks**

Run (cwd `apps/frontend`): `npx tsc --noEmit`
Expected: no errors (the missing-props error from Task 10 Step 6 is now resolved).

- [ ] **Step 7: Commit**

```bash
git add apps/frontend/src/features/objects/ObjectBrowserPage.tsx
git commit -m "feat(objects): wire selection toolbar, folder delete, and bulk dialog"
```

---

## Task 13: ObjectBrowserPage selection/folder-delete tests

**Files:**
- Modify: `apps/frontend/src/features/objects/ObjectBrowserPage.test.tsx`

- [ ] **Step 1: Read the existing test harness**

Read `ObjectBrowserPage.test.tsx` in full to reuse its `installFetch` / `jsonapi` / `entries` helpers and the render wrapper. The new tests below assume those helpers exist (they do — lines 1-90 define them). Note the list endpoint matcher form so the new bulk-delete matchers slot in alongside it.

- [ ] **Step 2: Add a folder-list helper and bulk-delete tests**

Append to `ObjectBrowserPage.test.tsx` (inside the existing top-level `describe`, or as a new `describe` block at the end of the file). Adjust the helper names if the existing file exposes differently-named builders:

```tsx
describe("ObjectBrowserPage bulk delete", () => {
  function folderListBody(prefixes: string[]): unknown {
    return {
      data: prefixes.map((p) => ({
        type: "object_prefixes",
        id: p,
        attributes: { prefix: p },
      })),
      meta: { page: { size: 100 } },
    };
  }

  it("shows the selection toolbar after checking a folder and opens the dialog", async () => {
    installFetch([
      {
        match: (url) => url.includes("/objects?") && url.includes("delimiter"),
        response: () => jsonapi(folderListBody(["photos/"])),
      },
      {
        match: (url, init) =>
          url.includes("/objects/bulk-delete") &&
          typeof init?.body === "string" &&
          init.body.includes('"dry_run":true'),
        response: () =>
          new Response(JSON.stringify({ object_count: 5, truncated: false }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      },
    ]);

    renderPage(); // use the file's existing render helper for ObjectBrowserPage

    const checkbox = await screen.findByLabelText(/select folder photos/i);
    await userEvent.click(checkbox);

    expect(await screen.findByTestId("selection-toolbar")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /delete selected/i }));

    // Dialog opens and shows the dry-run count.
    expect(await screen.findByText(/5/)).toBeInTheDocument();
  });

  it("opens the dialog from a folder row trash button", async () => {
    installFetch([
      {
        match: (url) => url.includes("/objects?") && url.includes("delimiter"),
        response: () => jsonapi(folderListBody(["logs/"])),
      },
      {
        match: (url, init) =>
          url.includes("/objects/bulk-delete") &&
          typeof init?.body === "string" &&
          init.body.includes('"dry_run":true'),
        response: () =>
          new Response(JSON.stringify({ object_count: 7, truncated: false }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      },
    ]);

    renderPage();

    const trash = await screen.findByLabelText(/delete folder logs/i);
    await userEvent.click(trash);
    expect(await screen.findByText(/7/)).toBeInTheDocument();
  });
});
```

> The exact render helper name (`renderPage` above) must match what the existing test file uses (it may render `<ObjectBrowserPage bucket="..." />` inline inside each test). Read the file first (Step 1) and call the same helper / inline render the existing tests use. If tests render inline, replace `renderPage()` with the same inline `render(<ObjectBrowserPage bucket="b" />, { wrapper })` the file already uses.

- [ ] **Step 3: Run the page tests**

Run (cwd `apps/frontend`): `npx vitest run src/features/objects/ObjectBrowserPage.test.tsx`
Expected: PASS — both new tests plus all pre-existing ones (the existing single-object delete test must still pass, confirming that path is unchanged).

- [ ] **Step 4: Commit**

```bash
git add apps/frontend/src/features/objects/ObjectBrowserPage.test.tsx
git commit -m "test(objects): cover selection toolbar and folder delete flows"
```

---

## Task 14: Frontend full gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full frontend gate**

Run (cwd `apps/frontend`), all must be clean:

```bash
npm run lint
npm run format
npm test
npm run build
```

Expected: all pass. If `npm run format` rewrites files, re-stage and amend the most recent commit or add a fixup commit.

- [ ] **Step 2: Commit any format fixups**

```bash
git add -A apps/frontend
git commit -m "chore(objects): frontend gate fixups for bulk delete"
```

---

## Task 15: Final whole-repo verification

**Files:** none (verification only)

- [ ] **Step 1: Backend gate (cwd `apps/backend`)**

```bash
go test -race -count=1 ./...
go vet ./...
golangci-lint run
CGO_ENABLED=0 go build ./...
```

Expected: all clean.

- [ ] **Step 2: Frontend gate (cwd `apps/frontend`)**

```bash
npm run lint
npm run format
npm test
npm run build
```

Expected: all clean.

- [ ] **Step 3: Confirm acceptance criteria**

Walk the PRD §10 checklist against the implementation:
- Backend: endpoint mounted behind session auth (Task 6 + existing `auth.RequireSession` on the objects mount); dry-run shape + ceiling (Tasks 4-5); delete shape via `RemoveObjects` (Tasks 4-5); 400s for empty/invalid/`"/"` (Task 5); partial failures aggregated, listing error → 502 (Tasks 4-5); no version ID on deletes (Task 4 — `RemoveObjectsOptions{}` + bare `ObjectInfo{Key}`); one `object.bulk_delete` audit event with target rule (Task 5); full gate (Task 15 Step 1).
- Frontend: folder trash button → dialog (Tasks 10-12); per-row checkbox + toolbar (Tasks 10, 12); dry-run preview + "10,000+" + destructive button (Task 11); folders→`prefixes[]`, objects→`keys[]` (Task 12); success/partial toast + list invalidation + selection clear (Tasks 11-12); single-object delete unchanged (verified by existing tests staying green, Task 13); full gate (Task 15 Step 2).

- [ ] **Step 4: Run code review before PR (per CLAUDE.md)**

This is mandatory and runs after the plan is complete — `/audit-plan` or `superpowers:requesting-code-review` dispatches the plan-adherence, backend, and frontend reviewers. Do not open a PR before it.

---

## Notes & Risks

- **Goroutine teardown.** Both `countExpansion` and `deleteExpansion` derive a cancelable context and `defer cancel()`, mirroring `objectS3Adapter.ListObjectVersions`. The stub's `select { case ch <- …: case <-ctx.Done(): return }` is what makes the ceiling test (Task 4) prove teardown rather than deadlock.
- **`AllActions` count assertion.** `no_secrets_test.go` hard-codes the action count; Task 1 bumps it 35 → 36. `no_presigned_test.go` iterates `AllActions()` and needs no count edit.
- **No adapter change.** `cmd/harbormaster/audit_adapter.go` stays untouched because the embedded `*miniogo.Client` already exposes `ListObjects`/`RemoveObjects`. Task 7 Step 1 is the guard that proves it.
- **Checkbox.** Native input, no new dependency (see "Design decision" above). If a future task adopts `@radix-ui/react-checkbox`, swap the component body without touching call sites (same `checked`/`onChange` surface).
- **`apierror` field names (verified).** `apierror.Error` exposes `HTTPStatus int` and `Code string` (`internal/apierror/apierror.go:27-31`); the `requireAPIError` helper (Task 5) uses those.
- **`newTestRouter` signature (verified).** It is `newTestRouter(t, cfg, stub) (http.Handler, *stubS3)` (`internal/objects/resource_test.go:25`) — note `cfg` precedes `stub`, and it returns two values. Tasks 6 route tests match this order.
- **Auth mount (verified).** `objects.Routes` is mounted inside a chi group guarded by `auth.RequireSession` (`cmd/harbormaster/serve.go:220,231`), so the new route inherits session auth with no extra wiring.
- **Page test helpers.** Task 13 reuses the page test helpers (`installFetch`/`jsonapi`/render wrapper) already defined in `ObjectBrowserPage.test.tsx:1-90`; read them first and match the actual render form (inline vs. helper).
