---
description: Phase 4 — invoke superpowers:subagent-driven-development to implement a planned task in its existing worktree
argument-hint: Task identifier — accepts "task-001-bucket-replication", "task-001", "001", or "1"
---

You are starting Phase 4 of the Harbormaster four-phase development workflow. Argument: **$ARGUMENTS**

## Process

### Step 1 — Resolve the task

Same fuzzy-match algorithm as `/design-task` Step 1:

1. Glob `docs/tasks/task-*` (main) and `.worktrees/*/docs/tasks/task-*` (sibling worktrees).
2. Match `$ARGUMENTS` against folder names — exact, number-only (`1`/`001`/`task-1`/`task-001`), or slug fragment.
3. Zero matches → ask for correction. Multiple matches → list and let the user pick.
4. If the task lives only on main with no worktree, stop and tell the user the task needs a worktree.
5. Resolve to `<worktree>/docs/tasks/<id>/`.

### Step 2 — Ensure we're in the right worktree

Run `pwd`. If it does NOT match `<worktree>`, `cd <worktree>` yourself and continue from there. Do NOT ask the user to re-run the command — per CLAUDE.md's "Worktree Discipline" rule, cd into the task worktree yourself.

Do NOT create a new worktree — the worktree was created by `/spec-task` and must be reused so phase artifacts stay co-located.

### Step 3 — Validate inputs

Confirm `<worktree>/docs/tasks/<id>/plan.md` AND `context.md` exist. If either is missing, tell the user to complete `/plan-task` first.

### Step 4 — Invoke subagent-driven-development

Use the Skill tool to invoke `superpowers:subagent-driven-development`. Pass:

- Plan path: `<worktree>/docs/tasks/<id>/plan.md`
- Context path: `<worktree>/docs/tasks/<id>/context.md`
- Project conventions: `<worktree>/CLAUDE.md`
- **Worktree absolute path** (`<worktree>`) for every dispatched subagent. Every
  Bash call is prefixed `cd <worktree> && ...`; post-commit, verify branch and
  toplevel; no destructive git ops; never `git add -A` or `git add .`.

#### Dispatch contract

- **Implementers are `subagent_type: task-implementer`, never
  `general-purpose`.** The generic dispatch carries no tool-call budget, so a
  task that is going badly burns a session instead of surfacing early.
- **Extract each task's brief with `tools/task-brief.sh`** rather than handing
  over `plan.md`:

  ```sh
  tools/task-brief.sh docs/tasks/<id>/plan.md <N>
  ```

  It writes `.superpowers/sdd/<plan-basename>/task-<N>-brief.md`. Give the
  implementer that path. Never tell an implementer to read the whole plan.
- **The gate runs in `task-verifier`, never inside an implementer.** Dispatch it
  (`model: haiku`) after the implementer reports, with the command to run —
  `tools/verify.sh --quick` per task. Quote its `VERIFY:` line, not just its
  exit code.
- **Per-task review is `task-reviewer`** (`model: sonnet`) over that task's
  commit range with the brief as the requirement. Its artifact goes to
  `docs/tasks/<id>/reviews/task-<N>.md` — never to `audit.md`, which belongs to
  the pre-PR reviewers.

#### Handling `PARTIAL`

`task-implementer` reports `PARTIAL` when it reaches its 120 tool-call cap with
work remaining. That is a correct, expected outcome, not a failure.

1. Read the implementer's report file — it names what is done and committed, what
   remains file by file, the exact next step, and the interfaces it defined.
2. Write a continuation brief from that report plus the remaining part of the
   original brief.
3. Dispatch a **fresh** `task-implementer` with the continuation brief and the
   same report file. Do not resume the capped agent.
4. Repeat until it reports `DONE`.

On `BLOCKED` or `NEEDS_CONTEXT`, supply what is missing and re-dispatch; do not
let the agent guess.

If the user explicitly requests inline mode this session (rare), invoke `superpowers:executing-plans` instead.

### Step 5 — On completion

After all plan tasks complete and verify via the `task-implementer` / `task-verifier` / `task-reviewer` trio, the chosen skill hands off to `superpowers:finishing-a-development-branch`. Honor that handoff. Then suggest:

> All plan tasks complete. Recommend running `superpowers:requesting-code-review` next, which dispatches the plan-adherence-reviewer agent (plus any guideline reviewers once they're defined).

## Important Rules

- The worktree was created by `/spec-task`. NEVER create a new one here.
- Never start implementation outside the task worktree.
- Follow plan steps exactly; stop and ask when blocked rather than guessing.
- Run the verification commands the plan specifies; don't claim completion based on assumption.
- Never dispatch `general-purpose` for an implementation task — that is what `task-implementer` is for.
