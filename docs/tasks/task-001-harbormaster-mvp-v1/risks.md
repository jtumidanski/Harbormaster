# Risks — Harbormaster v1

Companion to `prd.md`. Risks the design phase should explicitly address. Each entry: what could go wrong, why it matters for an MVP targeting homelab operators, and the mitigation direction. The plan phase should incorporate a concrete countermeasure for each.

## R1 — Scope is enormous for a single task

This PRD captures the full v1 MVP at the explicit request of the operator. Comprising ~9 feature areas plus CI/CD, deployment, security, and supply-chain work, it will not fit a normal single-PR `/execute-task` cycle without internal subdivision.

- **Why it matters:** without milestone-level checkpoints, the branch will drift far from `main` before anything is reviewable.
- **Mitigation direction:** the design phase must group functionality into milestone phases (e.g., M1: scaffolding + auth + setup; M2: bucket + object CRUD; M3: users + access keys + policies; M4: lifecycle + audit; M5: deployment + CI/CD + supply chain), and the plan phase must structure tasks so each milestone is independently demoable and mergeable as a stacked branch series back into `task-001-harbormaster-mvp-v1`. Even though we ship as one task, internal stacking is the only realistic execution model.

## R2 — Encryption key handling is fragile

The encryption key file is the single point of failure for credential decryption. If it is lost, the SQLite file becomes unrecoverable; if it is captured, all stored MinIO credentials leak.

- **Why it matters:** homelab operators routinely lose track of files outside their container's primary volume.
- **Mitigation direction:** put the key file inside the persistent data dir by default (so the standard "back up the data dir" advice covers both), enforce `0600` permissions at startup, surface a clear "key mismatch" error if `encryption_key_fingerprint` in `app_settings` does not match the loaded key, and document the back-up-and-restore flow prominently in operator docs.

## R3 — CGO vs static-binary tension for the SQLite driver

The most popular Go SQLite driver (`mattn/go-sqlite3`) requires CGO, which conflicts with the smallest distroless image (`gcr.io/distroless/static-debian12:nonroot`) and with simple cross-compilation for `linux/arm64`.

- **Why it matters:** affects the entire CI build pipeline; switching late is expensive.
- **Mitigation direction:** design phase to commit to `modernc.org/sqlite` (pure Go, CGO-free) **or** accept `gcr.io/distroless/base-debian12:nonroot` (slightly larger, glibc available). Decide before plan phase; backtracking after binary builds are wired is costly.

## R4 — Bucket destruction is irreversible — mitigated by empty-then-delete split

The previous PRD draft allowed force-deleting a non-empty bucket in one click. The current PRD requires an explicit **Empty bucket** step (asynchronous, progress-streamed) before **Delete bucket** is enabled. Each step demands typed bucket-name confirmation.

- **Residual risk:** the empty operation itself is still destructive — once started it deletes objects in 1000-object batches and there is no undo. An operator who reflexively types the name to dismiss the modal will lose data.
- **Why it matters:** still the single largest data-loss surface in the product; Harbormaster is the user's only guardrail.
- **Mitigation direction:** the Empty-bucket modal must show object count + total size + (if versioning is enabled) a note that "with versioning on, deleted objects become delete-markers and are recoverable; with versioning off, deletion is permanent"; the audit event records the final deleted count and total bytes for forensic visibility; the Delete step is intentionally a separate post-empty action so the operator must intentionally walk through two destructive confirmations.

## R5 — Object browser performance at scale

The current PRD targets responsive listings at **10,000 objects per page-worth of prefix** (revised down from an earlier 100,000 target as unrealistic against the S3 `ListObjectsV2` 1000-entries-per-call ceiling). The UI commits to virtualized rendering so DOM size stays bounded as the operator browses deep prefixes incrementally.

- **Residual risk:** virtualization alone is not sufficient — the auto-load-next-page UX must avoid runaway request fan-out if an operator scrolls aggressively through a million-object prefix.
- **Why it matters:** the main user-visible performance commitment.
- **Mitigation direction:** server-side pagination via S3 continuation tokens (PRD specifies); UI uses `@tanstack/react-virtual` (or equivalent — design phase decides) for the listing view; auto-load throttles outstanding requests to a small constant (e.g., 1 in flight); design phase commits per-page render budget and the throttle parameter; consider an "estimated total" hint sourced from `BucketInfo.objects` so the UI can warn before chewing through a million-object prefix.

