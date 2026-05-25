# Harbormaster

> Self-hosted MinIO admin UI for homelab and small-cluster operators.

Harbormaster is a single-binary administrative web app for a **single**
MinIO deployment. It serves a REST/JSON:API backend and an embedded
React/TypeScript SPA from one Go process, stores its state in an
embedded SQLite database, and ships as a multi-arch
(`linux/amd64`, `linux/arm64`) container image on GHCR. The goal is to
feel operationally similar to Portainer or Grafana — a focused console
for bucket / object / user / lifecycle workflows, not an enterprise
platform.

## Table of contents

1. [Quick start (Docker Compose)](#quick-start-docker-compose)
2. [Production deployment](#production-deployment)
3. [Configuration reference](#configuration-reference)
4. [Security model](#security-model)
5. [Importing from `mc` config](#importing-from-mc-config)
6. [Recovery](#recovery)
7. [First-publish reminder (GHCR visibility)](#first-publish-reminder-ghcr-visibility)
8. [Project goals + non-goals](#project-goals--non-goals)
9. [License](#license)
10. [Links](#links)

## Quick start (Docker Compose)

The `deploy/docker/docker-compose.yml` ships a working setup. A bundled
MinIO is available via the optional `with-minio` profile for local
testing only.

```bash
# 1. Bring the stack up (Harbormaster + a local MinIO).
docker compose -f deploy/docker/docker-compose.yml --profile with-minio up -d

# 2. Open the UI and walk the first-run setup wizard.
#    Create a local admin account, point at MinIO (use http://minio:9000
#    inside the compose network), and the wizard validates connectivity.
open http://localhost:8080

# 3. Tail logs while you click around. Harbormaster emits JSON logs by
#    default; HARBORMASTER_LOG_FORMAT=console makes them readable.
docker compose -f deploy/docker/docker-compose.yml logs -f harbormaster
```

For production: point at your own MinIO cluster (drop the
`--profile with-minio` flag and the `minio:` service from the compose
file), and override sensitive defaults via env vars.

## Production deployment

### Docker

The image is `ghcr.io/jtumidanski/harbormaster:<tag>` (multi-arch). Pin
to a `vX.Y.Z` tag for production; `latest` follows `main` and is fine
for development. Volume-mount a directory for `/var/lib/harbormaster`
(holds the SQLite DB, the encryption key file, and WAL/journal files)
and put a reverse proxy (TLS-terminating) in front. The
`deploy/docker/docker-compose.yml` is heavily commented and is the
canonical template.

### Kubernetes

Raw manifests live under `deploy/kubernetes/` (Deployment, Service,
PVC, example Ingress and Secret). See `deploy/kubernetes/README.md` for
the full walkthrough — including why the Deployment pins
`replicas: 1` + `strategy: Recreate` (login rate limiter, empty-bucket
worker, and audit sweeper are all in-process singletons; the PVC is
`ReadWriteOnce`).

### Reverse proxy

TLS terminates at the proxy, not at Harbormaster. Two endpoints care
about proxy behaviour:

- **SSE**: `POST /api/v1/buckets/{bucket}/empty` streams progress events.
  Disable response buffering (`proxy_buffering off` for nginx) and raise
  the read timeout to ~1h for million-object buckets.
- **Uploads**: respect `HARBORMASTER_UPLOAD_MAX_BYTES` at the proxy too,
  or the proxy will 413 before Harbormaster sees the request.

Copy-pastable nginx, Caddy, and Traefik configs are in
[`docs/operator/reverse-proxy.md`](docs/operator/reverse-proxy.md).

## Configuration reference

Sources, highest priority first:

1. Environment variables (prefix `HARBORMASTER_`)
2. Config file (path via `HARBORMASTER_CONFIG`; YAML/TOML/JSON via Viper)
3. Built-in defaults

| Variable                                  | Default                          | Meaning                                                                                       |
| ----------------------------------------- | -------------------------------- | --------------------------------------------------------------------------------------------- |
| `HARBORMASTER_LISTEN_ADDR`                | `:8080`                          | HTTP listen address.                                                                          |
| `HARBORMASTER_DATA_DIR`                   | `/var/lib/harbormaster`          | Persistent directory for SQLite, encryption key, and WAL files.                               |
| `HARBORMASTER_DATABASE_PATH`              | `${DATA_DIR}/harbormaster.db`    | SQLite database path. Defaults under `DATA_DIR`.                                              |
| `HARBORMASTER_LOG_LEVEL`                  | `info`                           | zerolog level (`trace`, `debug`, `info`, `warn`, `error`).                                    |
| `HARBORMASTER_LOG_FORMAT`                 | `json`                           | `json` (production) or `console` (developer-friendly).                                        |
| `HARBORMASTER_SESSION_TIMEOUT`            | `8h`                             | Session inactivity timeout. Go duration string.                                               |
| `HARBORMASTER_SESSION_COOKIE_NAME`        | `harbormaster_session`           | Session cookie name.                                                                          |
| `HARBORMASTER_BASE_PATH`                  | `/`                              | URL prefix when reverse-proxied at a subpath (e.g. `/harbormaster`). Must start with `/`.     |
| `HARBORMASTER_TRUSTED_PROXIES`            | (empty)                          | CSV of CIDRs; controls client-IP derivation behind a proxy.                                   |
| `HARBORMASTER_UPLOAD_MAX_BYTES`           | `104857600` (100 MiB)            | Hard cap on per-request upload size.                                                          |
| `HARBORMASTER_SHARE_LINK_MAX_TTL`         | `168h` (7 days)                  | Upper bound an operator may pick for object share-link expiry.                                |
| `HARBORMASTER_DOWNLOAD_PROXY_MODE`        | `proxy`                          | `proxy` streams via Harbormaster; `direct` returns a presigned MinIO URL (MinIO must be reachable from the browser). |
| `HARBORMASTER_MC_CONFIG_PATH`             | `/root/.mc/config.json`          | Read **only** during first-run setup. Bind-mount your host `~/.mc/config.json` here to opt in (see below). |
| `HARBORMASTER_TLS_CERT_FILE` / `_KEY_FILE`| (empty)                          | If both set, Harbormaster serves HTTPS directly.                                              |
| `HARBORMASTER_ENCRYPTION_KEY_FILE`        | `${DATA_DIR}/encryption.key`     | 32-byte key file used to encrypt sensitive columns (MinIO secret, custom CA). Auto-generated `0600` on first start if absent. |
| `HARBORMASTER_METRICS_ENABLED`            | `false`                          | Enables the Prometheus listener.                                                              |
| `HARBORMASTER_METRICS_LISTEN_ADDR`        | `:9090`                          | Bind address for the metrics listener.                                                        |
| `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT`| (empty)                          | If set, the OTLP-HTTP trace exporter is enabled.                                              |
| `HARBORMASTER_AUDIT_RETENTION`            | `90 * 24h` (~90 days)            | How long the audit-retention sweeper keeps events.                                            |
| `HARBORMASTER_CONFIG`                     | (empty)                          | Optional path to a config file (YAML/TOML/JSON).                                              |

A longer-form table with valid values, validators, and side effects
lives in [`docs/operator/configuration.md`](docs/operator/configuration.md).

## Security model

- **Encryption at rest.** MinIO credentials and (optional) custom CA
  PEMs are encrypted with AES-256-GCM and stored in SQLite. The 32-byte
  key file (`encryption.key`) lives in `DATA_DIR` with `0600` perms; its
  SHA-256 fingerprint is recorded in `app_settings` and re-checked on
  startup — a mismatch refuses to boot with a `key_fingerprint_mismatch`
  error. Lose or rotate this file → all encrypted columns are
  unrecoverable. **Back it up alongside the SQLite file.**
- **Sessions.** Server-issued opaque IDs persisted in SQLite. Cookie is
  `HttpOnly`, `Secure` (when served over HTTPS — or behind a TLS proxy
  forwarding `X-Forwarded-Proto`), and `SameSite=Lax`.
- **CSRF.** Synchronizer-token middleware on all state-changing routes;
  token issued on session creation and verified on every non-GET.
- **Audit.** Append-only `audit_events` table records every privileged
  action with actor, action, target, outcome, and an opaque payload
  summary (no secrets). Retention is bounded by `AUDIT_RETENTION`.
- **MinIO floor version.** v1 supports `RELEASE.2025-09-07T16-13-09Z`
  and later. The nightly workflow exercises both the floor and `latest`.
- **Single replica.** v1 holds in-process state (rate limiter, empty
  worker, sweeper) and is intentionally a single replica. See R6.

Full threat-model and operational guidance:
[`docs/operator/security.md`](docs/operator/security.md).

## Importing from `mc` config

The first-run setup wizard can read your existing `~/.mc/config.json`
to pre-fill the MinIO connection form (endpoint, access key, secret
key, TLS flags). It is **opt-in** and **never used after setup**.

To enable, bind-mount your host config read-only:

```yaml
# in deploy/docker/docker-compose.yml — uncomment:
volumes:
  - ${HOME}/.mc/config.json:/root/.mc/config.json:ro
```

Rationale + caveats (R18): the secret key lives plaintext in the file.
Mounting it widens the blast radius of a compromised Harbormaster
container. The wizard only reads the file while
`setup_completed=false`; once setup finishes, the file is never
re-opened. Only `mc` config `version: "10"` is parsed.

## Recovery

### Reset the local admin password

```bash
# Docker
docker compose exec harbormaster \
  /usr/local/bin/harbormaster admin reset-password --username admin

# Kubernetes
kubectl -n harbormaster exec -it deploy/harbormaster -- \
  /usr/local/bin/harbormaster admin reset-password --username admin
```

### Rotate / recover the encryption key

`reset-encryption --confirm` destroys all encrypted columns (MinIO
credentials, custom CA). The admin account and audit history are
**preserved**. You'll need to re-run the MinIO connection wizard
afterwards.

```bash
# Docker
docker compose exec harbormaster \
  /usr/local/bin/harbormaster admin reset-encryption --confirm

# Kubernetes
kubectl -n harbormaster exec -it deploy/harbormaster -- \
  /usr/local/bin/harbormaster admin reset-encryption --confirm
```

Step-by-step walkthrough (including pre-flight backups):
[`docs/operator/recovery.md`](docs/operator/recovery.md).

## First-publish reminder (GHCR visibility)

GHCR packages default to **private** the first time `main.yml` pushes
to `ghcr.io/jtumidanski/harbormaster`. After the first successful run,
visit:

> https://github.com/users/jtumidanski/packages/container/harbormaster/settings

…and flip **Package visibility** to **Public** so unauthenticated users
can `docker pull`. This only needs to happen once.

## Project goals + non-goals

### Goals

- Intuitive web UI for the common MinIO workflows (bucket CRUD; object
  browse/upload/download; user + access-key management; template-driven
  policies; simple lifecycle rules) so an operator can avoid routine
  `mc` usage.
- Self-hosted with zero external dependencies. SQLite is the only
  datastore; no SaaS calls, no telemetry.
- Single-container multi-arch deployment. Docker Compose is the
  primary story; example Kubernetes manifests bundled.
- Operationally lightweight: stateless app upgrades, externalised
  config, `/healthz` and `/readyz`, structured JSON logs.
- Secure by default: encrypted credentials at rest, hardened session
  cookies, CSRF, confirmation gates on destructive operations.
- Supply-chain-aware CI/CD: PR gates, multi-arch GHCR publish,
  cosign keyless signing, Renovate, pinned action SHAs.

### Non-goals (v1)

- SaaS hosting; multi-cluster / multi-tenant management; enterprise IAM;
  Kubernetes Operator; replication topology editing; arbitrary IAM JSON;
  billing/tenants; AIStor enterprise integrations; object locking;
  backup orchestration; advanced audit exploration; multipart-upload UX
  beyond what the SDK provides; object diff / versioned browsing; OIDC /
  SSO; Helm chart; mobile-responsive admin beyond Tailwind defaults.

The full PRD lives at
[`docs/tasks/task-001-harbormaster-mvp-v1/prd.md`](docs/tasks/task-001-harbormaster-mvp-v1/prd.md).

## License

Copyright (C) 2026 Harbormaster contributors.

Harbormaster is free software: you can redistribute it and/or modify it
under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version. See `LICENSE` for the full text.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
Affero General Public License for more details.

**AGPL implications for hosters:** modifications you ship to users
(including over a network) must be made available under AGPL terms.
See [`docs/operator/security.md`](docs/operator/security.md#agpl-implications).

## Links

- **PRD:** [`docs/tasks/task-001-harbormaster-mvp-v1/prd.md`](docs/tasks/task-001-harbormaster-mvp-v1/prd.md)
- **Architecture overview:** [`docs/architecture/overview.md`](docs/architecture/overview.md)
- **Configuration reference:** [`docs/operator/configuration.md`](docs/operator/configuration.md)
- **Security guide:** [`docs/operator/security.md`](docs/operator/security.md)
- **Recovery guide:** [`docs/operator/recovery.md`](docs/operator/recovery.md)
- **Reverse-proxy guide:** [`docs/operator/reverse-proxy.md`](docs/operator/reverse-proxy.md)
- **Kubernetes manifests:** [`deploy/kubernetes/README.md`](deploy/kubernetes/README.md)
- **Changelog:** [`CHANGELOG.md`](CHANGELOG.md)
