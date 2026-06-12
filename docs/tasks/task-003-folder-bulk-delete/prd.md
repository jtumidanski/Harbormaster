# Folder Delete & Multi-Select Bulk Delete — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-06-12
---

## 1. Overview

The bucket object browser currently supports deleting **one object at a time** via a per-row trash button (`apps/frontend/src/features/objects/VirtualizedObjectList.tsx:193`), backed by `DELETE /api/v1/buckets/{bucket}/objects?key=…` → `Processor.Delete` (`apps/backend/internal/objects/processor.go:270`). Folders (S3 common prefixes) are navigation-only and have no actions; there is no way to remove a folder and its contents, nor to delete several objects in one action. Operators clearing out a homelab bucket today must delete files individually — impractical for anything beyond a handful of objects.

This feature adds two related capabilities to the object browser:

1. **Folder delete** — a per-folder action that recursively removes every object under a prefix.
2. **Multi-select bulk delete** — row checkboxes plus a selection toolbar that deletes any mix of selected objects and folders in one action.

Both are powered by a single new server-side bulk-delete endpoint that expands prefixes server-side and batch-deletes via MinIO's `RemoveObjects` channel API. This keeps deletes fast (one round-trip, server-side batching of up to 1000 keys per call), gives clean audit semantics (one event per operation), and avoids a chatty client-side delete loop.

The deletion semantics intentionally mirror the existing single-object delete: a delete with no version ID creates a delete marker on a **versioned** bucket (recoverable via the existing undelete flow) and permanently removes the object on an **unversioned** bucket. No new recovery behavior is introduced.

## 2. Goals

Primary goals:
- Add a recursive **folder (prefix) delete** action to folder rows in the object browser.
- Add **multi-select bulk delete** for any mix of objects and folders via row checkboxes and a selection toolbar.
- Introduce a single server-side bulk-delete endpoint that expands prefixes and batch-deletes via `RemoveObjects`.
- Show a **dry-run object-count preview** in the confirmation dialog before the operator commits.
- Report **partial failures** (per-key errors) back to the operator after the delete.
- Record exactly **one audit event per bulk operation**.

Non-goals:
- Streaming / live progress UI (progress bars, per-object tick). The batched call returns a summary; that is sufficient.
- Typed-text confirmation (e.g. "type the folder name to confirm"). A plain destructive confirm button is used.
- A "restore folder" / bulk-undelete feature. Per-object undelete already exists for versioned buckets (`Processor.Undelete`).
- Any change to the existing single-object delete fast path (per-row trash button + `Processor.Delete`).
- Cross-bucket or whole-bucket delete (a separate "empty bucket" job already exists in `internal/jobs/bucketempty`).
- Object-version-targeted bulk delete (selecting specific versions to delete in bulk).

## 3. User Stories

- As an operator, I want to delete an entire folder and everything under it in one action, so that I don't have to delete hundreds of objects individually.
- As an operator, I want to select several objects (and/or folders) with checkboxes and delete them together, so that I can clean up a bucket efficiently.
- As an operator, I want to see how many objects a folder/bulk delete will affect **before** I confirm, so that I don't accidentally delete far more than I intended.
- As an operator, I want to be told which objects failed to delete (if any), so that I can retry or investigate rather than assume everything succeeded.
- As a security-conscious admin, I want each bulk delete recorded in the audit log with the actor, target, and count, so that destructive actions remain traceable.

## 4. Functional Requirements

### 4.1 Bulk-delete primitive (backend)

