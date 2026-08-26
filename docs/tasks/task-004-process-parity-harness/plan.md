# Process Parity Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Harbormaster the context-discipline layer atlas already has — one executable verification entrypoint, a budget-capped agent trio, the portable enforcement hooks, collision-safe task numbering, ten owner documents, and a rule-list `CLAUDE.md`.

**Architecture:** Ninety percent of this is copying text from the atlas worktree into this repo with targeted rebinding. The one piece of genuine engineering is `tools/verify.sh`, built from the prose checklist currently in `CLAUDE.md`: a fixed eleven-gate registry walked in order, fail-fast, three modes (`flagless` / `--quick` / `--no-docker`), and a `--list` dry-run that is the real selection path with the work removed. Work is sequenced so that each unit's dependencies already exist: pins → verify → task tooling → hooks → settings → agents/commands → docs → `CLAUDE.md` → phase commands → final sweep.

**Tech Stack:** Bash (all tooling), Markdown (all agents, commands, docs), JSON (`.claude/settings.json`). Go 1.25.12 module at `apps/backend`, Vite/React/TS at `apps/frontend` — **neither is modified by this task**.

**Spec:** `docs/tasks/task-004-process-parity-harness/design.md` (PRD at `prd.md`; canonical spec at `process-parity.md`; scope brief at `process-parity-brief.md`, all in the same folder)

## Global Constraints

- **`$ATLAS`** — the atlas source worktree — is
  `/home/tumidanski/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename`.
  It is read-only for this task. **Never write to it.** Never commit this absolute
  path into any file under `docs/` (NFR-6 / `block-home-paths-in-docs.sh`); write
  `$ATLAS` as a literal placeholder instead.
- **Worktree:** all work happens in
  `/home/tumidanski/source/Harbormaster/.worktrees/task-004-process-parity-harness`
  on branch `task-004-process-parity-harness`. Prefix every Bash call with
  `cd <worktree> && ...`.
- **No application changes.** `git diff --name-only main...HEAD` must never list a
  path under `apps/backend/` or `apps/frontend/` (AC-21).
- **`golangci-lint` pin: `v2.12.2`**, verbatim from `.github/workflows/pr.yml:49`
  (`with: { version: v2.12.2, working-directory: apps/backend }`).
- **Node pin: `24.19.0`**, from the three `setup-node` blocks in
  `.github/workflows/pr.yml`. Recorded, not asserted.
- **Go version: `1.25.12`**, from `apps/backend/go.mod`. Recorded, not asserted.
- **`.golangci.yml` lives at `apps/backend/.golangci.yml`**, not at the repo root
  (this is where atlas differs and every `-c` path must be rebound).
- **`grep -l 'atlas-' .claude/hooks/*.sh` must print nothing** when this task is done
  (FR-1 / AC-1). Note that `services/atlas-ui` in `format-on-write.sh` matches this,
  as do two fixture strings in `wait-loop-guard_test.sh`.
- **`npm run lint` must not gain `--max-warnings 0`.** The tree has 7 eslint warnings
  and 0 errors; tightening the gate is explicitly out of scope (PRD §9 Q1).
- **Nothing in `verify.sh` may mutate the working tree** — no `prettier --write`,
  no `eslint --fix`, no `go mod tidy`, no `gofmt -w` (FR-14 / NFR-3).
- **Never `git add -A` or `git add .`** — add the paths you changed, by name.
- Every task's heading is `### Task N: ...` so `tools/task-brief.sh` can slice it.

---

## File Structure

**New scripts** (`tools/`):

| File | Responsibility |
|---|---|
| `tools/toolchain.versions` | Shell-sourceable `KEY=value` pins. Single source of truth for `GOLANGCI_LINT_VERSION`. |
| `tools/verify.sh` | The executable definition of "done". Eleven-gate registry, fail-fast, three modes plus `--list`/`--help`. |
| `tools/verify_test.sh` | Behavioural (`--list`), structural (reads `verify.sh`), and hermetic first-failure assertions. Never runs a real gate. |
| `tools/task-numbers.sh` | Collision-safe `task-NNN` allocation. Verbatim port. `next`/`check`/`list`. |
| `tools/task-numbers_test.sh` | Verbatim port. Builds throwaway repos under `mktemp -d`. |
| `tools/task-brief.sh` | Slices one `### Task N` section out of a plan into a standalone brief. Verbatim port. |

**New hooks** (`.claude/hooks/`): `wait-loop-guard.sh`, `wait-loop-guard_test.sh`,
`block-home-paths-in-docs.sh`, `turn-budget.sh`, `turn-budget-guard.sh`,
`fork-dispatch-guard.sh`, `commit-boundary.sh`, `task-num-collision-detector.sh`
(the eight portable ones), plus `format-on-write.sh` (rebound).
`skill-activation-prompt.{py,sh}` are untouched.

**New agents** (`.claude/agents/`): `task-implementer.md`, `task-verifier.md`,
`task-reviewer.md`, `service-documentation.md`. The four existing agent files stay
byte-identical.

**New commands** (`.claude/commands/`): `fix-pr-bug.md`, `service-doc.md`.
Modified: `spec-task.md`, `execute-task.md`, `plan-task.md`, `design-task.md`.

**New docs** (`docs/`): `agent-dispatch.md`, `verification.md`,
`superpowers-integration.md`, `review-protocol.md`, `post-implementation.md`,
`codemod-vs-agents.md`, `slice-first.md`, `tooling-conventions.md`,
`git-workflow.md`, `observability.md`, `process-parity.md`.

**Rewritten:** `CLAUDE.md`. **Modified:** `.gitignore`, `.claude/settings.json`.

---

### Task 1: Toolchain pins and `.cache/` ignore

Everything downstream reads `GOLANGCI_LINT_VERSION` from this file:
`verify.sh`'s drift check (gate 0), `verify.sh`'s linter bootstrap, and the ported
`format-on-write.sh`. It lands first so nothing downstream has to invent a pin.

**Files:**
- Create: `tools/toolchain.versions`
- Modify: `.gitignore` (append a `.cache/` entry)

**Interfaces:**
- Consumes: nothing.
- Produces: a shell-sourceable file exporting exactly three assignments —
  `GO_VERSION=1.25.12`, `NODE_VERSION=24.19.0`, `GOLANGCI_LINT_VERSION=v2.12.2`.
  Tasks 2, 3 and 4 all `source "$ROOT/tools/toolchain.versions"` and read
  `GOLANGCI_LINT_VERSION`.
- Produces: the gitignored path prefix `.cache/`, under which Task 2 caches the
  `golangci-lint` binary at `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION`.

- [ ] **Step 1: Confirm the pin values against their sources**

Do not take these from this plan. Read them:

```bash
cd <worktree> && grep -n -A2 'golangci-lint-action' .github/workflows/pr.yml
cd <worktree> && grep -n 'node-version' .github/workflows/pr.yml
cd <worktree> && grep -E '^go ' apps/backend/go.mod
```

Expected: `version: v2.12.2`, `node-version: 24.19.0` (three occurrences),
`go 1.25.12`. If any differs, use what you read and say so in your report.

- [ ] **Step 2: Write `tools/toolchain.versions`**

```sh
# tools/toolchain.versions — the repo's toolchain pins, as shell-sourceable
# KEY=value. Read by tools/verify.sh (the drift check and the golangci-lint
# bootstrap) and by .claude/hooks/format-on-write.sh.
#
# Only GOLANGCI_LINT_VERSION is load-bearing: verify.sh gate 0 asserts it matches
# the `golangci-lint-action` version in .github/workflows/pr.yml, because local
# and CI lint disagreeing silently is exactly the failure that check exists to
# catch. Bump here and in pr.yml together; the gate will tell you if you forgot.
#
# NODE_VERSION and GO_VERSION are recorded for reference and are NOT asserted.
# Node's pin appears at three separate `setup-node` sites in pr.yml and Go's is
# derived by CI from apps/backend/go.mod via `go-version-file`; grepping for
# either would be brittle for no gain.
GO_VERSION=1.25.12
NODE_VERSION=24.19.0
GOLANGCI_LINT_VERSION=v2.12.2
```

- [ ] **Step 3: Verify it sources cleanly and exports the expected value**

```bash
cd <worktree> && ( set -u; . tools/toolchain.versions; echo "$GOLANGCI_LINT_VERSION" )
```

Expected: `v2.12.2` and exit 0.

- [ ] **Step 4: Append the `.cache/` ignore**

Append to the end of `.gitignore`:

```
# Locally bootstrapped tool binaries (tools/verify.sh caches golangci-lint here)
.cache/
```

- [ ] **Step 5: Confirm the ignore takes effect**

```bash
cd <worktree> && mkdir -p .cache/tools/bin && touch .cache/tools/bin/probe \
  && git status --porcelain && rm -rf .cache
```

Expected: `git status --porcelain` prints nothing.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add tools/toolchain.versions .gitignore \
  && git commit -m "chore(task-004): pin toolchain versions and ignore .cache/"
```

---

### Task 2: `tools/verify.sh` and `tools/verify_test.sh`

The one piece of real engineering. Written test-first: `verify_test.sh` asserts
flag parsing, per-mode gate selection, first-failure behaviour, and the structural
properties that keep `--list` from becoming a second source of truth. None of the
assertions run a real gate, so the whole suite finishes in well under a second.

**Do not** get the real gates green in this task — that is Task 3. This task ends
when `tools/verify_test.sh` passes.

**Files:**
- Create: `tools/verify.sh` (mode `755`)
- Create: `tools/verify_test.sh` (mode `755`)
- Read only: `tools/toolchain.versions`, `.github/workflows/pr.yml`,
  `apps/frontend/package.json`, `apps/backend/.golangci.yml`

**Interfaces:**
- Consumes: `GOLANGCI_LINT_VERSION` from `tools/toolchain.versions` (Task 1).
- Produces, for Tasks 3, 6, 12 and 13:
  - CLI surface `tools/verify.sh [--quick] [--no-docker] [--list] [--help]`;
    unknown flag exits `2`.
  - Exactly three terminal verdict strings, which `task-verifier.md` (Task 6) and
    `docs/verification.md` (Task 10) quote verbatim:
    - `VERIFY: DONE — all gates passed; the branch may be called done.`
    - `VERIFY: PARTIAL — all selected gates passed, but <N> were skipped (<labels>). This does NOT count as done.`
    - `VERIFY: FAILED — <label>` (stderr)
  - Gate labels, in registry order: `toolchain drift`, `go test -race` (or
    `go test` under `--quick`), `go vet`, `golangci-lint`, `go build`, `npm ci`,
    `npm run lint`, `npm run format`, `npm test`, `npm run build`, `docker buildx`.
  - The cached linter path
    `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION`, which
    `format-on-write.sh` (Task 4) reads and never creates.

- [ ] **Step 1: Write the failing test**

Create `tools/verify_test.sh` (mode `755`):

```bash
#!/usr/bin/env bash
# verify_test.sh — tests for tools/verify.sh.
#
# The claim under test is the one that makes --list worth trusting:
#
#   --list does not re-implement the gate selection; it IS the selection, with
#   the work removed.
#
# Three kinds of assertion enforce that.
#
#   Behavioural  run --list in each mode and check the selected/skipped labels.
#   Structural   read verify.sh and require that no gate label can be produced
#                anywhere except inside step()/skip() — which is what makes the
#                behavioural agreement hold for mode combinations this test
#                never enumerates.
#   First-failure  run the real driver loop with a stub `go` on PATH and require
#                  it to stop at the first failing gate.
#
# No assertion here runs a real gate. A test that takes two minutes and needs
# Docker is a test that does not get run.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY="$HERE/verify.sh"
[ -x "$VERIFY" ] || { echo "FATAL: $VERIFY not executable" >&2; exit 2; }

fails=0
assert_eq() {
  if [ "$2" = "$3" ]; then echo "ok   - $1"
  else echo "FAIL - $1"; echo "        want: $2"; echo "        got:  $3"; fails=$((fails + 1)); fi
}
assert_contains() {
  case "$3" in
    *"$2"*) echo "ok   - $1" ;;
    *) echo "FAIL - $1"; echo "        expected to contain: $2"; fails=$((fails + 1)) ;;
  esac
}
assert_not_contains() {
  case "$3" in
    *"$2"*) echo "FAIL - $1"; echo "        expected NOT to contain: $2"; fails=$((fails + 1)) ;;
    *) echo "ok   - $1" ;;
  esac
}

# ---- behavioural: gate selection per mode ---------------------------------

flagless="$("$VERIFY" --list 2>/dev/null)"
nodocker="$("$VERIFY" --list --no-docker 2>/dev/null)"
quick="$("$VERIFY" --list --quick 2>/dev/null)"
both="$("$VERIFY" --list --quick --no-docker 2>/dev/null)"

assert_eq "flagless selects 11 gates" "11" \
  "$(printf '%s\n' "$flagless" | grep -cv '^- ')"
assert_contains "flagless includes docker buildx" "docker buildx" "$flagless"
assert_contains "flagless names the race-enabled test gate" "go test -race" "$flagless"
assert_eq "flagless skips nothing" "0" \
  "$(printf '%s\n' "$flagless" | grep -c '^- ')"

assert_eq "--no-docker selects 10 gates" "10" \
  "$(printf '%s\n' "$nodocker" | grep -cv '^- ')"
assert_eq "--no-docker skips exactly one gate" "1" \
  "$(printf '%s\n' "$nodocker" | grep -c '^- ')"
assert_contains "--no-docker skips docker buildx" "- docker buildx" "$nodocker"

assert_contains "--quick skips docker buildx" "- docker buildx" "$quick"
assert_contains "--quick names the un-raced test gate" "go test" "$quick"
assert_eq "--quick does not name a race-enabled test gate" "0" \
  "$(printf '%s\n' "$quick" | grep -c 'go test -race')"

assert_contains "--quick --no-docker skips docker buildx" "- docker buildx" "$both"
assert_eq "--quick --no-docker does not name a race-enabled test gate" "0" \
  "$(printf '%s\n' "$both" | grep -c 'go test -race')"

# ---- behavioural: flag parsing and --help ---------------------------------

help="$("$VERIFY" --help 2>&1)"; help_rc=$?
assert_eq "--help exits 0" "0" "$help_rc"
assert_contains "--help names the integration suite" "HARBORMASTER_INTEGRATION" "$help"
assert_contains "--help names the e2e suite" "test:e2e" "$help"
assert_contains "--help says a flagged run is not done" "does NOT count as done" "$help"

"$VERIFY" --nonsense >/dev/null 2>&1; bad_rc=$?
assert_eq "an unknown flag exits 2" "2" "$bad_rc"

# ---- structural: labels come only from step()/skip() ----------------------
#
# The anti-drift assertions. If a future edit prints a gate label directly, or
# runs a gate command outside step(), --list starts answering from a second
# source of truth and NFR-2 quietly dies.

assert_eq "SELECTED is appended in exactly one place" "1" \
  "$(grep -c 'SELECTED+=' "$VERIFY")"
assert_eq "that one place is inside step()" "1" \
  "$(awk '/^step\(\) \{/,/^\}/' "$VERIFY" | grep -c 'SELECTED+=')"
assert_eq "SKIPPED is appended in exactly one place" "1" \
  "$(grep -c 'SKIPPED+=' "$VERIFY")"
