# Harbormaster MVP v1 — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-05-23
Task ID: task-001-harbormaster-mvp-v1
Repository: jtumidanski/Harbormaster
Container image (planned): ghcr.io/jtumidanski/harbormaster

---

## 1. Overview

Harbormaster is a self-hosted administrative web application for managing a **single** MinIO deployment through a modern browser-based UI. It targets homelab operators, self-hosted infrastructure users, k3s/Kubernetes operators, small engineering teams, and developers using S3-compatible storage locally. The product aspires to feel operationally similar to Portainer, Grafana, or Proxmox VE — a focused operational console rather than an enterprise platform.

The MVP delivers a single Go binary that serves both a REST JSON API and an embedded React/TypeScript SPA, packaged as a multi-arch container image published to GitHub Container Registry. The application stores its own state (encrypted MinIO connection credentials, local admin account, session data, local audit/event history) in an embedded SQLite database, requiring no external dependencies. Deployment is via Docker Compose primarily, with example Kubernetes manifests provided.

This PRD describes the complete v1 MVP scope. It is intentionally large and is being tracked as a single task per explicit user decision. The implementation plan (`plan.md`) will break the work into milestone-level phases with intermediate verification checkpoints; this PRD captures **what** v1 must deliver, not **how** the work will be sequenced.

## 2. Goals

### Primary goals (v1 MVP)

- **Simplify MinIO administration.** Provide an intuitive web UI for the most common operational workflows (bucket CRUD, object browsing/upload/download, user and access-key management, template-driven policy assignment, simple lifecycle rules) so an operator can avoid routine `mc` CLI usage.
- **Self-hosted, zero external dependencies.** Application runs entirely inside the user's environment. No SaaS calls, no external databases, no telemetry. SQLite is the only datastore.
- **Single-container, multi-arch deployment.** One Go binary embedding the SPA, packaged into multi-arch (linux/amd64, linux/arm64) container images published to GHCR. Docker Compose is the primary deployment story; example Kubernetes/k3s manifests are bundled.
- **Operationally lightweight.** Stateless app upgrades (state lives in a mounted SQLite file + mounted config), externalized configuration via env vars and/or mounted files, `/healthz` and `/readyz` endpoints, structured JSON logs.
- **Secure by default.** Encrypted credentials at rest, HTTP-only / Secure / SameSite session cookies, CSRF protection on state-changing routes, confirmation gates on destructive operations, secrets never appearing in logs/metrics/traces.
- **Supply-chain-aware CI/CD.** PR workflow with build/test/lint/format/secret-scan/dependency-scan gates; main-branch workflow that builds and publishes multi-arch container images to GHCR; Renovate dependency management with minimum-release-age policies; pinned GitHub Action versions.

### Non-goals (v1 — explicitly out of scope)

- SaaS-hosted deployments
- Multi-cluster or multi-tenant management
- Enterprise IAM management
- Kubernetes Operator functionality
- Distributed replication topology editing
- Full MinIO server configuration editing (mc admin config equivalents)
- Arbitrary / freeform IAM JSON policy editing
- Billing or tenant systems
- AIStor-specific enterprise integrations
- Object locking and legal-hold management
- Backup orchestration (Harbormaster does not orchestrate snapshots/backups of MinIO data)
- Advanced audit log exploration (the local activity feed is intentionally simple)
- Multipart upload UI affordances beyond what the underlying SDK provides transparently
- Object diff or object-version browsing
- OIDC / OAuth2 Proxy / SSO authentication (future scope)
- Helm chart (future scope; raw manifests only in v1)
- Mobile-responsive admin layout beyond what naturally falls out of Tailwind defaults

## 3. User Stories

### As a homelab operator

- I want to deploy Harbormaster as a single Docker container in a few minutes so that I can administer my MinIO instance from a browser instead of memorizing `mc` commands.
- I want to point Harbormaster at my MinIO endpoint with admin credentials during first-run setup so that I never have to put my MinIO admin secret in a config file.
- I want my MinIO admin credentials encrypted on disk so that a stolen SQLite file does not leak them in plaintext.
- I want to see at a glance whether my MinIO server is healthy, how many buckets I have, and how much storage is in use.
- I want to create a bucket, optionally enable versioning, and optionally apply a lifecycle template without touching `mc`.
- I want to upload and download objects through the browser, including drag-and-drop, for ad-hoc tasks.
- I want to create a service account for my backup script, assign it the `backup-target` policy template, and copy its secret key exactly once.
- I want to be prompted to confirm before any destructive action (bucket delete, user delete, credential revoke).
- I want to log out and have my session expire on a configurable schedule.

### As a small dev team member

- I want a teammate to create me an access key with a `read-write` policy template scoped to a specific bucket so that I can use it from my local dev environment.
- I want the application to remain responsive when listing buckets containing tens of thousands of objects.

### As the project maintainer / future contributor

- I want PRs gated on build, test, lint, format, secret scan, and dependency vulnerability scan so that I can merge with confidence.
- I want main-branch merges to automatically build and publish multi-arch container images to GHCR with `latest`, semver, and commit-SHA tags.
- I want Renovate to manage dependency updates with a minimum release age so that a compromised upstream package has time to be flagged before it auto-merges.

## 4. Functional Requirements

### 4.1 Repository structure

The v1 repo lays down the canonical structure for the rest of the project:

```
/apps
  /backend                Go source for the Harbormaster API/SPA-serving binary
  /frontend               React/Vite/TypeScript SPA source
/deploy
  /docker                 Dockerfile, docker-compose.yml example, .env.example
  /kubernetes             Raw manifests (Deployment, Service, Ingress example, Secret example)
/scripts                  Local dev convenience scripts (build, lint, dev server proxy, etc.)
/.github
  /workflows              GitHub Actions workflows (PR, main, release)
  renovate.json5          Renovate configuration (or in repo root, project convention)
/docs                     Product, architecture, and operator documentation
```

Frontend assets compiled during the Docker build stage and embedded into the Go binary via `//go:embed`. **No Node.js runtime is present in the production container image.**

### 4.2 Configuration & runtime

Configuration sources, in priority order (highest wins):

1. Environment variables (prefix: `HARBORMASTER_`)
2. Mounted YAML or TOML configuration file (path settable via `HARBORMASTER_CONFIG`)
3. Built-in defaults

Configurable settings (non-exhaustive — finalized during design phase):

