# Backend Audit — task-002-p0-ui-feature-parity

- **Scope:** Go changes on branch `task-002-p0-ui-feature-parity` vs `main` (`internal/{lifecycle,objects,policies,metrics,users,audit,config,minio,jsonapi,apierror}`, `cmd/harbormaster`, `migrations/0007_*`)
- **Backend root:** `apps/backend`
- **Guidelines source:** backend-dev-guidelines skill (DOM/SUB/SEC principles)
- **Date:** 2026-06-05
- **Build:** PASS
- **Tests:** PASS (race, all packages)
- **Lint:** PASS (golangci-lint, 0 issues)
- **Vet:** PASS
- **Overall:** PASS (with one non-blocking gofmt nit)

## Architecture note (not a finding)

This service does NOT use the api2go / `server.RegisterHandler` / `server.RegisterInputHandler[T]`
transport template literally described in the skill resources. It uses a chi router with the
project's own `internal/jsonapi` codec and `internal/apierror` envelope (two styles:
`StyleJSONAPI` for resource routes, `StyleAction` for action routes). The DOM/SUB checklist
IDs that name api2go-specific symbols (`RegisterInputHandler`, `Transform`/`TransformSlice`,
`Make`/`ToEntity`, `database.Query`) do not map 1:1. The audit below evaluates the *principles*
those checks encode — DDD layering, immutability, error→status mapping, audit coverage, no
manual envelope hand-rolling beyond the established action-endpoint convention — against the
actual, internally-consistent conventions. This deviation is pre-existing and project-wide, not
introduced by this task, so it is not scored as a failure.

## Done-Bar Results (verified)

| Command | Result |
|---------|--------|
| `CGO_ENABLED=0 go build ./...` | PASS (exit 0) |
| `go vet ./...` | PASS (exit 0) |
| `go test -race -count=1 ./...` | PASS — every package `ok`, 0 failures |
| `$HOME/go/bin/golangci-lint run` | PASS — "0 issues" |

## Domain: policies (NEW)

| ID (principle) | Check | Status | Evidence |
|----|-------|--------|----------|
| Layering | model→provider→administrator→processor→resource→rest | PASS | model.go, provider.go, administrator.go, processor.go, resource.go, rest.go all present and correctly separated |
| Immutable model | Policy/PolicyDetail value types, no exported mutators | PASS | policies/model.go:16-28 |
| Small unexported interface + exported mirror | `adminAPI` (unexported) + `AdminClient` (exported) + `NewClientGetter` | PASS | administrator.go:87-95, processor.go:22-49 |
| ClientGetter injection | Processor depends only on `ClientGetter` | PASS | processor.go:36-71 |
| Handlers call processor only | list/get/create/update/delete → `h.p.*` | PASS | resource.go:52-132 |
| Error→status mapping | 422 name/json/structure, 409 reserved/in-use, 403 read-only, 502 minio | PASS | processor.go:142-165, 199-324 |
| Reserved-name guard | builtin + template prefix rejected (409) before write | PASS | processor.go:148-151 |
| Read-only guard | `!EditableFor` → 403 before doc validation | PASS | processor.go:240-243 (Update), 280-283 (Delete) |
| In-use guard | `attachmentScan` users+groups → 409 `policy_in_use` with `attached_to` meta | PASS | processor.go:288-307, administrator.go:35-65 |
| Audit on mutation (failure+success) | Create/Update/Delete both outcomes | PASS | Create processor.go:200-225; Update 237-266; Delete 277-322 |
| Audit payload carries no policy document | payload = `{policy: name}` only | PASS | processor.go:132-135, 221-223, 261-263, 318-320 |

## Domain: metrics (NEW, read-only + background poller)

