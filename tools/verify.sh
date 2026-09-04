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

# Pin every Go invocation below (the four `go`/`go test`/`go vet`/`go build`
# gates, the golangci-lint gate itself, AND the `go install` bootstrap inside
# gate_golangci) to the toolchain CI actually lints under. golangci-lint
# v2.12.2's bundled honnef.co/go/tools panics building IR for the go1.27
# standard library; a bare `go` on PATH can resolve to 1.27 while
# apps/backend/go.mod pins 1.25.12. CI never hits this because
# .github/workflows/pr.yml's lint job uses actions/setup-go with
# `go-version-file: apps/backend/go.mod` (line ~48), i.e. Go 1.25.12, with the
# same golangci-lint version. Exporting GOTOOLCHAIN here — once, before any
# gate runs — is what makes this script actually reproduce CI instead of
# whatever Go happens to be first on PATH. Set at the top, not per-gate, so no
# future gate can be added without it.
# shellcheck source=./toolchain.versions
. "$ROOT/tools/toolchain.versions"
if [ -z "${GO_VERSION:-}" ]; then
    echo "tools/toolchain.versions: GO_VERSION is unset" >&2
    exit 1
fi
export GOTOOLCHAIN="go${GO_VERSION}"

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
