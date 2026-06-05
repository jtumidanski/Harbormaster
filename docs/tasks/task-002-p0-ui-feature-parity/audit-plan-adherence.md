# Plan Audit — task-002-p0-ui-feature-parity

**Plan Path:** docs/tasks/task-002-p0-ui-feature-parity/plan.md
**Audit Date:** 2026-06-05
**Branch:** task-002-p0-ui-feature-parity (worktree on `gitignore` HEAD; 48 commits vs `main`)
**Base Branch:** main

## Executive Summary

All 38 plan tasks across the six tracks (A–F) were faithfully implemented with file-level evidence — no task was skipped, stubbed, or silently deferred. The four P0 parity surfaces (object version browser/restore, noncurrent/abort-MPU lifecycle rules, custom IAM policy CRUD, Prometheus metrics dashboard) are present end-to-end (model → wire → REST → cmd wiring → React UI). All five flagged deviations from the plan were confirmed present and are acceptable; each is a correctness improvement over the plan's literal text. The full done-bar is green: backend `go test -race`, `go vet`, `golangci-lint` (0 issues), and `CGO_ENABLED=0 go build` all pass; frontend `npm run lint` (0 errors, 3 pre-existing warnings), `npm test` (93 passed), `npm run build`, and `npm run format` all pass.

## Task Completion

### Track A — Lifecycle Extensions

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| A1 | Kind constants + noncurrent/abort-mpu model fields | DONE | `internal/lifecycle/model.go:16-21` (constants), `:59-71` (NoncurrentDays/NewerNoncurrentVersions/DaysAfterInitiation) |
| A2 | Deterministic IDs + validators | DONE | `builder.go:99-109` (generate*RuleID), `:114-131` (validateNoncurrent/validateDaysAfterInitiation), `builder_test.go` |
| A3 | Classifier — three managed families | DONE | `classifier.go:14-90`; **DeleteMarker-escape fix present** (`:56-60` `DelMarkerExpiration.IsNull()`, `IsDeleteMarkerExpirationEnabled()`, `DeleteAll.IsEnabled()`, `IsDateNull()`) and foreign-Expiration rejection (`isNoncurrent/isAbortMPUShaped` use `r.Expiration.IsNull()`) |
| A4 | Processor Create paths per kind | DONE | `processor.go:235 CreateNoncurrent`, `:279 CreateAbortMPU`, `:198 upsertManaged`. `DaysAfterInitiation` correctly typed `mlifecycle.ExpirationDays(days)` (`:301`) |
| A5 | REST kind-discriminated decode + attrs | DONE | `rest.go:98 CreateRequest` superset, `:37 MarshalJSON` per-kind shapes; `resource.go:105-115` kind dispatch + `:113 unsupported_lifecycle_kind` 422 |
| A6 | Frontend kind selector | DONE | `CreateRuleDialog.tsx:42 discriminatedUnion`, `:166 showVersioningWarning`, `:204` Select items; `LifecycleRulesTab.tsx` badges; `CreateRuleDialog.test.tsx` |

