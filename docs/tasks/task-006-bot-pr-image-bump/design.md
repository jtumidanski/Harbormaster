# Bot-PR Image Bump — Design

Task: task-006-bot-pr-image-bump
PRD: [`prd.md`](prd.md) (approved)
Status: Draft
Created: 2026-11-11

---

## 1. Intent

`main` is protected by required status checks enforced for admins, and Forgejo
applies them to direct pushes. `bump-manifest`'s direct push to `main` is
therefore always rejected, and nothing has deployed since `sha-780b019`
(2026-10-23). The fix routes the image pin through a bot PR with auto-merge, the
same way atlas's `update-image-tags` job already does on this forge, and gives
`pr.yml` one aggregate job so that branch protection needs only one context and a
bump-only PR can skip the suite.

What would make this a failure even if it technically works:

- a human PR can get to green without the full suite (the gate is weakened);
- the deployment rolls *backwards* (an older pin lands after a newer one);
- the pipeline needs a person to step in on the normal path.

## 2. Verified facts this design rests on

These were checked against the live forge's API spec
(`https://git.tumidanski/swagger.v1.json`, `info.version` =
`17.0.1+gitea-1.22.0`) and against atlas's working workflow
(`Chronicle20/atlas`, `.forgejo/workflows/main-publish.yml`, `update-image-tags`;
`pr-validation.yml`, `detect-changes` and `pr-validation-complete`).

| Call | Path | Notes |
|---|---|---|
| Look up the PR for one head branch | `GET /repos/{o}/{r}/pulls/{base}/{head}` | 200 = latest PR for that head in **any** state; 404 = none. Atlas uses this. |
| List open PRs | `GET /repos/{o}/{r}/pulls?state=open&page=N&limit=50` | The `head` query parameter is an **exact** branch-name filter, not a prefix, so the `bot/image-bump-` prefix match is done client-side with `jq`. |
| Create PR | `POST /repos/{o}/{r}/pulls` | `{head, base, title, body}`. |
| Schedule auto-merge | `POST /repos/{o}/{r}/pulls/{n}/merge` | `MergePullRequestOption` has `Do` (enum includes `squash`), `merge_when_checks_succeed`, `delete_branch_after_merge`. Documented responses: 200, 404, 405, 409, 413, 423. |
| Close PR | `PATCH /repos/{o}/{r}/pulls/{n}` | `EditPullRequestOption.state` = `closed`. |
| Delete branch | `DELETE /repos/{o}/{r}/branches/{branch}` | 204, 403, 404, 423. The branch name contains `/`; URL-encode it (`bot%2Fimage-bump-…`). |

Atlas details we copy rather than reinvent:

- Check out with `token: ${{ secrets.FORGE_PUSH_TOKEN }}` so that a plain
  `git push origin <branch>` authenticates through the credential the checkout
  action configured. The token never appears in a URL or in the log (PRD §8,
  Secrets). Today's job puts the token in the push URL; that goes away.
- A PR opened with the run token does not trigger `pull_request` workflows, so
  every API call uses `FORGE_PUSH_TOKEN`.
- Auto-merge retry: 12 attempts, 10 s apart. 2xx or 409 is success. Anything
  else is logged with the first 300 bytes of the body, and if all attempts fail
  the job fails with `::error::`.
- Forgejo throws away `$GITHUB_STEP_SUMMARY`, so the aggregate's result table is
  `tee`d to the log as well.
- Change detection uses `git diff --name-only` on a blobless full-history
  checkout (`fetch-depth: 0`, `filter: blob:none`).

Still unverified, and how the design deals with each:

| Unknown | Handling |
|---|---|
| Whether Forgejo counts a `skipped` required context as passing | Not relied on. Protection moves to the single aggregate context (§6). |
| The exact context string for the aggregate job | Read it from this task's own PR run, which runs the new `pr.yml` (§6). |
| `FORGE_PUSH_TOKEN` scopes (create, merge, close PRs; delete branches) | Operator precondition, checked before the protection switch (§6). The job fails loudly with the HTTP status and body if a scope is missing. |
| `github.head_ref` and `github.event.pull_request.head.repo.full_name` being populated on Forgejo `pull_request` events | The change-detection job logs both. If either one is empty, it classifies the PR as **full** (fails closed). Confirmed on this task's own PR run. |
| Whether `pull_request` on Forgejo checks out the PR head or a merge ref | Doesn't matter: detection diffs against the merge base (`origin/main...HEAD`), which gives the same answer either way. |

## 3. Architecture

