# P0 UI Feature Parity — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-05-28
---

## 1. Overview

Harbormaster's v1 (task-001) shipped a credible MinIO admin UI: bucket CRUD, an object
browser, IAM users + service accounts, bundled policy templates, quotas, a versioning
toggle, expiration-only lifecycle rules, an audit log, and a point-in-time dashboard. A
competitive delta against the (now-archived) full MinIO Console — the tool our target
homelab/small-cluster operators are migrating away from — surfaced four highest-value
gaps. Each is a place where Harbormaster *started* a capability but left it unusable in
practice, or omitted a feature operators specifically miss from the old Console.

This task closes those four gaps as a single coordinated release ("P0 parity"):

1. **Object version history browser + restore** — versioning can be toggled on today, but
   there is no way to browse, preview, download, delete, or restore individual versions or
   see delete markers. Enabling versioning currently gives the operator nothing usable.
2. **Noncurrent-version & abort-incomplete-multipart lifecycle rules** — lifecycle is
   expiration-only, so a versioned bucket grows forever with no UI path to reap old
   versions or stranded multipart uploads. This is the natural partner to (1).
3. **Custom / inline IAM policy editor** — only three bundled templates are creatable;
   every other policy is read-only. Operators routinely need a bespoke (e.g.
   path-scoped) policy. A JSON policy editor with create/edit/delete and the ability to
   attach custom policies to users unlocks the full IAM story.
4. **Prometheus metrics dashboard** — the current dashboard is point-in-time (counts,
   node state). The Console's headline feature was a time-series metrics view. This adds
   throughput / request / error / capacity charts over a selectable window.

**Explicitly deferred (not in this task):** lifecycle *transition* rules and remote-tier
management (no transition target exists yet — tracked as a future P2/P3 item), object
locking/legal-hold/retention, IAM groups, bucket events/notifications, and external
identity providers. Transition rules were considered and cut because they require remote
tier configuration that is out of scope; only the tier-free half of lifecycle (noncurrent
expiration + abort-incomplete-multipart) ships here.

## 2. Goals

Primary goals:

- Make bucket versioning *useful*: browse version history for an object, preview/download a
  specific version, restore a prior version to current, permanently delete a version, and
  see/clear delete markers.
- Let operators bound storage growth on versioned buckets entirely from the UI via
  noncurrent-version expiration and abort-incomplete-multipart lifecycle rules.
- Let operators author, edit, and delete arbitrary named IAM (canned) policies as JSON, and
  attach those custom policies to users alongside the existing bundled templates.
- Give operators a time-series operational view (API requests, throughput, errors,
  capacity, drive health over time) sourced from MinIO's metrics, without requiring an
  external Prometheus deployment.
- Preserve every existing convention: JSON:API for resource endpoints, plain-JSON action
  endpoints, CSRF on writes, full audit coverage of mutations, one-time secret exposure,
  and the per-domain `model/builder/processor/provider/resource/rest` package layout.

Non-goals:

- Lifecycle **transition** rules and any remote-tier (S3/Azure/GCS/MinIO) management.
- Object locking, WORM, legal hold, or per-object retention.
- IAM **groups**, STS, or external identity providers (OIDC/LDAP).
- Bucket events/notifications, SSE/KMS encryption, CORS, site/bucket replication.
- A general-purpose policy *visual builder* — the editor is raw JSON with validation, not a
  drag-and-drop statement composer.
- Requiring or embedding a full Prometheus/Grafana stack. (External-Prometheus federation
  is an explicitly considered alternative — see §9.)

## 3. User Stories

**Versioning**
- As an operator, I want to open an object and see its version history (timestamps, sizes,
  version IDs, which is current, and delete markers) so I can understand what changed.
- As an operator, I want to download or preview a specific past version so I can recover
  the right copy.
- As an operator, I want to restore a past version to be the current version so a bad
  overwrite is undone without me re-uploading anything.
- As an operator, I want to permanently delete a specific version (with confirmation) so I
  can purge a sensitive or oversized version.
- As an operator, I want to see delete-marked ("deleted") objects and undelete them by
  removing the delete marker.

