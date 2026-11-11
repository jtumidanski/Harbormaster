#!/usr/bin/env bash
# stale-guard.sh — decide whether main already has code newer than the built SHA.
#
# Run with cwd = a checkout whose HEAD is main's tip. Input: GITHUB_SHA.
# Stdout: exactly one line, "stale=true" or "stale=false". Reasons go to stderr.
# Exit 2 (loudly) on an empty GITHUB_SHA or an unresolvable ancestor check:
# a silent skip would lose a deploy.
#
# Stale means: GITHUB_SHA is not on main, or any path other than
# deployment.yaml changed between it and the tip (bump-only movement is not
# "moved on"; see design section 4.2).

set -euo pipefail

if [ -z "${GITHUB_SHA:-}" ]; then
  echo "stale-guard: GITHUB_SHA is empty" >&2
  exit 2
fi

rc=0
git merge-base --is-ancestor "$GITHUB_SHA" HEAD || rc=$?
case "$rc" in
  0) ;;
  1)
    echo "$GITHUB_SHA is not on main" >&2
    echo "stale=true"
    exit 0
    ;;
  *)
    echo "stale-guard: cannot check ancestry of $GITHUB_SHA (git exit $rc)" >&2
    exit 2
    ;;
esac

newer="$(git diff --no-renames --name-only "$GITHUB_SHA" HEAD -- . ':(exclude)deploy/kubernetes/deployment.yaml')"
if [ -n "$newer" ]; then
  echo "main has newer code changes: $(echo "$newer" | tr '\n' ' ')" >&2
  echo "stale=true"
  exit 0
fi

echo "no newer code on main" >&2
echo "stale=false"