## R6 — Login rate limit is in-memory only

The login rate limiter is per-process and in-memory. A restart or a load-balancer in front of multiple replicas (Kubernetes) bypasses it.

- **Why it matters:** the PRD scopes v1 to single-instance deployments, but Kubernetes manifests are bundled, and someone will run two replicas.
- **Mitigation direction:** document that v1 supports only a single replica; the Kubernetes manifests should set `replicas: 1` and a strategy of `Recreate` (not `RollingUpdate`); add a startup warning if `HARBORMASTER_REPLICA_AWARE` is set and there is no shared backend.

## R7 — Static manifests vs Helm chart

The PRD ships raw Kubernetes manifests rather than a Helm chart, leaving environment-specific customization (ingress class, storage class, secret strategy, image tags) up to copy-paste editing.

- **Why it matters:** every realistic deployment will need to fork the manifests; updates are then manual.
- **Mitigation direction:** keep the manifests minimal and well-commented, list the parameters operators will routinely change at the top of each file, and explicitly track "Helm chart" as a top-priority follow-up task once v1 ships.

## R8 — `harbormaster admin reset-password` requires data-dir access

The CLI recovery path requires the operator to exec into the container (or run the binary against the mounted data dir from outside). Documentation needs to be airtight or operators will get locked out.

- **Why it matters:** lockout recovery is the single most common path to product abandonment for self-hosted tools.
- **Mitigation direction:** quick-start docs include a one-liner `docker exec -it harbormaster harbormaster admin reset-password --username admin`; the Kubernetes docs include the `kubectl exec` equivalent; the binary's `--help` for the subcommand explains where it expects the database to be.

## R9 — JSON:API + multipart upload composition

JSON:API does not natively cover multipart form uploads. Mixing JSON:API for resource endpoints with plain JSON + multipart for action endpoints means two HTTP idioms in the same API surface, which the client must handle uniformly.

- **Why it matters:** the frontend fetch wrapper has to support both Content-Types and both error envelopes.
- **Mitigation direction:** centralize the difference in `lib/api/`; clearly mark in code which endpoints are "resource" vs "action"; document the split in the api-contracts file (done). Avoid sprinkling action endpoints throughout resource paths.

## R10 — Trivy false positives could block all merges

Trivy on the dependency-scan and image-scan jobs will flag CVEs. Without an allowlist, a single false positive blocks all PRs.

- **Why it matters:** the supply-chain story is only useful if it doesn't poison developer experience.
- **Mitigation direction:** ship a `.trivyignore` from day one with a documented review cadence; main-branch publish job allowlist is reviewed monthly; CRITICAL vulnerabilities never allowlisted, HIGH allowlisted only with a justification comment.

## R11 — GHCR public visibility is opt-in

A newly published GHCR package is private by default. If the publish workflow succeeds but the package is unreachable to homelab users, the project ships but appears broken.

- **Why it matters:** affects the "users can deploy locally" success criterion.
- **Mitigation direction:** the README publish instructions include the one-time "make the package public" step in the GHCR UI; the release workflow's first successful run should be followed by manually flipping the package to public.

## R12 — Embedded SPA route-vs-asset disambiguation

Serving the SPA via `//go:embed` requires the Go handler to disambiguate "this is a static asset, serve from FS" from "this is a SPA route, serve index.html" from "this is an API route, never touch the SPA."

- **Why it matters:** subtle wrong-mime-type bugs and 404s for client-side routes are common in this pattern.
- **Mitigation direction:** registration order: `/api/*` → API mux; `/healthz`/`/readyz` → health handlers; `/assets/*` and `/favicon.*` → embedded `fs.FS`; everything else (and only when the method is GET and the Accept header indicates HTML) → `index.html`. Document the rule in the server package.

## R13 — Test budget for end-to-end MinIO interactions

