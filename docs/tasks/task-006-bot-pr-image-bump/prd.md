# Bot-PR Image Bump — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-11-11
---

## 1. Overview

`main.yml`'s `bump-manifest` job is the deploy step: after the multi-arch image
for a `main` commit is built, scanned and published, it rewrites the image tag in
`deploy/kubernetes/deployment.yaml` to `sha-<short-sha>` and pushes that commit
straight to `main`. Argo CD's `harbormaster-main` Application on the bee cluster
watches that file.

On 2026-10-24 a branch protection rule was added to `main` (status checks
required — the 10 `pr / … (pull_request)` contexts — and enforced for admins).
Forgejo applies required status checks to direct pushes as well as merges: a
pushed commit must already carry passing statuses. The bump commit is created
inside the job and has none, so every push since then has been rejected by the
pre-receive hook (`remote rejected … (pre-receive hook declined)`, surfaced as
`Forgejo: Internal Server Error`). The last successful bump was `aa07c81`
(2026-10-23, `sha-780b019`); every merge since has failed to deploy. Adding
`deploy/kubernetes/deployment.yaml` as an unprotected file pattern was tried and
does not bypass the status-check requirement.

The chosen fix is to make Harbormaster work the same way the atlas repository
(`Chronicle20/atlas`, `.forgejo/workflows/main-publish.yml`, job
`update-image-tags`) already does on this forge: the bump goes through a bot PR
with auto-merge, and `pr.yml` gains an atlas-style aggregate check so a
bump-only PR is gated by one cheap summary job instead of the full suite.

## 2. Goals

Primary goals:
- Every successful `main` build results in `deployment.yaml` on `main` pinned to
  that build's `sha-<short-sha>` without anyone touching it, with branch
  protection left on and enforced for admins.
- The bump flow mirrors atlas's: bot branch → PR via API → auto-merge (squash,
  delete branch).
- A bump-only PR does not run the full lint/test/build suite.
- Human PRs remain gated exactly as strictly as today (all ten checks must pass).

Non-goals:
- Changing Argo CD (it keeps tracking `deploy/kubernetes/` on `main`).
- Porting atlas's `reconcile-bump-prs.yml` (single image; superseded PRs are
  closed instead).
- Backfilling the pins missed since 2026-10-23 (the first `main` push after this
  lands pins itself).
- Changes to `release.yml`, `nightly.yml`, or any backend/frontend code.

## 3. User Stories

- As the operator, I want a merge to `main` to deploy itself so that I don't
  hand-edit `deployment.yaml` after every merge.
- As the operator, I want `main` to stay protected by required checks so that a
  red PR can't be merged, including by me.
- As the operator, I want a bump PR to merge in about a minute without running
  the whole suite, so that deploys aren't delayed or runner capacity wasted.
- As the operator, I want stale bump PRs cleaned up automatically so that the PR
  list doesn't accumulate dead bot PRs.

## 4. Functional Requirements

### 4.1 `bump-manifest` (in `.forgejo/workflows/main.yml`)

1. Runs only on `refs/heads/main` after `manifest` succeeds (unchanged), with the
   existing `concurrency: bump-manifest`, `cancel-in-progress: false`.
2. **Stale-run guard kept:** if `main`'s tip is no longer `GITHUB_SHA`, exit 0
   without opening a PR.
3. Rewrites the image line exactly as today (`sha-${GITHUB_SHA::7}`), keeps the
   "already pinned → exit 0" and the post-rewrite `grep` sanity check.
4. Commits as `harbormaster-image-bot` on branch
   `bot/image-bump-<short-sha>` and pushes that branch with `FORGE_PUSH_TOKEN`
   (not the run token — atlas documents that a PR opened by the run token does
   not trigger `pull_request` workflows, which would leave auto-merge with
   nothing to wait on). No `[skip ci]` in the message.
5. Opens a PR `bot/image-bump-<short-sha>` → `main` via the Forgejo API. The PR
   body states which build it pins and that it auto-merges.
6. **Idempotent on re-run:** looks up an existing open PR for the branch before
   creating one, and tolerates the branch already existing (re-running the job
   must not fail or open a duplicate).
7. Enables auto-merge via the API: `Do: squash`,
   `merge_when_checks_succeed: true`, `delete_branch_after_merge: true`. HTTP 409
   ("already scheduled") counts as success. Any other non-2xx fails the job with
   the response body printed. Retries follow atlas's pattern (bounded attempts
   for the transient case where the PR's mergeability is not yet computed).
8. **Supersede older bumps:** after its own PR is open, closes every other open
   PR whose head branch matches `bot/image-bump-*` and deletes those branches.
   Never touches PRs from any other branch prefix.
9. Loop prevention is unchanged: `main.yml`'s `paths-ignore` on
   `deploy/kubernetes/deployment.yaml` means the squash-merge commit does not
   start another build.

### 4.2 `pr.yml`

1. Remove `paths-ignore: deploy/kubernetes/deployment.yaml` from the
   `pull_request` trigger, so the workflow runs on bump PRs.
2. Add a change-detection job that classifies the PR as **bump-only** iff
   **both**: the head branch starts with `bot/image-bump-`, **and** the PR's
   changed files are exactly `deploy/kubernetes/deployment.yaml`. Anything else
   (including a human PR touching only `deployment.yaml`) is a full PR.