- `HARBORMASTER_LISTEN_ADDR` (default `:8080`)
- `HARBORMASTER_DATABASE_PATH` (default `/var/lib/harbormaster/harbormaster.db`)
- `HARBORMASTER_DATA_DIR` (default `/var/lib/harbormaster`)
- `HARBORMASTER_LOG_LEVEL` (default `info`)
- `HARBORMASTER_LOG_FORMAT` (default `json`; allowed: `json`, `console`)
- `HARBORMASTER_SESSION_TIMEOUT` (default `8h`)
- `HARBORMASTER_SESSION_COOKIE_NAME` (default `harbormaster_session`)
- `HARBORMASTER_TRUSTED_PROXIES` (CIDR list, default empty — affects how the app derives client IPs)
- `HARBORMASTER_BASE_PATH` (default `/`; e.g. `/harbormaster` when reverse-proxied at a subpath. Affects router mount, asset URLs, and session/CSRF cookie `Path`. Must begin with `/`; trailing slash is normalized off.)
- `HARBORMASTER_UPLOAD_MAX_BYTES` (default `104857600` = 100 MiB; per-request hard cap for object uploads through Harbormaster — see §4.8)
- `HARBORMASTER_SHARE_LINK_MAX_TTL` (default `168h` = 7 days; upper bound on the expiry an operator may pick when minting an object share link)
- `HARBORMASTER_DOWNLOAD_PROXY_MODE` (default `proxy`; allowed: `proxy`, `direct`. `direct` returns a presigned MinIO URL to the browser and only works when MinIO is reachable from the browser. See §4.8.)
- `HARBORMASTER_MC_CONFIG_PATH` (default `/root/.mc/config.json` inside the container; only consulted by the first-run setup wizard while `setup_completed=false`. Operators bind-mount their host `~/.mc/config.json` here to pre-fill the connection form. Never read after setup.)
- `HARBORMASTER_TLS_CERT_FILE` / `HARBORMASTER_TLS_KEY_FILE` (optional; if both set, app serves HTTPS)
- `HARBORMASTER_ENCRYPTION_KEY_FILE` (path to a file containing the 32-byte key used to encrypt sensitive columns; if absent, app generates and persists one with `0600` perms inside the data dir on first run)
- `HARBORMASTER_METRICS_ENABLED` (default `false`)
- `HARBORMASTER_METRICS_LISTEN_ADDR` (default `:9090`)
- `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` (optional; enables OTLP trace export if set)

**Fail-fast on startup** for: invalid configuration, unreadable / corrupted SQLite database, failed schema migrations, missing or unreadable encryption key file, listen address bind failure.

### 4.3 Health endpoints

- `GET /healthz` → 200 once the process is up and HTTP listener is serving; no dependency checks.
- `GET /readyz` → 200 only when: the SQLite database opened successfully, schema migrations completed, and (if Harbormaster has previously been bound to a MinIO endpoint) the MinIO admin connectivity check passes; otherwise 503.

### 4.4 Initial setup wizard

A first-run UX (gated by "has-been-initialized" flag persisted in SQLite):

1. **Create local admin account.** Form: username (3–64 chars, lowercase alphanumerics + `_-.`), password (min 12 chars, strength meter shown, server-side bcrypt or argon2id hashing with sensible cost), password confirmation.
2. **Connect to MinIO.** Form: endpoint URL (scheme + host + optional port), MinIO access key, MinIO secret key, `Use TLS` toggle (defaults to true if URL scheme is `https`), `Skip TLS certificate verification` toggle (defaults false, warning displayed when enabled), optional custom CA certificate paste/upload (PEM).

   **Import from `mc` config (optional helper).** If a readable file is present at `HARBORMASTER_MC_CONFIG_PATH` (default `/root/.mc/config.json` inside the container; operators bind-mount their host `~/.mc/config.json` to enable this), the wizard surfaces a dropdown of detected `mc` aliases. Selecting an alias pre-fills endpoint URL, access key, secret key, and TLS flags from the alias; the operator can edit any field before submitting. The mc-config file is consulted **only** while `setup_completed=false`, the file contents are never persisted into Harbormaster's database, and aliases are never returned over the API with their secret keys — the secret is read on the server only when the operator's submission references the alias by name.
3. **Validate.** Backend performs: TCP connectivity test, S3 list-buckets call (validates standard credentials), admin API ping (validates admin capability). All three must succeed or the wizard surfaces the specific failing check.
4. **Persist.** MinIO access key, secret key, and (if present) custom CA cert are encrypted with the encryption key and stored in SQLite. Admin password hash is stored separately. Setup-complete flag is set.

Re-running the setup wizard after completion is not exposed in v1; a "Connection settings" page on the dashboard allows updating MinIO endpoint/credentials, and the local admin password can be changed from a profile page.

### 4.5 Authentication & sessions

- **Single local admin account** in v1. Username + password login form. Server rejects after 5 failed login attempts within 5 minutes from the same IP (in-memory rate limit; sliding window; reset on success).
- **Session cookies:** name configurable, `HttpOnly`, `Secure` (always set, even on HTTP — modern browsers tolerate this when accessed via `localhost`; documentation will note the limitation), `SameSite=Lax`, `Path=<HARBORMASTER_BASE_PATH>` (defaults to `/`; matches the configured base path so reverse-proxy subpath deployments work — see §4.2), opaque random session ID. Session record persisted in SQLite with creation time, last-active time, IP-at-issue. CSRF cookie has the same `Path` scope.
- **Session lifetime:** absolute timeout configurable via `HARBORMASTER_SESSION_TIMEOUT` (default 8h); sliding expiration is **not** implemented in v1 (keeps reasoning simple). Sessions can be explicitly revoked via logout.
- **CSRF protection:** double-submit cookie token. All non-`GET`/`HEAD`/`OPTIONS` requests must echo a CSRF token (from a cookie) in an `X-CSRF-Token` header.
- **No password reset flow in v1.** Operator must recover by editing/rotating the admin user via a CLI subcommand (`harbormaster admin reset-password --username <u>`).
- **Encryption-key loss recovery.** If the encryption key file is lost, the encrypted columns (MinIO credentials, custom CA cert) become permanently undecryptable. A CLI subcommand `harbormaster admin reset-encryption --confirm` provides a one-way recovery path: it backs up the current SQLite file with a `.pre-reset-<unix-ts>.bak` suffix, generates a fresh encryption key (writing it to `HARBORMASTER_ENCRYPTION_KEY_FILE` if set, else the default path), truncates the `minio_connections` table, clears the `setup_completed` flag, and exits. The next startup returns the operator to the first-run wizard with the admin account and audit history intact. The `--confirm` flag is mandatory; running without it prints the destructive-operation warning and exits non-zero.

### 4.6 Dashboard

Single-page overview displaying:

- MinIO server version, deployment mode (single-node / single-drive / single-node multi-drive / distributed), and uptime (from MinIO admin API)
- Total bucket count
- Estimated total storage usage (sum of bucket-level usage reported by admin info)
- Per-node health indicators: online/offline, drive count, drives healthy / unhealthy
- Warnings/errors surfaced from the admin API (e.g., disk full, decommissioning in progress)
- "Recent activity" feed: last 25 administrative actions performed **through Harbormaster** (bucket created, user disabled, etc.), sourced from the local SQLite audit table