**Lifecycle**
- As an operator of a versioned bucket, I want a rule that expires noncurrent versions
  after N days (optionally keeping the newest M noncurrent versions) so old versions don't
  accumulate forever.
- As an operator, I want a rule that aborts incomplete multipart uploads after N days so
  failed/abandoned uploads stop consuming space.
- As an operator, I want these to appear and be deletable in the same lifecycle list as my
  existing expiration rules, clearly labeled by kind.

**Policies**
- As an operator, I want to see all canned IAM policies on the deployment (bundled,
  built-in like `readwrite`/`consoleAdmin`, and custom) in one place.
- As an operator, I want to create a named policy by pasting/editing a JSON policy document,
  with validation that catches malformed JSON and obviously-invalid policy structure before
  it reaches MinIO.
- As an operator, I want to edit or delete a custom policy I created.
- As an operator, I want to attach a custom named policy to a user (alongside bundled
  templates) so I'm not limited to read-only/read-write/backup-target.

**Metrics**
- As an operator, I want a metrics dashboard showing S3 API request rate, throughput
  (in/out bytes), error rate, and total capacity used/free over a selectable time window
  (e.g. last 1h/6h/24h/7d) so I can spot trends and incidents.
- As an operator, I want this to work out of the box against my MinIO deployment without
  standing up a separate Prometheus.

## 4. Functional Requirements

### 4.1 Object version browser

- The object browser exposes, per object key, a "Versions" view listing all versions and
  delete markers for that key, newest first.
- Each entry shows: version ID, size (n/a for delete markers), last-modified, ETag,
  `is_latest` flag, and `is_delete_marker` flag.
- The view is only meaningful on versioned (or previously-versioned) buckets; on a
  never-versioned bucket the API returns a single `null`-version entry and the UI shows an
  informational note instead of the version affordances.
- Pagination uses the same opaque `page[token]` cursor model as the object list (S3
  `ListObjectVersions` key/version-id markers), `page[size]` default 100.
- Per-version actions:
  - **Download** a specific version (proxy or direct mode, same as current download).
  - **Preview** a specific version (same MIME rules as current preview).
  - **Restore**: server-side copy of the chosen version onto the same key, creating a new
    current version (no client round-trip of bytes). Disabled for delete-marker entries.
  - **Delete version**: permanent removal of that one version ID (confirmation required;
    distinct wording from soft "delete object" since it is irreversible).
- Delete markers: a delete-marked latest version is shown as a "deleted" object; the UI
  offers **Undelete** (remove the latest delete marker, exposing the prior version).
- All version mutations (restore, delete-version, undelete) emit audit events (§4.x audit).

### 4.2 Noncurrent-version & abort-incomplete-multipart lifecycle rules

- The lifecycle create form accepts three `kind` values now: existing `expiration`, plus
  `noncurrent-expiration` and `abort-incomplete-multipart`.
- `noncurrent-expiration` fields: `noncurrent_days` (int ≥ 1, required), `newer_noncurrent_versions`
  (int ≥ 0, optional — keep this many newest noncurrent versions before expiring),
  `prefix` (optional, same as expiration).
- `abort-incomplete-multipart` fields: `days_after_initiation` (int ≥ 1, required),
  `prefix` (optional).
- Managed rules created via these forms get deterministic IDs following the existing
  `harbormaster-*` convention (e.g. `harbormaster-noncurrent-<prefix-or-all>-<days>d`,
  `harbormaster-abortmpu-<prefix-or-all>-<days>d`), and are round-trip classifiable as
  `managed` by the existing classifier.
- `noncurrent-expiration` rules SHOULD warn (non-blocking) in the UI if the target bucket
  does not currently have versioning enabled, since the rule is inert without it.
- Listing, the `managed`/unmanaged distinction, and DELETE behave exactly as today; new
  kinds appear in the managed-rule attribute shape with their `kind` discriminator.
- Transition (`kind: "transition"`) remains **rejected** with the existing
  "only expiration"-style 422, broadened to "only expiration, noncurrent-expiration, and
  abort-incomplete-multipart are supported".

### 4.3 Custom / inline IAM policy editor

