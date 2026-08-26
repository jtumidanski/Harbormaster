# Slice First; Escalate When Necessary

How to read a large artifact inside an agent or a controller.

**This is a default, not a prohibition.** If semantic correctness requires the
whole document, read the whole document. Under-reading and getting it wrong
costs a fix round, which is far more than any read it saved. The rule is only
that a whole-file read should be a *decision*, not the reflex opening move.
A source file you are about to edit is not a large reference document — read
it.

---

## The measurement

Cost is not bytes; it is bytes × the turns that re-read them. A result
entering at turn 5 of a 200-turn agent is re-billed ~195 times; the same
result at turn 190 is re-billed 10 times. That is why a large document read
early, as a discovery reflex, is far more expensive than the same document
read late, once you already know which part you need.

Two concrete cases from this task's own artifacts:

| Whole read | Bytes | Targeted slice | Bytes | Ratio |
|---|---|---|---|---|
| `docs/tasks/task-004-process-parity-harness/prd.md` | 26,283 | §4 "Functional Requirements" alone (`sed -n '97,339p'`) | 11,672 | ~2.3× |
| `docs/tasks/task-004-process-parity-harness/plan.md` | 122,650 | `tools/task-brief.sh` extract for Task 9 | 4,969 | ~24.7× |

The plan case is the sharper illustration: a controller that reads the whole
291-line-per-task plan once per task, instead of extracting the one task it
needs, pays that 24.7× multiplier on every one of the plan's tasks, at
whichever turn each task starts — nearly always long before the plan is
otherwise needed in full.

**The documents are not the problem.** A 122 KB plan with fifteen fully
specified tasks is the reason those tasks are executed consistently; a
thinner plan would be re-derived by each implementer at far greater cost.
Change the access pattern; leave the content complete.

---

## The rule

**For any file you expect to exceed ~20 KB, lead with a slice.**

Escalate to a full read when the slice is insufficient — and when you do, say
so in your report, because a document that is repeatedly escalated is a
document that needs restructuring.

This repo has no `doc-slice.sh`; the accessors are plain shell and the
harness-native `Read` parameters:

| Need | Command |
|---|---|
| A document's shape | `grep -n '^#' <path>` |
| The one section | `sed -n '120,180p' <path>` |
| A needle in an offloaded tool result | `grep -n -B4 -A12 '<needle>' <path>` |
| Harness-native | `Read` with `offset` / `limit` |

Each of these prints (or lets you infer from `wc -c <path>`) the source size,
so escalating to a full read is always one deliberate step, not a default.

## Where each case lands

| Situation | Slice-first move | Escalate when |
|---|---|---|
| One plan task out of a 122 KB `plan.md` | `tools/task-brief.sh <plan> <N> [outfile]` — the brief IS the extract | the task references a decision recorded elsewhere in the plan |
| Auditing many plan tasks (`plan-adherence-reviewer`) | one `task-brief.sh` extract per task under audit | never read the whole plan once per task under review |
| A reference doc (this document, `tooling-conventions.md`, a PRD) | `grep -n '^#'` for the outline, then `sed -n 'A,Bp'` for the named section | the section cross-references another section you have not read |
| A large audit/result table (e.g. `docs/tasks/<task>/audit.md`) | `grep -n '<row key>'` for the rows in scope | a row's meaning depends on the document's preamble — read the preamble once, then slice rows |
| A review diff | `git diff --stat <range>` first, then `git diff <range> -- <file>` per flagged file | a change's correctness genuinely spans files |
| An offloaded large tool result spilled to disk | `grep -n -B/-A '<needle>'` or `sed -n` against the spilled file | you need the whole log, which is rare |
| A config or routing table (e.g. `.claude/skills/skill-rules.json`) | `grep -n` for the entries you need | you are changing its structure, not one entry |
| Source code | targeted `grep -n` / `sed -n` for the symbol, then read the file that owns it | reading a file you are about to edit — do read it |

That last row matters: **targeted read slices are not the problem.** Choosing
what to look at with `grep -n` / `sed -n` is semantic work, done well. This
document is about whole-document reads as a discovery reflex, not about
reading less.

## Front-load the cheap thing, not the expensive one

If you need both an inventory and a detail, take the inventory first:
`git diff --stat` before hunks, `grep -n '^#'` before `sed -n`, `ls` before
`Read`. The inventory is small and tells you which expensive read is
actually required — and it arrives at the position where a large result
would have been worst.

## What already works — do not "improve" it

Build and test output is consistently bounded — `go build ./...`,
`go test -race -count=1 ./...`, and `tools/verify.sh`'s own gate output are
short pass/fail summaries with tails, not multi-megabyte dumps, precisely
because that is what a CI-shaped gate is supposed to produce. The tool-result
spill mechanism works too — large payloads land as small stubs and get sliced
from disk. Both are the model this document asks documentation reads to
copy, not costs to trim.
