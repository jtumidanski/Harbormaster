# Harbormaster — Configuration reference

Harbormaster configuration is loaded by `internal/config/config.go` via
[Viper](https://github.com/spf13/viper). The Config struct is
**immutable** after `Load()` returns.

## Precedence (highest wins)

1. **Environment variables** prefixed `HARBORMASTER_`. Names match the
   table below; dots in nested keys become underscores.
2. **Config file** at the path in `HARBORMASTER_CONFIG`. Extension picks
   the format (`.yaml`, `.toml`, `.json`). Optional.
3. **Built-in defaults** (see `internal/config/config.go::defaults`).

Validation runs after merging; any failure aborts startup with a
descriptive error. The validators are:

- `BASE_PATH` must begin with `/`.
- `LOG_FORMAT` must be `json` or `console`.
- `DOWNLOAD_PROXY_MODE` must be `proxy` or `direct`.
- `TLS_CERT_FILE` and `TLS_KEY_FILE` must both be set or both be empty.
- `UPLOAD_MAX_BYTES` must be positive.

## Reference

| Env var                                    | Default                          | Type                  | Effect / notes                                                                                                  |
| ------------------------------------------ | -------------------------------- | --------------------- | --------------------------------------------------------------------------------------------------------------- |
| `HARBORMASTER_CONFIG`                      | (empty)                          | path                  | Optional path to a YAML/TOML/JSON config file. Read once during `Load()`.                                       |
| `HARBORMASTER_LISTEN_ADDR`                 | `:8080`                          | `host:port`           | Main HTTP listener.                                                                                             |
| `HARBORMASTER_DATA_DIR`                    | `/var/lib/harbormaster`          | path                  | Holds SQLite DB, WAL/journal, and (default location of) the encryption key.                                     |
| `HARBORMASTER_DATABASE_PATH`               | `${DATA_DIR}/harbormaster.db`    | path                  | SQLite DB path. If empty, defaults under `DATA_DIR`.                                                            |
| `HARBORMASTER_LOG_LEVEL`                   | `info`                           | enum                  | zerolog level: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`.                                     |
| `HARBORMASTER_LOG_FORMAT`                  | `json`                           | enum                  | `json` for production; `console` for human-readable colour output.                                               |
| `HARBORMASTER_SESSION_TIMEOUT`             | `8h`                             | Go duration           | Inactivity timeout for sessions. Cookie is refreshed on each request.                                           |
| `HARBORMASTER_SESSION_COOKIE_NAME`         | `harbormaster_session`           | string                | Cookie name for the session ID.                                                                                 |
| `HARBORMASTER_SESSION_COOKIE_SECURE`       | `true`                           | bool                  | Sets the `Secure` attribute on the session + CSRF cookies. Keep `true` behind HTTPS (browsers silently drop `Secure` cookies on plain HTTP, breaking login). Set `false` only for plain-HTTP LAN/homelab access — the cookie then travels unencrypted. |
| `HARBORMASTER_BASE_PATH`                   | `/`                              | string                | URL prefix when reverse-proxied at a subpath. Must start with `/`; trailing `/` is normalised off.              |
| `HARBORMASTER_TRUSTED_PROXIES`             | (empty)                          | CSV of CIDRs          | Reverse-proxy networks whose `X-Forwarded-For` hops are skipped when deriving the client IP. Empty (the default) means the TCP peer address is used and forwarding headers are ignored entirely. Invalid CIDRs fail startup. |
| `HARBORMASTER_UPLOAD_MAX_BYTES`            | `104857600` (100 MiB)            | int64                 | Hard cap on per-request upload body size. Configure your reverse proxy to match.                                |
| `HARBORMASTER_SHARE_LINK_MAX_TTL`          | `168h` (7 days)                  | Go duration           | Upper bound an operator may pick when minting an object share link.                                             |
| `HARBORMASTER_DOWNLOAD_PROXY_MODE`         | `proxy`                          | enum                  | `proxy`: Harbormaster streams the object body. `direct`: return a presigned URL from the object store; the object store must be reachable from the browser. |
| `HARBORMASTER_MC_CONFIG_PATH`              | `/root/.mc/config.json`          | path                  | Consulted **only** while `setup_completed=false`. Bind-mount your host `~/.mc/config.json` here to opt in.      |
| `HARBORMASTER_TLS_CERT_FILE`               | (empty)                          | path                  | PEM cert. If both this and the key are set, Harbormaster serves HTTPS directly.                                 |
| `HARBORMASTER_TLS_KEY_FILE`                | (empty)                          | path                  | PEM private key. Pair with the cert.                                                                            |
| `HARBORMASTER_ENCRYPTION_KEY_FILE`         | `${DATA_DIR}/encryption.key`     | path                  | 32-byte key used to encrypt sensitive columns. Auto-generated `0600` on first boot if absent.                   |
| `HARBORMASTER_METRICS_ENABLED`             | `false`                          | bool                  | Enables the Prometheus listener on a separate `http.Server`.                                                    |
| `HARBORMASTER_METRICS_LISTEN_ADDR`         | `:9090`                          | `host:port`           | Bind address for the metrics listener; ignored when metrics are disabled.                                       |
| `HARBORMASTER_PROMETHEUS_URL`              | (empty)                          | URL                   | When set, the dashboard's request/capacity series are read from this Prometheus (PromQL over `rustfs_*` metrics) instead of the target's `/minio/v2/metrics` endpoint. Required for RustFS targets. Empty keeps the object store's own scrape path. |
| `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` | (empty)                          | URL                   | If set, enables OTLP-HTTP trace exporter; otherwise tracing is a no-op.                                         |
| `HARBORMASTER_AUDIT_RETENTION`             | `2160h` (~90 days)               | Go duration           | Audit-event retention. The sweeper runs daily and deletes rows older than this.                                 |
| `HARBORMASTER_INTEGRATION`                 | (empty)                          | bool gate             | Test-only: when `1`, the integration suite stops skipping. Not consumed by the running server.                  |
| `HARBORMASTER_IT_TARGET`                   | (empty)                          | enum                  | Test-only: `minio` (default) or `rustfs`; selects the server the integration suite runs against.                |
| `HARBORMASTER_MINIO_IMAGE`                 | (empty)                          | image ref             | Test-only: when set, overrides the MinIO testcontainer image. Ignored when `HARBORMASTER_MINIO_BINARY` is set.  |
| `HARBORMASTER_MINIO_BINARY`                | (empty)                          | path                  | Test-only: when set, each integration test runs this `minio server` binary as a local process (no Docker). The nightly workflow uses this. |
| `HARBORMASTER_RUSTFS_IMAGE`                | (empty)                          | image ref             | Test-only: overrides the RustFS testcontainer image (`rustfs/rustfs:1.0.0`). Ignored when `HARBORMASTER_RUSTFS_BINARY` is set. |
| `HARBORMASTER_RUSTFS_BINARY`               | (empty)                          | path                  | Test-only: when set, each integration test runs this `rustfs` binary as a local process (no Docker). The nightly workflow uses this for the rustfs leg. |

## Config-file example

Equivalent to the defaults except for log format and metrics:

```yaml
# /etc/harbormaster/config.yaml — point HARBORMASTER_CONFIG at this.
listen_addr: ":8080"
data_dir: "/var/lib/harbormaster"
log_level: "info"
log_format: "console"
session_timeout: "8h"
base_path: "/"
upload_max_bytes: 104857600
share_link_max_ttl: "168h"
download_proxy_mode: "proxy"
metrics_enabled: true
metrics_listen_addr: ":9090"
prometheus_url: ""
audit_retention: "2160h"
```

## Operational tips

- **Trusted proxies must be set** when running behind nginx/Caddy/Traefik;
  otherwise audit events log the proxy's IP instead of the real client.
- **`UPLOAD_MAX_BYTES` cap is per-request.** Set your reverse-proxy's
  body-size limit to the same value (`client_max_body_size` for nginx)
  or you'll get a 413 from the proxy before Harbormaster sees the
  upload.
- **`DOWNLOAD_PROXY_MODE=direct`** halves Harbormaster's CPU/memory for
  large downloads but only works when the object store is reachable from
  the browser (homelab usually OK; production behind a private network
  usually not).
- **Backup the encryption key with the database.** They're a matched
  pair; restoring one without the other is unusable.

## Renamed in 2026-09

The `minio_connections` table is now `connections` (migration 0008). The
rename runs automatically on the next start; no operator action is
required.

## Renamed in 2026-10

Wire-level names changed:

- `minio` request field → `object_store` (legacy `minio` shim accepted for one release)
- `minio_version` response field → `server_version` (legacy `minio_version` emitted alongside it for one release)
- `minio_*` metric series keys → `objectstore_*` (legacy `minio_*` aliases emitted for one release)
- `minio-builtin` policy origin → `server-builtin`
- `minio_unreachable` error code → `object_store_unreachable`
- `minio_invalid_credentials` error code → `object_store_invalid_credentials`
- `minio_not_admin` error code → `object_store_not_admin`
- `minio_unavailable` error code → `object_store_unavailable`
- `minio_error` error code → `object_store_error`
- `minio_rejected_policy` error code → `object_store_rejected_policy`

Migration 0009 renames the `minio_`-prefixed rows in `metrics_samples` to
their `objectstore_` equivalents, in one transaction, at startup. No
operator action is required.

### Rolling back to an image older than this release

The previous image (`sha-42d7065`, golang-migrate v4) refuses to start
against a database at schema version 9 — it exits with "no migration found
for version 9". Rolling back requires manually unwinding the schema version
before deploying the older image.

1. Take a Longhorn snapshot of the `harbormaster-data` PVC first, so you
   have a restore point if any of the following steps go wrong.
2. Scale the Deployment to 0 (see `docs/operator/recovery.md` for the
   `kubectl ... scale deployment/harbormaster --replicas=0` form).
3. Run a one-shot debug pod that mounts the same PVC — the same pattern
   used for `admin reset-encryption` in
   [`docs/operator/recovery.md`](recovery.md#admin-reset-encryption---confirm),
   substituting `sqlite3` for the `harbormaster` binary as the pod's
   command — and against the DB file on the PVC run the statement from
   `apps/backend/migrations/0009_rename_metric_names.down.sql`, followed
   by:

   ```sql
   UPDATE schema_migrations SET version = 8, dirty = 0;
   ```

4. Deploy the previous image.

Skipping the `metrics_samples` rewrite and only resetting
`schema_migrations.version` is possible, but the `objectstore_*`-keyed rows
written since the upgrade won't match the older image's `minio_*`-only
queries — pre-rollback history disappears from the dashboard until
retention clears those rows.