- A new **Policies** management area lists all canned policies on the MinIO deployment via
  `madmin` (built-in + custom). Each entry shows: name, whether it is Harbormaster-bundled
  (template-derived), MinIO built-in, or custom/editable, and a statement summary.
- **Create**: operator supplies a policy `name` and a JSON `document`. Validation:
  - valid JSON;
  - top-level shape is an IAM policy (`Version` string + `Statement` array; each statement
    has `Effect` ∈ {Allow, Deny} and at least one of `Action`/`NotAction`);
  - name matches MinIO policy-name rules (charset/length);
  - reject names that collide with Harbormaster-bundled template names to avoid confusing
    the templates UI (`policy_name_reserved`).
  Server-side validation is best-effort structural; MinIO remains the final authority and
  its rejection is surfaced verbatim under a `minio_rejected_policy` code.
- **Edit**: replace the document of a custom policy (built-in/bundled policies are
  read-only — edit/delete return `policy_read_only`).
- **Delete**: remove a custom policy; refuse with `409 policy_in_use` if MinIO reports it
  attached to any user/group (list attachments in the error details so the operator can
  detach first).
- **Attach to user**: the user policy-attachment flow is extended so a user can have, in
  addition to bundled `templates`, a set of named `policies` (plain MinIO canned policy
  names). The user-detail page gains a "Custom policies" section to attach/detach these.
  Externally-attached policies that are neither templates nor known custom policies remain
  in the existing read-only "Other attached policies" row.
- Every create/edit/delete/attach/detach emits an audit event. Policy *documents* are not
  stored by Harbormaster; MinIO is the source of truth (read back on demand).

### 4.4 Prometheus metrics dashboard

- A new **Metrics** dashboard page renders time-series charts over a selectable window
  (`1h`, `6h`, `24h`, `7d`):
  - S3 API request rate (requests/sec), optionally split by 2xx/4xx/5xx;
  - error rate (4xx/5xx);
  - network throughput (rx/tx bytes/sec);
  - capacity: total usable, used, free (and used-% gauge);
  - online drives / nodes over time (or current with sparkline if history is thin).
- **Data source (default):** Harbormaster runs an internal poller that periodically scrapes
  MinIO's metrics and retains a rolling window of samples locally (SQLite), so the feature
  works with **no external Prometheus**. The poller:
  - authenticates to MinIO using the stored admin credentials (bearer token generated via
    the admin API / `madmin`, refreshed as needed), or uses `madmin` realtime metrics;
  - polls at a configurable interval (`HARBORMASTER_METRICS_POLL_INTERVAL`, default `30s`);
  - retains samples for a configurable horizon (`HARBORMASTER_METRICS_RETENTION`, default
    `8d` to cover the 7d window) and sweeps older rows (reuse the audit-retention sweeper
    pattern);
  - degrades gracefully: if MinIO is unreachable, gaps appear in the series and the page
    shows a "metrics collection paused" banner rather than erroring.
- The frontend reads aggregated series from a Harbormaster API (§5), never from MinIO
  directly, so no MinIO metrics endpoint is exposed to the browser.
- The exact MinIO metrics source (Prometheus text endpoint `/minio/v2/metrics/cluster`
  with a generated bearer token vs. `madmin` MetricsV3/realtime API) is settled in the
  design phase — both satisfy this PRD. See §9.

### 4.5 Cross-cutting

- All new write endpoints require a valid session and CSRF token, consistent with v1.
- All mutations emit audit events using the existing processor; new action verbs:
  `object.version.restore`, `object.version.delete`, `object.undelete`,
  `lifecycle.rule.create` (extended with new kinds), `policy.create`, `policy.update`,
  `policy.delete`, `user.policy.attach`, `user.policy.detach`. Audit payloads never include
  policy documents or presigned URLs.
- New nav entries: **Metrics** (top-level, near Dashboard) and **Policies** (top-level,
  near Users). Version browsing lives inside the existing bucket-detail Objects tab.

## 5. API Surface

All resource endpoints are JSON:API (`application/vnd.api+json`); action endpoints are
plain JSON. Writes require `X-CSRF-Token`. Shapes below extend the task-001 contracts in
`docs/tasks/task-001-harbormaster-mvp-v1/api-contracts.md`.

