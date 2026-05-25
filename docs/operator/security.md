# Harbormaster — Security guide

This document is the operator's reference for Harbormaster's security
posture: what it does for you out of the box, what assumptions it makes,
and what you remain responsible for.

## Threat model (summary)

Harbormaster is designed for **trusted networks** — homelabs,
single-tenant Kubernetes clusters, small engineering teams behind a
VPN. It is **not** a public-internet SaaS.

| Asset                              | Defence                                                                                                  |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------- |
| MinIO admin credentials at rest    | AES-256-GCM encryption with a key file stored at `${DATA_DIR}/encryption.key` (`0600`).                  |
| Local admin password               | argon2id (RFC 9106 params, `memory=64MiB`, `iterations=3`, `parallelism=2`).                             |
| Session theft                      | HTTPOnly + Secure (behind HTTPS) + SameSite=Lax cookies; opaque server-issued IDs; `SESSION_TIMEOUT`.    |
| CSRF                               | Synchronizer-token middleware on every non-GET; token issued at session creation.                        |
| Login brute-force                  | In-memory token bucket (5 failures per 5 min per IP); 401 after lockout; window resets on success.       |
| Destructive ops via stolen session | Typed-confirmation modals (`empty bucket` / `delete user`); audit event regardless of outcome.           |
| Audit-event leakage of secrets     | Payload summaries are bounded structured maps; secrets (S3 access keys, passwords) never appear.         |
| Supply-chain compromise            | PR Trivy scan, main-build Trivy scan, cosign keyless sign, license allowlist, Renovate min-release-age.  |
| Reverse-proxy header spoofing      | `HARBORMASTER_TRUSTED_PROXIES` defaults empty — only configured CIDRs may set `X-Forwarded-*` headers.   |

Out-of-scope for v1: SaaS deployments, multi-tenancy, SSO/OIDC, MFA,
WebAuthn. See PRD §2 non-goals.

## Encryption key handling

The encryption key is a 32-byte random value used to encrypt the MinIO
secret key, the local admin's optional custom CA PEM, and any other
sensitive columns. Operationally:

- **File:** `${DATA_DIR}/encryption.key` by default; override with
  `HARBORMASTER_ENCRYPTION_KEY_FILE`.
- **Permissions:** `0600`. Created with these perms on first boot;
  Harbormaster refuses to read it if perms are looser than `0600`
  (e.g. world-readable) and aborts startup.
- **Fingerprint check:** SHA-256 of the key bytes is recorded in
  `app_settings.encryption_key_fingerprint` the first time the key is
  loaded. On every subsequent boot the fingerprint is recomputed and
  compared; a mismatch aborts with `key_fingerprint_mismatch`. This
  catches "operator swapped the key file by accident" — without this
  check, the app would silently fail to decrypt every existing row.
- **Backup:** **back up the key file alongside the SQLite DB.** They
  are a matched pair. Restoring the DB without the key (or vice versa)
  produces an unusable installation; the recovery path is
  `admin reset-encryption --confirm`, which destroys all encrypted
  columns and forces the MinIO connection wizard to re-run.
- **Rotation:** v1 does not support online rotation. Use
  `admin reset-encryption --confirm` (see `recovery.md`).

## Sessions + CSRF

- Sessions are server-side rows in SQLite, keyed by an opaque ID
  delivered in the cookie. The cookie is `HttpOnly`, `Secure` (when
  Harbormaster sees an HTTPS scheme — either direct TLS or via a
  trusted-proxy `X-Forwarded-Proto: https`), `SameSite=Lax`.
- `HARBORMASTER_SESSION_TIMEOUT` (default `8h`) is the inactivity
  timeout; activity refreshes the row's `last_seen_at`.
- CSRF uses a synchronizer token bound to the session row. The token
  is set on session creation and returned in a non-HttpOnly cookie so
  the SPA can mirror it back via the `X-CSRF-Token` header on every
  non-GET (chi middleware checks the match). Failures return 403.

## Single-replica deployment (R6)

Harbormaster v1 keeps multiple pieces of state in-process:

