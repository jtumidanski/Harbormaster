# Harbormaster — Image bump runbook

`deploy/kubernetes/deployment.yaml` names the image Argo CD deploys. Its tag is
machine-written: every build of `main` pins it to `sha-<7-char sha>` through an
auto-merged bot PR. This page covers how that works, what it needs from the
forge, and how to recover when a bump PR gets stuck.

Design and rationale: `docs/tasks/task-006-bot-pr-image-bump/design.md`.

---

## 1. Flow

`main` is protected: required checks apply to pushes as well as merges, and
admins are enforced, so a direct push to `main` is rejected. The bump
therefore goes through a PR.

1. A push to `main` runs `.forgejo/workflows/main.yml`. After the image is
   built and the multi-arch manifest is published, the `bump-manifest` job
   runs (branch pushes only, never tags).
2. Step `pin` runs `tools/ci/stale-guard.sh`. If the built commit is not on
   `main`, or any path other than `deploy/kubernetes/deployment.yaml` changed
   between it and the tip of `main`, the run is stale and the job skips: a
   newer run will pin itself. Otherwise it rewrites the image line to
   `sha-<7>`. If the file is already pinned to that value it skips. A guard
   that cannot resolve ancestry exits 2 and fails the step rather than losing
   a deploy.
3. Step `supersede` pages through **all** open PRs first, then closes every
   open PR whose head branch starts with `bot/image-bump-`, is in this
   repository (never a fork), and is not this run's own branch. It deletes the
   closed PRs' branches (a 404 on delete is accepted). This happens before
   our branch exists so two bump PRs never edit the same line from the same
   base.
4. Step `pr` looks up an existing PR for `bot/image-bump-<short-sha>`. If one
   is open it is reused. If it is already **merged** or closed, the log says so
   ("PR #n is merged" / "closed") and the job exits 0 without scheduling
   auto-merge. This path is reached only when the PR is not open and main's
   pin still differs, for example a PR closed by hand; a normal post-merge
   re-run never reaches it (see section 6). A closed PR is not reopened. Otherwise it pushes the branch
   (never force-pushing; an existing branch is reused), and opens the PR.
5. Step `automerge` asks the forge to squash-merge the PR when checks succeed
   and to delete the branch afterwards. It retries up to 12 times, 10 seconds
   apart, because a just-opened PR can be briefly refused. HTTP 409 counts as
   success (already scheduled by an earlier attempt).
6. The PR runs `.forgejo/workflows/pr.yml`. Its `changes` job classifies the
   PR (section 2). For a bump-only PR the ten suite jobs are skipped and
   `pr-complete` passes if they were all skipped; any other PR runs the full
   suite and `pr-complete` passes only if all ten succeeded.
7. The squash-merge commit touches only `deploy/kubernetes/deployment.yaml`,
   which `main.yml` ignores through `paths-ignore`, so no further build starts.
   Argo CD syncs the new pin.

Every API call and the branch push in `bump-manifest` use `FORGE_PUSH_TOKEN`,
not the run token. A PR opened with the run token does not trigger
`pull_request` workflows, so `pr-complete` would never report and the PR
would never merge.

---

## 2. Bump-only classification

`tools/ci/classify-pr.sh` prints exactly one line, `bump-only=true` or
`bump-only=false`. It is `true` only when **all five** checks hold:

1. The head ref starts with `bot/image-bump-`, and none of `HEAD_REF`,
   `HEAD_REPO`, `REPO` or `BASE_REF` is empty.
2. The head repository equals the base repository (a fork never qualifies).
3. The changed files (`git diff --no-renames --name-only
   origin/<base>...HEAD`) are exactly `deploy/kubernetes/deployment.yaml`.
4. The zero-context diff of that file has exactly one removed and one added
   line. Lines before the first `@@` hunk header are treated as the file
   header and skipped by position, so a content line that happens to look like
   `--- x` is still counted.
5. The added line is `image: registry.tumidanski/jtumidanski/harbormaster:sha-<7 hex>`,
   the removed line is an `image:` line for the same repository with any tag,
   and both have identical leading whitespace. The removed-line check stops a
   PR from deleting some other line while adding a valid-looking pin.

**It fails closed.** Any failure of any check, an empty input, or a failing
`git diff` prints `bump-only=false`, which runs the full suite. In
`tools/ci/pr-complete.sh`, only the exact string `true` selects bump-only
mode; an unset, empty or garbled value means full mode.

`pr-complete` aggregates the results:

- full mode: `changes` and all ten suite jobs must be `success`;
- bump-only mode: `changes` must be `success` and all ten suite jobs must be
  `skipped`.

Anything else, including an unset result, fails with a
`pr-complete: FAIL` line naming the offenders. The job runs with
`if: always()`, so a failed or cancelled upstream job still produces a red
`pr-complete` instead of a missing one.

### Trust boundary

