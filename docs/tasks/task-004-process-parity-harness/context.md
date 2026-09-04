# Context — task-004-process-parity-harness

Everything an implementer needs that is not in `plan.md`. Facts below were
verified in this worktree on 2026-08-26; nothing here is recalled.

---

## 1. Where things are

| Thing | Path |
|---|---|
| This worktree | `/home/tumidanski/source/Harbormaster/.worktrees/task-004-process-parity-harness` |
| Branch | `task-004-process-parity-harness` |
| Main repo | `/home/tumidanski/source/Harbormaster` |
| **`$ATLAS`** — the source worktree, read-only | `/home/tumidanski/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename` |

`$ATLAS` is **read-only for this task**. Never write to it. Never commit that
absolute path into any file under `docs/` — `block-home-paths-in-docs.sh` goes
live in plan Task 6 and will reject it. Write `$ATLAS` as a literal placeholder.

Confirm `$ATLAS` before the first copy:

```sh
diff "$ATLAS/docs/process-parity.md" \
     docs/tasks/task-004-process-parity-harness/process-parity.md
```

The PRD's provenance note says this repo's copy was re-synced at atlas commit
`e83f59e61`, one commit past the `e75c2a168` pin the brief names, and that the
delta was a single additive paragraph in §7 check 3. If the diff is non-empty for
any other reason, stop and re-sync rather than merging by hand.

---

## 2. Repository state this task starts from

| Fact | Value |
|---|---|
| `tools/` | contains only `licenses/`. All four new scripts are new files. |
| `.claude/hooks/` | `skill-activation-prompt.py`, `skill-activation-prompt.sh` only. Nine hooks to add, none to reconcile. |
| `.claude/agents/` | `backend-guidelines-reviewer.md`, `frontend-guidelines-reviewer.md`, `plan-adherence-reviewer.md`, `todo-scanner.md`. Four to add, these four untouched. |
| `.claude/commands/` | `audit-plan.md`, `design-task.md`, `execute-task.md`, `plan-task.md`, `review-todos.md`, `spec-task.md`. |
| `.claude/settings.json` | `UserPromptSubmit` → `skill-activation-prompt.sh`, plus `enabledPlugins`. No `disableBundledSkills`. Purely additive edit. |
| `docs/` | `architecture/overview.md`; `operator/{configuration,recovery,reverse-proxy,security}.md`; `tasks/task-001…004`. Owner docs land beside these, not inside them. |
| `docs/TODO.md` | **does not exist** in this repo. Do not link to it. |
| `.gitignore` | no `.cache/` entry, no `.superpowers/` entry. |
| `apps/backend` | one Go module, `go 1.25.12`, no `go.work`. `.golangci.yml` is at `apps/backend/.golangci.yml`, **not** at the repo root. |
| `apps/backend/internal/` | `apierror audit auth buckets config connection crypto dashboard db httpx integration jobs jsonapi lifecycle metrics minio objects observability policies server setup sse users` |
| `apps/backend/Makefile` | `lint/test/vet/build/tidy/run`. `build` uses `-ldflags` and writes `bin/harbormaster` — **not** the same as `go build ./...`. The Makefile is a human convenience, not the gate (FR-9). |
| `apps/frontend/package.json` | `lint: eslint .` · `format: prettier --check .` · `test: vitest run` · `build: tsc -b && vite build` · `test:e2e: playwright test`. `lint` and `format` are already non-mutating, satisfying FR-14 by construction. |
| `apps/frontend/node_modules/.package-lock.json` | exists, mtime newer than `package-lock.json`. A real staleness signal for the `--quick` `npm ci` skip. |
| `deploy/` | `docker/{Dockerfile,docker-compose.yml,caddy.example.Caddyfile,nginx.conf.example}`, `kubernetes/{deployment,service,pvc,ingress.example,secret.example}.yaml`, `kubernetes/README.md`. |

---

## 3. Toolchain facts