```
push to main (code change)
  └─ main.yml: build-amd64 → build-arm64 → manifest → bump-manifest
        bump-manifest (concurrency: bump-manifest, never cancelled)
          1. stale-run guard (code-newest check)       → exit 0 if superseded
          2. rewrite image line; already pinned?       → exit 0
          3. close + delete older bot/image-bump-* PRs
          4. find-or-create branch bot/image-bump-<7>   (push with FORGE_PUSH_TOKEN)
          5. find-or-create PR → main
          6. schedule auto-merge (squash, delete branch), retry
                     │
                     ▼
pull_request on bot/image-bump-<7>
  └─ pr.yml: changes ──► 10 suite jobs (skipped: bump-only)
                     └─► pr-complete (always) ── green ─► Forgejo auto-merges
                                                          (squash, deletes branch)
                     │
                     ▼
squash commit on main touches only deployment.yaml
  └─ main.yml paths-ignore → no new build (loop prevention, unchanged)
  └─ Argo CD harbormaster-main syncs the new tag
```

Three files change, plus the docs. No application code changes.

## 4. `bump-manifest` (`.forgejo/workflows/main.yml`)

### 4.1 Shape

Trigger, `needs: [manifest]`, `if: github.ref == 'refs/heads/main'`, and
`concurrency: { group: bump-manifest, cancel-in-progress: false }` stay as they
are. The job becomes these steps:

1. **Checkout** `ref: main`, `fetch-depth: 0`, `filter: blob:none`,
   `token: ${{ secrets.FORGE_PUSH_TOKEN }}`. Full history is needed for the stale
   guard (§4.2). Blobless keeps it cheap.
2. **Reset toolchain caches** (unchanged).
3. **Pin image** (one `run:` step): stale guard, `sed` rewrite, "already pinned"
   exit, and the post-rewrite `grep`, all kept from today. Writes
   `skip=true|false` to `$GITHUB_OUTPUT` so the later steps can be skipped
   cleanly.
4. **Supersede older bump PRs** (§4.4).
5. **Push branch and open PR** (§4.3). Writes `number` to `$GITHUB_OUTPUT`.
6. **Enable auto-merge** (`if: steps.pr.outputs.number != ''`): the atlas retry
   loop as it stands, with the payload
   `{"Do":"squash","merge_when_checks_succeed":true,"delete_branch_after_merge":true}`.

Every `curl` call uses `-sS -o <file> -w '%{http_code}'` and
`-H "Authorization: token $FORGE_TOKEN"`, logs `METHOD path → HTTP code`, and on
an unexpected code prints `head -c 300` of the body and exits 1 (PRD §8,
Observability). A small shell function `api METHOD PATH [JSON]` inside each step
holds that pattern. Per-step duplication is accepted: steps don't share a shell,
and pulling it into a checked-in script would add a file for about 15 lines.
**Builder's call:** whether to put it in `tools/ci/forge-api.sh` if the duplicate
grows past one function.

`API` = `${{ github.server_url }}/api/v1/repos/${{ github.repository }}`, as in
atlas.

### 4.2 Stale-run guard: redefined (a deliberate refinement of PRD §4.1.2)

The PRD keeps today's check, "if `main`'s tip is no longer `GITHUB_SHA`, exit 0".
That check was correct when bumps were direct pushes. With bot PRs it is wrong,
because **bump merges now move `main`'s tip as well**:

> Human merge A lands; its build starts. An older bump PR for X, opened before A,
> finishes its checks and auto-merges onto A, so the tip becomes `bump(X)` on
> top of A. A's `bump-manifest` then sees tip ≠ A and skips. `main` stays pinned
> to X, which is older than A, until the next human merge. A deploy is silently
> lost.

The intent of the guard is "a newer *code* commit exists on `main`", so the guard
tests exactly that:

```sh
git merge-base --is-ancestor "$GITHUB_SHA" HEAD || { echo "…not on main…"; exit 0; }
newer="$(git diff --name-only "$GITHUB_SHA" HEAD -- . ':(exclude)deploy/kubernetes/deployment.yaml')"
if [ -n "$newer" ]; then echo "main has newer code changes …; skipping"; exit 0; fi
```

Taken together: `GITHUB_SHA` is on `main`, and nothing between it and the tip
changed any path other than `deployment.yaml`. That is exactly the set of
commits `main.yml` ignores through `paths-ignore`, so "the newest commit that
built an image" is still `GITHUB_SHA`, and pinning to it cannot roll anything
backwards. The rewrite is then applied to the **tip** (which may already carry
another bump), not to `GITHUB_SHA`'s tree.