### Track B — Object Version Browser + Restore

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| B1 | Audit verbs for version mutations | DONE | `audit/model.go:43-45` + registered in `AllActions()` `:84-86` |
| B2 | Version model + provider mapping | DONE | `objects/model.go` (ObjectVersion/VersionListResult), `provider.go versionFromObjectInfo`, `provider_test.go` |
| B3 | Interface split (version-list + copy/remove helpers) | DONE | `processor.go` `s3API`/`S3Client` gain `ListObjectVersions`/`CopyObject`; `administrator.go` helpers |
| B4 | Processor version ops (list/restore/delete/undelete) | DONE | `processor.go:565 ListVersions`, `:608 RestoreVersion` (`:628 cannot_restore_delete_marker`), `:654 DeleteVersion` (confirm gate), `:686 Undelete` (`:703 not_delete_marked`); `:517 encode/decodeVersionToken` |
| B5 | Version-aware download/preview + REST surface | DONE | `processor.go:318 Download(...,versionID,...)`, `:381 PresignedURL(...,versionID,...)`; `resource.go:72-75` four new routes; `rest.go` versionResource + request bodies; golden + resource tests |
| B6 | Wire version-list + copy adapter | DONE | `cmd/harbormaster/audit_adapter.go:137 ListObjectVersions`; **goroutine-leak fix present** (`:138-139 context.WithCancel(ctx)` + `defer cancel()`) |
| B7 | Frontend version history sheet | DONE | `ObjectVersionsSheet.tsx` (useInfiniteQuery, restore/delete/undelete), `VirtualizedObjectList.tsx` Versions action, `ObjectBrowserPage.tsx` wiring, `keys.ts` versions key, `ObjectVersionsSheet.test.tsx` (5 tests pass) |