- FR-1: A new endpoint `POST /api/v1/buckets/{bucket}/objects/bulk-delete` accepts a JSON body with `keys` (array of object keys), `prefixes` (array of folder prefixes), and `dry_run` (boolean).
- FR-2: At least one of `keys` or `prefixes` must be non-empty; an otherwise-empty request returns `400 bad_request`.
- FR-3: Every entry in `keys` is validated with the existing `ValidateObjectKey`. An invalid key returns `400 object_invalid_key`.
- FR-4: Each entry in `prefixes` MUST be non-empty and not equal to `"/"`. An empty or `"/"` prefix is rejected with `400 bad_request` ("prefix must not be empty") to prevent an accidental whole-bucket wipe (an empty prefix matches every object).
- FR-5: Each prefix is expanded by recursively listing all object keys under it (delimiter-less listing).
- FR-6: When `dry_run: true`, the endpoint expands prefixes and counts the affected objects (explicit `keys` + all keys under each prefix) WITHOUT deleting anything, returning `{ "object_count": N, "truncated": bool }`. The count is exact up to a ceiling of **10,000**; beyond that, counting stops, `object_count` is reported as `10000` and `truncated: true` (the UI renders this as "10,000+"). De-duplication of overlapping keys/prefixes is best-effort and not required for the count.
- FR-7: When `dry_run: false`, the endpoint deletes the explicit keys plus all keys expanded from prefixes, using MinIO's batched `RemoveObjects` (batches of up to 1000), and returns `{ "deleted_count": N, "failures": [{ "key": "...", "error": "..." }] }`. The delete itself is NOT capped at 10,000.
- FR-8: Deletes are issued WITHOUT a version ID, so behavior matches the existing single-object delete: delete marker on a versioned bucket, permanent removal on an unversioned bucket.
- FR-9: A transport/listing error during expansion aborts the operation and surfaces as the standard `502 minio_error` envelope (consistent with `mapClientError`). Per-key delete failures reported on the `RemoveObjects` error channel are collected into `failures[]` and do NOT abort the remaining deletes.
- FR-10: `deleted_count` reflects keys submitted to `RemoveObjects` minus those that returned an error (i.e. confirmed-deleted; `RemoveObjects` signals success by silence on the error channel, matching the `drainWithOpts` convention in `internal/jobs/bucketempty/worker.go:46`).

### 4.2 Folder delete (frontend)

- FR-11: Each folder row in `VirtualizedObjectList` gains a trash-icon action button (folders currently have none — `VirtualizedObjectList.tsx:126`).
- FR-12: Clicking a folder's trash button opens the shared bulk-delete confirmation dialog scoped to that single prefix.
- FR-13: The dialog issues a `dry_run` request on open and displays "Delete **N** objects under `<folder>/`?" using the returned count (rendering "10,000+" when `truncated`).

### 4.3 Multi-select bulk delete (frontend)

- FR-14: Each row (both `object_entries` and `object_prefixes`) gains a selection checkbox.
- FR-15: A selection toolbar appears above the list only when at least one row is selected, showing the selected count, a "Delete selected" destructive action, and a "Clear" action.
- FR-16: "Delete selected" opens the shared confirmation dialog. Selected folders are sent as `prefixes[]` and selected objects as `keys[]`.
- FR-17: The dialog's count preview reflects the dry-run total across all selected keys and prefixes (e.g. "Delete **1,243** objects (8 selected items)?"). Selection state is held in `ObjectBrowserPage`.
- FR-18: Selection is cleared after a successful delete and when the operator navigates to a different prefix.

### 4.4 Confirmation & result (frontend)

- FR-19: The confirmation dialog uses a plain destructive "Delete" button (no typed confirmation). While the dry-run count is loading, the button is disabled and the dialog shows a loading state.
- FR-20: On confirm, the dialog fires the real (`dry_run: false`) bulk-delete, disabling the button while pending.
- FR-21: On success with no failures, a success toast reports the deleted count (e.g. "Deleted 1,243 objects."). On partial failure, a warning/error toast reports both counts (e.g. "Deleted 1,240 · 3 failed."). The dialog may surface the first few failing keys.
- FR-22: After completion (success or partial), the object list query for the current prefix is invalidated (`objectsKeys.list(bucket, prefix)`) so the view refreshes.

## 5. API Surface

### New endpoint