This matches the PRD's acceptance criterion ("a stale run (main moved on) exits
0"). It only stops treating bump-only movement as "moved on". The deviation is
called out here so the plan and the reviewers check it against this section
rather than against the PRD's wording.

### 4.3 Branch and PR, idempotent

Branch: `bot/image-bump-${GITHUB_SHA::7}`. Commit author
`harbormaster-image-bot <harbormaster-image-bot@git.tumidanski>`. The commit
message is today's, minus anything resembling `[skip ci]`.

The algorithm follows atlas's, in this order:

1. `GET pulls/main/<branch>` (branch URL-encoded).
   - 200, `state=open`: reuse it. Set `number` and stop (auto-merge is
     re-requested in step 6; a 409 there means it was already scheduled).
   - 200, any other state: stop with exit 0 and the message "PR #n is <state>".
     A merged PR means the work is done. A PR someone closed by hand is respected
     and not reopened.
   - 404: go on to step 2. Any other code: fail.
2. `git ls-remote --exit-code --heads origin <branch>`: 0 means the branch
   exists, so reuse it without pushing. 2 means it is absent, so
   `git switch -c`, commit, `git push origin <branch>`. Anything else: fail.
   **Never force-push** (atlas: a branch may have been fixed up by hand).
3. `POST pulls` with the title `chore(deploy): pin harbormaster image to sha-<7>`
   and a body that names the run (`main.yml run ${GITHUB_RUN_ID}`), the
   commit it pins (`${GITHUB_SHA}`), the tag, that Argo CD's
   `harbormaster-main` picks it up, and that the PR auto-merges once
   `pr-complete` passes. Non-2xx: fail. The number comes from `jq -r .number`.

This handles both re-run cases from PRD §4.1.6: a branch pushed but no PR
opened, and a PR open but no auto-merge scheduled.

### 4.4 Superseding older bump PRs: before our own branch, not after

PRD §4.1.8 says to supersede "after its own PR is open". This design does it
**before** pushing our own branch. Two bump PRs edit the same line from the same
base value. If the older one merges while the newer one is open, the newer one
no longer applies (base `old`, main `X`, PR `Y`), its squash conflicts, and it
sits open forever. Closing the older ones first narrows that window from "the
whole lifetime of the older PR's checks" down to the seconds between the close
and our branch's creation.

It's safe because the guard in §4.2 has already shown that `GITHUB_SHA` is the
newest built commit. Every other open `bot/image-bump-*` PR pins something older
or the same, and is dead weight. If this run then fails before opening its own
PR, nothing is lost that should have survived: re-running the job (or the next
merge) produces the right pin.

Procedure:

- Page through `GET pulls?state=open&limit=50&page=N` until a page returns
  fewer than 50 items.
- Select the PRs where `.head.ref` starts with `bot/image-bump-`, **and**
  `.head.repo.full_name == .base.repo.full_name` (never a fork), **and**
  `.head.ref != <our branch>` (a re-run must not close its own PR).
- For each one: `PATCH pulls/{n}` `{"state":"closed"}` and expect 2xx, then
  `DELETE branches/<url-encoded ref>` and accept 204 or 404 (already gone).
- Any other failure fails the job. A half-superseded state is visible in the PR
  list and harmless.

PRs from any other prefix are never matched, because the selection is a literal
prefix test on `.head.ref`.

Residual race, accepted and documented: an older PR's auto-merge may be in
flight at the instant we close it. If it lands, our new PR conflicts. Recovery
(§7) is to close the stuck PR, delete its branch, and re-run `bump-manifest`. The
re-run builds the branch from the new tip, and the §4.2 guard still passes.

### 4.5 Header comment

The comment block above `bump-manifest` is replaced. The new one says: `main` is
protected (required checks, admins enforced), Forgejo applies required checks to
pushes, so a direct push is rejected; the job therefore opens a bot PR that
auto-merges once `pr / pr-complete` passes; it uses `FORGE_PUSH_TOKEN`, not the
run token, and why; and loop prevention is still the `paths-ignore` above. The
false claim that "the forge repository carries no such rule" is removed. The
stale-guard comment is updated to the §4.2 definition. The top-of-file
`paths-ignore` comment stays accurate and is reworded only to say "its squash-merge
commit" instead of "its commit".

## 5. `pr.yml`

### 5.1 Trigger

Remove the `paths-ignore` block and its comment. `branches: [main, "task-*"]`
stays. Bump PRs target `main`.

