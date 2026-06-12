# Backend Audit — objects (task-003 folder bulk-delete)

- **Scope:** changed Go packages in range `818f5ed..HEAD` — `internal/objects` (bulkdelete.go, processor.go, resource.go, rest.go, model.go, builder.go usage) and `internal/audit` (model.go, no_secrets_test.go)
- **Guidelines Source:** backend-dev-guidelines skill (DOM-*, SUB-*, SEC-*)
- **Date:** 2026-06-12
- **Build:** PASS (`CGO_ENABLED=0 go build ./...` exit 0)
- **Vet:** PASS (`go vet ./...` exit 0)
- **Lint:** PASS (`golangci-lint run ./internal/objects/... ./internal/audit/...` — 0 issues)
- **Tests:** PASS (`go test -race -count=1 ./...` — full backend clean; objects + audit packages `ok`)
- **Overall:** PASS

## Architecture note (checklist applicability)

The classic DOM-* / SUB-* checklist assumes a GORM-backed DDD domain package
(`model.go` + `entity.go` + `builder.go` + `provider.go` + `administrator.go` +
JSON:API `Transform`/`TransformSlice` via `server.RegisterInputHandler`). The
`objects` package is deliberately **not** that shape: it is a stateless MinIO
proxy (model.go:8-9 — "the source of truth is MinIO itself — nothing in this
package persists local state"). There is no local DB, no GORM entity, no
provider/administrator split, and the HTTP layer is chi + a hand-rolled
`jsonapi.Encoder`, not `server.RegisterHandler`. This is the established
convention for the whole service (buckets, connection, audit packages share it).

Consequently the DB/GORM-specific checks (DOM-01 builder invariants, DOM-02/03
Make/ToEntity, DOM-10 lazy providers, DOM-14 db.Create, DOM-15 administrator,
DOM-17/18 JSON:API request models, SUB-02 administrator, SUB-03
RegisterInputHandler) are **N/A** — they describe a persistence layer this
package does not and should not have. Items are marked N/A with that rationale
rather than forced to FAIL, because failing them would be enforcing a rule the
guideline does not impose on a no-persistence proxy domain. The
layer-separation, error-handling, logger, cross-domain, and security checks
**do** apply and were enforced.

## Domain Checklist Results

### internal/objects (stateless proxy domain)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | builder.go / validated construction | N/A (PASS-equiv) | No domain model to build; `builder.go:15` holds `ValidateObjectKey`, the edge validator actually used (processor.go:805, :297, :578…) |
| DOM-02 | ToEntity() | N/A | No GORM entity — no local persistence (model.go:8-9) |
| DOM-03 | Make(Entity) | N/A | Same — domain types built from `miniogo.ObjectInfo`, e.g. `entryFromObjectInfo`, `versionFromObjectInfo` |
| DOM-04 | Transform function | N/A | Package uses hand-rolled `jsonapi.Encoder` resource wrappers (rest.go:12-59) not api2go `Transform`; consistent with buckets/audit |
| DOM-05 | TransformSlice / no inline loops | N/A | Encoder `Collection` consumes resource slices (resource.go:106-126); no api2go transform layer exists here |
| DOM-06 | Processor accepts FieldLogger | N/A (variant) | Package uses `zerolog.Logger` not `logrus`; `Processor.Logger` defaults to `zerolog.Nop()` (processor.go:159,168), service-wide convention |
| DOM-07 | Handlers pass d.Logger() | N/A (variant) | No `logrus.StandardLogger()`; logger injected at construction via `WithLogger` (processor.go:172) |
| DOM-08 | POST/PATCH typed input handler | N/A (variant) | chi handlers, not `server.RegisterInputHandler`; bulk-delete decodes a flat `BulkDeleteRequest` (rest.go:110-114, resource.go:433-438) — consistent with restore/undelete |
| DOM-09 | Transform errors handled | PASS | No discarded-error Transform pattern; bulk-delete error path checks every return (processor.go:816,822,829; resource.go:442-445) |
| DOM-10 | Lazy providers | N/A | No provider layer — direct MinIO SDK calls |
| DOM-11 | No os.Getenv in handlers | PASS | grep `os.Getenv` in resource.go → zero matches; config read from `Processor.Config` (resource.go:144,254) |
| DOM-12 | No cross-domain logic in handlers | PASS | `bulkDelete` handler (resource.go:431-463) only calls `h.p.BulkDelete`; no foreign-domain orchestration |
| DOM-13 | Handlers don't call providers directly | PASS | Handler → processor only; all S3 work behind `p.BulkDelete` (resource.go:441) |
| DOM-14 | No direct entity creation in handlers | PASS (vacuous) | No DB; no `db.Create/Save/Delete` anywhere in package |
| DOM-15 | administrator.go for writes | N/A | No DB writes; MinIO writes encapsulated in processor + `bulkdelete.go` helpers |
| DOM-16 | Domain error → HTTP status mapping | PASS | 400 bad_request (processor.go:801,811), 400 object_invalid_key (processor.go:806), 502 minio_error via `mapClientError` (processor.go:520,823,830); verified by tests (bulkdelete_test.go:107,113,119,125,165) |
| DOM-17 | JSON:API interface on REST models | N/A (variant) | Hand-rolled `ResourceType()`/`ResourceID()`/`MarshalJSON()` (rest.go:17-21,49-59); bulk-delete returns plain-JSON action shape by design (resource.go:25-31,447-462) |
| DOM-18 | Flat request models | PASS | `BulkDeleteRequest` is flat, no nested Data/Type/Attributes (rest.go:110-114) |
| DOM-19 | Table-driven / behavioral tests | PASS | Focused behavioral tests over a real in-memory stub with goroutine-accurate channel semantics (bulkdelete_test.go, resource_test.go:493-576) |

## Sub-Domain Checklist Results

Bulk-delete is implemented as a **method on the existing objects Processor**
(`BulkDelete`, processor.go:799) plus two package-private helpers
(`countExpansion`/`deleteExpansion`, bulkdelete.go), not a separate
action-event package. This is exactly the guideline's prescribed alternative
("fold the action into the parent domain's processor as a method instead of
creating a separate package with layer violations" — anti-patterns.md:156).

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Business logic not in handler | PASS | All expansion/batch/audit logic in processor + bulkdelete.go; handler is thin (resource.go:431-463) |
| SUB-02 | Writes not done in handler | PASS | Deletes issued in `deleteExpansion` (bulkdelete.go:80) called by processor, never the handler |
| SUB-03 | Typed POST input | PASS (variant) | Flat `BulkDeleteRequest` decoded once (resource.go:433-438) |
| SUB-04 | No manual JSON parsing of envelope | PARTIAL | `json.NewDecoder(r.Body).Decode(&body)` (resource.go:434) — see Finding I3. This is the package-wide action-endpoint convention (restore/undelete/share-link), not new drift; flat body, no JSON:API envelope hand-rolling |

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-route | Endpoint inherits session auth | PASS | `objects.Routes` mounted inside the `RequireSession` + `RequireCSRF` group (serve.go:219-231); the new `/bulk-delete` route (resource.go:76) is in that same `Routes` set |
| SEC-wipe | Whole-bucket-wipe guard | PASS | Empty or `"/"` prefix → 400 bad_request **before** any MinIO call (processor.go:809-813); tests TestBulkDelete_EmptyPrefix_400 / _SlashPrefix_400 (bulkdelete_test.go:110-120). Empty request (no keys, no prefixes) → 400 (processor.go:800-803) |
| SEC-key | Object-key validation | PASS | Every explicit key run through `ValidateObjectKey` before expansion (processor.go:804-808); rejects empty / >1024B / NUL (builder.go:15-29); test TestBulkDelete_InvalidKey_400 (bulkdelete_test.go:122-126) |
| SEC-ver | No version ID on delete (delete-marker parity) | PASS | `miniogo.ObjectInfo{Key: key}` + `miniogo.RemoveObjectsOptions{}` — no VersionID (bulkdelete.go:91,80), matching single-`Delete` (processor.go:304 `removeObject` carries no version). Mirrors delete-marker semantics |
| SEC-502 | Transport/listing error → 502 minio_error | PASS | Listing error aborts and maps via `mapClientError(...,"minio_error",502)` (bulkdelete.go:43,104 → processor.go:823,830,520); test TestBulkDelete_ListError_502 asserts 502/minio_error (bulkdelete_test.go:161-166) |
| SEC-audit | One audit event per real delete; correct target-id; none on dry-run/reject | PASS | Exactly one `recordAudit` on the real-delete path (processor.go:844-857); dry-run returns before audit (processor.go:820-826); pure rejects return before audit (processor.go:800-813). target-id = `bucket/prefix` only for single-prefix/no-keys, else `bucket` (processor.go:836-839). Outcome flips to failure when any per-key failure exists (processor.go:840-843) |
| SEC-leak | Audit payload cannot leak secrets/URLs | PASS | `Record` calls `Sanitize` unconditionally (processor.go:30,38); regex drops secret/password/token/csrf/signature/presigned/url keys (sanitize.go:8). Bulk-delete payload carries only key_count/prefixes/deleted_count/failure_count (processor.go:851-856) — no URLs or secrets. no_secrets_test enumerates all 36 actions incl. `object.bulk_delete` (no_secrets_test.go:23, model.go:46) |
| SEC-04 | No hardcoded secrets | PASS | None in changed files |

## Goroutine / Resource-Leak Review

| Concern | Status | Evidence |
|---------|--------|----------|
| `countExpansion` producer teardown | PASS | `cctx, cancel := context.WithCancel(ctx)` + `defer cancel()` (bulkdelete.go:31-32); fires on early ceiling break (bulkdelete.go:46) and on listing-error return (bulkdelete.go:42). Stub producer honours `ctx.Done()` on send (helpers_test.go:297-303), proving no leaked goroutine when the consumer stops early |
| `deleteExpansion` producer teardown | PASS | `defer cancel()` (bulkdelete.go:64-65); listing-error return (bulkdelete.go:104) still triggers the deferred cancel, unblocking both the ListObjects producer and the RemoveObjects worker (helpers_test.go:320-322) |
| RemoveObjects channel draining | PASS | `flush` fully drains `errCh` (bulkdelete.go:81-85) before returning, so the worker goroutine completes; objCh is closed after filling (bulkdelete.go:79) |
| Race detector | PASS | `go test -race` clean across objects package (incl. the channel-heavy bulk-delete tests) |

## Interface Lockstep Review

| Concern | Status | Evidence |
|---------|--------|----------|
| `s3API` vs `S3Client` parity | PASS | Both interfaces add identical `ListObjects` (processor.go:68 / :101) and `RemoveObjects` (processor.go:71 / :104) signatures; `NewClientGetter` adapter still compiles (processor.go:112-120) and full build passes |
| Stub implements new methods | PASS | `stubS3.ListObjects` / `RemoveObjects` implemented (helpers_test.go:283,309); package builds and tests under `-race` |

## Other Findings (non-blocking, by severity)

### Info / Nits (non-blocking)
- **I1 (info):** `BulkDeleteResult` overloads one struct for two response shapes (dry-run vs delete) with comments delineating which fields apply (model.go:76-84). The handler picks the subset by `DryRun` (resource.go:447-462). Acceptable; documented. No action required.
- **I2 (info):** Over-report on overlapping keys/prefixes is intentional and documented as best-effort per PRD (bulkdelete.go:23-25). Count may exceed the true unique set; not a correctness bug for a preview number.
- **I3 (info):** `bulkDelete` handler uses `json.NewDecoder(r.Body).Decode` (resource.go:434) rather than `server.RegisterInputHandler[T]`. This is *not* JSON:API-envelope hand-rolling (the body is a flat plain-JSON action body, matching restore-version/undelete/share-link in the same file) and is the deliberate package convention for action endpoints, so it does not trip the "manual JSON:API envelope handling" anti-pattern. Flagged only so reviewers know it was considered, not missed.
- **I4 (info):** `deleteExpansion` returns `0, nil, err` on a listing error mid-run (bulkdelete.go:104), discarding any already-submitted count. The processor maps that to a 502 and returns an empty `BulkDeleteResult` (processor.go:829-830), so no misleading partial count is ever surfaced to the client. Correct.
- **I5 (info):** `errFailing` has a redundant `var _ = errFailing` keep-alive (helpers_test.go:281) whose comment claims "no negative tests use it," yet three tests now reference it (bulkdelete_test.go:52,97,162). The keep-alive and its stale comment are now dead. Trivial test-only cleanup; non-blocking.

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- I5: remove the now-stale `var _ = errFailing` keep-alive and its misleading comment (helpers_test.go:279-281).

### Verdict
PASS. All four gates clean (build, vet, lint 0-issues, `go test -race`). Every
security-relevant requirement in the focus areas — whole-bucket-wipe guard,
object-key validation, version-ID-free delete-marker parity, 502 error mapping,
one-audit-per-delete with correct target-id and no audit on dry-run/reject,
session-auth inheritance, and goroutine teardown via cancel-on-every-return —
is implemented and proven by file:line evidence and passing tests. The DOM/SUB
items marked N/A reflect this package's intentional no-persistence proxy
architecture, not missing work.
