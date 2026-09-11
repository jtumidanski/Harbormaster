# Process Parity Harness — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-26
---

## 1. Overview

Harbormaster shares the *workflow* backbone with `atlas`, `home-hub`, and `MyFleet`:
the four phase commands, the worktree convention, the artifact-location override, the
three reviewer agents, and the two guideline skills. What it does not have is the
*context-discipline* layer that `atlas` grew afterwards — the enforcement hooks, the
owner-doc set, the budget-capped implementer, the isolated verifier, and the terse
rule-list `CLAUDE.md` shape that the owner-doc table makes possible. Without an
implementer/verifier/reviewer trio, `/execute-task` in this repo falls back to uncapped
generic dispatch.

This task executes `process-parity.md` §6 step 2 for Harbormaster. The canonical
specification and the Harbormaster-specific brief are embedded verbatim alongside this
PRD (`process-parity.md`, `process-parity-brief.md`) so this task folder is
self-contained; there is no sync mechanism and no dependency on the atlas worktree once
the port is done.

The bulk of the work is porting text. The one piece of genuine engineering is
`tools/verify.sh`, which must be built from a prose checklist that currently lives in
`CLAUDE.md`. That is the point of the exercise: a ten-command prose checklist cannot be
run by a verifier agent, and it is exactly the kind of thing that gets partially run and
then reported as green.

### Provenance

`process-parity.md` was re-synced from the atlas worktree
`.worktrees/task-266-process-parity-agent-rename` at commit `e83f59e61` on 2026-08-26,
one commit past the `e75c2a168` pin named in the brief. The delta is a single additive
paragraph in §7 check 3 recording that the `atlas-*` grep returned zero hits in all
three downstream repos. No binding, task, or check text changed, so nothing in the brief
is invalidated.

## 2. Goals

Primary goals:

- Give Harbormaster a single executable verification entrypoint, `tools/verify.sh`, that
  replaces the prose build checklist and can be run unattended by a verifier agent.
- Give `/execute-task` a budget-capped implementer, an isolated verifier, and a
  per-unit reviewer, so plan execution stops relying on uncapped generic dispatch.
- Install the portable enforcement hooks and wire them in `.claude/settings.json`.
- Replace the prose-narrative `CLAUDE.md` with the eight-heading rule-list shape backed
  by a trigger → owner-document table.
- Make collision-safe task numbering mechanical (`tools/task-numbers.sh` + the
  `SessionStart` collision detector) instead of the manual directory scan the phase
  commands do today.
- Satisfy `process-parity.md` §7 checks 2 and 3 in this repository, and report
  Harbormaster's side of checks 1, 4, 5, and 6.

Non-goals:

- Changing any application behaviour. No file under `apps/backend/` or `apps/frontend/`
  is modified except as noted in §7 (a possible `tools/toolchain.versions` reference; no
  source changes).
- Re-creating what Harbormaster already has: the four phase commands, `/audit-plan`,
  `/review-todos`, `backend-guidelines-reviewer`, `frontend-guidelines-reviewer`,
  `plan-adherence-reviewer`, `todo-scanner`, the `backend-dev-guidelines` /
  `frontend-dev-guidelines` skills, `skill-rules.json`, and the
  `skill-activation-prompt` hook.
- The `atlas-*` → `task-*` rename sweep. Harbormaster never used those names; §7 check 3
  already returns clean here (verified 2026-08-26).
- Porting `docs/packets/`, `docs/reverse-engineering.md`, or
  `docs/adding-a-new-service.md`. Harbormaster is a single Go binary plus one frontend
  and has no multi-service scaffolding story.
- Changing CI. `.github/workflows/*` keeps its current gate definitions; `verify.sh` is
  the local mirror of those gates, not a replacement for them.
- Loosening or tightening any existing gate's strictness (see §9, open question 1).

## 3. User Stories

- As the repository owner, I want one command that tells me whether the branch is done,
  so that "done" is a verified exit code rather than a memory of which checklist items I
  ran.
- As a verifier agent, I want a single entrypoint I can run in a clean context and report
  `PASS` or the first failing block, so that I never have to interpret a prose checklist.
- As an implementer agent, I want a bounded tool-call budget with an explicit `PARTIAL`
  hand-back, so that a task that is going badly surfaces early instead of burning a
  session.
- As a developer in the inner loop, I want `tools/verify.sh --quick` to skip the slow
  gates, so that I can get signal in a fraction of the time without pretending the branch
  is done.
