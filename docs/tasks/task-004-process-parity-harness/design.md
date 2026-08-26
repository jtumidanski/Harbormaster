# Process Parity Harness — Design

Task: `task-004-process-parity-harness`
Status: Draft for review
Inputs: `prd.md`, `process-parity.md` (§3–§7), `process-parity-brief.md`
Date: 2026-08-26

---

## 1. Framing

Ninety percent of this task is copying text between repositories. That part has no
architecture. The design decisions that matter are the ones where the atlas original
cannot be copied — either because Harbormaster's shape is different, or because a
mechanism the atlas file depends on does not exist here.

There are exactly six of those, and they are what this document is about:

1. **`tools/verify.sh` has no original.** It is built from a prose checklist, and the
   PRD's contract (fail-fast, three modes, "done" vs "not done") differs from atlas's
   verify.sh contract (run-everything, change-gated, `--base`/`--all`/`--facts`) in ways
   that make the atlas script a reference for *style*, not a template.
2. **`golangci-lint` is not installed on this machine and nothing in the repo provisions
   it.** Both `verify.sh` (AC-7) and the ported `format-on-write.sh` (AC-3) depend on it.
   Somebody has to bootstrap it, and where the binary lands is a contract between the two.
3. **The `-race`/`npm ci`/buildx tiering is a judgement call**, and `--quick` skipping
   `npm ci` is a correctness/speed trade the PRD explicitly hands to this phase.
4. **`verify_test.sh` cannot run the gates it is testing.** Testing gate *selection*
   without executing `go test -race` or a two-platform buildx needs a seam.
