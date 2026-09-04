# Codemod vs. agents — when a templated transformation earns a tool

This document owns one decision, taken at dispatch time: **you are about to
send a second implementer at the same mechanical, repeated change — should one
of them write a codemod instead?**

It exists because the upstream repository this process was ported from
(`atlas`) answered "no" without ever asking. A single templated
transformation there — task-232, batch 4 — consumed **6,231 implementer turns
/ 760M billed input tokens / 59% of that session's total spend**, and nobody
evaluated whether an AST rewrite would have been cheaper. Those figures are
atlas's measurements, quoted as the origin of the rule. Harbormaster has not
run a comparable transformation, so there are no local numbers to quote and
none are invented below.

This document is the rule, the worked example, and the specification for the
rewriter that would apply it. **The rewriter itself is not built** — see
"Current status" at the end.

## The rule

> Evaluate whether an AST codemod is cheaper **before dispatching the second
> implementer** at the same templated transformation.

"Templated transformation" means: the same multi-step edit, repeated across
call sites / files / packages, where most steps are syntactic (a rename, an
added import, a threaded parameter, a fixed call inserted at a fixed location)
and at most one step requires per-site judgment (a log message, an error
string, a domain-specific choice).

### The arithmetic that sets the trigger at the second dispatch

One figure is a standing contract in this repo, not a measurement: a
`task-implementer` dispatch is capped at **120 tool calls, warned at 100**,
before it must hand back `PARTIAL` — see
[`docs/agent-dispatch.md`](agent-dispatch.md) "The implementer budget". The cap
is enforced by `.claude/hooks/turn-budget-guard.sh`, so it is a real ceiling,
not an aspiration.

One figure is borrowed from atlas's measurement, because this repo has no
equivalent: 760,000,000 billed input tokens / 6,231 turns ≈ **122,010 tokens
per turn**. Treat it as an order-of-magnitude anchor for a long-running
implementer, not as a Harbormaster number.

The threshold follows from two separate steps, each with its own source:

1. **Why not evaluate at the first dispatch — a precondition, not a cost
   argument.** A single site cannot tell you a transformation is templated; it
   could be a one-off. The first implementer dispatch is what reveals the shape
   (the same edit is needed again elsewhere). There is nothing to evaluate
   before that.
2. **Why the second dispatch is the trigger — a cost argument.** Writing a
   codemod is itself exactly the implementer's shape of task — a small,
   self-contained module (four or five files: a manifest, the rewrite logic, a
   table-driven test, a CLI entry point, before/after fixtures) — so building
   and testing it once is bounded by the *same* 120-tool-call cap as any other
   implementer dispatch. A second manual dispatch at the same transformation is
   bounded by that identical cap, because it is the same kind of dispatch. So
   the second manual dispatch is the first point that both (a) confirms the
   transformation is templated (step 1) and (b) has not yet cost more than the
   rewriter itself would — one further manual dispatch already reaches the
   codemod's own worst-case build cost. That is the break-even: evaluate there,
   not later.

Every site the codemod covers beyond that point is a `--check`-verified
mechanical rewrite instead of another implementer turn.

Measured against what actually happened upstream: batch 4's 6,231 turns / 760M
tokens is **~52×** a single 120-call dispatch ceiling at ~122k tokens/turn
(≈14.6M tokens; 760M / 14.6M ≈ 52.1). That comparison shows the transformation
ran far past any plausible threshold — it does not by itself pin the threshold
at the second dispatch rather than the third or fourth. The two-step reasoning
above is what does that.

## The escape hatch — when it is not a codemod

A transformation whose *every* site needs judgment is not templated, and no
tool will help. The test is per-step, not per-site: count how many of the steps
are derivable from the syntax tree alone.

- **All steps syntactic** → codemod, no residue.
- **Most steps syntactic, one judgment step** → codemod for the syntactic
  steps, residue list for the judgment one. This is the common and most
  valuable case.
- **The shape differs per site** — different call signatures, different
  control flow, a decision about *whether* the change applies at all →
  dispatch implementers. Writing a codemod here means encoding the judgment in
  the tool, where it is harder to review than in a diff.

A change that looks templated but is spread over fewer than three sites also
fails the arithmetic: the codemod costs a dispatch and saves at most one.

## Worked example: a JSON:API attribute rename

