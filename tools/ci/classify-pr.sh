#!/usr/bin/env bash
# classify-pr.sh — decide whether a PR is a bot "bump-only" PR.
#
# Env inputs: HEAD_REF, HEAD_REPO, REPO, BASE_REF. Run with cwd = a git
# checkout that has origin/$BASE_REF.
#
# Stdout: exactly one line, "bump-only=true" or "bump-only=false" (suitable
# for appending to $GITHUB_OUTPUT). Diagnostics go to stderr. Exit 0 whenever
# it classifies.

set -euo pipefail

DEPLOYMENT=deploy/kubernetes/deployment.yaml
BRANCH_PREFIX=bot/image-bump-
ADDED_RE='^[[:space:]]*image: registry\.tumidanski/jtumidanski/harbormaster:sha-[0-9a-f]{7}$'
REMOVED_RE='^[[:space:]]*image: registry\.tumidanski/jtumidanski/harbormaster:[^[:space:]]+$'

HEAD_REF="${HEAD_REF-}"
HEAD_REPO="${HEAD_REPO-}"
REPO="${REPO-}"
BASE_REF="${BASE_REF-}"

{
  echo "HEAD_REF=$HEAD_REF"
  echo "HEAD_REPO=$HEAD_REPO"
  echo "REPO=$REPO"
  echo "BASE_REF=$BASE_REF"
} >&2

full() {
  echo "classified full: $1" >&2
  echo "bump-only=false"
  exit 0
}

if [ -z "$HEAD_REF" ] || [ -z "$HEAD_REPO" ] || [ -z "$REPO" ] || [ -z "$BASE_REF" ]; then
  full "empty HEAD_REF, HEAD_REPO, REPO or BASE_REF"
fi

case "$HEAD_REF" in
  "$BRANCH_PREFIX"*) ;;
  *) full "head ref does not start with $BRANCH_PREFIX" ;;
esac

if [ "$HEAD_REPO" != "$REPO" ]; then
  full "head repo $HEAD_REPO is not $REPO (fork)"
fi

if ! files=$(git diff --no-renames --name-only "origin/${BASE_REF}...HEAD" 2>/dev/null); then
  full "git diff failed"
fi
echo "changed files:" >&2
printf '  %s\n' "$files" >&2
if [ "$files" != "$DEPLOYMENT" ]; then
  full "changed files are not exactly $DEPLOYMENT"
fi

if ! diff_out=$(git diff --no-renames -U0 "origin/${BASE_REF}...HEAD" -- "$DEPLOYMENT" 2>/dev/null); then
  full "git diff failed"
fi
removed=()
added=()
in_hunk=false
while IFS= read -r line; do
  # Everything before the first "@@" hunk header is the file header; skip it
  # by position, not by text, so content lines like "--- x" are still counted.
  if [ "$in_hunk" = false ]; then
    case "$line" in
      "@@"*) in_hunk=true ;;
    esac
    continue
  fi
  case "$line" in
    +*) added+=("${line#+}") ;;
    -*) removed+=("${line#-}") ;;
  esac
done <<<"$diff_out"

echo "removed lines: ${#removed[@]}, added lines: ${#added[@]}" >&2
if [ "${#removed[@]}" -ne 1 ] || [ "${#added[@]}" -ne 1 ]; then
  full "diff is not exactly one removed and one added line"
fi
if ! printf '%s\n' "${added[0]}" | grep -Eq "$ADDED_RE"; then
  full "added line is not a sha-<7 hex> image pin"
fi
if ! printf '%s\n' "${removed[0]}" | grep -Eq "$REMOVED_RE"; then
  full "removed line is not an image line"
fi
add_ws="${added[0]%%[![:space:]]*}"
rem_ws="${removed[0]%%[![:space:]]*}"
if [ "$add_ws" != "$rem_ws" ]; then
  full "indentation differs between removed and added lines"
fi

echo "bump-only=true"
