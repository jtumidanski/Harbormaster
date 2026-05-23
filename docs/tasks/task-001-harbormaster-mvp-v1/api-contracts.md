# API Contracts — Harbormaster v1

Companion to `prd.md` §5. Shows concrete request/response shapes for the v1 endpoint inventory. All examples assume `Content-Type: application/json` unless stated otherwise, and write requests require an `X-CSRF-Token` header matching the value held in the CSRF cookie.

Conventions:

- Resource endpoints use [JSON:API](https://jsonapi.org) (Content-Type `application/vnd.api+json`).
- Action endpoints use plain JSON (`application/json`).
- Error model: JSON:API `errors[]` for resource endpoints, `{"error":{"code","message","details?"}}` for action endpoints.
- ULIDs are 26-char Crockford base32 strings; timestamps are RFC 3339 UTC.

---

## Setup & auth

### `GET /api/v1/setup/status`

Public. No auth required.

```json
{ "initialized": false }
```

### `GET /api/v1/setup/mc-aliases`

Public, **available only while `initialized=false`**. Reads `HARBORMASTER_MC_CONFIG_PATH` (default `/root/.mc/config.json`). Returns `{"aliases": []}` when the file is absent, unreadable, or the wizard is past setup.

```json
{
  "aliases": [
    { "name": "myminio", "endpoint": "https://minio.lan:9000", "access_key": "AKIA...", "tls_skip_verify": false },
    { "name": "play",    "endpoint": "https://play.min.io",    "access_key": "Q3AM...", "tls_skip_verify": false }
  ]
}
```

**Never** includes secret keys. The secret is read on the server only when `POST /api/v1/setup` references the alias by name.

### `POST /api/v1/setup`

Public, single-use (returns `409 already_initialized` after first success).

Explicit-credentials form:

```json
{
  "admin": { "username": "admin", "password": "correct horse battery staple!" },
  "minio": {
    "endpoint_url": "https://minio.lan:9000",
    "access_key": "AKIA...",
    "secret_key": "abcd1234...",
    "tls_skip_verify": false,
    "custom_ca_pem": null
  }
}
```

mc-alias form (server re-reads the mc config to fetch the secret):

```json
{
  "admin": { "username": "admin", "password": "correct horse battery staple!" },
  "minio": { "from_mc_alias": "myminio" }
}
```

When `from_mc_alias` is present, all other `minio.*` fields are ignored except `tls_skip_verify` and `custom_ca_pem`, which may be overridden by the caller.

Success `201`:

```json
{ "initialized": true }
```

Failure `422`:

```json
{
  "error": {
    "code": "minio_unreachable",
    "message": "MinIO admin API ping failed",
    "details": { "underlying": "Get \"https://minio.lan:9000/minio/admin/v3/info\": x509: ..." }
  }
}
```

Failure `422` when alias name is unknown:

```json
{ "error": { "code": "mc_alias_not_found", "message": "No alias named 'myminio' in /root/.mc/config.json" } }
```

### `POST /api/v1/auth/login`

Request: `{ "username": "admin", "password": "..." }`

Success `204` — sets `harbormaster_session` (HttpOnly, Secure, SameSite=Lax) and `harbormaster_csrf` (Secure, SameSite=Lax, **not** HttpOnly) cookies. Body is empty.

Failure `401`: `{ "error": { "code": "invalid_credentials", "message": "Invalid username or password" } }`

Rate-limited `429`: `{ "error": { "code": "too_many_attempts", "message": "Too many failed attempts; try again in 5 minutes" } }`

### `POST /api/v1/auth/logout`

Auth required. Invalidates current session. Response `204`, clears both cookies.

### `GET /api/v1/auth/me`

Auth required.

```json
{ "username": "admin", "session_expires_at": "2026-05-23T19:42:00Z" }
```

### `POST /api/v1/auth/password`

Auth required.

```json
{ "current_password": "...", "new_password": "..." }
```

Success `204`. Failure `401` if `current_password` is wrong; `422` if the new password fails policy (`code: "weak_password"`).

### `GET /api/v1/csrf`

Auth required. Returns / refreshes the CSRF token.

```json
{ "csrf_token": "9Vd...long opaque token..." }
```

Also sets `harbormaster_csrf` cookie if missing.

---

## Connection settings

### `GET /api/v1/connection`

```json
{
  "endpoint_url": "https://minio.lan:9000",
  "tls_skip_verify": false,
  "access_key_masked": "•••• •••• ABCD",
  "secret_key_present": true,
  "custom_ca_pem_present": false
}
```

### `PUT /api/v1/connection`

Request: same shape as `POST /api/v1/setup`'s `minio` block. Validates before persisting; failure returns 422 with the same `code` taxonomy as setup.

### `POST /api/v1/connection/test`

Request: same shape. Validate-only; no persistence. Response:

```json
{
  "tcp_connect": "ok",
  "list_buckets": "ok",
  "admin_ping": "ok",
  "minio_version": "RELEASE.2026-04-30T12-00-00Z"
}
```

Any non-`ok` value carries a `failed: { reason }` shape:

```json
{ "tcp_connect": "ok", "list_buckets": { "failed": "AccessDenied: ..." }, "admin_ping": null }
```

---

## Dashboard

### `GET /api/v1/dashboard`

```json
{
  "server": {
    "version": "RELEASE.2026-04-30T12-00-00Z",
    "deployment_mode": "single-node-single-drive",
    "uptime_seconds": 84321
  },
  "totals": { "buckets": 12, "estimated_bytes": 42949672960, "objects": 198432 },
  "nodes": [
    {
      "endpoint": "minio.lan:9000",
      "state": "online",
      "drives": { "total": 1, "healthy": 1, "unhealthy": 0 }
    }
  ],
  "warnings": [],
  "recent_activity": [
    {
      "id": "01HJ...",
      "occurred_at": "2026-05-23T14:01:22Z",
      "action": "bucket.create",
      "target_type": "bucket",
      "target_id": "photos",
      "outcome": "success"
    }
  ]
}
```

---

## Buckets

### `GET /api/v1/buckets?page[number]=1&page[size]=50&sort=-created`

JSON:API resource collection.

```json
{
  "data": [
    {
      "type": "buckets",
      "id": "photos",
      "attributes": {
        "name": "photos",
        "created_at": "2025-12-01T10:00:00Z",
        "estimated_bytes": 12884901888,
        "object_count": 9421,
        "versioning_enabled": true,
        "has_lifecycle_rules": true,
        "public_access": "private",
        "quota": { "kind": "hard", "bytes": 21474836480, "used_bytes": 12884901888 }
      }
    }
  ],
  "meta": { "page": { "number": 1, "size": 50, "total_pages": 1, "total_records": 12 } },
  "links": { "self": "/api/v1/buckets?page[number]=1&page[size]=50&sort=-created" }
}
```

`quota` is `null` when no quota is set. `public_access` is one of `private` / `public-read` / `public-read-write`.

### `POST /api/v1/buckets`

Request:

```json
{
  "data": {
    "type": "buckets",
    "attributes": {
      "name": "backups",
      "versioning_enabled": false,
      "public_access": "private",
      "lifecycle_template": null,
      "quota": { "kind": "hard", "bytes": 21474836480 }
    }
  }
}
```

`public_access` and `quota` are optional (default `private`, no quota). Success `201` returns the created resource. Failure `422` (e.g., invalid bucket name) uses JSON:API `errors[]`:

```json
{
  "errors": [
    {
      "status": "422",
      "code": "invalid_bucket_name",
      "title": "Invalid bucket name",
      "detail": "Bucket names must be between 3 and 63 characters, lowercase, and consist of letters, digits, dots, and hyphens.",
      "source": { "pointer": "/data/attributes/name" }
    }
  ]
}
```

### `GET /api/v1/buckets/{name}`

JSON:API single resource. Same shape as the list entry above.

### `DELETE /api/v1/buckets/{name}`

Request:

```json
{ "confirm_name": "backups" }
```

Success `204`. **No `force` flag in v1.**

- `409 bucket_not_empty` if any objects remain. Body: `{ "error": { "code": "bucket_not_empty", "message": "Bucket contains N objects; empty it first", "details": { "object_count": 142 } } }`. The client surfaces the link to the Empty-bucket flow.
- `403 confirm_name_mismatch` if `confirm_name != name`.

### `PUT /api/v1/buckets/{name}/versioning`

Request: `{ "enabled": true }` — Success `204`.

### `PUT /api/v1/buckets/{name}/public-access`

Request:

```json
{ "mode": "public-read", "confirm_name": "backups" }
```

`confirm_name` is required when transitioning into `public-read-write` (write-allowing). Success `204`. `403 confirm_name_mismatch` on mismatch.

### `PUT /api/v1/buckets/{name}/quota`

Set/update:

```json
{ "kind": "hard", "bytes": 21474836480 }
```

Clear:

```json
{ "kind": "none" }
```

Success `204`. `422 invalid_quota` if `bytes` is missing or non-positive while `kind != "none"`.

### `POST /api/v1/buckets/{name}/empty`

Initiates the asynchronous empty-bucket operation. Request:

```json
{ "confirm_name": "backups" }
```

Response: `text/event-stream`. Events:

```text
event: progress
data: {"deleted": 1000, "estimated_total": 5210}

event: progress
data: {"deleted": 2000, "estimated_total": 5210}

event: done
data: {"deleted_total": 5210, "duration_ms": 4321}
```

On unrecoverable error:

```text
event: error
data: {"message": "MinIO returned 503 during batch 3 of 6"}
```

Re-issuing while a job is in progress for the same bucket attaches the new stream to the existing job (no duplicate operation is started).

`403 confirm_name_mismatch` returned as a normal JSON body (HTTP error path) when `confirm_name` is wrong — the SSE stream is only started after validation passes.

---

## Objects

### `GET /api/v1/buckets/{name}/objects?prefix=2025/&delimiter=/&page[size]=100&page[token]=...`

```json
{
  "data": [
    {
      "type": "object_entries",
      "id": "2025/01/IMG_0001.jpg",
      "attributes": {
        "key": "2025/01/IMG_0001.jpg",
        "size": 4329122,
        "last_modified": "2025-01-15T08:11:02Z",
        "content_type": "image/jpeg",
        "etag": "\"d41d8cd98f00b204e9800998ecf8427e\""
      }
    },
    {
      "type": "object_prefixes",
      "id": "2025/02/",
      "attributes": { "prefix": "2025/02/" }
    }
  ],
  "meta": { "page": { "size": 100, "next_token": "eyJrZXkiOi..." } },
  "links": { "next": "/api/v1/buckets/photos/objects?prefix=2025/&delimiter=/&page[size]=100&page[token]=eyJrZXkiOi..." }
}
```

### `POST /api/v1/buckets/{name}/objects`

`Content-Type: multipart/form-data`. Form fields:

- `key` (string, required) — target object key.
- `file` (file, required).
- `content_type` (string, optional; sniffed if absent).

Success `201`:

```json
{
  "data": {
    "type": "object_entries",
    "id": "uploads/foo.bin",
    "attributes": { "key": "uploads/foo.bin", "size": 12345, "etag": "...", "last_modified": "..." }
  }
}
```

Failure `413 upload_too_large` when the body exceeds `HARBORMASTER_UPLOAD_MAX_BYTES`:

```json
{ "error": { "code": "upload_too_large", "message": "Upload exceeds the configured per-request cap of 100 MiB", "details": { "limit_bytes": 104857600 } } }
```

### `DELETE /api/v1/buckets/{name}/objects?key=<urlencoded>`

Success `204`.

### `GET /api/v1/buckets/{name}/objects/download?key=<urlencoded>`

**Proxy mode (default, `HARBORMASTER_DOWNLOAD_PROXY_MODE=proxy`).** Streams the object body through Harbormaster to the browser with:

- `Content-Type: <object MIME type or application/octet-stream>`
- `Content-Length: <bytes>` (when known)
- `Content-Disposition: attachment; filename="<basename>"`
- `Cache-Control: private, no-store`

Authenticated session required. No URL is exposed; only the operator's browser, within the session, can fetch the bytes.

**Direct mode (`HARBORMASTER_DOWNLOAD_PROXY_MODE=direct`).** Same endpoint, but responds with:

```text
HTTP/1.1 307 Temporary Redirect
Location: https://minio.lan:9000/photos/uploads/foo.bin?X-Amz-Algorithm=...
```

The presigned URL is short-lived (default 5 minutes, capped at 1 hour). Only works when MinIO is reachable from the browser.

### `POST /api/v1/buckets/{name}/objects/share-links`

Request:

```json
{ "key": "uploads/foo.bin", "expires_seconds": 604800 }
```

Response `201`:

```json
{
  "url": "https://minio.lan:9000/photos/uploads/foo.bin?X-Amz-Algorithm=...",
  "expires_at": "2026-05-30T15:00:00Z"
}
```

`expires_seconds` is clamped server-side to `[30, HARBORMASTER_SHARE_LINK_MAX_TTL]` (default upper bound = 7 days). The clamp is silent (the operator sees the actual `expires_at` returned).

Always writes a `object.share_link.create` audit event with `{bucket, key, expires_seconds}` — **never** the URL itself, since the URL embeds the signature.

`422 share_link_disabled` if a future configuration toggle disables share-link minting (placeholder — toggle not in v1).

---

## Users (MinIO IAM users)

### `GET /api/v1/users`

```json
{
  "data": [
    {
      "type": "users",
      "id": "alice",
      "attributes": {
        "access_key": "alice",
        "status": "enabled",
        "attached_templates": [{ "name": "read-write", "params": null }]
      }
    }
  ]
}
```

### `POST /api/v1/users`

Request:

```json
{
  "data": {
    "type": "users",
    "attributes": {
      "access_key": "alice",
      "templates": [{ "name": "read-write", "params": null }]
    }
  }
}
```

Success `201` — **only response that includes `secret_key`:**

```json
{
  "data": {
    "type": "users",
    "id": "alice",
    "attributes": {
      "access_key": "alice",
      "secret_key": "S3cr3t...shown-only-once",
      "status": "enabled",
      "attached_templates": [{ "name": "read-write", "params": null }]
    }
  }
}
```

### `PUT /api/v1/users/{access_key}/status`

Request: `{ "enabled": false }` — Success `204`.

### `DELETE /api/v1/users/{access_key}`

Request: `{ "confirm_access_key": "alice" }` — Success `204`. `403` on mismatch.

### `PUT /api/v1/users/{access_key}/policies`

Request:

```json
{ "templates": [{ "name": "backup-target", "params": { "bucket": "backups" } }] }
```

Success `204`.

---

## Service accounts

### `GET /api/v1/users/{access_key}/service-accounts`

```json
{
  "data": [
    {
      "type": "service_accounts",
      "id": "SA_alice_001",
      "attributes": {
        "access_key": "SA_alice_001",
        "parent_user": "alice",
        "name": "alice-backup-script",
        "description": "weekly restic backups",
        "attached_template": { "name": "backup-target", "params": { "bucket": "backups" } }
      }
    }
  ]
}
```

### `POST /api/v1/users/{access_key}/service-accounts`

Request:

```json
{
  "data": {
    "type": "service_accounts",
    "attributes": {
      "name": "alice-backup-script",
      "description": "weekly restic backups",
      "template_override": { "name": "backup-target", "params": { "bucket": "backups" } }
    }
  }
}
```

Success `201` includes `secret_key` (one-time).

### `DELETE /api/v1/service-accounts/{access_key}`

Success `204`.

---

## Policy templates

### `GET /api/v1/policy-templates`

```json
{
  "data": [
    {
      "type": "policy_templates",
      "id": "read-only",
      "attributes": { "name": "read-only", "description": "Read-only across all buckets", "params_schema": null }
    },
    {
      "type": "policy_templates",
      "id": "read-write",
      "attributes": { "name": "read-write", "description": "Read/write across all buckets, no admin operations", "params_schema": null }
    },
    {
      "type": "policy_templates",
      "id": "backup-target",
      "attributes": {
        "name": "backup-target",
        "description": "Read/write/delete in a specific bucket",
        "params_schema": {
          "type": "object",
          "required": ["bucket"],
          "properties": { "bucket": { "type": "string", "minLength": 3, "maxLength": 63 } }
        }
      }
    }
  ]
}
```

Three templates are bundled in v1. `administrator` (full `consoleAdmin`) is intentionally **not** in this list — see `prd.md` §4.10. Externally-attached policies (including `consoleAdmin`) still appear on the user-detail page in a read-only "Other attached policies" row, but are not surfaced here.

---

## Lifecycle rules

### `GET /api/v1/buckets/{name}/lifecycle-rules`

```json
{
  "data": [
    {
      "type": "lifecycle_rules",
      "id": "harbormaster-expire-uploads-30d",
      "attributes": {
        "managed": true,
        "kind": "expiration",
        "days": 30,
        "prefix": "uploads/"
      }
    },
    {
      "type": "lifecycle_rules",
      "id": "rule-from-mc-abc",
      "attributes": {
        "managed": false,
        "summary": "Unmanaged rule (created outside Harbormaster) — 2 actions: Transition, AbortIncompleteMultipart; scoped to 1 tag filter"
      }
    }
  ]
}
```

Managed rule attributes never include tag filters in v1 (tag filtering is not exposed in the create form). Unmanaged rules' tag count is summarized in the human-readable `summary` string (count, not values, to avoid surfacing potentially sensitive tag values).

### `POST /api/v1/buckets/{name}/lifecycle-rules`

Request:

```json
{
  "data": {
    "type": "lifecycle_rules",
    "attributes": { "kind": "expiration", "days": 30, "prefix": "uploads/" }
  }
}
```

Success `201` returns the created rule. v1 only accepts `kind = "expiration"`. Tag filters are intentionally not accepted — see `prd.md` §4.11.

### `DELETE /api/v1/buckets/{name}/lifecycle-rules/{rule_id}`

Success `204`. Allowed for both managed and unmanaged rules.

---

## Audit events

### `GET /api/v1/audit-events?filter[action]=bucket.delete&page[size]=50`

```json
{
  "data": [
    {
      "type": "audit_events",
      "id": "01HJ...",
      "attributes": {
        "occurred_at": "2026-05-23T14:01:22Z",
        "actor": "local-admin",
        "source_ip": "10.0.1.5",
        "action": "bucket.delete",
        "target_type": "bucket",
        "target_id": "old-backups",
        "outcome": "success",
        "error_message": null,
        "payload_summary": { "force": true, "object_count_at_delete": 142 }
      }
    }
  ],
  "meta": { "page": { "number": 1, "size": 50, "total_pages": 3, "total_records": 142 } }
}
```

Filter keys: `action`, `target_type`, `target_id`, `outcome`, `from` (RFC 3339), `to` (RFC 3339).

---

## Error code reference (non-exhaustive)

| Code | HTTP | Meaning |
| ---- | ---- | ------- |
| `unauthenticated`           | 401 | No or expired session |
| `csrf_token_invalid`        | 403 | Missing or mismatched CSRF token |
| `invalid_credentials`       | 401 | Login form mismatch |
| `too_many_attempts`         | 429 | Login rate-limited |
| `already_initialized`       | 409 | `/setup` called after first success |
| `weak_password`             | 422 | New password fails policy |
| `mc_alias_not_found`        | 422 | `/setup` referenced an unknown mc alias |
| `minio_unreachable`         | 422 | Setup / connection-test failed at TCP / TLS layer |
| `minio_invalid_credentials` | 422 | MinIO rejected the provided keys |
| `minio_not_admin`           | 422 | Provided MinIO keys lack admin capability |
| `invalid_bucket_name`       | 422 | Bucket name violates MinIO rules |
| `bucket_not_empty`          | 409 | Delete attempted on a non-empty bucket (no force flag in v1; use Empty-bucket first) |
| `invalid_quota`             | 422 | Quota payload missing or non-positive `bytes` |
| `upload_too_large`          | 413 | Upload body exceeds `HARBORMASTER_UPLOAD_MAX_BYTES` |
| `confirm_name_mismatch`     | 403 | `confirm_name` / `confirm_access_key` did not match |
| `not_found`                 | 404 | Resource missing |
| `internal_error`            | 500 | Bug; correlation_id included for log lookup |
