# Context — Harbormaster MVP v1

Companion to `plan.md`. Distills the design into the smallest set of facts an executing engineer (or subagent) needs to keep in working memory while implementing any task. Anything not here lives in `prd.md`, `design.md`, `api-contracts.md`, `data-model.md`, or `risks.md` — read them when a task references a section.

---

## 1. What this task ships

A complete self-hosted MinIO admin UI: one Go binary embedding a React/TS SPA, packaged into a multi-arch GHCR container image with full CI/CD, deployment manifests, and supply-chain controls. State: SQLite + encrypted credentials. License: AGPL-3.0-or-later.

Scope is locked by `prd.md`. Out-of-scope items are reaffirmed in `design.md` §6.

## 2. Repo casing & module identity

- GitHub repo: `jtumidanski/Harbormaster` (capital `H`).
- Go module path: `github.com/jtumidanski/Harbormaster`.
- Container image: `ghcr.io/jtumidanski/harbormaster` (GHCR lowercases).
- Branch this work lands on: `task-001-harbormaster-mvp-v1` (already checked out in this worktree).

## 3. Execution model — six stacked sub-branches

Per `design.md` §4 and `risks.md` R1, this task is too large for a single PR. The plan lands as six **stacked sub-branches**, each off the previous, all eventually rebased onto `task-001-harbormaster-mvp-v1`:

- **M0** — scaffolding + CI baseline
- **M1** — backend platform (config, db, crypto, audit, server, CLI)
- **M2** — setup wizard + auth + connection
- **M3** — buckets + objects + lifecycle
- **M4** — users + service accounts + policies
- **M5** — dashboard + activity
- **M6** — deployment, CI/CD finalization, supply chain

Each milestone is independently demoable and gets its own `superpowers:requesting-code-review` pass before merging into the task branch. Within `plan.md` each milestone is a top-level section; tasks are numbered `T<milestone>.<n>` (e.g., `T3.4`).

## 4. Locked stack

| Concern | Choice | Source |
|---|---|---|
| Go | 1.24 | design §1.1 |
| HTTP router | `chi/v5` | design §1.1 |
| ORM | GORM | design §1.1 |
| SQLite driver | `glebarez/sqlite` (pure Go) | design §1.1 |
| Migrations | `golang-migrate` with `iofs` + embed.FS | design §1.1 |
| Password hash | argon2id (RFC 9106 params) | design §1.1 |
| Encryption | AES-256-GCM, base64(`nonce \|\| ct \|\| tag`) | design §3.3 |
| Logging | zerolog (via `internal/observability/log` wrapper) | design §1.1 |
| MinIO clients | `madmin-go/v3` + `minio-go/v7` | design §1.1 |
| ULID | `oklog/ulid/v2` | design §1.1 |
| Config | viper | design §1.1 |
| Test framework | `testing` + `testify` | design §1.1 |
| Integration tests | `testcontainers-go/modules/minio`, build tag `integration` | design §1.1 |
| Frontend | Vite + React 18 + TS strict | design §1.1 |
| Frontend libs | React Query, React Router, react-hook-form + zod, @tanstack/react-virtual, shadcn/ui, Tailwind, sonner | design §1.1 |
| Frontend tests | Vitest + RTL + jsdom | design §1.1 |
| E2E | Playwright (single smoke, on-demand) | design §1.1 |
| Container base | `gcr.io/distroless/static-debian12:nonroot` | design §1.1 |
| License | AGPL-3.0-or-later | PRD/design |

## 5. Backend layout (canonical)

```
apps/backend/
  cmd/harbormaster/
    main.go                    CLI dispatch (serve, version, admin reset-password, admin reset-encryption)
    serve.go
    admin_reset_password.go
    admin_reset_encryption.go
  internal/
    config/                    Config struct + Load(ctx)
    db/                        Open + PRAGMA + migration runner
    crypto/                    Cipher{Encrypt,Decrypt} + key loader + fingerprint
    jsonapi/                   Hand-rolled encoder/decoder
    apierror/                  Typed error sentinels + envelope dispatcher
    sse/                       40-line SSE writer + heartbeat
    audit/                     model.go, entity.go, builder.go, processor.go, provider.go, administrator.go, resource.go, rest.go, sanitize.go, retention.go
    observability/             zerolog setup, request ID, optional Prometheus, optional OTLP
    server/                    Bootstrap, middleware order, SPA fallback, graceful shutdown
    minio/                     Pool (cached madmin + minio-go), Rebuild on connection update
    auth/                      seven-file domain + middleware (RequireSession, RequireCSRF)
    setup/                     seven-file domain + mc-alias parser
    connection/                seven-file domain
    dashboard/                 aggregate handler
    buckets/                   seven-file domain + empty-job orchestration
    objects/                   seven-file domain + share-link minting
    users/                     seven-file domain + service-accounts sub-handlers
    policies/                  templates (read-only, read-write, backup-target), materializer
    lifecycle/                 seven-file domain + managed/unmanaged classifier
    jobs/bucketempty/          background worker + per-bucket fanout channels
  migrations/                  0001_*.sql … 0006_*.sql
```