`POST /api/v1/buckets/{bucket}/objects/bulk-delete`

Mounted alongside the existing object routes in `Routes` (`apps/backend/internal/objects/resource.go:64`).

**Request body** (plain JSON, mirroring the action-style endpoints like restore/undelete):

```json
{
  "keys": ["reports/q1.pdf", "notes.txt"],
  "prefixes": ["photos/2024/", "logs/"],
  "dry_run": false
}
```

**Dry-run response** (`200 OK`, `dry_run: true`):

```json
{ "object_count": 1243, "truncated": false }
```

**Delete response** (`200 OK`, `dry_run: false`):

```json
{
  "deleted_count": 1240,
  "failures": [
    { "key": "logs/locked.bin", "error": "object is WORM-locked" }
  ]
}
```

**Error envelopes** — action style (`apierror.StyleAction`), consistent with the existing single-object delete handler:
- `400 bad_request` — neither keys nor prefixes provided; empty/`"/"` prefix.
- `400 object_invalid_key` — an invalid object key.
- `502 minio_error` — transport/listing failure during prefix expansion.

### Unchanged endpoints

The existing `DELETE /buckets/{bucket}/objects?key=…` single-object delete is retained unchanged as the per-row fast path.

## 6. Data Model

No persistent data-model changes. Object deletion is a MinIO operation with no Harbormaster-side persistence (the objects domain has no GORM tables — `Processor` has no DB handle, per `processor.go:136`).

**Audit model change** (`apps/backend/internal/audit/model.go`):
- Add a new action constant `ActionObjectBulkDelete = "object.bulk_delete"` alongside `ActionObjectDelete` (`model.go:19`), and register it in the known-actions list (`model.go:59`).
- One `audit.Event` is recorded per bulk operation (success or failure), with `Action = object.bulk_delete`, `TargetType = "object"`, `TargetID = bucket` (or `bucket + "/" + prefix` for a single-prefix folder delete), and a `PayloadSummary` of `{ "key_count": K, "prefixes": [...], "deleted_count": N, "failure_count": F }`. Per-object events are NOT emitted (would flood the log).

## 7. Service Impact

### Backend (`apps/backend`)

- `internal/objects/resource.go` — register the `POST .../objects/bulk-delete` route in `Routes`; add a `bulkDelete` handler that decodes the JSON body and calls the processor, rendering action-style success/error.
- `internal/objects/processor.go` — add `Processor.BulkDelete(ctx, bucket, keys, prefixes, dryRun, actor, sourceIP) (BulkDeleteResult, error)`. Extend the unexported `s3API` interface (`processor.go:53`) and its public twin `S3Client` (`processor.go:80`) with the two methods the bulk path needs: high-level recursive `ListObjects(ctx, bucket, opts) <-chan ObjectInfo` and `RemoveObjects(ctx, bucket, objectsCh, opts) <-chan RemoveObjectError`. Add the batching/expansion helper (mirroring `drainWithOpts` from `internal/jobs/bucketempty/worker.go:46`).
- `internal/objects/model.go` — add request/result types (`BulkDeleteRequest`, `BulkDeleteResult`, failure entry).
- `internal/objects/provider.go` — no new mapping required beyond reusing `entryFromObjectInfo`; the bulk path works on raw keys.
- `cmd/harbormaster` — extend the live minio-go adapter that satisfies `S3Client` so it also exposes `ListObjects` and `RemoveObjects` (both already exist on `*miniogo.Client`; the `bucketempty` worker already uses them via its own interface, confirming availability).
- `internal/audit/model.go` — add and register `ActionObjectBulkDelete`.

### Frontend (`apps/frontend`)