5. **The owner docs cite twenty-two atlas tool paths that will not exist here.** Rule
   preservation (FR-34: "a rule MUST NOT be deleted because its example does not
   transfer") requires a per-reference decision, not a find/replace.
6. **`grep -l 'atlas-' .claude/hooks/*.sh` (AC-1) and byte-identical hooks
   (`process-parity.md` §7 check 1) are in direct conflict** on one file. Verified below.

Everything else — the eight hooks, `task-numbers.sh`, `task-brief.sh`, the settings
wiring — is a copy, and §6 says so and moves on.

---

## 2. Verified current state

Every row below was checked in this worktree on 2026-08-26. Nothing here is recalled.

| Fact | Value | Consequence |
|---|---|---|
| `tools/` contents | `licenses/` only | All four new scripts are new files. |
| `.claude/hooks/` | `skill-activation-prompt.{py,sh}` only | 9 hooks to add; none to reconcile. |
| `.claude/settings.json` | `UserPromptSubmit` + `enabledPlugins`, no `disableBundledSkills` | Additive edit; nothing to remove. |
| `.claude/agents/` | the 4 named in the PRD | 4 to add; 4 untouched (AC-12). |
| `docs/` | `architecture/overview.md`, `operator/{configuration,recovery,reverse-proxy,security}.md`, `tasks/` | Owner docs land beside these, not inside them. |
| `golangci-lint` on `PATH` | **absent** | See D-2. Blocks AC-7 if unaddressed. |
| `go` | 1.27.0; `apps/backend/go.mod` says `go 1.25.12` | Local toolchain satisfies the module. |
| `node`/`npm` | v24.19.0, matching `pr.yml`'s pin | No nvm bootstrap needed (unlike MyFleet). |
| `docker` | `/usr/bin/docker` present | buildx gate is runnable locally. |
| `golangci-lint` pin in CI | `v2.12.2`, `.github/workflows/pr.yml:49` (`golangci-lint-action` `with.version`) | Drift check must parse *that* key, not a `run:` line. |
| Node pin in CI | `24.19.0`, three `setup-node` blocks | Recorded in `toolchain.versions`, not asserted (three sites). |
| `apps/frontend/package.json` | `lint: eslint .`, `format: prettier --check .`, `test: vitest run`, `build: tsc -b && vite build`, `test:e2e: playwright test` | `format` and `lint` are already non-mutating (FR-14 is satisfied by construction, but must be asserted — see D-1.6). |
| `apps/frontend/node_modules/.package-lock.json` | exists, mtime newer than `package-lock.json` | A real staleness signal exists for D-3. |
| `apps/backend/Makefile` | `lint/test/vet/build/tidy/run`; `build` uses `-ldflags -o bin/harbormaster` | Confirms FR-9: the Makefile is not the gate. |
| CI gates beyond the checklist | `gitleaks`, `trivy` fs scan, `go-licenses` allowlist, `go vet -tags=integration ./...` | See D-14. |
| `.gitignore` | no `.cache/`, no `.superpowers/` entry | D-2 adds one; D-7 needs none. |
| atlas `turn-budget*.sh` state dir | `${TMPDIR:-/tmp}/claude-turn-budget` | Open question 3 answered: outside the repo. |
| atlas `task-brief.sh` workspace | `<root>/.superpowers/sdd/` with a self-ignoring `.gitignore` it writes itself | No `.gitignore` edit needed. |
| atlas `task-numbers.sh` + its test | zero repo-specific paths; test builds throwaway repos in `mktemp -d` | FR-21 is a no-op for these two: verbatim port. |
| `atlas-` occurrences in the 8 portable hooks | **two**, both fixture strings in `wait-loop-guard_test.sh` (`kubectl get pods -n atlas-pr-1370`, `journalctl -u atlas ...`) | See D-6. |
| atlas `tools/*` cited in the 10 owner docs | 22 distinct paths, of which 4 will exist here | See D-10. |
| atlas `docs/*` links in the 10 owner docs | include `docs/packets/*`, `docs/adding-a-new-service.md`, `docs/runbooks/ephemeral-pr-deployments.md`, and six specific atlas task folders | All must be rebound or dropped (FR-35). |
| Harbormaster backend observability | `internal/observability/log`, `internal/metrics` (scrapes MinIO's own Prometheus endpoint via `prom2json`) — **no** OTel pipeline, no Grafana/Loki stack of its own | `observability.md` is a rewrite, not a port (D-10.3). |
| `.claude/commands/spec-task.md:52` | contains a literal absolute home-directory example path | Outside `docs/`, so `block-home-paths-in-docs.sh` does not fire; fixed opportunistically in D-12. |

---

## 3. `tools/verify.sh` — the one piece of real engineering

### 3.1 Contract

```
tools/verify.sh [--quick] [--no-docker] [--list] [--help]
```

| Mode | Exit 0 means | Skips |
|---|---|---|
| flagless | **the branch may be called done** | nothing |
| `--no-docker` | gates passed; NOT done | buildx |
| `--quick` | gates passed; NOT done | buildx, `-race`, `npm ci` (conditionally) |
| `--quick --no-docker` | gates passed; NOT done | union |
| `--list` | — | runs nothing; prints the gates the *same flags* would select |
| `--help` | — | usage, incl. the two excluded on-demand suites |

Non-zero: the first failing gate, named on stderr, and the script stops there.

### 3.2 Divergence from atlas's verify.sh, and why

This is deliberate and should not be read as an incomplete port.

| Dimension | atlas | Harbormaster | Reason |
|---|---|---|---|
| Failure policy | run every check, summarise at the end | **stop at the first failure** | FR-15. Atlas's 80-module fan-out makes a full picture worth the wall time; Harbormaster has ten gates and the first failure is almost always the whole story. A verifier agent returning "the first failing block" wants the run to have *stopped* there — otherwise it must parse which of several failures came first. |
| Change detection | `--base`/`--all`, path-gated guards | **none** | Ten gates over one Go module and one frontend. A change-detection layer here would cost more in complexity and failure modes than the seconds it saves, and `--quick` already covers the inner loop. This is the single largest simplification. |
| Dry-run | `--facts` (rich: modules, bake targets, fan-out reason) | `--list` (just the ordered gate labels) | Nothing to report but the gate list. Same anti-drift property (§3.5). |
| `--no-ui` | present | absent | Not in the PRD's flag surface, and the frontend gates are not the slow part here. |
| Colour/summary block | yes | yes, kept | Cheap, and NFR-7 wants each gate announced. |

Atlas's `step()`-registry structure, its `--facts`-is-the-real-run principle, and its
`ROOT="$(cd "$(dirname "$0")/.." && pwd)"` cwd resolution are all kept. The bodies are not.

### 3.3 Gate registry

One array of `(label, mode-predicate, command)` records, walked in order. `step()` is the
only place a gate label is emitted or a command is run — the property `verify_test.sh`
asserts structurally (§3.5).

| # | Label | cwd | Command | flagless | `--no-docker` | `--quick` |
|---|---|---|---|---|---|---|
| 0 | `toolchain drift` | root | compare `GOLANGCI_LINT_VERSION` in `tools/toolchain.versions` against `pr.yml` | ✔ | ✔ | ✔ |
| 1 | `go test -race` / `go test` | `apps/backend` | `go test -race -count=1 ./...` (flagless) / `go test -count=1 ./...` (`--quick`) | ✔ | ✔ | ✔ (no `-race`) |
| 2 | `go vet` | `apps/backend` | `go vet ./...` | ✔ | ✔ | ✔ |
| 3 | `golangci-lint` | `apps/backend` | `"$GOLANGCI" run` (D-2) | ✔ | ✔ | ✔ |
| 4 | `go build` | `apps/backend` | `CGO_ENABLED=0 go build ./...` | ✔ | ✔ | ✔ |
| 5 | `npm ci` | `apps/frontend` | `npm ci` | ✔ | ✔ | conditional (D-3) |
| 6 | `npm run lint` | `apps/frontend` | `npm run lint` | ✔ | ✔ | ✔ |
| 7 | `npm run format` | `apps/frontend` | `npm run format` | ✔ | ✔ | ✔ |
| 8 | `npm test` | `apps/frontend` | `npm test` | ✔ | ✔ | ✔ |
| 9 | `npm run build` | `apps/frontend` | `npm run build` | ✔ | ✔ | ✔ |
| 10 | `docker buildx` | root | `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .` | ✔ | — | — |

The drift check runs **first and cheapest**: if the pin the linter gate is about to use
disagrees with CI's, every downstream lint result is suspect, and finding that out after
a 60-second test run is wasted time.

Ordering note: FR-8 specifies backend-then-frontend-then-container, and that is kept.
Within the backend block the checklist's order (test, vet, lint, build) is also kept even
though build-before-test would fail faster, because FR-8 says "in order" and the ordering
is part of the ported contract, not an optimisation target.

### 3.4 Mode semantics and the "not done" banner

NFR-2 ("MUST NOT report success for a gate it did not run") and FR-7 ("a verifier agent
cannot mistake a `--quick` pass for a done branch") are the same requirement seen from two
sides. Three mechanisms, all required:

1. **Per-gate announcement** (NFR-7): every gate prints `── <label>` before running, and
   every skipped gate prints `− <label> (skipped: --quick)`. A truncated transcript still
   shows what did and did not run.
2. **Terminal verdict line**, one of exactly three strings, chosen so a grep is
   unambiguous:
   - `VERIFY: DONE — all gates passed; the branch may be called done.`
   - `VERIFY: PARTIAL — all selected gates passed, but <N> were skipped (<labels>). This does NOT count as done.`
   - `VERIFY: FAILED — <label>` (on stderr)
3. **Exit code is not the whole answer.** All three of `DONE`/`PARTIAL` exit 0, per FR-7.
   The distinction lives in the verdict line, and `task-verifier.md` is written to quote
   that line verbatim (D-8). An agent that reports "verify.sh exited 0" without the
   verdict line has under-reported, and the agent definition says so explicitly.

Rejected alternative: making `--quick` exit a distinct non-zero code (e.g. 3) to force the
distinction mechanically. It reads better in the abstract but breaks FR-7's explicit
"`--quick` and `--no-docker` also exit 0 on success" and would make `--quick` unusable in
any `&&` chain in the inner loop, which is its entire purpose.

### 3.5 `--list` and how `verify_test.sh` gets a seam

FR-17 wants tests for flag parsing, per-mode gate selection, and first-failure behaviour.
None of that can be tested by running the real gates — a test that takes two minutes and
needs Docker will not be run.

`--list` walks the identical registry with the identical mode predicates and prints each
selected gate's label instead of executing it, plus the skipped set. Borrowed directly
from atlas's `--facts`, including the reason it is trustworthy: **it is the real
selection path with the work removed, not a second description of it.**

`verify_test.sh` then splits into two kinds of assertion, matching atlas's `verify_test.sh`:

- **Behavioural**, over `--list`: flagless lists 11 gates including `docker buildx`;
  `--no-docker` lists 10 and skips exactly `docker buildx`; `--quick` skips buildx, names
  gate 1 as `go test` not `go test -race`, and the combination is the union; `--help`
  exits 0 and mentions both `HARBORMASTER_INTEGRATION` and `test:e2e`; an unknown flag
  exits 2.
- **Structural**, by reading `verify.sh` itself: the gate-label array is appended in
  exactly one place and that place is inside `step()`; no gate command string appears
  outside the registry; neither `HARBORMASTER_INTEGRATION` nor `test:e2e` appears in any
  executable line (FR-13/AC-11 asserted by construction rather than by a run); no
  `prettier --write` / `eslint --fix` / `format:fix` anywhere (FR-14/NFR-3).
- **First-failure**, hermetically: run `verify.sh` with a stub `PATH` prepended
  (`mktemp -d` containing a `go` that exits 1) and assert the run stops at gate 1 — the
  transcript contains gate 1's label and *not* gate 2's, exit is non-zero, and stderr
  names the failing gate. This is the only test that executes the real driver loop, and
  it costs milliseconds.

The structural assertions are what keep the behavioural ones honest for mode combinations
the test never enumerates. Without them `--list` becomes a second source of truth and
NFR-2 quietly dies.

### 3.6 Non-mutation (FR-14, NFR-3)

Two layers:

- The structural test above forbids the writing variants textually.
- The script itself does not `npm install`, `go mod tidy`, `gofmt -w`, or write anything
  into the tree. The only in-repo writes are under `.cache/tools/bin/` (D-2), which is
  gitignored, so `git status --porcelain` is byte-identical across a run (AC-10).

`docker buildx build` with no `--load`/`--push` produces no image and no tree change; the
two-platform build cannot `--load` anyway.

### 3.7 Robustness (NFR-4)

- `set -euo pipefail`; `ROOT` resolved from `$0`, `cd "$ROOT"` immediately, so the script
  runs from any cwd (FR-16). Gate cwds are `pushd`/subshell-scoped, never leaked.
- No TTY assumptions: no `read`, no `-t 0` gating of behaviour, colour emitted only when
  `[ -t 1 ]`.
- Docker unavailable: if `docker` is missing or `docker buildx version` fails, the buildx
  gate **fails with a clear message naming `--no-docker`** rather than hanging or silently
  passing. Silently downgrading to a pass would be exactly the NFR-2 violation this task
  exists to prevent. The message tells the operator the one-flag workaround.
- `docker buildx build` gets `--progress=plain` so a non-TTY transcript is readable.

---

## 4. Design decisions

### D-1. `verify.sh` structure — as §3. Fail-fast, no change detection, `--list` seam.

### D-2. `golangci-lint` provisioning, and the two-place pin

**Problem.** `golangci-lint` is not on `PATH` here. The flagless gate must exit 0
(AC-7), and atlas's `format-on-write.sh` — which we are porting — expects a binary at
`$ROOT/.cache/tools/bin/golangci-lint-${GOLANGCI_LINT_VERSION}` that atlas's
`tools/lint.sh` bootstraps. Harbormaster has no `tools/lint.sh` and porting one is not in
scope.

**Decision.** `verify.sh` owns the bootstrap, at the path `format-on-write.sh` already
looks in.

- `tools/toolchain.versions` is a shell-sourceable `KEY=value` file pinning
  `GOLANGCI_LINT_VERSION=v2.12.2` (matching `pr.yml`), plus `NODE_VERSION=24.19.0` and
  `GO_VERSION` recorded from `apps/backend/go.mod` for documentation. Only the
  `golangci-lint` pin is *asserted* (FR-6); Node's appears at three `setup-node` sites and
  Go's is derived by CI from `go.mod`, so asserting either would be a brittle grep for no
  gain. `toolchain.versions` says which pins are load-bearing in a comment.
- `verify.sh` gate 3 resolves the binary in this order: (a)
  `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` if executable; (b) otherwise
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION`
  into a temporary `GOBIN` and move it to that path. The install is announced as its own
  step so a first run's minute of compilation is not mistaken for a hung lint.
- `format-on-write.sh` is ported with only two edits: the prettier arm rebound to
  `apps/frontend`, and the golangci arm's module-walk simplified (single module at
  `apps/backend`, so it can `cd apps/backend` directly — the dirname-walk loop is kept
  anyway, because it is fail-open and costs nothing). Its fail-open contract is preserved
  verbatim: **it never bootstraps anything**; if the cached binary is absent it exits 0
  silently. Only `verify.sh` provisions. This is what keeps a Write from stalling for a
  minute on a cold cache.
- `.gitignore` gains `.cache/`.

**Alternatives rejected.** *Require `golangci-lint` on `PATH` and fail with an install
hint* — makes AC-7 depend on undocumented machine state and makes the version unpinned, so
local and CI lint can disagree silently; that disagreement is precisely what FR-6 exists to
catch. *Vendor atlas's `tools/lint.sh`* — 11KB of atlas-specific analyzer wiring for one
`go install`. *Run `golangci-lint` via `go run`* — recompiles the linter on every gate.

**Drift check (gate 0).** Parses `.github/workflows/pr.yml` for the
`golangci-lint-action` step's `version:` value and compares to `GOLANGCI_LINT_VERSION`.
Implementation note: the value currently sits in an inline-flow mapping
(`with: { version: v2.12.2, working-directory: apps/backend }`), so the extraction must
tolerate both flow and block style; the check fails loudly with both values when it
cannot find the key at all, rather than passing vacuously. A drift check that silently
matches nothing is worse than none.

### D-3. `--quick` tiering, and the `npm ci` question (PRD open question 2)

`--quick` skips exactly three things, all measured as the expensive ones (NFR-1: `-race`
≈ 60s, `npm ci` and the two-platform buildx being the other large costs):

1. buildx — minutes, needs Docker.
2. `-race` on the backend test gate — the gate still runs, just without the race detector.
   The label changes to `go test` so the transcript cannot claim a race-checked run.
3. `npm ci` — **conditionally**, per below.

**Decision on `npm ci`:** skip it only when `apps/frontend/node_modules/.package-lock.json`
exists *and* is not older than `apps/frontend/package-lock.json`. Otherwise run it, even
under `--quick`.

npm writes `.package-lock.json` inside `node_modules` on every install as a record of the
installed tree, so this is a real staleness signal rather than a proxy. Bare directory
presence (the PRD's other candidate) would let a stale tree mask a lockfile change — a
Renovate bump followed by `--quick` would report green on dependencies that were never
installed. Comparing the lockfile *content* hash would be stricter still, but requires
storing a hash somewhere, and the mtime comparison already catches every case where the
lockfile was rewritten.

The skip is announced (`− npm ci (skipped: --quick, node_modules current)`) and the
not-done banner names it, so it can never be read as "dependencies verified".

### D-4. Excluded suites (FR-13)

`HARBORMASTER_INTEGRATION=1 go test -tags=integration` and `npm run test:e2e` appear in
`--help` only, as prose, under a heading that says they are on-demand and not part of any
mode. **No opt-in flag is added** even though FR-13 permits one: a flag that runs the
integration suite would be a fourth mode whose exit code means something different again,
and the two commands are one copy-paste away in `--help`. The structural test asserts
neither string appears in an executable line (AC-11).

### D-5. CI parity is asymmetric, on purpose (see also D-14)

`verify.sh` mirrors the ten-command checklist. CI additionally runs `gitleaks`, a Trivy
filesystem scan, a `go-licenses` allowlist check, and `go vet -tags=integration ./...`.
None are added to `verify.sh`:

- gitleaks and Trivy need network and their own toolchains; a local miss is caught in CI
  before merge, and NFR-5 forbids introducing credential handling here anyway.
- `go-licenses` installs a tool and needs `yq`.
- `go vet -tags=integration ./...` is cheap, local, and genuinely tempting — but FR-8
  fixes the flagless gate list, and adding an eleventh gate to it is scope the PRD did not
  authorise. It is recorded in `docs/verification.md` as the known asymmetry, together
  with the rule for what to do when the script and CI disagree (CI is the authority; the
  script is the bug), which is that document's stated ownership.

This asymmetry is written down rather than silently tolerated, because "verify.sh is green
but CI is red" is otherwise a recurring rediscovery.

### D-6. The `wait-loop-guard_test.sh` fixture conflict

**The conflict is real, not hypothetical.** `wait-loop-guard_test.sh` contains
`kubectl get pods -n atlas-pr-1370` and `journalctl -u atlas --since "5 min ago"` as
*allow-list fixtures*. The first matches `atlas-`. So:

- AC-1 / FR-1: `grep -l 'atlas-' .claude/hooks/*.sh` must print nothing → the file must
  be edited.
- `process-parity.md` §7 check 1: the eight files byte-identical across all four repos →
  the file must not be edited.

Both cannot hold. **Decision: honour AC-1; rebind the two fixture strings** to
`kubectl get pods -n harbormaster-pr-1370` and `journalctl -u harbormaster ...`, and
record the divergence explicitly in the AC-23 report as: *seven of eight files
byte-identical; `wait-loop-guard_test.sh` differs in two fixture strings only, by
`process-parity.md` §5.2 (examples are genericized) and FR-1.*

Rationale: AC-1 is mechanically checkable in this repository and is a hard acceptance
criterion here; check 1 is cross-repo, cannot be evaluated from Harbormaster at all
(AC-23 already says so), and §5.2 explicitly authorises replacing atlas-specific examples.
The *guard* — `wait-loop-guard.sh`, the file whose byte-identity actually matters for
future re-harmonisation — stays byte-identical. What diverges is two strings in its test's
fixture table. FR-2/AC-2 still holds: the test must pass after the edit, which it will,
since both strings are on the allow list and neither the hostname nor the unit name
affects the guard's decision.

The other seven files are copied with `cp` and their hashes recorded for AC-23. No
reformatting, no shebang normalisation, no "while I'm here" edits — a single whitespace
change destroys the property check 1 is measuring.

### D-7. `turn-budget` state (PRD open question 3)

Confirmed by inspection: both `turn-budget.sh` and `turn-budget-guard.sh` use
`${TMPDIR:-/tmp}/claude-turn-budget`. Nothing lands in the repository, so no `.gitignore`
entry is needed and the hooks port verbatim. `task-brief.sh` writes
`<root>/.superpowers/sdd/` but creates its own self-ignoring `.gitignore` there, so that
needs no entry either. The only `.gitignore` addition in this task is `.cache/` (D-2).

### D-8. Agent trio rebinding

`task-implementer.md`, `task-verifier.md`, `task-reviewer.md` are ports with targeted
rebinding. What changes:

| Agent | Atlas assumption | Harbormaster binding |
|---|---|---|
| `task-implementer` | module-local build/test is `cd services/atlas-<svc>/atlas.com/<svc> && go build ./... && go test ./...`; `libs/` fan-out; `go.work` | `cd apps/backend && go build ./... && go test ./...` for a Go task; `cd apps/frontend && npm run lint && npm test` (no `npm ci`, no build) for a frontend task. Single module, no `go.work`, no fan-out — so the "check whether a `libs/` change fans out" guidance is replaced by the simpler true statement that every Go change is one module. |
| `task-implementer` | `tools/doc-slice.sh` for reading large docs | Replaced with the D-10.2 slicing idioms. Prohibition on running `tools/verify.sh` inside an implementer is **kept verbatim** — it is the point of the trio. |
| `task-implementer` | `libs/atlas-constants` reuse rule (DOM-21) | Rebound to the equivalent Harbormaster rule from `backend-dev-guidelines`; if no exact analogue exists, the rule is restated generically ("check `internal/` for an existing equivalent before adding a constant or helper") rather than dropped (FR-34). |
| `task-verifier` | default `tools/verify.sh --quick`; `--base <sha>`; quotes the script's `change base ... — N changed path(s)` line | Default stays `tools/verify.sh --quick`. **All `--base` guidance is removed** — the flag does not exist here (D-1) — and the "quote the change-base line" instruction is replaced by "quote the `VERIFY:` verdict line verbatim" (§3.4). The `never edits` / `an unrun gate is ERROR, never PASS` contract is kept word for word. `model: haiku` kept. |
| `task-reviewer` | commit-range review, durable artifact, verdict-first, no fan-out; `model: sonnet` | Kept. Artifact path rebound to `docs/tasks/task-NNN-slug/` per this repo's artifact-location override. Must not collide with the existing reviewers' `audit.md` — per-unit reviews write `review-task-<N>.md`, and `docs/review-protocol.md` records that split. |
| `service-documentation` | argument is a service name or `services/<name>` path | See D-9. |

The 120-call budget, the `PARTIAL` hand-back, the warn-at-100 hook coupling, and
brief-first discovery all port unchanged — they are repo-agnostic and are the reason the
agent exists.

### D-9. What "service" means for `service-documentation` (FR-25)

Harbormaster has no `services/*` tree. Porting the agent with a `services/` assumption
would produce an agent that can never be invoked.

**Decision:** rebind the unit of documentation to a **component**, resolved in this order:

1. a backend domain package — `apps/backend/internal/<name>` (`buckets`, `objects`,
   `policies`, `users`, `auth`, `jobs`, `metrics`, `sse`, …);
2. a frontend feature — `apps/frontend/src/features/<name>`;
3. the whole backend or frontend app, when the argument is `backend` / `frontend`.

Output lands in `docs/architecture/<name>.md`, beside the existing
`docs/architecture/overview.md`, and `/service-doc` is documented as taking a component
name or path. The command file keeps its name (`service-doc.md`) so the parity manifest
and the other three repos still line up; its body says what a "service" is here.

**Alternative rejected:** renaming the command to `/component-doc`. Cleaner in isolation,
but it breaks the one thing §7 check 4/6 is trying to assert — that the four repos carry
the same command set — for a cosmetic gain.

### D-10. Owner-doc genericization

Ten documents, ~83KB of atlas prose. The transformation is mechanical in shape and
judgement-heavy in detail, so it is specified as three passes rather than left to feel.

**10.1 — Tool-reference pass.** Every `tools/<x>` reference gets one of three fates.
Twenty-two distinct paths were counted; the policy is:

| Atlas reference | Fate here |
|---|---|
| `tools/verify.sh`, `tools/verify_test.sh`, `tools/task-brief.sh`, `tools/task-numbers.sh`, `tools/toolchain.versions` | Kept — they exist here. Flag specifics rewritten to this repo's surface (`--quick`/`--no-docker`/`--list`, never `--base`/`--all`/`--facts`). |
| `tools/doc-slice.sh` (7 refs, `slice-first.md` + `task-implementer`) | **Rule kept, mechanism replaced** — see 10.2. |
| `tools/lint.sh` (4) | Replaced by `tools/verify.sh`'s lint gate + `tools/toolchain.versions`. |
| `tools/task-resolve.sh`, `tools/task-facts.sh`, `tools/agent-ledger.sh`, `tools/change-surfaces.sh`, `tools/plan-context.sh` | Not ported. Where a rule depends on one, the rule is restated as a manual procedure; where the reference is only an illustration, a neutral example replaces it. |
| `tools/atlasguards`, `tools/rediskeyguard`, `tools/cideps`, `tools/build-services.sh`, `tools/db-bootstrap.sh`, `tools/gen-lb-ports.sh`, `tools/go-analyzer-guards.sh`, `tools/service-registration-guard.sh` | Atlas-domain guards with no Harbormaster analogue. The *rules* they illustrate (a repo-specific invariant belongs in a script, not in a reviewer's head) survive with Harbormaster examples — the FR-6 toolchain drift check is the natural one, since this task creates it. |
| `tools/foo.sh` / `tools/foo_test.sh` | Already neutral placeholders; kept. |

**10.2 — `slice-first.md` without `doc-slice.sh`.** The rule ("read a slice, escalate to
the whole only when the slice proves insufficient") is repo-agnostic and is exactly the
kind of rule FR-34 forbids dropping. Its four worked examples are re-expressed with tools
that exist everywhere: `grep -n '^#'` for a document's shape, `sed -n 'A,Bp'` for the one
section, `grep -n -A/-B` for a needle in an offloaded tool result, and `Read` with
`offset`/`limit` for the harness-native form. The measurement section's atlas token counts
are replaced with this repo's own concrete cases — reading all of `prd.md` versus its §4,
or the whole `plan.md` versus one `tools/task-brief.sh` slice.

**10.3 — `observability.md` is a rewrite, not a port.** Atlas's version documents an OTel
→ spanmetrics → Prometheus/Loki/Grafana pipeline that Harbormaster does not have. What
Harbormaster has: structured logging in `internal/observability/log`, an
`internal/metrics` collector that polls MinIO's own Prometheus endpoint, `deploy/docker`,
`deploy/kubernetes`, and an existing `docs/operator/` set. The ported document keeps the
*rules* — log-field naming discipline, "diagnose a runtime failure by reading the logs
before changing code", a deploy smoke test that proves the thing actually came up — bound
to those. Sections with no analogue (manual spans, spanmetrics dimensions, cardinality
budget, sampling caveat, Grafana panels) are dropped as *pipeline mechanics*, not as
rules; nothing in them states a rule that survives the loss of the pipeline. It must link
to `docs/operator/*` rather than restate it — the operator docs are already the authority
on configuration and recovery, and a second copy would drift.

**10.4 — Link hygiene (FR-35/AC-16).** Links to `docs/packets/*`,
`docs/adding-a-new-service.md`, `docs/runbooks/ephemeral-pr-deployments.md`, and six
specific atlas task folders are rebound to a Harbormaster equivalent or removed with their
sentence. Verification is a script-shaped grep over `CLAUDE.md` + the ten documents,
extracting `docs/...` targets and asserting each exists — run as an explicit step at the
end of the task, not eyeballed.

**10.5 — NFR-6.** `block-home-paths-in-docs.sh` becomes active in this same task, and the
ported documents are `docs/` content. Any absolute or home path in atlas's originals must
become a repo-relative path or a placeholder (`$ATLAS`, `<worktree>`). Order matters: the
hook is wired in D-11's settings step, so the docs pass must satisfy it or the writes get
rejected — which is the intended behaviour and a useful forcing function.

### D-11. `CLAUDE.md` restructure

Eight headings, in the FR-36 order. The mapping from today's prose to the new shape, so
nothing in FR-40 is lost by accident:

| New heading | Content |
|---|---|
| `# Harbormaster` | Corrected overview (FR-39): Go backend at `apps/backend` (single module, `cmd/`, `internal/`, `migrations/`, `Makefile`), React/TS frontend at `apps/frontend`, `deploy/{docker,kubernetes}`, `docs/{architecture,operator,tasks}`. The "unscaffolded — only README.md exists" claim is deleted, as is the "update this file once layout is settled" instruction it justified. |
| `## Never do this` | Don't implement when asked to understand or plan; don't edit the main repo when a task worktree exists; don't skip code review before a PR; don't call a `--quick`/`--no-docker` run "done"; don't walk design/plan documents section-by-section. |
| `## Evidence & grounding` | Verification over memory for MinIO admin API contracts and configuration values; read the source rather than speculate; report what could not be verified rather than asserting it. |
| `## Development workflow` | The four phases, the worktree convention, the artifact-location override, fuzzy task-identifier resolution, `tools/task-numbers.sh` for numbering. |
| `## Done means verified` | `tools/verify.sh` and the three modes — **referenced, not restated** (FR-38). The ten commands live in the script; the on-demand suites are named as on-demand. |
| `## Dispatching agents` | The trio, the four existing reviewers, the model pins, the budget — pointing at `docs/agent-dispatch.md`. |
| `## Handing off context` | Brief-first discovery, `tools/task-brief.sh`, slice-first. |
| `## Repository conventions` | Straightforward moves over re-exported type aliases; don't break service boundaries by calling another layer's internals; the code-review pattern and its three modular reviewers; the guideline skills and `skill-rules.json`. |
| `## Where the procedures live` | Trigger → owner table, one row per §4.8 document, every target verified to exist (FR-37/AC-16). |

Every FR-40 item appears above exactly once. The rule that a rule may move but not vanish
is the whole point of writing this mapping down before editing.

### D-12. Phase-command amendments, landed last (PRD open question 4)

FR-28 edits `/spec-task` and `/execute-task` — the very commands running this task. All
`.claude/commands/` edits land in the **final** work unit, after everything they reference
exists and after `/execute-task` has finished dispatching for this task. Editing a command
file mid-flight does not affect the already-loaded session, but it does make the tree
inconsistent with the session's behaviour, and a failed run would then be ambiguous.

Scope of the edits, deliberately minimal:

- `spec-task.md`: replace the hand-rolled "pick the next free NNN" step with
  `tools/task-numbers.sh next`, worded as atlas words it (that script is the only
  collision-safe source; do not pick by hand). Opportunistically replace the literal
  a literal absolute home-directory example path with a `<worktree>` placeholder.
- `execute-task.md`: implementer dispatches use `subagent_type: task-implementer`, never
  `general-purpose`; per-task review uses `task-reviewer`; the gate runs in
  `task-verifier`, never inside an implementer; brief extraction uses
  `tools/task-brief.sh`; the `PARTIAL` status and its continuation protocol are added.
- `plan-task.md` / `design-task.md`: one-line references only (`task-brief.sh` slices
  `## Task N` headings, so plans must use that heading shape).

**Not** a wholesale port of atlas's 400-line `execute-task.md`: it encodes `go.work`
fan-out, `libs/` change surfaces, and module-count heuristics that are false here, and
importing them would be importing bugs.

### D-13. `docs/process-parity.md` placement (PRD open question 5)

**Commit it at `docs/process-parity.md`** on this branch, as a verbatim copy of the
task-folder version. Three reasons: §7 check 3's grep exempts that exact path by name,
which only makes sense if the file is expected to exist there; the ported owner docs
already link to it; and a future re-harmonisation reads it from `docs/`, not from a task
folder it would have to go looking for. The task-folder copy stays too — it is the PRD's
input record and `docs/tasks/` is excluded from the check.

### D-14. `docs/verification.md` records the CI asymmetry — as D-5.

---

## 5. Sequencing

Ordered by dependency, not by size. Each unit ends in a commit.

| # | Unit | Depends on | Why here |
|---|---|---|---|
| 1 | `tools/toolchain.versions`; `.gitignore` += `.cache/` | — | Everything downstream reads the pin. |
| 2 | `tools/verify.sh` + `tools/verify_test.sh` | 1 | The single largest piece; the verifier agent and `CLAUDE.md` both reference it. Flagless run must go green here (AC-7), which is also the first time `golangci-lint` gets bootstrapped. |
| 3 | `tools/task-numbers.sh`, `task-numbers_test.sh`, `task-brief.sh` (verbatim ports) | — | Independent; could run parallel to 2. `task-num-collision-detector.sh` needs `task-numbers.sh` present before it is wired. |
| 4 | The nine hooks (8 portable + rebound `format-on-write.sh`) | 1, 3 | `format-on-write.sh` sources `toolchain.versions`; the collision detector needs `task-numbers.sh`. AC-1/AC-2 checked here. |
| 5 | `.claude/settings.json` wiring | 4 | Activates everything in 4 — notably `block-home-paths-in-docs.sh`, which the docs pass must then satisfy. |
| 6 | Agent trio + `service-documentation` + `/service-doc` + `/fix-pr-bug` | 2 (verifier cites `verify.sh` flags) | AC-12/AC-13. |
| 7 | Ten owner documents + `docs/process-parity.md` | 5 (hook active), 2 and 6 (they are cited) | The judgement-heavy pass; three sub-passes per D-10. |
| 8 | `CLAUDE.md` rewrite | 7 (its table's targets must exist) | AC-16/AC-18/AC-19. |
| 9 | Phase-command amendments | 2, 3, 6 | Last, per D-12. |
| 10 | Final assertion sweep + the AC-23/AC-24 report | all | §6. |

Units 2 and 7 are the bulk of the effort and are the two worth splitting further at plan
time. Unit 7 in particular should be one document per work unit — ten documents in one
unit is exactly the shape that overruns an implementer's budget and comes back `PARTIAL`.

---

## 6. Verification plan

Everything in §10 of the PRD is checkable in-repo except AC-23/AC-24. The final unit runs
them as one scripted sweep and pastes the output into the audit, rather than asserting
them from memory:

- AC-1, AC-20, AC-22: greps, exact commands as written in the PRD.
- AC-2, AC-6: run the two test scripts.
- AC-3, AC-4, AC-5, AC-12, AC-13, AC-14, AC-15: file-presence and content checks; AC-12's
  "byte-unchanged" asserted with `git diff --stat` over `.claude/agents/` restricted to
  the four pre-existing files.
- AC-7, AC-8, AC-9, AC-11: three real `verify.sh` runs plus `--help`; AC-11 additionally
  asserted structurally (§3.5) since a passing run cannot prove a command *did not* run.
- AC-10: `git status --porcelain` captured before and after the flagless run, diffed.
- AC-16: the link-resolution grep from D-10.4.
- AC-17: reviewer judgement against the atlas originals — dispatched as a review, not
  self-asserted.
- AC-21: `git diff --name-only main...HEAD | grep -c '^apps/'` must be 0.
- **AC-23/AC-24: reported, never asserted.** The report gives Harbormaster's
  `sha256sum` for each of the eight hook files, states plainly that the pairwise
  comparison cannot be performed from this repository, and names the one known divergence
  (D-6) rather than leaving it to be discovered by whoever runs check 1.

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| `go install golangci-lint@v2.12.2` fails or is slow on first run, making AC-7 look like a `verify.sh` bug | Bootstrap is its own announced step with its own error message; cached under `.cache/tools/bin/` so it happens once. If the install proves unworkable, the fallback is a `PATH` binary with a version assertion — recorded here so the decision is not re-litigated from scratch. |
| The 7 existing eslint warnings tempt a "while I'm here" cleanup | PRD open question 1 already ruled it out of scope; `verify.sh` must not add `--max-warnings 0`. The structural test asserts that flag is absent. |
| Owner-doc pass degenerates into find/replace, silently dropping rules whose examples don't transfer | D-10's three-pass structure plus AC-17 being reviewer-judged against the originals, not self-certified. |
| `block-home-paths-in-docs.sh` blocks the docs pass mid-flight | Intended. Unit ordering puts hook activation (5) before the docs pass (7) deliberately, so the failure is immediate and local rather than discovered at AC-22. |
| Unit 7 overruns the implementer budget | Split one document per unit at plan time. |
| Drift check silently matches nothing after a `pr.yml` reformat | The check fails loudly when the key is not found, rather than passing vacuously (D-2). |

---

## 8. Open questions — all resolved

| PRD §9 | Resolution |
|---|---|
| 1. `npm run lint` strictness | Out of scope, confirmed. No `--max-warnings 0`; asserted absent by the structural test. Follow-up task to fix the 7 warnings and tighten. |
| 2. `--quick` and `npm ci` | Lockfile-mtime comparison against `node_modules/.package-lock.json`, not bare directory presence (D-3). |
| 3. `turn-budget.sh` state | `${TMPDIR:-/tmp}/claude-turn-budget`, outside the repo. No `.gitignore` entry (D-7). |
| 4. Phase-command coupling | Edits land in the final unit (D-12). |
| 5. `docs/process-parity.md` placement | Committed at `docs/process-parity.md` as well as in the task folder (D-13). |

One new question, raised and answered here rather than deferred: **the AC-1 / §7-check-1
conflict on `wait-loop-guard_test.sh`** (D-6). Resolved in favour of AC-1, with the
divergence reported rather than hidden.
