# P0 UI Feature Parity — Design

Version: v1
Status: Approved-pending-review
Created: 2026-05-28
PRD: `docs/tasks/task-002-p0-ui-feature-parity/prd.md`

---

## 0. Scope & Guiding Principles

This design closes the four P0 gaps from the PRD as one coordinated release:

1. Object version-history browser + restore (extends `internal/objects`).
2. Noncurrent-version & abort-incomplete-multipart lifecycle rules (extends `internal/lifecycle`).
3. Custom/inline IAM policy editor (new `/policies` surface in `internal/policies`, plus user attachment in `internal/users`).
4. Prometheus metrics dashboard (new `internal/metrics` domain + background poller + SQLite store).

Every feature obeys the conventions verified in the existing code:

- **Per-domain DDD layering** `model → builder → provider → administrator → processor → resource → rest`. The processor is the orchestration core; it depends only on small unexported interfaces (`s3API`/`adminAPI`) injected via a `ClientGetter`, with an exported public interface + `NewClientGetter` adapter wired in `cmd/harbormaster/serve.go`. Tests stub the private interface (see `objects/processor.go:44-94`, `users/processor.go`).
- **MinIO is the source of truth.** No MinIO-authoritative state is persisted for versions, lifecycle, or policies. The only new persistence is metrics samples.
- **JSON:API for resources, plain JSON for actions.** Resources implement `jsonapi.Resource` (`ResourceType`/`ResourceID`/`MarshalJSON`) and are encoded via `jsonapi.Single`/`Collection`. Action endpoints return plain JSON. Errors flow through `apierror` with typed codes, rendered `StyleJSONAPI` for resource routes and `StyleAction` for action routes (`apierror/apierror.go:26-73`).
- **Audit every mutation** via `processor.recordAudit(ctx, audit.Event{...})`, best-effort and nil-safe (`objects/processor.go:162-167`). `PayloadSummary` is sanitized; documents and presigned URLs are never included.
- **CSRF on all writes** (handled by the existing client/middleware; no per-handler work).
- **Immutable domain models, no JSON tags on domain types** — wire adaptation lives in `rest.go`.

The four features are independent at the package level and can be planned/built as parallel tracks; the only shared touch-points are nav registration (frontend) and route registration in `serve.go`.

---

## 1. Cross-Cutting Decisions

### 1.1 New audit verbs

Added to `internal/audit` action constants (mirroring `ActionObjectUpload` etc.):

| Constant | Action string | TargetType / TargetID |
| --- | --- | --- |
| `ActionObjectVersionRestore` | `object.version.restore` | `object` / `bucket/key` |
| `ActionObjectVersionDelete` | `object.version.delete` | `object` / `bucket/key` |
| `ActionObjectUndelete` | `object.undelete` | `object` / `bucket/key` |
| `ActionLifecycleRuleCreate` | `lifecycle.rule.create` | `lifecycle_rule` / `bucket:ruleID` |
| `ActionPolicyCreate` | `policy.create` | `policy` / `name` |
| `ActionPolicyUpdate` | `policy.update` | `policy` / `name` |
| `ActionPolicyDelete` | `policy.delete` | `policy` / `name` |
| `ActionUserPolicyAttach` | `user.policy.attach` | `user` / `accessKey` |
| `ActionUserPolicyDetach` | `user.policy.detach` | `user` / `accessKey` |

`PayloadSummary` carries only non-sensitive identifiers (bucket, key, version_id, rule kind, policy name, attached/detached policy names). It NEVER carries policy documents, presigned URLs, or secrets. `lifecycle.rule.create` reuses the existing verb but its payload gains a `kind` field. The existing `audit.Sanitize` already strips `*url*`/`*token*`/`*secret*` keys; version IDs and policy names are safe to record.

### 1.2 Error codes

