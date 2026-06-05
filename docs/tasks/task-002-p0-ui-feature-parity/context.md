# P0 UI Feature Parity — Implementation Context

Companion to `plan.md`. This captures the key files, verified facts, and decisions an engineer needs before touching code. Read this first.

## Source documents

- PRD: `docs/tasks/task-002-p0-ui-feature-parity/prd.md`
- Design: `docs/tasks/task-002-p0-ui-feature-parity/design.md`

## Verified SDK facts (do NOT re-derive from memory)

Versions (from `apps/backend/go.mod`): `minio-go/v7 v7.0.74`, `madmin-go/v3 v3.0.66`, `prom2json v1.3.3` (currently `// indirect`).

- **minio-go version listing:** only the high-level channel API `Client.ListObjects(ctx, bucket, ListObjectsOptions{WithVersions: true, Prefix: key})` exposes versions. `ListObjectsOptions.WithVersions bool` exists (api-list.go:687). There is **no** public version-id-marker pagination on `Core`. → in-memory windowing (design §2.1).
- **minio-go restore/delete:** `Core.CopyObject(ctx, srcBucket, srcObject, dstBucket, dstObject, metadata, CopySrcOptions{VersionID}, PutObjectOptions)` (core.go:59); `CopySrcOptions.VersionID string` (api-compose-object.go:150); `RemoveObjectOptions.VersionID string` (api-remove.go:126); `GetObjectOptions`/`StatObjectOptions` both carry `VersionID`. `ObjectInfo` has `VersionID`, `IsLatest`, `IsDeleteMarker` (api-datatypes.go:198-200).
- **madmin canned policies:** `ListCannedPolicies(ctx) (map[string]json.RawMessage, error)`, `InfoCannedPolicy(ctx, name) ([]byte, error)`, `AddCannedPolicy(ctx, name, []byte) error`, `RemoveCannedPolicy(ctx, name) error`. All on `*madmin.AdminClient`.
- **madmin groups (for `policy_in_use`):** `ListGroups(ctx) ([]string, error)`, `GetGroupDescription(ctx, group) (*GroupDesc, error)`; `GroupDesc{Members []string, Policy string}`. Group's attached policy is the `Policy` field (comma-joined names).
- **madmin metrics:** `madmin.NewMetricsClient(endpoint, accessKey, secretKey string, secure bool) (*MetricsClient, error)` (metrics_client.go:77); `(*MetricsClient).SetCustomTransport(http.RoundTripper)` (metrics_client.go:168); `(*MetricsClient).ClusterMetrics(ctx) ([]*prom2json.Family, error)` and `.ResourceMetrics(ctx)` (prometheus_metrics.go:47,57). The realtime `AdminClient.Metrics(...)` does **not** carry S3 request/error/traffic series — do not use it.
- **prom2json shapes:** `Family{Name string, Type string, Metrics []interface{}}`; elements are `prom2json.Metric{Labels map[string]string, Value string}` (Value is a string — `strconv.ParseFloat`). Histograms/summaries are other element types; skip non-`Metric` elements.

## Per-domain DDD conventions (verified in existing code)

Layering: `model → builder → provider → administrator → processor → resource → rest`. The processor depends on a small **unexported** interface (`s3API` / `adminAPI`) injected via a `ClientGetter func(ctx) (iface, error)`. An **exported** mirror interface (`S3Client` / `AdminClient`) plus `NewClientGetter(resolve func(ctx) (Exported, error)) ClientGetter` is the supported wiring point; tests inject a getter returning a hand-rolled stub. See `objects/processor.go:44-94`, `lifecycle/processor.go:21-55`, `users/processor.go:22-66`.

- **JSON:API resources** implement `jsonapi.Resource` (`ResourceType()`, `ResourceID()`, `MarshalJSON()`); encode via `jsonapi.NewEncoder().Single/Collection`; decode request attributes via `jsonapi.NewDecoder().Single(&attrs)`. Domain structs carry **no** JSON tags — wire shaping lives in `rest.go` (`objects/rest.go`, `lifecycle/rest.go`).
- **Action endpoints** return plain JSON; errors render `apierror.StyleAction`. Resource routes render `apierror.StyleJSONAPI`.
- **Errors:** `apierror.New(status, code, msg)`; `.WithPointer("/data/attributes/x")` for JSON:API source; `.WithDetails(map)` for action details (e.g. `policy_in_use.attached_to`). `mapClientError` wraps raw SDK errors as 502 `minio_error`/`minio_unavailable` and passes typed `*apierror.Error` through.
- **Audit:** every mutation uses the `failAudit` closure pattern — build a `payload` map, define `failAudit(err)` that records `Outcome: OutcomeFailure`, then record `OutcomeSuccess` at the end. `recordAudit` is nil-safe (`objects/processor.go:162-167`). Action verbs are constants in `audit/model.go`; `AllActions()` must list every new verb (a test enumerates it). Payloads are sanitized by `audit.Sanitize` (drops `*url*`/`*token*`/`*secret*` keys) — never put documents or presigned URLs in a payload.
- **`actorFromRequest(r)`** (duplicated per package) pulls `(username, sourceIP)` off the auth session context.