| Fact | Value | Source |
|---|---|---|
| `golangci-lint` CI pin | `v2.12.2` | `.github/workflows/pr.yml:49` — `with: { version: v2.12.2, working-directory: apps/backend }` (inline-flow mapping; the drift check must tolerate both flow and block style) |
| `golangci-lint` on `PATH` | **absent** | `verify.sh` bootstraps it into `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` |
| Node CI pin | `24.19.0`, three `setup-node` blocks | recorded in `toolchain.versions`, **not** asserted — three sites make a grep brittle |
| Local `node` / `npm` | v24.19.0 — matches the pin | no nvm bootstrap needed |
| Local `go` | 1.27.0; `apps/backend/go.mod` declares `go 1.25.12` | local toolchain satisfies the module |
| `docker` | `/usr/bin/docker` present | the buildx gate is runnable locally |
| Atlas's own `toolchain.versions` | pins `v2.13.1` | **do not copy that value.** Harbormaster's CI pins `v2.12.2`; copying atlas's would make gate 0 red on arrival. |

CI runs four things `verify.sh` deliberately does not: `gitleaks`, a Trivy
filesystem scan, a `go-licenses` allowlist check, and
`go vet -tags=integration ./...`. That asymmetry is recorded in
`docs/verification.md` with the rule **CI is the authority; the script is the
bug** (plan Task 10, Step 4).

---

## 4. Design decisions an implementer must not re-litigate

These were settled in `design.md`. If one looks wrong, raise it — do not silently
choose differently.

| Decision | Ruling | Where |
|---|---|---|
| `verify.sh` failure policy | **Stop at the first failing gate**, unlike atlas's run-everything-and-summarise. Eleven gates over one module; the first failure is almost always the whole story, and a verifier asked for "the first failing block" wants the run to have stopped there. | design D-1, §3.2 |
| Change detection | **None.** No `--base`, no `--all`. Would cost more in complexity than the seconds it saves. | design §3.2 |
| Dry-run flag | `--list` (ordered gate labels only), the seam `verify_test.sh` uses. It is the real selection path with the work removed — never a second description of it. | design §3.5 |
| Exit codes | All of `DONE` / `PARTIAL` exit 0. A distinct non-zero for `--quick` was **rejected**: it breaks FR-7 and makes `--quick` unusable in an `&&` chain, which is its entire purpose. The distinction lives in the `VERIFY:` line. | design §3.4 |
| `golangci-lint` provisioning | `verify.sh` owns the bootstrap, at the path `format-on-write.sh` already looks in. `format-on-write.sh` **never** bootstraps — it exits 0 silently on a cold cache, so a Write never stalls for a minute. | design D-2 |
| `--quick` and `npm ci` (PRD Q2) | Skip only when `node_modules/.package-lock.json` is not older than `package-lock.json`. Bare directory presence would let a stale tree mask a lockfile change. | design D-3 |
| Opt-in flag for the on-demand suites | **Not added**, though FR-13 permits it. It would be a fourth mode whose exit code means something different again, and both commands are one copy-paste away in `--help`. | design D-4 |
| `turn-budget` state (PRD Q3) | `${TMPDIR:-/tmp}/claude-turn-budget` — outside the repo. No `.gitignore` entry needed. | design D-7 |
| `task-brief.sh` workspace | `<root>/.superpowers/sdd/`, with a self-ignoring `.gitignore` the script writes itself. No `.gitignore` entry needed. | design D-7 |
| The only `.gitignore` addition | `.cache/` | design D-2, D-7 |
| `service-documentation`'s unit | A **component**: `apps/backend/internal/<name>`, or `apps/frontend/src/features/<name>`, or a whole app. Output at `docs/architecture/<name>.md`. Renaming the command to `/component-doc` was rejected — it breaks the cross-repo command-set parity §7 checks 4/6 assert, for a cosmetic gain. | design D-9 |
| Phase-command edits (PRD Q4) | **Land last** (plan Task 14). Editing a command mid-flight makes the tree inconsistent with the running session's behaviour. | design D-12 |
| `docs/process-parity.md` (PRD Q5) | **Committed at that path** as well as in the task folder. §7 check 3's grep exempts it by name; the ported owner docs link to it. | design D-13 |
| `npm run lint` strictness (PRD Q1) | **Out of scope.** 7 warnings / 0 errors today. `verify.sh` must not add `--max-warnings 0`; `verify_test.sh` asserts that flag is absent. Follow-up task. | design §8 |