### Track C — Custom / Inline IAM Policy Editor

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| C1 | Audit verbs for policy mutations | DONE | `audit/model.go:33-35` + `AllActions()` `:74-76` |
| C2 | Model + origin classifier + validators | DONE | `policies/model.go`, `classifier.go` (**case-fold guard present**: `:19 isBuiltinName` uses `strings.EqualFold`, `:31 isTemplateName` uses `strings.ToLower`), `builder.go` validators |
| C3 | Provider mapping + administrator (madmin + in-use scan) | DONE | `provider.go policyFromEntry/statementSummary`, `administrator.go listCanned/attachmentScan/containsPolicy`. Combined with C4 in commit `85eec19` to avoid a non-compiling intermediate; both tasks' content fully present |
| C4 | Processor CRUD + guards | DONE | `processor.go:199 Create`, `:236 Update`, `:276 Delete`, `:142 validateForWrite`; reserved-name/in-use/read-only guards all present; `processor_test.go` (covers all guard codes) |
| C5 | REST resource + routes | DONE w/ deviation #1 (see below) | `rest.go` policyResource/policyDetailResource, `resource.go` routes `:list/get/create/update/delete`; golden + resource tests |
| C6 | Wire policies in cmd | DONE | `audit_adapter.go:247 newPoliciesClientGetter`; `serve.go:183 policyProc`, `:244 policies.Routes(policyProc)(g)` |
| C7 | Frontend Policies page + editor | DONE | `PoliciesPage.tsx`, `PolicyEditorDialog.tsx`, `policiesApi.ts`, `ui/textarea.tsx`, `routes.tsx:48` repurposed to PoliciesPage, `PolicyTemplatesPage.tsx` removed. **policy_in_use meta fix present** end-to-end (see deviation #5) |

### Track D — User Custom-Policy Attachment

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| D1 | Audit verbs attach/detach | DONE | `audit/model.go:27-28` + `AllActions()` `:68-69` |
| D2 | users adminAPI gains canned-policy introspection | DONE | Combined with D3 (commit `6809a46`) to avoid non-compiling intermediate; `ListCannedPolicies`/`InfoCannedPolicy` on both interfaces, both tasks' content present |
| D3 | Owned-set diff including custom | DONE | `model.go:31 AttachedPolicies`, `administrator.go:98 classifyAttachments` (templates/customOwned/other), `processor.go:387 UpdatePolicies(...,customPolicies,...)`, `:428 unknown_policy`, `:540-590` per-policy Detach/Attach with audit verbs — builtins/foreign land in `other`, never touched |
| D4 | REST accept `policies`, expose `attached_policies` | DONE | `rest.go` UpdatePoliciesRequest.Policies + UserResource attached_policies; `resource.go:` handler passes `body.Policies`; golden + resource tests |
| D5 | Frontend custom-policy attach UI | DONE | `EditCustomPoliciesDialog.tsx:39 filter origin==="custom"`, `UserDetailPage.tsx:150/210` section+dialog; `EditPoliciesDialog.tsx:103` passes `currentPolicies` 3rd arg (no-drop); `EditCustomPoliciesDialog.test.tsx` |

### Track E — Prometheus Metrics Dashboard

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| E1 | Migration + GORM entity | DONE | `migrations/0007_metrics_samples.up.sql` / `.down.sql`, `metrics/entity.go` |
| E2 | Model + window validation | DONE | `model.go ParseWindow/Window/Step/Duration`; 7d step raised to keep ≤300 pts (test passes); `model_test.go` |
| E3 | Store (insert/query/retention) | DONE | `store.go Insert/Query/RetentionSweep`, `store_test.go` |
| E4 | Collector (madmin → flattened samples) | DONE | `collector.go flattenFamilies/trackedMetrics/counterMetrics/MetricsSource`, `go.mod` prom2json un-indirected; `collector_test.go` |
| E5 | Aggregator (downsample + rate) | DONE w/ deviation #3 (see below) | `aggregator.go Aggregate/downsampleRate/downsampleGauge`; `aggregator_test.go` (rate/reset/gauge all pass) |
| E6 | Processor + poller | DONE | `processor.go:33 View` + isFresh; `poller.go pollOnce/StartPoller/StartRetentionSweeper`; `poller_test.go` |
| E7 | Config poll interval + retention | DONE | `config.go:38-39` fields, `:80-81` Load, `:116-117` defaults, `:136-140` validate |
| E8 | Pool.NewMetricsClient | DONE | `minio/pool.go:75 NewMetricsClient` (reuses transport, SetCustomTransport) |
| E9 | REST GET /api/v1/metrics | DONE | `metrics/rest.go viewResponse/toResponse`, `resource.go:view` (`invalid_metrics_window` 422), `resource_test.go` |
| E10 | Wire metrics in cmd | DONE | `audit_adapter.go:354 newMetricsSourceGetter`; `serve.go:98-102` store/collector/proc/poller/sweeper, `:250 metrics.Routes(metricsProc)(g)` |
| E11 | Frontend Metrics page | DONE | `metrics/api.ts`, `types.ts`, `MetricsPage.tsx` (window persistence, paused banner, Recharts), `keys.ts metricsKeys`, `MetricsPage.test.tsx` (2 tests pass) |

### Track F — Nav / Route Integration + Final Verification

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| F1 | Metrics nav entry + route | DONE | `AppShell.tsx:7 LineChart import`, `:47 { to:"/metrics", label:"Metrics" }`; `routes.tsx:43 /metrics`, `:48 /policies → PoliciesPage` |
| F2 | Full done-bar verification | DONE | All commands re-run by this audit (see Build & Test Results); all green |

**Completion Rate:** 38/38 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

## Skipped / Deferred Tasks

None. The only explicitly deferred item is the F2 Step 3 manual smoke checklist against a live MinIO, which the plan itself permits deferring to on-demand integration/E2E per CLAUDE.md (the unit/component suite covers each surface). This is consistent with the plan's own done-bar and is not a gap.

## Known-Deviation Judgments

1. **C5 `Processor.Update` returns `error` (not `(Policy, error)`).** ACCEPTABLE. `processor.go:236 Update` returns `error`; the REST `update` handler (`resource.go`) does a read-after-write via `h.p.Get(name)` and renders the refreshed `policyResource`. The endpoint still returns the updated policy resource document (200 single), so the externally observable contract is satisfied. Cleaner than returning a `Policy` built from the request echo, since `Get` reflects MinIO's canonical stored document.

2. **`MetricsView` struct renamed to `View`.** ACCEPTABLE and consistent. `model.go:72 type View struct`; `Processor.View` returns `View` (`processor.go:33`), `Aggregate` returns `View` (`aggregator.go:9`), `toResponse(View)` (`rest.go`). No lingering `MetricsView` references. Rename avoids the revive stutter lint (`metrics.MetricsView`). The sibling `MetricsSource` interface keeps a deliberate `//nolint:revive` (`collector.go:39`) because it is the stable cross-package name.

3. **E5 aggregator uses an inter-bucket rate, replacing the plan's intra-bucket formula.** ACCEPTABLE — meets design intent. `downsampleRate` (`aggregator.go:50-101`) takes the last sample per step bucket, then computes per-second rate as `(curBucket - prevBucket)/stepSeconds` between consecutive occupied buckets, clamping negative deltas (resets) to 0 (`:85-87`), resetting continuity across gaps (`:80`), and emitting 0 for a lone bucket (`:95-98`). Gauges pass through last-value-per-bucket (`downsampleGauge`). This correctly yields ~1/s for the 0→60-over-60s fixture (the plan's intra-bucket delta would have been 0 at normal poll cadence where each bucket holds one sample). Verified by `aggregator_test.go` (rate ~1.0, reset ≥0, gauge=4) — all pass. ≤300 points is enforced by `Window.Step()` sizing (E2), unchanged.

