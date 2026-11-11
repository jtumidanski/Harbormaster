#!/usr/bin/env bash
# stale-guard_test.sh — table tests for tools/ci/stale-guard.sh.
#
# Each case builds a fresh hermetic temp repo whose HEAD is "main's tip" and
# runs the guard with GITHUB_SHA set to the built commit. No network.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$HERE/stale-guard.sh"

fails=0
assert_eq() {
  if [ "$2" = "$3" ]; then echo "ok   - $1"
  else echo "FAIL - $1"; echo "        want: $2"; echo "        got:  $3"; fails=$((fails + 1)); fi
}

[ -x "$GUARD" ] || { echo "FAIL - $GUARD missing or not executable"; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null

DEP=deploy/kubernetes/deployment.yaml
A_SHA=""

# new_repo <name>: fresh repo with commit A (the built SHA) as HEAD.
new_repo() {
  local d="$TMP/$1"
  mkdir -p "$d"
  cd "$d" || exit 2
  git init -q -b main
  git config user.name test
  git config user.email test@example.invalid
  git config commit.gpgsign false
  mkdir -p deploy/kubernetes apps
  echo "image: sha-0000000" > "$DEP"
  echo "code 0" > apps/x.txt
  git add -A
  git commit -q -m A
  A_SHA="$(git rev-parse HEAD)"
}
bump() { echo "image: sha-$1" > "$DEP"; git commit -q -am "bump $1"; }
code() { echo "code $1" > apps/x.txt; git commit -q -am "code $1"; }

# run_case <name> <want-rc> <want-stdout|-> <sha>
run_case() {
  local name="$1" want_rc="$2" want_out="$3" sha="$4" out rc=0
  out="$(GITHUB_SHA="$sha" "$GUARD" 2>"$TMP/stderr")" || rc=$?
  assert_eq "$name: exit code" "$want_rc" "$rc"
  if [ "$want_out" != "-" ]; then
    assert_eq "$name: stdout" "$want_out" "$out"
    assert_eq "$name: one stdout line" "1" "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
  else
    assert_eq "$name: empty stdout" "" "$out"
    if [ -s "$TMP/stderr" ]; then echo "ok   - $name: stderr message present"
    else echo "FAIL - $name: stderr empty"; fails=$((fails + 1)); fi
  fi
}

new_repo tip
run_case "tip is A" 0 "stale=false" "$A_SHA"

new_repo bumps
bump 1111111; bump 2222222
run_case "bump commits only" 0 "stale=false" "$A_SHA"

new_repo code
code 1
run_case "code commit" 0 "stale=true" "$A_SHA"

new_repo bumpcode
bump 1111111; code 1
run_case "bump then code" 0 "stale=true" "$A_SHA"

new_repo sibling
git checkout -q -b side
# Only deployment.yaml differs, so just the ancestry check can flag it.
echo "image: sha-side" > "$DEP"; git commit -q -am side
SIDE_SHA="$(git rev-parse HEAD)"
git checkout -q main
# main moves on from A; the built SHA is on a sibling branch only.
bump 1111111
run_case "A not on main" 0 "stale=true" "$SIDE_SHA"

new_repo unknown
run_case "unknown SHA" 2 "-" "$(printf 'f%.0s' $(seq 40))"

new_repo empty
run_case "empty SHA" 2 "-" ""

[ "$fails" -eq 0 ] || { echo "$fails failure(s)"; exit 1; }
echo "all passed"