### 5.1 Object versions

`GET /api/v1/buckets/{name}/objects/versions?key=<urlencoded>&page[size]=100&page[token]=...`

```json
{
  "data": [
    {
      "type": "object_versions",
      "id": "2025/01/IMG_0001.jpg@3sL9...verid",
      "attributes": {
        "key": "2025/01/IMG_0001.jpg",
        "version_id": "3sL9...verid",
        "size": 4329122,
        "last_modified": "2025-01-15T08:11:02Z",
        "etag": "\"d41d...\"",
        "content_type": "image/jpeg",
        "is_latest": true,
        "is_delete_marker": false
      }
    },
    {
      "type": "object_versions",
      "id": "2025/01/IMG_0001.jpg@2aB...",
      "attributes": {
        "key": "2025/01/IMG_0001.jpg",
        "version_id": "2aB...",
        "size": null,
        "last_modified": "2025-01-14T22:00:00Z",
        "is_latest": false,
        "is_delete_marker": true
      }
    }
  ],
  "meta": { "page": { "size": 100, "next_token": "eyJrZXkiOi..." } }
}
```

On a never-versioned bucket: a single entry with `version_id: null`, `is_latest: true`.

`GET /api/v1/buckets/{name}/objects/download?key=<urlencoded>&version_id=<id>` — existing
download endpoint gains an optional `version_id` query param (proxy and direct modes both
honor it). Preview reuses the same param.

`POST /api/v1/buckets/{name}/objects/restore-version` (action)

```json
{ "key": "2025/01/IMG_0001.jpg", "version_id": "2aB..." }
```

Success `200`: returns the new current version metadata
`{ "key": "...", "version_id": "<new>", "restored_from": "2aB..." }`.
`422 cannot_restore_delete_marker` if `version_id` is a delete marker.

`DELETE /api/v1/buckets/{name}/objects/version?key=<urlencoded>&version_id=<id>` (action) —
permanent single-version delete. Body `{ "confirm": true }`. Success `204`.

`POST /api/v1/buckets/{name}/objects/undelete` (action)

```json
{ "key": "2025/01/IMG_0001.jpg" }
```

Removes the latest delete marker. Success `200` `{ "key": "...", "version_id": "<exposed>" }`.
`422 not_delete_marked` if the latest version is not a delete marker.

### 5.2 Lifecycle (extended)

`POST /api/v1/buckets/{name}/lifecycle-rules` — `kind` now accepts three values:

```json
{ "data": { "type": "lifecycle_rules",
  "attributes": { "kind": "noncurrent-expiration", "noncurrent_days": 30,
                  "newer_noncurrent_versions": 3, "prefix": "uploads/" } } }
```

```json
{ "data": { "type": "lifecycle_rules",
  "attributes": { "kind": "abort-incomplete-multipart", "days_after_initiation": 7,
                  "prefix": null } } }
```

`GET` list adds the new managed-rule attribute shapes (discriminated by `kind`). DELETE
unchanged. `422 unsupported_lifecycle_kind` replaces/extends the old expiration-only error.

### 5.3 Policies (new domain)

`GET /api/v1/policies`

```json
{
  "data": [
    { "type": "policies", "id": "readwrite",
      "attributes": { "name": "readwrite", "origin": "minio-builtin",
                      "editable": false, "statement_summary": "Allow s3:* on *" } },
    { "type": "policies", "id": "photos-readonly",
      "attributes": { "name": "photos-readonly", "origin": "custom", "editable": true,
                      "statement_summary": "Allow s3:GetObject on arn:aws:s3:::photos/*" } }
  ]
}
```

`origin` ∈ `minio-builtin` | `harbormaster-template` | `custom`. `editable` is true only for
`custom`.

`GET /api/v1/policies/{name}` — single resource, adds `document` (parsed JSON policy object)
to attributes.

`POST /api/v1/policies`

