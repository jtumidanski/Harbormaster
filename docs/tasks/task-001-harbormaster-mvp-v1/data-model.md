# Data Model — Harbormaster v1 (SQLite)

Companion to `prd.md` §6. Documents the v1 SQLite schema, key invariants, and crypto envelope. DDL is illustrative — the final form will be expressed as numbered migration files chosen during the design phase (`golang-migrate`, `goose`, or hand-rolled).

## Invariants

- v1 stores **all** local state in a single SQLite file at `HARBORMASTER_DATABASE_PATH`.
- WAL mode (`PRAGMA journal_mode=WAL;`), `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000` set at connection open.
- All sensitive columns store AES-256-GCM ciphertext encoded as `base64( nonce(12B) || ciphertext || tag(16B) )`. The encryption key is loaded from `HARBORMASTER_ENCRYPTION_KEY_FILE` (or `<data_dir>/encryption.key`); see `prd.md` §6.2.
- All timestamps stored as ISO-8601 UTC strings (`TEXT`). Application converts to/from `time.Time`. (SQLite has no native timestamp type; ISO-8601 strings sort correctly.)
- ULIDs stored as Crockford base32 `TEXT(26)` so they sort lexicographically by time.

## Tables

### `schema_migrations`

Managed by the chosen migration library. Shape varies by library; commonly:

```sql
CREATE TABLE schema_migrations (
  version BIGINT PRIMARY KEY,
  dirty BOOLEAN NOT NULL DEFAULT 0
);
```

### `app_settings`

Singleton key/value store for application-level flags.

```sql
CREATE TABLE app_settings (
  key         TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
```

Well-known keys (v1):

| Key | Value type | Purpose |
| --- | ---------- | ------- |
| `setup_completed`              | `"true"` / absent | Gates the first-run wizard |
| `encryption_key_fingerprint`   | SHA-256(key) hex  | Detects accidental key swap |
| `audit_retention_days`         | integer string    | Cached config snapshot |

### `admin_users`

In v1, exactly one row. Schema is plural to keep migration room for future multi-user / OIDC support.

```sql
CREATE TABLE admin_users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  username        TEXT NOT NULL UNIQUE,
  password_hash   TEXT NOT NULL,            -- argon2id encoded string ("$argon2id$v=19$m=...$...")
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  disabled_at     TEXT
);
```

### `sessions`

```sql
CREATE TABLE sessions (
  id              TEXT PRIMARY KEY,         -- ULID
  admin_user_id   INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL,
  expires_at      TEXT NOT NULL,
  last_active_at  TEXT NOT NULL,
  source_ip       TEXT,                     -- string form; v4 or v6
  user_agent      TEXT
);

CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
CREATE INDEX sessions_admin_user_id_idx ON sessions(admin_user_id);
```

Session pruning sweeper runs once per minute; expired rows are deleted.

### `minio_connections`

Singleton row in v1 (a single MinIO endpoint per Harbormaster). The PK + partial unique index enforces this.

```sql
CREATE TABLE minio_connections (
  id                          INTEGER PRIMARY KEY AUTOINCREMENT,
  singleton_guard             INTEGER NOT NULL DEFAULT 1,  -- always 1
  endpoint_url                TEXT NOT NULL,
  tls_skip_verify             BOOLEAN NOT NULL DEFAULT 0,
  access_key_ciphertext       TEXT NOT NULL,
  secret_key_ciphertext       TEXT NOT NULL,
  custom_ca_pem_ciphertext    TEXT,                         -- nullable
  created_at                  TEXT NOT NULL,
  updated_at                  TEXT NOT NULL
);

-- Enforce exactly one row:
CREATE UNIQUE INDEX minio_connections_singleton ON minio_connections(singleton_guard);
```

### `audit_events`