- As a developer without Docker available, I want `tools/verify.sh --no-docker` to skip
  only the buildx step, so that the rest of the gate still runs.
- As a session starting work on a new task, I want task numbers allocated collision-safely
  across every worktree and branch, so that two concurrent tasks cannot claim the same
  number.
- As any agent reading `CLAUDE.md`, I want a short rule list plus a trigger → owner
  table, so that I can find the procedure that applies without reading prose narrative.

## 4. Functional Requirements

### 4.1 Portable hooks (`process-parity.md` §3.1)

Copy verbatim from `$ATLAS/.claude/hooks/` into `.claude/hooks/`:

| File | Wired at | Purpose |
|---|---|---|
| `wait-loop-guard.sh` | `PreToolUse` / `Bash` | Blocks polling and `sleep` loops. |
| `wait-loop-guard_test.sh` | — | Ships with its subject. |
| `block-home-paths-in-docs.sh` | `PreToolUse` / `Write\|Edit` | Rejects literal home/absolute paths under `docs/`. |
| `turn-budget.sh` | `PostToolUse` / `*` | Counts tool calls per agent. |
| `turn-budget-guard.sh` | `PreToolUse` / `*` | Makes the implementer tool-call cap binding. |
| `fork-dispatch-guard.sh` | `PreToolUse` / `Agent` | Surfaces the cost of `subagent_type: "fork"`. |
| `commit-boundary.sh` | `PostToolUse` / `Bash` | Commit-boundary guidance; references `tools/task-brief.sh`. |
| `task-num-collision-detector.sh` | `SessionStart` | Requires `tools/task-numbers.sh`. |

FR-1. After copying, `grep -l 'atlas-' .claude/hooks/*.sh` MUST print nothing.

FR-2. `wait-loop-guard_test.sh` MUST pass when run in this repository.

FR-3. `.claude/hooks/skill-activation-prompt.{py,sh}` MUST be left untouched and MUST
remain wired at `UserPromptSubmit`.

### 4.2 `format-on-write.sh` rebinding (`process-parity.md` §3.2)

`format-on-write.sh` is NOT portable. Atlas's version hardcodes `services/atlas-ui` for
prettier and sources `tools/toolchain.versions` for the pinned `golangci-lint`.

FR-4. The prettier path MUST be rebound to `apps/frontend`.

FR-5. A new `tools/toolchain.versions` MUST be created, pinning at minimum
`golangci-lint` to the version CI uses. As of 2026-08-26 that is `v2.12.2`
(`.github/workflows/pr.yml:49`). The file SHOULD also record the Node version CI pins
(`24.19.0`) and MAY record the Go version, which CI derives from
`apps/backend/go.mod` via `go-version-file`.

FR-6. Because the pin now exists in two places, `tools/verify.sh` MUST include a drift
check asserting that the `golangci-lint` version in `tools/toolchain.versions` matches
the one in `.github/workflows/pr.yml`, failing the gate if they diverge.

Note for design: the brief anticipated that nothing pinned `golangci-lint` in this repo.
That is not quite the situation — a pin exists, it is just in the CI workflow rather than
in a file `format-on-write.sh` can source. FR-5/FR-6 resolve this rather than leaving it
dangling.

### 4.3 `tools/verify.sh` (`process-parity.md` §3.4, §4 Harbormaster row)

This is the only component built from scratch rather than ported.

FR-7. Contract, identical to atlas's: a **flagless run that exits 0 means the branch may
be called done**. `--quick` and `--no-docker` also exit 0 on success but skip gates and
do NOT count as done. The script MUST make this distinction visible in its output, so a
verifier agent cannot mistake a `--quick` pass for a done branch.

FR-8. The flagless run MUST execute, in order:

*Backend* (cwd `apps/backend`):
1. `go test -race -count=1 ./...`
2. `go vet ./...`
3. `golangci-lint run`
4. `CGO_ENABLED=0 go build ./...`

*Frontend* (cwd `apps/frontend`):
5. `npm ci`
6. `npm run lint`
7. `npm run format`
8. `npm test`
9. `npm run build`

*Container* (cwd repo root):
10. `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .`

Plus the FR-6 toolchain drift check.

FR-9. Backend gates MUST be invoked as the literal `go` / `golangci-lint` commands
above, not by delegating to `apps/backend/Makefile`. The Makefile's `build` target uses
`-ldflags` and writes `bin/harbormaster`, which is not the same as `go build ./...`; the
Makefile remains a human convenience and is not the gate definition.

FR-10. `--no-docker` MUST skip step 10 and nothing else.

