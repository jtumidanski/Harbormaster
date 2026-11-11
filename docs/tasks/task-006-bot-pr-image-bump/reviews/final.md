# task-006 final whole-branch review (96a35fe..e51e3d2)

Reviewer: final cross-task code review (superpowers code-reviewer template).
Inputs: the review package `.superpowers/sdd/plan/review-96a35fe..e51e3d2.diff`,
`plan.md` (Global Constraints, Review Focus), `design.md`, the SDD ledger, and
per-task reviews `task-1.md` to `task-8.md`. Evidence I did not re-run: a
flagless `tools/verify.sh` run returned `VERIFY: DONE` at 8fa753c (later
commits are docs-only), and actionlint shows only the baseline finding.

I reviewed the diff in one pass. I read every changed workflow, script, test
and doc hunk, then checked the end-to-end flow against the current
`.forgejo/workflows/main.yml` in the worktree.

## Verdict

**Ready to merge: with fixes (docs only).** The code path is sound and fails
closed everywhere I looked. One Important finding: the rollout checklist and
the runbook both predict a log line on the post-merge re-run that the code
never reaches. Fix that before the operator works through the checklist. No
code change is required.

## Strengths

- **The stale guard was redefined correctly for the new flow.** Bump merges
  now move main's tip. `stale-guard.sh` checks two things: that GITHUB_SHA is
  an ancestor of HEAD, and that no path other than `deployment.yaml` changed
  between them. That matches exactly the commit set `main.yml`'s
  `paths-ignore` skips, so a bump merge that lands mid-build does not lose a
  deploy, and a newer code commit still makes the run stale. The rewrite is
  applied to the tip, so a human `deployment.yaml` edit is preserved.
  I traced C1/C2 interleavings under the `bump-manifest` concurrency group.
  Each one ends with either the newer pin or a stale skip, never a rollback.
- **The fail-closed chain holds from end to end.**
  - The classifier prints only `true` or `false`. A crash under `pipefail` in
    `changes` makes `changes` fail.
  - The suite jobs gate on `!= 'true'`.
  - `pr-complete` runs under `always()` and requires `changes == success`.
  - Only the exact string `true` selects bump mode, and every one of the ten
    jobs must match the expected result.
  - Every degradation I could construct gives either a red `pr-complete` or a
    full suite run. Examples: a dropped `changes` output, an unparseable
    expression, a garbled BUMP_ONLY, or a cancelled run.
- **The classifier was tightened beyond the design (Review Focus 3).** The
  removed line must also be an image line with the same indentation, it uses
  `--no-renames`, and the header is skipped by position. The 17 table cases
  cover the realistic bypass shapes, including the `-- `/`++ ` content-line
  trick found in Task 1.
- **The `pr-complete_test.sh` agreement checks (a) to (f)** pin the job list,
  `needs`, env keys, per-job `needs`/`if`, `always()`, the missing `name:`,
  and the absence of `paths-ignore`. That closes Review Focus 5 structurally.
- **Token handling is better than before.** The token is no longer in the push
  URL. It reaches git only through checkout's persisted credential and reaches
  the API only through a header. Every API call uses `FORGE_PUSH_TOKEN`, so the
  bot PR triggers `pull_request`.
- **The `bump-manifest` steps are idempotent:**
  - The `pr` step reuses an open PR.
  - It never force-pushes.
  - `supersede` collects every page before it closes anything.
  - `supersede` excludes its own branch and forks.

## Issues

### Critical

None.

### Important

**I1. The post-merge re-run expectation is wrong in the rollout checklist and
the runbook.**

- Files:
  - `docs/tasks/task-006-bot-pr-image-bump/rollout-checklist.md:79-83`
  - `docs/operator/image-bump.md:35-36` and `:166-167`