assert_eq "that one place is inside skip()" "1" \
  "$(awk '/^skip\(\) \{/,/^\}/' "$VERIFY" | grep -c 'SKIPPED+=')"

# FR-14 / NFR-3: the gate must never mutate the tree, and must never tighten
# the eslint gate (PRD open question 1 put that out of scope).
assert_eq "no writing formatter or linter variants" "0" \
  "$(grep -cE 'prettier --write|eslint --fix|--max-warnings|gofmt -w|go mod tidy' "$VERIFY")"

# FR-13 / AC-11: a passing run cannot prove a command did NOT run, so assert it
# textually. Both strings are permitted in comments and in the usage heredoc —
# --help must mention them — and nowhere else.
outside_help="$(awk '
  /^usage\(\) \{/ { inusage = 1 }
  inusage && /^\}/ { inusage = 0; next }
  inusage { next }
  /^[[:space:]]*#/ { next }
  /HARBORMASTER_INTEGRATION|test:e2e/ { print }
' "$VERIFY" | wc -l | tr -d ' ')"
assert_eq "excluded suites appear only in comments and --help" "0" "$outside_help"

# ---- first-failure: the real driver loop, hermetically --------------------
#
# A stub `go` that always fails. Gate 0 (toolchain drift) is pure grep and still
# passes; gate 1 must fail, and the run must stop there.

stub="$(mktemp -d)"
printf '#!/usr/bin/env bash\nexit 1\n' > "$stub/go"
chmod +x "$stub/go"
ff="$(PATH="$stub:$PATH" "$VERIFY" --quick --no-docker 2>&1)"; ff_rc=$?
rm -rf "$stub"

assert_eq "a failing gate exits non-zero" "1" "$ff_rc"
assert_contains "the failing gate is named on the verdict line" "VERIFY: FAILED — go test" "$ff"
assert_contains "the failing gate announced itself" "go test" "$ff"
assert_not_contains "the run stopped before the next gate" "go vet" "$ff"

echo
if [ "$fails" -eq 0 ]; then
  echo "verify_test.sh: all assertions passed"
  exit 0
fi
echo "verify_test.sh: $fails failure(s)" >&2
exit 1
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd <worktree> && chmod +x tools/verify_test.sh && ./tools/verify_test.sh
```

Expected: FAIL with `FATAL: .../tools/verify.sh not executable`, exit 2.

- [ ] **Step 3: Write `tools/verify.sh`**

Create `tools/verify.sh` (mode `755`):

```bash
#!/usr/bin/env bash
# tools/verify.sh — the single executable definition of "this branch is done".
#
# It replaces the ten-command prose checklist that used to live in CLAUDE.md. A
# prose checklist cannot be run by a verifier agent, and is exactly the kind of
# thing that gets partially run and then reported as green.
#
# Contract
# --------
#   flagless          exit 0 means THE BRANCH MAY BE CALLED DONE
#   --no-docker       exit 0 means the selected gates passed; NOT done
#   --quick           exit 0 means the selected gates passed; NOT done
#   --list            prints the gates these same flags would select; runs none
#   --help            usage, including the two on-demand suites this never runs
#
# Non-zero exit: the first failing gate, named on stderr. The run STOPS there —
# a verifier agent asked for "the first failing block" should not have to work
# out which of several failures came first.
#
# The distinction between DONE and PARTIAL lives in the terminal `VERIFY:` line,
# not in the exit code — all three of DONE/PARTIAL/skip-free exit 0 by design so
# that `--quick` stays usable in an `&&` chain in the inner loop. An agent that
# reports "verify.sh exited 0" without quoting the verdict line has
# under-reported.
#
# This script MUST NOT mutate the working tree. `npm run format` is
# `prettier --check` and `npm run lint` is bare `eslint .`; neither may be
# switched to its writing variant, and the eslint gate must not be tightened to
# fail on warnings — the tree carries 7 warnings and 0 errors, and tightening
# that is a separate task. (verify_test.sh asserts the tightening flag's literal
# absence from this file, which is why it is not spelled out here.)
#
# See docs/verification.md for the known asymmetries with CI.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

QUICK=0
NO_DOCKER=0
LIST=0

usage() {
    cat <<'EOF'
usage: tools/verify.sh [--quick] [--no-docker] [--list] [--help]

Runs the repository's verification gates in order and stops at the first
failure, naming it on stderr.

  (no flags)    Run every gate. Exit 0 means the branch may be called done.
  --quick       Skip the container build, drop -race from the backend tests,
                and skip `npm ci` when node_modules is already current.
                Exit 0 does NOT count as done.
  --no-docker   Skip the container build and nothing else.
                Exit 0 does NOT count as done.
  --list        Print the gates the same flags would select, and run none.
  --help        This message.

Gates, in order:

  toolchain drift   tools/toolchain.versions vs .github/workflows/pr.yml
  go test -race     apps/backend   (go test without -race under --quick)
  go vet            apps/backend
  golangci-lint     apps/backend
  go build          apps/backend   (CGO_ENABLED=0)
  npm ci            apps/frontend
  npm run lint      apps/frontend
  npm run format    apps/frontend  (prettier --check; non-mutating)
  npm test          apps/frontend
  npm run build     apps/frontend
  docker buildx     repo root      (linux/amd64,linux/arm64)

On demand, and deliberately NOT part of any mode above — these are not per-PR
gates and never affect this script's exit code. Run them by hand:

  cd apps/backend  && HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...
  cd apps/frontend && npm run test:e2e
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --quick)     QUICK=1; shift ;;
        --no-docker) NO_DOCKER=1; shift ;;
        --list)      LIST=1; shift ;;
        -h|--help)   usage; exit 0 ;;
        *) echo "verify.sh: unknown option $1" >&2; usage >&2; exit 2 ;;
    esac
done

if [ -t 1 ]; then
    B=$'\033[1m'; RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; RST=$'\033[0m'
else
    B=""; RED=""; GRN=""; YEL=""; RST=""
fi

SELECTED=()
SKIPPED=()

# step <label> <command...>
#
# The ONLY place a selected gate is recorded or a gate command is executed.
# Under --list the real selection logic still runs — every predicate below is
# evaluated exactly as it would be in a real run — and each reached gate records
# its label instead of executing. That is why --list cannot drift from a real
# run: it IS the real run with the work removed. Never reimplement the
# predicates in a separate list printer.
step() {
    local label="$1"; shift
    SELECTED+=("$label")
    if [ "$LIST" -eq 1 ]; then
        printf '%s\n' "$label"
        return 0
    fi
    printf '\n%s── %s%s\n' "$B" "$label" "$RST"
    if ! "$@"; then
        printf '%s✗ %s FAILED%s\n' "$RED" "$label" "$RST"
        echo "VERIFY: FAILED — $label" >&2
        exit 1
    fi
}

# skip <label> <reason>
#
# The only place a skipped gate is recorded. A skipped gate is announced in both
# --list and a real run (NFR-7): a truncated transcript must still make clear
# what did and did not run.
# The leading marker is a plain ASCII hyphen, and verify_test.sh greps for
# '^- '. Do not "improve" it to a Unicode minus: the script and its test would
# then have to agree on a character that is easy to mistype in one of the two.
skip() {
    local label="$1" reason="$2"
    SKIPPED+=("$label")
    printf -- '- %s (skipped: %s)\n' "$label" "$reason"
}

join_labels() {
    local out="" l
    for l in "$@"; do
        if [ -z "$out" ]; then out="$l"; else out="$out, $l"; fi
    done
    printf '%s' "$out"
}

# ------------------------------------------------------------------ gate 0

# The pin exists in two places — tools/toolchain.versions (which the linter gate
# and format-on-write.sh read) and .github/workflows/pr.yml (which CI reads).
# If they disagree, every downstream lint result is suspect, so this runs first
# and cheapest. It fails loudly when it cannot find the key at all: a drift
# check that silently matches nothing is worse than no drift check.
gate_drift() {
    local pinned actual
    # shellcheck source=./toolchain.versions
    . "$ROOT/tools/toolchain.versions"
    pinned="${GOLANGCI_LINT_VERSION:-}"
    if [ -z "$pinned" ]; then
        echo "tools/toolchain.versions: GOLANGCI_LINT_VERSION is unset" >&2
        return 1
    fi
    # Tolerates both block style and the inline-flow mapping pr.yml currently
    # uses: `with: { version: v2.12.2, working-directory: apps/backend }`.
    actual="$(awk '
        /golangci-lint-action/ { found = 1 }
        found && match($0, /version:[[:space:]]*v?[0-9][0-9A-Za-z.+-]*/) {
            print substr($0, RSTART, RLENGTH); exit
        }
    ' "$ROOT/.github/workflows/pr.yml" | sed 's/^version:[[:space:]]*//')"
    if [ -z "$actual" ]; then
        echo "could not find the golangci-lint-action version key in .github/workflows/pr.yml" >&2
        echo "the drift check will not pass vacuously — fix the extraction or the workflow" >&2
        return 1
    fi
    if [ "$pinned" != "$actual" ]; then
        echo "golangci-lint pin drift:" >&2
        echo "  tools/toolchain.versions   = $pinned" >&2
        echo "  .github/workflows/pr.yml   = $actual" >&2
        return 1
    fi
    echo "golangci-lint $pinned matches .github/workflows/pr.yml"
}

# ------------------------------------------------------------- backend gates

gate_go_test_race() { ( cd "$ROOT/apps/backend" && go test -race -count=1 ./... ); }
gate_go_test()      { ( cd "$ROOT/apps/backend" && go test -count=1 ./... ); }
gate_go_vet()       { ( cd "$ROOT/apps/backend" && go vet ./... ); }
gate_go_build()     { ( cd "$ROOT/apps/backend" && CGO_ENABLED=0 go build ./... ); }

# This script is the ONLY thing that provisions golangci-lint.
# .claude/hooks/format-on-write.sh reads the same cached path and never creates
# it — a PostToolUse hook that compiled a linter would stall every Write for a
# minute on a cold cache.
gate_golangci() {
    local v bin tmp
    # shellcheck source=./toolchain.versions
    . "$ROOT/tools/toolchain.versions"
    v="${GOLANGCI_LINT_VERSION}"
    bin="$ROOT/.cache/tools/bin/golangci-lint-$v"
    if [ ! -x "$bin" ]; then
        echo "bootstrapping golangci-lint $v into .cache/tools/bin"
        echo "(first run only — this compiles the linter and can take a minute;"
        echo " it is not a hung lint)"
        mkdir -p "$ROOT/.cache/tools/bin"
        tmp="$(mktemp -d)"
        if ! GOBIN="$tmp" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$v"; then
            rm -rf "$tmp"
            echo "failed to install golangci-lint $v" >&2
            return 1
        fi
        mv "$tmp/golangci-lint" "$bin"
        rm -rf "$tmp"
    fi
    ( cd "$ROOT/apps/backend" && "$bin" run )
}

# ------------------------------------------------------------ frontend gates

gate_npm_ci()     { ( cd "$ROOT/apps/frontend" && npm ci ); }
gate_npm_lint()   { ( cd "$ROOT/apps/frontend" && npm run lint ); }
gate_npm_format() { ( cd "$ROOT/apps/frontend" && npm run format ); }
gate_npm_test()   { ( cd "$ROOT/apps/frontend" && npm test ); }
gate_npm_build()  { ( cd "$ROOT/apps/frontend" && npm run build ); }

# npm writes node_modules/.package-lock.json on every install as a record of the
# installed tree, so comparing its mtime against package-lock.json is a real
# staleness signal rather than a proxy. Bare directory presence would let a
# stale tree mask a lockfile change — a dependency bump followed by --quick
# would report green on packages that were never installed.
npm_ci_needed() {
    local lock="$ROOT/apps/frontend/package-lock.json"
    local stamp="$ROOT/apps/frontend/node_modules/.package-lock.json"
    [ -f "$stamp" ] || return 0
    [ "$lock" -nt "$stamp" ] && return 0
    return 1
}

# ----------------------------------------------------------- container gate

# Failing loudly beats silently downgrading to a pass: reporting success for a
# gate that did not run is the single failure mode this script exists to
# prevent. The message names the one-flag workaround.
gate_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        echo "docker is not installed; re-run with --no-docker to skip this gate" >&2
        return 1
    fi
    if ! docker buildx version >/dev/null 2>&1; then
        echo "docker buildx is unavailable; re-run with --no-docker to skip this gate" >&2
        return 1
    fi
    docker buildx build --progress=plain \
        --platform linux/amd64,linux/arm64 \
        -f "$ROOT/deploy/docker/Dockerfile" "$ROOT"
}

# ------------------------------------------------------------- the registry

step 'toolchain drift' gate_drift

if [ "$QUICK" -eq 1 ]; then
    step 'go test' gate_go_test
else
    step 'go test -race' gate_go_test_race
fi
step 'go vet' gate_go_vet
step 'golangci-lint' gate_golangci
step 'go build' gate_go_build

if [ "$QUICK" -eq 1 ] && ! npm_ci_needed; then
    skip 'npm ci' '--quick, node_modules current'
else
    step 'npm ci' gate_npm_ci
fi
step 'npm run lint' gate_npm_lint
step 'npm run format' gate_npm_format
step 'npm test' gate_npm_test
step 'npm run build' gate_npm_build

if [ "$QUICK" -eq 1 ]; then
    skip 'docker buildx' '--quick'
elif [ "$NO_DOCKER" -eq 1 ]; then
    skip 'docker buildx' '--no-docker'
else
    step 'docker buildx' gate_docker
fi

# --------------------------------------------------------------- the verdict

if [ "$LIST" -eq 1 ]; then
    exit 0
fi

if [ "${#SKIPPED[@]}" -eq 0 ]; then
    printf '\n%sVERIFY: DONE — all gates passed; the branch may be called done.%s\n' "$GRN" "$RST"
else
    printf '\n%sVERIFY: PARTIAL — all selected gates passed, but %d were skipped (%s). This does NOT count as done.%s\n' \
        "$YEL" "${#SKIPPED[@]}" "$(join_labels "${SKIPPED[@]}")" "$RST"
fi
exit 0
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd <worktree> && chmod +x tools/verify.sh && ./tools/verify_test.sh
```

Expected: every line `ok   - ...`, final line `verify_test.sh: all assertions passed`, exit 0.

If a `--list` count assertion fails, count the `step`/`skip` calls in the registry
before changing the test — the registry is the contract, and the eleven-gate count
comes straight from FR-8.

- [ ] **Step 5: Confirm the script is cwd-independent (FR-16)**

```bash
cd <worktree>/apps/backend && ../../tools/verify.sh --list | head -3
cd /tmp && <worktree>/tools/verify.sh --list | head -3
```

Expected: identical output from both, starting with `toolchain drift`.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add tools/verify.sh tools/verify_test.sh \
  && git commit -m "feat(task-004): add tools/verify.sh with per-mode gate registry and tests"
```

---

### Task 3: Get the real gates green

Task 2 proved the driver works. This task proves the gates pass — including the
first `golangci-lint` bootstrap, which is the one step that can fail for reasons
that have nothing to do with `verify.sh`.

**Files:**
- Modify (only if a run surfaces a defect): `tools/verify.sh`, `tools/verify_test.sh`

**Interfaces:**
- Consumes: everything Task 2 produced.
- Produces: evidence for AC-7, AC-8, AC-9, AC-10 — pasted into your report file
  verbatim, because a later task's audit quotes it rather than re-running it.

