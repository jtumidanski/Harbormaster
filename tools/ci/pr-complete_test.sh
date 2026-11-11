#!/usr/bin/env bash
# pr-complete_test.sh — table tests for tools/ci/pr-complete.sh.
#
# Runs the script under `env -i` so the caller's environment cannot leak in.
# No network.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC2034  # consumed by sections appended in later tasks
ROOT="$(cd "$HERE/../.." && pwd)"
SCRIPT="$HERE/pr-complete.sh"

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

[ -x "$SCRIPT" ] || { echo "FAIL - $SCRIPT missing or not executable"; exit 1; }

JOBS=(frontend-lint frontend-test frontend-build backend-lint backend-test
  backend-build backend-integration-build gitleaks dependency-scan license-allowlist)

varname() { local v="${1^^}"; printf 'RESULT_%s' "${v//-/_}"; }

# run_case <changes> <bump_only> <default-result> [job=value|job=UNSET ...]
# Sets OUT and RC.
run_case() {
  local changes="$1" bump="$2" def="$3"
  shift 3
  local envs=("CHANGES_RESULT=$changes" "BUMP_ONLY=$bump") j ov
  for j in "${JOBS[@]}"; do
    local val="$def" skip=0
    for ov in "$@"; do
      if [ "${ov%%=*}" = "$j" ]; then
        val="${ov#*=}"
        [ "$val" = UNSET ] && skip=1
      fi
    done
    [ "$skip" = 1 ] || envs+=("$(varname "$j")=$val")
  done
  OUT="$(env -i PATH="$PATH" "${envs[@]}" bash "$SCRIPT" 2>&1)"
  RC=$?
}

check() { # name want-rc [substring...]
  local name="$1" want="$2" s
  shift 2
  assert_eq "$name: exit" "$want" "$RC"
  for s in "$@"; do assert_contains "$name: contains $s" "$s" "$OUT"; done
}

fail_line() { printf '%s\n' "$OUT" | grep '^pr-complete: FAIL' || true; }
assert_absent() {
  case "$3" in
    *"$2"*) echo "FAIL - $1"; echo "        expected NOT to contain: $2"; fails=$((fails + 1)) ;;
    *) echo "ok   - $1" ;;
  esac
}
# check_fail <name> <offender...>: exit 1, and every offender is named on the
# `pr-complete: FAIL` line (the table above it lists every job, so asserting
# against the full output would be vacuous).
check_fail() {
  local name="$1" s line
  shift
  assert_eq "$name: exit" 1 "$RC"
  line="$(fail_line)"
  assert_contains "$name: has FAIL line" "pr-complete: FAIL" "$line"
  for s in "$@"; do assert_contains "$name: FAIL line names $s" "$s" "$line"; done
}

run_case success false success;                       check "full all success" 0 "pr-complete: PASS" "Mode: full"
run_case success true skipped;                        check "bump all skipped" 0 "pr-complete: PASS" "Mode: bump-only"
run_case success false success gitleaks=cancelled;    check_fail "full one cancelled" gitleaks
run_case success false success backend-test=failure;  check_fail "full one failure" backend-test
run_case success false success frontend-build=skipped; check_fail "full one skipped" frontend-build
run_case success true skipped dependency-scan=success; check_fail "bump one success" dependency-scan
run_case success true skipped license-allowlist=failure; check_fail "bump one failure" license-allowlist
run_case failure true skipped;                        check "changes failed" 1 "pr-complete: FAIL"
run_case skipped false success;                       check "changes skipped" 1 "pr-complete: FAIL"
run_case success "" skipped;                          check "empty bump-only" 1 "Mode: full"
run_case success TRUE skipped;                        check "TRUE is not true" 1 "Mode: full"
run_case success yes skipped;                         check "yes is not true" 1 "Mode: full"
run_case success false success gitleaks=UNSET;        check_fail "missing result var" gitleaks
run_case success false success frontend-lint=failure backend-lint=cancelled
check_fail "two offenders" frontend-lint backend-lint
assert_absent "two offenders: healthy job not blamed" "gitleaks" "$(fail_line)"

# --list-jobs
got="$(env -i PATH="$PATH" bash "$SCRIPT" --list-jobs)"
rc=$?
assert_eq "--list-jobs exit" 0 "$rc"
assert_eq "--list-jobs ids" "$(printf '%s\n' "${JOBS[@]}")" "$got"

# Step summary receives the table header.
SUM="$(mktemp)"
trap 'rm -f "$SUM"' EXIT
env -i PATH="$PATH" GITHUB_STEP_SUMMARY="$SUM" CHANGES_RESULT=success BUMP_ONLY=false \
  "$(varname frontend-lint)=success" "$(varname frontend-test)=success" \
  "$(varname frontend-build)=success" "$(varname backend-lint)=success" \
  "$(varname backend-test)=success" "$(varname backend-build)=success" \
  "$(varname backend-integration-build)=success" "$(varname gitleaks)=success" \
  "$(varname dependency-scan)=success" "$(varname license-allowlist)=success" \
  bash "$SCRIPT" >/dev/null 2>&1
assert_contains "step summary header" "| Job | Result |" "$(cat "$SUM")"

echo
if [ "$fails" -ne 0 ]; then echo "$fails failure(s)"; exit 1; fi
echo "all passed"
exit 0
