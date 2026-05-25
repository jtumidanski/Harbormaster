# Harbormaster — Architecture overview

This document is a navigational map of the Harbormaster codebase aimed
at new contributors and operators who want to understand how a request
flows through the system. It is descriptive of what was actually
implemented; see `docs/tasks/task-001-harbormaster-mvp-v1/design.md`
for the design rationale and rejected alternatives.

## Top-level layout

```
apps/
  backend/                Go binary (API + embedded SPA)
    cmd/harbormaster/     main.go, serve subcommand, admin CLIs, version
    internal/
      apierror/           Structured error envelope
      audit/              Append-only audit-event processor
      auth/               Sessions, login, CSRF, rate-limit, admin entity
      buckets/            Bucket DDD context (CRUD, settings)
      config/             Viper-backed config loader
      connection/         Encrypted MinIO connection settings
      crypto/             AES-256-GCM helpers; key file load + fingerprint
      dashboard/          Aggregate dashboard read-model
      db/                 GORM open, PRAGMAs, migrate driver
      integration/        Integration tests behind `integration` build tag
      jobs/bucketempty/   Empty-bucket background worker + SSE
      jsonapi/            Hand-rolled JSON:API encoder/decoder
      lifecycle/          Bucket lifecycle-rule DDD context
      minio/              MinIO admin + S3 client pool
      objects/            Object browse/upload/download DDD context
      observability/      zerolog wrapper + Prometheus + OTLP plumbing
      policies/           Policy templates + materializer
      server/             chi router, middleware, SPA embed, health
      setup/              First-run wizard state machine
      sse/                Server-Sent-Events writer
      users/              MinIO users + service accounts DDD contexts
    migrations/           Embedded SQL migrations (golang-migrate iofs)
  frontend/               React 18 + Vite 5 + TS 5 (strict)
    src/                  components/, features/, lib/api/, hooks/, pages/
    e2e/                  Playwright smoke test
deploy/
  docker/                 Dockerfile, docker-compose.yml, nginx/caddy snippets
  kubernetes/             Raw manifests + README
docs/
  architecture/           This document
  operator/               Operator-facing guides (config, security, recovery, proxy)
  tasks/                  Per-task PRD/design/plan/audit
.github/workflows/        pr.yml, main.yml, release.yml, nightly.yml
```

## Bounded contexts

Each domain context in `apps/backend/internal/<ctx>/` follows the same
shape (per the backend-dev-guidelines skill):

- **`model.go`** — immutable domain model, constructor validates.
- **`entity.go`** — GORM entity (DB row shape); never escapes its package.
- **`builder.go`** — entity ↔ model conversion.
- **`administrator.go`** — interface that wraps MinIO operations.
- **`processor.go`** — orchestrates administrator + audit + side-effects;
  this is the public entry point handlers call.
- **`provider.go`** — wires processor to runtime dependencies.
- **`resource.go`** + **`handler.go`** — JSON:API resource shape + HTTP
  handlers.
- **`rest.go`** — route registration helper.

The v1 contexts are: `auth`, `buckets`, `objects`, `lifecycle`,
`users` (with sub-context for service accounts), `policies`,
`connection`, `setup`, `dashboard`, `audit`, `jobs/bucketempty`.

Cross-cutting (not full DDD contexts): `config`, `crypto`, `db`,
`minio`, `jsonapi`, `sse`, `server`, `apierror`, `observability`.

## Request lifecycle (HTTP → MinIO)

A typical state-changing request — e.g. `POST /api/v1/buckets` to
create a bucket — flows through these layers:

```
Browser
  │  POST /api/v1/buckets  Content-Type: application/vnd.api+json
  ▼
chi router (internal/server)
  │  - chi/middleware: RequestID, RealIP, Recoverer, Timeout
  │  - in-house: Logger (zerolog ctx), AuditTagger (per-request actor)
  ▼
Session middleware (internal/auth/middleware.go)
  │  - cookie → DB lookup → ctx-bound session+admin
  │  - 401 if absent/expired
  ▼
CSRF middleware (internal/auth/csrf.go)
  │  - synchronizer token check on non-GET
  ▼
JSON:API content-type middleware
  ▼
Handler (internal/buckets/handler.go)
  │  - decode JSON:API request into domain input
  │  - call processor
  ▼
Processor (internal/buckets/processor.go)
  │  - resolves MinIO client via internal/minio.Pool
  │  - calls administrator (MakeBucket, EnableVersioning, ...)
  │  - records audit event via internal/audit
  │  - optionally enqueues lifecycle creation
  ▼
MinIO (madmin-go / minio-go)
  ▲
  │  S3 / admin API response
  │
Handler encodes JSON:API response (internal/jsonapi)
  │  - 201 + resource document on success
  │  - apierror.Render on failure (typed envelope)
  ▼
Browser
```

