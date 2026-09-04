# Phase 5 — After Implementation

The four-phase flow ends at `/execute-task`. The work does not: PR validation,
live-environment testing, bug reproduction, regression investigation, and
follow-up fixes all happen afterwards, and nothing told those sessions to
delegate. This document is the missing phase, and it is the owner document for
`/fix-pr-bug` — the command mechanizes what is written here and links back to
it, so the two must agree.

It introduces no new context-clearing rule and requires nothing of the user. It
generalizes the handoff principle the execute loop already applies — see
[`docs/agent-dispatch.md`](agent-dispatch.md) §Context handoff — to the work
that follows implementation.

---

## Why

Measured across one task's post-PR phase — four sessions of live testing and bug
fixing:

| | Execute phase | Post-PR phase |
|---|---|---|
| Billed input | 800.0M (84.3% of the task) | **120.5M (12.7%)** |
| Main-thread share | 19% | **94%** |
| Subagents | 133 | **3** |
| Peak context | 349k across 8 sessions | 328k and 274k, solo |

Main-thread tokens are the expensive kind: the context grows monotonically and
every turn re-reads all of it. A turn at 200–330k costs several times the same
investigation inside a fresh subagent at 36–120k. Two of those sessions burned
76.7M between them with no subagents at all.

The habit was half-formed already. That same task folder contained four bug
diagnosis files, written and then acted on inline. **The artifact habit existed;
the delegation habit did not.** The one session that came closest to the right
shape opened by reading a bug file and dispatched two agents.

The rediscovery cost is visible too: one post-PR session re-grepped the repo for
a feature keyword to relocate code the same branch had written 48 hours earlier,
and another opened `plan.md` four times for 19.6 KB to re-establish what the
plan had said.

---

## The loop

**Reproduce inline. Diagnose into a file. Delegate the fix. Verify fresh.**

### 1. Reproduce — stay in your own context

Reproduction is interactive: an operator is in the loop with a live MinIO
deployment and a browser, round-trip latency matters more than tokens, and each
step depends on what the last one showed. Do this yourself. Do not delegate it.

Confirm the MinIO deployment and the exact Harbormaster version before anything
else; the wrong version sends the whole investigation down the wrong path — a
MinIO admin API response shape that differs across server releases will look
like a Harbormaster bug and is not one.

Read the service logs first:

```sh
docker compose -f deploy/docker/docker-compose.yml logs harbormaster
kubectl logs deploy/harbormaster
```

whichever matches the deployment under test. Read the logs for the workload you
name, never a whole-namespace listing. See
[`docs/observability.md`](observability.md).

If a prior `docs/tasks/<task>/bug-*.md` already describes this symptom, read
that instead of reproducing from scratch.

### 2. Write the diagnosis to `docs/tasks/<task>/bug-<slug>.md`

Before dispatching anything. **This file is the boundary** between the diagnosis
context and the fix context: everything after it must be resumable from
repository state plus this file.

```markdown
# bug: <one-line symptom>

**Reproduced:** <deployment, Harbormaster version, MinIO server version, exact steps>
**Observed:** <what happens, with the log line / HTTP response / error verbatim>
**Expected:** <what should happen, and where that is specified — PRD/FR, plan task>
**Root cause:** <what you established, with file:line>  — or: "not yet established; <what is ruled out>"

## Fix

- `path/to/file.go:120` — <what changes here>
- `path/to/other_test.go` — <the test that must fail before and pass after>

## Not yet answered

- <anything the fix agent must decide, and what it should do if unsure>
```

The `## Fix` section is a `### Files` inventory by another name, and it does the
same job: it removes the implementer's discovery phase — the phase that inflates
context before a single edit happens. You already know these paths from
reproducing; the fix agent would otherwise pay to rediscover them, at its own
context depth, on top of what you already paid at yours.