- [ ] **Step 1: Capture the working-tree state before the run (AC-10)**

```bash
cd <worktree> && git status --porcelain > /tmp/hm-verify-before.txt \
  && wc -l < /tmp/hm-verify-before.txt
```

- [ ] **Step 2: Run the flagless gate**

```bash
cd <worktree> && ./tools/verify.sh
```

Expected: each gate announced as `── <label>`, and the final line
`VERIFY: DONE — all gates passed; the branch may be called done.`, exit 0.

Give this a generous timeout — 15 minutes. The first `golangci-lint` bootstrap
compiles the linter, `go test -race` takes about a minute, and the two-platform
buildx build takes several.

If a gate fails, that is a real finding. **Do not weaken the gate to make it
pass.** Report the failure with the verbatim output and stop; a red gate on
arrival is information the task owner needs, and PRD §9 Q1 already ruled the
one known category (eslint warnings) out of scope — `npm run lint` is bare
`eslint .` and exits 0 on warnings, so warnings will not fail it.

- [ ] **Step 3: Confirm the tree is unchanged (AC-10 / NFR-3)**

```bash
cd <worktree> && git status --porcelain > /tmp/hm-verify-after.txt \
  && diff /tmp/hm-verify-before.txt /tmp/hm-verify-after.txt && echo "TREE UNCHANGED"
```

Expected: `TREE UNCHANGED`, exit 0. In particular `.cache/` must not appear —
Task 1 ignored it.

- [ ] **Step 4: Run the two flagged modes and confirm each says "not done" (AC-8)**

```bash
cd <worktree> && ./tools/verify.sh --quick
cd <worktree> && ./tools/verify.sh --no-docker
```

Expected from each: exit 0 and a terminal line beginning `VERIFY: PARTIAL` and
containing `This does NOT count as done.` — with the skipped labels named.

- [ ] **Step 5: Confirm `--help` (AC-9)**

```bash
cd <worktree> && ./tools/verify.sh --help
```

Expected: exit 0; the output documents `--quick`, `--no-docker`, `--list`, and
names both `HARBORMASTER_INTEGRATION` and `npm run test:e2e` as on-demand.

- [ ] **Step 6: Re-run the test suite and commit any fixes**

```bash
cd <worktree> && ./tools/verify_test.sh
```

Expected: exit 0. If Steps 2–5 required a change to `verify.sh`, commit it:

```bash
cd <worktree> && git add tools/verify.sh tools/verify_test.sh \
  && git commit -m "fix(task-004): correct verify.sh against a real gate run"
```

If nothing changed, say so in your report and create no commit.

- [ ] **Step 7: Record the evidence**

Paste into your report file, verbatim: the `VERIFY:` line from each of the three
runs, the `TREE UNCHANGED` result, and the wall-clock duration of the flagless
run versus the `--quick` run (NFR-1).

---

### Task 4: Port the task tooling scripts

Three verbatim ports. `task-numbers.sh` and its test encode zero repo-specific
paths — the script resolves the main repo root from
`git rev-parse --git-common-dir` and the test builds throwaway repos under
`mktemp -d` — so FR-21 is a no-op here. `task-brief.sh` resolves its workspace
from `git rev-parse --show-toplevel` and writes its own self-ignoring
`.gitignore`, so no `.gitignore` change is needed.

Verify that claim before copying rather than assuming it.

