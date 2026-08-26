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