If MinIO integration tests run on every PR via `testcontainers-go`, CI minutes will balloon, and contributor laptops will need Docker available.

- **Why it matters:** affects iteration speed and contributor friction.
- **Mitigation direction:** split into "unit / fast" tests (always-on, run on PR) and "integration / minio-required" tests (gated by an env var, run on a nightly schedule and locally on demand). Design phase to commit to this split and define the naming convention.

## R14 — License is unset

No license file has been chosen, but the PRD requires one as an acceptance criterion. Contributions cannot be accepted until a license exists.

- **Why it matters:** legal ambiguity blocks community contributions and complicates GHCR publish copy.
- **Mitigation direction:** the operator chooses a license during the design phase from a short list (Apache-2.0, MIT, AGPL-3.0). Apache-2.0 is the conventional default for infrastructure tools of this shape.

## R15 — Operator unfamiliarity with MinIO admin API behavior

Some MinIO admin behaviors are subtly version-specific (e.g., the way service-account policies inherit from parent users, behavior of `RemoveBucketWithOptions(forceDelete=true)`, version of the admin API used by `mc admin`).

- **Why it matters:** the project assumes a single supported MinIO version range.
- **Mitigation direction:** design phase commits to a supported MinIO release floor (e.g., "RELEASE.2025-01-01T00-00-00Z or later"); CI integration tests pin to that floor and to the latest stable; `README.md` documents the support window.

## R16 — SSE streams behind reverse proxies often get buffered

The empty-bucket operation streams progress via `text/event-stream`. Common reverse proxies (nginx, Caddy, Traefik) default to buffering response bodies until "complete" or until a chunk threshold is reached, which collapses real-time progress into a single end-of-stream dump and makes the progress bar look stuck.

- **Why it matters:** the primary visual feedback for the most time-consuming destructive operation; if it appears broken, operators will assume the operation hung and may re-issue or kill it.
- **Mitigation direction:** the server emits `X-Accel-Buffering: no` and `Cache-Control: no-cache` headers; the example Compose file documents reverse-proxy configuration snippets (nginx `proxy_buffering off`, Caddy `flush_interval -1`, Traefik passthrough); the SSE endpoint also emits periodic comment-only heartbeat events (`: keepalive\n\n`) so proxies that buffer up to a byte threshold still get flushed periodically; the UI shows a "no progress events received in 30 s" warning when the stream stalls.

## R17 — Share links are not revocable from Harbormaster

Share links are S3 presigned URLs and embed a cryptographic signature derived from the MinIO admin's secret key plus the bucket/key/expiry. There is no way to invalidate a specific URL short of rotating the MinIO admin secret key (which invalidates every other access derived from it).

- **Why it matters:** an operator who shares a 7-day link with the wrong person has no in-product remedy; this contradicts a reasonable user expectation that a Harbormaster admin can "undo" their actions.
- **Mitigation direction:** the share-link modal copy makes the no-revocation property explicit before the link is generated; the audit feed records `object.share_link.create` with bucket/key/TTL so the operator can at least find what they shared; the operator docs include the "rotate the MinIO secret to invalidate all outstanding presigned URLs" recipe with the warning that this also invalidates Harbormaster's stored connection (forcing a Connection-settings update afterward).

## R18 — mc-config file access boundary

The setup wizard reads `HARBORMASTER_MC_CONFIG_PATH` directly off the filesystem the Harbormaster container can see. Default of `/root/.mc/config.json` puts an unusual file path on the read surface, and a malicious or compromised image could exfiltrate it on startup before any operator interaction.

- **Why it matters:** the file is only meaningful if it's bind-mounted (operators must opt in), but the convenience pulls operators toward mounting it; a future supply-chain compromise of the Harbormaster image would have read access to the mounted file.
- **Mitigation direction:** the mc-aliases endpoint is gated on `initialized=false` so the file is not re-read after setup; the operator docs explicitly recommend mounting the file read-only and only for the duration of first-run setup; reads of the file emit a log line so the operator can audit when it was accessed; design phase to confirm we never persist file contents anywhere on disk or in the database.
