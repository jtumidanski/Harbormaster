# Harbormaster MVP v1 — Design

Status: Draft
Task ID: task-001-harbormaster-mvp-v1
Companion to: `prd.md`, `api-contracts.md`, `data-model.md`, `risks.md`
Author: design phase (2026-05-24)

This document commits to the architecture and resolves every "design-phase decision" called out in `prd.md` §9 and `risks.md`. It deliberately stops short of a step-by-step task plan — that belongs in `plan.md` (phase 3). The intent of this design is to lock in:

1. **Technology choices** the implementation will be built on (one decision per row, each with the rejected alternative and why).
2. **Bounded contexts and package layout** for the backend and frontend so the plan can be sliced cleanly.
3. **Cross-cutting concerns** (auth, CSRF, encryption, audit, SSE, structured errors) that every feature inherits.
4. **Internal milestone shape** so this large task can land as a stacked series of mergeable sub-branches into `task-001-harbormaster-mvp-v1`.
5. **Risk countermeasures** distilled from `risks.md` into concrete design hooks.

Everything in §1 of this doc is "follow this." Everything in §3 is "do not deviate without revisiting this doc."

---

## 1. Decisions Locked In

### 1.1 Stack & libraries

| Area | Decision | Rejected alternative(s) | Reason |
| --- | --- | --- | --- |
| Go version | 1.24 (latest stable) | — | Skill baseline; required for `slog`/embed maturity, generics ergonomics |
| Backend module path | `github.com/jtumidanski/Harbormaster` (capital `H`, per repo casing) | lower-case alias | Matches the upstream GitHub URL; GHCR image stays lower-case (`ghcr.io/jtumidanski/harbormaster`) |
| HTTP router | `github.com/go-chi/chi/v5` | Echo, Fiber, stdlib only | stdlib-shaped `http.Handler`, mature middleware ecosystem (chi/middleware), zero external runtime deps, easy to test with `httptest` |
| Middleware stack | chi built-ins (`RequestID`, `Recoverer`, `Timeout`, `RealIP`) + in-house: `Logger`, `Auth`, `CSRF`, `JSONApiContentType`, `AuditTagger` | reinvent | The chi extras we need are < 40 LOC each |
| JSON:API encoding | Hand-rolled in `internal/jsonapi/` (~250 LOC: encode collection, encode single, decode request, render errors) | `google/jsonapi` (unmaintained), `manyminds/api2go` (heavy, opinionated routing) | Full control over field order, custom errors shape, JSON:API patterns prescribed by `backend-dev-guidelines/resources/patterns-rest-jsonapi.md` |
| Multipart form transport | Action endpoints; `Content-Type: multipart/form-data`, plain JSON error envelope | shoehorning into JSON:API | Avoids the JSON:API + multipart mismatch called out in `risks.md` R9 |
| SSE transport | Stdlib `http.Flusher` + a 40-line `internal/sse` writer (writes `event:`/`data:`/`: keepalive` framing + sets `X-Accel-Buffering: no`, `Cache-Control: no-cache`, `Connection: keep-alive`) | `r3labs/sse` | One file we control; addresses the proxy-buffering risk (R16) head-on by always emitting heartbeats and the right headers |
| Persistence | GORM (`gorm.io/gorm`) + SQLite | `sqlc`, hand-written `database/sql` | `backend-dev-guidelines` mandates GORM-backed providers; GORM's per-connection `PRAGMA` hook covers our SQLite tuning |
| SQLite driver | `github.com/glebarez/sqlite` (wraps `modernc.org/sqlite`, pure Go) | `mattn/go-sqlite3` (CGO) | Pure-Go enables `CGO_ENABLED=0` + `gcr.io/distroless/static-debian12:nonroot` + simple arm64 cross-compile (`risks.md` R3, PRD §7.3) |
| Migration runner | `github.com/golang-migrate/migrate/v4` with `iofs` source and `embed.FS` of `migrations/*.sql` | `goose`, hand-rolled | Battle-tested, forward-only by configuration, supports SQLite via pure-Go `modernc`-compatible driver path, plays well with our `db.sqlite` instance |
| Password hashing | argon2id (`golang.org/x/crypto/argon2`), RFC 9106 minimum params: `memory=64MiB`, `iterations=3`, `parallelism=2`, salt=16B, hashLen=32B; encoded as the standard `$argon2id$v=19$m=...$...` PHC string | bcrypt | Argon2id is the modern recommendation; cost knobs are explicit |
| Symmetric encryption | AES-256-GCM via `crypto/cipher`, 12-byte random nonce, `base64( nonce \|\| ciphertext \|\| tag )` (see `data-model.md` §6.2) | NaCl secretbox, age | Stdlib-only; matches existing column shape |
| Logging | `github.com/rs/zerolog` for the application; tiny `internal/observability/log` wrapper exposes `log.Ctx(ctx)` and field helpers | `zap`, `logrus`, `slog` | Allocation-free, JSON-default, contextual logger via `zerolog.Ctx`. PRD §9 q8 listed zerolog/zap; zerolog wins on dependency size for our static binary. The `backend-dev-guidelines` skill mentions logrus as the generic reference; this design **deviates by name only** — the wrapper exposes the same `Info/Warn/Error + fields` shape, so call-sites match the skill's pattern |
| Metrics | `github.com/prometheus/client_golang` exposed on a separate listener bound only when `HARBORMASTER_METRICS_ENABLED=true` (PRD §4.2, §5.1) | OpenMetrics directly | Skill-compatible; the secondary listener is its own `http.Server` with no shared middleware |
| Tracing | `go.opentelemetry.io/otel` + OTLP-HTTP exporter, enabled only when `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` is set | always-on | Off-by-default keeps the production image lean for homelab users |
| Config loading | `github.com/spf13/viper` with explicit precedence: env (`HARBORMASTER_*`) > config file (`HARBORMASTER_CONFIG`) > defaults | hand-rolled | Standard, supports YAML/TOML/JSON files, mockable via a thin `internal/config` wrapper |
| MinIO admin client | `github.com/minio/madmin-go/v3` | reverse-engineering admin API | Upstream-maintained, matches the admin endpoints we need |
| MinIO S3 client | `github.com/minio/minio-go/v7` | aws-sdk-go-v2 | Native MinIO client, supports `PutObject` multipart fan-out, `ListObjectsV2` continuation tokens, `RemoveObjects` bulk delete (1000 obj batches), and `ListObjectVersions` |
| ULID | `github.com/oklog/ulid/v2` | `google/uuid` | Lexicographically sortable IDs we already chose in `data-model.md` |
| Test framework | Stdlib `testing` + `github.com/stretchr/testify/{assert,require}` for ergonomics | `ginkgo`, `gocheck` | Skill convention; `testify` is widely understood and minimal |
| Integration tests | `github.com/testcontainers/testcontainers-go` with `modules/minio` | hand-rolled docker driver | First-party MinIO module; gated by `HARBORMASTER_INTEGRATION=1` env (R13 split) |
| Frontend bundler | Vite 5 + React 18 + TypeScript 5 (strict) | CRA, Next.js | PRD §7.2 + frontend skill |
| Frontend libs | `@tanstack/react-query`, `react-router-dom`, `react-hook-form`, `zod`, `@hookform/resolvers/zod`, `sonner`, `lucide-react`, `tailwindcss`, shadcn/ui-generated primitives, `@tanstack/react-virtual` | Mantine, AntD, custom | All required by frontend skill + PRD §8.6 (virtualization) |
| Frontend test runner | Vitest + React Testing Library + jsdom | Jest | Vitest aligns with Vite, faster TS pipeline; skill mentions Jest as a generic baseline but the user-visible behavior (`describe/it`, RTL queries) is identical and Vitest is the natural fit for a Vite project. `npm test` script invokes Vitest |
| E2E (smoke) | Playwright, gated to a single golden-path test in `apps/frontend/e2e/smoke.spec.ts`, run on-demand via `npm run test:e2e` (not on every PR) | Cypress | Lightweight; runs against a `docker compose up` stack. CI runs it only on `main` |
| Linters | `golangci-lint` (Go) with `errcheck`, `gosec`, `govet`, `staticcheck`, `revive`, `gocyclo`, `unparam`, `unused`, `bodyclose`, `noctx`, `forbidigo` (forbids `fmt.Println`, `panic` in non-test, etc.); `eslint` (TS) with `@typescript-eslint`, `react-hooks`, `jsx-a11y`; `prettier` for formatting | bare gofmt | PRD §7.4 |
| Container base image | `gcr.io/distroless/static-debian12:nonroot` | Alpine, scratch | PRD §7.3 (resolved). Combined with `CGO_ENABLED=0` and pure-Go SQLite |

