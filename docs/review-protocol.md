# Review Protocol

This document owns what a reviewer **returns to its controller**, what it writes
**to disk instead**, and **which file** it writes to. It applies to every review
dispatch in this repo — `task-reviewer`, the two guideline reviewers, the
plan-adherence reviewer, and any ad-hoc per-unit code review.

It does not change what reviewers look for, how adversarial they are, or their
scope. Each agent's own `## Scope` section and the audit checklists remain the
contract for *what* is reviewed. This is only the shape of the answer, and where
it lands.

---

## Who owns what

| Reviewer | Owns |
|---|---|
| `task-reviewer` | One unit of work — a commit range, a fix round, a bug fix — reviewed for correctness against the brief, plan task, or bug file it was meant to satisfy. Ad-hoc and mid-plan. |
| `backend-guidelines-reviewer` | The Go `DOM-*` / `SUB-*` / `SEC-*` checklists, sourced from the `backend-dev-guidelines` skill. Runs before a PR. |
| `frontend-guidelines-reviewer` | The React/TypeScript `FE-*` checklist, sourced from the `frontend-dev-guidelines` skill. Runs before a PR. |
| `plan-adherence-reviewer` | "Was every task in `plan.md` actually implemented" — with `file:line` evidence per task. Runs before a PR. |

All four are pinned to `sonnet` in their frontmatter. Review is reading against
a checklist; see [`docs/agent-dispatch.md`](agent-dispatch.md) §Model selection
for why that pin has no exceptions.

`task-reviewer` is not a substitute for the guideline reviewers, and they are
not a substitute for it: the checklists never ask "does this do what the brief
said", which is where the most common real finding lives.

## The artifact split — two paths, no collisions

| Reviewer | Artifact |
|---|---|
| `task-reviewer` (per unit, per fix round) | `docs/tasks/<task>/reviews/<unit>.md` |
| `plan-adherence-reviewer`, `backend-guidelines-reviewer`, `frontend-guidelines-reviewer` | `docs/tasks/<task>/audit.md` |

**A per-unit review must never write `audit.md`.** The three pre-PR reviewers
share that file and a per-unit review landing there would overwrite work it did
not do. Conversely a pre-PR audit does not scatter itself into `reviews/` —
`superpowers:requesting-code-review` dispatches those three in parallel and they
append to one file on purpose, so the pre-PR verdict is readable in one place.

If a dispatch does not name an artifact path, derive it from this table. If the
unit name would collide with an existing file under `reviews/`, suffix the fix
round (`<unit>-fix-2.md`) rather than overwriting — a re-review is a new
artifact, not an edit of the old one.

---

## Why this shape

A review's prose is consumed by the controller on the turn it arrives and then
carried as dead weight for every turn after. Only the verdict and the blocking
lines change what the controller does next; the reasoning belongs on disk, where
it stays readable without being re-billed.

Implementer and verifier returns are already the right shape and are explicitly
**not** changed by this document — `task-verifier`'s few-hundred-byte return,
which quotes the gate's terminal `VERIFY:` line and little else, is the
reference. Reviewers are the outlier this contract exists to fix.

---

## The contract

### Write the full review to a durable artifact — always

Before returning, write the complete reasoning to the artifact path above.
Everything below stays there and only there:

- every PASS and its `file:line` evidence
- every evidence table, enumeration, and checklist disposition
- every N/A and the trigger that settled it
- pasted command output, gate logs, test transcripts
- narration, false-positive dismissals, and how you arrived at a conclusion

This is not deletion. The reasoning must exist — it is what makes a review
auditable and greppable after the session ends, which is precisely what an
artifact-less review cannot offer. It simply does not belong in a context that
re-reads it on every subsequent turn.

### Return this block, verdict first

```text
verdict: APPROVED | APPROVED_WITH_FINDINGS | CHANGES_REQUIRED
artifact: <repo-relative path>
scope_confirmed: <what you actually reviewed>
blocking: <n>
  - <file:line> — <one sentence>
non_blocking: <n>
not_evaluable: <n>
```

`.claude/agents/task-reviewer.md` reproduces this block verbatim. If you change
it here, change it there in the same commit; the two drifting is itself a defect.

Rules — **the first two are the ones most often broken**:

1. **`verdict` is the first line.** A controller must be able to decide from the
   first token whether to read further. Do not open with narration, a
   false-positive dismissal, or "Now I have everything needed to write the
   report."
2. **Blocking findings are enumerated, not counted.** One line each: `file:line`
   plus one sentence. This is the one place detail stays inline, because a
   controller must be able to dispatch a fix agent without opening the artifact.
   A compressed blocking finding the controller misreads is the failure mode
   this rule exists to prevent — so if a finding genuinely needs two sentences,
   use two.
3. **Non-blocking and not-evaluable are counts only.** The detail is in the
   artifact. `not_evaluable` is never zero by omission: if you could not
   evaluate something within your scope, it is counted here and described in the
   artifact — it is never silently absorbed into a PASS.
4. **`scope_confirmed` is not padding.** Scope is the reviewer's contract, and
   it is the one fact a controller cannot recover from the artifact path without
   opening the file. State the diff range, commit range, package path, or task
   range you actually reviewed — and say so plainly if it differs from what you
   were asked to review.
5. **No PASS evidence in the return.** If a check passed, the count and the
   artifact carry it.
6. **A clean review is small.** Target ≤600 B for `APPROVED`; ≤1,200 B with
   blocking findings. These are targets, not truncation points — never drop a
   blocking finding to hit a byte count.

