# Agent Dispatch

This document owns *how* to dispatch any agent — model, budget, isolation,
handoff — for every dispatch in every session, including ad-hoc ones outside the
four-phase workflow. [`docs/superpowers-integration.md`](superpowers-integration.md)
owns *which* command, agent, or skill to reach for in a given situation.

The agent roster this repository actually has, and the model each one carries in
its own frontmatter under `.claude/agents/`:

| Agent | Model | What it is for |
|---|---|---|
| `task-implementer` | `sonnet` | Implement one plan task in Phase 4, or one bug file in Phase 5 |
| `task-verifier` | `haiku` | Run the repo gate and quote its verdict |
| `task-reviewer` | `sonnet` | Per-unit / ad-hoc correctness review of one commit range against its brief |
| `service-documentation` | `sonnet` | Generate or refresh documentation for one area |
| `plan-adherence-reviewer` | `sonnet` | "Was every plan task actually implemented" |
| `backend-guidelines-reviewer` | `sonnet` | Go `DOM-*` / `SUB-*` / `SEC-*` audit |
| `frontend-guidelines-reviewer` | `sonnet` | React/TypeScript `FE-*` audit |
| `todo-scanner` | `haiku` | Whole-codebase TODO/FIXME sweep |

Those pins are read out of the agent files, not remembered. If you change one,
change it in the agent's frontmatter — that file is the source of truth and this
table follows it.

---

## Model selection

Pass an explicit `model` on **every** Agent/Task dispatch. Never rely on
inheritance: an unspecified model inherits the main-loop model (Opus), and an
Opus subagent turn costs several times a Sonnet one.

The pin is chosen by the **job the agent is doing**, not by its
`subagent_type`. The named agents above carry their pin in frontmatter, but an
ad-hoc `general-purpose` dispatch carrying a review prompt does not — that is
the hole this rule closes. If you find yourself writing a review prompt into a
`general-purpose` dispatch, use `task-reviewer` instead; it is the named home
for exactly that work.

| Job | Model | Notes |
|---|---|---|
| Review, verify, audit, re-review, whole-branch review | **`sonnet`** — always | No exceptions. Reviewing is reading against a checklist; Opus buys nothing and these run long |
| Scan, inventory, doc sweep, file-finding (`todo-scanner`) | `haiku` | Frontmatter pin |
| Run the verification gate (`task-verifier`) | `haiku` | Frontmatter pin; it runs one command and quotes the output |
| Implement a plan task (`task-implementer`) | `sonnet` | Default; frontmatter pin. Also carries the 120-call PARTIAL budget |
| Document an area (`service-documentation`) | `sonnet` | Frontmatter pin |
| Implement a plan task tagged `model: opus` in `plan.md` | `opus` | Opt-in only — see below; pass `model: opus` on the dispatch to override the frontmatter |

A plan task may be tagged `model: opus` in `plan.md` when it is genuinely
derivation-heavy: reverse-engineering an undocumented MinIO admin API response
shape from live output, or a change whose contract crosses the
`apps/backend` ⁄ `apps/frontend` seam in a way that has to be derived rather
than transcribed. `/plan-task` should apply that tag sparingly and justify it in
one line. Everything else — REST surfaces, JSON:API documents, handlers,
React Query hooks, forms, tests — runs Sonnet.

If an implementer comes back wrong twice on Sonnet, escalate that one task to
Opus and note it, rather than raising the default.

Never use Fable for background or review workflows.

## The implementer budget

The implementer budget is **120 tool calls, warned at 100**, counted by
`.claude/hooks/turn-budget.sh` and contracted in
`.claude/agents/task-implementer.md`. At the cap the implementer commits what
works and reports `PARTIAL`; the controller dispatches a continuation against
the same report file. The number is stated once, in the counting hook, and is
changed there only.

The cap is **binding**: `.claude/hooks/turn-budget-guard.sh` (PreToolUse) denies
subagent tool calls past CAP+5, with a narrow allowlist for the commands an
agent needs to land its work and hand back cleanly. It exempts the controller
entirely — a controller's budget is the dispatch loop, not a single task, and
denying it mid-plan would strand the whole run. The guard parses the cap out of
`turn-budget.sh` at runtime so the two cannot drift.