The dashboard reads its data via a single `/api/v1/dashboard` aggregate endpoint that fans out to the MinIO admin SDK and the local DB; it must render in under 2 seconds (see §8).

### 4.7 Bucket management

- **List buckets:** name, creation timestamp, estimated size, object count (sourced from MinIO admin `BucketInfo`), versioning status, lifecycle-rule presence indicator, public-access mode, quota state. Sortable by name/created/size/count. Server-side paginated when more than 100 buckets exist.
- **Create bucket:** form fields = name (validated client-side and server-side per MinIO bucket naming rules), enable-versioning toggle, initial public-access mode (default `private`), optional lifecycle template selector (see §4.10), optional quota.
- **View bucket detail:** name, creation date, versioning status, public-access mode, quota state (current usage / limit if set), lifecycle rules summary, object count, total size, link to object browser, action buttons (toggle versioning, change public access, set/clear quota, edit/remove lifecycle rule, **empty bucket**, **delete bucket**).
- **Change public access:** dropdown of three modes — `private` (no anonymous access; default), `public-read` (anonymous `s3:GetObject` + `s3:ListBucket` allowed), `public-read-write` (additionally `s3:PutObject` and `s3:DeleteObject`; warning copy makes clear this is rarely what you want). Implemented via MinIO's `SetBucketPolicy` / `GetBucketPolicy` admin API using deterministic canned-policy JSON for each mode; switching to `private` removes the bucket policy entirely. Switching into any mode that allows anonymous writes requires a confirmation dialog with typed bucket-name confirmation; switching to read-only or back to private requires a single-click confirmation.
- **Quotas:** set / update / clear a per-bucket quota via MinIO admin `SetBucketQuota`. Form fields: quota kind (`hard` or `fifo`), size value with unit selector (MiB, GiB, TiB), or `Remove quota`. The bucket-detail page surfaces current usage vs limit when a quota is set, including a progress bar that turns amber at 80 % and red at 95 %.
- **Toggle versioning:** allowed when bucket is empty or already versioned; warning shown when enabling on a non-empty bucket.
- **Empty bucket (separate action from Delete):** runs an asynchronous "empty" operation that iteratively calls `RemoveObjects` in batches of 1000 (MinIO Go SDK bulk delete). Progress is reported to the UI via a server-sent-events stream (`POST /api/v1/buckets/{name}/empty` returns `text/event-stream` with `progress` events carrying `{deleted, estimated_total}` and a terminal `done` or `error` event). The bucket itself is **not** deleted by this action — only its contents. The UI shows a progress bar; the operator can close the tab and reopen the bucket-detail page to observe the ongoing operation (see §9 open question 14 for the operation-tracking trade-off). A single `bucket.empty` audit event is written on completion containing the final deleted count and elapsed duration. Empty must be confirmed via typed bucket-name entry before it starts.
- **Delete bucket:** modal confirmation requiring the user to type the bucket name. **Only permitted on an empty bucket.** If the bucket is non-empty, the modal disables the Delete button, shows the current object count, and links directly to the Empty-bucket flow. The backend re-checks emptiness immediately before deletion and returns `409 bucket_not_empty` if any objects remain. **No `force=true` shortcut exists in v1** — the empty-then-delete flow is mandatory.

### 4.8 Object browser

- **Folder-style navigation** for objects within a bucket, treating `/` in object names as path separators (standard S3 convention). Breadcrumb navigation.
- **Listing:** name, size, last-modified, MIME content type. Server-side paginated using S3 continuation tokens (default page size 100; user can change to 25, 50, 100, 250). The frontend uses **virtualized list rendering** (`@tanstack/react-virtual` or equivalent — final library chosen in design phase) so the UI remains responsive regardless of how many pages have been incrementally loaded. The SLO target is **10,000 objects per page-worth of prefix** server-side (see §8.1) — for prefixes that exceed a single page, the UI offers an "auto-load next page" affordance for progressive incremental browsing.
- **Upload object(s):** drag-and-drop and file-picker. Uses the MinIO Go SDK's `PutObject` server-side, which transparently switches to multipart. **Per-upload hard cap of `HARBORMASTER_UPLOAD_MAX_BYTES` (default 100 MiB).** Uploads larger than the cap are rejected with `413 upload_too_large`; the UI surfaces a message explaining the cap and recommending `mc` or another direct S3 client for larger files. A presigned-PUT "browser direct-to-MinIO" upload path is **not** in v1 (called out in non-goals); it is a documented follow-up to remove the cap. Single progress bar per upload; no resumable-upload UI.
- **Download object — two modes selected via `HARBORMASTER_DOWNLOAD_PROXY_MODE`:**
  - **Proxy (default, `proxy`).** `GET /api/v1/buckets/{name}/objects/download?key=...` streams the object body through Harbormaster to the browser with `Content-Disposition: attachment`. No URL is exposed; the request inherits the operator's session/CSRF context. Use this when MinIO is on a private network and Harbormaster is the only externally-reachable endpoint.
  - **Direct (`direct`).** The same endpoint returns `307` redirecting the browser to a short-lived presigned MinIO URL (default 5 minutes, capped at 1 hour). Only works when MinIO is reachable from the browser.
  - Both modes are wired into the same UI Download button; the choice is made server-side from configuration.
- **Share link (separate from in-UI download):** dedicated UX for generating a longer-lived shareable URL. `POST /api/v1/buckets/{name}/objects/share-links` accepts `{key, expires_seconds}`; the server clamps `expires_seconds` into `[30, HARBORMASTER_SHARE_LINK_MAX_TTL]` (default upper bound 7 days). Response returns the URL plus its `expires_at`. The UI displays both in a modal with copy-to-clipboard. **Share-link creation always writes an audit event** with the bucket, key, and TTL but **never the URL** (the URL contains the signature). Share links are not revocable from Harbormaster in v1 (S3 presigned URLs cannot be revoked without rotating MinIO credentials) — the modal copy makes this explicit.
- **Delete object:** confirmation required.
- **Preview supported files:** in-browser preview for `image/*`, `text/*` (with size cap, default 1 MiB), `application/pdf` (using browser-native PDF viewer), `application/json` (pretty-printed text). Anything else shows a "no preview available — download to view" placeholder.
- **Explicit non-goals:** advanced metadata editing, multipart-resume UI, object version browser, object diff, presigned-PUT direct-to-MinIO upload (deferred), share-link revocation, share-link analytics.

### 4.9 User & service-account management

#### Users (MinIO IAM users)