```sql
CREATE TABLE audit_events (
  id                  TEXT PRIMARY KEY,                   -- ULID
  occurred_at         TEXT NOT NULL,
  actor               TEXT NOT NULL,                      -- "local-admin" in v1
  source_ip           TEXT,
  action              TEXT NOT NULL,                      -- e.g. "bucket.create"
  target_type         TEXT NOT NULL,
  target_id           TEXT,
  outcome             TEXT NOT NULL,                      -- "success" | "failure"
  error_message       TEXT,                               -- truncated to 1024 chars by writer
  payload_summary_json TEXT                               -- small JSON object; never contains secrets
);

CREATE INDEX audit_events_occurred_at_idx ON audit_events(occurred_at);
CREATE INDEX audit_events_action_idx      ON audit_events(action, occurred_at);
CREATE INDEX audit_events_target_idx      ON audit_events(target_type, target_id);
```

Retention sweeper deletes rows where `occurred_at < now - HARBORMASTER_AUDIT_RETENTION` once per day.

## Allowed `action` values (v1)

`bucket.create`, `bucket.delete`, `bucket.versioning.enable`, `bucket.versioning.disable`,
`object.upload`, `object.delete`, `object.presign_download`,
`user.create`, `user.delete`, `user.disable`, `user.enable`, `user.policies.update`,
`service_account.create`, `service_account.revoke`,
`lifecycle_rule.create`, `lifecycle_rule.delete`,
`session.login`, `session.logout`, `session.login_failed`,
`connection.update`, `connection.test`,
`admin.password.change`.

Unknown actions must be rejected by the writer (compile-time enum in Go).

## Example rows

```text
admin_users:
  (1, "admin", "$argon2id$v=19$m=65536,t=3,p=4$...$...", "2026-05-23T14:00:00Z", "2026-05-23T14:00:00Z", NULL)

sessions:
  ("01HKZ...", 1, "2026-05-23T14:05:00Z", "2026-05-23T22:05:00Z", "2026-05-23T14:12:34Z", "10.0.1.5", "Mozilla/5.0 ...")

minio_connections:
  (1, 1, "https://minio.lan:9000", 0,
   "B7cZk1+...", "Q9aLm0+...", NULL, "2026-05-23T14:00:30Z", "2026-05-23T14:00:30Z")

audit_events:
  ("01HKZ...", "2026-05-23T14:06:11Z", "local-admin", "10.0.1.5",
   "bucket.create", "bucket", "photos", "success", NULL,
   '{"versioning_enabled":true,"lifecycle_template":null}')
```

## Crypto envelope

- **Algorithm:** AES-256-GCM via `crypto/cipher`.
- **Key length:** 32 bytes (256 bits).
- **Nonce length:** 12 bytes, random per encryption.
- **Storage:** `base64.StdEncoding.EncodeToString(nonce || ciphertext || tag)`.
- **Associated data:** none in v1 (each column is self-contained; we accept the trade-off for simplicity).
- **Key rotation:** out of scope for v1. The `encryption_key_fingerprint` row in `app_settings` lets the app detect a key change at startup and refuse to start with a clear "key mismatch — refusing to start to avoid corrupting data" error.

## Backup / restore guidance (operator docs)

- Stop Harbormaster, copy the SQLite file + the encryption key file together (they're useless apart), restart.
- WAL mode means the data directory contains `harbormaster.db`, `harbormaster.db-wal`, `harbormaster.db-shm` — copy all three (or run `PRAGMA wal_checkpoint(TRUNCATE);` first, decided in the operator docs).

## Migration plan (v1 → future)

v1 ships migrations `0001_init.sql` through whatever number naturally falls out of the design phase. Future expansion areas anticipated by the schema:

- **Multi-admin / OIDC:** `admin_users` already plural; add `auth_provider`, `external_subject` columns later.
- **Multi-connection:** drop the singleton guard, add a `name` + display-name column to `minio_connections`.
- **Audit export:** add a `audit_export_cursors` table later; no v1 columns affected.
- **Encryption-key rotation:** add a `data_encryption_keys` table holding wrapped keys + an active-key pointer; ciphertext columns get a `key_id` neighbor. Doable additively.

All schema decisions in v1 favor additive future migrations.