The underlying arithmetic: context grows with turn count and every turn re-reads
all of it, so one 600-turn agent costs far more than the same work split across
fresh contexts. Splitting is the designed outcome, not a failure. A `PARTIAL` at
the cap is a correct result.

Before dispatching a second implementer at the same templated transformation,
check whether an AST codemod is cheaper than the remaining manual dispatches —
see [docs/codemod-vs-agents.md](codemod-vs-agents.md).

## Verification split

Implementers never run `tools/verify.sh`, `-race`, `golangci-lint`, `npm ci`, or
the docker bake — a `--quick` run inside a 400k-token implementer costs a large
multiple of the same run in a clean 20k one, and its output is the biggest
avoidable consumer of an implementer's window. Implementers run module-local
`go build ./... && go test ./...` in `apps/backend`, or the targeted frontend
test, and nothing more. The repo gate belongs to `task-verifier`, in its own
clean context. See [docs/verification.md](verification.md).

Two facts about that gate a dispatcher must not get wrong:

- `tools/verify.sh` takes `--quick`, `--no-docker`, `--list`, and `--help`. It
  has **no** `--base` and does no change detection — it selects gates by flag,
  not by diff. An unknown flag exits 2.
- **`DONE` and `PARTIAL` both exit 0.** The answer is the terminal `VERIFY:`
  line, not the exit code. A verifier that reports "exit 0, so we're done" has
  not read the gate; a `PARTIAL` means gates were skipped and the branch is not
  done.

Run the gate backgrounded and keep going. Do not poll it —
`.claude/hooks/wait-loop-guard.sh` refuses the poll, and
[`docs/tooling-conventions.md`](tooling-conventions.md) has the procedure for
waiting on a process without burning turns.

## Inline vs delegate

Delegation is strongly preferred when it replaces a meaningful sequence of
expensive turns in an already-large context. It is a loss when it replaces one
or two cheap ones.

A fresh subagent carries a **~35–38k dispatch floor** before it has done
anything (measured: an agent's turn-1 input is 38,178 tokens; a whole two-turn
agent cost 52,857). So the decision is arithmetic, not conceptual:

> **If you can answer the question in roughly one or two targeted tool calls,
> answer it yourself. Break-even is about four to five turns of your own
> work** — a ~35k floor against a parent turn at 100–150k.

Decide on *expected turns and context*, not on how big the task sounds. "Audit
which handlers still return the old JSON:API attribute set" sounds like a
delegation and is a grep. "Fix this one-line bug" sounds trivial and is a fresh
implementer, because your own context is 300k.

What this rules out, with the measurement behind it. One
`backend-guidelines-reviewer` dispatched six children for checklist questions:

| Child | Turns | Billed input | Output tokens |
|---|---|---|---|
| Domain DOM/FILE checklist for one package | 20 | 2,009,604 | 2,997 |
| "Is this constant already in the shared constants package" | 10 | 713,475 | **25** |
| Orphan reconciliation severity assessment | 10 | 634,682 | **39** |
| Handler coverage audit | 12 | 576,718 | 1,581 |
| Hand-mirrored struct parity check | 6 | 335,081 | **22** |
| Transaction boundary audit | 2 | 52,857 | **5** |
| **Total** | 60 | **4.32M** | 4,669 |

Four of the six produced fewer than 40 output tokens. Their *returns* were
maximally compact — the cost was the floor plus each child's own context growth.
The last one made a single tool call and cost 52,857.

The parent then had nothing to do while its async children ran and emitted **30
`Bash true` no-op turns** — 33% of its tool calls, ≈3.6M tokens, ≈36% of the
whole agent, for zero information. `.claude/hooks/wait-loop-guard.sh` now
refuses those calls and the polling equivalents; this rule removes the reason to
make them.

**Reviewers do not fan out at all.** A reviewer answers its own checklist. See
[docs/review-protocol.md](review-protocol.md).

### Shrinking the floor itself

Two parts of the floor are ours, not the harness's:

- **The agent roster.** The custom-agent listing is delivered to every dispatch
  as an "Available agent types for the Agent tool" reminder and costs several
  thousand tokens before the agent has read anything. That figure is unmeasured
  in this repo — running `/context` here would give the current number; treat
  "several thousand" as a placeholder to replace, not a permanent hedge. Whether
  denying the tool also suppresses the listing is **inferred, not measured** —
  the emitting code has no named gate. Leaf agents should deny it regardless,
  because it enforces
  the no-fan-out rule structurally. Prefer a `tools:` allowlist that simply
  omits `Agent` — which is what `task-reviewer` (`tools: Read, Grep, Glob, Bash,
  Write`) and `task-implementer` (`tools: Read, Write, Edit, Bash, Grep, Glob`)
  already do — or `disallowedTools: [Agent]` where an agent must inherit MCP
  tools, since a `tools:` allowlist cannot express "everything plus MCP". No
  agent in this repository legitimately fans out.
- **Bundled skills.** Claude Code's built-in skills (`dataviz`, `claude-api`,
  `design`, `update-config`, …) are a few thousand tokens of listing this repo
  never invokes. `"disableBundledSkills": true` is already set in
  `.claude/settings.json` and removes them. Plugin skills (`superpowers:*`) and
  the phase commands are unaffected; the built-in `/code-review` skill goes with
  them, and repo code review runs through the reviewer agents anyway.

**Never idle waiting on a child.** Agent completions arrive as notifications —
do other work, or end the turn and be re-invoked. There is no wait primitive
because none is needed.

## Fan-out vs fork

Fan out with **fresh-context agents, not `subagent_type: "fork"`** — a named
agent type plus an explicit brief. A fork inherits the parent's entire
conversation and re-reads it on every turn; nothing at the call site hints at
that, and it needs no brief, which is exactly what makes it the tempting choice
mid-task. Fork only to continue an interactive debugging thread whose brief
would be longer than the context it saves, and say why inline.

`.claude/hooks/fork-dispatch-guard.sh` (PreToolUse) **denies** a fork dispatch
that carries no justification and states the cost. To justify one, retry with a
line beginning `FORK-JUSTIFIED:` in the prompt, stating what the child needs
from this conversation that a brief cannot carry. The asymmetry is the point:
the reflexive fork is blocked, the considered fork costs one sentence.

### What a fan-out unit looks like here

The recurring Harbormaster shape is a change that touches both
`apps/backend/internal/<domain>` and the matching
`apps/frontend/src/features/<name>` — buckets, policies, users, lifecycle,
service accounts. That is **two units with one seam, not one unit**: one
implementer per side, each with its own brief and its own module-local build,
because a single implementer holding both halves pays Go and TypeScript
discovery in the same context and then re-reads both for the rest of its run.

The seam is what `task-reviewer` is for. Neither implementer can see it — the
backend agent never opens the query hook that consumes its handler, and the
frontend agent never opens the handler whose JSON:API attribute set it is
typing against. Dispatch `task-reviewer` over the pair once both land, with the
seam named explicitly in the brief. `tools/verify.sh` cannot see it either:
both sides can build, vet, test and bake clean while the frontend still reads
an attribute the backend stopped emitting.

## Context handoff

The unit of work is a **briefable task, not a conversation.** Context cost
scales with turn count × context size, so 50 turns carried at 190k cost roughly
ten times the same 50 turns at 19k — regardless of what they accomplish.

At every durable boundary — a commit landing, a verification gate returning, a
fan-out of agents reporting — the decision criterion is:

> **Does the next unit of work depend materially on this conversation's
> history, or only on repository state?**

If it can be resumed from repo state + the task's own reports + a short written
diagnosis, hand off.

Handing off means delegating, not clearing — `/clear` is a user action, and no
agent can clear itself. The diagnosis is written down *before* the handoff, not
carried in your head: one paragraph into the task folder, so the handoff is
lossless even though the reasoning does not survive in conversation.