FR-11. `--quick` MUST skip:
- step 10 (buildx),
- the `-race` flag on step 1 (running `go test -count=1 ./...` instead),
- step 5 (`npm ci`) when `apps/frontend/node_modules` is already present.

All other gates MUST still run under `--quick`.

FR-12. `--quick` and `--no-docker` MUST be combinable.

FR-13. The following MUST NOT run in any of the three modes, being on-demand and not
per-PR:
- `HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...`
- `npm run test:e2e`

The script SHOULD mention them in `--help` so they are discoverable, and MAY expose an
opt-in flag for them, but they MUST NOT affect the flagless exit code.

FR-14. The script MUST NOT mutate the working tree. `npm run format` is
`prettier --check` (non-mutating) and `npm run lint` is bare `eslint .`; neither may be
switched to its writing variant.

FR-15. On failure the script MUST stop at the first failing gate and identify it by name,
so a verifier agent can return "the first failing block" without parsing full output.

FR-16. The script MUST be runnable from any directory within the repository, resolving
paths relative to the repository root rather than the caller's cwd.

FR-17. A companion `tools/verify_test.sh` SHOULD be provided covering at least flag
parsing, gate selection per mode, and the FR-15 first-failure behaviour, following the
pattern of atlas's `tools/verify_test.sh`.

### 4.4 Task tooling (`process-parity.md` §3.4)

FR-18. Port `tools/task-numbers.sh` and `tools/task-numbers_test.sh` from `$ATLAS/tools/`.
The ported script MUST allocate numbers collision-safely across `docs/tasks/`, every
`.worktrees/*/docs/tasks/`, and local `task-*` branches — matching the scan the
`/spec-task` command performs by hand today.

FR-19. Port `tools/task-brief.sh` from `$ATLAS/tools/`. `commit-boundary.sh` references
it in its guidance text and is only portable once it exists.

FR-20. `tools/task-numbers_test.sh` MUST pass in this repository.

FR-21. Both scripts MUST be adapted to Harbormaster's layout where they encode
repo-specific assumptions (task folder location, worktree root), and MUST NOT retain
atlas-specific paths.

### 4.5 Agents (`process-parity.md` §3.3)

FR-22. Create `.claude/agents/task-implementer.md` from `$ATLAS/.claude/agents/`:
one plan task, 120 tool-call budget with a `PARTIAL` hand-back, module-local build/test
only, brief-first discovery. "Module-local build/test" MUST be rebound to Harbormaster's
layout: a single Go module at `apps/backend` (no `go.work`) and a frontend at
`apps/frontend`.

FR-23. Create `.claude/agents/task-verifier.md`: runs the repo-wide verification gate
(`tools/verify.sh`) in its own clean context, returns `PASS` or the first failing block,
never edits.

FR-24. Create `.claude/agents/task-reviewer.md`: per-unit review of one commit range
against its brief, durable artifact plus verdict-first return, no recursive fan-out.

FR-25. Port `.claude/agents/service-documentation.md` and `.claude/commands/service-doc.md`.
Rebind "service" to Harbormaster's actual unit of documentation — this repo has one
backend service, not a `services/*` tree — rather than porting a `services/*` assumption
verbatim.

FR-26. The four existing agents MUST NOT be modified.

### 4.6 Commands (`process-parity.md` §3.6)

FR-27. Port `.claude/commands/fix-pr-bug.md` (Phase 5) as-is, adjusting only repo-specific
references.

FR-28. The existing phase commands (`spec-task`, `design-task`, `plan-task`,
`execute-task`) MUST be updated where they conflict with the new tooling — specifically,
`/spec-task`'s hand-rolled task-number scan SHOULD defer to `tools/task-numbers.sh`, and
`/execute-task` MUST dispatch the new agent trio rather than generic subagents.

### 4.7 Settings (`process-parity.md` §3.7)

FR-29. `.claude/settings.json` MUST gain `disableBundledSkills: true` and the full hook
wiring: `PreToolUse` (`Write|Edit`, `Agent`, `Bash`, `*`), `PostToolUse`
(`Write|Edit`, `*`, `Bash`), `SessionStart`, `UserPromptSubmit`.

FR-30. The existing `UserPromptSubmit` → `skill-activation-prompt.sh` wiring and the
`enabledPlugins` block MUST be preserved.

FR-31. The resulting hook set and events MUST match atlas's, differing only where §4
permits.

### 4.8 Owner documents (`process-parity.md` §3.5, §5.2)