| ID (principle) | Check | Status | Evidence |
|----|-------|--------|----------|
| Layering | store(persistence)→collector→aggregator→processor→resource | PASS | store.go, collector.go, aggregator.go, processor.go, resource.go, rest.go |
| Read-only endpoint, no mutation audit needed | GET /metrics only | PASS | resource.go:18-49 |
| Endpoint behind auth | mounted under `protectedRoutes` (session+CSRF) | PASS | serve.go:218-251 (metrics.Routes at 250) |
| Poller goroutine lifecycle: ctx cancellation | `select { case <-ctx.Done(): return }`, `defer ticker.Stop()` | PASS | poller.go:29-44 (StartPoller), 49-70 (StartRetentionSweeper) |
| Poller wired to server ctx (no leak) | `StartPoller(ctx, …)` with cobra cmd ctx | PASS | serve.go:101-102 |
| minio-go producer goroutine teardown | `context.WithCancel` + `defer cancel()` on every return of the WithVersions drain | PASS | cmd/harbormaster/audit_adapter.go:137-157 |
| Opaque version cursor | base64 raw-url offset token | PASS | objects/processor.go encode/decodeVersionToken; metrics window is enum-only |
| Store uses WithContext for query/insert | Insert/Query use `db.WithContext(ctx)` | PASS | store.go:31, 41 |

## Domain: objects (version browser/restore/delete/undelete)

| ID (principle) | Check | Status | Evidence |
|----|-------|--------|----------|
| s3API split: version-list + copy added to unexported + exported mirror | both `s3API` and `S3Client` carry `ListObjectVersions`+`CopyObject` | PASS | processor.go:53-66, 80-93 |
| ObjectVersion immutable, delete-marker Size=nil | versionFromObjectInfo | PASS | provider.go (versionFromObjectInfo); model ObjectVersion |
| ListVersions read-only, opaque cursor, sibling filter | offset window + exact-key filter | PASS | processor.go:565-604; administrator.go:77-89 |
| RestoreVersion: reject delete-marker, audit both outcomes | 422 `cannot_restore_delete_marker`; failure+success audit | PASS | processor.go:608-649 |
| DeleteVersion: confirm gate, audit both outcomes | 422 unless `confirm:true`; failure+success audit | PASS | processor.go:654-681 |
| Undelete: not-delete-marked guard, audit both outcomes | 422 `not_delete_marked`; failure+success audit | PASS | processor.go:686-719 |
| Audit payload carries no body/presigned/secret | payloads = bucket/key/version_id only | PASS | processor.go:609, 655, 687 (sanitize also applies, see SEC) |

## Domain: users (custom-policy attachment, depends on policies)

| ID (principle) | Check | Status | Evidence |
|----|-------|--------|----------|
| UpdatePolicies diff: never detach builtin/foreign | diff operates only on `currentManaged`/`currentCustom` from `classifyAttachments`; `other` (builtins/foreign) never in either set | PASS | processor.go:437-444, 477-590; resolveDeploymentCustom 595-607 |
| Requested custom policies validated against deployment-custom set | unknown → 422 `unknown_policy` with pointer | PASS | processor.go:426-431 |
| Per-policy attach/detach audit (success) + umbrella failure audit | diffCustomPolicies records each Attach/Detach; failAudit on error | PASS | processor.go:553-588, 393-405 |
| Generated secret never retained / never audited | secret returned once, payload = access_key/templates/policies | PASS | processor.go:207-281 (payload 208-211), 388-392 |
| Layering + ClientGetter + exported mirror | adminAPI/AdminClient/NewClientGetter | PASS | processor.go:23-71 |

## Domain: lifecycle (noncurrent + abort-mpu kinds)

| ID (principle) | Check | Status | Evidence |
|----|-------|--------|----------|
| Kind-discriminated create dispatch | switch on `attrs.Kind`; unknown → 422 `unsupported_lifecycle_kind` + pointer | PASS | resource.go:104-117 |
| Validators per kind | validateNoncurrent / validateDaysAfterInitiation | PASS | builder.go (per plan A2); processor.go:235-307 |
| Managed-iff invariant preserved | per-family `isXShaped` + ID regex; foreign attr flips unmanaged | PASS | classifier.go (isExpirationShaped/isNoncurrentShaped/isAbortMPUShaped) |
| Audit on Create*/Delete (failure+success) | lifecycleFailAudit + upsertManaged success; Delete both outcomes | PASS | processor.go:151,179-185,218-224,245,288,319-363 |
| Discriminated REST marshal per kind | RuleResource.MarshalJSON emits only that kind's fields | PASS | rest.go (per plan A5) |

