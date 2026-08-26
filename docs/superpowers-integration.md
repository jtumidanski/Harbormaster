# Superpowers Integration — When to Use What

This document is the quick-reference companion to `CLAUDE.md`. It tells you
which command, agent, or skill to reach for in each situation, how a bare task
number resolves to a folder, and what changes when a skill is invoked outside a
phase command.

It owns *which* thing to reach for. [`docs/agent-dispatch.md`](agent-dispatch.md)
owns *how* to dispatch it — model, budget, isolation, handoff. The cross-repo
rationale for this workflow is [`docs/process-parity.md`](process-parity.md).

## The Five-Phase Workflow

| Phase | Command | What it does | Output |
|---|---|---|---|
| 1. Requirements | `/spec-task <idea>` | Interactive PRD interview; creates the worktree and branch | `docs/tasks/task-NNN-slug/prd.md` |
| 2. Design | `/design-task <task>` | Architecture, alternatives, tradeoffs (`superpowers:brainstorming`) | `design.md` |
| 3. Plan | `/plan-task <task>` | Bite-sized step-by-step plan (`superpowers:writing-plans`) | `plan.md` + `context.md` |
| 4. Execute | `/execute-task <task>` | Subagent-driven implementation (`superpowers:subagent-driven-development`) | code + commits |
| 5. After implementation | `/fix-pr-bug <task> <bug-slug>` | PR validation, live testing, debugging, follow-up fixes — diagnosis to a durable file, fix in a fresh context | `bug-<slug>.md` + fix commits |

