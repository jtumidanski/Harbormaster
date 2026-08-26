# Git Workflow

This document owns the mechanics of branch safety, pushing after history
rewrites, what triggers a PR build, and `gh` authentication in this repo.

## Branch safety

Never commit or push directly to `main`. Branch protection blocks the push,
so a commit made on local `main` is stranded and never reaches the remote.
Check the branch before every `git commit`.

Setup work that must precede a feature branch still goes *on* the feature
branch — create it first; it branches from the same HEAD. In this repo that
branch is named `task-NNN-slug` and lives in its own worktree under
`.worktrees/task-NNN-slug/` (see `/spec-task` in the root `CLAUDE.md`).

Recovery from a stray `main` commit: preserve the content on a branch
(cherry-pick if needed), then:

```sh
git fetch origin main && git reset --hard origin/main
```

## Pushing and history rewrites

After completing a rebase/merge/history-rewrite, always push (force-push
when history was rewritten) so the PR reflects the resolved state. Do not
stop at local-only completion — a rebase resolved only locally leaves the PR
still showing conflicts.

## Build triggering and the conflict exception

A push to an open PR's branch triggers `.github/workflows/pr.yml` (frontend
lint/test/build, backend lint/test/build, the integration compile-only
guard, `gitleaks`, dependency scan, license allowlist). Do not merge
`origin/main` into a task branch as a routine build-triggering ritual — a
plain push already reruns the checks.

The one exception: when the branch conflicts with `main`, resolve it —
merge `origin/main`, fix the conflicts, push the merge commit. The merge is
the conflict resolution, not the trigger.

## `gh` authentication

If `gh` commands intermittently fail with a 401 despite being logged in,
suspect a stray `GH_TOKEN` or `GITHUB_TOKEN` environment variable — either
takes precedence over the stored `gh auth login` credentials and, if stale
or scoped wrong, causes the failure. Clear them explicitly so `gh` falls
back to the stored account:

```sh
env -u GH_TOKEN -u GITHUB_TOKEN gh …
```

Never echo the token while diagnosing this.