3. Each of the ten existing jobs (`frontend-lint`, `frontend-test`,
   `frontend-build`, `backend-lint`, `backend-test`, `backend-build`,
   `backend-integration-build`, `gitleaks`, `dependency-scan`,
   `license-allowlist`) depends on change detection and is skipped when the PR is
   bump-only; otherwise it runs exactly as today.
4. Add an aggregate job (`if: always()`, needs change detection and all ten jobs)
   that:
   - fails if change detection did not succeed;
   - on a full PR, passes only if all ten jobs are `success`;
   - on a bump-only PR, passes only if all ten are `skipped`;
   - writes a per-job result table to the step summary.
   A `cancelled` or `failure` result for any job fails it in both modes.
5. Rewrite the `pr.yml` header comment that justifies the `paths-ignore`.

### 4.3 Branch protection (manual, documented step)

1. After this change is on `main`, the `main` rule's required status contexts
   are replaced with the single aggregate context. `apply_to_admins` stays on.
2. The exact context string (expected form `pr / <job-name> (pull_request)`) is
   taken from a real run, not guessed, and recorded in the docs.
3. Rollout order is documented (see §8).

### 4.4 Documentation

1. Replace the `main.yml` comment above `bump-manifest` that says the forge
   repository carries no branch protection rule; describe the bot-PR flow and
   why a direct push cannot work.
2. Add a short section to an operator/CI doc (location decided in design)
   covering: the bump flow, the bump-only classification, the required context,
   the token precondition, and how to recover a stuck bump PR.

## 5. API Surface

No Harbormaster application API changes. Forgejo REST API calls made by the
workflow (verify each path and payload against the Forgejo 17 API before
implementation — do not port from memory; atlas's working calls are the
reference):

- List open PRs for the repo (filter by head branch prefix `bot/image-bump-`).
- Create PR (`head`, `base: main`, `title`, `body`).
- Merge PR with `merge_when_checks_succeed: true` (auto-merge); 409 = already
  scheduled.
- Close PR (edit `state: closed`) and delete branch for superseded bumps.

## 6. Data Model

None. The only file the bot writes is `deploy/kubernetes/deployment.yaml`.

## 7. Service Impact

- `.forgejo/workflows/main.yml` — `bump-manifest` rewritten; header comments.
- `.forgejo/workflows/pr.yml` — trigger, change detection, `needs`/`if` on ten
  jobs, new aggregate job, header comment.
- Docs — one CI/operator section (location per design).
- Forge settings (manual) — `main` branch protection contexts.
- `apps/backend`, `apps/frontend`, `deploy/` manifests: no changes.

## 8. Non-Functional Requirements

- **Safety of the gate:** the bump-only bypass must not be reachable by a human
  PR. Both conditions in §4.2.2 are required; the aggregate fails closed on any
  unexpected result.
- **Rollout without a gap:** this task's own PR is gated by the current ten
  contexts and runs the full suite, so it merges normally. Its merge triggers
  `main.yml`, whose new `bump-manifest` opens the first bump PR. Then the
  protection contexts are switched to the aggregate. Until they are, the bump PR
  may wait (its ten jobs are skipped; whether Forgejo treats a skipped required
  context as satisfied is unverified) — that is acceptable, it auto-merges once
  the rule changes.
- **Secrets:** `FORGE_PUSH_TOKEN` is never echoed; API calls use an
  `Authorization` header, not a URL with the token where avoidable.
- **Cost:** a bump-only PR runs change detection + the aggregate only.
- **Observability:** every API call's HTTP status is logged; failures print the
  response body.

## 9. Open Questions

- **Token scope (precondition):** `FORGE_PUSH_TOKEN` must be able to create,
  merge (auto-merge) and close PRs and delete branches via the API
  (`write:repository`). Its current scopes are unverified; the operator confirms
  before rollout.
- **Skipped required contexts:** whether Forgejo 17.0.1 counts a `skipped`
  required context as passing is unverified. The design does not rely on it
  after the protection rule is switched to the aggregate, but it affects how
  long the first bump PR waits during rollout.
- **Aggregate context name:** confirm the exact context string Forgejo reports
  for the new job from the first run.
- **Change detection mechanism:** git diff against the merge base vs. the
  Forgejo "list PR files" API — decide in design.

## 10. Acceptance Criteria

- [ ] A push to `main` that builds successfully results in an open
      `bot/image-bump-<short-sha>` PR changing only `deployment.yaml`, with
      auto-merge (squash, delete branch) enabled.
- [ ] On that PR, the ten suite jobs are skipped and the aggregate job passes.
- [ ] With branch protection requiring only the aggregate context (admins
      enforced), the bump PR merges automatically and its branch is deleted.
- [ ] `deployment.yaml` on `main` then names `sha-<short-sha>` of the build, and
      the merge commit does not trigger another `main.yml` run.
- [ ] Re-running a `bump-manifest` job for the same SHA neither fails nor opens a
      duplicate PR.
- [ ] When a newer bump PR is opened, older open `bot/image-bump-*` PRs are
      closed and their branches deleted.
- [ ] A stale run (main moved on) exits 0 without opening a PR.
- [ ] A human PR (any branch not `bot/image-bump-*`) runs all ten jobs, and the
      aggregate fails if any of them fails — including a human PR that touches
      only `deployment.yaml`.
- [ ] A bot-prefixed branch that changes any file other than `deployment.yaml`
      runs the full suite.
- [ ] The `main.yml` comment claiming no branch protection exists is gone; the
      operator doc describes the flow, the required context, the token
      precondition and stuck-PR recovery.
- [ ] `tools/verify.sh` prints `VERIFY: DONE`.