- **Login rate limiter** (token bucket).
- **Empty-bucket worker registry** (one goroutine per active job).
- **Audit-retention sweeper** (daily singleton).

Plus the SQLite database is `ReadWriteOnce` in Kubernetes terms — only
one node can hold it open at a time. **Therefore**:

- Docker: run one container per `harbormaster-data` volume.
- Kubernetes: the manifests pin `replicas: 1` with `strategy: Recreate`
  so the old pod releases the PVC before the new one starts. Don't
  scale up.

The 5-10 s gap during upgrade is by design. HA is a v2 conversation
(leader election + per-node session affinity).

## Share-link non-revocability (R17)

Object share links are minted by Harbormaster as presigned MinIO URLs
with an operator-chosen TTL (bounded by
`HARBORMASTER_SHARE_LINK_MAX_TTL`, default 7 days). **Once issued, a
share link cannot be revoked from Harbormaster.** It is signed by the
MinIO root credentials we stored at setup; the only way to invalidate
in-flight links is to rotate the MinIO root credentials themselves
(MinIO docs: `mc admin user svcacct edit`).

The audit event for share-link creation records bucket, key, and TTL
so an incident response has the exposure window in one place. The UI
modal that mints a link spells this out before the operator confirms.

## `mc` config exposure (R18)

The first-run setup wizard can read your host `~/.mc/config.json` to
pre-fill the MinIO connection form. **The secret key in `~/.mc/config.json`
is plaintext.** Bind-mounting it widens the blast radius of a
compromised Harbormaster container: an attacker with code execution in
the container can read every alias's secret, not just the one the
operator selected.

Mitigations:

- The wizard only opens the file while `setup_completed=false`. After
  setup, the path is never re-read.
- Mount the file **read-only** (`:ro` in compose, or `readOnly: true`
  on the volumeMount in Kubernetes).
- The aliases endpoint never returns secret keys to the browser — they
  are read server-side only when the form is submitted referring to a
  selected alias.
- Only `mc` config `version: "10"` is parsed; other versions are
  treated as "no aliases found" with a log line.

The compose file ships the volume **commented out** so this is opt-in.

## AGPL implications

Harbormaster is **AGPL-3.0-or-later**. The practical implications for
operators / hosters:

- **Internal-only deployment** (your homelab, your team's VPN): no
  obligations beyond running the binary.
- **Network-accessible deployment to users outside your organisation**:
  any modifications you ship must be made available to those users
  under AGPL terms. This includes patched binaries, custom UI tweaks,
  added features. Pointing users at the upstream
  `github.com/jtumidanski/Harbormaster` repo satisfies the obligation
  when you ship the upstream binary unmodified.
- **Bundling Harbormaster into a SaaS product**: the AGPL "remote
  network interaction" clause attaches; you must offer source for your
  modifications to the SaaS's users.

If AGPL terms don't suit your use case, do not deploy Harbormaster as
a network-facing service for users outside your control. The PRD §2
non-goals exclude "SaaS-hosted deployments" precisely because of this.

## Audit

Every privileged action emits an audit event (table:
`audit_events`). Each row carries actor (admin username), action
verb, target type + ID, outcome (`success` / `failure`), error message
when applicable, and a bounded structured payload summary. Secrets
never appear in payloads; the audit processor sanitises known
sensitive fields.

Retention is bounded by `HARBORMASTER_AUDIT_RETENTION` (default
`2160h` ≈ 90 days). The daily sweeper deletes older rows.

There is no audit-export UI in v1. To pull events out, attach to the
SQLite DB read-only and `SELECT * FROM audit_events` — or build a
small CLI on top.

## What to back up

A complete backup is:

1. `${DATA_DIR}/harbormaster.db` (the SQLite file).
2. `${DATA_DIR}/harbormaster.db-wal` and `*-shm` if present
   (WAL/journal — safer to copy while the app is stopped, or use a
   snapshot-consistent volume backup).
3. `${DATA_DIR}/encryption.key` (or wherever
   `HARBORMASTER_ENCRYPTION_KEY_FILE` points).

Restore is "drop these back into `DATA_DIR` and start the container."
Losing the key without the DB or vice versa requires
`admin reset-encryption --confirm`.