- What's wrong: the checklist's item 7 says that after the bump PR has merged,
  re-running `bump-manifest` makes the `pr` step log `PR #N is merged`
  (main.yml:473), and that the lookup's 200 "still proves `%2F` decoding". The
  code never gets there. The re-run checks out `ref: main`, whose tip now
  contains the merged pin. The `pin` step's sed therefore produces no diff, so
  it logs `already pinned to sha-<7> — nothing to bump` (main.yml:382) and
  writes `skip=true`. That skips `supersede` and `pr` entirely: no lookup is
  made and nothing is logged as "merged". The runbook repeats the same claim:
  :166-167 says "a re-run for a commit whose PR is already merged logs
  'merged'", and :35-36 describes the merged branch of the `pr` step as if it
  were the normal path. The Task 6 review had already noted this ("A merged
  re-run normally stops even earlier, at 'already pinned'"). The Task 8 fix
  round, following the controller's ruling, wrote the opposite into item 7.
- Why it matters: the operator will see a different log line than the
  checklist predicts, during the one live rollout. They may read a correct
  no-op as a failure, or tick a `%2F` check that was never exercised. AC5
  itself still holds, because the re-run is a green no-op with no duplicate
  PR. Coverage is not lost either, because item 4's open-PR re-run already
  proves `%2F` decoding.
- How to fix:
  - Item 7: expect `already pinned to sha-<7> — nothing to bump` in the `pin`
    step, with `supersede`, `pr` and `automerge` skipped. Drop the `%2F` claim
    and point to item 4.
  - Runbook :166-167: say the same thing.
  - Runbook :35-36: note that the "merged" branch is only reached if main's
    pin has since been changed by something other than this run.

### Minor

**M1. 409 from the merge endpoint is accepted wholesale** (main.yml:540).
- The constraint "2xx or 409 = success" comes from the design and atlas.
  From memory of Gitea's `MergePullRequest`, and unverified against this
  forge: 409 is also returned on the *immediate*-merge path for push-out-of-date,
  SHA-mismatch or push-rejected errors. That path runs when the checks are
  already green at call time, for example a re-run on a green, unscheduled PR.
  In that case the step reports "auto-merge scheduled" and nothing merges.
- Low likelihood, and recovery is the runbook's "green but not merging" row.
- Can wait. Optionally log the 409 body, which the step currently discards on
  success.

**M2. `delete_branch_after_merge` on a *scheduled* auto-merge is unverified on
this forge.**
- Rollout checklist item 6 observes it ("its branch was deleted"). The runbook
  says nothing for the case where it is not honoured: merged `bot/image-bump-*`
  branches would pile up, because `supersede` only deletes branches of *open*
  PRs. That is harmless to correctness.
- Can wait. Add a line to the runbook if item 6 shows branches left behind.

**M3. The token is passed in curl argv** (main.yml:403, :450, :537).
- `-H "Authorization: token …"` is visible in `/proc/<pid>/cmdline` to other
  processes on the host executor while the request runs. The old
  `git push https://…:${TOKEN}@…` had the same exposure, so this is not a
  regression. It also lasts longer now: up to 12 attempts × 10 s in
  `automerge`.
- Can wait. Feed the header through `curl -H @-` or `--config -` from stdin.
  Runner topology was not verified; the design states runner capacity 1.

## Declined to judge

- **Whether Forgejo honours job-level `concurrency`.** This predates the
  branch, and the plan keeps the setting unchanged. The C1/C2 safety analysis
  above assumes it is honoured. Without it, two bump jobs could interleave
  between `supersede` and the branch push and leave a conflicting PR. Recovery
  would still be the next push.
- **A same-repo writer can pin an image built by a branch `workflow_dispatch`.**
  Such a dispatch still pushes `sha-<short>` (main.yml:294). The writer could
  then open a `bot/image-bump-*` PR that skips the suite. This falls under the
  documented trust boundary: same-repo writers who can merge are already
  trusted, and Review Focus 1 makes it policy.
- **Pre-existing `.github/workflows/` references** in `docs/git-workflow.md`,
  `docs/tooling-conventions.md` and `docs/architecture/overview.md`. They are
  outside this diff.
- **The `sed`-matches-nothing → "already pinned" ordering** (main.yml:381-385).
  The plan says to keep today's order; see the triage of the deferred minors
  below.

## Deferred-minor triage

None of the deferred minors needs fixing before merge. Each was checked for a
fail-open direction, and none has one.

| Ledger item | Verdict | Reason |
|---|---|---|
| T1: no rename-of-deployment.yaml case | can wait | `--no-renames` already turns a rename into delete+add (2 paths). The "bot deletes a file" case exercises that mechanism. |
| T1: classify-pr.sh `2>/dev/null` hides git error text | can wait | Diagnosability only. The result is still `bump-only=false` with "git diff failed". |
| T2: empty/TRUE/yes cases don't assert the FAIL line | can wait | Exit 1 and `Mode: full` are asserted. A crash would also exit non-zero, which is closed in every case. |
| T2: comma in a result value mangled by join+sed | can wait | Cosmetic. Result values never contain commas. |
| T3: git diff failure exits 128, not 2 | can wait | Still non-zero under `set -e`, so the `pin` step fails loudly. The header only promises exit 2 for an empty SHA or an ancestry failure. |
| T3: trailing space in the stderr reason | can wait | Cosmetic. |
| T4: verification.md:362 sentence tacked onto a long line | can wait | Formatting. |
| T4: verification.md:355 "a twelfth gate was scope that task did not authorise" reads stale | can wait | Now misleading, because there *are* twelve gates and integration vet still isn't one of them. A one-line edit; worth folding into the I1 doc fix. |
| T4: verification.md:327 "one command in a subshell" vs the loop gate | can wait | Guidance text only. |
| T4: verify_test lacks no-match/ordering tests for ci scripts | can wait | The gate already fails on no match (`found=0` → return 1). |
| T5: yq join errors on a scalar `needs` | can wait | The test still fails, just with a worse message. |
| T5: agreement test doesn't pin the `changes` job | can wait | Dropping `changes` or its output degrades to running the full suite on bumps, or to a red `pr-complete`. Never a pass. |
| T5: no assertion that `${{ }}` is absent inside `run:` | can wait | Correct by inspection today, and the risk is injection hygiene only. |
| T6: main.yml:370 only matches `stale=true` | can wait | The guard prints exactly one of two lines or exits non-zero, and a non-zero exit fails the step under `set -e`. The "unknown → proceed" branch can't be reached. |
| T6: curl network failure aborts `automerge` at attempt 1 | can wait | Fails closed (red job), and the runbook's re-run recovery applies. |
| T6: a sed that matches nothing → "already pinned" before the grep | can wait | Kept from today's order per the plan. But it is the one *silent* failure: if the image line's form changes, every bump goes green with no deploy. Recommend a follow-up to move the grep sanity check (or a pre-sed `grep -q`) ahead of the "already pinned" exit. |
| T7: the 403-body claim doesn't apply to a `git push` failure | can wait | Doc precision. Fold into the I1 doc edit (image-bump.md:120-123). |
| T7: `automerge` also prints ≤300 bytes of body per attempt | can wait | Doc precision; the runbook understates it. |
| T8 M1: checklist :87-89 omits runbook row 2 (:163) from the "unverified" update list | can wait | Row 2 does carry the "unverified forge behaviour" text. A one-word fix; fold into I1. |
| T8 M2: the scope and limit=50 checks sit under item 1 | can wait | Defensible, since they are preconditions. |
| T8 M3: no checkbox for AC10 "main.yml comment is gone" | can wait | Verified by review: the old "no such rule" comment is removed. |
| T8: checklist :50-51 assumes the PR stays open until item 5 | can wait | The "not observed, retry on a later bump" fallback covers it. |

## Assessment of the rulings

- **Pre-flight: T8 Step 2 via task-verifier, not the implementer.** Sound. This
  is the CLAUDE.md rule, and the flagless DONE result was obtained that way.
- **T5: `pr-complete` keeps a sparse checkout of `tools/ci`.** Sound. Context
  decision 5 moved the verdict logic into a script, so the script has to be on
  the runner. The trust exposure is unchanged: `pr.yml` itself already comes
  from the PR tree. Cost is seconds.
- **T6: the actionlint baseline moved to main.yml:209.** Sound. It is the same
  finding, shifted down one line by the header comment.
- **T6: `supersede` collects every page before closing.** Sound, and strictly
  better than the brief. Closing while paging shifts the offsets and skips PRs.
- **T7: keep the closed → exit 0 behaviour and fix the runbook instead.**
  Sound. It respects a deliberate human close and matches design §4.3. The
  runbook's recovery text (merge by hand, or the next push) is correct for
  closed and conflicting PRs.
- **T8: move the 409 / `%2F` / "reusing it" check into item 4, and have item 7
  expect "PR #N is merged".** Half sound. Moving the open-PR re-run into item 4
  is right: it is the only point where the lookup returns 200 *and*
  `automerge` runs. The item 7 half is wrong. After the merge, the re-run
  stops at `pin` with "already pinned", so the `pr` step never runs; see I1.
  The ruling's "cost if wrong: none" also holds only for code. The operator
  would be misled during the one live rollout.

## Merge preconditions (process, not findings)

- `docs/operator/image-bump.md:106` still reads **unconfirmed**. Per design §6
  and checklist item 2, it must be replaced with the observed context string
  in this same PR before merge.
- The `FORGE_PUSH_TOKEN` scope check (checklist item 1) comes before the
  merge.

---

## Re-review of fix wave (e51e3d2..ac3b464)

Verdict: APPROVED_WITH_FINDINGS. 0 blocking, 1 non-blocking. Scope: the three
docs files in ac3b464, checked against `.forgejo/workflows/main.yml` `pin`,
`pr` and `automerge`.

| # | Finding | Verdict | Evidence |
|---|---|---|---|
| 1a | Checklist item 7 expects the `pin` skip | ADDRESSED | `rollout-checklist.md` item 7 quotes `already pinned to sha-<7> — nothing to bump`, cites `main.yml:382` (matches the `echo`), says green job, no new PR, `pr` never runs, no %2F claim. It points %2F proof at item 4, whose re-run cites `main.yml:470` (matches `PR #$num is open, reusing it`). |
| 1b | Runbook :166-167 says "already pinned" | ADDRESSED | `image-bump.md:168-171` (the note after the failure table). The quoted line matches `main.yml:382` exactly, including the em dash. |
| 1c | Runbook :35-36 conditions the merged/closed path | ADDRESSED | `image-bump.md:35-40`: reached only when the PR is not open and main's pin still differs; the post-merge re-run is sent to section 6, which is the stuck-PR recovery section holding the note. The code logs `PR #$num is $state` (`main.yml:473`), so "is merged" and "is closed" are both real. |
| 2 | `verification.md` "twelfth gate" reworded | ADDRESSED | `verification.md:~355` now names the `go vet -tags=integration` gate. No other "twelfth" remains in the file. |
| 3 | Token scope: 403 for API, git auth error for push/ls-remote | ADDRESSED | `image-bump.md:~123-126`. The API calls in `supersede` (PATCH/DELETE) and `pr` (POST /pulls) go through `api()`, which prints the 403 body (`main.yml` api helper). `git ls-remote` and `git push` fail with git's own error. Note that ls-remote exit codes other than 0 or 2 also emit `::error::git ls-remote ... failed` (`main.yml` `*)` case), which the wording does not contradict. |
| 4 | Checklist update list covers every "unverified forge behaviour" row | ADDRESSED | Checklist item 7 now lists rows 1, 2 and 3 plus the residual-race paragraph. Runbook lines 164, 165, 166 and 179 are exactly those four. |

Log-line verification: every quoted line in the touched text matches the code:
`already pinned to ${short} — nothing to bump` (:382), `PR #$num is open,
reusing it` (:470), `PR #$num is $state` (:473), `auto-merge scheduled (HTTP
$code)`.

Non-blocking: `image-bump.md:39` is one overlong line after the reflow
(cosmetic). The runbook quotes `"PR #n is merged" / "closed"` as shorthand,
and the real line is `PR #n is closed`. Both are unambiguous to an operator.

New breakage in the fix diff: none found.
