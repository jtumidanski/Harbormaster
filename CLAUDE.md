# Harbormaster

Harbormaster is a self-hosted MinIO admin UI for homelab and small-cluster
operators.

- `apps/backend` — a single Go module (`cmd/`, `internal/`, `migrations/`,
  `Makefile`, `.golangci.yml`). There is no `go.work`; every Go change is
  local to this module.
- `apps/frontend` — Vite / React / TypeScript.
- `deploy/docker`, `deploy/kubernetes` — the two deploy paths.
- `docs/architecture/`, `docs/operator/`, `docs/tasks/` — design, runbooks,
  and per-task artifacts.
- `tools/` — `verify.sh` (the gate), `task-numbers.sh`, `task-brief.sh`,
  `toolchain.versions`.

## Never do this

- Don't implement when you were asked to understand or plan. Planning and
  implementation are separate phases; wait for explicit approval.
- Don't edit files in the main repo when a task worktree exists for that work.
- Don't open a PR without running the code review step (`/audit-plan` or
  `superpowers:requesting-code-review`) — not even when the plan looks complete.
- Don't call a `--quick` or `--no-docker` run "done". Those print
  `VERIFY: PARTIAL`, and they mean it.
- Don't walk a design or plan document through section-by-section approval. Write
  the full document to the file; the reader will read the committed file.
- Don't pick a task number by hand. `tools/task-numbers.sh next`.
- Don't `git add -A` or `git add .`.

## Evidence & grounding

- Verify MinIO admin API contracts, configuration values, and service-to-service
  interactions against local source or upstream MinIO docs. Never cite them from
  memory.
- When uncertain about behaviour, read the source rather than speculating.
- Report what you could not verify as unverified. Unverified is "unknown", not a
  plausible guess.

## Development workflow

1. `/spec-task <idea>` — from the main repo. Creates the worktree at
   `.worktrees/task-NNN-slug/` on branch `task-NNN-slug`, commits `prd.md`.
2. `/design-task <id>` — `design.md`.
3. `/plan-task <id>` — `plan.md` + `context.md`.
4. `/execute-task <id>` — implementation, in the existing worktree. Never
   creates a new one.

Phase 5 for a post-implementation bug is `/fix-pr-bug <task> <slug>`.

- Each phase runs in a fresh (`/clear`'d) session so it consumes only the prior
  phase's documented artifacts.
- **Artifact-location override:** `superpowers:brainstorming` and
  `superpowers:writing-plans` default to `docs/superpowers/specs/` and
  `docs/superpowers/plans/`. **In this project both go under
  `docs/tasks/task-NNN-slug/`.** Pass the task folder explicitly when invoking
  those skills outside a phase command.
- Fuzzy task identifiers: `task-001-slug`, `task-001`, `001` and `1` all resolve.
  Search both `docs/tasks/` and `.worktrees/*/docs/tasks/`.
- Worktree discipline: verify cwd is the right worktree before working; `cd`
  there yourself rather than asking. Search all worktrees (`git worktree list`)
  before concluding an artifact is missing.
- Task numbers come from `tools/task-numbers.sh next`. A `SessionStart` hook runs
  `tools/task-numbers.sh check` and reports collisions.
- Skip `/spec-task` only for trivial fixes that don't warrant a PRD.

## Done means verified

```sh
tools/verify.sh              # every gate. Exit 0 → the branch may be called done.
tools/verify.sh --quick      # skips buildx, -race, and npm ci when current. NOT done.
tools/verify.sh --no-docker  # skips buildx only. NOT done.
tools/verify.sh --list       # the gates these flags would select. Runs none.
tools/verify.sh --help       # usage, and the two on-demand suites.
```

Read the terminal `VERIFY:` line, not just the exit code — `DONE` and `PARTIAL`
both exit 0.

Two suites are on-demand and are not part of any mode:
`HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...` in
`apps/backend` (needs Docker), and `npm run test:e2e` in `apps/frontend`
(needs the Compose stack).

Gate failure, or the script disagreeing with CI: `docs/verification.md`.

## Dispatching agents

- The trio: `task-implementer` (one plan task, 120 tool-call budget, `PARTIAL`
  hand-back, module-local build/test only), `task-verifier` (runs
  `tools/verify.sh` in its own clean context, never edits), `task-reviewer` (one
  unit against its brief, artifact under `docs/tasks/<task>/reviews/`).
- Never run `tools/verify.sh` inside an implementer.
- Before a PR: `plan-adherence-reviewer`, `backend-guidelines-reviewer` (Go
  `DOM-*` / `SUB-*` / `SEC-*`), `frontend-guidelines-reviewer` (`FE-*`) — all
  writing to `docs/tasks/<task>/audit.md`, dispatched by
  `superpowers:requesting-code-review`.
- `service-documentation` / `/service-doc <component>` documents one component
  into `docs/architecture/<name>.md`.
- `todo-scanner` / `/review-todos` refreshes `docs/TODO.md`.
- Model pinning, fan-out vs. fork, and the handoff decision:
  `docs/agent-dispatch.md`.

## Handing off context

- Brief-first: an implementer gets `tools/task-brief.sh plan.md N`, not the whole
  plan. Plan task headings must match `^#+[ \t]+Task[ \t]+[0-9]+`.
- Slice a large document, diff, plan, or tool result before reading it whole:
  `docs/slice-first.md`.
- The handoff question: does the next unit depend on this conversation, or only
  on repository state? If only on repository state, write the state down and
  hand off.

## Repository conventions

- Prefer straightforward moves over re-exported type aliases when refactoring
  shared types or creating common libraries.
- Keep abstractions clean — don't break service boundaries by having one layer
  call another's internals directly.
- The `backend-dev-guidelines` and `frontend-dev-guidelines` skills in
  `.claude/skills/` are the authority on Go and React/TS patterns; the
  `skill-activation-prompt` hook auto-suggests them from the triggers in
  `.claude/skills/skill-rules.json`.
- Use repo-relative paths in committed files — never literal home or absolute
  paths. `block-home-paths-in-docs.sh` enforces this under `docs/`.
- Long-running processes go in the background and are never polled;
  `wait-loop-guard.sh` refuses a poll. See `docs/tooling-conventions.md`.

## Where the procedures live

| Trigger | Owner |
|---|---|
| Model pinning, fan-out vs. fork, the handoff decision | [`docs/agent-dispatch.md`](docs/agent-dispatch.md) |
| A gate failed, or the script disagrees with CI | [`docs/verification.md`](docs/verification.md) |
| A bare task number; a superpowers skill outside a phase command | [`docs/superpowers-integration.md`](docs/superpowers-integration.md) |
| Dispatching a reviewer; writing up a review | [`docs/review-protocol.md`](docs/review-protocol.md) |
| A bug found after implementation; Phase 5 / `/fix-pr-bug` | [`docs/post-implementation.md`](docs/post-implementation.md) |
| About to point a second implementer at the same transformation | [`docs/codemod-vs-agents.md`](docs/codemod-vs-agents.md) |
| Reading a large document, diff, plan, or tool result | [`docs/slice-first.md`](docs/slice-first.md) |
| Long-running processes, mechanical repo facts, shell conventions | [`docs/tooling-conventions.md`](docs/tooling-conventions.md) |
| Committing, pushing, rebasing, a stray commit on `main` | [`docs/git-workflow.md`](docs/git-workflow.md) |
| Deploying, reading logs, the runbook story | [`docs/observability.md`](docs/observability.md) |