**If the root cause is not established, say so explicitly and name what is ruled
out. Do not guess one.** An honest "not yet established; the handler emits the
attribute, so the loss is downstream of `apps/backend`" is a fine brief. A
guessed root cause is worse than none, because the fix agent will implement
against it.

### 3. Delegate the fix to a fresh agent

```text
subagent_type: task-implementer
model: sonnet
brief: docs/tasks/<task>/bug-<slug>.md
```

The bug file is the brief. Do not restate it in the dispatch prompt — add only
what the file cannot carry: the worktree path, and any ruling you have made
since writing it. Do not restate the agent's own contracts either; its budget
and verification scope are in its definition.

This is the step the audit found missing. It is also where the saving is: the
fix agent starts near 36k instead of inheriting your 300k.

On `PARTIAL`, dispatch a continuation against the same bug file and the same
report file — not a fresh diagnosis.

### 4. Verify in a clean context

`task-verifier` (`model: haiku`) for the gate, or run it yourself backgrounded:

```sh
tools/verify.sh --quick > /tmp/gate-<slug>.log 2>&1
```

Launch it and **keep going** — do not poll it.
`.claude/hooks/wait-loop-guard.sh` will refuse the poll anyway.

Read the terminal `VERIFY:` line, not the exit code: `DONE` and `PARTIAL` both
exit 0, and `--quick` produces `PARTIAL` by construction because it skips gates.
`PARTIAL` does not count as done. See [`docs/verification.md`](verification.md).

`task-reviewer` (`model: sonnet`) as well, if the fix crosses the
`apps/backend` ⁄ `apps/frontend` seam or changes a JSON:API contract — the gate
cannot see a seam defect. Give it the bug file as the requirement. Its review
goes to `docs/tasks/<task>/reviews/<unit>.md`, never to `audit.md`; see
[`docs/review-protocol.md`](review-protocol.md).

### 5. Record the outcome in the bug file

There is no ledger script in this repository — **the bug file is the record.**
Append to `docs/tasks/<task>/bug-<slug>.md` and commit that update:

- the commit that fixed it
- the agents dispatched, their models, and their statuses
- the verifier's terminal `VERIFY:` line, quoted
- any reviewer verdict, and whether it caused a further fix
- whether a live re-test confirmed the fix

**A bug file that never records its resolution is the next session's
rediscovery.** The whole point of writing the diagnosis down was that the next
context can start from it; a file that stops at the symptom sends that context
back to the beginning.

---

## When to hand off your own context

The same question as every other durable boundary: **does the next unit depend
materially on this conversation's history, or only on repository state plus a
short written diagnosis?**

After a bug file is written, the answer is almost always "repository state".
Once you have written the diagnosis, the reproduction conversation that produced
it is no longer load-bearing.

Concretely, in a debugging session:

- **After each bug file is written and its fix dispatched**, ask the question.
  If the next bug is unrelated to the one you just fixed, it is a fresh unit —
  run `/fix-pr-bug` again against its own bug file rather than continuing to
  accumulate.
- **Past ~150k**, stop starting new investigations in this context. Write the
  remaining leads into the task folder and hand off. The controller ceiling from
  [`docs/agent-dispatch.md`](agent-dispatch.md) §Context handoff applies here for
  the same reason it applies there: **a handoff the same context then works past
  is not a handoff.** The marker on disk is meaningless if the session that
  wrote it keeps going.

`.claude/hooks/commit-boundary.sh` raises the question for you at each commit
past its floor, which in a fix loop is exactly the right moment.

`/fix-pr-bug` mechanizes steps 2–5 for a single bug.

---

## What does not change

- **Reproduction stays inline.** An over-delegated interactive debugging session
  is worse than an expensive one.
- **The bug-file habit is already right** — this document adds the delegation
  step after it, not a new artifact format.
- **The execute phase's ceiling is unchanged.** Phase 5 borrows it; it does not
  redefine it.
- **The gate and the review are still two different gates.** A green
  `tools/verify.sh --quick` closes neither the bug nor the review; only a live
  re-test closes the bug.