The canonical Harbormaster candidate. Backend resources encode their attribute
block in snake_case through `internal/jsonapi.Encoder`, per-package in
`apps/backend/internal/<domain>/rest.go` (ten such files today: `audit`,
`buckets`, `connection`, `dashboard`, `lifecycle`, `metrics`, `objects`,
`policies`, `setup`, `users`), and the frontend mirrors each shape in
`apps/frontend/src/features/<domain>/types.ts` and `api.ts`. Renaming one
attribute across that surface is six steps:

1. The attribute key in the resource's `MarshalJSON` attributes block —
   **AST** (Go).
2. The matching struct field tag on the request type in the same file —
   **AST** (Go).
3. Every read of the old key elsewhere in the package — **AST** (Go),
   call-graph derivable.
4. The TypeScript type in `apps/frontend/src/features/<domain>/types.ts` —
   **AST** (TS).
5. Every property access on that type across the feature's components —
   **AST** (TS), derivable from the type.
6. The user-visible label, validation message, or column header that happened
   to be derived from the old name — **judgment**. Whether the rename should
   also change what a human reads on screen is a product decision, and the
   wording is written for its site.

Five of six steps are pure syntax; one is irreducibly judgment. That is the
split a rewriter formalizes: **rewrite what is derivable, list what is not, and
never silently skip a site.** A codemod covering steps 1–5 turns each site's
remaining work into "confirm or write one label," reviewable from a residue
list rather than dispatched as a full implementer turn per site.

Two more candidates with the same shape:

- **An error-wrapping convention change** across every `apps/backend/internal/*`
  package — the wrap call and its format string are syntactic; the message
  wording at a handful of sites is judgment.
- **A `prettier` or `eslint` rule change rippling through
  `apps/frontend/src`** — this one is the degenerate case, and the point of
  including it: the tool already exists. `prettier --write` and
  `eslint --fix` *are* the codemod. Dispatching an implementer to hand-apply a
  formatter's output is the failure this document is about, in its purest
  form. Reach for the existing fixer before writing a new one.

## The deferred rewriter's contract (specification only)

If a templated transformation clears the second-dispatch threshold, the
rewriter that gets written should follow this shape. This is a specification
for future work, not a description of an existing tool — nothing under `tools/`
implements it.

**Module layout.** `tools/` currently holds shell scripts and
`tools/toolchain.versions`; a Go rewriter would be the first Go module there,
and must be its own module so it does not enter `apps/backend`'s dependency
graph:

- `tools/<name>/go.mod` — own module, not part of `apps/backend`
- `tools/<name>/analyzer.go` — the AST rewrite logic
- `tools/<name>/analyzer_test.go` — table-driven tests over `testdata/`
- `tools/<name>/cmd/` — the CLI entry point
- `tools/<name>/testdata/` — before/after fixture pairs, built from the real
  diffs of the sites already migrated by hand

A TypeScript-side rewriter is a devDependency of `apps/frontend`, driven by the
TypeScript compiler API, with the same fixture-pair test shape. A
transformation that spans both languages needs both halves, or it produces a
half-migrated tree.

**Two contracts it must honor:**

- **Every site is rewritten or listed, never silently skipped.** A site the
  tool cannot safely rewrite — the judgment step, or any pattern it does not
  recognize — goes into a residue report with `file:line` and a reason. Silent
  omission is the failure mode that makes a codemod untrustworthy: a human has
  to be able to trust that "not in the residue list" means "rewritten," not
  "not looked at."
- **A `--check` mode, for use as a guard afterward.** The same analyzer, run in
  a mode that exits non-zero if any un-migrated site remains, becomes the
  regression guard once the migration lands — replacing a hand-maintained
  allowlist with a mechanical check. That is the same argument
  [`docs/verification.md`](verification.md) "Adding a gate" makes for every
  other invariant: a repo-specific rule belongs in a script, not in a
  reviewer's head. A landed `--check` mode is a `tools/verify.sh` gate.

## Current status — dormant

**No rewriter exists.** This document specifies what one would look like and
the threshold at which writing one pays for itself; it does not claim one is
available to run. Because no `--check` mode exists, no batch today can be
verified as codemod-applied, so every batch is treated as judgment-bearing and
gets the full per-task review described in
[`docs/review-protocol.md`](review-protocol.md). When a rewriter with a
`--check` mode lands, revisit that: a mechanically verified batch does not need
the same reading as a hand-written one.