---

## 5. The one conflict, and how it was resolved

`.claude/hooks/wait-loop-guard_test.sh` contains two allow-list fixtures that
name atlas:

```
allow 'kubectl get pods -n atlas-pr-1370'
allow 'journalctl -u atlas --since "5 min ago" | tail -50'
```

The first matches `atlas-`. So:

- **AC-1 / FR-1** — `grep -l 'atlas-' .claude/hooks/*.sh` must print nothing →
  the file must be edited.
- **`process-parity.md` §7 check 1** — the eight files byte-identical across all
  four repos → the file must not be edited.

Both cannot hold. **AC-1 wins.** Rebind the two strings to
`harbormaster-pr-1370` and `journalctl -u harbormaster`, change nothing else, and
report the divergence in `parity-report.md` rather than leaving it for whoever
runs check 1 to discover. `wait-loop-guard.sh` itself — the file whose
byte-identity actually matters — stays untouched, and FR-2/AC-2 still hold
because neither the namespace nor the unit name affects the guard's decision.

The other seven files are copied with `cp`: no reformatting, no shebang
normalisation, no "while I'm here" edits. A single whitespace change destroys the
property check 1 is measuring.

---

## 6. Source-file inventory in `$ATLAS`

**Hooks** (`$ATLAS/.claude/hooks/`): `wait-loop-guard.sh` (5.3K),
`wait-loop-guard_test.sh` (2.4K), `block-home-paths-in-docs.sh` (1.1K),
`turn-budget.sh` (4.9K), `turn-budget-guard.sh` (4.3K),
`fork-dispatch-guard.sh` (2.7K), `commit-boundary.sh` (6.6K),
`task-num-collision-detector.sh` (1.2K), `format-on-write.sh` (1.8K).

**Tools** (`$ATLAS/tools/`): `task-numbers.sh` (6.6K),
`task-numbers_test.sh` (4.2K), `task-brief.sh` (3.3K). `verify.sh` (31K) and
`verify_test.sh` (8.9K) are **reference for style only** — a different contract,
not a template. `lint.sh`, `plan-lint.sh`, `toolchain-pin-guard.sh` are **not**
ported.

**Agents** (`$ATLAS/.claude/agents/`): `task-implementer.md` (11.3K),
`task-verifier.md` (3.9K), `task-reviewer.md` (5.6K),
`service-documentation.md` (2.1K).

**Commands** (`$ATLAS/.claude/commands/`): `fix-pr-bug.md` (4.3K),
`service-doc.md` (474B). Atlas's `execute-task.md` is 18.9K and is **not**
wholesale ported — it encodes `go.work` fan-out, `libs/` change surfaces and
module-count heuristics that are false here.

**Docs** (`$ATLAS/docs/`): `verification.md` (19.4K), `agent-dispatch.md`
(12.6K), `superpowers-integration.md` (10.7K), `review-protocol.md` (7.9K),
`observability.md` (7.8K), `codemod-vs-agents.md` (7.0K),
`post-implementation.md` (6.1K), `slice-first.md` (5.2K),
`tooling-conventions.md` (4.5K), `git-workflow.md` (1.8K).
`adding-a-new-service.md`, `reverse-engineering.md` and `docs/packets/` are
**not** ported (FR-33 and PRD §2 non-goals).

---

## 7. Atlas tool paths that do not exist here

Twenty-two distinct `tools/*` paths appear in the ten owner documents. Only five
survive as-is (`verify.sh`, `verify_test.sh`, `task-brief.sh`, `task-numbers.sh`,
`toolchain.versions`). The rest need a per-reference decision — the policy table
is in `plan.md`'s "Owner documents" section, and the governing rule is FR-34:

> **A rule MUST NOT be deleted because its example does not transfer. A new
> example MUST be found.**

The single legitimate exception is a passage that is *pipeline mechanics* stating
no rule at all — `observability.md`'s spanmetrics, cardinality-budget and Grafana
sections. That judgement is checked in review (AC-17), so it must be argued
section by section in the implementer's report, not asserted.