FR-32. Port these ten documents into `docs/`:

| Document | Owns |
|---|---|
| `agent-dispatch.md` | Model pinning, fan-out vs. fork, handoff decision |
| `verification.md` | Gate failures, script/CI disagreement |
| `superpowers-integration.md` | Bare task numbers, skills outside a phase command |
| `review-protocol.md` | Dispatching a reviewer, writing up a review |
| `post-implementation.md` | Phase 5, `/fix-pr-bug` |
| `codemod-vs-agents.md` | Second implementer at the same transformation |
| `slice-first.md` | Reading a large document, diff, plan, or tool result |
| `tooling-conventions.md` | Long-running processes, mechanical repo facts, shell conventions |
| `git-workflow.md` | Committing, pushing, rebasing, stray `main` commits |
| `observability.md` | Deploy/runbook story (see below) |

`observability.md` is a conditional port under §5.2 and is included by decision:
Harbormaster has `deploy/docker`, `deploy/kubernetes`, and an existing `docs/operator/`
set, so it has a deploy story worth a runbook. It MUST be genericized away from atlas's
service topology.

FR-33. `docs/adding-a-new-service.md` MUST NOT be ported — Harbormaster has no
service-scaffolding story.

FR-34. Genericization per §5.2: replace atlas-specific illustrations (packet work, WZ
data, IDA, service opcodes, atlas `verify.sh` flag specifics) with Harbormaster
equivalents — MinIO admin API contracts, bucket/object/policy handling, the
backend/frontend split, this repo's `verify.sh` flags — or with a neutral example.
**A rule MUST NOT be deleted because its example does not transfer; a new example MUST be
found.**

FR-35. Every `docs/` link in the ported documents MUST resolve to a file that exists in
this repository.

### 4.9 `CLAUDE.md` restructure (`process-parity.md` §5.3)

FR-36. `CLAUDE.md` MUST be rewritten from prose narrative into the rule-list shape with
exactly these eight headings, in order:

`# Harbormaster`, `## Never do this`, `## Evidence & grounding`,
`## Development workflow`, `## Done means verified`, `## Dispatching agents`,
`## Handing off context`, `## Repository conventions`, `## Where the procedures live`.

FR-37. `## Where the procedures live` MUST be a trigger → owner-document table, and every
target file in it MUST exist.

FR-38. The current "Build & Verification" prose command set MUST move into
`tools/verify.sh` and be referenced from `## Done means verified`, not duplicated.

FR-39. The claim that the repository "is currently unscaffolded — only `README.md` exists"
is **stale** and MUST NOT be carried forward. `apps/backend` (single Go module, `cmd/`,
`internal/`, `migrations/`, `Makefile`) and `apps/frontend` (Vite/React/TS) both exist,
as do `deploy/docker`, `deploy/kubernetes`, `docs/architecture/`, and `docs/operator/`.
The rewritten overview MUST describe the actual layout.

FR-40. These existing rules MUST survive the rewrite rather than be lost in it:
- the four-phase development workflow and its worktree discipline;
- the artifact-location override (`docs/tasks/task-NNN-slug/`, not
  `docs/superpowers/{specs,plans}/`);
- fuzzy task-identifier resolution across `docs/tasks/` and `.worktrees/*/docs/tasks/`;
- "when asked to understand or plan something, do not start implementing";
- the design/plan output style rule (write the full document, do not walk sections
  interactively);
- code review before PR is mandatory;
- verification over memory for MinIO admin API contracts and configuration values;
- the code-review pattern and its three modular reviewer agents;
- the refactoring guidance on straightforward moves over re-exported type aliases and
  not breaking service boundaries.

## 5. API Surface

No HTTP API changes. The "API" of this task is three command-line contracts.

### `tools/verify.sh`

```
tools/verify.sh [--quick] [--no-docker] [--help]
```

| Invocation | Exit 0 means | Gates run |
|---|---|---|
| (flagless) | Branch may be called done | All ten + drift check |
| `--no-docker` | Gates passed; **not** done | All except buildx |
| `--quick` | Gates passed; **not** done | All except buildx, `-race`, and `npm ci` when `node_modules` present |
| `--quick --no-docker` | Gates passed; **not** done | Union of the two skips |
| `--help` | — | None; prints usage incl. the on-demand suites |

Non-zero exit: the first failing gate, named on stderr.

### `tools/task-numbers.sh`