- `src/features/objects/api.ts` — add `bulkDelete(bucket, { keys, prefixes, dryRun })` plus request/response types.
- `src/features/objects/VirtualizedObjectList.tsx` — add a trash button to folder rows; add a selection checkbox to every row; thread new callbacks (`onDeletePrefix`, selection state/handlers) through props.
- `src/features/objects/ObjectBrowserPage.tsx` — hold selection state; render the selection toolbar; render a new `BulkDeleteDialog`; wire dry-run preview + real delete mutations; clear selection on success and on prefix navigation.
- `src/features/objects/BulkDeleteDialog.tsx` (new) — confirmation dialog that fetches the dry-run count, renders the preview, fires the real delete, and reports the summary.

## 8. Non-Functional Requirements

- **Performance:** Prefix expansion and deletion use MinIO's batched `RemoveObjects` (≤1000 keys/batch) over a streamed recursive listing, so memory stays bounded regardless of folder size. A single bulk delete is one client→server round-trip (plus one dry-run round-trip for the preview).
- **Safety:** Empty/`"/"` prefixes are rejected server-side (FR-4). The dry-run count preview gives the operator a chance to abort before deleting. Versioned buckets retain recoverability (delete markers) exactly as the single-object delete does today.
- **Security / authorization:** The new route mounts behind the same `auth.RequireSession` middleware as the rest of the object endpoints; the actor and source IP are pulled from the session via `actorFromRequest` for the audit event.
- **Observability:** One audit event per operation with actor, target, counts, and outcome. Listing/transport failures surface via the standard `minio_error` envelope and the access log.
- **Consistency:** Deletion semantics, error-envelope style (action), and the request-body convention (plain JSON like restore/undelete) all follow existing patterns in the objects package.

## 9. Open Questions

- None outstanding. All key decisions were resolved during brainstorming:
  - Scope = both folder delete and multi-select bulk delete.
  - Execution = new server-side bulk endpoint (not a client-side loop).
  - Confirmation = dry-run count preview + plain destructive button (no typed confirmation).
  - Count ceiling = exact to 10,000, then "10,000+"; the delete itself is uncapped.

## 10. Acceptance Criteria

Backend:
- [ ] `POST /api/v1/buckets/{bucket}/objects/bulk-delete` exists and is mounted behind session auth.
- [ ] `dry_run: true` returns `{ object_count, truncated }` and deletes nothing; count is exact to 10,000 then reports `10000` + `truncated: true`.
- [ ] `dry_run: false` deletes explicit keys + all keys under each prefix via `RemoveObjects`, returning `{ deleted_count, failures[] }`.
- [ ] Empty request (no keys, no prefixes) → `400`; empty or `"/"` prefix → `400`; invalid key → `400 object_invalid_key`.
- [ ] Per-key delete failures are aggregated into `failures[]` without aborting the rest; a listing/transport error aborts with `502 minio_error`.
- [ ] Deletes issue no version ID (delete marker on versioned, permanent on unversioned).
- [ ] Exactly one `object.bulk_delete` audit event is recorded per operation with actor, target, and counts.
- [ ] `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`, and `CGO_ENABLED=0 go build ./...` all pass (processor unit tests with the in-memory `s3API` stub cover expansion, empty-prefix rejection, dry-run count + ceiling, and partial-failure aggregation; resource tests cover the route).

Frontend:
- [ ] Folder rows show a trash button that opens the confirmation dialog scoped to that prefix.
- [ ] Every row has a selection checkbox; a selection toolbar appears when ≥1 row is selected, with selected count, "Delete selected", and "Clear".
- [ ] The confirmation dialog shows a dry-run count preview (and "10,000+" when truncated) and a plain destructive Delete button.
- [ ] Selected folders are sent as `prefixes[]`, selected objects as `keys[]`.
- [ ] Success and partial-failure outcomes are reported via toast; the list refreshes (query invalidation) afterward; selection clears on success and on prefix navigation.
- [ ] The existing single-object delete (per-row trash + dialog) is unchanged.
- [ ] `npm run lint`, `npm run format`, `npm test`, and `npm run build` all pass (tests cover the count-preview flow, selection toolbar, folder-delete button, and partial-failure toast).