`pull_request` runs `.forgejo/workflows/pr.yml` and `tools/ci/*.sh` from the
PR's own tree, so a human PR that edits them can turn `pr-complete` green.
This is not new (a PR could already delete jobs from `pr.yml`), but the
trust is now concentrated in two scripts, `classify-pr.sh` and
`pr-complete.sh`. Changes under `.forgejo/` and `tools/ci/` need human
review.

---

## 3. Required context

Required context: `pr / pr-complete (pull_request)` — **unconfirmed**: replace this line with the string from this task's PR status list before merge.

After the rollout in section 5, this is the **only** required context on the
`main` rule. The ten suite jobs are no longer required individually;
`pr-complete` covers them.

---

## 4. Token precondition

`FORGE_PUSH_TOKEN` needs the `write:repository` scope: it creates and merges
PRs, edits PRs, pushes branches and deletes branches. This has not been
verified for the current token. Confirm it before the rollout.

A missing scope shows up in the `bump-manifest` log as an HTTP 403 with the
forge's response body (the first 300 bytes) for API calls: a `PATCH`/`DELETE`
in `supersede` or a `POST /pulls` in `pr`. A failed `git push` or
`git ls-remote` shows a git authentication error instead. The `automerge` step prints the status of each attempt
and fails after 12.

---

## 5. Rollout order

1. **Precondition.** Confirm `FORGE_PUSH_TOKEN` has `write:repository`
   (section 4).
2. **The PR that introduces this flow.** The new `pr.yml` runs on it with all
   ten suite jobs plus `pr-complete`. Read the exact `pr-complete` context
   string from that PR's status list and record it in section 3 **before
   merge**, in the same PR. The ten old contexts are still required and
   pass, so it merges normally.
3. **Merge.** `main.yml` builds the merge commit. `bump-manifest` opens the
   first `bot/image-bump-*` PR and schedules auto-merge. Its ten suite jobs
   are skipped and `pr-complete` passes.
4. **Switch protection (manual, forge settings).** Replace the `main` rule's
   required contexts with the single `pr-complete` context. Keep
   `apply_to_admins` on. Until this is done, the bump PR may wait if the
   forge does not count skipped contexts as passing. It auto-merges once the
   rule changes.
5. **Confirm.** The bump PR merges, its branch is deleted,
   `deploy/kubernetes/deployment.yaml` on `main` names the build's
   `sha-<7>`, no new `main.yml` run starts, and Argo CD syncs.

**Rollback.** Restore the ten contexts on the `main` rule and revert the merge
commit. A direct push from `bump-manifest` would still be rejected, so rolling
back means hand-pinning the image tag until the flow is restored.

---

## 6. Stuck-PR recovery

`bump-manifest` is idempotent: re-running it reuses an open PR and an existing
branch, and `409` from the merge endpoint counts as already scheduled.

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| Open bump PR with a red `pr-complete` | The PR was not classified bump-only (read the `changes` job log; `classify-pr.sh` prints why to stderr), or a suite job failed. | If the diff is a pure pin, the classification reason in the log says which check failed. Fix the cause, then merge the PR by hand once the checks are understood, or let the next push to `main` produce a fresh bump PR. Closing the PR and re-running `bump-manifest` for the same commit is expected to do nothing (unverified forge behaviour; see the third row). |
| Open bump PR that is green but not merging | Conflict: main moved under the PR (see residual race below), or auto-merge was never scheduled. | Re-run `bump-manifest` to reschedule. If the PR conflicts, the reliable ways forward are to merge it by hand while it is still open, or to let the next push to `main` produce a fresh bump PR. Closing it and re-running `bump-manifest` for the same commit is expected to do nothing (unverified forge behaviour; see the third row). |
| `bump-manifest` red at `automerge` | The merge endpoint refused all 12 attempts. The log shows the HTTP status and body of each. | A 403 is the token scope (section 4). Otherwise fix the cause and re-run `bump-manifest`. If a PR for the branch was merged or closed by hand, the `pr` step does not reopen it, and a re-run for the same commit is expected to do nothing (unverified forge behaviour: this assumes the forge's PR lookup by branch returns closed PRs). The reliable ways forward are to merge the PR by hand if it is still open, or to let the next push to `main` produce a fresh bump PR. |

Note that a re-run for a commit whose PR is already merged checks out `main`,
which already carries the pin, so the `pin` step logs
`already pinned to sha-<7> — nothing to bump` and the job exits 0 with no new
PR; that is the expected result, not a failure.

**Residual race.** `supersede` closes older bump PRs just before our branch
is created. If an older PR's auto-merge lands at that instant, our new PR no
longer applies cleanly (its base value has changed) and conflicts. The
window is a few seconds, and it is accepted. Recovery is as above: merge the
stuck PR by hand if it is still open, or let the next push to `main` produce a
fresh bump PR. Re-running `bump-manifest` for the same commit after closing its
PR is expected to do nothing (unverified forge behaviour).