Allocates and reports task numbers collision-safely across `docs/tasks/`,
`.worktrees/*/docs/tasks/`, and local `task-*` branches. Exact subcommand surface is
inherited from the atlas version; the port MUST NOT invent a different interface.

### `tools/task-brief.sh`

Emits the brief for a task, addressed by fuzzy identifier. Interface inherited from the
atlas version. Consumed by `commit-boundary.sh` and by `task-implementer`'s brief-first
discovery.

## 6. Data Model

No database entities, migrations, or persisted schema changes. The only new persistent
state is on-disk:

- `tools/toolchain.versions` — pinned tool versions, sourced by `format-on-write.sh` and
  asserted by `verify.sh`.
- Whatever transient per-agent counter file `turn-budget.sh` maintains; its location and
  format are inherited from atlas unchanged and MUST be git-ignored if it lands inside
  the repository.

## 7. Service Impact

Harbormaster is a single Go module plus a single frontend; there is no `services/*` tree.
Impact is confined to repository tooling and documentation.

| Area | Change |
|---|---|
| `.claude/hooks/` | +8 portable hooks, +1 rebound (`format-on-write.sh`); `skill-activation-prompt.*` untouched |
| `.claude/agents/` | +`task-implementer`, +`task-verifier`, +`task-reviewer`, +`service-documentation`; existing four untouched |
| `.claude/commands/` | +`fix-pr-bug`, +`service-doc`; phase commands updated per FR-28 |
| `.claude/settings.json` | Full hook wiring + `disableBundledSkills` |
| `tools/` | +`verify.sh` (+test), +`task-numbers.sh` (+test), +`task-brief.sh`, +`toolchain.versions` |
| `docs/` | +10 owner documents; `docs/architecture/`, `docs/operator/`, `docs/tasks/` untouched |
| `CLAUDE.md` | Full restructure |
| `apps/backend/` | **No changes.** Gate commands are read, not modified. |
| `apps/frontend/` | **No changes.** |
| `.github/workflows/` | **No changes.** Read by the FR-6 drift check only. |
| `deploy/` | **No changes.** `Dockerfile` path is read by the buildx gate. |

## 8. Non-Functional Requirements

NFR-1 (Performance). `tools/verify.sh --quick` SHOULD complete substantially faster than
the flagless run. Measured baseline on this branch, 2026-08-26: backend
`go test -race` ≈ 60s wall (dominated by `internal/auth` at 23s), frontend `npm test`
≈ 20s, `npm ci` and the two-platform buildx build being the other large costs. The three
`--quick` skips target exactly those.

NFR-2 (Correctness of the gate). `verify.sh` MUST NOT report success for a gate it did
not run. This is the failure mode the whole task exists to prevent.

NFR-3 (Idempotence). Repeated runs on an unchanged tree MUST produce the same verdict and
MUST leave the tree unmodified (`git status --porcelain` unchanged across a run).

NFR-4 (Portability). Scripts target the repository's existing shell conventions and MUST
run non-interactively. They MUST NOT assume a TTY, and MUST degrade with a clear message
rather than hanging when Docker is unavailable.

NFR-5 (Security). No credentials, tokens, or MinIO connection details may be introduced
into any script or document. `.gitleaks.toml` and `.trivyignore` remain authoritative and
unmodified.

NFR-6 (No absolute home paths). `block-home-paths-in-docs.sh` becomes active as part of
this task; all new and ported documentation MUST satisfy it — no literal home or absolute
paths under `docs/`. This applies to the ported owner documents themselves.

NFR-7 (Observability of the gate). Each gate MUST announce itself before running so a
partial transcript makes clear which gates ran and which did not.

## 9. Open Questions

1. **`npm run lint` strictness.** `npm run lint` is bare `eslint .`, which exits 0 with
   warnings; the tree currently has 7 warnings (0 errors), e.g.
   `src/features/users/EditPoliciesDialog.tsx:72` (`react-hooks/exhaustive-deps`).
   `verify.sh` MUST NOT add `--max-warnings 0`, which would make the gate red on arrival.
   Whether to fix the warnings and tighten the gate is out of scope for this task; flagged
   for a follow-up.
2. **`--quick` and `npm ci`.** Skipping `npm ci` when `node_modules` exists trades
   fidelity for speed: a stale `node_modules` could mask a lockfile problem. Design phase
   should decide whether `--quick` compares the lockfile mtime against `node_modules` or
   simply tests for directory presence.
3. **`turn-budget.sh` state location.** Whether its counter file lands inside the repo
   (and therefore needs a `.gitignore` entry) or under a user-level directory is
   determined by the atlas implementation; confirm on inspection during design.