All PRD §5.6 codes are added as `apierror.New(status, code, message)` call sites. No new machinery — `apierror` already supports `WithPointer` (JSON:API source) and `WithDetails` (for `policy_in_use`'s `attached_to`). The codes:

`cannot_restore_delete_marker`(422), `not_delete_marked`(422), `unsupported_lifecycle_kind`(422), `invalid_policy_json`(422), `invalid_policy_structure`(422), `invalid_policy_name`(422), `policy_name_reserved`(409), `policy_read_only`(403), `policy_in_use`(409), `minio_rejected_policy`(422), `unknown_policy`(422), `invalid_metrics_window`(422).

### 1.3 Frontend nav & routing

Two new top-level entries appended to the `NAV` array in `components/AppShell.tsx:44-51` and two routes added under the authenticated `<AppShell>` block in `routes.tsx`:

- `/metrics` → `MetricsPage` (icon: `LineChart` / `Activity` from lucide-react), placed adjacent to Dashboard.
- `/policies` route already exists for the read-only **policy templates** page (`PolicyTemplatesPage`). It is **repurposed/extended** into the full Policies management page (list of all canned policies + JSON editor). The existing template list becomes a section/filter within it (templates show as `origin: harbormaster-template`, read-only). Nav label stays "Policies".

Version browsing is **not** a nav entry — it lives inside the existing bucket-detail Objects tab (see §2.5).

---

## 2. Feature 1 — Object Version Browser + Restore

### 2.1 Key architectural decision: listing & pagination

**Constraint (verified):** minio-go v7.0.74 exposes version listing ONLY through the high-level channel API `Client.ListObjects(ctx, bucket, ListObjectsOptions{WithVersions: true})`. `miniogo.Core` — which the objects domain uses today for raw, opaque-token `ListObjectsV2` pagination (`objects/administrator.go:13-26`) — has **no public `ListObjectVersions`**, and `ListObjectsOptions` exposes no version-id marker (only key-based `StartAfter`). True S3 `key-marker`+`version-id-marker` pagination is therefore not reachable through the public SDK.

**Options considered:**

- **A — Reach into raw S3 (rejected):** issue the `ListObjectVersions` request ourselves to recover the real markers. Rejected: duplicates SDK internals minio-go deliberately encapsulates, brittle across SDK upgrades, and violates the "don't break abstraction boundaries" rule in CLAUDE.md.
- **B — Channel + in-memory windowing (chosen):** the version browser is scoped to a **single object key** (`?key=`), whose version cardinality is bounded (typically tens). Call `ListObjects(ctx, bucket, ListObjectsOptions{Prefix: key, WithVersions: true})`, filter to exact `obj.Key == key` matches (the prefix can match siblings), collect into a slice (already newest-first per S3 semantics), and window it server-side. `page[token]` is an **opaque offset cursor** (base64 of the next index), not an S3 marker. A safety cap (`maxVersionScan = 10_000`) bounds the channel drain; if exceeded, `meta.page.truncated = true` and `next_token` is omitted.

**Tradeoff:** This is a deliberate, documented deviation from the PRD §4.1 wording ("S3 ListObjectVersions key/version-id markers"). The external contract (`page[size]`/`page[token]`, opaque token) is preserved exactly; only the token's internal meaning changes. Given the per-key scope, the in-memory window is correct and simpler, and the cap protects against pathological version counts. If a future feature needs whole-bucket version enumeration, option A can be revisited behind the same contract.

### 2.2 Package changes (`internal/objects`)

- **`model.go`** — add immutable `ObjectVersion{Key, VersionID, Size *int64, LastModified, ETag, ContentType, IsLatest, IsDeleteMarker}` and `VersionListResult{Versions []ObjectVersion, NextToken string, Truncated bool}`. `Size` is a pointer so delete markers serialize as `null` (PRD §5.1).
- **`provider.go`** — `versionFromObjectInfo(info miniogo.ObjectInfo) ObjectVersion`, mapping `IsLatest`/`IsDeleteMarker`/`VersionID`. Delete markers → `Size: nil`, `ContentType: ""`.
- **`administrator.go`** — `listObjectVersions(ctx, s3High, bucket, key)` draining the channel with the cap; `copyObjectVersion(ctx, core, bucket, key, srcVersionID)` (server-side restore via `Core.CopyObject` with `CopySrcOptions{VersionID}`); `removeObjectVersion(ctx, s3, bucket, key, versionID)` (`RemoveObject` with `RemoveObjectOptions{VersionID}`). Download/stat/get/presign gain an optional `versionID` passed through `GetObjectOptions{VersionID}` / `StatObjectOptions{VersionID}` / presign query.
- **`processor.go`** — interface split: the existing private `s3API` (backed by `Core`) gains `CopyObject(...)` and `RemoveObject(ctx, bucket, key, RemoveObjectOptions)`; a **new** private interface method backed by the **high-level** `*miniogo.Client` is needed for `ListObjects` (channel). The adapter in `serve.go` already holds both `Core` and `Client`, so `NewClientGetter` wires both. New processor methods: `ListVersions(ctx, bucket, key, pageSize, pageToken)`, `RestoreVersion(ctx, bucket, key, versionID, actor, ip)`, `DeleteVersion(ctx, bucket, key, versionID, actor, ip)`, `Undelete(ctx, bucket, key, actor, ip)`. Download/Preview/ShareLink signatures gain an optional `versionID string`.
- **`resource.go` / `rest.go`** — `object_versions` resource (`ResourceID = key + "@" + versionID`, matching PRD §5.1). New handlers: `GET .../objects/versions`, action handlers for restore/delete-version/undelete; `version_id` query param threaded into the existing download/preview handlers.

### 2.3 Action semantics

- **Restore** = `Core.CopyObject` same-bucket/same-key with `CopySrcOptions{VersionID: <ver>}`. Server-side; no client byte transfer. Returns the new current version metadata (`StatObject` after copy). Reject delete-marker source up front with `422 cannot_restore_delete_marker` (we already have `is_delete_marker` from the version we resolve).
- **Delete version** = `RemoveObject` with `VersionID` (permanent, single version). Action body `{ "confirm": true }`; missing/false `confirm` → `422`. `204` on success.
- **Undelete** = resolve the latest version for the key; if it is NOT a delete marker → `422 not_delete_marked`; else `RemoveObject` with that delete marker's `VersionID` (removing a delete marker re-exposes the prior version). Returns `{ key, version_id: <newly-exposed-latest> }`.

All three emit success/failure audit events via the established `failAudit` closure pattern (`objects/processor.go:214-249`).

### 2.4 Never-versioned / suspended buckets (resolves PRD Open Q #3)

`ListVersions` on a never-versioned bucket returns a single entry with `VersionID: ""` (the SDK reports `"null"`/empty) and `IsLatest: true`; the resource maps empty → JSON `null` and the UI shows the informational note (PRD §4.1). **Suspended** versioning is treated identically to enabled for read/restore/delete (S3 keeps historical versions when suspended; only new writes stop creating versions). No distinct wording is needed: the version list is the authority. The UI gates the version affordances on "more than one version OR any non-null version id exists," not on the live versioning toggle, so suspended-with-history behaves correctly.

### 2.5 Frontend

Add a per-row **Versions** action (icon `History`) to `VirtualizedObjectList.tsx:150-192`, opening a shadcn **Sheet** (drawer) `ObjectVersionsSheet`. The sheet uses `useInfiniteQuery` (key `objectsKeys.versions(bucket, key)`) over the `page[token]` cursor, renders a table (version id, size, modified, latest badge, delete-marker badge), and per-row Download/Preview/Restore/Delete actions. Restore and Delete-version use confirmation dialogs with distinct, irreversible-action wording (Delete-version is styled destructive, separate from the existing soft "delete object"). Delete-marked latest objects render an **Undelete** affordance. Mutations invalidate `objectsKeys.versions(...)` and `objectsKeys.list(bucket, prefix)`. Download/preview reuse existing helpers with an added `version_id` param.

---

## 3. Feature 2 — Lifecycle Extensions

### 3.1 Design

`internal/lifecycle` today recognizes exactly one managed kind, `expiration`, via a regex `^harbormaster-expire-\d+d(-slug)?$` and a strict classifier that rejects any rule bearing transitions, noncurrent actions, abort-mpu, or tag filters (`classifier.go:18-90`). We extend the closed set to three kinds while preserving the "managed iff Harbormaster-shaped AND no foreign attributes" invariant.

- **`model.go`** — `Rule.Kind` already exists. Add typed fields used per kind: `NoncurrentDays int`, `NewerNoncurrentVersions int`, `DaysAfterInitiation int` (kept zero-valued for kinds that don't use them). Keep the model flat with a `Kind` discriminator rather than a union — matches the existing flat `Rule` and the JSON:API discriminated-attribute shape.
- **`builder.go`** — deterministic IDs extended:
  - `noncurrent-expiration` → `harbormaster-noncurrent-<slug-or-all>-<days>d`
  - `abort-incomplete-multipart` → `harbormaster-abortmpu-<slug-or-all>-<days>d`
  reusing the existing `slugifyPrefix`. Validation: `noncurrent_days ≥ 1`, `newer_noncurrent_versions ≥ 0` (optional), `days_after_initiation ≥ 1`.
- **`classifier.go`** — generalize the ID regex to three families and split classification per family. A rule is managed iff its ID matches one family AND it carries exactly the matching action and nothing foreign:
  - expiration: one `Expiration.Days > 0`, no other actions, no tags (unchanged).
  - noncurrent: one `NoncurrentVersionExpiration` (`NoncurrentDays`, optional `NewerNoncurrentVersions`), no other actions, no tags.
  - abort-mpu: one `AbortIncompleteMultipartUpload.DaysAfterInitiation > 0`, no other actions, no tags.
  Unmanaged summary logic (value-free, action-kind + tag-count only) is unchanged.
- **`processor.go`** — `Create` builds the right minio-go `lifecycle.Rule` per kind (`NoncurrentVersionExpiration{NoncurrentDays, NewerNoncurrentVersions}` or `AbortIncompleteMultipartUpload{DaysAfterInitiation}`), then reuses the existing read-modify-write upsert against `GetBucketLifecycle`/`SetBucketLifecycle`. The kind-validation error broadens to `422 unsupported_lifecycle_kind` ("only expiration, noncurrent-expiration, and abort-incomplete-multipart are supported"); `transition` and all others stay rejected.
- **`resource.go` / `rest.go`** — request decoding switches on `kind`; the managed-rule resource attributes are discriminated by `kind` and include only that kind's fields.

### 3.2 `newer_noncurrent_versions` compatibility (resolves PRD Open Q #5)

The minio-go `lifecycle.NoncurrentVersionExpiration` struct carries `NewerNoncurrentVersions`. We always send it when `> 0`. If the target MinIO release silently ignores it, the rule still expires noncurrent versions by age (the primary intent); we do not attempt to detect server support. The UI labels it "optional; requires a recent MinIO" in helper text. No runtime probing — graceful degradation is "the field is honored or harmlessly ignored."

### 3.3 Non-versioned-bucket warning

`noncurrent-expiration` creation is permitted regardless of versioning state (the rule is valid but inert without versioning). The frontend checks the bucket's `versioning_enabled` (already on the bucket resource) and shows a **non-blocking** inline warning in the create form when versioning is off. No backend enforcement.

### 3.4 Frontend

Extend `features/lifecycle/CreateRuleDialog.tsx`: add a `kind` `Select` (Expiration / Noncurrent versions / Abort incomplete multipart). The Zod schema becomes a discriminated union on `kind`, revealing the relevant fields per selection (`days`/`prefix`; `noncurrent_days`/`newer_noncurrent_versions`/`prefix`; `days_after_initiation`/`prefix`). The rules list/tab labels each managed rule by kind via a badge. Error-pointer mapping extends to the new field names.

---

## 4. Feature 3 — Custom / Inline IAM Policy Editor

### 4.1 New `/policies` domain surface

`internal/policies` is currently a library (templates + materializer + bucket_canned) with **no HTTP resource**. We add a full canned-policy CRUD surface following the standard DDD layout, while keeping the existing template/materializer code intact (templates become one `origin` in the unified listing).

**madmin calls (verified available on v3.0.66 `AdminClient`):** `ListCannedPolicies`, `InfoCannedPolicy`, `AddCannedPolicy`, `RemoveCannedPolicy`. These back the new `adminAPI` interface for the policies processor.

New files:
- **`model.go`** — `Policy{Name, Origin, Editable, StatementSummary string}` and `PolicyDetail{Policy, Document json.RawMessage}`. `Origin` ∈ `minio-builtin | harbormaster-template | custom`.
- **`builder.go`** — the structural validator (§4.2) and name validator.
- **`classifier.go`** — `originFor(name)`: name in MinIO built-in set (`readonly`, `readwrite`, `writeonly`, `consoleAdmin`, `diagnostics`) → `minio-builtin`; name matches a bundled template's `MaterializedName` (any params) or the `harbormaster-` prefix owned by templates → `harbormaster-template`; else → `custom`. `editable = (origin == custom)`.
- **`provider.go`** — map madmin policy entries → `Policy`, compute `StatementSummary` ("Allow s3:GetObject on arn:aws:s3:::photos/*", truncated) from the parsed document.
- **`administrator.go`** — wrap the four madmin calls.
- **`processor.go`** — `List`, `Get`, `Create`, `Update`, `Delete`, each emitting audit. Injected `adminAPI` via `ClientGetter` (same pattern as users).
- **`resource.go` / `rest.go`** — `policies` JSON:API resource (`ResourceID = name`); `GET /policies`, `GET /policies/{name}` (adds `document`), `POST /policies`, `PUT /policies/{name}`, `DELETE /policies/{name}`.

### 4.2 Validation (server-side, best-effort)

`builder.ValidatePolicyDocument([]byte)`:
1. valid JSON → else `422 invalid_policy_json`;
2. structural: top-level object with `Version` (string) + `Statement` (non-empty array); each statement has `Effect` ∈ {`Allow`,`Deny`} and at least one of `Action`/`NotAction` → else `422 invalid_policy_structure`;
3. `ValidatePolicyName(name)`: MinIO charset/length (alphanumeric + `-_./`, ≤ 128) → else `422 invalid_policy_name`;
4. reserved-name check against bundled template `MaterializedName`s and the built-in set → `409 policy_name_reserved`.

MinIO remains final authority: `AddCannedPolicy` rejection is surfaced verbatim as `422 minio_rejected_policy` (message = MinIO's error). Validation is intentionally structural-only, not a full IAM grammar — the PRD explicitly rules out a visual/semantic builder.

### 4.3 Edit / Delete guards

- `Update`/`Delete` first resolve `origin`; non-`custom` → `403 policy_read_only`.
- `Delete` → if MinIO reports the policy attached, return `409 policy_in_use` with `details.attached_to: {users, groups}`. **Detection mechanism:** madmin has no direct "who is this policy attached to" call, so we enumerate users (`ListUsers`) and inspect each `UserInfo.PolicyName` for the target; groups via `ListGroups`/`GetGroupDescription` if available on v3.0.66 (verify in plan; if group enumeration is unavailable, populate `users` and leave `groups: []`, and rely on MinIO's own rejection as the backstop). This is a read-only scan bounded by user count (small for the target audience).

### 4.4 User custom-policy attachment (resolves PRD Open Q #4)

This is the highest-risk piece: MinIO's policy attach/detach must not clobber externally-managed policies (e.g. `consoleAdmin`). The existing users processor already does a **set-diff** using `AttachPolicy`/`DetachPolicy` (NOT `SetUserPolicy` full-replace), touching only Harbormaster-template-materialized policies and leaving "other" policies untouched (`users/processor.go:367-457`). We extend the "owned" set to include **named custom policies**:

- **Ownership rule:** the set Harbormaster will detach on update = {template-materialized policies} ∪ {policies that exist on the deployment with `origin == custom`}. Built-ins (`consoleAdmin`, etc.) and any name not classifiable as template/custom are **never** detached — they remain in the read-only "Other attached policies" row.
- `PUT /users/{access_key}/policies` request gains optional `policies: []string` (plain canned names) alongside `templates`. Up-front validation: every named policy must exist on the deployment (`InfoCannedPolicy`) → else `422 unknown_policy`.
- Diff: desired = materialize(templates) ∪ policies; current-owned = classify(user.PolicyName) ∩ owned-set. `AttachPolicy` the additions, `DetachPolicy` the removals, all via the per-policy add/detach API (never set-replace). Each attach/detach emits `user.policy.attach`/`user.policy.detach`.
- `GET /users/{access_key}` attributes gain `attached_policies: []string` (custom/named only), distinct from `attached_templates` and from the read-only "other" classification.

This guarantees `consoleAdmin` and other foreign grants survive any Harbormaster-driven update — the safety property the PRD demands.

### 4.5 Frontend

- Extend the existing `/policies` page (`PolicyTemplatesPage` → `PoliciesPage`): a unified table of all canned policies with `origin`/`editable` badges and statement summary. A "New policy" button opens a `PolicyEditorDialog` with a name field and a JSON textarea (monospace; client-side `JSON.parse` pre-check mirrors `invalid_policy_json` before submit). Edit/Delete actions appear only for `editable` rows; delete confirmation surfaces `policy_in_use` attachment details.
- User-detail gains a "Custom policies" section beside the existing template chips: an `EditCustomPoliciesDialog` listing all `origin: custom` policies as a multi-select, submitting the `policies` array to the extended attachment endpoint. Externally-attached unknowns stay in the existing read-only "Other attached policies" row.

---

## 5. Feature 4 — Prometheus Metrics Dashboard

### 5.1 Metrics source decision (resolves PRD Open Q #1)

**Verified SDK facts (madmin-go/v3 v3.0.66):**
- The realtime `AdminClient.Metrics(ctx, MetricsOptions, out)` covers scanner/disk/OS/cpu/mem/net/rpc — **it does NOT expose S3 API request counters, per-status errors, or S3 traffic bytes.** Insufficient for the PRD's headline series.
- madmin ships a dedicated **`MetricsClient`** (`metrics_client.go`, `prometheus_metrics.go`): `NewMetricsClient(endpoint, accessKey, secretKey, secure)` generates the Prometheus **bearer token internally** (`getPrometheusToken`), exposes `ClusterMetrics(ctx)`, `NodeMetrics(ctx)`, `BucketMetrics(ctx)`, `ResourceMetrics(ctx)` returning already-parsed `[]*prom2json.Family`, and `SetCustomTransport` to reuse our TLS config.

**Decision: use `madmin.MetricsClient.ClusterMetrics()` (+ `ResourceMetrics()` for capacity/drives).** This is the only path that yields the PRD's request/error/traffic/capacity series, the token is handled by the SDK (no manual JWT minting), and `SetCustomTransport` lets us reuse the pool's custom CA / skip-verify transport (`minio/pool.go:117-136`). The realtime `Metrics()` API is rejected for this feature.

**Series → Prometheus family mapping** (cluster/resource subsystems; exact names verified during the plan against the running MinIO per CLAUDE.md):
| PRD series | Prometheus metric (cluster/resource) | Kind |
| --- | --- | --- |
| `api_requests_total` | `minio_s3_requests_total` | counter |
| `api_errors_4xx` | `minio_s3_requests_4xx_errors_total` | counter |
| `api_errors_5xx` | `minio_s3_requests_5xx_errors_total` | counter |
| `rx_bytes_per_sec` | `minio_s3_traffic_received_bytes` | counter |
| `tx_bytes_per_sec` | `minio_s3_traffic_sent_bytes` | counter |
| capacity total/free | `minio_cluster_capacity_usable_total_bytes` / `_free_bytes` | gauge |
| drives online/offline | `minio_cluster_drive_online_total` / `_offline_total` | gauge |

Counters are stored **raw**; rates are derived at query time from adjacent samples (handles counter resets by clamping negative deltas to 0). Gauges are stored and read directly.

**Alternative (deferred, contract-compatible):** external-Prometheus federation (PRD Open Q #2). The `GET /api/v1/metrics` contract is source-agnostic, so a future `HARBORMASTER_METRICS_SOURCE=prometheus` mode can swap the collector without touching the frontend. Not built now.

### 5.2 New domain `internal/metrics`

- **`entity.go`** — GORM entity `metricsSample{ID string (ULID, pk), CollectedAt string (RFC3339Nano, indexed), Metric string (indexed), Value float64}`, `TableName() "metrics_samples"`, mirroring `audit/entity.go`. Migration `migrations/0007_metrics_samples.up.sql`: table + composite index `(metric, collected_at)`. No backfill, no changes to existing tables.
- **`model.go`** — `Window` (`1h|6h|24h|7d`), `Series`/`Point{T time.Time, V float64}`, `MetricsView{Window, StepSeconds, Collected bool, Series map[string][]Point, Capacity, Drives}`.
- **`collector.go`** — the `MetricsClient` adapter: given the pool's credentials, build/refresh a `madmin.MetricsClient`, call `ClusterMetrics`/`ResourceMetrics`, flatten the `prom2json.Family` list into `(metric, value)` pairs for the metrics we track. Rebuilt when the connection pool is rebuilt (re-read creds on each poll, cheap).
- **`store.go`** — insert samples (ULID + `collected_at`), query a window (`WHERE metric IN (...) AND collected_at >= cutoff ORDER BY collected_at`), and `RetentionSweep(cutoff)` deleting old rows. Mirrors `audit/processor.go` + `audit/retention.go`.
- **`poller.go`** — `StartPoller(ctx, deps, interval)` ticker goroutine, **structurally identical to `audit.StartRetentionSweeper`** (`audit/retention.go:14-35`): `select { <-ctx.Done(): return; <-ticker.C: collect+store }`. On collect error (MinIO down / pool not initialized), log and continue — **no sample written**, producing a natural gap. Started in `serve.go` after pool hydration, before routes; cancelled by the existing root context on shutdown.
- **`aggregator.go`** — query-time downsampling: pick `step` per window so each series returns ≤ ~300 points (`1h→60s`, `6h→300s`, `24h→300s`, `7d→1800s`), bucket samples into steps, compute counter rates from deltas, pass gauges through. Produces `MetricsView`.
- **`resource.go` / `rest.go`** — `GET /api/v1/metrics?window=&step=` returns the **plain-JSON** shape in PRD §5.5 (this is an aggregate read, not a JSON:API resource — matches the dashboard's plain shape). `collected: false` + empty series when no samples in window (fresh install or paused). `422 invalid_metrics_window` for bad `window`.

### 5.3 Config

Add to `internal/config` (viper, `HARBORMASTER_` prefix, `config.go` defaults block):
- `HARBORMASTER_METRICS_POLL_INTERVAL` (duration, default `30s`),
- `HARBORMASTER_METRICS_RETENTION` (duration, default `8d` — covers the 7d window with headroom).

The retention sweeper for metrics reuses the audit sweeper cadence (24h) but with the metrics retention horizon. The unused `MetricsEnabled`/`MetricsListenAddr` config (app self-metrics, distinct concept) is left untouched.

### 5.4 Resilience

- Pool not initialized / MinIO unreachable → poller skips the tick (gap), never panics. The `/metrics` endpoint always returns 200 with whatever samples exist; if the most recent sample is older than ~2 poll intervals, `rest.go` sets a `collected: false`/paused indicator so the page shows the "metrics collection paused" banner instead of stale-looking data. Recovery is automatic on the next successful tick.

### 5.5 Frontend

New `MetricsPage` under `/metrics` using the already-present **Recharts 2.15.4** + the shadcn `chart` wrapper (`components/ui/chart.tsx`, pattern in `dashboard/BucketSizeChart.tsx`). A window `Select` (1h/6h/24h/7d) persisted to localStorage (mirroring `DashboardPage`'s window pattern). `useQuery` (key `metricsKeys.view(window)`) hits `GET /api/v1/metrics`; charts: request-rate (line, optional 2xx/4xx/5xx split), error-rate, throughput (rx/tx area), capacity (used/free + used-% gauge), drives online/offline. When `collected: false`, render the explanatory banner instead of empty axes. Auto-refetch on a modest interval (e.g. 30s) so the page tracks live data.

---

## 6. Testing Strategy

Per CLAUDE.md's "done" bar (`go test -race`, `go vet`, `golangci-lint`, `CGO_ENABLED=0 build`; frontend `lint`/`format`/`test`/`build`), each track adds unit + golden tests mirroring the existing `*_test.go` / `golden_test.go` conventions:

- **objects** — processor tests stub the `s3API` (Core) + new high-level list interface: version listing (incl. never-versioned single-null-entry, delete markers, windowing token round-trip, cap/truncation), restore (rejects delete marker; copy invoked with right `VersionID`), delete-version (confirm gate), undelete (rejects non-delete-marked). Audit-event tests assert verbs + sanitized payloads. JSON:API golden for `object_versions`.
- **lifecycle** — classifier round-trip tests for all three kinds (managed in → minio rule → managed out), foreign-attribute rules stay unmanaged, deterministic-ID golden, `unsupported_lifecycle_kind` for transition. Builder validation tests.
- **policies** — validator unit tests (each `invalid_*`/`reserved` path), classifier `origin` tests, processor CRUD with stubbed `adminAPI` (read-only guard, `policy_in_use` detection via stubbed user enumeration), JSON:API golden. users processor: attach/detach diff tests proving built-ins/foreign policies are never detached (the Open-Q-#4 safety property) and `unknown_policy` rejection.
- **metrics** — collector flattening from canned `prom2json.Family` fixtures; aggregator rate computation (counter delta, reset clamp, downsample point-count bound); store insert/query/sweep; poller tick test with a fake clock/collector (success writes sample, error writes nothing → gap); window validation; `collected:false` paused-state test.
- **frontend** — component/hook tests for the versions sheet, lifecycle kind union form, policy editor validation, custom-policy attachment, and metrics charts (mocked query data incl. the paused state), following existing Vitest patterns.

Integration tests (`HARBORMASTER_INTEGRATION=1`, Docker MinIO) are added on-demand for the version restore/undelete round-trip and the metrics scrape against a live MinIO (also the point where exact Prometheus metric names are confirmed).

---

## 7. Risks & Sequencing

- **Highest risk: custom-policy detach safety (§4.4).** Mitigated by reusing the proven add/detach set-diff (never set-replace) and an explicit, test-enforced ownership rule. Build users-attachment changes only after the policies CRUD + classifier exist.
- **Metric name drift across MinIO versions (§5.1).** Mitigated by confirming names against the running server in an integration test and keeping the series→family mapping in one table in `collector.go`.
- **Version pagination deviation (§2.1).** Documented; external contract unchanged; safety-capped.
- **Suggested build order (independent tracks, parallelizable):** (1) lifecycle (smallest, self-contained), (2) objects versions, (3) policies CRUD → then users attachment, (4) metrics (poller + store + page). Nav/route registration is a final small integration step.

## 8. Resolved Open Questions (PRD §9)

1. **Metrics source** → `madmin.MetricsClient.ClusterMetrics`/`ResourceMetrics` (Prometheus subsystem, SDK-minted token). Realtime `Metrics()` rejected (no S3 request/error series). §5.1.
2. **External-Prometheus federation** → deferred; `/metrics` contract kept source-agnostic for later. §5.1.
3. **Suspended versioning** → version list is authoritative; suspended treated like enabled for read/restore/delete; affordances gated on version presence, not the live toggle. §2.4.
4. **Custom-policy detach safety** → owned set = templates ∪ existing `custom`-origin policies; built-ins/foreign never detached; per-policy attach/detach, never set-replace. §4.4.
5. **`newer_noncurrent_versions`** → always sent when > 0; harmlessly ignored on older servers; UI labels it as requiring a recent MinIO. §3.2.
