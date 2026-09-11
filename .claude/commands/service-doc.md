---
description: Generate or update documentation for one Harbormaster component — dispatches the service-documentation agent
argument-hint: Component name or path (e.g., "buckets", "apps/backend/internal/buckets", "backend", "frontend")
---

Dispatch the `service-documentation` agent against: **$ARGUMENTS**.

Harbormaster has no `services/*` tree — the unit of documentation is a
**component**: a backend domain package under `apps/backend/internal/`, a
frontend feature under `apps/frontend/src/features/`, or a whole app
(`backend` / `frontend`). Output lands at `docs/architecture/<name>.md`.

The agent treats code as the single source of truth, follows the structure of
[`docs/architecture/overview.md`](../../docs/architecture/overview.md) and the
documentation contract in its own definition, and operates only within the
resolved component directory. It outputs only updated doc files — no
commentary, no analysis.