- **List users:** access key, status (enabled / disabled), attached policy templates.
- **Create user:** access key, secret key (auto-generated by Harbormaster using a CSPRNG to MinIO's accepted character set and length; **shown exactly once** in a modal with a copy-to-clipboard control and a "Reveal" toggle that defaults hidden), policy template selection.
- **Disable user:** confirmation modal.
- **Enable user.**
- **Delete user:** confirmation modal requiring the user to retype the access key.
- **Edit attached policies:** allow swapping which template policies are attached to a user.

#### Service accounts / access keys

In MinIO, "service accounts" are credentials that inherit a parent user's policies. Harbormaster exposes them as a list nested under each user.

- **List service accounts** for a given parent user.
- **Create service account:** generates new access/secret keys (secret shown exactly once, same once-only modal as user creation), optional name/description, optional policy override (limited to template policies in v1).
- **Revoke service account:** confirmation modal.

#### Security

- **Secret keys are NEVER returned by `GET` endpoints after creation.** The create response is the only time the secret is sent; subsequent operations only expose the access key.
- **All credential operations are logged to the local audit table** (access key, action, timestamp, source IP, actor = "local-admin"). Secrets are never logged.

### 4.10 Policy management (template-driven)

Harbormaster ships with three bundled policy templates:

| Template name   | Description                                                                  |
| --------------- | ---------------------------------------------------------------------------- |
| `read-only`     | List/read all buckets and objects                                            |
| `read-write`    | List/read/write objects in all buckets, no admin operations                  |
| `backup-target` | Read/write/delete objects in a specified bucket; cannot list other buckets    |

Template definitions are stored in Go source (embedded as JSON literals), not in the database, so they cannot be modified at runtime.

- `read-only` and `read-write` apply unmodified.
- `backup-target` is parameterized by bucket name; the create-credential UI prompts for which bucket to scope it to when the user picks this template.

An `administrator` template (equivalent to MinIO's built-in `consoleAdmin`) is intentionally **not** bundled in v1. Anyone with full MinIO admin already has the Harbormaster admin's MinIO credentials; issuing additional `consoleAdmin` accounts is rare in a single-admin homelab and is the most dangerous footgun in the bundled-template list. Operators who genuinely need this can attach MinIO's built-in `consoleAdmin` policy directly via `mc admin policy attach` outside Harbormaster; the user-detail page surfaces externally-attached MinIO policies in a read-only "Other attached policies" row so they remain visible.

When attaching a template to a MinIO user, Harbormaster ensures a MinIO policy with a deterministic name (`harbormaster-<template>-<bucket?>`) exists (creating it via the admin API if missing) and attaches it.

**Explicit non-goal:** freeform IAM JSON policy editor.

### 4.11 Lifecycle rules

Per-bucket simplified rule builder. v1 supports one operation only:

- **Expire objects after N days.** Form: integer days (1–10000), optional object prefix filter.

Tag-based filtering is **not** exposed in the create form in v1 — supporting it would require a bucket/object tag-management UI that is itself out of scope. Rules created outside Harbormaster that use tag filters are surfaced as unmanaged read-only entries (see the unmanaged-rule handling below).

UI shows a human-readable summary of every existing rule for the bucket and allows deleting an existing rule. Editing an existing rule is implemented as "delete + create" in v1.

Rules are written to MinIO via the standard bucket-lifecycle API. Reading them back is best-effort: any rule Harbormaster doesn't recognize (e.g., transitions, abort-incomplete-multipart) is displayed as a read-only "Unmanaged rule (created outside Harbormaster) — N actions" entry that can only be deleted, not edited.

### 4.12 Local activity feed / audit

Every state-changing operation performed through Harbormaster writes an entry to a local `audit_events` SQLite table:

- `id` (ULID), `occurred_at` (UTC), `actor` (`local-admin` in v1), `source_ip`, `action` (enum string — full inventory in `data-model.md`), `target_type` (`bucket` / `object` / `user` / `service_account` / `policy_attachment` / `lifecycle_rule` / `session` / `connection_settings` / `admin_security`), `target_id` (string), `outcome` (`success` / `failure`), `error_message` (nullable, truncated to 1 KB), `payload_summary` (small JSON blob with non-sensitive context — never contains secrets, presigned URLs, share-link URLs, or password material).

The dashboard shows the last 25 entries. A dedicated `/activity` page shows a paginated full list with filters by `action`, `target_type`, and date range. v1 does **not** expose log export.

A configurable retention policy purges entries older than `HARBORMASTER_AUDIT_RETENTION` (default 90 days) once per day via an in-process scheduler.

## 5. API Surface

### 5.1 Conventions

- All application APIs live under `/api/v1/`.
- Health endpoints (`/healthz`, `/readyz`) live at the root and require no auth.
- The CSRF token endpoint `/api/v1/csrf` requires only a valid session.
- Optional Prometheus metrics endpoint `/metrics` is served on a **separate listener** (default `:9090`) and binds only when `HARBORMASTER_METRICS_ENABLED=true`.
- **Transport format:** JSON:API where it fits naturally (resource collections — buckets, users, service accounts, policy attachments, lifecycle rules, audit events). Action endpoints (login, logout, connection-settings test, object proxy-download, share-link minting, bucket empty SSE stream, bucket public-access set, bucket quota set) use plain JSON (or `text/event-stream` for SSE) request/response bodies as documented per endpoint, because they do not map cleanly to a resource representation.
- **Error responses** follow JSON:API's `errors` array shape (`{"errors":[{"status":"422","title":"...","detail":"...","source":{"pointer":"/data/attributes/name"}}]}`) for resource endpoints, and `{"error":{"code":"...","message":"..."}}` for action endpoints.
- All write endpoints require the `X-CSRF-Token` header to match the value held in the CSRF cookie.

### 5.2 Endpoint inventory (v1)

**Setup & auth**

- `GET  /api/v1/setup/status` — `{"initialized": bool}`. Unauthenticated.
- `GET  /api/v1/setup/mc-aliases` — unauthenticated; available **only while `initialized=false`**. Returns `{"aliases": [{"name": "myminio", "endpoint": "...", "access_key": "AKIA...", "tls_skip_verify": false}, ...]}` parsed from the file at `HARBORMASTER_MC_CONFIG_PATH`. Returns `{"aliases": []}` when the file is absent or unreadable. **Never returns secret keys** — secrets remain in the file until the operator submits the setup form referencing an alias by name.
- `POST /api/v1/setup` — first-run only. Body may either specify explicit MinIO credentials or `{"minio": {"from_mc_alias": "myminio"}}`; in the alias form, the server re-reads the mc config to fetch the secret for the named alias. Returns 409 if already initialized.
- `POST /api/v1/auth/login` — body: `{username, password}`. Sets session + CSRF cookies on success.
- `POST /api/v1/auth/logout` — invalidates current session.
- `GET  /api/v1/auth/me` — current session info (no password fields).
- `POST /api/v1/auth/password` — change current admin password. Body: `{current_password, new_password}`.
- `GET  /api/v1/csrf` — returns current CSRF token in body and refreshes cookie if missing.

**Connection settings**

- `GET  /api/v1/connection` — current MinIO endpoint (host, TLS flag, skip-verify flag), access key prefix only (last 4 chars masked), redacted secret indicator.
- `PUT  /api/v1/connection` — update; validates connectivity before persisting.
- `POST /api/v1/connection/test` — validate-only, does not persist.

**Dashboard**

- `GET  /api/v1/dashboard` — aggregate response: server info, totals, node health, warnings, recent activity (last 25).

**Buckets**

- `GET    /api/v1/buckets` — JSON:API resource collection. Query params: `page[number]`, `page[size]`, `sort`.
- `POST   /api/v1/buckets` — create. JSON:API request document.
- `GET    /api/v1/buckets/{name}` — detail.
- `DELETE /api/v1/buckets/{name}` — body: `{"confirm_name": "<bucket-name>"}`. **Only succeeds on an empty bucket;** returns `409 bucket_not_empty` otherwise. No force flag in v1.
- `PUT    /api/v1/buckets/{name}/versioning` — body: `{"enabled": bool}`.
- `PUT    /api/v1/buckets/{name}/public-access` — body: `{"mode": "private" | "public-read" | "public-read-write", "confirm_name"?: "<bucket-name>"}`. `confirm_name` is required when transitioning into a write-allowing mode.
- `PUT    /api/v1/buckets/{name}/quota` — body: `{"kind": "hard" | "fifo", "bytes": <int>}` or `{"kind": "none"}` to clear.
- `POST   /api/v1/buckets/{name}/empty` — initiates the asynchronous empty-bucket operation. Body: `{"confirm_name": "<bucket-name>"}`. Response is `text/event-stream`; emits `progress` events `{deleted, estimated_total}` (every batch) and terminates with a `done` event `{deleted_total, duration_ms}` or `error` event `{message}`. Re-issuing while an operation is in progress for the same bucket attaches to the existing job (does not start a duplicate).

**Objects**

- `GET    /api/v1/buckets/{name}/objects?prefix=...&delimiter=/&page[size]=...&page[token]=...` — paginated listing; opaque `page[token]` is the S3 continuation token.
- `POST   /api/v1/buckets/{name}/objects` — upload (multipart/form-data); accepts file plus target object key. Returns the resulting object's metadata. Rejects bodies larger than `HARBORMASTER_UPLOAD_MAX_BYTES` with `413 upload_too_large`.
- `DELETE /api/v1/buckets/{name}/objects?key=<urlencoded>` — delete a single object.
- `GET    /api/v1/buckets/{name}/objects/download?key=<urlencoded>` — **proxy download** (default mode). Streams the object body through Harbormaster with `Content-Disposition: attachment`. When `HARBORMASTER_DOWNLOAD_PROXY_MODE=direct`, returns `307` redirecting to a short-lived presigned MinIO URL (default 5 minutes, capped at 1 hour) instead. Same UI button regardless of mode.
- `POST   /api/v1/buckets/{name}/objects/share-links` — body: `{"key": "...", "expires_seconds": <int>}`. Returns `{"url": "...", "expires_at": "..."}`. `expires_seconds` is clamped server-side to `[30, HARBORMASTER_SHARE_LINK_MAX_TTL]`. Always writes an audit event (bucket, key, TTL — never the URL).

**Users**

- `GET    /api/v1/users` — list.
- `POST   /api/v1/users` — create. Response includes the **one-time** secret key. Subsequent `GET` calls never return it.
- `GET    /api/v1/users/{access_key}` — detail.
- `PUT    /api/v1/users/{access_key}/status` — body: `{"enabled": bool}`.
- `DELETE /api/v1/users/{access_key}` — body: `{"confirm_access_key": "<access_key>"}`.
- `PUT    /api/v1/users/{access_key}/policies` — body: `{"templates": [{"name":"backup-target","params":{"bucket":"foo"}}, ...]}`.

**Service accounts**

- `GET    /api/v1/users/{access_key}/service-accounts` — list.
- `POST   /api/v1/users/{access_key}/service-accounts` — create (one-time secret in response).
- `DELETE /api/v1/service-accounts/{access_key}` — revoke.

**Policy templates**

- `GET    /api/v1/policy-templates` — list of bundled templates with their parameter schemas.

**Lifecycle rules**

- `GET    /api/v1/buckets/{name}/lifecycle-rules` — list (includes recognized + unmanaged).
- `POST   /api/v1/buckets/{name}/lifecycle-rules` — create simple-expiration rule.
- `DELETE /api/v1/buckets/{name}/lifecycle-rules/{rule_id}` — delete.

**Audit / activity**

- `GET    /api/v1/audit-events?filter[action]=...&filter[target_type]=...&filter[from]=...&filter[to]=...&page[number]=...&page[size]=...` — paginated.

**Error model examples** are documented in `api-contracts.md` (companion file).

## 6. Data Model

### 6.1 SQLite schema (v1)

All tables use `INTEGER PRIMARY KEY` autoincrement IDs unless noted. ULIDs are stored as TEXT. Encrypted columns store base64-encoded AES-256-GCM ciphertext with prepended random nonce.

- **`admin_users`** — `id`, `username` (UNIQUE), `password_hash` (argon2id), `created_at`, `updated_at`, `disabled_at` (nullable; v1 only ever has 1 row).
- **`sessions`** — `id` (ULID, PK), `admin_user_id` (FK), `created_at`, `expires_at`, `last_active_at`, `source_ip`, `user_agent`.
- **`minio_connections`** — `id` (singleton row enforced via partial unique index on a dummy column), `endpoint_url`, `tls_skip_verify` (bool), `access_key_ciphertext`, `secret_key_ciphertext`, `custom_ca_pem_ciphertext` (nullable), `created_at`, `updated_at`.
- **`app_settings`** — `key` (PK), `value`, `updated_at`. Used for `setup_completed`, `encryption_key_fingerprint`, schema-version pin, etc.
- **`audit_events`** — `id` (ULID, PK), `occurred_at`, `actor`, `source_ip`, `action`, `target_type`, `target_id`, `outcome`, `error_message`, `payload_summary_json`. Indexed on `(occurred_at)`, `(target_type, target_id)`, `(action, occurred_at)`.
- **`bucket_empty_jobs`** *(conditional, design-phase decision per §9 question 14)* — `id` (ULID, PK), `bucket_name`, `started_at`, `last_progress_at`, `deleted_count`, `estimated_total`, `state` (`running` | `done` | `error`), `error_message`, `finished_at`. Partial unique index ensures at most one `running` job per bucket. Omitted entirely if the in-flight-only model is chosen.
- **`schema_migrations`** — managed by the migration runner (chosen library decided in design phase; candidates: `golang-migrate`, `goose`, or a hand-rolled lightweight one).

### 6.2 Encryption-at-rest

- Symmetric encryption key (32 bytes) loaded from `HARBORMASTER_ENCRYPTION_KEY_FILE` if set, else read from `<data_dir>/encryption.key`. If neither exists on first boot, the app generates one, writes it with `0600` perms, and logs that it has done so.
- AES-256-GCM with per-ciphertext random 12-byte nonce. Ciphertext encoding: `base64( nonce || ciphertext || tag )`.
- Plain credentials live only in process memory while bound to an active MinIO SDK client; they are never written to disk in plaintext, never logged, and never serialized into audit `payload_summary` blobs.
- Encryption-key rotation is **not** implemented in v1; the migration plan documents it as a follow-up.

### 6.3 Outside SQLite

- Policy template definitions live in Go source.
- Per-template "rendered" MinIO policy JSON is created on the MinIO side via the admin API; Harbormaster does not cache rendered policies locally.

## 7. Service Impact

This is the first task in an empty repository, so v1 **creates** rather than modifies services. Two services / modules ship in this task:

### 7.1 Backend (`apps/backend`)

A single Go binary (working module path: `github.com/jtumidanski/harbormaster` — confirm against final repo URL during design phase). Layout (provisional; finalized in design.md):

```
apps/backend/
  cmd/harbormaster/         main package (CLI subcommands: serve, admin reset-password, admin reset-encryption, version)
  internal/api/             HTTP handlers, JSON:API encoders, CSRF middleware, auth middleware
  internal/auth/            password hashing, session lifecycle, login rate limit
  internal/audit/           audit event writer + retention sweeper
  internal/buckets/         bucket-domain operations (wraps MinIO admin/SDK clients)
  internal/config/          configuration loading + validation
  internal/crypto/          AES-256-GCM helpers, encryption key loader
  internal/db/              SQLite open, migration runner, schema migrations dir
  internal/minio/           MinIO admin + S3 client construction
  internal/objects/         object-listing/upload/download domain operations
  internal/policies/        policy template definitions + renderer
  internal/server/          HTTP server bootstrap, embed.FS for SPA, graceful shutdown
  internal/users/           IAM user + service-account domain operations
  internal/observability/   structured logging setup, optional Prometheus, optional OTLP
  internal/lifecycle/       lifecycle-rule domain operations
  migrations/               *.sql files
```

Backend follows the conventions in `.claude/skills/backend-dev-guidelines/` (DDD, immutable models, functional composition, GORM entities, JSON:API transport). Notable per-skill conformance items to verify during plan/execute:

- Domain types are immutable value types; mutations return new values.
- Persistence happens through GORM-backed repositories returning domain types.
- HTTP transport uses JSON:API serialization for resource endpoints (see §5.1).
- One package per bounded context (buckets, users, etc.).
- Tests use the standard library `testing` package plus a SQLite in-memory database for repository tests; MinIO interaction tests run against a containerized MinIO via `testcontainers-go` (decision to confirm in design phase — could also be a manual integration test suite gated by env var).

### 7.2 Frontend (`apps/frontend`)

A React + TypeScript + Vite SPA built per `.claude/skills/frontend-dev-guidelines/` (shadcn/ui, TanStack React Query, react-hook-form + Zod, Tailwind, React Router). Layout (provisional):

```
apps/frontend/
  index.html
  package.json
  tsconfig.json
  vite.config.ts
  tailwind.config.ts
  src/
    main.tsx                app bootstrap, QueryClient + Router setup
    App.tsx                 route tree
    routes/                 page-level components, one per route
    components/             reusable UI components (built on shadcn/ui primitives)
    components/ui/          shadcn-generated primitives
    features/               feature folders: setup, auth, dashboard, buckets, objects, users, service-accounts, policies, lifecycle, activity
      <feature>/components/
      <feature>/hooks/      React Query hooks, react-hook-form schemas
      <feature>/api.ts      typed fetch wrappers
      <feature>/types.ts    shared TS types for the feature
    lib/                    api client (fetch wrapper with CSRF header injection), error helpers, formatters
    styles/                 global Tailwind layer additions, theme tokens
```

Production build outputs to `apps/frontend/dist/`. The Docker build pipeline copies that directory into a Go module path that is embedded via `//go:embed dist/*` (or equivalent), and the Go SPA-serving handler serves it with `index.html` fallback for client-side routing.

### 7.3 Deployment (`deploy/`)

- `deploy/docker/Dockerfile` — multi-stage build (Node stage builds SPA, Go stage builds binary with embedded assets, final stage = minimal distroless or `gcr.io/distroless/static-debian12:nonroot` image).
- `deploy/docker/docker-compose.yml` — example deployment with a named volume for `/var/lib/harbormaster`, environment placeholders, optional `minio` service for local testing.
- `deploy/docker/.env.example` — documented env vars.
- `deploy/kubernetes/deployment.yaml`, `service.yaml`, `ingress.example.yaml`, `secret.example.yaml`, `pvc.yaml` — raw manifests. No Helm chart in v1.

### 7.4 CI/CD (`.github/workflows/`)

Three workflows:

- **`pr.yml`** — triggered on `pull_request`. Jobs (each pinned to specific runner OS versions and action SHAs):
  - `frontend-lint` (eslint + prettier --check)
  - `frontend-test` (vitest)
  - `frontend-build` (vite build)
  - `backend-lint` (golangci-lint, configured via `.golangci.yml`)
  - `backend-test` (`go test -race ./...`)
  - `backend-build` (`go build ./...`)
  - `gitleaks` (secret scanning)
  - `dependency-scan` (trivy fs scan for vulnerable dependencies)
- **`main.yml`** — triggered on push to `main` and on semver tag push. Builds the frontend, builds the Go binary, builds and pushes a multi-arch container image (`linux/amd64`, `linux/arm64`) to `ghcr.io/jtumidanski/harbormaster` with tags: `latest` (main only), `sha-<shortsha>` (always), `v<semver>` (tags only). Container scan (trivy image) runs **before** push; build fails on critical/high CVEs (configurable allowlist).
- **`release.yml`** — triggered on GitHub Release publish. Generates changelog (e.g., from `git-cliff` or hand-curated `CHANGELOG.md`), attaches release notes, references the published container image tags.

Workflow security baseline:

- All third-party actions pinned to commit SHAs.
- Default `GITHUB_TOKEN` permissions set to `contents: read` per workflow, with elevated permissions narrowed to specific jobs that need them (e.g., `packages: write` only on the publish job).
- No `pull_request_target` triggers.
- Reusable workflow patterns where it reduces duplication, kept inside `.github/workflows/` (no external reusable workflows in v1).

### 7.5 Renovate (`.github/renovate.json5` or `renovate.json5`)

- Ecosystem groups: frontend npm, Go modules, GitHub Actions, Docker base images, dev tooling.
- Minimum release age: `patch` 3 days, `minor` 7 days, `major` manual review.
- Auto-merge: `patch` updates of dev-tooling and GitHub Actions only — **never** auth/security/runtime libraries, **never** major updates.
- Dependency Dashboard issue enabled.
- Vulnerability alerts enabled with elevated priority labels.

## 8. Non-Functional Requirements

### 8.1 Performance

- Dashboard `GET /api/v1/dashboard` p95 < **2 s** against a MinIO instance with 100 buckets.
- Bucket listing `GET /api/v1/buckets` p95 < **2 s** against 100 buckets.
- Object listing `GET /api/v1/buckets/{name}/objects?prefix=...` p95 < **3 s** for a single page of up to **10,000 objects** under the given prefix (server-side). Deep prefixes are browsed via repeated paginated calls; the UI's virtualization keeps DOM render time bounded regardless of total loaded volume.
- SPA initial render to dashboard-interactive < **3 s** on a typical homelab LAN.
- SPA bundle gzipped size < **500 KiB** initial route (excluding lazily-loaded routes).

### 8.2 Reliability

- Fail-fast startup as documented in §4.2.
- Graceful shutdown: 10 s drain on SIGTERM; in-flight uploads/downloads attempt to finish; new requests rejected with `503`.
- All database writes wrapped in retries for `SQLITE_BUSY` with exponential backoff capped at 1 second (SQLite WAL mode enabled).
- The app must survive an unreachable MinIO endpoint: read pages render with a clearly-labeled error state; write actions return a structured error. `/readyz` reports the unreachability.

### 8.3 Security

- All credentials encrypted at rest (§6.2). Encryption key file mandatory `0600` permission check at startup (warn loudly if relaxed; fail if widely readable).
- Sessions: HttpOnly + Secure + SameSite=Lax + opaque random IDs (256 bits of entropy).
- CSRF: double-submit cookie on all unsafe methods.
- Destructive operations: typed confirmation (bucket name, access key) on UI; backend re-validates.
- Logging: never log secret keys, session cookie values, CSRF tokens, encryption key, MinIO secret key, or any field marked sensitive in the data model. Lint/test for this via a small "sensitive-strings" allow-list test in CI.
- Default deny on auth-required endpoints; `setup` and `auth` endpoints have an explicit allow-list of public paths.
- Brute-force resistance on login (§4.5).
- Supply chain: gitleaks on PR, trivy on PR + on image, Renovate with minimum release age (§7.5), pinned GitHub Actions (§7.4), branch protection on `main` (required checks = all PR-workflow jobs) — branch protection is configured manually on GitHub, but the task documents the required setting.
- Signed container images: **preferred** but not blocking for v1. The task may include cosign keyless signing if it lands without significant effort; otherwise it's documented as a follow-up.

### 8.4 Observability

- **Logs:** zerolog or zap (chosen in design phase) emitting newline-delimited JSON with `level`, `timestamp`, `caller`, `request_id`, plus structured fields per event. Request ID propagated via `X-Request-Id` (echoed back; generated if absent).
- **Metrics:** optional Prometheus exposition on a separate listener (`HARBORMASTER_METRICS_ENABLED`). Built-in metrics: HTTP request count/latency histograms by route and status, MinIO admin call count/latency by operation, current session count, audit event write count, sqlite write retry count.
- **Tracing:** optional OpenTelemetry trace export via OTLP when `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` is set. Spans for inbound HTTP requests, MinIO SDK calls, database operations.

### 8.5 Upgradeability

- The container image is stateless. Persistent state lives in the mounted data dir (default `/var/lib/harbormaster`) containing the SQLite file and the encryption key.
- Schema migrations run automatically at startup; failure aborts startup. Migrations are forward-only; downgrade is **not** supported in v1 and the docs say so.
- Config changes only require a process restart, not data migration.

### 8.6 Accessibility & UX (light requirements)

- Dark mode supported (Tailwind `dark:` classes; theme toggle persisted in `localStorage`; defaults to system preference).
- Keyboard-navigable primary nav and primary actions; visible focus rings.
- shadcn/ui components inherit reasonable a11y defaults; we don't audit beyond that in v1.
- The object browser uses virtualized list rendering (`@tanstack/react-virtual` or equivalent) for any view that may exceed ~500 rows, so DOM size remains bounded regardless of the listed object count.

## 9. Open Questions

These remain for the design phase to resolve:

1. **Web framework / router:** stdlib `net/http` + `chi`, or Echo, or Fiber? Recommend `chi` for stdlib affinity, but design phase to confirm.
2. **JSON:API library:** hand-rolled encoder/decoder, `google/jsonapi`, or `manyminds/api2go`? Skill resources reference patterns that may already prefer one.
3. **GORM vs `sqlc` / hand-written SQL** for SQLite access: backend skill says GORM, design phase confirms it fits SQLite migration story.
4. **Migration runner:** `golang-migrate`, `goose`, or hand-rolled with embedded `*.sql` files.
5. **Argon2id vs bcrypt** for admin password hashing — argon2id is preferred for new systems; design phase to commit.
6. **Session store backend:** SQLite (documented above) or in-memory with periodic snapshot? — going with SQLite for survivability across restarts.
7. **Test strategy for MinIO interactions:** `testcontainers-go` integration suite vs a hand-coded `mc`-driven integration suite vs both. Affects CI runtime budget.
8. **Logging library:** zerolog vs zap.
9. **Distroless variant:** `gcr.io/distroless/static-debian12:nonroot` requires fully static Go binary (means `CGO_ENABLED=0`, which conflicts with `mattn/go-sqlite3`). Either switch to `modernc.org/sqlite` (pure Go) or use `gcr.io/distroless/base-debian12:nonroot`. Design phase decides.
10. **cosign signing:** include in v1 main-branch workflow or defer to a follow-up task.
11. **Repo URL casing:** confirm whether the GitHub repo is `jtumidanski/Harbormaster` (capital) or `jtumidanski/harbormaster` (lower). User confirmed capital `Harbormaster` for the repo; GHCR image path is `ghcr.io/jtumidanski/harbormaster` (GHCR is case-insensitive but conventionally lowercase).
12. **License:** not specified by the operator yet — design phase or a separate task to pick (e.g., Apache-2.0, MIT, AGPL-3.0). v1 should not ship without a `LICENSE` file.
13. **SSE implementation:** stdlib `http.Flusher` + handwritten SSE encoding vs a library (`r3labs/sse`). Affects the empty-bucket endpoint and any future streaming endpoints.
14. **Empty-bucket operation tracking:** purely in-flight (the operation runs for as long as the request is open; reconnecting attaches to the in-process operation if still running, otherwise reports terminal state and exits) vs a `bucket_empty_jobs` SQLite table the worker drains in the background with the SSE endpoint subscribing to its progress. The latter is more robust against process restarts mid-operation but is meaningfully more infrastructure for a v1 MVP. Design phase to commit.
15. **Tag-filter readout for unmanaged lifecycle rules:** confirm the read-only display summarizes tag-scoped rules without exposing tag values that might be operator-sensitive (lean: show tag count, not tag values).
16. **mc config path inside container vs `--mount`-only:** decide whether to ship a default volume-mount in the example `docker-compose.yml` for the host's `~/.mc/config.json`, or leave it to the operator to add. Convenience vs unexpected file access on first boot.

## 10. Acceptance Criteria

A reviewer can mark v1 done when **all** of the following are demonstrably true. Each item is independently verifiable.

### Foundation

- [ ] Repo layout matches §4.1 (`/apps`, `/deploy`, `/scripts`, `/.github/workflows`, `/docs` all present and populated).
- [ ] `LICENSE` file present at repo root.
- [ ] Go module initialized at `apps/backend` with module path agreed in design phase.
- [ ] Frontend Vite/React/TS project initialized at `apps/frontend` with Tailwind, shadcn/ui, TanStack React Query, react-hook-form + Zod, React Router installed and configured.
- [ ] `.golangci.yml`, `eslint.config.*`, `.prettierrc`, `.editorconfig` all present.

### Application functionality

- [ ] First-run setup wizard creates the local admin and persists encrypted MinIO credentials; replaying setup returns 409.
- [ ] Login/logout works; session cookie is HttpOnly + Secure + SameSite=Lax; CSRF tokens are required on writes.
- [ ] Failed login attempts beyond the rate limit return 429.
- [ ] Admin password change works; old password validated server-side.
- [ ] Dashboard renders server version, totals, node health, warnings, and recent local activity within the 2 s p95 SLO under a 100-bucket fixture.
- [ ] Bucket list/create/versioning toggle all work end-to-end against a live MinIO. Bucket delete is gated on emptiness; non-empty deletes return `409 bucket_not_empty` and the UI links the operator into the Empty-bucket flow.
- [ ] Empty-bucket SSE stream successfully drains a 10,000-object bucket and reports per-batch progress to the UI; closing/reopening the tab attaches to the existing job; a single `bucket.empty` audit event is written with the final deleted count and duration.
- [ ] Bucket public-access mode can be set to `private` / `public-read` / `public-read-write` and the change is observable via `mc anonymous get`. Transitions to a write-allowing mode require typed bucket-name confirmation.
- [ ] Bucket quota can be set (hard or FIFO), updated, and cleared; the bucket-detail page shows current usage vs limit when set, with amber/red thresholds.
- [ ] Object browser navigates folder-style listings with virtualized rendering, uploads (drag/drop and picker, rejected with 413 above the cap), downloads via the configured mode (proxy default, direct optional), deletes (with confirm), and previews supported MIME types.
- [ ] Object listings remain responsive at 10,000 objects per page-worth of prefix; virtualized rendering keeps the DOM bounded as additional pages are auto-loaded.
- [ ] Share-link creation returns a URL with a server-clamped `expires_at`; the audit event records bucket + key + TTL and never the URL.
- [ ] Users list/create/disable/enable/delete and policy attachments work; secret key shown exactly once on create. Only `read-only`, `read-write`, and `backup-target` templates are bundled; externally-attached MinIO policies (including `consoleAdmin`) are surfaced read-only under "Other attached policies" on the user-detail page.
- [ ] Service accounts list/create/revoke work; secret shown exactly once.
- [ ] Lifecycle rule create (simple expiration with optional prefix filter — **no tag filter in v1**), list, and delete work; rules created outside Harbormaster (including tag-scoped rules) are surfaced as unmanaged read-only entries.
- [ ] Local activity feed records every state-changing action through Harbormaster; the `/activity` page filters and paginates entries; retention sweep deletes entries older than the configured age.
- [ ] `harbormaster admin reset-password` CLI subcommand works against the SQLite store.
- [ ] `harbormaster admin reset-encryption --confirm` backs up the SQLite file with a `.pre-reset-<unix-ts>.bak` suffix, generates a new encryption key, truncates `minio_connections`, clears `setup_completed`, and exits. Next startup returns the operator to the first-run wizard with the admin account and audit history intact. Running without `--confirm` prints a warning and exits non-zero.
- [ ] Setup wizard surfaces detected `mc` aliases when `HARBORMASTER_MC_CONFIG_PATH` is mounted and `initialized=false`; selecting an alias pre-fills endpoint and credentials; the operator can still edit any field before submission. The mc-aliases endpoint never returns secret keys.
- [ ] Subpath deployments work: with `HARBORMASTER_BASE_PATH=/harbormaster`, the SPA loads at `https://host/harbormaster/`, session and CSRF cookies are scoped to `/harbormaster`, and no asset URLs escape the prefix.

### Security & supply chain

- [ ] All credentials in `minio_connections` are encrypted at rest with AES-256-GCM; ciphertext inspection in the DB shows non-plaintext blobs.
- [ ] Encryption key file enforces `0600`; relaxed perms emit a startup warning, dangerously-open perms (world-readable) abort startup.
- [ ] No secret values appear in logs, metrics labels, traces, or audit `payload_summary` (covered by an automated test).
- [ ] All four PR-workflow jobs (frontend lint/test/build, backend lint/test/build, gitleaks, dependency-scan) run on every PR.
- [ ] All GitHub Actions pinned to commit SHAs.
- [ ] Renovate config present with minimum release ages, ecosystem groupings, and auto-merge constraints documented in §7.5.

### Deployment & operations

- [ ] `docker compose up` from `deploy/docker/` produces a running Harbormaster + optional MinIO sidecar and the SPA loads at `http://localhost:8080`.
- [ ] Kubernetes manifests in `deploy/kubernetes/` apply cleanly to a kind/k3s cluster and the app becomes ready.
- [ ] Multi-arch container image is produced for `linux/amd64` and `linux/arm64`.
- [ ] `/healthz` returns 200 once the listener is up; `/readyz` returns 200 only when migrations and (if configured) MinIO connectivity succeed.
- [ ] Structured JSON logs by default; `console` format works when set.
- [ ] Prometheus metrics endpoint serves when enabled and is absent (port not bound) when disabled.

### CI/CD

- [ ] PR workflow blocks merge on any failing required check.
- [ ] Main-branch workflow builds and publishes multi-arch container images to `ghcr.io/jtumidanski/harbormaster` tagged `latest` + `sha-<shortsha>`.
- [ ] Release workflow on tag push produces a GitHub release with notes, semver-tagged container image, and links to the image.
- [ ] Trivy image scan runs before publish; failure on critical/high vulns aborts publish (allowlist documented).

### Documentation

- [ ] `README.md` updated with: feature list, quick-start (Docker Compose), production deployment notes, configuration reference, security model summary, project goals + non-goals, link to this PRD.
- [ ] `docs/` contains an architecture overview matching what was actually built, a configuration reference, and an operator security guide.

---

**Companion files** (in this folder):

- `api-contracts.md` — request/response examples for the endpoints in §5.2.
- `data-model.md` — full SQLite DDL with column types, indices, and example rows.
- `risks.md` — risks called out for the design phase to address.
