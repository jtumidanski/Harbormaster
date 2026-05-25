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
| `HARBORMASTER_BASE_PATH`                   | `/`                              | string                | URL prefix when reverse-proxied at a subpath. Must start with `/`; trailing `/` is normalised off.              |
| `HARBORMASTER_TRUSTED_PROXIES`             | (empty)                          | CSV of CIDRs          | Networks whose `X-Forwarded-For` / `X-Forwarded-Proto` headers are honoured by `chi/middleware.RealIP`.         |
| `HARBORMASTER_UPLOAD_MAX_BYTES`            | `104857600` (100 MiB)            | int64                 | Hard cap on per-request upload body size. Configure your reverse proxy to match.                                |
| `HARBORMASTER_SHARE_LINK_MAX_TTL`          | `168h` (7 days)                  | Go duration           | Upper bound an operator may pick when minting an object share link.                                             |
| `HARBORMASTER_DOWNLOAD_PROXY_MODE`         | `proxy`                          | enum                  | `proxy`: Harbormaster streams the object body. `direct`: return a presigned MinIO URL; MinIO must be reachable from the browser. |
| `HARBORMASTER_MC_CONFIG_PATH`              | `/root/.mc/config.json`          | path                  | Consulted **only** while `setup_completed=false`. Bind-mount your host `~/.mc/config.json` here to opt in.      |
| `HARBORMASTER_TLS_CERT_FILE`               | (empty)                          | path                  | PEM cert. If both this and the key are set, Harbormaster serves HTTPS directly.                                 |
| `HARBORMASTER_TLS_KEY_FILE`                | (empty)                          | path                  | PEM private key. Pair with the cert.                                                                            |
| `HARBORMASTER_ENCRYPTION_KEY_FILE`         | `${DATA_DIR}/encryption.key`     | path                  | 32-byte key used to encrypt sensitive columns. Auto-generated `0600` on first boot if absent.                   |
| `HARBORMASTER_METRICS_ENABLED`             | `false`                          | bool                  | Enables the Prometheus listener on a separate `http.Server`.                                                    |
| `HARBORMASTER_METRICS_LISTEN_ADDR`         | `:9090`                          | `host:port`           | Bind address for the metrics listener; ignored when metrics are disabled.                                       |
| `HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` | (empty)                          | URL                   | If set, enables OTLP-HTTP trace exporter; otherwise tracing is a no-op.                                         |
| `HARBORMASTER_AUDIT_RETENTION`             | `2160h` (~90 days)               | Go duration           | Audit-event retention. The sweeper runs daily and deletes rows older than this.                                 |
| `HARBORMASTER_INTEGRATION`                 | (empty)                          | bool gate             | Test-only: when `1`, the integration suite stops skipping. Not consumed by the running server.                  |
| `HARBORMASTER_MINIO_IMAGE`                 | (empty)                          | image ref             | Test-only: when set, overrides the MinIO testcontainer image. The nightly workflow's matrix uses this.          |

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
  large downloads but only works when MinIO is reachable from the
  browser (homelab usually OK; production behind a private network
  usually not).
- **Backup the encryption key with the database.** They're a matched
  pair; restoring one without the other is unusable.