**The floor.** Below roughly 60k tokens a fresh agent re-discovers files you
already hold, and you pay for that discovery twice; under ~40 tool calls, prefer
continuing. `.claude/hooks/commit-boundary.sh` encodes this floor (`FLOOR=40`)
and raises the question once per commit past it.

**The backstop.** ~150k tokens for a controller, or 4 completed plan tasks in
one session, whichever comes first — the one context that lives for a whole
plan, where every wake-up re-reads it, and the second trigger exists for a
controller that cannot read its own context size. `commit-boundary.sh` expresses
the same threshold in the only unit it can observe (`ESCALATE=60` tool calls,
from a measured ~2.1k tokens of controller context growth per call over a 23k
standing-prompt floor).

Apply the ceiling unconditionally: past the threshold, the controller does not
start another plan task, however many remain — a carve-out for "only a couple
left" is exactly the shape of the failure below. Measured on a real 18-task run:
the controller finished at 402k tokens having produced only 165KB of its own
tool output across 157 calls. Its last 42 turns — a self-contained segment
sharing no state with the preceding tasks — ran at 360–400k each; in a fresh
session those same turns would have run at ~80k. A second run wrote a handoff
marker at 243k tokens and then ran 26 more turns at an average of 259k (6.73M
tokens) to finish one more plan task anyway; all 17 sessions in that run ended
at their peak context. **A handoff the same context then works past is not a
handoff** — the marker on disk is meaningless if the session that wrote it keeps
going.

Generate briefs with `tools/task-brief.sh PLAN_FILE TASK_NUMBER [OUTFILE]`,
never by hand out of `plan.md` — assembling them by hand is exactly the context
bloat the brief exists to prevent. It exits 2 on a usage error and 3 when the
plan has no `Task <N>` heading, which is a real signal: it slices on
`^#+[ \t]+Task[ \t]+<N>([^0-9]|$)`, so a plan whose headings do not match that
pattern cannot be briefed at all. The durable artifacts that make a handoff
resumable are the per-task report file the implementer writes and the plan's
`progress.md` ledger under `.superpowers/sdd/<plan>/`.

Apply this shape in any session. Where a canonical ledger already exists, write
there rather than inventing a second artifact.

**The rule does not stop when implementation does.** PR validation, live
testing, debugging, regression investigation, and follow-up fixes are the same
question at the same boundaries — and they are where it was measured to be
ignored: one task's post-PR phase was 12.7% of its total spend at **94% main
thread**, three subagents across four sessions, peaking at 328k and 274k solo.
Against the execute phase's 19% main-thread share, that is the workflow's
largest single structural regression. The concrete loop — reproduce inline,
diagnose into `docs/tasks/<task>/bug-<slug>.md`, dispatch a fresh implementer
against that file, verify in a clean context — is
[docs/post-implementation.md](post-implementation.md), mechanized as
`/fix-pr-bug`.

## Recording what a dispatch cost

This repository has no ledger script. Record it by hand, in the artifact the
work already has, at reconcile time:

- **A plan task** — in the implementer's report file under
  `.superpowers/sdd/<plan>/task-N-report.md`, and one line in that plan's
  `progress.md`: the unit, the agent type, the model, the status
  (`DONE` / `DONE_WITH_CONCERNS` / `PARTIAL` / `BLOCKED` / `NEEDS_CONTEXT`), and
  the commit SHA.
- **A review** — the reviewer's own artifact carries the verdict; note in
  `progress.md` whether it *caused a fix*. That is the only cheap way to learn
  afterwards whether reviews were load-bearing. One measured task produced 84
  reviews and exactly one explicit Critical; how many of the other 83 mattered
  is, today, unknowable.
- **A bug fix** — in `docs/tasks/<task>/bug-<slug>.md` itself. See
  [docs/post-implementation.md](post-implementation.md).
- **A handoff** — write it where the next session will look: the task folder,
  with the context size at which you stopped. That marker is the only record
  anywhere of a handoff that was written and then worked past.

**Unknown is `-`, never a guess.** If the runtime does not hand you a turn count
or a byte size, leave the field out. Both cost audits behind this document were
reconstructed from transcripts by hand precisely because nothing aggregated
them; a fabricated number would be worse than the gap it fills.