4. **C3+C4 and D2+D3 each landed as one combined commit.** ACCEPTABLE. C3+C4 = commit `85eec19`; D2+D3 = commit `6809a46`. The plan itself warned these would not compile as separate intermediate commits (the administrator file needs `adminAPI`, the stub needs the new interface methods). Both tasks' full content is present and independently verified above (C3 provider/administrator + C4 processor CRUD; D2 interface methods + D3 owned-set diff). No content lost to the merge.

5. **Plan-snippet bug fixes.** ALL PRESENT:
   - A3 DeleteMarker escape — `classifier.go:56-60` uses the richer `IsNull()`/`IsDeleteMarkerExpirationEnabled()`/`DeleteAll.IsEnabled()`/`IsDateNull()` guards instead of the plan's `.Days == 0` shorthand, closing the foreign-action bypass.
   - B6 goroutine leak — `audit_adapter.go:138-139` cancelable context drains the SDK producer.
   - C case-fold guard bypass — `classifier.go:19/31` use `strings.EqualFold` / `strings.ToLower` so `ConsoleAdmin`/`Harbormaster-foo` cannot bypass the reserved guard.
   - C7 `policy_in_use` JSON:API error `meta` — `apierror/apierror.go:68` carries `Meta: ae.Details`; frontend `lib/api/errors.ts` parses `e.meta` into `details`; `PoliciesPage.tsx:70-76` reads `details.attached_to.users/groups`. Covered by `apierror_test.go:33-72`.

## Build & Test Results

| Service | Build | Tests | Vet | Lint | Notes |
|---------|-------|-------|-----|------|-------|
| apps/backend | PASS | PASS | PASS | PASS | `go test -race -count=1 ./...` all `ok`; `go vet ./...` clean; `golangci-lint run` → 0 issues; `CGO_ENABLED=0 go build ./...` clean |
| apps/frontend | PASS | PASS | PASS (0 errors) | — | `npm test` 24 files / 93 tests passed; `npm run build` built in 5.47s; `npm run format` clean; `npm run lint` 0 errors, 3 warnings |

Frontend lint warnings (3, all `react-hooks/exhaustive-deps`) are in **pre-existing** files not materially changed by this task: `service-accounts/CreateServiceAccountDialog.tsx`, `users/CreateUserDialog.tsx`, and `users/EditPoliciesDialog.tsx` (the last had only a 4-line caller change at `:103` that did not introduce the warned `useMemo` pattern). They are non-blocking and not regressions from this branch's new code.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE

## Action Items

None required for plan adherence. Optional (non-blocking) follow-ups:

1. Consider clearing the 3 pre-existing `react-hooks/exhaustive-deps` warnings in a separate cleanup (out of scope for this task).
2. Run the F2 Step 3 manual/integration smoke against a live MinIO before release to confirm the `trackedMetrics` Prometheus family names (E4) match the deployed MinIO version, as the plan flagged.
