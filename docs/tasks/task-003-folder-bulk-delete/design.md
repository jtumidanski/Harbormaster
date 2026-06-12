# Folder Delete & Multi-Select Bulk Delete — Design

Status: Proposed
Created: 2026-06-12
PRD: `docs/tasks/task-003-folder-bulk-delete/prd.md`

---

## 1. Summary

This feature adds recursive **folder (prefix) delete** and **multi-select bulk delete** to the object
browser, both powered by a single new backend endpoint
`POST /api/v1/buckets/{bucket}/objects/bulk-delete`. The endpoint expands prefixes server-side via a
streamed recursive listing and batch-deletes through MinIO's channel-based `RemoveObjects`, returning a
dry-run object count (for the pre-confirm preview) or a delete summary with per-key failures. One audit
event is recorded per real delete operation.

The design's load-bearing decisions are: (a) extend the existing `s3API`/`S3Client` interfaces with the
two channel-based MinIO methods rather than reaching into another package's client, (b) add an
objects-package-local expansion/batching helper modelled on — but not importing — `bucketempty`'s
`drainWithOpts`, because the failure semantics differ, and (c) on the frontend, a single shared
`BulkDeleteDialog` serves both the folder-delete and multi-select entry points, with selection state
owned by `ObjectBrowserPage`.

Nothing here changes the existing single-object delete fast path (`Processor.Delete` +
`VirtualizedObjectList` per-row trash button).

---

## 2. Backend architecture

### 2.1 Interface extension (the elegant part)

The processor consumes MinIO through the unexported `s3API` interface
(`internal/objects/processor.go:53`) and its exported twin `S3Client` (`processor.go:80`). The bulk path
needs two methods neither currently exposes:

```go
ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
```

Both are added to **both** interfaces (the two are intentionally kept in lockstep — see the doc comment
at `processor.go:74`). The key payoff: the live adapter `objectS3Adapter`
(`cmd/harbormaster/audit_adapter.go:108`) embeds `*miniogo.Client`, and `*miniogo.Client` already has
`ListObjects` and `RemoveObjects` natively. So extending the interface requires **zero new adapter
code** — the embedded client satisfies the larger interface automatically. This is the same reason
`bucketempty`'s own `s3Iface` (`internal/jobs/bucketempty/worker.go:13`) can name those methods directly.

The only non-trivial wiring cost lands in the **test stub** (`internal/objects/helpers_test.go:25`,
`stubS3`), which must grow channel-returning `ListObjects`/`RemoveObjects` implementations. See §6.

### 2.2 New processor method

```go
func (p *Processor) BulkDelete(
    ctx context.Context,
    bucket string,
    keys []string,
    prefixes []string,
    dryRun bool,
    actor, sourceIP string,
) (BulkDeleteResult, error)
```

`BulkDeleteResult` is a single struct serving both modes (the REST layer renders the relevant subset):

```go
type BulkDeleteResult struct {
    // Dry-run fields
    ObjectCount int  // explicit keys + expanded prefix keys, exact up to the ceiling
    Truncated   bool // count hit the 10,000 ceiling

    // Delete fields
    DeletedCount int
    Failures     []BulkDeleteFailure
}

type BulkDeleteFailure struct {
    Key   string
    Error string
}
```

Request body type lives alongside the other action requests in `rest.go`:

```go
type BulkDeleteRequest struct {
    Keys     []string `json:"keys"`
    Prefixes []string `json:"prefixes"`
    DryRun   bool     `json:"dry_run"`
}
```

### 2.3 Validation (runs before any MinIO call)

In order, all returning action-style `apierror`:

1. **`keys` and `prefixes` both empty** → `400 bad_request` ("at least one of keys or prefixes is
   required"). (FR-2)
2. **Each key** → `ValidateObjectKey`; first invalid → `400 object_invalid_key`. (FR-3)
3. **Each prefix** non-empty and `!= "/"`; otherwise `400 bad_request` ("prefix must not be empty").
   This is the whole-bucket-wipe guard — an empty prefix matches every object. (FR-4)

These validation rejects do **not** write an audit event (see §2.6).

### 2.4 Prefix expansion + batching helper

A new file `internal/objects/bulkdelete.go` holds the expansion/batch machinery. It deliberately
**mirrors** `bucketempty.drainWithOpts` (`worker.go:46`) rather than importing it, because the failure
contract is different (CLAUDE.md "Code Patterns": don't cross service boundaries to reuse internals; a
small duplicated loop is cleaner than coupling the objects domain to the jobs domain).

Two entry points share one streamed-listing core:

- **Count path (`dryRun: true`)** — recursively list each prefix
  (`miniogo.ListObjectsOptions{Prefix: prefix, Recursive: true}`), incrementing a counter, plus
  `len(keys)` for the explicit keys. Stop and set `Truncated` once the running count reaches the
  **10,000 ceiling**; report `ObjectCount = 10000`. De-dup of overlapping keys/prefixes is **not**
  performed (PRD FR-6: best-effort). Deletes nothing.

- **Delete path (`dryRun: false`)** — feed explicit `keys` (as `miniogo.ObjectInfo{Key: k}`) and every
  key streamed from each prefix listing into `RemoveObjects` in **batches of ≤1000**. The delete is
  **uncapped** (no 10,000 ceiling — FR-7). `RemoveObjects` signals per-key success by *silence* on its
  error channel; each `RemoveObjectError` with a non-nil `Err` becomes a `BulkDeleteFailure`.
  `DeletedCount = (keys submitted) − (failures)` (FR-10), matching the `drainWithOpts` convention.

**Deletes carry no version ID** — `RemoveObjectsOptions{}` and bare `ObjectInfo{Key}` (the prefix-listed
infos are from a non-versioned `Recursive` listing, so they too carry no `VersionID`). This makes a
delete marker on a versioned bucket and a permanent removal on an unversioned bucket, exactly mirroring
the single-object `Processor.Delete` semantics (FR-8).

#### Goroutine teardown (correctness)

`miniogo.Client.ListObjects` spawns a producer goroutine that blocks on channel sends until the
consumer drains the channel or the context is cancelled. The count path **breaks early** when it hits
the ceiling, and any path may return early on error — so the helper derives a cancelable context
(`ctx, cancel := context.WithCancel(ctx); defer cancel()`) and ranges over the listing channel under it.
This is the same leak-avoidance the existing `objectS3Adapter.ListObjectVersions`
(`audit_adapter.go:137`) already applies; the bulk helper must not regress it.

#### Error handling during expansion

A listing error (`obj.Err != nil` on the channel, or a transport failure) **aborts** the whole
operation and surfaces via `mapClientError` as the standard `502 minio_error` envelope (FR-9,
consistent with `processor.go:500`). Per-key delete failures on the `RemoveObjects` error channel do
**not** abort — they accumulate into `Failures[]` and the loop continues.

### 2.5 REST handler + route

- Register `r.Post("/buckets/{bucket}/objects/bulk-delete", h.bulkDelete)` in `Routes`
  (`resource.go:64`), mounted behind the same `auth.RequireSession` middleware as the rest of the object
  routes.
- `bulkDelete` decodes `BulkDeleteRequest` (plain JSON, mirroring `restoreVersion`/`undelete` at
  `resource.go:333`/`379`), pulls `actor, ip := actorFromRequest(r)`, calls `p.BulkDelete`, and renders
  **action-style**:
  - error → `apierror.Write(w, apierror.StyleAction, err)`
  - success → `writeActionJSON(w, http.StatusOK, body)` (`resource.go:25`) where `body` is
    `{object_count, truncated}` for dry-run or `{deleted_count, failures}` for delete.

A malformed JSON body returns `400 bad_request` ("invalid JSON body"), matching the existing action
handlers.

### 2.6 Audit semantics

One `audit.Event` per **real delete** operation (success, partial, or total failure):

- `Action = audit.ActionObjectBulkDelete` — new constant `"object.bulk_delete"` added at
  `audit/model.go:19` and registered in `AllActions()` (`model.go:50`). A `golden_test`/known-actions
  test enumerates the action space, so the registration is enforced.
- `TargetType = "object"`.
- `TargetID = bucket`, except a **single-prefix folder delete** (exactly one prefix, no explicit keys)
  uses `bucket + "/" + prefix` so folder deletes are individually traceable (PRD §6).
- `PayloadSummary = {key_count, prefixes, deleted_count, failure_count}`.
- `Outcome` = success when `len(Failures) == 0`, else failure (with the count context in the payload).

**Dry-run records no audit event** (it deletes nothing — it is a read-only preview). **Pure validation
rejects** (the 400s in §2.3) also record nothing. Rationale: the acceptance criterion is "exactly one
event per *operation* with actor, target, **and counts**" — only the delete path has counts. This is a
deliberate, small divergence from `Processor.Delete`, which records a failure audit even on an invalid
key; the bulk operation treats the dry-run/validation phase as pre-operation. Audit writes remain
best-effort via the nil-safe `recordAudit` (`processor.go:176`) and never block the foreground result.

---

## 3. Backend alternatives & tradeoffs

**A. Reuse `bucketempty.drainWithOpts` vs. a new objects-local helper — chose new helper.**
`drainWithOpts` aborts on the first per-key error and lives in the jobs domain. The bulk endpoint must
*collect* per-key failures without aborting, seed the batch with explicit keys, and support a count-only
ceiling mode — three behaviours the existing helper doesn't have. Importing it would also couple
`objects` → `jobs`. The cost of a fresh helper is ~60 lines of duplicated batching loop; the benefit is
clean boundaries and the exact failure contract the PRD specifies. **Recommended and adopted.**

**B. One `BulkDelete(…, dryRun)` method vs. two methods (`CountBulkDelete` + `BulkDelete`).**
The PRD names a single method. A single method keeps the validation (§2.3) and prefix-expansion core in
one place with a single branch on `dryRun`. Two methods would duplicate validation. **Adopted: one
method**, with the count/delete divergence isolated inside the helper.

**C. Stream-and-batch vs. collect-all-keys-then-delete.** Collecting every key into a slice before
deleting is simpler to read but unbounded in memory for large folders. Streaming the listing straight
into ≤1000-key `RemoveObjects` batches keeps memory at O(batch) regardless of folder size (PRD
non-functional: "memory stays bounded"). **Adopted: stream-and-batch.**

**D. Dedup overlapping keys/prefixes.** Skipped per PRD FR-6 (best-effort). Real-world overlap is rare,
exact dedup needs a full in-memory key set (defeating the memory bound), and `RemoveObjects` tolerates a
duplicate key (the second delete is a no-op or a harmless not-found). The count may slightly over-report
on overlap; acceptable. **Adopted: no dedup.**

---

## 4. Frontend architecture

### 4.1 Selection state (owned by `ObjectBrowserPage`)

Selection lives in `ObjectBrowserPage` (`ObjectBrowserPage.tsx:32`), as two sets keyed by their natural
identifiers:

```ts
const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());      // object keys
const [selectedPrefixes, setSelectedPrefixes] = useState<Set<string>>(new Set()); // prefix names
```

Two sets (rather than one set of tagged composite ids) is chosen because the bulk-delete request
partitions exactly along this line — `keys[]` vs `prefixes[]` — so submission is a direct
`Array.from(set)` with no parsing. A row's checked state is `selectedKeys.has(e.key)` /
`selectedPrefixes.has(p.prefix)`. Because selection state lives above the virtualizer, it survives row
recycling for free.

Selection is cleared (FR-18): on successful delete, and on prefix navigation — the existing `setPrefix`
wrapper (`ObjectBrowserPage.tsx:37`) gains a `setSelectedKeys(new Set()); setSelectedPrefixes(new Set())`.

### 4.2 `VirtualizedObjectList` changes

- New props: `selectedKeys`, `selectedPrefixes`, `onToggleKey(key)`, `onTogglePrefix(prefix)`,
  `onDeletePrefix(prefix)`.
- **Both** row branches (`object_prefixes` at `:126`, `object_entries` at `:146`) gain a leading
  shadcn `Checkbox` bound to the matching set + toggle handler.
- **Folder rows** gain a trailing `Trash2` ghost button (folders have no actions today — `:126`) wired
  to `onDeletePrefix(p.prefix)`, visually matching the per-object trash button (`:193`).
- The 36px row (`ESTIMATED_ROW_HEIGHT`) absorbs a checkbox inline; no height change needed.

### 4.3 Selection toolbar

Rendered in `ObjectBrowserPage` directly above `VirtualizedObjectList`, only when
`selectedKeys.size + selectedPrefixes.size > 0` (FR-15). Shows the selected count, a destructive
"Delete selected" button (opens the shared dialog with the current selection), and "Clear" (resets both
sets).

### 4.4 Shared `BulkDeleteDialog` (new component)

A single dialog serves both entry points (FR-12 folder delete, FR-16 multi-select):

```ts
type BulkDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  keys: string[];
  prefixes: string[];
  onDeleted: () => void; // page clears selection + the mutation invalidates the list
};
```

- Folder-delete entry: `keys=[]`, `prefixes=[thatPrefix]`.
- Multi-select entry: the selected keys and prefixes.

**Dry-run preview** uses React Query `useQuery` keyed on `[...objectsKeys..., 'bulk-delete-preview',
bucket, sortedKeys, sortedPrefixes]`, `enabled: open`. While loading, the dialog shows a loading state
and the Delete button is disabled (FR-19). The dialog copy adapts:
- single prefix, no keys → "Delete **N** objects under `<folder>/`?" (FR-13)
- mixed selection → "Delete **N** objects (`M` selected items)?" (FR-17)
- truncated → render the count as **"10,000+"** (FR-6/FR-13).

**Confirm** fires the real delete via `useMutation` (`dry_run: false`), disabling the button while
pending (FR-20). On settle:
- no failures → `toast.success("Deleted N objects.")`
- partial → `toast.warning("Deleted N · F failed.")`, optionally listing the first few failing keys (FR-21).
- always → invalidate `objectsKeys.list(bucket, prefix)` and call `onDeleted()` (FR-22).

A plain destructive button is used — **no typed confirmation** (PRD non-goal).

### 4.5 API client (`features/objects/api.ts`)

Two thin functions over the one endpoint (clearer call sites than a single function with a
`dryRun`-discriminated union return):

```ts
export async function previewBulkDelete(bucket, { keys, prefixes }): Promise<{ object_count: number; truncated: boolean }>;
export async function bulkDelete(bucket, { keys, prefixes }): Promise<{ deleted_count: number; failures: { key: string; error: string }[] }>;
```

Both `POST .../objects/bulk-delete` with `dry_run` true/false respectively. Both rely on the existing
`api.post` action-style error handling (non-2xx → `AppError`), already exercised by `restore`/`undelete`.

---

## 5. Frontend alternatives & tradeoffs

**E. Selection: two sets vs. one set of composite ids.** One `Set<string>` of `${type}:${id}` dedups
uniformly and mirrors the virtualizer's row key, but every submission and every checked-state read must
parse the tag. Two sets read trivially and submit directly as `keys[]`/`prefixes[]`. **Adopted: two
sets.**

**F. Shared dialog vs. two dialogs.** Folder delete and multi-select share identical mechanics (dry-run
preview → confirm → summary toast → invalidate); only the initial `{keys, prefixes}` differ. One shared
`BulkDeleteDialog` removes duplication and keeps the count/preview/failure logic in one tested place.
**Adopted: shared dialog.**

**G. Dry-run via `useQuery` vs. `useMutation`-on-open.** The dry-run is semantically a read (deletes
nothing), so `useQuery` gives loading/error/refetch state and caching for free, re-running automatically
when the selection (query key) changes. A `useMutation` fired from a `useEffect` would re-implement that
state machine by hand. **Adopted: `useQuery`** (POST-backed query is fine here).

---

## 6. Testing strategy

**Backend (`internal/objects`)**

- Extend `stubS3` (`helpers_test.go:25`) with channel-based `ListObjects` and `RemoveObjects`. The stub
  holds a configurable map of prefix → keys and an optional per-key error set; `ListObjects` emits the
  matching `ObjectInfo`s on a freshly-created-and-closed channel; `RemoveObjects` drains its input
  channel and emits a `RemoveObjectError` for each key in the configured error set. This is the trickiest
  test-infra piece and is shared by all `BulkDelete` cases.
- `processor` unit tests cover: prefix expansion (key set = explicit + expanded); empty-request reject
  (400); empty/`"/"` prefix reject (400); invalid key reject (400 `object_invalid_key`); dry-run count
  exactness; dry-run **ceiling** (>10,000 → `object_count == 10000`, `truncated == true`, and the
  producer goroutine is torn down — assert no deadlock); partial-failure aggregation (`failures[]`
  populated, `deleted_count` excludes them, non-failing keys still deleted); listing error → 502; no
  version ID on deletes; audit recorded once on the delete path with the right `TargetID` (bucket vs
  `bucket/prefix`) and **not** recorded for dry-run/validation rejects.
- `resource_test.go` covers the route: dry-run JSON shape, delete JSON shape, and the action-style error
  envelope for the 400s.
- `audit` known-actions/golden test picks up the new constant automatically (registration enforced).

**Frontend (`features/objects`)**

- Count-preview flow: dialog fires `previewBulkDelete` on open, shows the count, "10,000+" when
  truncated, Delete disabled while loading.
- Selection toolbar: appears at ≥1 selected, shows count, Clear resets, "Delete selected" opens the
  dialog with the right `{keys, prefixes}`.
- Folder-delete button: folder row trash opens the dialog scoped to that single prefix.
- Partial-failure toast: a delete response with `failures[]` renders the "Deleted N · F failed" warning
  and the list query is invalidated.
- Selection clears on success and on prefix navigation.

**Full gate** (CLAUDE.md): backend `go test -race`, `go vet`, `golangci-lint run`, `CGO_ENABLED=0 go
build`; frontend `npm run lint`, `format`, `test`, `build`.

---

## 7. Files touched

**Backend**
- `internal/objects/processor.go` — extend `s3API` + `S3Client` (two methods); add `Processor.BulkDelete`.
- `internal/objects/bulkdelete.go` *(new)* — expansion/batching helper (count + delete cores, cancelable ctx).
- `internal/objects/model.go` — `BulkDeleteResult`, `BulkDeleteFailure`.
- `internal/objects/rest.go` — `BulkDeleteRequest`.
- `internal/objects/resource.go` — route + `bulkDelete` handler.
- `internal/audit/model.go` — `ActionObjectBulkDelete` constant + `AllActions` registration.
- `internal/objects/helpers_test.go`, `processor_test.go`, `resource_test.go` — stub channel methods + cases.
- `cmd/harbormaster/audit_adapter.go` — **no change** (embedded `*miniogo.Client` already satisfies the wider interface).

**Frontend**
- `src/features/objects/api.ts` — `previewBulkDelete`, `bulkDelete` + types.
- `src/features/objects/BulkDeleteDialog.tsx` *(new)*.
- `src/features/objects/VirtualizedObjectList.tsx` — checkboxes + folder trash button + new props.
- `src/features/objects/ObjectBrowserPage.tsx` — selection state, toolbar, dialog wiring, clear-on-nav.

---

## 8. Risks & mitigations

- **Goroutine leak on early break (dry-run ceiling / error).** Mitigated by the cancelable-context
  pattern (§2.4), asserted by a ceiling test.
- **Test stub channel semantics.** A naively-written stub that never closes its channel deadlocks the
  test. The stub must close emitted channels; covered by §6.
- **Large multi-select count keys.** The dry-run `useQuery` key encodes the selection arrays; sort them
  for stable keys so reordering selection doesn't refetch needlessly (§4.4).
- **Whole-bucket-wipe via empty prefix.** Hard server-side reject of empty/`"/"` prefix (§2.3, FR-4) —
  the primary safety control, independent of the UI.