4. **Phase-command coupling.** FR-28 changes `/spec-task` and `/execute-task`, which are
   the commands being used to run this very task. Design should decide whether those
   edits land last, to avoid changing the tooling mid-flight.
5. **`docs/process-parity.md` placement.** The spec and brief are currently untracked in
   the main repo and embedded in this task folder. Whether a copy also lands at
   `docs/process-parity.md` on the task branch is a design decision; §7 check 3's grep
   exempts that path by name, which suggests it is expected to exist there.

## 10. Acceptance Criteria

Mechanically checkable in this repository:

- [ ] AC-1. The eight §3.1 hook files exist in `.claude/hooks/` and
      `grep -l 'atlas-' .claude/hooks/*.sh` prints nothing.
- [ ] AC-2. `.claude/hooks/wait-loop-guard_test.sh` passes.
- [ ] AC-3. `.claude/hooks/format-on-write.sh` references `apps/frontend`, not
      `services/atlas-ui`, and sources `tools/toolchain.versions`.
- [ ] AC-4. `tools/toolchain.versions` exists and pins `golangci-lint` to the version in
      `.github/workflows/pr.yml`.
- [ ] AC-5. `tools/verify.sh`, `tools/task-numbers.sh`, and `tools/task-brief.sh` all
      exist and are executable (`process-parity.md` §7 check 2).
- [ ] AC-6. `tools/task-numbers_test.sh` passes.
- [ ] AC-7. Flagless `tools/verify.sh` exits 0 on this branch (§7 check 2).
- [ ] AC-8. `tools/verify.sh --quick` and `tools/verify.sh --no-docker` each exit 0 and
      each state in their output that the run does not count as done.
- [ ] AC-9. `tools/verify.sh --help` documents the flags and names the two excluded
      on-demand suites.
- [ ] AC-10. `git status --porcelain` is unchanged across a full `verify.sh` run (NFR-3).
- [ ] AC-11. Neither `HARBORMASTER_INTEGRATION=1 go test -tags=integration` nor
      `npm run test:e2e` runs in any of the three modes.
- [ ] AC-12. `.claude/agents/` contains `task-implementer.md`, `task-verifier.md`,
      `task-reviewer.md`, and `service-documentation.md`; the four pre-existing agent
      files are byte-unchanged.
- [ ] AC-13. `.claude/commands/` contains `fix-pr-bug.md` and `service-doc.md`.
- [ ] AC-14. `.claude/settings.json` sets `disableBundledSkills: true`, wires
      `PreToolUse` (`Write|Edit`, `Agent`, `Bash`, `*`), `PostToolUse` (`Write|Edit`,
      `*`, `Bash`), `SessionStart`, and `UserPromptSubmit`, and retains the
      `skill-activation-prompt` wiring and `enabledPlugins` (§7 check 4, Harbormaster
      side).
- [ ] AC-15. The ten §4.8 owner documents exist under `docs/` (§7 check 6, Harbormaster
      side).
- [ ] AC-16. Every `docs/` link in `CLAUDE.md` and in each ported owner document resolves
      to an existing file (§7 checks 5 and 6).
- [ ] AC-17. No ported document contains an atlas-only example that has no Harbormaster
      analogue, and no rule was dropped for want of an example (§5.2; reviewer-judged
      against the atlas originals).
- [ ] AC-18. `CLAUDE.md` carries exactly the eight §5.3 headings in order and ends with a
      `## Where the procedures live` table (§7 check 5, Harbormaster side).
- [ ] AC-19. `CLAUDE.md` contains no claim that the repository is unscaffolded, and every
      rule listed in FR-40 is present in the rewritten file.
- [ ] AC-20. This command prints nothing (§7 check 3):
      ```sh
      git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
        | grep -vxE 'docs/process-parity\.md'
      ```
- [ ] AC-21. No file under `apps/backend/` or `apps/frontend/` is modified by this task.
- [ ] AC-22. `docs/` contains no literal home or absolute paths (NFR-6), i.e.
      `block-home-paths-in-docs.sh` would accept every file this task adds.

Not evaluable from Harbormaster alone — must be **reported as such, not asserted**:

- [ ] AC-23. §7 check 1 (the eight hook files byte-identical across all four repos,
      pairwise) — report the Harbormaster-side hashes and state plainly that the pairwise
      comparison cannot be completed here.
- [ ] AC-24. §7 checks 4, 5, and 6 in their cross-repo form — report Harbormaster's side
      only.