```json
{ "data": { "type": "policies",
  "attributes": { "name": "photos-readonly",
    "document": { "Version": "2012-10-17",
      "Statement": [ { "Effect": "Allow", "Action": ["s3:GetObject"],
                       "Resource": ["arn:aws:s3:::photos/*"] } ] } } } }
```

Success `201` (returns the resource incl. `document`). Errors (JSON:API `errors[]`):
`422 invalid_policy_json`, `422 invalid_policy_structure`, `422 invalid_policy_name`,
`409 policy_name_reserved`, `422 minio_rejected_policy`.

`PUT /api/v1/policies/{name}` — replace `document`. `403 policy_read_only` for non-custom.

`DELETE /api/v1/policies/{name}` — Success `204`. `403 policy_read_only`;
`409 policy_in_use` with `details.attached_to: { users: [...], groups: [...] }`.

### 5.4 User custom-policy attachment (extended)

`PUT /api/v1/users/{access_key}/policies` request gains an optional `policies` array of
plain canned-policy names, alongside the existing `templates`:

```json
{ "templates": [{ "name": "read-write", "params": null }],
  "policies": ["photos-readonly", "logs-writer"] }
```

`GET /api/v1/users/{access_key}` user attributes gain `attached_policies: ["photos-readonly"]`
(custom/named policies, distinct from `attached_templates` and the read-only "other" row).
`422 unknown_policy` if a named policy does not exist on the deployment.

### 5.5 Metrics

`GET /api/v1/metrics?window=24h&step=auto` — `window` ∈ `1h|6h|24h|7d`; `step` optional
(server picks a sane bucket size per window).

```json
{
  "window": "24h",
  "step_seconds": 300,
  "collected": true,
  "series": {
    "api_requests_total":   [ { "t": "2026-05-28T00:00:00Z", "v": 12.4 }, ... ],
    "api_errors_4xx":       [ ... ],
    "api_errors_5xx":       [ ... ],
    "rx_bytes_per_sec":     [ ... ],
    "tx_bytes_per_sec":     [ ... ]
  },
  "capacity": { "total_bytes": 0, "used_bytes": 0, "free_bytes": 0,
                "history": [ { "t": "...", "used_bytes": 0 } ] },
  "drives": { "online": 4, "offline": 0, "history": [ ... ] }
}
```

`collected: false` + empty series when the poller has no samples yet (fresh install) or
collection is paused; the page renders an explanatory banner. `422 invalid_metrics_window`
for a bad `window`.

### 5.6 New error codes

| Code | HTTP | Meaning |
| ---- | ---- | ------- |
| `cannot_restore_delete_marker` | 422 | Restore targeted a delete marker |
| `not_delete_marked` | 422 | Undelete on a non-delete-marked latest version |
| `unsupported_lifecycle_kind` | 422 | `kind` not in {expiration, noncurrent-expiration, abort-incomplete-multipart} |
| `invalid_policy_json` | 422 | Document is not valid JSON |
| `invalid_policy_structure` | 422 | Document is not a well-formed IAM policy |
| `invalid_policy_name` | 422 | Policy name violates MinIO naming rules |
| `policy_name_reserved` | 409 | Name collides with a Harbormaster bundled template |
| `policy_read_only` | 403 | Edit/delete attempted on built-in/bundled policy |
| `policy_in_use` | 409 | Delete blocked; policy attached to users/groups |
| `minio_rejected_policy` | 422 | MinIO rejected the policy document |
| `unknown_policy` | 422 | User attach referenced a non-existent policy |
| `invalid_metrics_window` | 422 | `window` not in {1h,6h,24h,7d} |

## 6. Data Model

Harbormaster persists no MinIO-authoritative state for versions, lifecycle, or policies —
those are read from / written to MinIO on demand (consistent with v1). New persistence is
limited to metrics samples.

**New table `metrics_samples`** (GORM, SQLite):

| Column | Type | Notes |
| ------ | ---- | ----- |
| `id` | ULID PK | |
| `collected_at` | timestamp (UTC), indexed | poll time |
| `metric` | string, indexed | e.g. `api_requests_total`, `rx_bytes`, `capacity_used_bytes`, `drives_online` |
| `value` | float64 | counter or gauge raw value |