**Seven-file pattern** (per `backend-dev-guidelines/resources/file-responsibilities.md`) applies to every bounded-context package: `model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `administrator.go`, `resource.go`, `rest.go`. Cross-cutting platform packages (`config`, `db`, `crypto`, `jsonapi`, `apierror`, `sse`, `observability`, `server`, `minio`) are libraries — they don't need the seven-file shape.

## 6. Frontend layout (canonical)

```
apps/frontend/
  index.html
  package.json
  vite.config.ts
  tsconfig.json
  tailwind.config.ts
  postcss.config.cjs
  eslint.config.mjs
  .prettierrc
  e2e/smoke.spec.ts                       (Playwright)
  src/
    main.tsx
    App.tsx
    routes.tsx
    components/                           shared UI
    components/ui/                        shadcn primitives
    context/AuthContext.tsx
    context/ThemeProvider.tsx
    features/
      setup/                              SetupWizard, mc-alias selector
      auth/                               LoginPage, password change
      dashboard/                          DashboardPage, RecentFailuresWidget
      buckets/                            list, detail, create, delete, public-access, quota, versioning, empty modal (SSE)
      objects/                            browser, upload modal, share-link modal, preview
      users/                              list, detail, create, status, policies
      service-accounts/                   list under user, create modal
      policies/                           template list
      lifecycle/                          rules list + create form
      activity/                           feed page + filters
      connection/                         settings page
    lib/
      api/client.ts                       fetch wrapper, CSRF injection, dual envelope
      api/errors.ts
      api/keys.ts                         hierarchical query-key factories
      schemas/                            zod schemas per feature
      hooks/                              React Query hooks
    styles/index.css