### 5.2 `changes` job (new)

Job id `changes`, `runs-on: ubuntu-24.04`. Output `bump-only` (`true`/`false`).
Checkout `fetch-depth: 0`, `filter: blob:none` (blobless still has every tree,
which is all `git diff` needs, plus on-demand blobs for the `-U0` diff of a
single file).

A PR is **bump-only** iff *all* of these hold. Each one is logged with its value:

1. `HEAD_REF` (`github.head_ref`) starts with `bot/image-bump-`.
2. `HEAD_REPO` (`github.event.pull_request.head.repo.full_name`) equals
   `github.repository`. A fork can't name its way into the bypass.
3. `git diff --name-only origin/${BASE_REF}...HEAD` is exactly
   `deploy/kubernetes/deployment.yaml`, one line.
4. The `-U0` diff of that file has exactly one removed and one added content
   line, and the added line matches
   `^\s*image: registry\.tumidanski/jtumidanski/harbormaster:sha-[0-9a-f]{7}$`.

Conditions 1 and 3 are the PRD's (§4.2.2). Conditions 2 and 4 are this design's
own additions to PRD §8 ("the bypass must not be reachable by a human PR"). Being
inside the bot prefix and the one file still isn't enough to reach the bypass:
the change also has to be shaped exactly like a pin, so a bot-prefixed branch
that edits anything else in `deployment.yaml` (env, probes, securityContext)
runs the full suite.

Any empty input (`HEAD_REF`, `HEAD_REPO`, `BASE_REF`) or a failed `git` command
→ `bump-only=false`, logged as "classified full: <reason>". The step uses
`set -euo pipefail`. A crash fails the job, and the aggregate's
`changes`-must-succeed rule turns that into a red PR. The classifier fails
closed in both ways: it either says `false` or the job is red.

Explicitly a full PR: a human branch that changes only `deployment.yaml`, a bot
branch that changes any other file, a bot branch with a non-pin edit to
`deployment.yaml`, and any fork.

**Git diff, not the "list PR files" API** (PRD §9 open question). Reasons: it
needs no token and no API pagination. It's the mechanism atlas already runs on
this forge. And condition 4 needs the diff *content*, which the files API would
send us back to git for anyway. Cost: about one blobless full-history fetch per
PR, which atlas measured as acceptable at a much larger repo size.

### 5.3 The ten suite jobs

Each of `frontend-lint`, `frontend-test`, `frontend-build`, `backend-lint`,
`backend-test`, `backend-build`, `backend-integration-build`, `gitleaks`,
`dependency-scan`, `license-allowlist` gets:

```yaml
needs: [changes]
if: needs.changes.outputs.bump-only != 'true'
```

`!= 'true'` rather than `== 'false'`: a missing or garbled output runs the suite.
If `changes` itself fails, the default `success()` implied by `needs` skips
these jobs, and `pr-complete` catches that (§5.4). No other line of the ten jobs
changes. In particular, the `golangci-lint-action` `with: { version: … }` line
that `tools/verify.sh`'s drift gate parses is left byte-identical.

Cost on full PRs: the ten jobs now wait for `changes` (one small checkout plus a
diff, expected well under a minute at runner capacity 1). Accepted.

### 5.4 `pr-complete` aggregate (new)

Job id `pr-complete`, no `name:` (so the context is predictable from the id),
`needs: [changes, <the ten>]`, `if: always()`, `runs-on: ubuntu-24.04`, no
checkout. One step, with each `needs.<job>.result` and
`needs.changes.outputs.bump-only` passed through `env:` (no `${{ }}`
interpolation inside the script body).

Logic, in this order:

1. Print the mode and a `| Job | Result |` table of all eleven, through
   `say() { printf '%s\n' "$*" | tee -a "$GITHUB_STEP_SUMMARY"; }`.
2. `changes` result ≠ `success` → fail.
3. Mode = bump-only iff the output is exactly `true`. Otherwise full.
4. Full: every one of the ten must be `success`. Bump-only: every one must be
   `skipped`. Any other value, including `cancelled` and `failure` in either
   mode, `success` in bump-only mode, or `skipped` in full mode, → fail, and
   name the offending job(s).

The ten job names are listed once, in a shell array. The `needs:` list and the
array must agree. A plan task adds a tiny check (an `actionlint`-clean workflow
plus a `yq` comparison in the local test, §8) so that a future eleventh job
can't be added to `needs` but left out of the array, or the reverse.

### 5.5 Header comment