Not ported and needing a manual-procedure restatement wherever a rule leaned on
them: `task-resolve.sh`, `task-facts.sh`, `agent-ledger.sh`, `change-surfaces.sh`,
`plan-context.sh`, `doc-slice.sh`, `lint.sh`, plus the atlas-domain guards
(`atlasguards`, `rediskeyguard`, `cideps`, `build-services.sh`,
`db-bootstrap.sh`, `gen-lb-ports.sh`, `go-analyzer-guards.sh`,
`service-registration-guard.sh`).

Atlas `docs/` links that will not resolve here and must be rebound or removed
with their sentence: `docs/packets/*`, `docs/adding-a-new-service.md`,
`docs/reverse-engineering.md`, `docs/runbooks/ephemeral-pr-deployments.md`, and
six specific atlas task folders.

---

## 8. Dependency order

```
1  toolchain.versions + .gitignore
      │
2  verify.sh + verify_test.sh ────────┐
      │                               │
3  real gates green (AC-7/8/9/10)     │
                                      │
4  task-numbers.sh, its test,         │   (independent of 2–3)
   task-brief.sh                      │
      │                               │
5  the nine hooks  ◄──────────────────┘   (format-on-write sources 1;
      │                                    collision detector needs 4)
6  settings.json wiring
      │   └─ activates block-home-paths-in-docs.sh, which then polices 9–13
      │
7  agent trio            ◄── cites verify.sh's flags (2)
8  service-documentation, /service-doc, /fix-pr-bug   ◄── dispatches 7
      │
9–12  the ten owner documents + docs/process-parity.md   ◄── cite 2, 7, 8
      │
13  CLAUDE.md rewrite    ◄── its trigger table's targets must exist (9–12)
      │
14  phase-command amendments   ◄── reference 2, 4, 7; land last by decision
      │
15  final assertion sweep + parity-report.md
```

Tasks 2–3 and 4 are independent and can run in parallel. Everything else is
sequential.

---

## 9. Traps

- **`git stash` is shared across worktrees.** Never bare `git stash` / `git stash
  pop` — another session may pop your entry, or you theirs. Use a temporary WIP
  commit, or `git stash push -u -m "<unique-tag>"` + `git stash apply <sha>`.
- **`docs/adding-a-new-service.md` must not exist** when this is done (FR-33).
  Plan Task 12 Step 5 asserts its absence.
- **AC-21 is absolute**: `git diff --name-only main...HEAD | grep -c '^apps/'`
  must print `0`. Gate commands are *read* from `apps/`, never written to. If a
  `verify.sh` gate fails against `apps/`, that is a finding to report — not a
  licence to edit application code.
- **`verify.sh` must not mutate the tree.** `git status --porcelain` byte-identical
  across a full run (AC-10, NFR-3). `.cache/` is gitignored by plan Task 1, which
  is what makes the linter bootstrap invisible to that check.
- **A `--quick` pass is not "done."** It prints `VERIFY: PARTIAL`. Quote the
  verdict line, not the exit code.
- **`--max-warnings 0` must never appear in `verify.sh`.** The tree has 7 eslint
  warnings and the gate would be red on arrival. `verify_test.sh` asserts its
  absence.
- **The two on-demand suites must not run in any mode.** A passing run cannot
  prove a command did *not* run, which is why `verify_test.sh` asserts textually
  that `HARBORMASTER_INTEGRATION` and `test:e2e` appear only in comments and in
  the `--help` heredoc.
- **Plan task headings must stay `### Task N:`** — `tools/task-brief.sh` slices on
  `^#+[ \t]+Task[ \t]+[0-9]+`, and `/execute-task` hands each implementer only
  its own sliced brief.
- **`skill-activation-prompt.{py,sh}` are untouched** (FR-3) and must remain wired
  at `UserPromptSubmit` (FR-30).
- **The four pre-existing agent files must be byte-unchanged** (FR-26 / AC-12).
  `git diff --stat main...HEAD` over those four paths must be empty.
- **AC-17 and AC-23/AC-24 are not self-certifiable.** AC-17 is reviewer-judged
  against the atlas originals; AC-23/AC-24 are cross-repo and must be *reported*
  as un-evaluable from here, with Harbormaster's side given, never asserted.