### 1.2 Open questions from PRD §9 — final answers

| # | Question | Decision |
| --- | --- | --- |
| 1 | Web framework / router | **chi v5** (§1.1) |
| 2 | JSON:API library | **Hand-rolled** in `internal/jsonapi/` (§1.1) |
| 3 | GORM vs sqlc | **GORM** (§1.1) |
| 4 | Migration runner | **golang-migrate** with embedded `iofs` source (§1.1) |
| 5 | argon2id vs bcrypt | **argon2id** with RFC 9106 params (§1.1) |
| 6 | Session store backend | **SQLite** (already resolved in PRD; no change) |
| 7 | MinIO test strategy | **Split**: fast unit suite always-on (in-memory SQLite, MinIO faked behind interfaces); integration suite gated by `HARBORMASTER_INTEGRATION=1`, running `testcontainers-go/modules/minio`. CI runs unit on every PR and integration nightly via a separate workflow `nightly.yml` |
| 8 | Logging library | **zerolog** (§1.1) |
| 9 | Distroless variant | **`gcr.io/distroless/static-debian12:nonroot`** (already resolved) |
| 10 | cosign signing | **Include in v1.** `main.yml` calls `sigstore/cosign-installer` + `cosign sign --yes ghcr.io/...@<digest>` using keyless OIDC signing. Cheap to add (~10 lines of workflow), high security value, no key material to manage |
| 11 | Repo URL casing | **Resolved**: `jtumidanski/Harbormaster` (capital `H`); image path `ghcr.io/jtumidanski/harbormaster` |
| 12 | License | **AGPL-3.0-or-later** (resolved) |
| 13 | SSE implementation | **Stdlib + tiny in-house encoder** (§1.1) |
| 14 | Empty-bucket operation tracking | **Persistent `bucket_empty_jobs` table** + a single background worker goroutine. SSE handlers subscribe to a per-job Go channel for live progress; reconnects after process restart attach by reading the row's `state` (`running`/`done`/`error`) and replaying the final event. See §3.6 |
| 15 | Tag-filter readout for unmanaged lifecycle rules | **Show count only, never values.** The `summary` string says "scoped to N tag filter(s)" with N alone |
| 16 | Default-mount mc config in compose | **Do not bind-mount by default.** Ship a commented-out volume line with a comment explaining the trade-off, plus a 4-line "Importing from mc config" section in the README quick-start |

### 1.3 Risk countermeasures distilled

Each `risks.md` item maps to a concrete design hook:

| Risk | Design hook |
| --- | --- |
| R1 (scope) | Six internal milestones (§4) — each stacked branch back into `task-001-harbormaster-mvp-v1`, each independently demoable |
| R2 (encryption key fragility) | Key file lives at `<data_dir>/encryption.key` by default; bootstrap writer enforces `0600`; startup loader recomputes SHA-256 fingerprint and compares against `app_settings.encryption_key_fingerprint`, refusing to start on mismatch with a `key_fingerprint_mismatch` exit code |
| R3 (CGO/static tension) | Pure-Go SQLite via `glebarez/sqlite` (§1.1) |
| R4 (destructive bucket ops) | Empty-bucket modal displays object count + total size + versioning-aware copy; `purge_versions` checkbox default-off; typed bucket-name confirmation; audit event records final counts (PRD §4.7, §6.1) |
| R5 (object browser perf) | Server-side pagination (continuation tokens), `@tanstack/react-virtual` rows, auto-load at 90 % with one-outstanding-request cap, manual "Load more", `BucketInfo.objects` headline in UI for deep prefixes (§5.4) |
| R6 (in-memory login rate limit) | Single replica documented; k8s manifests pin `replicas: 1` + `strategy: Recreate`; startup warning when the deployment is detected as multi-replica via `HARBORMASTER_REPLICA_COUNT` env if operator sets it |
| R7 (raw k8s manifests) | Manifests are heavily commented; first 30 lines of each file list the parameters operators typically tune; "Helm chart" tracked as the immediate v1.1 follow-up task |
| R8 (admin CLI rescue) | `cmd/harbormaster admin reset-password` + `cmd/harbormaster admin reset-encryption`; docs show `docker exec` and `kubectl exec` one-liners; `--help` for each subcommand prints expected DB path |
| R9 (JSON:API + multipart) | `lib/api/client.ts` centralizes both content-type paths; `internal/api/apierror` exposes both error envelopes from one error type |
| R10 (Trivy noise) | `.trivyignore` ships with the repo with a documented monthly review cadence; HIGH allowlisted only with comment, CRITICAL never |
| R11 (GHCR private by default) | README "first release" checklist explicitly includes the GHCR "Make public" step |
| R12 (SPA-asset disambiguation) | Server router registration order documented in §5.2; `/api/*`, `/healthz`, `/readyz`, `/assets/*`, `/metrics` (separate listener), all other GETs accepting `text/html` → `index.html` |
| R13 (test budget) | Split unit / integration as in question 7; integration suite naming convention `*_integration_test.go` + `//go:build integration` tag |
| R14 (AGPL deps) | `dependency-scan` workflow runs an allowlist check (compatible licenses: Apache-2.0, MIT, BSD-2/3-Clause, ISC, AGPL-3.0-or-later, GPL-3.0-or-later, MPL-2.0-with-justification); `tools/licenses/allowlist.yaml` holds exceptions |
| R15 (MinIO version drift) | Supported floor documented in README: **`RELEASE.2025-01-01T00-00-00Z` or later**; integration tests pin both the floor and latest stable as parallel matrix entries |
| R16 (SSE buffering) | `internal/sse` always emits `X-Accel-Buffering: no` + heartbeat every 15 s; example compose ships `nginx.conf` snippet (`proxy_buffering off; proxy_read_timeout 1h;`) plus Caddy/Traefik notes in docs; UI shows "no progress in 30s" stall warning |
| R17 (share-link non-revocability) | Share-link modal copy is explicit; audit event records bucket+key+TTL; docs include MinIO secret-key rotation recipe |
| R18 (mc config exposure) | mc-aliases endpoint gated on `setup_completed=false`; access logged once per read; no persistence of file contents; example compose leaves the volume commented-out (question 16) |

---

## 2. Bounded Contexts

