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

## R4 — `force_delete` on non-empty buckets is irreversible

The MVP allows force-deleting a non-empty bucket via typed-name confirmation. A homelab user can wipe out a thousand objects in two clicks.

- **Why it matters:** data-loss surface; the user owns the data and Harbormaster is the only safety rail.
- **Mitigation direction:** require typed bucket name confirmation, surface the object count and total size in the confirmation modal, write an audit event that captures the count, and document recovery options (i.e., none unless versioning was enabled — explicitly warn about this in the modal copy).

## R5 — Object browser performance at 100k objects per prefix

The PRD targets responsive object listings at 100k objects per page-worth of prefix. The MinIO `ListObjectsV2` API returns up to 1000 entries per call; populating 100k entries requires 100 round-trips, plus a UI that can render that volume without jank.

- **Why it matters:** main user-visible performance commitment in the PRD.
- **Mitigation direction:** server-side pagination using S3 continuation tokens (PRD already specifies); UI uses virtualized lists (`@tanstack/react-virtual` or similar) for any view that can exceed a few hundred rows; never load 100k entries client-side at once. Design phase to spec the per-page render budget.

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