```

## 7. Cross-cutting rules (memorize)

- **Middleware order** on `/api/v1/*` (excluding `/setup/*`, `/csrf`, `/auth/login`): `RequestID → Recoverer → Logger → RealIP → Timeout(30s) → RequireSession → RequireCSRF (writes only) → AuditTagger → Handler`. See design §3.4 and §5.2.
- **Cookies:** `harbormaster_session` HttpOnly+Secure+SameSite=Lax+Path=`base_path`; `harbormaster_csrf` same minus HttpOnly. Both regenerated on login (rotate session ID).
- **CSRF:** double-submit cookie. Cookie value must equal `X-CSRF-Token` header byte-for-byte. Mismatch → `403 csrf_token_invalid`.
- **Audit:** every state-changing handler `defer`s a single `audit.From(ctx).Record(action, target, outcome, payload, err)`. `payload_summary` runs through `audit.Sanitize` which drops any field whose name matches `secret|password|token|csrf|signature|presigned|url`. There is a `TestNoSecretsInPayload` test enforcing this on every action constant.
- **Encryption:** key file at `<data_dir>/encryption.key` by default. Startup checks SHA-256 fingerprint against `app_settings.encryption_key_fingerprint`. Mismatch → fatal `key_fingerprint_mismatch`. World-readable perms → fatal. Group-readable → warning.
- **Error envelopes:** JSON:API `errors[]` for resource routes; `{error:{code,message,details?}}` for action routes. Selected per-route by an `apierror.Style` constant.
- **SPA serving order:** `/api/*` → `/healthz`,`/readyz` → `/assets/*`,`/favicon.*` → SPA fallback (GET only, `Accept` contains `text/html`). 404 otherwise.
- **SSE:** writer emits `X-Accel-Buffering: no`, `Cache-Control: no-cache`, `Connection: keep-alive`. Heartbeat `: keepalive\n\n` every 15 s.
- **Subpath:** `HARBORMASTER_BASE_PATH` defaults to `/`; chi mux mounted under it; `<base href>` rewritten at serve time; cookies pinned to `Path=<base_path>`.
- **SQLite tuning:** `MaxOpenConns=1`, `MaxIdleConns=1`, `ConnMaxLifetime=0`; `journal_mode=WAL`, `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000`; GORM plugin retries `SQLITE_BUSY` with exponential backoff capped at 1 s (5 attempts).
- **MinIO client pool:** single `madmin.AdminClient` + `minio.Client` swapped atomically behind `sync.RWMutex` on `connection.update`. Providers take `func(ctx) (*madmin.AdminClient, *minio.Client, error)` so they always see the live pair.
- **No-secrets log lint:** a Go test scans every emitted log/audit/error message for a forbidden allow-list (the MinIO secret key, the encryption key bytes, the session-cookie value, the CSRF token, any `password` literal). Runs in CI.

## 8. Empty-bucket job (`jobs/bucketempty`)

Per design §3.6:

- Persistent `bucket_empty_jobs` table with partial unique index `WHERE state='running'` enforces single-flight per bucket.
- Worker goroutine iterates `ListObjectsV2` (or `ListObjectVersions` if `purge_versions=true && versioning_enabled`), batches 1000 keys into `RemoveObjects`, updates `deleted_count` and `last_progress_at` per batch, fans progress out to all attached SSE channels.
- Reconnects after process restart: startup migration flips any `running` row to `error="orphaned by restart"` and writes a `bucket.empty` audit event recording the orphan.
- Stall warning: SSE heartbeats every 15 s; UI warns at 30 s of no `progress` events.

## 9. Open-question commitments locked in design

All 16 items from `prd.md` §9 are resolved in design §1.2. Quick reference for the ones the plan must enforce mechanically:

| # | Locked decision |
|---|---|
| 1 | chi v5 |
| 2 | Hand-rolled `internal/jsonapi` |
| 3 | GORM |
| 4 | golang-migrate + iofs + embed |
| 5 | argon2id (RFC 9106 params) |
| 7 | Split tests: unit always-on; integration gated by `HARBORMASTER_INTEGRATION=1` + `//go:build integration` tag, runs nightly |
| 8 | zerolog |
| 10 | cosign keyless sign in `main.yml` |
| 13 | Stdlib SSE + in-house encoder |
| 14 | Persistent `bucket_empty_jobs` table |
| 15 | Unmanaged tag-filter readout shows count only, never values |
| 16 | mc-config volume left commented-out in compose; README covers opt-in |

## 10. Risk countermeasures the plan must implement

Each `risks.md` entry has a concrete task in the plan:

- **R2** — fingerprint check in `internal/crypto.Load`; permission check on key file (T1.5).
- **R3** — pure-Go SQLite via `glebarez/sqlite`; `CGO_ENABLED=0` in Dockerfile (T0.7, T1.4).
- **R4** — Empty-bucket modal copy + typed confirmation + audit final counts (T3.10–T3.12).
- **R5** — Virtualized list + 90 % auto-load + one-outstanding cap + manual "Load more" (T3.16).
- **R6** — k8s manifests pin `replicas: 1` + `strategy: Recreate` (T6.5).
- **R8** — `admin reset-password` + `admin reset-encryption` CLIs (T1.8, T1.9); docs in T6.10.
- **R10** — `.trivyignore` with review cadence comment (T0.11); allowlist process in T6.7.
- **R12** — Router registration order enforced + tested in `internal/server` (T1.11).
- **R13** — Unit/integration split with `//go:build integration` (T1.2, T6.8).
- **R14** — License allowlist `tools/licenses/allowlist.yaml` + CI job (T0.12, T6.7).
- **R15** — Floor MinIO release pinned to `RELEASE.2025-01-01T00-00-00Z`; matrixed in nightly (T6.8).
- **R16** — SSE heartbeat + headers + reverse-proxy docs (T1.7, T6.10).
- **R17** — Share-link modal copy + audit-without-URL (T3.18).
- **R18** — mc-aliases gated on `setup_completed=false`; single log line per read; no persistence (T2.2).

## 11. Verification commands (post-implementation)

Per design §7:

- Backend unit: `cd apps/backend && go test ./... -race -count=1`
- Backend integration (Docker required): `cd apps/backend && HARBORMASTER_INTEGRATION=1 go test ./... -tags=integration -count=1`
- Backend vet/lint/build: `cd apps/backend && go vet ./... && golangci-lint run && go build ./...`
- Frontend: `cd apps/frontend && npm ci && npm run lint && npm test && npm run build`
- Container: `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .`
- E2E (on-demand): `cd apps/frontend && npm run test:e2e`

These are codified in `CLAUDE.md` "Build & Verification" as part of T0.14.

## 12. Conventional Commits

All commits use Conventional Commits. Prefixes used by this plan:

- `feat(<scope>): …`
- `fix(<scope>): …`
- `chore(<scope>): …`
- `test(<scope>): …`
- `docs(<scope>): …`
- `ci(<scope>): …`
- `build(<scope>): …`
- `refactor(<scope>): …`

`<scope>` is the smallest meaningful unit, e.g., `backend/auth`, `frontend/buckets`, `deploy/docker`, `ci/main`. Each task in `plan.md` includes the exact commit message to use.

## 13. Reading order for executing engineers/subagents

For each task in `plan.md`, read:

1. The task itself (files + steps).
2. The matching design.md section if referenced.
3. The matching api-contracts.md section for HTTP shape.
4. The matching data-model.md section for schema.
5. The two skill files (`backend-dev-guidelines` or `frontend-dev-guidelines`) and any specific resource file referenced.

Do not read every doc upfront. The plan calls out which doc/section is load-bearing for each task.