- Counters (e.g. request totals, bytes) are stored raw; rate/derivative is computed at query
  time from adjacent samples. Composite index on `(metric, collected_at)`.
- Retention sweeper deletes rows older than `HARBORMASTER_METRICS_RETENTION` (default 8d),
  reusing the audit-retention sweeper pattern.
- Migration adds the table only; no changes to existing tables. No backfill.

No schema change is required for versions, lifecycle, or policies.

## 7. Service Impact

- **`internal/objects`** — add version listing (provider/model/resource/rest), version-aware
  download/preview (`version_id` param), and the restore/delete-version/undelete actions.
  New S3 calls: `ListObjectVersions`, `CopyObject` (same-key, source version id) for restore,
  `RemoveObject` with `VersionID` for delete-version and for removing the delete marker on
  undelete.
- **`internal/lifecycle`** — extend `builder`/`model`/`classifier`/`resource` for two new
  managed kinds; extend deterministic-ID generation and the unmanaged-vs-managed classifier;
  broaden the kind-validation error. Existing `SetBucketLifecycle` plumbing reused.
- **`internal/policies`** — currently a materializer for bundled templates. Add a new
  canned-policy CRUD surface (list/info/add/remove) and a structural policy-document
  validator. New `madmin` calls: `ListCannedPolicies`, `InfoCannedPolicy`, `AddCannedPolicy`,
  `RemoveCannedPolicy`. Likely a new `resource.go`/`rest.go` for the `/policies` domain.
- **`internal/users`** — extend the policy-attachment processor so a user's effective policy
  set = bundled-template-materialized policies ∪ named custom policies; surface
  `attached_policies` in the user resource. Reuse `SetUserPolicy` / policy attach plumbing;
  ensure detach semantics don't strip externally-managed policies Harbormaster doesn't own.
- **`internal/observability`** (or a new `internal/metrics`) — add the metrics poller
  (background goroutine started in `serve.go`), a MinIO metrics client, the SQLite sample
  store + sweeper, and the aggregation/query layer behind `GET /api/v1/metrics`.
- **`internal/dashboard`** — unchanged, but the new Metrics page is a sibling; confirm no
  overlap/duplication of the existing point-in-time totals.
- **`internal/db`** — add `metrics_samples` migration.
- **`internal/config`** — add `HARBORMASTER_METRICS_POLL_INTERVAL`,
  `HARBORMASTER_METRICS_RETENTION` (and any metrics-auth knobs the design settles on).
- **Frontend (`apps/frontend`)** — Objects tab: version-history drawer/modal + per-version
  actions + delete-marker/undelete affordances; Lifecycle form: kind selector with the two
  new kinds and their fields; new **Policies** page (list + JSON editor with validation) and
  a "Custom policies" section on user-detail; new **Metrics** page with charts (window
  selector). New nav entries for Metrics and Policies. Reuse existing JSON:API client,
  React Query, react-hook-form + Zod, and shadcn/Recharts patterns.

## 8. Non-Functional Requirements

- **Performance:** version listing and metrics queries must paginate / bucket server-side;
  a 7d metrics window must not return more than a few hundred points per series (downsample
  via `step`). The metrics poller must add negligible load (one scrape per interval).
- **Security:** no MinIO metrics endpoint, presigned URL, or policy document is exposed to
  the browser beyond what the operator explicitly requests; version downloads stay within
  the authenticated proxy/direct model; CSRF on all writes; permanent-delete and
  restore actions are clearly distinguished in the UI to prevent accidental data loss.
- **Auditability:** every mutation emits an audit event with actor/IP/outcome; documents and
  URLs are excluded from payloads.
- **Resilience:** metrics collection degrades to gaps (never 500s the page) when MinIO is
  down; policy/lifecycle/version operations surface MinIO errors verbatim under typed codes.
- **Consistency:** follows the v1 JSON:API + action-endpoint split, the per-domain DDD
  package layout, and the immutable-model/functional-composition backend guidelines.
- **Testing:** unit + golden tests per the existing package conventions; race-clean
  (`go test -race`), `go vet`, `golangci-lint`, frontend lint/format/test/build all green
  per CLAUDE.md's "done" bar.