### Verdict semantics

| Verdict | Meaning | Controller's next action |
|---|---|---|
| `APPROVED` | No findings that change the code | Record it and move on. Do not read the artifact. |
| `APPROVED_WITH_FINDINGS` | Non-blocking findings only | Record it; read the artifact if the findings bear on upcoming work. |
| `CHANGES_REQUIRED` | At least one blocking finding | Dispatch a fix from the enumerated `blocking` lines. **Read the artifact** if a finding is not actionable as written. |

`APPROVED_WITH_FINDINGS` exists so a real concern is never suppressed to hit a
compact return. A reviewer that hides a concern to look clean has broken this
protocol far more seriously than one that returns 2 KB.

### Never approve on the strength of a green build

Verification and review are **different gates**. A green `tools/verify.sh` does
not mean the branch is correct. Every module can build, vet, test and bake clean
while the branch carries blocking defects, because each side is self-consistent
in isolation. The gate cannot see:

- a handler that stopped emitting a JSON:API attribute the frontend query still
  reads;
- a MinIO admin API call whose response shape changed under a client the tests
  stub;
- a test that pins the old behavior and therefore passes either way.

When a change crosses the `apps/backend` ⁄ `apps/frontend` seam, trace the
contract into its consumer by hand and check that a test asserts the **new**
contract, not the old silent drop. "The build is green" is never evidence in a
review artifact; a `file:line` is.

---

## The controller's side

- On `APPROVED`, do not read the artifact. That read is the counter-metric for
  this whole contract: if artifact reads rise by more than about one per review,
  the return is too tight and detail belongs back in it.
- On `CHANGES_REQUIRED`, the `blocking` lines are the fix brief. Read the
  artifact when a line is not actionable as written — that is the designed
  escalation, not a failure.
- Record the review where the task's other outcomes are recorded — the plan's
  `progress.md`, or the bug file for a Phase 5 fix. There is no ledger script
  here, so note four things by hand: the unit, the reviewer, the verdict, and
  **whether it caused a fix**. That last one is the only cheap way to learn
  afterwards whether reviews were load-bearing. One measured task produced 84
  reviews and exactly one explicit Critical; how many of the other 83 mattered
  is, today, unknowable.

---

## Worked example — a clean review

Full reasoning (12.2 KB: five PASS sections with `file:line` evidence, a
seven-row consumer enumeration, the checklist disposition for every family) goes
to the artifact. The return is:

```text
verdict: APPROVED
artifact: docs/tasks/task-004-process-parity-harness/reviews/task-11-owner-docs.md
scope_confirmed: 4 new files under docs/, commit 8c3736a..bcb5cf5
blocking: 0
non_blocking: 0
not_evaluable: 0
```

312 bytes against 3,370. Nothing was lost: every PASS justification is on disk,
and the controller's next action — mark the unit done — is unchanged.

## Worked example — a failed review

```text
verdict: CHANGES_REQUIRED
artifact: docs/tasks/task-004-process-parity-harness/audit.md
scope_confirmed: 9 changed packages under apps/backend and apps/frontend, commits 8c3736a..bcb5cf5
blocking: 2
  - apps/backend/internal/buckets/handler.go:212 — the retention-policy attribute the plan requires is never emitted; the branch that should set it returns early when the bucket has no lock configuration.
  - apps/frontend/src/features/policies/api.ts:88 — the response type still declares the pre-change attribute set, so a policy without `version` deserializes to undefined rather than failing loudly.
non_blocking: 3
not_evaluable: 1
```

Both findings are dispatchable as written; the controller never has to open the
12.2 KB artifact to act. The three non-blocking findings and the one
not-evaluable item are on disk, counted here so nothing is hidden. Note the two
artifact paths across the two examples — the per-unit review went to
`reviews/`, the pre-PR guideline audit to `audit.md`.

---

## Reviewers do not fan out

A reviewer answers its checklist itself. Do not dispatch child agents for
individual checklist questions. `task-reviewer` and the guideline reviewers
should be dispatched with a tool set that omits `Agent`, so this is structural
rather than advisory.

Measured: one `backend-guidelines-reviewer` dispatched six children — "does a
Dockerfile exist and is this service referenced in it", "is this constant
already defined in the shared package", and four similar. Together they cost
**4.32M billed input for 4,669 output tokens**; four of the six returned *fewer
than 40 output tokens*, and one made a single tool call and cost 52,857. Their
returns were maximally compact — the cost was the ~35k dispatch floor plus each
child's own context growth, spent on questions the parent could answer with a
path glob.

Worse, the same agent then had nothing to do while its async children ran and
burned **30 `Bash true` no-op turns** — 33% of its tool calls, ≈3.6M tokens,
≈36% of the entire agent — for zero information.
`.claude/hooks/wait-loop-guard.sh` now refuses those calls; this rule removes
the reason to make them.

The break-even, and when delegation *is* right, is in
[`docs/agent-dispatch.md`](agent-dispatch.md) §Inline vs delegate.

## Reading the thing under review

A review starts with `git diff --stat <range>`, not with hunks. For any artifact
over ~20 KB, take a slice before a whole read — see
[`docs/slice-first.md`](slice-first.md). Three review diffs of 53.2 / 39.7 /
34.3 KB were read whole in one measured session; the median >12 KB result lands
at 0.10 of an agent's stream, where it is re-billed on ~90% of that agent's
turns. Escalating to a full read when the slice is insufficient is expected; the
reflexive whole-file read is not.