## Pool & wiring

- `internal/minio/pool.go`: `Pool.Get(ctx) (*madmin.AdminClient, *miniogo.Client, error)`; `ErrNotInitialized` when unconfigured. Pool privately holds `cred Credentials` and builds an `*http.Transport` via `transport(c, useTLS)`. **It does NOT currently expose credentials or a transport** — the metrics track adds `Pool.NewMetricsClient(ctx)` (Task E-pool).
- Client getters live in `cmd/harbormaster/audit_adapter.go`. `objectS3Adapter{*miniogo.Client}` already routes `ListObjectsV2` through a synthesized `miniogo.Core`; the versions feature extends this adapter. The live `*madmin.AdminClient` satisfies `users.AdminClient` structurally.
- Routes are registered in `cmd/harbormaster/serve.go` `protectedRoutes` (≈ lines 206-234). Background goroutines (audit sweeper) start at ≈ line 88; the metrics poller + sweeper start in the same region.
- Migrations: `apps/backend/migrations/NNNN_name.up.sql` + `.down.sql`, embedded via `migrations/embed.go`, run by `db.Migrate`. Latest is `0006_bucket_empty_jobs`. New: `0007_metrics_samples`.
- Config: `internal/config/config.go` — add fields to `Config`, a `v.SetDefault` in `defaults()`, read in `Load()`, and validation in `validate()`.

## Frontend conventions

- API client: `src/lib/api/client.ts` (`api.get/post/put/delete`, auto CSRF, JSON:API Accept). Errors: `src/lib/api/errors.ts` (`AppError{status,code,message,details,pointer}`, parses both `errors[]` and `{error}`). Query keys: `src/lib/api/keys.ts`.
- Per-feature folder under `src/features/<name>/` with `api.ts`, `types.ts`, page/dialog components and colocated `*.test.tsx` (Vitest + Testing Library).
- Forms: `react-hook-form` + `zodResolver`; shadcn `Form*` primitives; `useMutation` invalidating query keys; `toast` (sonner) for success/error; `AppError` for typed messages. Discriminated forms: see lifecycle `CreateRuleDialog.tsx`.
- Infinite lists: `useInfiniteQuery` with opaque `next_token` pageParam (`features/objects/useInfiniteObjects.ts`).
- Charts: Recharts via shadcn `components/ui/chart.tsx` (`ChartContainer`/`ChartConfig`/`ChartTooltip`), pattern in `features/dashboard/BucketSizeChart.tsx`. Window persistence to `localStorage`: pattern in `DashboardPage.tsx:15-29`.
- shadcn primitives present: `sheet`, `chart`, `select`, `dialog`, `table`, `tabs`, `alert`, `badge`, `form`, `input`.
- Nav: `components/AppShell.tsx` `NAV` array (line 44). Routes: `src/routes.tsx`. `/policies` currently maps to read-only `PolicyTemplatesPage` and is **repurposed** into the full Policies page.

## Build / done bar (run in `apps/backend` and `apps/frontend`)

Backend: `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build ./...`.
Frontend: `npm run lint`, `npm run format`, `npm test`, `npm run build`.
After importing `prom2json` directly in `internal/metrics`, run `go mod tidy` (drops the `// indirect` marker).

## Build order (independent tracks; see plan)

A = lifecycle (smallest) → B = objects versions → C = policies CRUD → D = users custom-policy attachment (depends on C's classifier) → E = metrics → F = nav/route integration + final verification.

## Cross-cutting decisions locked in design

- New audit verbs (design §1.1): `object.version.restore`, `object.version.delete`, `object.undelete`, `policy.create`, `policy.update`, `policy.delete`, `user.policy.attach`, `user.policy.detach`. (`lifecycle.rule.create` reused.)
- New error codes (design §1.2 / PRD §5.6): `cannot_restore_delete_marker`, `not_delete_marked`, `unsupported_lifecycle_kind`, `invalid_policy_json`, `invalid_policy_structure`, `invalid_policy_name`, `policy_name_reserved`, `policy_read_only`, `policy_in_use`, `minio_rejected_policy`, `unknown_policy`, `invalid_metrics_window`.
- Version pagination = opaque base64 offset cursor over an in-memory single-key window, cap `maxVersionScan = 10_000` (design §2.1).
- Custom-policy attach safety: owned set = template-materialized ∪ existing `custom`-origin policies; never set-replace, always per-policy `AttachPolicy`/`DetachPolicy`; built-ins/foreign never detached (design §4.4).
- Metrics counters stored raw; rates derived at query time, negative deltas clamped to 0; gauges passed through; ≤ ~300 points/series (design §5.1, §5.2).