## Cross-cutting: audit no-secrets enforcement (SEC)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-04 | Sanitize called unconditionally on every Record | PASS | audit/processor.go:30-44 (line 38 always) |
| SEC-04 | Sensitive-key drop covers secret/password/token/csrf/signature/presigned/url, recursive | PASS | audit/sanitize.go:8, 13-29 |
| SEC-04 | Key-based no-secrets test over ALL 35 actions | PASS | audit/no_secrets_test.go:17-62 (asserts Len==35) |
| SEC-04 | Value-based no-presigned-URL structural test over ALL 35 actions (JSON-walk every leaf) | PASS | audit/no_presigned_test.go:36-103 |
| SEC | Policy documents never enter audit payloads | PASS | policies/processor.go payloads = `{policy: name}` only |
| SEC | No hardcoded secrets/keys in changed source | PASS | grep across changed dirs — none |

## Cross-cutting: error envelope / meta details path (SEC + REST)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| meta path | apierror `Details` → jsonapi.Error `Meta` → wire `meta` | PASS | apierror.go:66-69; jsonapi/errors.go:24-25,49-51 |
| in-use details surface under errors[].meta.attached_to | WithDetails(attached_to) → meta | PASS | policies/processor.go:298-306 + above |
| action style emits details under error.details | PASS | apierror.go:75-82, struct tag model.go-equivalent at apierror.go:31 |

## Cross-cutting: config / wiring

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| No `os.Getenv()` in handlers/domain | env read once in config via viper, injected | PASS | grep: 0 matches in internal/{objects,lifecycle,policies,metrics,users}; config.go:77-80 |
| New config validated + defaulted | METRICS_POLL_INTERVAL / METRICS_RETENTION positive-checked, defaulted | PASS | config.go (defaults 113-116, validate 133-140) |
| Domain packages never import live pool type | all via `New*ClientGetter` adapters at wiring site | PASS | cmd/harbormaster/audit_adapter.go:164-255; serve.go:143-185 |

## Summary

### Blocking (must fix)
- None. Build, vet, race tests, and golangci-lint all pass; every SEC invariant in scope
  (audit coverage on all mutations with failure+success; no secrets/documents/presigned URLs in
  audit payloads; reserved-name/read-only/in-use policy guards; never-detach-builtin/foreign user
  invariant; meta details path; opaque version cursor; poller goroutine lifecycle) is satisfied
  with file:line evidence above.

### Non-Blocking (should fix)
- **gofmt not clean (style nit).** 6 task-changed non-test files are not `gofmt`-clean
  (`gofmt -l` flags map-literal alignment etc.). Concretely introduced by this task:
  `internal/objects/resource.go:128` (`"restored_from":  body.VersionID` — extra space).
  Also flagged: `internal/metrics/collector.go`, `internal/metrics/rest.go`,
  `internal/policies/processor.go`, `internal/lifecycle/resource.go`, `internal/users/rest.go`.
  This does NOT fail the done-bar: `.golangci.yml` declares an empty `formatters:` block (v2),
  so `golangci-lint run` does not enforce gofmt, and the project's stated done-bar
  (build/vet/golangci-lint/test) omits bare `gofmt`. Recommend `gofmt -w` on the changed files.
- **metrics Store.RetentionSweep omits `WithContext`** (`internal/metrics/store.go:60` uses bare
  `s.db`). Background sweeper with its own cutoff, so no request-context propagation is lost;
  cosmetic consistency only vs `Insert`/`Query` which do use `WithContext`.

### Informational (not findings)
- `internal/objects/administrator.go:82` `out := infos[:0]` filters in place over the caller's
  slice. Safe here: write index never overtakes read index, and `infos` is freshly allocated per
  call by the adapter (`ListObjectVersions` returns a new slice), so there is no shared-state
  aliasing hazard. Verified, not a bug.
- The `json.NewDecoder` body parsing in objects/users action handlers
  (e.g. resource.go restoreVersion/deleteVersion/undelete) matches the repo's pre-existing
  action-endpoint convention (StyleAction), not the api2go `RegisterInputHandler` template the
  skill describes. Consistent and pre-existing; not scored as a violation.