Phases 1–4 each run from a fresh (`/clear`'d) session, so the next phase
consumes only the prior phase's documented artifacts. Phase 1 runs from the main
repo; every later phase runs inside the worktree `/spec-task` created at
`.worktrees/task-NNN-slug/`.

Phase 5 is not a `/clear` boundary and does not require one: it hands off by
writing a diagnosis and dispatching against it. It exists because the flow used
to stop at phase 4 while the work did not — one measured task spent 12.7% of its
entire budget after the PR opened, at 94% main thread with three subagents
across four sessions. See
[`docs/post-implementation.md`](post-implementation.md), which owns Phase 5.

Skip `/spec-task` only for trivial fixes that do not warrant a PRD; document
those directly via a brainstorming session.

### Plan task headings have a required shape

`tools/task-brief.sh PLAN_FILE TASK_NUMBER [OUTFILE]` slices one task out of a
plan by matching `^#+[ \t]+Task[ \t]+<N>([^0-9]|$)`. So a plan task heading must
be `## Task 7 — <title>` or `### Task 7: <title>` — anything else (`## 7.`,
`## Step 7`, `## Task Seven`) cannot be sliced and the script exits 3 with "no
`Task <N>` heading". Exit 2 is a usage error. `/plan-task` writes headings in
that shape; if you hand-edit a plan, keep it.

This matters because the alternative to a generated brief is assembling one by
hand out of the whole `plan.md` — exactly the context bloat the brief exists to
prevent.

### Task resolution — bare numbers

Phase commands accept fuzzy task identifiers: `task-004-process-parity-harness`,
`task-004`, `004`, and `4` all resolve to the same folder.

There is no resolver script in this repository; the phase commands do the match
themselves, and so should you when working outside one:

1. `tools/task-numbers.sh list` prints `NNN <task-id> <source>` for every
   assignment it can see, deduplicated, including task branches that exist but
   are not checked out. Start here — it answers "which numbers exist" without
   touching the filesystem tree.
2. Match your identifier against those task IDs — exact name, bare number
   (`4` / `004` / `task-4` / `task-004`), or slug fragment.
3. Resolve to the folder: `docs/tasks/<task-id>/` in the main repo, or
   `.worktrees/<task-id>/docs/tasks/<task-id>/` in its worktree. `git worktree
   list` tells you which worktrees exist.
4. Zero matches → ask for a correction. Multiple matches → list the candidates
   and let the user pick. Never guess between two.

**Glob narrowly.** `.worktrees/*/docs/tasks/task-*` is the pattern the phase
commands use and it is correct for finding a task, but every worktree carries a
full copy of `docs/tasks/` from its branch point, so the raw result is
(tasks × worktrees) mostly-duplicate paths. Deduplicate by task ID before
reading anything, and never expand that glob just to answer "does task N exist"
— `tools/task-numbers.sh list` answers that in one call.

When searching for a task artifact and coming up empty: search across all
worktrees before concluding a file is missing.

### Task numbers are allocated, never chosen

`tools/task-numbers.sh next` prints the smallest unused 3-digit `NNN`.
`/spec-task` must call it. Never pick a number by eye off a directory listing —
a number in use on an unmerged branch is invisible there.

`tools/task-numbers.sh check` exits 1 and reports on stderr if any `NNN` has
more than one distinct task ID. It runs automatically on every session start via
the `SessionStart` hook `.claude/hooks/task-num-collision-detector.sh`, so a
collision surfaces at the start of the session that would otherwise deepen it.
If it fires, resolve the collision before doing anything else — renaming a task
folder after work has landed on its branch is far more expensive than renaming
it now.

### Artifact location override — the skills-outside-a-phase-command case

`superpowers:brainstorming` and `superpowers:writing-plans` default to
`docs/superpowers/specs/` and `docs/superpowers/plans/`. **In this project both
go under `docs/tasks/task-NNN-slug/` instead** — `design.md` from brainstorming,
`plan.md` + `context.md` from writing-plans — so that the PRD, design, plan,
audit, reviews and bug files for one task are one directory and one unit with
the branch and the eventual PR.

`/design-task` and `/plan-task` apply that override for you. **When you invoke
either skill directly, outside a phase command, you must pass the task folder
explicitly** — otherwise the artifact lands in the skill's default location,
outside the worktree's task folder, and the next phase will not find it. The
same applies to any other skill that writes an artifact: name the destination in
the invocation rather than accepting a default.

The general rule: a phase command is a skill invocation plus this project's
bindings — the task folder, the worktree, the artifact paths, the model pins.
Invoking the skill bare drops the bindings, so you have to restate them.

### Phase 4 context budget

`task-implementer` replaces `general-purpose` for every Phase 4 implementation
dispatch. Its contracts override the plugin's generic implementer template where
they disagree.

The controls that keep implementer contexts small — the 120 tool-call cap, the
verification split, and the brief-first file inventory — are owned by
[`docs/agent-dispatch.md`](agent-dispatch.md).

## Code Review

Invoke `superpowers:requesting-code-review` after completing a logical chunk of
work, and always before opening a PR. The skill dispatches the relevant subset
of these agents in parallel:

- `plan-adherence-reviewer` — checks every task in `plan.md` was implemented;
  cites `file:line` evidence
- `backend-guidelines-reviewer` — adversarial Go audit against the applicable
  families (`DOM-*`, `SUB-*`, `SEC-*`) from the `backend-dev-guidelines` skill
- `frontend-guidelines-reviewer` — adversarial TS/React audit (`FE-*` checks)
  from the `frontend-dev-guidelines` skill
- `task-reviewer` — per-unit / ad-hoc correctness review of one commit range
  against its brief. This is the named home for what used to ride bare
  `general-purpose`; use it rather than dispatching `general-purpose` with a
  review prompt.

`/audit-plan` invokes `plan-adherence-reviewer` on its own. For any other
one-off check, invoke an agent directly by name without the orchestration skill.

### Picking the roster — do not derive it from memory

Classify the change first, then read the roster off the classification:

```sh
git diff --name-only main...HEAD | sed -E 's|^(apps/[^/]+)/.*|\1|' | sort -u
```

- any `apps/backend/**/*.go` changed → `backend-guidelines-reviewer`
- any `apps/frontend/src/**` changed → `frontend-guidelines-reviewer`
- a `plan.md` exists for the task → `plan-adherence-reviewer`
- the change touches both `apps/` trees → also `task-reviewer` over the seam;
  see [`docs/agent-dispatch.md`](agent-dispatch.md) §Fan-out vs fork

**Pass the file list into each reviewer's dispatch brief**, rather than letting
each one rediscover it. One measured reviewer opened with a 13.6 KB
`git diff --stat` pair and carried it through all 83 of its turns, then spent
~12 later turns rediscovering whether a Dockerfile existed and whether config
keys had changed. The list is cheap to produce once and expensive to produce
eight times.

**The classification is additive and fails open.** It states what is
*definitely* in scope. A reviewer may add a family; a reviewer may **not** drop
one because your list omitted it. When you cannot classify the change — an
unresolvable merge base, a file in an unrecognised layout — say so and run the
review wide rather than narrow.

### What a reviewer returns, and where it writes

Every reviewer writes its full reasoning to a durable artifact and returns a
compact verdict-first block. The contract, the verdict semantics, the artifact
split between `reviews/<unit>.md` and `audit.md`, and the controller's read rule
are all in [`docs/review-protocol.md`](review-protocol.md), which owns them.

All reviewers are **scoped to the change under review**: the diff is the review
surface, repo surveying is off, and anything a reviewer could not evaluate
within that surface is counted and described rather than passed silently. Each
agent's own `## Scope` section is the contract — you do not need to restate it
in the dispatch prompt. Measured on a 67-file Go diff, this cost the same as an
unscoped review and returned a strict superset of its findings.

Code review is mandatory before opening a PR and is a **different gate** from
verification. See [`docs/verification.md`](verification.md) for what the gate
does cover, and `docs/review-protocol.md` §"Never approve on the strength of a
green build" for why green is not an approval.

## Maintenance Commands

| Command | What it does | Underlying agent |
|---|---|---|
| `/audit-plan <task>` | Verifies a plan was faithfully implemented | `plan-adherence-reviewer` |
| `/review-todos` | Whole-codebase TODO/FIXME scan; updates the repo TODO list | `todo-scanner` |
| `/service-doc <area>` | Generates/updates documentation for one area | `service-documentation` |
| `/fix-pr-bug <task> <slug>` | Phase 5 — diagnose to a file, fix in a fresh context | `task-implementer` |

## Domain Skills

These activate via the project hook (`.claude/hooks/skill-activation-prompt.py`,
wired in `.claude/settings.json`) when you mention relevant keywords or work on
relevant files. The file/intent triggers live in
`.claude/skills/skill-rules.json`.

- `backend-dev-guidelines` — Go service patterns for `apps/backend`
- `frontend-dev-guidelines` — React/TypeScript patterns for `apps/frontend`

The hook produces a visible skill-activation banner. Heed it before responding.
These are also the source of the `DOM-*` / `SUB-*` / `SEC-*` and `FE-*`
checklists the guideline reviewers run.

## Superpowers Skills (Self-Activating)

Reach for these explicitly when relevant; they also self-activate via Claude's
native skill matching. Remember the artifact-location override above for the two
that write files.

- `using-superpowers` — invoke at the start of any conversation
- `brainstorming` — used inside `/design-task`; writes to the task folder
- `writing-plans` — used inside `/plan-task`; writes to the task folder
- `subagent-driven-development` — used inside `/execute-task`
- `executing-plans` — fallback for inline execution
- `systematic-debugging` — for any bug, test failure, or unexpected behavior
- `test-driven-development` — when implementing any feature or bugfix
- `verification-before-completion` — before claiming work is complete
- `using-git-worktrees` — for isolated workspaces
- `finishing-a-development-branch` — when implementation is complete and the
  gate is green
- `requesting-code-review` — at the end of a chunk of work
- `receiving-code-review` — when processing review feedback
- `dispatching-parallel-agents` — used by code-review orchestration
- `writing-skills` — when authoring new skills

Claude Code's bundled skills are disabled repo-wide
(`"disableBundledSkills": true` in `.claude/settings.json`); the plugin skills
above and the phase commands are unaffected. See
[`docs/agent-dispatch.md`](agent-dispatch.md) §Shrinking the floor itself.

## When NOT to Use the Workflow

- **Trivial fixes** (typo, version bump, one-line change) — no workflow needed;
  branch and commit directly, per [`docs/git-workflow.md`](git-workflow.md).
- **Documentation-only updates** that do not need a PRD — go straight to
  editing. They still need a branch and still go through a PR.

Neither exemption waives the gate or the review before a PR.

## File Locations Cheat Sheet

| Artifact | Location |
|---|---|
| PRD, design, plan, context | `docs/tasks/task-NNN-slug/` |
| Pre-PR audits (plan adherence, backend, frontend) | `docs/tasks/task-NNN-slug/audit.md` |
| Per-unit reviews | `docs/tasks/task-NNN-slug/reviews/<unit>.md` |
| Phase 5 bug diagnoses | `docs/tasks/task-NNN-slug/bug-<slug>.md` |
| Generated task briefs and per-task reports | `.superpowers/sdd/<plan>/` |
| Backend source | `apps/backend` (one Go module; no `go.work`) |
| Frontend source | `apps/frontend` |
| Deployment | `deploy/docker`, `deploy/kubernetes` |