## 9. Open Questions

1. **Metrics source mechanism.** Prometheus text endpoint (`/minio/v2/metrics/cluster`,
   `/minio/v2/metrics/node`) with an admin-generated bearer token, vs. `madmin` MetricsV3 /
   realtime API. Both meet §4.4; the design phase picks one (and the exact metric names that
   back each series). Recommendation: whichever the installed `madmin`/MinIO version exposes
   most stably for capacity + per-API-status request counters.
2. **External-Prometheus alternative.** Should we *also* allow pointing Harbormaster at an
   existing operator Prometheus (federation) instead of self-collecting? Deferred unless the
   built-in poller proves insufficient; the API contract in §5.5 is source-agnostic so this
   can be added later without breaking the frontend.
3. **Restore semantics on a non-versioned bucket** — restore/undelete are simply hidden when
   versioning was never enabled; confirm there's no edge case where a bucket has versions but
   versioning is currently *suspended* (S3 allows this) that needs distinct UI wording.
4. **Custom-policy detach safety** — exact rule for not clobbering policies attached outside
   Harbormaster when we call `SetUserPolicy` (MinIO's attach is a full set-replace in some
   API versions). Must be resolved in design to avoid silently detaching `consoleAdmin`.
5. **`newer_noncurrent_versions`** support depends on the MinIO release; verify the field is
   honored by the target server versions and degrade gracefully if not.

## 10. Acceptance Criteria

**Versioning**
- [ ] On a versioned bucket, opening an object shows its full version history with version
      IDs, sizes, timestamps, latest flag, and delete markers.
- [ ] A specific past version can be downloaded and previewed.
- [ ] Restoring a past version creates a new current version (verified via re-listing) with
      no client-side byte transfer; delete markers cannot be restored.
- [ ] A single version can be permanently deleted (with confirmation) and disappears from the
      history.
- [ ] A delete-marked object shows as "deleted" and can be undeleted, re-exposing the prior
      version.
- [ ] All version mutations appear in the audit log; no documents/URLs in payloads.

**Lifecycle**
- [ ] A noncurrent-version expiration rule (with optional `newer_noncurrent_versions` and
      prefix) can be created, listed as managed, and deleted.
- [ ] An abort-incomplete-multipart rule can be created, listed as managed, and deleted.
- [ ] Both new kinds round-trip through the classifier as `managed`.
- [ ] Creating a noncurrent rule on a non-versioned bucket warns but is permitted.
- [ ] `transition` (and any other kind) is rejected with `unsupported_lifecycle_kind`.

**Policies**
- [ ] The Policies page lists built-in, bundled-template, and custom policies with correct
      `origin`/`editable` flags.
- [ ] A valid custom policy can be created from JSON; malformed JSON and malformed policy
      structure are rejected with the typed codes before reaching MinIO; reserved names are
      rejected.
- [ ] A custom policy can be edited and deleted; built-in/bundled policies cannot
      (`policy_read_only`); deleting an attached policy returns `policy_in_use` with
      attachment details.
- [ ] A custom policy can be attached to and detached from a user without clobbering
      externally-managed policies; `attached_policies` reflects the change.
- [ ] All policy mutations are audited; documents are never written to the audit payload.

**Metrics**
- [ ] The Metrics page renders request-rate, error-rate, throughput, and capacity series over
      1h/6h/24h/7d windows against a live MinIO with no external Prometheus.
- [ ] A fresh install (no samples yet) shows the "collection in progress/paused" banner, not
      an error.
- [ ] Killing MinIO produces gaps in the series and the paused banner, and recovery resumes
      collection automatically.
- [ ] `metrics_samples` retention sweeper prunes rows older than the configured horizon.

**Cross-cutting**
- [ ] Backend: `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`,
      `CGO_ENABLED=0 go build ./...` all clean.
- [ ] Frontend: `npm run lint`, `npm run format`, `npm test`, `npm run build` all clean.
- [ ] New nav entries (Metrics, Policies) present; version browsing reachable from the
      Objects tab.
- [ ] No transition/tiering, object-lock, groups, events, or encryption surface was added
      (scope discipline).