Notable variations:

- **Multipart uploads** (`POST /api/v1/buckets/{b}/objects`): action
  endpoint with `Content-Type: multipart/form-data`; errors are a plain
  JSON envelope (not JSON:API) to keep multipart parsing predictable.
  See `internal/objects/handler.go`.
- **SSE empty-bucket** (`POST /api/v1/buckets/{b}/empty`): persists a
  `bucket_empty_jobs` row, spawns a worker goroutine, streams progress
  events through `internal/sse`. Reconnect after process restart attaches
  by reading the job row's `state` and replaying the final event. See
  `internal/jobs/bucketempty/`.
- **SPA assets** (`GET /assets/*`, `GET /favicon.ico`, and any other GET
  accepting `text/html`): served from `//go:embed spa-dist`; all other
  HTML-accepting GETs fall through to `index.html` so client-side
  routing works (see `internal/server/spa.go`).
- **Health** (`GET /healthz`, `GET /readyz`): bypass auth and CSRF; see
  `internal/server/health.go`.

## State

- **SQLite** at `${DATA_DIR}/harbormaster.db`. GORM opens it with
  `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, and
  `MaxOpenConns=1` (the SQLite single-writer constraint).
- **Encryption key** at `${DATA_DIR}/encryption.key` (32 bytes, `0600`).
  Loaded at boot; SHA-256 fingerprint compared to
  `app_settings.encryption_key_fingerprint`; mismatch aborts startup
  with a clear error.
- **In-process singletons**: login rate limiter (per-IP token bucket),
  empty-bucket worker registry, audit-retention sweeper. All explain
  why v1 is single-replica.

## Wire formats

- **JSON:API** (`application/vnd.api+json`) for resource CRUD. The
  hand-rolled encoder lives in `internal/jsonapi/` (~250 LOC).
- **Plain JSON** action envelope for `/admin/*` and multipart-receiving
  endpoints; same `apierror` shape on failure either way.
- **SSE** (`text/event-stream`) for the empty-bucket progress stream.
  Headers: `Cache-Control: no-cache`, `Connection: keep-alive`,
  `X-Accel-Buffering: no`. Heartbeat comment frame every 15 s.

## Build pipeline

- **Backend**: `apps/backend/Makefile` targets `lint`, `test`,
  `test-integration` (gated by `HARBORMASTER_INTEGRATION=1`), `build`
  (CGO disabled, `-X main.version=$(git describe ...)`), `tidy`, `run`.
- **Frontend**: `apps/frontend/` ships `npm run lint`, `npm run format`,
  `npm test` (Vitest + RTL), `npm run build` (Vite → `dist/`),
  `npm run test:e2e` (Playwright; on-demand).
- **Container**: `deploy/docker/Dockerfile` is a multi-stage build —
  Node compiles the SPA, Go compiles the binary embedding the SPA,
  final stage is `gcr.io/distroless/static-debian12:nonroot`. Built
  multi-arch (`linux/amd64`, `linux/arm64`) by
  `.github/workflows/main.yml` (buildx + QEMU).
- **Supply chain**: PR workflow runs lint, test, build, gitleaks,
  Trivy, license-allowlist. Main workflow scans the built image with
  Trivy (CRITICAL,HIGH → exit 1), pushes to GHCR, and cosign-signs
  the digest keylessly via GitHub OIDC. Nightly workflow runs the
  integration suite against MinIO floor + `latest` in a matrix.

## Tests

- Unit tests live next to source as `*_test.go` (always-on).
- Integration tests in `internal/integration/` carry `//go:build
  integration` + skip when `HARBORMASTER_INTEGRATION` is unset. They
  use `testcontainers-go/modules/minio`; the image is the
  default constant unless `HARBORMASTER_MINIO_IMAGE` overrides it
  (used by the nightly matrix).

## Where to start when changing things

- **New API endpoint on an existing context** → that context's
  `handler.go` + `rest.go`; processor method if behaviour is new.
- **New domain context** → mirror an existing one's file layout; wire it
  in `cmd/harbormaster/serve.go`.
- **New config knob** → `internal/config/config.go` (add field, default,
  validation), surface in README + `docs/operator/configuration.md`.
- **New migration** → numbered file under `apps/backend/migrations/`;
  forward-only; golang-migrate `iofs` source picks it up via embed.
- **New UI feature** → `apps/frontend/src/features/<feature>/` (mirrors
  backend context naming); API client in `src/lib/api/`.