The removed `paths-ignore` justification is replaced by a short paragraph:
bump PRs now run this workflow; `changes` classifies them; the suite is skipped
only for a same-repo `bot/image-bump-*` branch whose sole change is the image
pin; `pr-complete` is the single required context; and it fails closed.

## 6. Branch protection and rollout

Order (also written into the operator doc):

1. **Precondition.** The operator confirms that `FORGE_PUSH_TOKEN` has
   `write:repository` (create, merge, edit PRs; delete branches). Unverified
   until then (PRD §9).
2. **This task's PR.** The new `pr.yml` runs on it with all ten jobs (it's a full
   PR) plus `pr-complete`. Read the exact `pr-complete` context string from that
   PR's status list (expected `pr / pr-complete (pull_request)`) and record it in
   the operator doc **before merge**, in the same PR. The ten old contexts are
   still required and pass, so it merges normally.
3. **Merge.** `main.yml` builds the merge commit. The new `bump-manifest` opens
   the first `bot/image-bump-*` PR and schedules auto-merge. Its ten jobs are
   skipped, and `pr-complete` passes.
4. **Switch protection.** Replace the `main` rule's required contexts with the
   single `pr-complete` context. Keep `apply_to_admins` on. Until this happens,
   the bump PR may wait (if Forgejo doesn't count skipped contexts as passing).
   That's acceptable: it auto-merges once the rule changes.
5. **Confirm.** The bump PR merges, its branch is deleted, `deployment.yaml` on
   `main` names the build's `sha-<7>`, no new `main.yml` run starts, and Argo CD
   syncs.

The protection switch is a manual forge-settings action, done by the operator
(PRD §4.3). The design does not script it.

Rollback: restore the ten contexts on the rule and revert the merge commit. The
old direct-push `bump-manifest` would still be rejected, so a rollback means
going back to hand-pinning.

## 7. Documentation

- **New `docs/operator/image-bump.md`.** `docs/operator/` is where the runbook
  docs live (`recovery.md`, `configuration.md`), and this is a runbook. It
  covers:
  1. The flow (the §3 diagram, in prose).
  2. The bump-only classification (all four conditions) and why it fails
     closed.
  3. The required context: the exact string from §6 step 2, and that it is the
     **only** required context.
  4. The `FORGE_PUSH_TOKEN` precondition and how a missing scope shows up (the
     HTTP 403 body in the `bump-manifest` log).
  5. The rollout order (§6).
  6. Stuck-PR recovery. The symptoms: open bump PR with a red `pr-complete`;
     open bump PR that's green but not merging (conflict); `bump-manifest` red
     at the auto-merge step. The fix for each: re-run `bump-manifest` (it's
     idempotent), or close the PR, delete its branch, and re-run, or merge by
     hand. Plus the residual race from §4.4.
- **`deploy/kubernetes/README.md`.** The "machine-written" callout points at
  `.github/workflows/main.yml`, which is stale. It changes to
  `.forgejo/workflows/main.yml`, says "through an auto-merged bot PR", and links
  to `docs/operator/image-bump.md`.
- The workflow header comments: §4.5 and §5.5.

Every path in these docs is repo-relative (`block-home-paths-in-docs.sh`).

## 8. Testing and verification

Workflow YAML has no unit-test harness here. Verification comes in three layers:

1. **Static.** `actionlint` on both workflows (it's installed locally and
   understands `needs`/`outputs`/`if`). `tools/verify.sh` must still print
   `VERIFY: DONE`, and its drift gate must still find the golangci-lint pin in
   `pr.yml`.
2. **Logic extracted and tested locally.** The two pieces of real logic are the
   bump-only classifier (§5.2) and the aggregate decision (§5.4). Both are pure
   functions of their inputs. Each goes in a small script under `tools/ci/`
   (`classify-pr.sh`, `pr-complete.sh`), which the workflow step calls after
   checkout. (`pr-complete` gains a sparse checkout of `tools/ci` only.) Each
   gets a table-driven test, `tools/ci/*_test.sh`, in the style of the existing
   `tools/verify_test.sh`, which builds throwaway git repos for the classifier
   cases. Cases to cover:
   - **Classifier.** The bot pin is bump-only. Each of these is full: a human
     branch with only the pin; a bot branch with a second file; a bot branch with
     a non-pin edit to `deployment.yaml`; a fork; empty `HEAD_REF`; a tag that
     isn't 7 hex.
   - **Aggregate.** All success in full mode passes. All skipped in bump-only
     mode passes. Each of these fails: one cancelled; one failure; skipped in
     full mode; success in bump-only mode; `changes` failed; output empty with
     everything skipped (full mode, so it fails).
   - **The `needs`/array agreement check.**
   The stale guard is likewise extracted (`tools/ci/stale-guard.sh`) and tested
   against a throwaway repo covering: tip == SHA; tip == SHA + bump commits
   (proceed); tip == SHA + a code commit (skip); SHA not on main (skip).

   **Builder's call:** whether `verify.sh` gets a new gate that runs
   `tools/ci/*_test.sh`, or they ride on an existing shell-test gate if one
   already picks up `tools/**/*_test.sh`. Check `tools/verify.sh --list` first.
   Either way they must run under `tools/verify.sh`.
3. **Live.** These can only be confirmed on the forge, and the plan's final
   task lists them for the operator as the PRD §10 acceptance checks: the §6
   rollout; a re-run of a `bump-manifest` job (no duplicate, no failure); a
   second quick merge (the older bump PR is closed and its branch deleted); a
   human PR touching only `deployment.yaml` (full suite).

Extracting the logic into `tools/ci/` scripts is my call, not the PRD's. The
reasons: inline YAML shell can't be tested, and these two decisions *are* the
security boundary of the gate. The API-calling steps stay inline, matching
atlas, because they can only be meaningfully exercised against the forge.

## 9. Alternatives considered

- **Push with a bypass instead of a PR** (a bot user on the protection rule's
  push allowlist, or exempting the file). Exempting the file was tried and
  doesn't bypass status checks (PRD §1). An allowlisted pusher would still face
  required checks with `apply_to_admins` on, and it would put a standing
  unguarded write path onto `main`. Rejected.
- **Keep ten required contexts and rely on "skipped = passing".** Unverified on
  17.0.1, and if it's true, a human PR whose jobs are skipped by any bug would
  pass too. The aggregate makes the pass/fail rule explicit and fails closed.
  Rejected.
- **A separate workflow file for bump PRs.** Two workflows would each have to
  post the same required context, or protection would need contexts from both,
  and a human PR could match the wrong one. A single `pr.yml` with one aggregate
  is simpler and is what atlas does. Rejected.
- **The files API for change detection.** See §5.2. Rejected for condition 4.
- **Port atlas's `reconcile-bump-prs.yml`.** It's a non-goal (PRD §2). Inline
  supersede covers a single image.
- **Force-push the bump branch on re-run.** It simplifies idempotency, but it
  overwrites any hand fix, and atlas deliberately rejects it. Rejected. The
  close, delete, and re-run recovery covers the case where a branch really is
  stale.

## 10. Decisions this design makes (for review)

1. Stale guard = "no newer non-`deployment.yaml` change on `main`" rather than
   "tip == SHA" (§4.2). This refines PRD §4.1.2.
2. Superseding runs **before** pushing our branch (§4.4). This reorders PRD
   §4.1.8.
3. Bump-only adds same-repo and pin-shaped-diff conditions on top of the PRD's
   two (§5.2).
4. Change detection uses git diff against the merge base, not the files API
   (§5.2).
5. The aggregate job id is `pr-complete`, expected context
   `pr / pr-complete (pull_request)`, to be confirmed from a real run (§6).
6. The operator doc is `docs/operator/image-bump.md`, and the deploy README
   callout gets fixed (§7).
7. Classifier, aggregate, and stale-guard logic live in `tools/ci/*.sh` with
   shell tests run by `tools/verify.sh` (§8).
8. Checkout carries `FORGE_PUSH_TOKEN`, so the token leaves the push URL (§2).

## 11. Guidance for `/plan-task`

`tools/task-brief.sh` slices the plan, so task headings must be `### Task N: …`.
A natural order:

1. `tools/ci/classify-pr.sh` and its test.
2. `tools/ci/pr-complete.sh`, its test, and the `needs`/array agreement check.
3. `tools/ci/stale-guard.sh` and its test.
4. Wire the `tools/ci` tests into `tools/verify.sh` (or confirm an existing gate
   runs them).
5. `pr.yml`: trigger, `changes`, `needs`/`if` on the ten jobs, `pr-complete`,
   header comment.
6. `main.yml`: rewrite `bump-manifest` and its comments.
7. Docs: `docs/operator/image-bump.md` and the `deploy/kubernetes/README.md`
   callout.
8. Full `tools/verify.sh`, plus the live-rollout checklist handed to the operator
   (§6 and §8.3), including recording the real context string before merge.