The backend is partitioned into eight bounded contexts plus four cross-cutting platform packages. Each context is one package under `internal/` and contains the standard skill-prescribed files (`model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `resource.go`, `rest.go`). Cross-cutting packages don't follow the seven-file shape — they're libraries, not domains.

| Context | Owns | Talks to |
| --- | --- | --- |
| `setup` | First-run state, mc-alias parsing, initialization gate | `crypto`, `connection`, `auth` |
| `auth` | `admin_users`, `sessions`, login rate limit, CSRF tokens, password change | `audit`, `crypto` |
| `connection` | `minio_connections` row, validation handshake (tcp/list/admin-ping) | `crypto`, `minio` (transient clients) |
| `dashboard` | Aggregate view (server info, totals, recent activity, recent failures) | `connection`, `audit`, `minio` |
| `buckets` | Bucket CRUD, versioning, public access, quota, **empty job orchestration** | `audit`, `minio`, `objects` (for the post-empty deletion target list) |
| `objects` | Object listing, upload, download (proxy + direct), delete, share-link minting | `audit`, `minio`, `buckets` (for enforcing per-bucket constraints) |
| `users` | IAM users, service accounts, policy attachment | `audit`, `policies`, `minio` (madmin) |
| `policies` | Bundled policy templates (read-only / read-write / backup-target), template materialization | `minio` (madmin policy ops) — read-only from `users` |
| `lifecycle` | Per-bucket lifecycle rules (managed + unmanaged readout) | `audit`, `minio` |
| `audit` | Local event writer, retention sweeper, query handler | none (terminal sink) |

Cross-cutting platform packages (not bounded contexts; they're shared libraries):

| Package | Purpose |
| --- | --- |
| `internal/config` | Viper wrapper, validation, fail-fast plumbing |
| `internal/db` | SQLite open + PRAGMAs, GORM init, migration runner harness |
| `internal/crypto` | AES-256-GCM helpers, encryption-key loader, fingerprint check |
| `internal/jsonapi` | Encode collection/single, decode request body, error rendering |
| `internal/apierror` | Typed error sentinels, mapping to HTTP status + envelope shape (JSON:API errors[] vs action `{error:{code,message}}`) |
| `internal/sse` | Tiny SSE writer (event/data/heartbeat/`X-Accel-Buffering`) |
| `internal/minio` | Builds and caches the `madmin` + `minio-go` clients keyed off the encrypted connection row; auto-rebuilds on `connection.update` |
| `internal/server` | HTTP server bootstrap, middleware order, embedded SPA `fs.FS`, graceful shutdown |
| `internal/observability` | zerolog setup, request-ID middleware, optional Prometheus, optional OTLP wiring |
| `internal/jobs/bucketempty` | Background worker for the empty-bucket operation (single-flight per bucket via the partial unique index) |

**Cross-domain orchestration** (per `backend-dev-guidelines/resources/architecture-overview.md`): when a handler needs multiple contexts (e.g., bucket creation that also applies a lifecycle template), orchestration lives in the **processor**, not the handler. The dashboard aggregate is the documented exception — it calls multiple processors directly because it's a read-only fan-out.

---

## 3. Cross-Cutting Concerns

### 3.1 Configuration

`internal/config` exposes a single `config.Load(ctx) (Config, error)` that:

1. Loads defaults.
2. Reads the file at `HARBORMASTER_CONFIG` if set.
3. Overlays env vars with prefix `HARBORMASTER_`.
4. Validates: listen addr parses; data dir exists or is creatable; durations parse; `HARBORMASTER_BASE_PATH` begins with `/`; `HARBORMASTER_DOWNLOAD_PROXY_MODE` ∈ {`proxy`,`direct`}; `HARBORMASTER_LOG_FORMAT` ∈ {`json`,`console`}; TLS cert/key both present or both absent; CIDRs in `HARBORMASTER_TRUSTED_PROXIES` parse.
5. Returns a typed `Config` value (immutable). No globals.

Fail-fast on validation error: log the structured error and exit non-zero. `Config` is passed by value to every constructor.

### 3.2 Database open & migrations

`internal/db.Open(cfg) (*gorm.DB, error)` performs:

1. `glebarez/sqlite` open with DSN `file:<path>?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)` (these get applied per connection on a pure-Go SQLite driver). Verified at boot by running `PRAGMA journal_mode;` and asserting `wal`.
2. Migration runner: `migrate.NewWithSourceInstance("iofs", iofs.New(migrationsFS, "migrations"), ...)` driven by `migrate/database/sqlite3` (which works with the modernc-backed driver via the registered `sqlite` driver name). Run forward-only at startup; fail-fast on error.
3. Pin GORM to use a single shared `*sql.DB` (don't let GORM open a second). Configure `MaxOpenConns=1`, `MaxIdleConns=1`, `ConnMaxLifetime=0` — SQLite is single-writer; serializing writes through one connection avoids busy errors. Reads pass through the same connection in WAL mode.

**Retry on `SQLITE_BUSY`** (PRD §8.2): a thin GORM plugin wraps `Create/Save/Updates/Delete/Raw` calls with an exponential-backoff retry capped at 1 s. Cap of 5 attempts; if all fail, surface as `500 internal_error`.

### 3.3 Encryption-at-rest

`internal/crypto.Load(cfg) (*Cipher, error)`:

1. Resolve key path = `HARBORMASTER_ENCRYPTION_KEY_FILE` if set, else `<data_dir>/encryption.key`.
2. If file doesn't exist: generate 32 random bytes from `crypto/rand`, write with `0600`, log a `key_generated` event.
3. If file exists: stat for `0600` perms. World-readable → fatal exit. Group-readable → warning and continue.
4. Compute SHA-256 fingerprint, compare against `app_settings.encryption_key_fingerprint` if set. Mismatch → fatal exit with code `key_fingerprint_mismatch`. If unset (first boot), write the fingerprint after migrations succeed.
5. Expose `Encrypt([]byte) (string, error)` / `Decrypt(string) ([]byte, error)` returning base64 ciphertext per `data-model.md` §6.2 envelope.

**Plain credentials never touch disk** outside the encrypted column; they live in memory inside `internal/minio`'s client struct and are zeroed when the client is rebuilt.

### 3.4 Auth + session middleware

Middleware order (outermost to innermost for `/api/v1/*` other than `/setup/*`, `/csrf`, `/auth/login`):

```
RequestID  →  Recoverer  →  Logger  →  RealIP  →  Timeout(30s)
  →  RequireSession  →  RequireCSRF (writes only)  →  AuditTagger  →  Handler
```

- `RequireSession` reads `harbormaster_session` cookie, looks up in `sessions` table, verifies `expires_at > now`, populates `req.Context()` with `auth.SessionInfo{AdminUserID, SessionID, SourceIP}`.
- `RequireCSRF` runs only when method ∉ {GET, HEAD, OPTIONS}. Double-submit pattern: cookie value (set by `/api/v1/csrf`) must equal `X-CSRF-Token` header byte-for-byte. Mismatch → `403 csrf_token_invalid`.
- `AuditTagger` writes `request_id`, `actor`, `source_ip` into context for the audit writer to pick up automatically.
- **Login rate limit:** in-memory sliding window keyed by source IP, 5 failures per 5 minutes. Reset on success. (R6: documented as single-replica only.)

Cookie attributes:
- `harbormaster_session`: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=<base_path>`, opaque ULID-shaped random 256 bits (base64url).
- `harbormaster_csrf`: same flags **except** `HttpOnly=false` (so JS can read it for the header).
- Both regenerated on login (rotate session ID) to defeat session-fixation.

### 3.5 Audit writer

`internal/audit.Writer` exposes `Write(ctx, Event)` and a context-bound helper `audit.From(ctx).Record(action, target, outcome, payloadSummary, err)`. Behavior:

- Every state-changing handler `defer`s a single `audit.Record(...)` call after the work completes. On error, `outcome=failure` and `error_message=err.Error()` (truncated to 1024 bytes).
- `payload_summary` is built with a `audit.Sanitize` helper that **drops any field whose name matches** `secret|password|token|csrf|signature|presigned|url`. A `TestNoSecretsInPayload` unit test enforces this on every action type. PRD §8.3 "no secrets in logs/metrics/traces/audit" is enforced by this single helper.
- Retention sweeper runs as a goroutine ticking every 24 h, deleting rows where `occurred_at < now - HARBORMASTER_AUDIT_RETENTION`. Logged at info level.

### 3.6 Empty-bucket job (PRD §9 q14, R4)

A persistent `bucket_empty_jobs` table (per `data-model.md`) is created. The mechanism:

1. **`POST /api/v1/buckets/{name}/empty`**: handler validates `confirm_name`, opens an SSE response, then asks `jobs/bucketempty.Service.StartOrAttach(name, opts)`:
   - If a row with `state='running'` exists for the bucket, return the existing job's progress channel.
   - Otherwise INSERT a new row (partial unique index on `(bucket_name) WHERE state='running'` guarantees single-flight) and spawn a goroutine.
2. **Worker goroutine** iterates `ListObjectsV2` (or `ListObjectVersions` when `purge_versions=true && versioning_enabled`), batches 1000 keys at a time into `RemoveObjects`, updates `deleted_count` and `last_progress_at` after each batch (UPDATE not INSERT — keeps the row small), and broadcasts each batch's progress to all attached SSE channels via a `chan Progress` fanout.
3. **Terminal state**: row moves to `state='done'` (or `error` with `error_message`), `finished_at` is set, channels are closed, a single `bucket.empty` audit event is written with final counts + `duration_ms` + `purge_versions` choice.
4. **Reconnect after process restart**: on startup, the service flips any `running` row to `error` with `error_message="orphaned by restart"` and writes a `bucket.empty` audit event recording the orphan. The operator can re-invoke. (This is the documented limitation of the persistent-job model — re-attaching the goroutine across restarts is out of scope; the row's terminal state is enough for the UI to render "this operation died, please retry.")
5. **Stall warning**: SSE writes a `: keepalive` comment every 15 s. The frontend renders a "no progress in 30 s" warning when it observes no `progress` events for that long (R16).

### 3.7 MinIO client lifecycle

`internal/minio.Pool` holds a single `*madmin.AdminClient` and a single `*minio.Client` (both safe for concurrent use). On startup and after every successful `connection.update`, the pool calls `Rebuild()` which:

1. Decrypts the current `minio_connections` row.
2. Builds new clients with the decrypted creds + custom CA (added to the transport's `RootCAs` if present).
3. Atomically swaps the pointers behind a `sync.RWMutex`.
4. Zeroes the previous credentials' backing slice.

All domain processors take `func(ctx) (*madmin.AdminClient, *minio.Client, error)` providers (per skill provider pattern) so they never bind to a specific instance and pick up rotations automatically.

### 3.8 Error envelope

`internal/apierror.Error` carries `Code string`, `HTTPStatus int`, `Title string`, `Detail string`, `Pointer string` (JSON:API), and an optional `Details map[string]any`. The HTTP layer renders:

- **JSON:API resource routes** (`GET/POST/PATCH/DELETE /api/v1/buckets`, `/users`, `/lifecycle-rules`, `/audit-events`, `/policy-templates`, `/users/.../service-accounts`): `errors[]` array shape.
- **Action routes** (`/auth/*`, `/setup`, `/setup/mc-aliases`, `/connection/*`, `/buckets/{name}/empty`, `/buckets/{name}/public-access`, `/buckets/{name}/quota`, `/buckets/{name}/versioning`, `/buckets/{name}/objects/download`, `/buckets/{name}/objects/share-links`, `/dashboard`): plain `{error:{code,message,details?}}` envelope.

A small `apierror.Write(w, err)` switch dispatches on a per-handler `apierror.Style` constant (set during route registration) so the handler code stays envelope-agnostic.

### 3.9 SPA serving (R12)

`internal/server` registers routes in this exact order:

1. `/api/v1/*` → JSON API mux (all sub-routes mounted here).
2. `/healthz`, `/readyz` → health handlers.
3. `/assets/*`, `/favicon.ico`, `/manifest.webmanifest` → `embed.FS` static handler with content-type sniffing from extension (not bytes) and `Cache-Control: public, max-age=31536000, immutable` for hashed asset paths.
4. **Fallback** → SPA index for any GET request whose `Accept` header contains `text/html`; respond `404 not_found` (no body) otherwise. The SPA fallback always serves `index.html` with `Cache-Control: no-cache`.

Subpath mounting (`HARBORMASTER_BASE_PATH=/harbormaster`): the chi router is mounted under that prefix using `chi.Mux.Mount`, the SPA's `<base href>` is rewritten on-the-fly at serve time by a tiny template substitution that replaces `<base href="/" />` with the configured path, and cookies are pinned to the same prefix in their `Path=` attribute.

### 3.10 Health endpoints

- `/healthz`: process up + HTTP listener serving. Always 200 once the server starts accepting connections.
- `/readyz`: 200 only when (a) `db.Open` succeeded, (b) migrations finished, (c) if `setup_completed=true`, the cached MinIO admin client's last `AccountInfo` call succeeded within the last 30 s. A background prober ticks every 10 s and stamps the cache.

### 3.11 Frontend cross-cutting

- **API client** (`lib/api/client.ts`): centralizes CSRF header injection (reads cookie, sets `X-CSRF-Token` on every non-GET), JSON:API vs action envelope detection, request dedup via React Query, 401-handler that clears the auth state.
- **Auth context** (`context/AuthContext.tsx`): minimal — login state, the currently authenticated username. Logout invalidates the React Query cache.
- **Query keys** (`lib/hooks/api/keys.ts`): hierarchical factory per skill convention, e.g. `bucketsKeys.detail(name) = ["buckets", "detail", name] as const`. Empty-bucket SSE uses an imperative `EventSource` (not React Query), but the terminal event invalidates `bucketsKeys.detail(name)` and `objectsKeys.list(name, "")`.
- **Form layer**: every form uses `react-hook-form` + `zodResolver`. Schemas live in `lib/schemas/<feature>.ts` and re-export inferred types. No inline schemas in components.
- **Error toasts**: `lib/api/errors.ts` exposes `createErrorFromUnknown(unknown) → AppError` and a `toastError(err)` helper that maps known codes to friendly copy. `toast.success` for write success, except destructive ops (delete bucket, revoke key) which use a slightly more pronounced toast variant.
- **Subpath**: Vite's `base` is set from `import.meta.env.VITE_BASE_PATH`, defaulted to `/`. The backend rewrites `<base href>` at serve time so the same built bundle works under any prefix.
- **Theme**: Tailwind `dark:` classes; `ThemeProvider` reads/writes `localStorage["theme"]` and defaults to `matchMedia("(prefers-color-scheme: dark)")`.

---

## 4. Internal Milestone Plan (Stacked Branches)

This task is too large for a single PR (R1). Internal execution will land as **six stacked sub-branches**, each off the previous, all eventually rebased onto `task-001-harbormaster-mvp-v1`. Each milestone is independently demoable. The plan phase will turn each milestone into a concrete task list in `plan.md`.

### M0 — Repo scaffolding & CI baseline (smallest, lands first)

- Repo layout per PRD §4.1 (`apps/`, `deploy/`, `docs/`, `.github/`, `scripts/`, `LICENSE`, `README.md`).
- `apps/backend`: Go module init, `cmd/harbormaster/main.go` that just logs "hello", `go.mod` with the locked-in deps.
- `apps/frontend`: Vite + React + TS scaffold, Tailwind, shadcn/ui init, single empty App component.
- `Dockerfile` multi-stage shell (frontend build → Go build → distroless), no app logic.
- `.github/workflows/pr.yml` with **all** the jobs the PRD requires (frontend lint/test/build, backend lint/test/build, gitleaks, dependency-scan), wired but mostly trivially passing.
- `.golangci.yml`, `eslint.config.mjs`, `.prettierrc`, `.editorconfig`, `.trivyignore`, `renovate.json5`, `tools/licenses/allowlist.yaml`.
- `LICENSE` (AGPL-3.0-or-later), `README.md` with boilerplate.

**Demoable:** `docker compose up` produces a container that serves a placeholder index. CI is green.

### M1 — Backend platform (config, DB, crypto, auth middleware, audit, server bootstrap)

- `internal/config`, `internal/db`, `internal/crypto`, `internal/audit`, `internal/jsonapi`, `internal/apierror`, `internal/sse`, `internal/observability`, `internal/server`.
- Migrations `0001_admin_users.sql`, `0002_sessions.sql`, `0003_minio_connections.sql`, `0004_app_settings.sql`, `0005_audit_events.sql`, `0006_bucket_empty_jobs.sql`.
- Auth/CSRF middleware shells (no handlers yet — wired up to fail closed on `/api/v1/*`).
- `/healthz`, `/readyz` working.
- CLI subcommands: `harbormaster serve`, `harbormaster version`, `harbormaster admin reset-password`, `harbormaster admin reset-encryption`.
- Tests: config validation, crypto round-trip + perms check, migration up/down, audit writer no-secrets test, JSON:API encode/decode, SSE writer.

**Demoable:** the binary boots, migrates, serves `/healthz`/`/readyz`, and rejects every `/api/v1` request with `401 unauthenticated`.

### M2 — Setup wizard + auth

- `setup` context: `GET /api/v1/setup/status`, `GET /api/v1/setup/mc-aliases` (version-10 only, file-presence guarded), `POST /api/v1/setup` (both explicit + alias forms).
- `auth` context: `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`, `POST /api/v1/auth/password`, `GET /api/v1/csrf`.
- `connection` context: `GET /api/v1/connection`, `PUT /api/v1/connection`, `POST /api/v1/connection/test`.
- `internal/minio.Pool` first version, rebuilds on connection update.
- Frontend: setup wizard route, login page, auth context, app shell with sidebar nav, theme toggle, base-path support.

**Demoable:** an operator can run `docker compose up`, point a browser at it, complete setup, log in, and see an empty "Buckets" page.

### M3 — Buckets, objects, lifecycle (the largest milestone)

- `buckets` context: list / get / create / delete (with `bucket_not_empty` gate) / versioning / public-access / quota / empty (SSE + persistent job).
- `objects` context: list (paginated continuation tokens) / upload (multipart, 100 MiB cap) / delete / download (proxy + direct modes) / share-link.
- `lifecycle` context: list (managed + unmanaged readout) / create (expiration only) / delete.
- `internal/jobs/bucketempty` worker.
- Frontend: bucket list, bucket detail, object browser with virtualized rows + 90 %-auto-load, upload modal, share-link modal, lifecycle-rules tab, empty-bucket modal with SSE progress + stall warning.

**Demoable:** full bucket and object workflows work end-to-end against a real MinIO instance.

### M4 — Users, service accounts, policy templates

- `policies` context: bundled templates in Go source (`read-only`, `read-write`, `backup-target`), template materialization (`harbormaster-<template>-<bucket?>` policy creation/upsert via `madmin`).
- `users` context: list / create (one-time secret) / status / delete (typed confirmation) / policy attachment.
- Service accounts under `users/{access_key}/service-accounts`.
- Frontend: users list, user detail with attached templates + read-only "Other attached policies" row, create-user modal with one-time secret reveal, service-account list under each user.

**Demoable:** an operator can create IAM users with bundled template policies and service accounts, copy the secret once, then use those credentials with `mc`.

### M5 — Dashboard + activity feed

- `dashboard` context: `GET /api/v1/dashboard?failures_window=7d` aggregate.
- `audit` exposed query handler: `GET /api/v1/audit-events?filter[...]&page[...]`.
- Frontend: dashboard page with server info, totals, node health, warnings, recent activity, recent-failures widget (window selector persisted in `localStorage`), `/activity` page with filters + pagination, "See all" deep link from dashboard widget.

**Demoable:** all PRD §4.6 dashboard requirements rendered; `/activity` filters and paginates.

### M6 — Deployment, CI/CD, supply chain

- Finalize `deploy/docker/Dockerfile` + `docker-compose.yml` + `.env.example`.
- `deploy/kubernetes/{deployment,service,ingress.example,secret.example,pvc}.yaml` with extensive comments.
- `.github/workflows/main.yml`: multi-arch buildx, GHCR push, trivy image scan (pre-push), cosign keyless sign post-push.
- `.github/workflows/release.yml`: tag-triggered, attaches release notes, links to image.
- `.github/workflows/nightly.yml`: integration suite against MinIO floor + latest stable.
- README finalization (quick-start, configuration reference, security model, AGPL boilerplate, GHCR publish-public reminder).
- `docs/` architecture overview, configuration reference, operator security guide.

**Demoable:** a tagged release on GitHub produces a signed multi-arch image on GHCR and a GitHub Release page; `docker compose up` from a clean clone works end-to-end.

Each milestone gets its own `/audit-plan` + `superpowers:requesting-code-review` pass before being merged into the task branch (per CLAUDE.md "Code Review Before PR" rule).

---

## 5. Component Detail Notes

The following are the design clarifications worth pinning down before the plan phase. They're not exhaustive; they're the calls that are easy to get wrong and expensive to revisit.

### 5.1 JSON:API encoder shape

`internal/jsonapi` exposes:

```
type Resource interface {
    ResourceType() string
    ResourceID() string
}
type Encoder struct { ... }
func (e *Encoder) Single(w io.Writer, r Resource, attrs any, links *Links) error
func (e *Encoder) Collection(w io.Writer, rs []Resource, attrsFn func(Resource) any, meta *Meta, links *Links) error
func (e *Encoder) Error(w io.Writer, code int, errs ...apierror.Error) error
type Decoder struct { ... }
func (d *Decoder) Single(r io.Reader, out any) error // out is *AttrsStruct
```

Each domain's `rest.go` defines an `attrs` struct and a `Resource` adapter on the model. No reflection beyond `encoding/json`. Pointer slices avoid heap thrash for large bucket lists.

### 5.2 Server bootstrap & middleware

`internal/server.New(cfg, deps).Run(ctx)` builds the chi mux, mounts sub-routers from each context (each exposes `Routes() chi.Router`), wires the SPA fallback, and runs two `http.Server`s (main + metrics) with graceful shutdown on SIGTERM (10 s drain — PRD §8.2).

Middleware globals applied at the root mux:

1. `chimw.RequestID` — sets `request_id`.
2. `chimw.Recoverer` — recovers from panics, returns `500 internal_error` with the request ID for log lookup.
3. `observability.Logger` — structured log line per request with method, path, status, latency, request ID; uses `zerolog.Ctx(r.Context())`.
4. `chimw.RealIP` — only honored when the request came from an IP in `HARBORMASTER_TRUSTED_PROXIES`.
5. `chimw.Timeout(30s)` — default per-request budget. Long-running endpoints (`/buckets/{name}/empty`, `/buckets/{name}/objects/download`) override locally.

API sub-router adds `auth.RequireSession` and `auth.RequireCSRF` after the globals.

### 5.3 Bucket empty SSE wire format

Already specified in `api-contracts.md`. Operational notes:

- `progress` event fires after each `RemoveObjects` batch and on a 5 s timer if the batch is taking longer (to avoid stale UIs during slow MinIO responses).
- `done.data` includes `deleted_total` (final count) and `duration_ms` (wall time).
- `error.data.message` is the truncated `madmin`/`minio-go` error message; never the raw secret-bearing URL.
- Heartbeat `: keepalive\n\n` every 15 s. Frontend stall threshold 30 s.

### 5.4 Object browser data flow (frontend)

```
ObjectsPage
  └─ useInfiniteObjects(bucket, prefix)   ← React Query useInfiniteQuery
        └─ services/objects.list({bucket, prefix, page_token})
              └─ apiClient.get("/api/v1/buckets/{bucket}/objects?...")
  └─ VirtualizedObjectList                ← @tanstack/react-virtual
        ├─ rowVirtualizer with 36px estimateSize
        ├─ onScroll handler that, when (scrollTop+clientHeight)/scrollHeight >= 0.9
        │     AND no fetchNextPage is in flight
        │     AND hasNextPage,
        │     calls fetchNextPage() once
        ├─ "Load more" button at the bottom (manual fallback)
        └─ Row component renders `key`, `size`, `last_modified`, `content_type`,
            with action menu (download / delete / share-link)
```

The auto-load + manual button + one-outstanding cap together solve R5.

### 5.5 Upload pipeline (frontend → backend → MinIO)

1. Frontend `fetch("/api/v1/buckets/{name}/objects", { method: "POST", body: formData })` with the file. `xhr`/`fetch` `progress` events drive a single per-upload progress bar.
2. Backend handler validates session + CSRF, validates Content-Length ≤ cap (rejecting up-front when the client sends it; falling back to a `http.MaxBytesReader` for chunked uploads), then streams the body directly to `minio.PutObject` via `mpart.NextPart()` and the SDK's `ContentType` form field.
3. On 413, the response body uses the action error envelope with `code: upload_too_large` and `details.limit_bytes`.
4. On success, audit event `object.upload` with `{bucket, key, size_bytes, content_type}`. **Never** the body.

### 5.6 Download pipeline modes

- **Proxy** (default): handler calls `minio.GetObject` and `io.Copy` to the response writer with `Content-Disposition: attachment; filename="<basename>"`. Streaming — no buffering. Aborted streams don't write an audit event.
- **Direct**: handler calls `minio.PresignedGetObject(ctx, bucket, key, 5*time.Minute, nil)` and responds `307 Temporary Redirect` with `Location: <presigned URL>`. No audit event (the actual bytes flow browser→MinIO).
- The choice is read once at startup from `HARBORMASTER_DOWNLOAD_PROXY_MODE`; the UI's Download button is mode-agnostic.

### 5.7 Share link minting

- `POST /api/v1/buckets/{name}/objects/share-links` body: `{key, expires_seconds}`.
- Server clamps `expires_seconds` into `[30, HARBORMASTER_SHARE_LINK_MAX_TTL]` silently.
- `minio.PresignedGetObject` with the clamped duration.
- Audit event `object.share_link.create` with `{bucket, key, expires_seconds}` — **never** the URL (it embeds the signature).
- Frontend modal: copy-to-clipboard, expiry timestamp, plus an explicit "Cannot be revoked" warning copy block above the button (R17).

### 5.8 Lifecycle rule readout

- `madmin.GetBucketLifecycle` returns a `lifecycle.Configuration` with `Rules []Rule`.
- A rule is **managed** iff `ID` matches `^harbormaster-expire-\d+d(-[a-z0-9.-]+)?$` AND it has exactly one `Expiration{Days:N}` action and no filter except prefix.
- Anything else is **unmanaged** and rendered as a read-only summary string. The summary lists action kinds (`Expiration`, `Transition`, `AbortIncompleteMultipart`, etc.) and tag-filter **count** (R15 — never values).
- Creating a rule writes a new rule with `ID = harbormaster-expire-<days>d[-<prefix-slug>]`; the prefix slug is the prefix lowercased + non-`[a-z0-9.-]` stripped, truncated to 32 chars.

### 5.9 Policy template materialization

`internal/policies` defines:

```
type Template struct {
    Name string                                // "read-only" | "read-write" | "backup-target"
    Render func(params map[string]string) (json.RawMessage, error)
    ParamsSchema *json.RawMessage              // nil for templates without params
}
```

Bundled templates live as Go literals (compile-time JSON validity via `var _ json.RawMessage = ...` and a unit test that round-trips each through `json.Unmarshal`). Attaching to a user calls `madmin.InfoCannedPolicy` first; if the named policy doesn't exist, creates it via `madmin.AddCannedPolicy`, then `madmin.AttachPolicy`.

The user-detail page lists all policies attached on the MinIO side; any policy whose name doesn't start with `harbormaster-` is rendered in a read-only "Other attached policies" row (R: PRD §4.10).

### 5.10 Frontend route map

```
/                                          → Redirect to /dashboard
/setup                                     → SetupWizard
/login                                     → LoginPage
/dashboard                                 → DashboardPage
/buckets                                   → BucketListPage
/buckets/:name                             → BucketDetailPage
/buckets/:name/objects                     → ObjectBrowserPage
/buckets/:name/lifecycle                   → LifecycleRulesPage
/users                                     → UserListPage
/users/:accessKey                          → UserDetailPage
/activity                                  → ActivityFeedPage (filters in query string)
/settings/connection                       → ConnectionSettingsPage
/settings/account                          → AccountSettingsPage (change password)
/*                                         → NotFoundPage
```

Auth guard wraps all non-`/setup`, non-`/login` routes. Setup guard redirects to `/setup` when `initialized=false`. Both guards use `useQuery(setupStatusKey)` and `useQuery(authMeKey)` and render a skeleton while in flight.

---

## 6. Out of Scope (Reaffirmed)

For clarity to the plan phase — these are **not** in v1, and any pull toward them should be deferred:

- Multi-tenant or multi-cluster control
- OIDC/SSO/external auth
- Helm chart (top-priority follow-up after v1)
- Encryption-key rotation (additive future migration only)
- Multipart-resume UI, presigned-PUT direct upload, share-link revocation
- Object version browsing UI
- Advanced metadata editing
- Freeform IAM JSON policy editor
- `consoleAdmin` template
- Tag-filter creation UI for lifecycle rules
- Sliding session expiration
- Sessions-management UI (active sessions list, revoke)
- Object lock / legal hold
- Backup orchestration

Anything outside this PRD requires a fresh `/spec-task`.

---

## 7. Build & Verification Commands (post-implementation)

Once the implementation lands, the commands required to call a branch "done" become:

- Backend: `go test ./... -race -count=1` (unit), `HARBORMASTER_INTEGRATION=1 go test ./... -tags=integration -count=1` (integration, requires Docker), `go vet ./...`, `golangci-lint run`, `go build ./...`.
- Frontend: `npm ci`, `npm run lint`, `npm test`, `npm run build`.
- Container: `docker build -f deploy/docker/Dockerfile .` (must succeed for both `linux/amd64` and `linux/arm64` via `docker buildx`).
- E2E smoke (on demand, not per-PR): `npm run test:e2e` (Playwright against `docker compose up`).

CLAUDE.md "Build & Verification" should be updated with these exact commands as part of M0.

---

## 8. Glossary (terms used above and in PRD)

- **Bounded context** — A package with its own model/processor/provider trio; corresponds to one PRD §4 sub-section.
- **Processor** — Pure business logic; depends on Providers (data) and other Processors (cross-domain orchestration).
- **Provider** — Lazy data accessor; returns Model values built from Entity rows.
- **Builder** — Fluent constructor enforcing model invariants; used in tests and command flows.
- **Action endpoint** — Non-resource HTTP route (login, empty-bucket, share-link, etc.); uses plain JSON envelope.
- **Resource endpoint** — JSON:API CRUD route on a collection; uses `data`/`errors` shape.
- **Managed lifecycle rule** — A rule whose `ID` matches the `harbormaster-*` pattern AND whose shape is the expiration-only form Harbormaster creates.
- **Unmanaged lifecycle rule** — Any rule on the bucket that doesn't match the managed predicate. Read-only in Harbormaster (delete allowed, edit not).
- **Single-flight (empty bucket)** — At most one `running` row per bucket in `bucket_empty_jobs`; enforced by a partial unique index. New requests for the same bucket attach to the in-flight operation.

---

**Next phase:** `/clear`, then `/plan-task task-001` from inside the worktree.
