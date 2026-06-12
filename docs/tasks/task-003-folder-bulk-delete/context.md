# Context — Folder Delete & Multi-Select Bulk Delete (task-003)

Companion to `plan.md`. Captures the key files, decisions, and verified facts an engineer needs before executing, so each task can be implemented without re-deriving the surrounding code.

## What this builds

One backend endpoint `POST /api/v1/buckets/{bucket}/objects/bulk-delete` plus two frontend entry points (folder-row trash button + multi-select toolbar) that both drive a shared confirmation dialog. The endpoint expands folder prefixes server-side and batch-deletes via MinIO's channel-based `RemoveObjects`, returning a dry-run object count (preview) or a delete summary with per-key failures. Deletion semantics mirror the existing single-object delete exactly (no version ID → delete marker on versioned buckets, permanent on unversioned).

## Key backend files

| File | Role | Touch |
|------|------|-------|
| `internal/objects/processor.go` | Domain orchestrator. `s3API` (unexported, :53) + `S3Client` (exported, :80) interfaces kept in lockstep. `recordAudit` (:176) is nil-safe. `mapClientError` (:500) → 502 `minio_error`. `clients()` (:484) resolves the stub/live client. | Extend both interfaces (+2 methods); add `BulkDelete`. |
| `internal/objects/bulkdelete.go` *(new)* | Expansion/batching helpers `countExpansion` + `deleteExpansion`; constants `bulkDeleteCeiling=10000`, `bulkRemoveBatchN=1000`. | Create. |
| `internal/objects/model.go` | Domain types (no JSON tags; wire shape lives in `rest.go`). | Add `BulkDeleteResult`, `BulkDeleteFailure`. |
| `internal/objects/rest.go` | JSON:API resource wrappers + request body types (`RestoreVersionRequest`, `UndeleteRequest`, …). | Add `BulkDeleteRequest`. |
| `internal/objects/resource.go` | Routes registrar (:64) + handlers. `writeActionJSON` (:25) writes plain JSON; `actorFromRequest` (:38) pulls actor/IP from session. Action errors use `apierror.Write(w, apierror.StyleAction, err)`. | Add route + `bulkDelete` handler. |
| `internal/objects/helpers_test.go` | White-box `stubS3` fake (:25) + `newTestProcessor` (:245). | Add channel `ListObjects`/`RemoveObjects` + control fields. |
| `internal/objects/resource_test.go` | HTTP-level tests via `newTestRouter(t, cfg, stub)` (:25) → `(http.Handler, *stubS3)`. | Add 3 route tests. |
| `internal/audit/model.go` | Action constants (:11) + `AllActions()` (:50). | Add + register `ActionObjectBulkDelete`. |
| `internal/audit/no_secrets_test.go` | `require.Len(allActions, 35, …)` at :23. | Bump 35 → 36. |
| `internal/jobs/bucketempty/worker.go` | **Reference, not imported.** `drainWithOpts` (:46) is the batching pattern the new helper mirrors (but with collect-don't-abort failure semantics). | None. |
| `cmd/harbormaster/audit_adapter.go` | Live adapter `objectS3Adapter` (:108) embeds `*miniogo.Client`; compile-time assertion `var _ objects.S3Client = objectS3Adapter{}` (:260). | **None** — embedded client already has `ListObjects`/`RemoveObjects`. |

## Key frontend files

| File | Role | Touch |
|------|------|-------|
| `src/components/ui/checkbox.tsx` *(new)* | shadcn-style Checkbox over a native input (no new dep). | Create. |
| `src/features/objects/api.ts` | Object API client (`listObjects`, `deleteObject`, `restoreVersion`, …). Uses `api.post<T>(path, body)`. | Add `previewBulkDelete`, `bulkDelete` + types. |
| `src/features/objects/BulkDeleteDialog.tsx` *(new)* | Shared dialog: `useQuery` dry-run preview + `useMutation` real delete + toasts + list invalidation. | Create. |
| `src/features/objects/VirtualizedObjectList.tsx` | Virtualized list. Folder rows (`object_prefixes`, :126) had no actions; object rows (:146) have Download/Share/Versions/Delete. 36px rows. | Add per-row checkbox + folder trash + 5 props. |
| `src/features/objects/ObjectBrowserPage.tsx` | Owns prefix nav (`setPrefix`, :37), single-delete dialog, query (`useInfiniteObjects`). | Add selection state, toolbar, `BulkDeleteDialog`, clear-on-nav. |
| `src/features/objects/types.ts` | Wire types `ObjectListItem` (discriminated on `type`), `ObjectEntry`, `ObjectPrefix`. | None (reuse). |
| `src/lib/api/keys.ts` | Query keys. `objectsKeys.list(bucket, prefix)` is what to invalidate. | None (preview key inlined in dialog). |

## Load-bearing decisions

1. **Interface extension over cross-package reuse.** Add `ListObjects`/`RemoveObjects` to `s3API`+`S3Client`. The live adapter embeds `*miniogo.Client`, which has both natively → zero adapter code. The `bucketempty` worker proves these methods exist on the real client. (design §2.1)
2. **New objects-local helper, not `bucketempty.drainWithOpts`.** The bulk endpoint must *collect* per-key failures (not abort-on-first), seed the batch with explicit keys, and support a count-only ceiling mode — three behaviors the jobs helper lacks. Importing it would also couple `objects` → `jobs` (against CLAUDE.md "Code Patterns"). ~60 lines duplicated batching loop is the accepted cost. (design §2.4, alt A)
3. **One `BulkDelete(…, dryRun)` method**, single validation path, branch on `dryRun`. (design alt B)
4. **Stream-and-batch, not collect-all-then-delete** — keeps memory O(batch) for huge folders. (design alt C)
5. **No dedup** of overlapping keys/prefixes (PRD FR-6 best-effort); count may slightly over-report. (design alt D)
6. **Goroutine teardown** via `context.WithCancel` + `defer cancel()` in both helpers, mirroring `objectS3Adapter.ListObjectVersions` (:137). The stub's `select { case ch<-…: case <-ctx.Done() }` makes the ceiling test prove teardown, not deadlock. (design §2.4)
7. **Audit:** one event per *real delete* only. Dry-run and pure validation rejects record nothing (the acceptance criterion is "one event per operation with counts" — only deletes have counts). `TargetID = bucket`, except single-prefix folder delete (1 prefix, 0 keys) → `bucket/prefix`. (design §2.6)
8. **Frontend selection = two `Set<string>`** (`selectedKeys`, `selectedPrefixes`), owned by `ObjectBrowserPage`, because the request partitions exactly along `keys[]`/`prefixes[]` → `Array.from(set)` with no parsing. Survives virtualizer row recycling for free. (design §4.1, alt E)
9. **Shared dialog** for both entry points; only the initial `{keys, prefixes}` differ. (design §4.4, alt F)
10. **Dry-run via `useQuery`** (it's a read), keyed on sorted selection arrays so reorder doesn't refetch. (design §4.4, alt G)
11. **Checkbox = native input** (plan-level decision): `@radix-ui/react-checkbox` is not a dependency and no `ui/checkbox.tsx` exists; a native input avoids an `npm install` while keeping a single reusable component with the `checked`/`onChange` surface.

## Verified facts (don't re-derive)

- `apierror.Error` fields: `HTTPStatus int`, `Code string` (`internal/apierror/apierror.go:27-31`). `apierror.New(status, code, msg)`. Action style = `apierror.StyleAction`.
- `newTestRouter(t, cfg, stub) (http.Handler, *stubS3)` — `cfg` before `stub`, returns two values (`resource_test.go:25`).
- Objects routes mount behind `auth.RequireSession` (`cmd/harbormaster/serve.go:220` group `.Use`, `:231` `objects.Routes`). New route inherits session auth automatically.
- `AllActions()` currently has 35 entries; `no_secrets_test.go:23` hard-asserts 35; `no_presigned_test.go:43` iterates `AllActions()` (no count edit needed there).
- `ValidateObjectKey` (`internal/objects/builder.go:15`) rejects empty, >1024 bytes, and NUL bytes.
- `miniogo.RemoveObjectError` has `ObjectName string`, `VersionID string`, `Err error`. `RemoveObjects` signals success by *silence* (only failures appear on the channel) — confirmed by `bucketempty/worker.go:71-74`.
- `api.post<T>(path, body?)` returns parsed JSON for 2xx and throws `AppError` on non-2xx (`src/lib/api/client.ts:68`). `restoreVersion`/`undeleteObject` already use this against plain-JSON action endpoints — the bulk endpoint follows the same shape.
- Existing frontend tests stub `fetch` via `installFetch` + `jsonapi` helpers and patch `ResizeObserver`/`getBoundingClientRect` for the virtualizer (`ObjectBrowserPage.test.tsx:1-90`).

## Build gate (a branch is "done" only when all are clean)

Backend (cwd `apps/backend`): `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build ./...`.
Frontend (cwd `apps/frontend`): `npm run lint`, `npm run format`, `npm test`, `npm run build`.

After implementation: run code review (`/audit-plan` or `superpowers:requesting-code-review`) before opening a PR — mandatory per CLAUDE.md.

## Out of scope (PRD non-goals)

Streaming/live progress UI; typed-text confirmation; bulk-undelete/restore; changes to the single-object delete fast path; whole-bucket/cross-bucket delete (separate `bucketempty` job); version-targeted bulk delete.
