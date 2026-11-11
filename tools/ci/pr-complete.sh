#!/usr/bin/env bash
# pr-complete.sh — fail-closed aggregate verdict behind the single required
# `pr-complete` context.
#
# Env: CHANGES_RESULT, BUMP_ONLY, and RESULT_<JOB> per suite job (job id
# upper-cased, '-' -> '_'). Exit 0 = pass, 1 = fail.
# `--list-jobs` prints the suite job ids, one per line.

set -euo pipefail

SUITE_JOBS=(frontend-lint frontend-test frontend-build backend-lint backend-test
  backend-build backend-integration-build gitleaks dependency-scan license-allowlist)

if [ "${1:-}" = "--list-jobs" ]; then
  printf '%s\n' "${SUITE_JOBS[@]}"
  exit 0
fi

say() { printf '%s\n' "$*" | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"; }

# Only the exact string "true" selects bump-only; anything else is full mode.
mode=full
expected=success
if [ "${BUMP_ONLY:-}" = "true" ]; then
  mode=bump-only
  expected=skipped
fi

changes="${CHANGES_RESULT:-}"

say "Mode: $mode"
say ""
say "| Job | Result |"
say "|---|---|"
say "| changes | ${changes:-<unset>} |"
for job in "${SUITE_JOBS[@]}"; do
  upper="${job^^}"
  var="RESULT_${upper//-/_}"
  say "| $job | ${!var:-<unset>} |"
done
say ""

offenders=()
if [ "$changes" != "success" ]; then
  offenders+=("changes (${changes:-<unset>})")
fi
for job in "${SUITE_JOBS[@]}"; do
  upper="${job^^}"
  var="RESULT_${upper//-/_}"
  val="${!var:-}"
  if [ "$val" != "$expected" ]; then
    offenders+=("$job (${val:-<unset>})")
  fi
done

if [ "${#offenders[@]}" -gt 0 ]; then
  reason="$(IFS=,; printf '%s' "${offenders[*]}" | sed 's/,/, /g')"
  say "pr-complete: FAIL — expected $expected in $mode mode: $reason"
  exit 1
fi

say "pr-complete: PASS"