**Files:**
- Create: `tools/task-numbers.sh` (mode `755`)
- Create: `tools/task-numbers_test.sh` (mode `755`)
- Create: `tools/task-brief.sh` (mode `755`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `tools/task-numbers.sh {next|check|list}`. `next` prints the smallest unused
    zero-padded three-digit number; `check` exits 1 and reports on stderr when
    one number has more than one distinct task ID; `list` prints
    `NNN <task-id> <source>` per assignment. `.claude/hooks/task-num-collision-detector.sh`
    (Task 5) calls `check`; `.claude/commands/spec-task.md` (Task 14) calls `next`.
  - `tools/task-brief.sh PLAN_FILE TASK_NUMBER [OUTFILE]`, defaulting to
    `<repo-root>/.superpowers/sdd/<plan-basename>/task-<N>-brief.md`. Exit 2 on a
    usage error, 3 when no `Task <N>` heading exists. It slices on the regex
    `^#+[ \t]+Task[ \t]+<N>([^0-9]|$)`, which is why every task heading in this
    plan is `### Task N:`. `.claude/hooks/commit-boundary.sh` (Task 5),
    `.claude/agents/task-implementer.md` (Task 6) and
    `.claude/commands/execute-task.md` (Task 14) all reference it.

- [ ] **Step 1: Confirm the three files carry no atlas-specific paths**

```bash
cd <worktree> && grep -nE 'services/|libs/|atlas' \
  "$ATLAS/tools/task-numbers.sh" \
  "$ATLAS/tools/task-numbers_test.sh" \
  "$ATLAS/tools/task-brief.sh"
```

Expected: matches only in comments (`task-brief.sh` has a comment block
explaining why it is vendored). If a match lands on an executable line, stop and
report it — the verbatim-port assumption in the design would be wrong and the
adaptation needs to be planned, not improvised.

- [ ] **Step 2: Copy the three files**

```bash
cd <worktree> && cp "$ATLAS/tools/task-numbers.sh" \
                   "$ATLAS/tools/task-numbers_test.sh" \
                   "$ATLAS/tools/task-brief.sh" tools/ \
  && chmod +x tools/task-numbers.sh tools/task-numbers_test.sh tools/task-brief.sh
```

Copy with `cp`. No reformatting, no shebang normalisation, no "while I'm here"
edits — `process-parity.md` §7 check 1 measures byte-identity across four repos
and a single whitespace change destroys it.

- [ ] **Step 3: Run `task-numbers_test.sh` (FR-20 / AC-6)**

```bash
cd <worktree> && ./tools/task-numbers_test.sh
```

Expected: exit 0.

- [ ] **Step 4: Exercise the subcommands against this repository**

```bash
cd <worktree> && ./tools/task-numbers.sh list
cd <worktree> && ./tools/task-numbers.sh next
cd <worktree> && ./tools/task-numbers.sh check; echo "check exit: $?"
```

Expected: `list` shows `001`–`004`; `next` prints `005`; `check` exits 0 and
prints nothing. If `check` reports a collision, that is a real finding in this
repository — report it, do not paper over it.

- [ ] **Step 5: Exercise `task-brief.sh` against this very plan**

```bash
cd <worktree> && ./tools/task-brief.sh docs/tasks/task-004-process-parity-harness/plan.md 4
```

Expected: `wrote .../\.superpowers/sdd/plan/task-4-brief.md: <n> lines`, exit 0,
and the brief contains this task's heading and steps.

```bash
cd <worktree> && ./tools/task-brief.sh docs/tasks/task-004-process-parity-harness/plan.md 999; echo "exit: $?"
cd <worktree> && git status --porcelain
```

Expected: exit 3 with `task 999 not found ...`, and `git status --porcelain`
prints nothing — `.superpowers/sdd/.gitignore` ignores its own directory.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add tools/task-numbers.sh tools/task-numbers_test.sh tools/task-brief.sh \
  && git commit -m "feat(task-004): port task-numbers.sh, its test, and task-brief.sh"
```

---

### Task 5: Port the nine hooks

Eight portable hooks copied verbatim (with one deliberate two-string exception),
plus `format-on-write.sh` rebound to Harbormaster's layout.
`skill-activation-prompt.{py,sh}` must be left untouched (FR-3).

Nothing is wired yet — that is Task 6. A hook file on disk with no
`settings.json` entry does nothing, which is what lets this task and the next be
reviewed separately.

**Files:**
- Create: `.claude/hooks/wait-loop-guard.sh`, `wait-loop-guard_test.sh`,
  `block-home-paths-in-docs.sh`, `turn-budget.sh`, `turn-budget-guard.sh`,
  `fork-dispatch-guard.sh`, `commit-boundary.sh`,
  `task-num-collision-detector.sh` (all mode `755`)
- Create: `.claude/hooks/format-on-write.sh` (mode `755`, rebound)
- Do not touch: `.claude/hooks/skill-activation-prompt.py`,
  `.claude/hooks/skill-activation-prompt.sh`

**Interfaces:**
- Consumes: `tools/toolchain.versions` (Task 1) — `format-on-write.sh` sources it;
  `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` (Task 2 creates it,
  Task 3 populates it) — `format-on-write.sh` reads it and never creates it;
  `tools/task-numbers.sh` (Task 4) — `task-num-collision-detector.sh` calls
  `check` and exits 0 silently when the script is absent or non-executable;
  `tools/task-brief.sh` (Task 4) — `commit-boundary.sh` names it in its guidance.
- Produces: nine hook scripts for Task 6 to wire, at the exact paths above.

- [ ] **Step 1: Copy the seven files that port byte-identically**

```bash
cd <worktree> && cp \
  "$ATLAS/.claude/hooks/wait-loop-guard.sh" \
  "$ATLAS/.claude/hooks/block-home-paths-in-docs.sh" \
  "$ATLAS/.claude/hooks/turn-budget.sh" \
  "$ATLAS/.claude/hooks/turn-budget-guard.sh" \
  "$ATLAS/.claude/hooks/fork-dispatch-guard.sh" \
  "$ATLAS/.claude/hooks/commit-boundary.sh" \
  "$ATLAS/.claude/hooks/task-num-collision-detector.sh" \
  .claude/hooks/
```

`cp` only. No edits of any kind to these seven.

- [ ] **Step 2: Copy `wait-loop-guard_test.sh` and rebind its two fixture strings**

`wait-loop-guard_test.sh` is the one file where AC-1 and `process-parity.md`
§7 check 1 conflict, and the conflict is real: the file contains two allow-list
fixtures that name atlas, one of which matches `atlas-`.

```bash
cd <worktree> && cp "$ATLAS/.claude/hooks/wait-loop-guard_test.sh" .claude/hooks/
cd <worktree> && grep -n 'atlas' .claude/hooks/wait-loop-guard_test.sh
```

Expected, before the edit — two lines:

```
allow 'kubectl get pods -n atlas-pr-1370'
allow 'journalctl -u atlas --since "5 min ago" | tail -50'
```

Edit exactly those two strings to:

```
allow 'kubectl get pods -n harbormaster-pr-1370'
allow 'journalctl -u harbormaster --since "5 min ago" | tail -50'
```

Change nothing else in the file. Neither the namespace nor the unit name affects
the guard's decision — both commands are on the allow list because they are
one-shot reads, not polls — so FR-2 still holds. `wait-loop-guard.sh` itself, the
file whose byte-identity actually matters for a future re-harmonisation, stays
untouched.

- [ ] **Step 3: Verify FR-1 / AC-1 for the eight**

```bash
cd <worktree> && grep -l 'atlas-' .claude/hooks/*.sh; echo "exit: $?"
```

Expected: no filenames printed, exit 1 (grep found nothing). `format-on-write.sh`
is not in place yet; Step 5 re-runs this.

- [ ] **Step 4: Run `wait-loop-guard_test.sh` (FR-2 / AC-2)**

```bash
cd <worktree> && chmod +x .claude/hooks/*.sh && ./.claude/hooks/wait-loop-guard_test.sh
```

Expected: exit 0, every assertion passing.

- [ ] **Step 5: Write the rebound `format-on-write.sh`**

Copy `$ATLAS/.claude/hooks/format-on-write.sh` and make exactly three edits.
The file's fail-open contract — missing toolchain, missing cached binary,
unparseable input, tool error all exit 0 silently — is preserved verbatim, and it
must **never** bootstrap anything. Only `tools/verify.sh` provisions the linter;
a PostToolUse hook that compiled it would stall every Write for a minute.

Edit 1 — the prettier arm. Replace:

```bash
    */services/atlas-ui/*.ts|*/services/atlas-ui/*.tsx)
        (cd "$ROOT/services/atlas-ui" && npx --no-install prettier --write "$fp") >/dev/null 2>&1 || true
        ;;
```

with:

```bash
    */apps/frontend/*.ts|*/apps/frontend/*.tsx)
        (cd "$ROOT/apps/frontend" && npx --no-install prettier --write "$fp") >/dev/null 2>&1 || true
        ;;
```

Edit 2 — the golangci config path. Harbormaster's `.golangci.yml` lives at
`apps/backend/.golangci.yml`, not at the repo root. Replace:

```bash
        (cd "$moddir" && "$GOLANGCI" fmt -c "$ROOT/.golangci.yml" "$fp") >/dev/null 2>&1 || true
```

with:

```bash
        (cd "$moddir" && "$GOLANGCI" fmt -c "$ROOT/apps/backend/.golangci.yml" "$fp") >/dev/null 2>&1 || true
```

Edit 3 — the header comment. Atlas's says the binary is cached by
`tools/lint.sh`, which this repo does not have. Replace `tools/lint.sh has
already cached it.` with `tools/verify.sh has already cached it.`

The dirname-walk that finds the enclosing `go.mod` is kept even though this repo
has exactly one Go module: it is fail-open, costs nothing, and stays correct if a
second module ever appears.

- [ ] **Step 6: Verify the rebind and re-check AC-1 across all nine**

```bash
cd <worktree> && grep -n 'apps/frontend\|apps/backend/.golangci.yml\|toolchain.versions' \
  .claude/hooks/format-on-write.sh
cd <worktree> && grep -l 'atlas-' .claude/hooks/*.sh; echo "exit: $?"
cd <worktree> && grep -rn 'services/atlas-ui' .claude/hooks/; echo "exit: $?"
```

Expected: the first prints the three rebound references (AC-3); the second and
third print nothing and exit 1.

- [ ] **Step 7: Confirm the untouched files are untouched (FR-3)**

```bash
cd <worktree> && git status --porcelain .claude/hooks/
```

Expected: only `??` entries for the nine new files. `skill-activation-prompt.py`
and `skill-activation-prompt.sh` must not appear at all.

- [ ] **Step 8: Smoke-test the two hooks that read repo state**

```bash
cd <worktree> && echo '{}' | ./.claude/hooks/task-num-collision-detector.sh; echo "exit: $?"
cd <worktree> && printf '{"tool_input":{"file_path":"%s/docs/README-probe.md","content":"see /home/someone/x"}}' "$PWD" \
  | ./.claude/hooks/block-home-paths-in-docs.sh; echo "exit: $?"
```

Expected: the collision detector exits 0 and prints nothing (no collisions). The
home-path guard exits non-zero or emits a block decision for the planted absolute
path — read the script to confirm which shape it uses, and record what you
observed. This is the hook that will police Tasks 9–13, so knowing its exact
refusal shape now saves a surprise later.

- [ ] **Step 9: Record the hashes for the AC-23 report**

```bash
cd <worktree> && sha256sum \
  .claude/hooks/wait-loop-guard.sh \
  .claude/hooks/wait-loop-guard_test.sh \
  .claude/hooks/block-home-paths-in-docs.sh \
  .claude/hooks/turn-budget.sh \
  .claude/hooks/turn-budget-guard.sh \
  .claude/hooks/fork-dispatch-guard.sh \
  .claude/hooks/commit-boundary.sh \
  .claude/hooks/task-num-collision-detector.sh
```

Paste the output verbatim into your report file. Task 15 quotes it.

- [ ] **Step 10: Commit**

```bash
cd <worktree> && git add .claude/hooks/ \
  && git commit -m "feat(task-004): port the eight portable hooks and rebind format-on-write"
```

---

### Task 6: Wire the hooks in `.claude/settings.json`

This activates everything Task 5 put on disk — including
`block-home-paths-in-docs.sh`, which then polices the documentation tasks. That
ordering is deliberate: a home path in a ported document should be rejected at
the moment of writing, not discovered at the AC-22 sweep.

**Files:**
- Modify: `.claude/settings.json`

**Interfaces:**
- Consumes: the nine hook files from Task 5, at
  `$CLAUDE_PROJECT_DIR/.claude/hooks/<name>.sh`.
- Produces: an active hook set matching atlas's — `PreToolUse` on `Write|Edit`,
  `Agent`, `Bash` and `*`; `PostToolUse` on `Write|Edit`, `*` and `Bash`;
  `SessionStart`; `UserPromptSubmit`. `turn-budget-guard.sh` becoming binding is
  what makes `task-implementer`'s 120-call budget (Task 7) real rather than
  advisory.

- [ ] **Step 1: Write the full file**

Replace `.claude/settings.json` with exactly this. It preserves the existing
`UserPromptSubmit` → `skill-activation-prompt.sh` wiring and the `enabledPlugins`
block verbatim (FR-30), and adds `disableBundledSkills` plus the seven new
entries (FR-29).

```json
{
  "disableBundledSkills": true,
  "permissions": {
    "allow": [],
    "deny": [],
    "ask": []
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/block-home-paths-in-docs.sh"
          }
        ]
      },
      {
        "matcher": "Agent",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/fork-dispatch-guard.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/wait-loop-guard.sh"
          }
        ]
      },
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/turn-budget-guard.sh"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/format-on-write.sh"
          }
        ]
      },
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/turn-budget.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/commit-boundary.sh"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/task-num-collision-detector.sh"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/skill-activation-prompt.sh"
          }
        ]
      }
    ]
  },
  "enabledPlugins": {
    "superpowers@claude-plugins-official": true
  }
}
```

- [ ] **Step 2: Verify it parses and asserts the required keys (AC-14)**

```bash
cd <worktree> && jq -e '.disableBundledSkills == true' .claude/settings.json
cd <worktree> && jq -r '.hooks.PreToolUse[].matcher' .claude/settings.json
cd <worktree> && jq -r '.hooks.PostToolUse[].matcher' .claude/settings.json
cd <worktree> && jq -e '.enabledPlugins["superpowers@claude-plugins-official"] == true' .claude/settings.json
cd <worktree> && jq -r '.hooks.UserPromptSubmit[].hooks[].command' .claude/settings.json
```

Expected: `true`; `Write|Edit`, `Agent`, `Bash`, `*`; `Write|Edit`, `*`, `Bash`;
`true`; a path ending `skill-activation-prompt.sh`.

- [ ] **Step 3: Verify every wired command points at a file that exists**

```bash
cd <worktree> && jq -r '.hooks | to_entries[] | .value[] | .hooks[] | .command' .claude/settings.json \
  | sed "s|\$CLAUDE_PROJECT_DIR|$PWD|" \
  | while read -r p; do [ -x "$p" ] && echo "ok   $p" || echo "MISSING $p"; done
```

Expected: nine `ok` lines, no `MISSING`.

- [ ] **Step 4: Diff against atlas's wiring (FR-31)**

```bash
cd <worktree> && diff \
  <(jq -S '.hooks | walk(if type == "string" then sub("^\\$CLAUDE_PROJECT_DIR"; "") else . end)' .claude/settings.json) \
  <(jq -S '.hooks | walk(if type == "string" then sub("^\\$CLAUDE_PROJECT_DIR"; "") else . end)' "$ATLAS/.claude/settings.json")
```

Expected: no output. If `jq` lacks `walk`, compare the two `jq -S '.hooks'`
outputs directly and confirm the only differences are none. Any difference is a
finding — record it and explain it rather than silently accepting it.

- [ ] **Step 5: Commit**

```bash
cd <worktree> && git add .claude/settings.json \
  && git commit -m "feat(task-004): wire the enforcement hooks and disable bundled skills"
```

---

### Task 7: The agent trio

`task-implementer`, `task-verifier`, `task-reviewer` — ported with targeted
rebinding to Harbormaster's shape. Together they replace the uncapped generic
dispatch `/execute-task` falls back to today: a budget-capped implementer that
hands back `PARTIAL`, a verifier that runs the gate in its own clean context, and
a per-unit reviewer.

The four existing agent files must be byte-unchanged (FR-26 / AC-12).

**Files:**
- Create: `.claude/agents/task-implementer.md`
- Create: `.claude/agents/task-verifier.md`
- Create: `.claude/agents/task-reviewer.md`
- Do not touch: `.claude/agents/backend-guidelines-reviewer.md`,
  `frontend-guidelines-reviewer.md`, `plan-adherence-reviewer.md`,
  `todo-scanner.md`

**Interfaces:**
- Consumes: `tools/verify.sh`'s flag surface and verdict lines (Task 2);
  `tools/task-brief.sh` (Task 4); `docs/slice-first.md`, `docs/agent-dispatch.md`,
  `docs/review-protocol.md` (Tasks 9–11 — these links must resolve by Task 15,
  so write them now and let Task 15's link sweep catch any that do not).
- Produces: three `subagent_type` values — `task-implementer`, `task-verifier`,
  `task-reviewer` — that `.claude/commands/execute-task.md` (Task 14) and
  `.claude/commands/fix-pr-bug.md` (Task 8) dispatch by name.
- Produces: the per-unit review artifact path convention
  `docs/tasks/task-NNN-slug/reviews/<unit>.md`, which must NOT collide with the
  existing reviewers' `docs/tasks/task-NNN-slug/audit.md`.
  `docs/review-protocol.md` (Task 11) records that split.

- [ ] **Step 1: Copy the three files verbatim as a starting point**

```bash
cd <worktree> && cp "$ATLAS/.claude/agents/task-implementer.md" \
                   "$ATLAS/.claude/agents/task-verifier.md" \
                   "$ATLAS/.claude/agents/task-reviewer.md" .claude/agents/
```

- [ ] **Step 2: Rebind `task-verifier.md`**

Keep verbatim: the YAML frontmatter (`name`, `model: haiku`,
`tools: Bash, Read`), the `## Process` numbered steps, the "You do not fix
anything" and "You do not run anything else" paragraphs, the three report blocks,
and the closing rule "Never report PASS for a run that did not complete. An unrun
gate is `ERROR`, never `PASS`."

Change:

1. "You run Atlas's verification gate" → "You run Harbormaster's verification gate".
2. In `## Inputs`, delete every mention of `--base`. The flag does not exist in
   this repo's `verify.sh`. Replace the whole "Command to run" bullet with:

   > - **Command to run** — defaults to `tools/verify.sh --quick` when the
   >   controller does not name one. Run exactly what you were given.
   >
   >   **Quote the script's terminal `VERIFY:` line verbatim in your report.** The
   >   exit code is not the whole answer: `tools/verify.sh` exits 0 for both
   >   `VERIFY: DONE — all gates passed; the branch may be called done.` and
   >   `VERIFY: PARTIAL — ... This does NOT count as done.`, and only the verdict
   >   line distinguishes a done branch from a run that skipped the container
   >   build, the race detector, or `npm ci`. Reporting "exited 0" without the
   >   verdict line is under-reporting.

3. Delete the paragraph beginning "If the command you were given omits `--base`
   and the script warns that a shared-lib change fanned out" — there is no
   change-gating and no fan-out here.
4. In the PASS block, replace `Checks: <the script's own passed-list summary
   line, verbatim>` with `Verdict: <the script's terminal VERIFY: line, verbatim>`.
5. In the FAIL block, replace `Failed checks: <the script's own failed-list,
   verbatim>` with `Verdict: <the script's `VERIFY: FAILED — <label>` line, verbatim>`,
   and replace the "If four checks failed, name all four" rule with:

   > `tools/verify.sh` stops at the first failing gate, so there is exactly one.
   > Quote up to 40 lines of that gate's output. If it exceeds 40 lines, quote the
   > first 20 and the last 20 with `[... N lines elided ...]` between them.

6. Update the two `<example>` blocks to name a Harbormaster task rather than an
   atlas one, and drop the `--base` invocation from the first.
7. Reduce the Step 2 timeout guidance from "the gate is change-gated but can
   still take minutes" to "`--quick` takes a couple of minutes; a flagless run
   including the two-platform container build takes considerably longer."

- [ ] **Step 3: Rebind `task-implementer.md`**

Keep verbatim, word for word: Contract 1 in full (the 120 tool-call budget, the
warn-at-100 hook coupling, the `PARTIAL` hand-back and everything it must
contain, "A silent 400-turn push-through is the failure mode this contract exists
to prevent"), the "You Do Not Dispatch Subagents" section, "When You're in Over
Your Head", "Before Reporting: Self-Review", "After Review Findings", and the
full Report Format including the status table. These are repo-agnostic and are
the reason the agent exists.

Change:

1. "one task from an Atlas plan.md" / "an Atlas implementation plan" → "a
   Harbormaster implementation plan". Update the two `<example>` blocks to name
   a Harbormaster task.
2. Contract 2's module-local command block. Replace:

   ```sh
   cd <worktree>/services/atlas-<svc>/atlas.com/<svc> && go build ./... && go test ./...
   ```

   and its "For a `libs/` change, the same two commands from that library's
   module root" follow-on, with:

   ```sh
   # Go task — Harbormaster has exactly one Go module.
   cd <worktree>/apps/backend && go build ./... && go test ./...

   # Frontend task.
   cd <worktree>/apps/frontend && npm run lint && npm test
   ```

   plus this sentence, which replaces the `libs/` fan-out guidance rather than
   dropping it: "There is one Go module and no `go.work`, so every Go change is
   local to `apps/backend` — there is no fan-out to reason about. Do not run
   `npm ci` or `npm run build`; those are the repo-wide gate's, not yours."

3. Contract 2's prohibition list. Replace `tools/lint.sh` with `golangci-lint`,
   replace `docker buildx bake / any docker command` with `docker buildx build /
   any docker command`, and replace "repo-wide `go build ./...` across modules or
   `go vet` sweeps" with "`go vet ./...` sweeps". Keep `tools/verify.sh` (any
   flag, including `--quick`) at the top of the list — that prohibition is the
   whole point of the trio and is kept verbatim.
4. Contract 3's slicing bullet. `tools/doc-slice.sh` does not exist here. Replace
   the three-command block with:

   ```sh
   grep -n '^#' <path>                 # the document's shape
   sed -n '120,180p' <path>            # the one section you need
   grep -n -B4 -A12 '<needle>' <path>  # a needle in an offloaded tool result
   ```

   and add: "or `Read` with `offset`/`limit` for the harness-native form."
   Keep the rest of the bullet — the measured justification, "This is a default
   with an escalation path, not a ban", and "Source files you are about to edit
   are not 'large reference documents' — read those" — and keep the
   `docs/slice-first.md` link.
5. The Go-module-path bullet ("To read a dependency's source, ask the toolchain
   for its path"). Keep the rule and `go list -m -f '{{.Dir}}'` / `go doc` /
   "never root a `find` at `/`". Delete the atlas-specific
   `!chronicle20` module-cache and `libs/` `replace` illustrations, and replace
   them with: "Run `go list` from `apps/backend`, the only module here."
6. "Atlas Code Discipline" → "Harbormaster Code Discipline". Replace the
   `libs/atlas-constants/` bullet with: "Before adding a domain type, a shared
   helper, or a numeric constant, check `apps/backend/internal/` for an existing
   equivalent — the `backend-dev-guidelines` skill's checklist is the authority
   on where it belongs." Keep every other bullet in that section verbatim,
   including "Never invent values or names. Unverified is 'unknown', not a
   plausible guess." — and add one Harbormaster-specific line: "Verify MinIO
   admin API contracts and configuration values against local source or upstream
   MinIO docs; never cite them from memory."

- [ ] **Step 4: Rebind `task-reviewer.md`**

Keep verbatim: the frontmatter (`model: sonnet`, `tools: Read, Grep, Glob, Bash,
Write`), the "It is NOT a substitute for the guideline reviewers" paragraph
(updating only the checklist names to `DOM-*`/`SUB-*`/`SEC-*` and `FE-*`, which
are Harbormaster's), the `## Scope` section, "Assume a check FAILS until you find
the line that proves otherwise", the `## Do not fan out` section with its
measured justification, the output block, and the four `## Important rules`.

Change:

1. In `## Input`, keep the derived artifact path
   `docs/tasks/<task>/reviews/<unit>.md` and add: "This is deliberately not
   `docs/tasks/<task>/audit.md` — that path belongs to
   `plan-adherence-reviewer`, `backend-guidelines-reviewer` and
   `frontend-guidelines-reviewer`, and a per-unit review must not overwrite it.
   See `docs/review-protocol.md`."
2. In `## Discovery — slice first`, drop `tools/doc-slice.sh` and replace it with
   `sed -n 'A,Bp'` / `grep -n -A/-B`, keeping the `docs/slice-first.md` link and
   the measured justification.
3. In `## What to look for`, item 3 "Cross-service seams". Harbormaster has no
   multi-service event topology, so rebind the *rule* rather than dropping it —
   this is the FR-34 case. Replace the atlas illustration with:

   > 3. **Seams the gate cannot see.** A backend handler and the frontend query
   >    that consumes it; a MinIO admin API call whose response shape changed; a
   >    JSON:API document a test pins to the old attribute set; a migration and
   >    the entity that reads it. When the unit crosses one of these, trace it by
   >    hand into the consumer and check that a test asserts the NEW contract.

4. Update the two `<example>` blocks to Harbormaster units.

- [ ] **Step 5: Verify no atlas assumptions survived**

```bash
cd <worktree> && grep -niE 'atlas|services/|libs/|go\.work|doc-slice|--base|lint\.sh|buildx bake' \
  .claude/agents/task-implementer.md .claude/agents/task-verifier.md .claude/agents/task-reviewer.md
```

Expected: nothing. Any hit is either a missed rebind or a rule you dropped
instead of rebinding — FR-34 forbids the latter.

- [ ] **Step 6: Verify the four existing agents are byte-unchanged (AC-12)**

```bash
cd <worktree> && git status --porcelain .claude/agents/
```

Expected: exactly three `??` lines for the new files. No `M` lines.

- [ ] **Step 7: Commit**

```bash
cd <worktree> && git add .claude/agents/task-implementer.md \
                        .claude/agents/task-verifier.md \
                        .claude/agents/task-reviewer.md \
  && git commit -m "feat(task-004): add the task-implementer/verifier/reviewer trio"
```

---

### Task 8: `service-documentation` agent, `/service-doc`, and `/fix-pr-bug`

Two ports with substantive rebinding. `service-documentation` assumes a
`services/*` tree and a `DOCS.md` contract, neither of which exists here;
`fix-pr-bug` calls two atlas scripts that are not being ported.

**Files:**
- Create: `.claude/agents/service-documentation.md`
- Create: `.claude/commands/service-doc.md`
- Create: `.claude/commands/fix-pr-bug.md`

**Interfaces:**
- Consumes: `task-implementer`, `task-verifier`, `task-reviewer` (Task 7);
  `tools/verify.sh --quick` (Task 2). Links to `docs/post-implementation.md` and
  `docs/observability.md` (Tasks 11 and 12).
- Produces: `/service-doc <component>` writing `docs/architecture/<name>.md`, and
  `/fix-pr-bug <task> <slug>` writing `docs/tasks/<task>/bug-<slug>.md`.

- [ ] **Step 1: Write `.claude/agents/service-documentation.md`**

Start from `$ATLAS/.claude/agents/service-documentation.md`. Keep the
frontmatter shape (`model: sonnet`, `tools: Read, Grep, Glob, Write, Edit, Bash`),
the `## Strict Rules` MUST/MUST NOT lists, `## Scope`, and `## Output` verbatim
— those are the rules, and they transfer unchanged.

Two rebinds:

1. **The unit of documentation.** Harbormaster has no `services/*` tree, so an
   agent that resolves its argument under `services/` could never be invoked.
   Replace the `## Task` section's argument resolution with:

   > Argument shape: a **component** name or path. Resolve in this order:
   >
   > 1. a backend domain package — `apps/backend/internal/<name>`
   >    (`buckets`, `objects`, `policies`, `users`, `auth`, `jobs`, `metrics`,
   >    `sse`, `lifecycle`, `audit`, `dashboard`, …);
   > 2. a frontend feature — `apps/frontend/src/features/<name>`;
   > 3. the whole backend or frontend app, when the argument is `backend` or
   >    `frontend`.
   >
   > Output goes to `docs/architecture/<name>.md`, beside the existing
   > `docs/architecture/overview.md`.

   Also change `## Scope`'s "Operate only within the target service directory" to
   "Operate only within the resolved component directory", and `## Strict Rules`'
   "Merge documentation concerns across services" to "Merge documentation
   concerns across components".

2. **The documentation contract.** There is no `DOCS.md` in this repo. Replace
   both references to it with `docs/architecture/overview.md`, described as the
   structural exemplar rather than a contract file, and add a
   `## Documentation contract` section stating the required structure explicitly:

   > Every component document has, in this order: a one-paragraph **Purpose**; a
   > **Public surface** section (exported Go identifiers, or the feature's
   > exported components and hooks); a **Data** section (entities, migrations,
   > or query keys) when the component owns any; a **Dependencies** section
   > naming the other components it calls and, for the backend, the MinIO admin
   > API operations it issues; and a **Failure modes** section. Omit a section
   > only when the component genuinely has nothing under it, and say so in one
   > line rather than deleting the heading.

Keep the agent name `service-documentation` unchanged. Renaming it to
`component-documentation` would break the one thing `process-parity.md` §7
checks 4 and 6 assert — that the four repos carry the same agent and command set
— for a cosmetic gain.

Update the two `<example>` blocks to `/service-doc buckets` and a re-document
request for `apps/backend/internal/buckets`.

- [ ] **Step 2: Write `.claude/commands/service-doc.md`**

Start from `$ATLAS/.claude/commands/service-doc.md` — it is 474 bytes and mostly
transfers. Rewrite it as:

```markdown
---
description: Generate or update documentation for one Harbormaster component — dispatches the service-documentation agent
argument-hint: Component name or path (e.g., "buckets", "apps/backend/internal/buckets", "backend", "frontend")
---

Dispatch the `service-documentation` agent against: **$ARGUMENTS**.

Harbormaster has no `services/*` tree — the unit of documentation is a
**component**: a backend domain package under `apps/backend/internal/`, a
frontend feature under `apps/frontend/src/features/`, or a whole app
(`backend` / `frontend`). Output lands at `docs/architecture/<name>.md`.

The agent treats code as the single source of truth, follows the structure of
[`docs/architecture/overview.md`](../../docs/architecture/overview.md) and the
documentation contract in its own definition, and operates only within the
resolved component directory. It outputs only updated doc files — no
commentary, no analysis.
```

- [ ] **Step 3: Write `.claude/commands/fix-pr-bug.md`**

Start from `$ATLAS/.claude/commands/fix-pr-bug.md`. Keep verbatim: the
frontmatter shape, the framing paragraph, Step 3 in full (the bug file is the
boundary; write it before dispatching anything; the `## Fix` inventory is what
removes the fix agent's discovery phase; do not guess a root cause), Step 4's
dispatch contract, Step 7's handoff question and the ~150k rule, and the
`## Important rules` list.

Rebind:

1. **Step 1** calls `tools/task-facts.sh`, which is not being ported. Replace the
   command block with the manual procedure, stated as a procedure rather than
   left as a gap:

   > Resolve the task by fuzzy identifier the way the phase commands do — glob
   > `docs/tasks/task-*` and `.worktrees/*/docs/tasks/task-*`, match on exact
   > name, bare number, or slug fragment. Then read, in one pass: the worktree
   > path (`git worktree list`), the branch (`git branch --show-current`), which
   > of `prd.md` / `design.md` / `plan.md` / `context.md` / `audit.md` exist, and
   > the changed surfaces (`git diff --name-only main...HEAD`).
   >
   > `cd` into the worktree yourself if `pwd` does not match. Do not create a
   > worktree.

2. **Step 2** assumes Kubernetes pods, a tenant, and a game client version.
   Rebind to Harbormaster's deploy story, keeping the rule (read the logs before
   changing code) and replacing the illustration:

   > Reproduction is interactive and stays here. Confirm the MinIO deployment and
   > the exact Harbormaster version first. Read the service logs before anything
   > else — `docker compose -f deploy/docker/docker-compose.yml logs harbormaster`
   > for the Compose stack, or `kubectl logs deploy/harbormaster` for the
   > Kubernetes one. See [`docs/observability.md`](../../docs/observability.md).
   > Read the logs for the workload you name, never a whole-namespace listing.

3. **Step 5** uses `--base` and `$CLAUDE_JOB_DIR`. Replace the command block with:

   ```sh
   tools/verify.sh --quick > /tmp/gate-<slug>.log 2>&1
   ```

   with `run_in_background: true`, or dispatch `task-verifier` (`model: haiku`)
   for a summarized verdict. Keep the "**keep going** — do not poll it" rule.
   Replace the "crosses a service boundary" sentence with "crosses the
   backend/frontend seam or changes a JSON:API contract — the gate cannot see a
   seam defect."

4. **Step 6** calls `tools/agent-ledger.sh`, which is not being ported. The rule
   it serves — a bug file that never records its resolution is the next session's
   rediscovery — survives; the mechanism becomes the file itself:

   > ## Step 6 — Record the outcome in the bug file
   >
   > There is no separate ledger in this repository; the bug file is the record.
   > Append to `docs/tasks/<task>/bug-<slug>.md`: the commit that fixed it, the
   > agents dispatched and their statuses, the verifier's `VERIFY:` line, any
   > reviewer verdict, and whether a live re-test confirmed the fix. Commit that
   > update. A bug file that never records its resolution is the next session's
   > rediscovery.

5. Retarget the two doc links to this repo's paths (`docs/post-implementation.md`,
   `docs/observability.md`) — both land in Tasks 11 and 12, and Task 15's link
   sweep will catch it if either is missing. Change "Phase 5 of the Atlas
   workflow" to "Phase 5 of the Harbormaster workflow" and the argument hint to a
   Harbormaster example.

- [ ] **Step 4: Verify no unported tool is still referenced**

```bash
cd <worktree> && grep -nE 'task-facts\.sh|agent-ledger\.sh|task-resolve\.sh|change-surfaces\.sh|plan-context\.sh|doc-slice\.sh|--base|DOCS\.md|services/' \
  .claude/agents/service-documentation.md .claude/commands/service-doc.md .claude/commands/fix-pr-bug.md
```

Expected: nothing.

- [ ] **Step 5: Verify AC-13 and that the frontmatter parses**

```bash
cd <worktree> && ls .claude/commands/fix-pr-bug.md .claude/commands/service-doc.md
cd <worktree> && head -5 .claude/commands/fix-pr-bug.md .claude/commands/service-doc.md
```

Expected: both files present; each starts with `---`, a `description:` line, an
`argument-hint:` line, and a closing `---`.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add .claude/agents/service-documentation.md \
                        .claude/commands/service-doc.md \
                        .claude/commands/fix-pr-bug.md \
  && git commit -m "feat(task-004): add service-documentation, /service-doc, and /fix-pr-bug"
```

---

## Owner documents — how Tasks 9 to 12 work

Tasks 9–12 port ten owner documents plus a verbatim copy of the spec. They share
one method, stated here once so each task does not repeat it. **Read this section
before starting any of Tasks 9–12.**

Each document gets three passes:

**Pass 1 — tool references.** Every `tools/<x>` mention gets one of three fates:

| Atlas reference | Fate |
|---|---|
| `tools/verify.sh`, `tools/verify_test.sh`, `tools/task-brief.sh`, `tools/task-numbers.sh`, `tools/toolchain.versions` | Kept — they exist here. Rewrite flag specifics to this repo's surface: `--quick`, `--no-docker`, `--list`. **Never** `--base`, `--all`, `--facts`, `--no-ui`. |
| `tools/doc-slice.sh` | Rule kept, mechanism replaced: `grep -n '^#'` for a document's shape, `sed -n 'A,Bp'` for the one section, `grep -n -A/-B` for a needle, `Read` with `offset`/`limit` for the harness-native form. |
| `tools/lint.sh` | Replaced by `tools/verify.sh`'s `golangci-lint` gate plus `tools/toolchain.versions`. |
| `tools/task-resolve.sh`, `tools/task-facts.sh`, `tools/agent-ledger.sh`, `tools/change-surfaces.sh`, `tools/plan-context.sh` | Not ported. Where a rule depends on one, restate the rule as a manual procedure. Where the reference is only an illustration, substitute a neutral example. |
| `tools/atlasguards`, `tools/rediskeyguard`, `tools/cideps`, `tools/build-services.sh`, `tools/db-bootstrap.sh`, `tools/gen-lb-ports.sh`, `tools/go-analyzer-guards.sh`, `tools/service-registration-guard.sh` | No Harbormaster analogue. The *rule* they illustrate — a repo-specific invariant belongs in a script, not in a reviewer's head — survives, illustrated by `tools/verify.sh`'s toolchain drift check, which this task creates. |
| `tools/foo.sh`, `tools/foo_test.sh` | Already neutral placeholders. Keep. |

**Pass 2 — domain examples.** Atlas illustrations — packet work, WZ data, IDA,
service opcodes, Kafka topics, `go.work`, `libs/`, `services/atlas-*`,
ephemeral PR deployments — become Harbormaster equivalents: MinIO admin API
contracts, bucket/object/policy/user handling, JSON:API documents, the
`apps/backend` ⁄ `apps/frontend` split, `deploy/docker` and `deploy/kubernetes`.

**A rule MUST NOT be deleted because its example does not transfer. Find a new
example.** (FR-34 / AC-17. This is reviewer-judged against the atlas originals in
Task 15, so a quietly dropped rule will be found.) The one legitimate reason to
drop a passage is that it is *pipeline mechanics* stating no rule at all — see
Task 12's `observability.md` for the worked case.

**Pass 3 — links and paths.** Every `docs/...` link must resolve to a file that
exists in this repository (FR-35 / AC-16). Atlas links to `docs/packets/*`,
`docs/adding-a-new-service.md`, `docs/reverse-engineering.md`,
`docs/runbooks/ephemeral-pr-deployments.md`, and six specific atlas task folders
must be rebound to a Harbormaster equivalent or removed together with the
sentence that carried them. `docs/adding-a-new-service.md` is explicitly **not**
ported (FR-33) — Harbormaster has no service-scaffolding story.

**No absolute or home paths anywhere under `docs/` (NFR-6 / AC-22).**
`block-home-paths-in-docs.sh` is live from Task 6, so a violation is rejected at
write time. Write `$ATLAS` or `<worktree>` as placeholders instead.

**Per-document verification**, run at the end of every one of Tasks 9–12:

```bash
cd <worktree> && grep -nE 'atlas|services/|libs/|go\.work|doc-slice|task-facts|agent-ledger|task-resolve|change-surfaces|plan-context|--base|--facts|--all|--no-ui|packets/|adding-a-new-service|reverse-engineering' docs/<name>.md
```

Every remaining hit must be one you can justify in your report — `docs/process-parity.md`
legitimately mentions atlas throughout, for instance. Then:

```bash
cd <worktree> && grep -oE '\]\((\.\./)*docs/[A-Za-z0-9._/-]+\)' docs/<name>.md \
  | sed -E 's/^\]\(//; s/\)$//; s|^(\.\./)+||' \
  | sort -u | while read -r p; do [ -e "$p" ] || echo "MISSING $p"; done
```

Expected: no `MISSING` lines, except for a document that lands in a later task —
note those in your report and let Task 15's sweep be the final word.

---

### Task 9: `process-parity.md`, `slice-first.md`, `tooling-conventions.md`, `git-workflow.md`

The four smallest documents, plus a verbatim copy of the spec.
**Read the "Owner documents" section above first.**

`docs/process-parity.md` is committed at that path (not only in the task folder)
because `process-parity.md` §7 check 3's grep exempts that exact path by name,
the ported owner docs link to it, and a future re-harmonisation reads it from
`docs/` rather than hunting through a task folder.

**Files:**
- Create: `docs/process-parity.md` (verbatim copy of
  `docs/tasks/task-004-process-parity-harness/process-parity.md`)
- Create: `docs/slice-first.md` (from `$ATLAS/docs/slice-first.md`, 5.2K)
- Create: `docs/tooling-conventions.md` (from `$ATLAS/docs/tooling-conventions.md`, 4.5K)
- Create: `docs/git-workflow.md` (from `$ATLAS/docs/git-workflow.md`, 1.8K)

**Interfaces:**
- Consumes: `tools/verify.sh`'s flag surface (Task 2), `tools/task-brief.sh` (Task 4).
- Produces, for `CLAUDE.md`'s trigger table (Task 13):
  - `docs/slice-first.md` owns "reading a large document, diff, plan, or tool result".
  - `docs/tooling-conventions.md` owns "long-running processes, mechanical repo
    facts, shell conventions".
  - `docs/git-workflow.md` owns "committing, pushing, rebasing, stray `main` commits".
  - `docs/process-parity.md` is the cross-repo parity spec, cited by the others.

- [ ] **Step 1: Copy the spec verbatim**

```bash
cd <worktree> && cp docs/tasks/task-004-process-parity-harness/process-parity.md docs/process-parity.md \
  && diff docs/process-parity.md docs/tasks/task-004-process-parity-harness/process-parity.md && echo IDENTICAL
```

Expected: `IDENTICAL`. Do not edit this file — it is the canonical spec and its
atlas references are correct as written.

- [ ] **Step 2: Port `docs/git-workflow.md`**

Read `$ATLAS/docs/git-workflow.md` in full (1.8K) and rewrite it here applying
the three passes. Every rule survives; only the illustrations move. Specifically:
branch naming becomes `task-NNN-slug`, the worktree root is `.worktrees/`, and
the "never commit to `main`" rule keeps its force. Add nothing about
`git stash` unless the original has it.

- [ ] **Step 3: Port `docs/tooling-conventions.md`**

Read `$ATLAS/docs/tooling-conventions.md` in full (4.5K). The long-running-process
rule (start it in the background, never poll; `wait-loop-guard.sh` refuses a poll)
transfers unchanged and is now enforced by a hook this task installed. The
"mechanical repo facts belong in a script, not in a reviewer's head" rule keeps
its force; its atlas guard examples become `tools/verify.sh`'s toolchain drift
check and `tools/task-numbers.sh`. Any `tools/lint.sh` reference becomes
`tools/verify.sh`'s `golangci-lint` gate plus `tools/toolchain.versions`.

- [ ] **Step 4: Port `docs/slice-first.md`**

Read `$ATLAS/docs/slice-first.md` in full (5.2K). This is the document that
depends most heavily on `tools/doc-slice.sh` (7 references across it and
`task-implementer.md`). The rule — read a slice, escalate to the whole only when
the slice proves insufficient — is repo-agnostic and is exactly the kind FR-34
forbids dropping.

Re-express its worked examples with tools that exist everywhere:

| Need | Command |
|---|---|
| a document's shape | `grep -n '^#' <path>` |
| the one section | `sed -n '120,180p' <path>` |
| a needle in an offloaded tool result | `grep -n -B4 -A12 '<needle>' <path>` |
| harness-native | `Read` with `offset` / `limit` |

Replace the measurement section's atlas token counts with this repo's own
concrete cases: reading all of `docs/tasks/task-004-process-parity-harness/prd.md`
versus its §4 alone, and the whole of that task's `plan.md` versus one
`tools/task-brief.sh` slice. Give real numbers — measure them:

```bash
cd <worktree> && wc -c docs/tasks/task-004-process-parity-harness/prd.md
cd <worktree> && wc -c docs/tasks/task-004-process-parity-harness/plan.md
cd <worktree> && ./tools/task-brief.sh docs/tasks/task-004-process-parity-harness/plan.md 9 /tmp/brief9.md && wc -c /tmp/brief9.md
```

Keep the escalation clause verbatim: this is a default with an escalation path,
not a ban, and a source file you are about to edit is not a large reference
document.

- [ ] **Step 5: Run the per-document verification**

Run both commands from the "Owner documents" section against
`docs/slice-first.md`, `docs/tooling-conventions.md`, and `docs/git-workflow.md`.
Skip `docs/process-parity.md` — its atlas references are correct by design.

- [ ] **Step 6: Confirm no absolute paths landed (AC-22)**

```bash
cd <worktree> && grep -rnE '/home/|/Users/|~/' docs/*.md
```

Expected: nothing.

- [ ] **Step 7: Commit**

```bash
cd <worktree> && git add docs/process-parity.md docs/slice-first.md \
                        docs/tooling-conventions.md docs/git-workflow.md \
  && git commit -m "docs(task-004): port process-parity, slice-first, tooling-conventions, git-workflow"
```

---

### Task 10: `docs/verification.md`

The largest owner document (19.4K) and the one with the most to rebind — atlas's
`verify.sh` has a different contract from this repo's. It also gains a section
atlas does not have: the CI asymmetry.

**Read the "Owner documents" section above first.**

**Files:**
- Create: `docs/verification.md` (from `$ATLAS/docs/verification.md`)

**Interfaces:**
- Consumes: `tools/verify.sh`'s exact flag surface and the three verdict lines
  (Task 2); the evidence Task 3 recorded.
- Produces, for `CLAUDE.md` (Task 13): `docs/verification.md` owns "gate failures,
  script/CI disagreement". `task-verifier.md` (Task 7) and `fix-pr-bug.md`
  (Task 8) both link here.

- [ ] **Step 1: Read the original and the local gate definitions**

```bash
cd <worktree> && grep -n '^#' "$ATLAS/docs/verification.md"
cd <worktree> && ./tools/verify.sh --help
cd <worktree> && grep -n 'run:\|uses:\|name:' .github/workflows/pr.yml
```

Slice the atlas document section by section rather than reading 19.4K in one
call — this document is the worked example of its own advice.

- [ ] **Step 2: Rewrite the flag surface throughout**

Atlas's `verify.sh` runs every check and summarises at the end, is change-gated
via `--base`/`--all`, and has `--facts` and `--no-ui`. This repo's stops at the
first failure, has no change detection, and offers `--quick`, `--no-docker`,
`--list`, `--help`.

Every passage about `--base`, `--all`, `--facts`, `--no-ui`, change surfaces,
module fan-out, or "the iteration gate" must be rewritten or removed. Where a
passage states a *rule* that survives the mechanism change — for instance "run
the gate in a clean context, not inside the implementer" — keep the rule and
change the mechanism.

Document, plainly, why the two scripts differ, so a reader of both repos does not
read this as an incomplete port:

> Atlas's 80-module fan-out makes a full picture worth the wall time, so its gate
> runs everything and summarises. Harbormaster has eleven gates over one Go
> module and one frontend, and the first failure is almost always the whole
> story. A verifier agent asked for "the first failing block" wants the run to
> have stopped there — otherwise it has to work out which of several failures came
> first.

- [ ] **Step 3: Write the three-verdict section**

State the exit-code contract exactly, because it is the thing most easily got
wrong:

> All three of `DONE`, `PARTIAL` and a failure-free flagged run exit 0. That is
> deliberate — `--quick` has to stay usable in an `&&` chain in the inner loop.
> The distinction lives in the terminal `VERIFY:` line:
>
> - `VERIFY: DONE — all gates passed; the branch may be called done.`
> - `VERIFY: PARTIAL — all selected gates passed, but <N> were skipped (<labels>). This does NOT count as done.`
> - `VERIFY: FAILED — <label>` on stderr, non-zero exit.
>
> Quote the verdict line. "verify.sh exited 0" is not a report.

- [ ] **Step 4: Write the CI asymmetry section**

This is the section atlas does not have, and it is why this document owns
"script/CI disagreement". State the asymmetry and the resolution rule:

> `tools/verify.sh` mirrors the ten-command checklist that used to live in
> `CLAUDE.md`, plus a toolchain drift check. CI runs four things it does not:
>
> | CI-only gate | Why it is not in the script |
> |---|---|
> | `gitleaks` | Needs its own toolchain and network; a local miss is caught in CI before merge, and no credential handling may be introduced here. |
> | Trivy filesystem scan | Same. |
> | `go-licenses` allowlist | Installs a tool and needs `yq`. |
> | `go vet -tags=integration ./...` | Cheap and genuinely tempting — but the flagless gate list is fixed by the task that created this script, and an eleventh gate was scope that task did not authorise. Recorded here so it is a known gap rather than a rediscovery. |
>
> **When the script and CI disagree, CI is the authority and the script is the
> bug.** Fix `tools/verify.sh` to match; never loosen CI to match the script.

Confirm those four against the workflow before writing them down — read
`.github/workflows/pr.yml` rather than trusting this plan.

- [ ] **Step 5: Document the two on-demand suites**

They are named in `--help` and run by nobody automatically:

```sh
cd apps/backend  && HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...
cd apps/frontend && npm run test:e2e
```

State that these deliberately do not affect any mode's exit code, and that
`tools/verify_test.sh` asserts textually that neither string appears in an
executable line of `verify.sh` — a passing run cannot prove a command did *not*
run.

- [ ] **Step 6: Document the `--quick` trade explicitly**

`--quick` skips the container build, drops `-race`, and skips `npm ci` when
`apps/frontend/node_modules/.package-lock.json` is not older than
`apps/frontend/package-lock.json`. Say what that costs: a `--quick` pass has not
race-checked the backend, has not built the container, and — when the skip
fired — has not verified that the installed dependency tree matches the lockfile.
That is exactly why it reports `PARTIAL`.

- [ ] **Step 7: Run the per-document verification**

Run both commands from the "Owner documents" section against
`docs/verification.md`. Then:

```bash
cd <worktree> && grep -nE '\-\-base|--facts|--all|--no-ui|bake|go\.work' docs/verification.md
```

Expected: nothing.

- [ ] **Step 8: Commit**

```bash
cd <worktree> && git add docs/verification.md \
  && git commit -m "docs(task-004): port verification.md and record the CI asymmetry"
```

---

### Task 11: `docs/agent-dispatch.md`, `docs/review-protocol.md`, `docs/superpowers-integration.md`, `docs/post-implementation.md`

The four process documents. **Read the "Owner documents" section above first.**

**Files:**
- Create: `docs/agent-dispatch.md` (from `$ATLAS/docs/agent-dispatch.md`, 12.6K)
- Create: `docs/review-protocol.md` (from `$ATLAS/docs/review-protocol.md`, 7.9K)
- Create: `docs/superpowers-integration.md` (from `$ATLAS/docs/superpowers-integration.md`, 10.7K)
- Create: `docs/post-implementation.md` (from `$ATLAS/docs/post-implementation.md`, 6.1K)

**Interfaces:**
- Consumes: the agent trio and their model pins (Task 7), `/fix-pr-bug` (Task 8),
  `tools/task-numbers.sh` and `tools/task-brief.sh` (Task 4).
- Produces, for `CLAUDE.md`'s trigger table (Task 13):
  - `docs/agent-dispatch.md` owns "model pinning, fan-out vs. fork, handoff decision".
  - `docs/review-protocol.md` owns "dispatching a reviewer, writing up a review".
  - `docs/superpowers-integration.md` owns "bare task numbers, skills outside a phase command".
  - `docs/post-implementation.md` owns "Phase 5, `/fix-pr-bug`".
- Produces: the review-artifact split — per-unit reviews at
  `docs/tasks/<task>/reviews/<unit>.md`, guideline/adherence audits at
  `docs/tasks/<task>/audit.md` — which `task-reviewer.md` (Task 7) points at.

- [ ] **Step 1: Port `docs/agent-dispatch.md`**

The seven-agent roster here is: `task-implementer`, `task-verifier`,
`task-reviewer`, `service-documentation`, `plan-adherence-reviewer`,
`backend-guidelines-reviewer`, `frontend-guidelines-reviewer`, plus
`todo-scanner`. Atlas's packet and dispatcher-family agents have no analogue and
their rows come out; the *rules* around them do not.

Keep and rebind: the model-pinning rule and the actual pins (`task-implementer`
sonnet, `task-verifier` haiku, `task-reviewer` sonnet, `service-documentation`
sonnet — read each agent's frontmatter and cite what is there, do not recall it);
the inline-vs-delegate rule with its measured justification (`task-reviewer.md`
links to this section by name, so the heading must survive); the fan-out-vs-fork
guidance, which `fork-dispatch-guard.sh` now enforces; the handoff decision
("does the next unit depend on this conversation, or only on repository state?").

Replace atlas's `libs/` fan-out illustration with the Harbormaster case that
actually recurs: a change touching both `apps/backend/internal/<domain>` and the
matching `apps/frontend/src/features/<name>` is two units with one seam, not one
unit — and the seam is what `task-reviewer` is for.

- [ ] **Step 2: Port `docs/review-protocol.md`**

Keep the verdict-first return block verbatim — `task-reviewer.md` reproduces it
and the two must not drift:

```text
verdict: APPROVED | APPROVED_WITH_FINDINGS | CHANGES_REQUIRED
artifact: <repo-relative path>
scope_confirmed: <what you actually reviewed>
blocking: <n>
  - <file:line> — <one sentence>
non_blocking: <n>
not_evaluable: <n>
```

Keep the two most-broken rules stated as such: the verdict is the **first line**,
and blocking findings are **enumerated with `file:line`**, not counted.

Add the artifact-path split, which is new to this repo and is this document's to
own:

> | Reviewer | Artifact |
> |---|---|
> | `task-reviewer` (per unit, per fix round) | `docs/tasks/<task>/reviews/<unit>.md` |
> | `plan-adherence-reviewer`, `backend-guidelines-reviewer`, `frontend-guidelines-reviewer` | `docs/tasks/<task>/audit.md` |
>
> A per-unit review must never write `audit.md`. The three pre-PR reviewers share
> that file and a per-unit review landing there would overwrite work it did not do.

Also record which reviewer owns what: `task-reviewer` reviews one unit against
its brief; the guideline reviewers own the `DOM-*` / `SUB-*` / `SEC-*` and `FE-*`
checklists and run before a PR; `plan-adherence-reviewer` owns "was every plan
task actually implemented". Keep the "never approve on the strength of a green
build" rule.

- [ ] **Step 3: Port `docs/superpowers-integration.md`**

The two triggers this document owns are "bare task numbers" and "skills invoked
outside a phase command". Rebind:

- The artifact-location override is Harbormaster's:
  `superpowers:brainstorming` and `superpowers:writing-plans` default to
  `docs/superpowers/specs/` and `docs/superpowers/plans/`; **here both go under
  `docs/tasks/task-NNN-slug/`**, and the skill must be told the task folder
  explicitly when invoked outside a phase command.
- Bare task numbers resolve through `tools/task-numbers.sh list` and the phase
  commands' fuzzy matcher (`task-001-slug`, `task-001`, `001`, `1` all resolve),
  searching both `docs/tasks/` and `.worktrees/*/docs/tasks/`.
- Task numbers are allocated by `tools/task-numbers.sh next` — never by hand.
  The `SessionStart` collision detector runs `check` on every session start.
- The four phase commands and their artifacts are Harbormaster's:
  `/spec-task` → `prd.md`, `/design-task` → `design.md`, `/plan-task` →
  `plan.md` + `context.md`, `/execute-task` → implementation. Phase 5 is
  `/fix-pr-bug`.
- Note that plan task headings must match `^#+[ \t]+Task[ \t]+[0-9]+` because
  `tools/task-brief.sh` slices on that pattern.

- [ ] **Step 4: Port `docs/post-implementation.md`**

This is `/fix-pr-bug`'s owner document — the command links to it by name and says
"read it if anything below is ambiguous", so the two must agree.

Keep verbatim: the loop, the rule that the bug file is the boundary between the
diagnosis context and the fix context, the bug-file template (reproduced /
observed / expected / root cause, then `## Fix` file inventory, then
`## Not yet answered`), "if the root cause is not established, say so and name
what is ruled out; do not guess one", and the rule that a bug file which never
records its resolution is the next session's rediscovery.

Rebind: reproduction is against a MinIO deployment, not a game client; logs come
from `deploy/docker/docker-compose.yml` or `kubectl logs deploy/harbormaster`
(link `docs/observability.md`); the gate command is `tools/verify.sh --quick`;
there is no `tools/agent-ledger.sh`, so the outcome is recorded in the bug file
itself.

- [ ] **Step 5: Cross-check the three documents that quote each other**

`task-reviewer.md` (Task 7) links to `docs/review-protocol.md` and to
`docs/agent-dispatch.md` §"Inline vs delegate"; `fix-pr-bug.md` (Task 8) links to
`docs/post-implementation.md`. Confirm the headings those links target actually
exist:

```bash
cd <worktree> && grep -n '^#' docs/review-protocol.md docs/agent-dispatch.md docs/post-implementation.md
cd <worktree> && grep -n 'review-protocol\|agent-dispatch\|post-implementation' \
  .claude/agents/task-reviewer.md .claude/commands/fix-pr-bug.md
```

Fix whichever side is wrong. A link that resolves to a file but not to the
section it names is still a broken reference.

- [ ] **Step 6: Run the per-document verification**

Run both commands from the "Owner documents" section against each of the four
files.

- [ ] **Step 7: Commit**

```bash
cd <worktree> && git add docs/agent-dispatch.md docs/review-protocol.md \
                        docs/superpowers-integration.md docs/post-implementation.md \
  && git commit -m "docs(task-004): port agent-dispatch, review-protocol, superpowers-integration, post-implementation"
```

---

### Task 12: `docs/codemod-vs-agents.md` and `docs/observability.md`

One straight port and one rewrite. **Read the "Owner documents" section above
first.**

`observability.md` is the conditional port under `process-parity.md` §5.2,
included by decision because Harbormaster has `deploy/docker`,
`deploy/kubernetes`, and an existing `docs/operator/` set — a deploy story worth
a runbook. But atlas's version documents an OpenTelemetry → spanmetrics →
Prometheus/Loki/Grafana pipeline this repo does not have, so it is a rewrite,
not a port.

**Files:**
- Create: `docs/codemod-vs-agents.md` (from `$ATLAS/docs/codemod-vs-agents.md`, 7.0K)
- Create: `docs/observability.md` (rewritten, informed by `$ATLAS/docs/observability.md`, 7.8K)

**Interfaces:**
- Consumes: nothing from earlier tasks except the doc conventions.
- Produces, for `CLAUDE.md`'s trigger table (Task 13):
  - `docs/codemod-vs-agents.md` owns "a second implementer at the same transformation".
  - `docs/observability.md` owns "the deploy/runbook story".
  `fix-pr-bug.md` (Task 8) and `docs/post-implementation.md` (Task 11) both link
  to `docs/observability.md`.

- [ ] **Step 1: Port `docs/codemod-vs-agents.md`**

The rule — when you are about to dispatch a second agent at the same mechanical
transformation, write a codemod instead — is repo-agnostic and needs only new
examples. Atlas's are packet handlers and service registrations. Harbormaster's
mechanical-sweep candidates: a JSON:API attribute rename across
`apps/backend/internal/*/rest.go` and the matching frontend types; a change to
every `internal/*` package's error-wrapping call; a `prettier`/`eslint` config
change rippling through `apps/frontend/src`. Use those.

Keep the cost comparison structure and the escape hatch (a transformation with
per-site judgement is not a codemod). If the original cites measured numbers from
atlas, keep them and attribute them to atlas rather than inventing Harbormaster
numbers you have not measured.

- [ ] **Step 2: Establish what Harbormaster actually has, before writing a word**

```bash
cd <worktree> && cat apps/backend/internal/observability/log/log.go
cd <worktree> && sed -n '1,60p' apps/backend/internal/metrics/collector.go
cd <worktree> && sed -n '1,40p' apps/backend/internal/metrics/poller.go
cd <worktree> && grep -n '^#' docs/operator/configuration.md docs/operator/recovery.md
cd <worktree> && sed -n '1,40p' deploy/docker/docker-compose.yml
cd <worktree> && grep -n 'image\|env\|containers' deploy/kubernetes/deployment.yaml
```

Write from what you read. Verification over memory applies to this document more
than any other in the set.

- [ ] **Step 3: Write `docs/observability.md`**

Keep these rules from the atlas original, bound to what Step 2 found:

1. **Log-field naming discipline.** State the conventions
   `internal/observability/log` actually establishes, from its source.
2. **Diagnose a runtime failure by reading the logs before changing code.** Give
   the two real commands —
   `docker compose -f deploy/docker/docker-compose.yml logs harbormaster` and
   `kubectl logs deploy/harbormaster` — and keep the "read the logs for the
   workload you name, never a whole-namespace listing" rule.
3. **A deploy smoke test that proves the thing actually came up.** Name the
   concrete check for each of the two deploy paths.
4. **Metrics.** `internal/metrics` polls MinIO's own Prometheus endpoint and
   aggregates; document what it collects and what it does not, from the source.

Drop, as pipeline mechanics rather than rules: manual spans, spanmetrics
dimensions, the cardinality budget, the sampling caveat, and Grafana panel
definitions. **Nothing in those sections states a rule that survives the loss of
the pipeline** — that is the test for a legitimate drop, and it is the only
place in Tasks 9–12 where it is met. Say so in your report, section by section,
so the AC-17 reviewer can check the judgement rather than take it on trust.

**Link to `docs/operator/*` rather than restating it.** The operator docs are
already the authority on configuration and recovery; a second copy would drift.

- [ ] **Step 4: Run the per-document verification**

Run both commands from the "Owner documents" section against both files. Then
confirm the operator links resolve:

```bash
cd <worktree> && grep -oE 'docs/operator/[a-z-]+\.md' docs/observability.md \
  | sort -u | while read -r p; do [ -e "$p" ] || echo "MISSING $p"; done
```

Expected: no output.

- [ ] **Step 5: Confirm all ten owner documents now exist (AC-15)**

```bash
cd <worktree> && for f in agent-dispatch verification superpowers-integration \
  review-protocol post-implementation codemod-vs-agents slice-first \
  tooling-conventions git-workflow observability; do
  [ -f "docs/$f.md" ] && echo "ok   docs/$f.md" || echo "MISSING docs/$f.md"
done
cd <worktree> && [ -e docs/adding-a-new-service.md ] \
  && echo "FR-33 VIOLATION: adding-a-new-service.md must not exist" \
  || echo "ok   adding-a-new-service.md correctly absent"
```

Expected: ten `ok` lines and the FR-33 confirmation.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add docs/codemod-vs-agents.md docs/observability.md \
  && git commit -m "docs(task-004): port codemod-vs-agents and rewrite observability for this repo"
```

---

### Task 13: Rewrite `CLAUDE.md`

From prose narrative to a rule list plus a trigger → owner-document table. It
lands after the documents so every target in its table already exists.

The current file's claim that the repository "is currently unscaffolded — only
`README.md` exists" is stale and must not be carried forward (FR-39).

**Files:**
- Modify: `CLAUDE.md` (full rewrite)

**Interfaces:**
- Consumes: all ten owner documents (Tasks 9–12), `tools/verify.sh` (Task 2),
  `tools/task-numbers.sh` and `tools/task-brief.sh` (Task 4), the agent trio
  (Task 7).
- Produces: the eight-heading structure that `process-parity.md` §7 check 5
  asserts, and the trigger table that lets the rule list stay terse.

- [ ] **Step 1: Write the file with exactly these eight headings, in order**

After the `# Harbormaster` title:

`## Never do this`, `## Evidence & grounding`, `## Development workflow`,
`## Done means verified`, `## Dispatching agents`, `## Handing off context`,
`## Repository conventions`, `## Where the procedures live`.

No other `##` headings. `###` subheadings are fine where a section needs them.

- [ ] **Step 2: Write `# Harbormaster` — the corrected overview (FR-39)**

Describe the layout that exists. Delete the "unscaffolded" claim and the "update
this file once layout is settled" instruction it justified.

> Harbormaster is a self-hosted MinIO admin UI for homelab and small-cluster
> operators.
>
> - `apps/backend` — a single Go module (`cmd/`, `internal/`, `migrations/`,
>   `Makefile`, `.golangci.yml`). There is no `go.work`; every Go change is
>   local to this module.
> - `apps/frontend` — Vite / React / TypeScript.
> - `deploy/docker`, `deploy/kubernetes` — the two deploy paths.
> - `docs/architecture/`, `docs/operator/`, `docs/tasks/` — design, runbooks,
>   and per-task artifacts.
> - `tools/` — `verify.sh` (the gate), `task-numbers.sh`, `task-brief.sh`,
>   `toolchain.versions`.

Confirm each path exists before writing it down.

- [ ] **Step 3: Write `## Never do this`**

- Don't implement when you were asked to understand or plan. Planning and
  implementation are separate phases; wait for explicit approval.
- Don't edit files in the main repo when a task worktree exists for that work.
- Don't open a PR without running the code review step (`/audit-plan` or
  `superpowers:requesting-code-review`) — not even when the plan looks complete.
- Don't call a `--quick` or `--no-docker` run "done". Those print
  `VERIFY: PARTIAL`, and they mean it.
- Don't walk a design or plan document through section-by-section approval. Write
  the full document to the file; the reader will read the committed file.
- Don't pick a task number by hand. `tools/task-numbers.sh next`.
- Don't `git add -A` or `git add .`.

- [ ] **Step 4: Write `## Evidence & grounding`**

- Verify MinIO admin API contracts, configuration values, and service-to-service
  interactions against local source or upstream MinIO docs. Never cite them from
  memory.
- When uncertain about behaviour, read the source rather than speculating.
- Report what you could not verify as unverified. Unverified is "unknown", not a
  plausible guess.

- [ ] **Step 5: Write `## Development workflow`**

The four phases, terse:

1. `/spec-task <idea>` — from the main repo. Creates the worktree at
   `.worktrees/task-NNN-slug/` on branch `task-NNN-slug`, commits `prd.md`.
2. `/design-task <id>` — `design.md`.
3. `/plan-task <id>` — `plan.md` + `context.md`.
4. `/execute-task <id>` — implementation, in the existing worktree. Never
   creates a new one.

Phase 5 for a post-implementation bug is `/fix-pr-bug <task> <slug>`.

Then, still in this section:

- Each phase runs in a fresh (`/clear`'d) session so it consumes only the prior
  phase's documented artifacts.
- **Artifact-location override:** `superpowers:brainstorming` and
  `superpowers:writing-plans` default to `docs/superpowers/specs/` and
  `docs/superpowers/plans/`. **In this project both go under
  `docs/tasks/task-NNN-slug/`.** Pass the task folder explicitly when invoking
  those skills outside a phase command.
- Fuzzy task identifiers: `task-001-slug`, `task-001`, `001` and `1` all resolve.
  Search both `docs/tasks/` and `.worktrees/*/docs/tasks/`.
- Worktree discipline: verify cwd is the right worktree before working; `cd`
  there yourself rather than asking. Search all worktrees (`git worktree list`)
  before concluding an artifact is missing.
- Task numbers come from `tools/task-numbers.sh next`. A `SessionStart` hook runs
  `tools/task-numbers.sh check` and reports collisions.
- Skip `/spec-task` only for trivial fixes that don't warrant a PRD.

- [ ] **Step 6: Write `## Done means verified` — referenced, not restated (FR-38)**

The ten commands must **not** be reproduced here. They live in the script.

> ```sh
> tools/verify.sh              # every gate. Exit 0 → the branch may be called done.
> tools/verify.sh --quick      # skips buildx, -race, and npm ci when current. NOT done.
> tools/verify.sh --no-docker  # skips buildx only. NOT done.
> tools/verify.sh --list       # the gates these flags would select. Runs none.
> tools/verify.sh --help       # usage, and the two on-demand suites.
> ```
>
> Read the terminal `VERIFY:` line, not just the exit code — `DONE` and
> `PARTIAL` both exit 0.
>
> Two suites are on-demand and are not part of any mode:
> `HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...` in
> `apps/backend` (needs Docker), and `npm run test:e2e` in `apps/frontend`
> (needs the Compose stack).
>
> Gate failure, or the script disagreeing with CI: `docs/verification.md`.

- [ ] **Step 7: Write `## Dispatching agents`**

- The trio: `task-implementer` (one plan task, 120 tool-call budget, `PARTIAL`
  hand-back, module-local build/test only), `task-verifier` (runs
  `tools/verify.sh` in its own clean context, never edits), `task-reviewer` (one
  unit against its brief, artifact under `docs/tasks/<task>/reviews/`).
- Never run `tools/verify.sh` inside an implementer.
- Before a PR: `plan-adherence-reviewer`, `backend-guidelines-reviewer` (Go
  `DOM-*` / `SUB-*` / `SEC-*`), `frontend-guidelines-reviewer` (`FE-*`) — all
  writing to `docs/tasks/<task>/audit.md`, dispatched by
  `superpowers:requesting-code-review`.
- `service-documentation` / `/service-doc <component>` documents one component
  into `docs/architecture/<name>.md`.
- `todo-scanner` / `/review-todos` refreshes `docs/TODO.md`.
- Model pinning, fan-out vs. fork, and the handoff decision:
  `docs/agent-dispatch.md`.

- [ ] **Step 8: Write `## Handing off context`**

- Brief-first: an implementer gets `tools/task-brief.sh plan.md N`, not the whole
  plan. Plan task headings must match `^#+[ \t]+Task[ \t]+[0-9]+`.
- Slice a large document, diff, plan, or tool result before reading it whole:
  `docs/slice-first.md`.
- The handoff question: does the next unit depend on this conversation, or only
  on repository state? If only on repository state, write the state down and
  hand off.

- [ ] **Step 9: Write `## Repository conventions`**

- Prefer straightforward moves over re-exported type aliases when refactoring
  shared types or creating common libraries.
- Keep abstractions clean — don't break service boundaries by having one layer
  call another's internals directly.
- The `backend-dev-guidelines` and `frontend-dev-guidelines` skills in
  `.claude/skills/` are the authority on Go and React/TS patterns; the
  `skill-activation-prompt` hook auto-suggests them from the triggers in
  `.claude/skills/skill-rules.json`.
- Use repo-relative paths in committed files — never literal home or absolute
  paths. `block-home-paths-in-docs.sh` enforces this under `docs/`.
- Long-running processes go in the background and are never polled;
  `wait-loop-guard.sh` refuses a poll. See `docs/tooling-conventions.md`.

- [ ] **Step 10: Write `## Where the procedures live` (FR-37)**

A trigger → owner-document table, exactly these ten rows:

| Trigger | Owner |
|---|---|
| Model pinning, fan-out vs. fork, the handoff decision | [`docs/agent-dispatch.md`](docs/agent-dispatch.md) |
| A gate failed, or the script disagrees with CI | [`docs/verification.md`](docs/verification.md) |
| A bare task number; a superpowers skill outside a phase command | [`docs/superpowers-integration.md`](docs/superpowers-integration.md) |
| Dispatching a reviewer; writing up a review | [`docs/review-protocol.md`](docs/review-protocol.md) |
| A bug found after implementation; Phase 5 / `/fix-pr-bug` | [`docs/post-implementation.md`](docs/post-implementation.md) |
| About to point a second implementer at the same transformation | [`docs/codemod-vs-agents.md`](docs/codemod-vs-agents.md) |
| Reading a large document, diff, plan, or tool result | [`docs/slice-first.md`](docs/slice-first.md) |
| Long-running processes, mechanical repo facts, shell conventions | [`docs/tooling-conventions.md`](docs/tooling-conventions.md) |
| Committing, pushing, rebasing, a stray commit on `main` | [`docs/git-workflow.md`](docs/git-workflow.md) |
| Deploying, reading logs, the runbook story | [`docs/observability.md`](docs/observability.md) |

This must be the last section in the file.

- [ ] **Step 11: Verify the heading structure (AC-18)**

```bash
cd <worktree> && grep -n '^#\{1,2\} ' CLAUDE.md
```

Expected, in exactly this order:

```
# Harbormaster
## Never do this
## Evidence & grounding
## Development workflow
## Done means verified
## Dispatching agents
## Handing off context
## Repository conventions
## Where the procedures live
```

- [ ] **Step 12: Verify FR-39 and FR-40 (AC-19)**

```bash
cd <worktree> && grep -niE 'unscaffolded|only .README\.md. exists|will be decided during the first task' CLAUDE.md
```

Expected: nothing.

Then walk PRD FR-40's nine items against the file and record, item by item in
your report, the heading each now lives under:

1. four-phase workflow + worktree discipline;
2. artifact-location override;
3. fuzzy task-identifier resolution across both roots;
4. "asked to understand or plan → do not implement";
5. design/plan output style (full document, no per-section approval);
6. code review before PR is mandatory;
7. verification over memory for MinIO admin API contracts and config values;
8. the code-review pattern and its three modular reviewer agents;
9. straightforward moves over re-exported type aliases; don't break service
   boundaries.

A missing item is a failed task, not a note.

- [ ] **Step 13: Verify every link resolves (AC-16)**

```bash
cd <worktree> && grep -oE '\]\(docs/[A-Za-z0-9._/-]+\)' CLAUDE.md \
  | sed -E 's/^\]\(//; s/\)$//' \
  | sort -u | while read -r p; do [ -e "$p" ] && echo "ok   $p" || echo "MISSING $p"; done
```

Expected: ten `ok` lines, no `MISSING`.

- [ ] **Step 14: Confirm the ten build commands are gone (FR-38)**

```bash
cd <worktree> && grep -nE 'go test -race|golangci-lint run|npm run build|docker buildx build' CLAUDE.md
```

Expected: nothing. `tools/verify.sh` owns them now. The two on-demand suites are
the deliberate exception and appear as prose, not as a checklist.

- [ ] **Step 15: Commit**

```bash
cd <worktree> && git add CLAUDE.md \
  && git commit -m "docs(task-004): restructure CLAUDE.md into rule list plus owner-doc table"
```

---

### Task 14: Amend the phase commands

FR-28 edits `/spec-task` and `/execute-task` — the very commands used to run this
task. They land last, after everything they reference exists, so the tree is never
inconsistent with the session's behaviour mid-flight.

The scope is deliberately minimal. This is **not** a wholesale port of atlas's
400-line `execute-task.md`: that file encodes `go.work` fan-out, `libs/` change
surfaces and module-count heuristics that are false here, and importing them
would be importing bugs.

**Files:**
- Modify: `.claude/commands/spec-task.md`
- Modify: `.claude/commands/execute-task.md`
- Modify: `.claude/commands/plan-task.md`
- Modify: `.claude/commands/design-task.md`

**Interfaces:**
- Consumes: `tools/task-numbers.sh next` (Task 4), `tools/task-brief.sh` (Task 4),
  `task-implementer` / `task-verifier` / `task-reviewer` (Task 7).
- Produces: `/execute-task` dispatching the trio by name instead of falling back
  to uncapped generic dispatch.

- [ ] **Step 1: `spec-task.md` — replace the hand-rolled number scan**

Replace the whole of Step 2's numbered items 1–3 (the `find .worktrees` scan, the
`git branch --list 'task-*'` check, and "pick the next free NNN") with:

```markdown
1. Allocate the task number:

   ```sh
   tools/task-numbers.sh next
   ```

   That script is the only collision-safe source. It scans `docs/tasks/`, every
   `.worktrees/*/docs/tasks/`, local and remote `task-*` branches, and git
   history — the last of which is what stops it re-issuing the number of a task
   whose branch was merged and deleted. **Do not pick a number by hand.**
   Picking by hand has produced two tasks sharing one number.
```

Keep items 4 and 5 (deriving the slug, composing `task-NNN-<slug>`) as they are,
renumbering to 2 and 3.

- [ ] **Step 2: `spec-task.md` — remove the literal home path**

`spec-task.md` currently contains a literal absolute home-directory path in
Step 1's refusal message and another in Step 5. Replace both with a placeholder:

```bash
cd <worktree> && grep -n '/Users/\|/home/' .claude/commands/spec-task.md
```

Rewrite each occurrence so the sentence reads with `<main repo root>` or
`<worktree>` instead. Step 1's message becomes:

> You're already inside a task worktree. `/spec-task` must be run from the main
> repo root. `cd` there and re-run.

and Step 5's absolute example becomes
`<worktree>/docs/tasks/task-NNN-<slug>/prd.md`.

These files are outside `docs/`, so `block-home-paths-in-docs.sh` does not fire
on them — this is an opportunistic fix, and the reason it is worth doing is that
the path is wrong as well as absolute.

- [ ] **Step 3: `execute-task.md` — dispatch the trio**

Rewrite Step 4 so the dispatch contract is explicit. Keep the existing Steps 1–3
and the worktree rules unchanged.

```markdown
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

If the user explicitly requests inline mode this session (rare), invoke
`superpowers:executing-plans` instead.
```

Also update Step 5's closing suggestion to name the trio's completion before
`superpowers:requesting-code-review`, and add to `## Important Rules`:
"Never dispatch `general-purpose` for an implementation task — that is what
`task-implementer` is for."

- [ ] **Step 4: `plan-task.md` and `design-task.md` — one line each**

In `plan-task.md`, add to the Step 5 instruction block:

> Plan task headings MUST match `^#+[ \t]+Task[ \t]+[0-9]+` (i.e. `### Task 1: …`).
> `tools/task-brief.sh` slices on that pattern, and `/execute-task` uses it to
> hand each implementer only its own task.

In `design-task.md`, add a single line to the same effect where it describes the
handoff to `/plan-task`:

> The plan `/plan-task` produces is sliced per task by `tools/task-brief.sh`, so
> its task headings must be `### Task N: …`.

- [ ] **Step 5: Verify the amendments**

```bash
cd <worktree> && grep -n 'task-numbers.sh' .claude/commands/spec-task.md
cd <worktree> && grep -n '/Users/\|/home/' .claude/commands/*.md; echo "exit: $?"
cd <worktree> && grep -n 'task-implementer\|task-verifier\|task-reviewer\|task-brief.sh' .claude/commands/execute-task.md
cd <worktree> && grep -n 'general-purpose' .claude/commands/execute-task.md
cd <worktree> && grep -n 'Task\[ \\t\]' .claude/commands/plan-task.md .claude/commands/design-task.md
```

Expected: `spec-task.md` calls `tools/task-numbers.sh next`; no absolute home
paths anywhere in `.claude/commands/`; `execute-task.md` names all three agents
and `task-brief.sh`; its only `general-purpose` mention is the prohibition; both
of the last two files carry the heading-shape note.

- [ ] **Step 6: Commit**

```bash
cd <worktree> && git add .claude/commands/spec-task.md .claude/commands/execute-task.md \
                        .claude/commands/plan-task.md .claude/commands/design-task.md \
  && git commit -m "feat(task-004): point the phase commands at the new tooling and agent trio"
```

---

### Task 15: Final assertion sweep and the parity report

Run every mechanically checkable acceptance criterion as one scripted sweep and
paste the output verbatim. Then write the report for the criteria that cannot be
evaluated from this repository — and **report** them, never assert them.

**Files:**
- Create: `docs/tasks/task-004-process-parity-harness/parity-report.md`

**Interfaces:**
- Consumes: everything from Tasks 1–14, plus the hook hashes Task 5 recorded and
  the `verify.sh` run evidence Task 3 recorded.
- Produces: the AC-23 / AC-24 report, which is this task's deliverable to whoever
  runs `process-parity.md` §7 across all four repositories.

- [ ] **Step 1: Run the mechanical sweep**

Run each of these and capture the output. Do not summarise — paste.

```bash
cd <worktree>

echo "== AC-1: no atlas- in hooks =="
grep -l 'atlas-' .claude/hooks/*.sh; echo "exit: $?"

echo "== AC-2: wait-loop-guard test =="
./.claude/hooks/wait-loop-guard_test.sh; echo "exit: $?"

echo "== AC-3: format-on-write rebound =="
grep -n 'apps/frontend\|toolchain.versions\|apps/backend/.golangci.yml' .claude/hooks/format-on-write.sh

echo "== AC-4: pin matches CI =="
. tools/toolchain.versions; echo "pinned=$GOLANGCI_LINT_VERSION"
grep -n -A2 'golangci-lint-action' .github/workflows/pr.yml

echo "== AC-5: the three scripts exist and are executable =="
for f in tools/verify.sh tools/task-numbers.sh tools/task-brief.sh; do
  [ -x "$f" ] && echo "ok   $f" || echo "MISSING/NOT-EXEC $f"
done

echo "== AC-6: task-numbers test =="
./tools/task-numbers_test.sh; echo "exit: $?"

echo "== AC-12: agents present, the four pre-existing untouched =="
ls .claude/agents/
git diff --stat main...HEAD -- \
  .claude/agents/backend-guidelines-reviewer.md \
  .claude/agents/frontend-guidelines-reviewer.md \
  .claude/agents/plan-adherence-reviewer.md \
  .claude/agents/todo-scanner.md
echo "(empty diff above == byte-unchanged)"

echo "== AC-13: commands present =="
ls .claude/commands/fix-pr-bug.md .claude/commands/service-doc.md

echo "== AC-14: settings =="
jq -e '.disableBundledSkills == true' .claude/settings.json
jq -r '.hooks.PreToolUse[].matcher' .claude/settings.json
jq -r '.hooks.PostToolUse[].matcher' .claude/settings.json
jq -r '.hooks | keys[]' .claude/settings.json
jq -e '.enabledPlugins["superpowers@claude-plugins-official"] == true' .claude/settings.json

echo "== AC-15: ten owner documents =="
for f in agent-dispatch verification superpowers-integration review-protocol \
         post-implementation codemod-vs-agents slice-first tooling-conventions \
         git-workflow observability; do
  [ -f "docs/$f.md" ] && echo "ok   docs/$f.md" || echo "MISSING docs/$f.md"
done

echo "== AC-16: every docs/ link resolves =="
grep -ohE '\]\((\.\./)*docs/[A-Za-z0-9._/-]+\)' CLAUDE.md docs/*.md \
  | sed -E 's/^\]\(//; s/\)$//; s|^(\.\./)+||' \
  | sort -u | while read -r p; do [ -e "$p" ] || echo "MISSING $p"; done
echo "(no MISSING lines above == pass)"

echo "== AC-18/AC-19: CLAUDE.md shape =="
grep -n '^#\{1,2\} ' CLAUDE.md
grep -niE 'unscaffolded|only .README\.md. exists' CLAUDE.md; echo "exit: $?"

echo "== AC-20: process-parity check 3 =="
git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'; echo "exit: $?"

echo "== AC-21: no app changes =="
git diff --name-only main...HEAD | grep -c '^apps/'

echo "== AC-22: no home/absolute paths under docs/ =="
grep -rnE '/home/|/Users/|(^|[^A-Za-z0-9_.])~/' docs/ --include='*.md'; echo "exit: $?"
```

Expected: AC-1 exit 1 with no filenames; AC-2 and AC-6 exit 0; AC-5 and AC-15 all
`ok`; AC-12's `git diff --stat` empty; AC-14 as in Task 6; AC-16 no `MISSING`;
AC-18 the eight headings in order; AC-19 exit 1; AC-20 exit 1 with nothing
printed; AC-21 prints `0`; AC-22 exit 1.

**A non-empty AC-20 or a non-zero AC-21 is a blocking failure.** Fix the cause
before continuing; do not narrow the grep.

- [ ] **Step 2: Re-run the three `verify.sh` modes and the test suite**

```bash
cd <worktree> && ./tools/verify_test.sh
cd <worktree> && ./tools/verify.sh --quick 2>&1 | tail -3
cd <worktree> && ./tools/verify.sh --no-docker 2>&1 | tail -3
cd <worktree> && git status --porcelain > /tmp/hm-final-before.txt
cd <worktree> && ./tools/verify.sh 2>&1 | tail -3
cd <worktree> && git status --porcelain > /tmp/hm-final-after.txt \
  && diff /tmp/hm-final-before.txt /tmp/hm-final-after.txt && echo "TREE UNCHANGED"
```

Expected: the test suite exits 0; two `VERIFY: PARTIAL ... does NOT count as
done` lines; one `VERIFY: DONE`; `TREE UNCHANGED`.

This is AC-7, AC-8 and AC-10 re-confirmed after every subsequent commit, which is
the point of running them again here rather than trusting Task 3's evidence.

- [ ] **Step 3: Write `docs/tasks/task-004-process-parity-harness/parity-report.md`**

Structure it as:

1. **Mechanical results** — the sweep output from Steps 1 and 2, verbatim, with
   each AC named and marked pass/fail.
2. **AC-11, asserted structurally.** A passing run cannot prove a command did
   *not* run, so quote the `verify_test.sh` assertion that does:

   ```bash
   cd <worktree> && ./tools/verify_test.sh 2>&1 | grep 'excluded suites'
   ```

3. **AC-23 — `process-parity.md` §7 check 1.** Give this repo's `sha256sum` for
   each of the eight hook files (recorded in Task 5, re-run here to be current),
   then state plainly:

   > The pairwise byte-identity comparison across atlas, home-hub, MyFleet and
   > Harbormaster **cannot be performed from this repository**. These are
   > Harbormaster's hashes only.
   >
   > One divergence is known and deliberate:
   > `.claude/hooks/wait-loop-guard_test.sh` differs from the atlas original in
   > two allow-list fixture strings — `kubectl get pods -n atlas-pr-1370` →
   > `harbormaster-pr-1370` and `journalctl -u atlas` → `journalctl -u
   > harbormaster`. `process-parity.md` §7 check 1 (byte-identity) and this
   > task's AC-1 / FR-1 (`grep -l 'atlas-' .claude/hooks/*.sh` prints nothing)
   > cannot both hold for this file. AC-1 won: it is mechanically checkable here
   > and is a hard criterion, check 1 cannot be evaluated from this repository at
   > all, and §5.2 explicitly authorises genericizing examples. The guard itself,
   > `wait-loop-guard.sh`, is byte-identical. Whoever runs check 1 should expect
   > seven of eight to match.

4. **AC-24 — §7 checks 4, 5 and 6 in their cross-repo form.** Report
   Harbormaster's side only: the hook set and events wired (check 4), the
   `CLAUDE.md` heading list and the trigger table's targets (check 5), and the
   ten owner documents present (check 6). State explicitly that the cross-repo
   comparison is not performed here.
5. **AC-17 — reviewer-judged, not self-asserted.** Do not mark this pass. State
   that it requires a reviewer comparing each ported document against its atlas
   original for a rule dropped along with its example, and list the documents
   with the substantive rebinds so the reviewer knows where to look:
   `verification.md` (whole flag surface), `observability.md` (rewritten; name
   the sections dropped as pipeline mechanics and why each states no rule),
   `slice-first.md` (`doc-slice.sh` replaced), `agent-dispatch.md` (roster),
   `codemod-vs-agents.md` (examples), and the three agent files.
6. **Known follow-ups.** The 7 eslint warnings and whether to tighten
   `npm run lint` (PRD §9 Q1, explicitly out of scope here); the four CI-only
   gates recorded in `docs/verification.md`.

- [ ] **Step 4: Commit**

```bash
cd <worktree> && git add docs/tasks/task-004-process-parity-harness/parity-report.md \
  && git commit -m "docs(task-004): record the parity sweep and the cross-repo report"
```

- [ ] **Step 5: Request code review**

Per `CLAUDE.md`, code review runs before a PR and is not optional. Invoke
`superpowers:requesting-code-review`. It dispatches `plan-adherence-reviewer`
(every task in this plan actually implemented) and, since no Go or TypeScript
source changed, neither guideline reviewer is applicable — say so rather than
dispatching them for nothing.

Additionally dispatch one `task-reviewer` against the full branch range with
**AC-17** as its requirement and the atlas originals as the comparison basis.
That is the criterion no script can check and the one most likely to have been
quietly failed.
