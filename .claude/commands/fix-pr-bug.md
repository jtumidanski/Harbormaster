---
description: Phase 5 — diagnose a post-implementation bug into a durable file, then fix it in a fresh context
argument-hint: Task identifier plus a short bug slug — e.g. "004 upload-progress-stall"
---

You are running Phase 5 of the Harbormaster workflow: a bug found after
implementation — in PR validation, live testing, or regression — fixed without
carrying a debugging conversation into the fix.

Argument: **$ARGUMENTS** (a task identifier, then a short bug slug).

The full rationale and the loop this command mechanizes live in
[`docs/post-implementation.md`](../../docs/post-implementation.md). Read it if
anything below is ambiguous.

## Step 1 — Resolve the task and load the facts

Resolve the task by fuzzy identifier the way the phase commands do — glob
`docs/tasks/task-*` and `.worktrees/*/docs/tasks/task-*`, match on exact
name, bare number, or slug fragment. Then read, in one pass: the worktree
path (`git worktree list`), the branch (`git branch --show-current`), which
of `prd.md` / `design.md` / `plan.md` / `context.md` / `audit.md` exist, and
the changed surfaces (`git diff --name-only main...HEAD`).

`cd` into the worktree yourself if `pwd` does not match. Do not create a
worktree.

## Step 2 — Reproduce, in your own context

Reproduction is interactive and stays here. Confirm the MinIO deployment and
the exact Harbormaster version first. Read the service logs before anything
else — `docker compose -f deploy/docker/docker-compose.yml logs harbormaster`
for the Compose stack, or `kubectl logs deploy/harbormaster` for the
Kubernetes one. See [`docs/observability.md`](../../docs/observability.md).
Read the logs for the workload you name, never a whole-namespace listing.

If a prior `docs/tasks/<task>/bug-*.md` already describes this symptom, read
that instead of reproducing from scratch.

## Step 3 — Write the diagnosis to disk

Write `docs/tasks/<task>/bug-<slug>.md` using the template in
`docs/post-implementation.md`: reproduced / observed / expected / root cause,
then a `## Fix` file inventory, then `## Not yet answered`.

**Write it before dispatching anything.** This file is the boundary — after it,
everything must be resumable from repository state plus this file. If the root
cause is not established, say so and name what is ruled out; do not guess one.

The `## Fix` inventory is what removes the fix agent's discovery phase. You
already know the paths from reproducing; without them the agent pays to
rediscover what you just found, at its own context depth.

## Step 4 — Dispatch a fresh implementer

```text
subagent_type: task-implementer
model: sonnet
```

Brief: the bug file path. Add only what the file cannot carry — the worktree
absolute path, and any ruling you made after writing it. Do not restate the
file, and do not restate the agent's own contracts.

On `PARTIAL`, follow the continuation rule in
[`docs/post-implementation.md`](../../docs/post-implementation.md) §3:
continuation brief, fresh agent, same report file.

## Step 5 — Verify in a clean context

Launch the gate for the fix range and **keep going** — do not poll it:

```sh
tools/verify.sh --quick > /tmp/gate-<slug>.log 2>&1
```

with `run_in_background: true`, or dispatch `task-verifier` (`model: haiku`)
for a summarized verdict.

If the fix crosses the backend/frontend seam or changes a JSON:API contract,
also dispatch `task-reviewer` (`model: sonnet`) against the fix range with the
bug file as the requirement — the gate cannot see a seam defect.

## Step 6 — Record the outcome in the bug file

There is no separate ledger in this repository; the bug file is the record.
Append to `docs/tasks/<task>/bug-<slug>.md`: the commit that fixed it, the
agents dispatched and their statuses, the verifier's `VERIFY:` line, any
reviewer verdict, and whether a live re-test confirmed the fix. Commit that
update. A bug file that never records its resolution is the next session's
rediscovery.

## Step 7 — Decide whether to continue here

Ask the handoff question: does the next bug depend on this conversation, or only
on repository state? An unrelated bug is a fresh unit — run `/fix-pr-bug` again
against its own bug file rather than accumulating.

Past ~150k context, stop starting new investigations in this session: write the
remaining leads into the task folder and say so as your final output. A handoff
the same context then works past is not a handoff.

## Important rules

- Never reproduce inside a subagent; never implement the fix inside this context.
- Never dispatch without a bug file on disk.
- Never poll a background process — `.claude/hooks/wait-loop-guard.sh` refuses it.
- Never commit to `main`; the fix lands on the task branch.
- Never claim the bug fixed without the gate verdict and, where applicable, a
  live re-test.
