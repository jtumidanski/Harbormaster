# Review: task-006 Task 8 (rollout checklist), 9c7e7af..8fa753c

Spec compliance: FAIL (one requirement misplaced, see I1)
Task quality: Important x1, Minor x3

Scope: the single added file `docs/tasks/task-006-bot-pr-image-bump/rollout-checklist.md`, checked against the Task 8 brief, PRD section 10, `.forgejo/workflows/main.yml`, `.forgejo/workflows/pr.yml` and `docs/operator/image-bump.md`. Step 2 (full verify) is out of scope per the controller ruling.

## PASS, with evidence
- Nine items in the brief's order: checklist sections 1-9 (rollout-checklist.md:16,30,38,42,53,58,65,77,85) match brief items 1-9. Section 10 (the verify gate) is an addition and is harmless.
- PRD section 10 mapping: the AC1-AC11 legend (rollout-checklist.md:3-10) matches the 11 bullets at prd.md:195-216 in order. Each section names its ACs.
- Repo-relative paths only: `grep /home/` returned 0 hits.
- Job and step names: `changes`, `pr-complete`, `bump-manifest` and the ten suite jobs exist (pr.yml:53,318; main.yml:334). The ten suite jobs are frontend-lint/test/build, backend-lint/test/build/integration-build, gitleaks, dependency-scan and license-allowlist, which makes ten.
- `HEAD_REF` and `HEAD_REPO` are set at pr.yml:64-65.
- Operator-doc references hold: "Required context" is at image-bump.md:106, with the "unconfirmed" wording. Section 4 is the token section (:114).
- 409 on already-scheduled auto-merge: main.yml:532-541 treats 409 as success and echoes `auto-merge scheduled (HTTP $code)`, so it is observable in the log. It is correctly placed at item 7 (rollout-checklist.md:68). (Superseded: see round 1, N1.)
- `%2F` on the branch delete: main.yml:431 accepts `204,404`, so the checklist is right to ask for a log check (rollout-checklist.md:81).
- Checkout-persisted credential: main.yml:481 and :498 (`git ls-remote`, `git push`), checklist at :47.
- Closed PRs: main.yml:465-473 handles state, and the checklist at :70-75 feeds image-bump.md rows 1 and 3 and the race paragraph (:162,:164,:175).
- Re-run after the bump merges is not wrongly skipped: the stale guard ignores bump-only movement (main.yml:364-369).
- Step 3: actionlint output in the report shows only the baseline finding. The report states it and I did not rerun it.

## Findings

### Important
- I1. rollout-checklist.md:50-51. The `%2F` PR-lookup confirmation sits under step 4 (first bump PR) as "the lookup found the PR". On the first run no PR exists yet, so main.yml:465 (`GET /pulls/main/$ENC_BRANCH`, `200,404`) returns 404 whether or not Forgejo decodes `%2F`. The first-run log cannot confirm decoding. Only the re-run in step 7 can, because that is where the lookup should find the existing open PR and log "PR #N is open, reusing it" (main.yml:470). Move or duplicate this checkbox under section 7 and state the expected log line. As written, the operator will either tick a box they cannot observe, or read a 404 as a pass.

### Minor
- M1. rollout-checklist.md:73-75. The checklist says to replace the "unverified" wording in failure-table rows 1 and 3 and the race paragraph. Row 2 (image-bump.md:163) carries the same "unverified forge behaviour" statement and is not listed. The doc will be left half-updated.
- M2. rollout-checklist.md:20-28. The `write:repository` sufficiency and `limit=50` checks sit in section 1, not where they are observed. The scope check does cross-reference steps 4, 7 and 8, which is acceptable. The `limit=50` check is really exercised by the supersede step (main.yml:421-423, section 8). A live check there can only show a short page, so the `app.ini` or count approach in section 1 is defensible. Treat as a note.
- M3. rollout-checklist.md:38-40. Item 3 (Merge) maps to AC10, which is loose. AC10's "the `main.yml` comment is gone" is not an explicit checkbox anywhere, since only the operator-doc half is covered (:32-34). Add a one-line check, or state that static review covers it.

## Not evaluable
- None. The live-forge behaviours are the checklist's subject, not the review's.

## Re-review round 1 (8fa753c..8c40631)

Verdict on I1: NOT ADDRESSED. The check moved to the right step but names a log line that item 7 cannot print.

Scope: the 5-insertion diff to `rollout-checklist.md`, read against the `pr` and `automerge` steps in `.forgejo/workflows/main.yml:436-541`.

### I1 trace
- Item 6 (`rollout-checklist.md:58`) requires the bump PR to have merged before item 7 runs.
- On the item-7 re-run, `GET /pulls/main/$ENC_BRANCH` (main.yml:465) returns 200 for the merged PR. Line 468 maps `.merged` to state `merged`.
- Line 469 (`state = open`) is false, so line 470 (`PR #N is open, reusing it`) is never reached. The log prints `PR #N is merged` (main.yml:473) and the step exits 0.
- `rollout-checklist.md:66-67` therefore asks for a line that will not appear. An operator will either tick a box they did not observe or report a failure on a healthy run.
- The underlying logic is sound: a 200 on that lookup proves Forgejo decoded `%2F`. Only the expected line is wrong. Item 7 should expect `PR #N is merged`. The claim "no duplicate PR was created" also holds on that path, since the step exits before any push.
- The hand-closed case in the same section expects `PR #N is closed`.

### Item 7 "Merge endpoint: 409 when already scheduled" (`rollout-checklist.md:71-72`)
- New finding N1, Important. It is unobservable at item 7, and my round-0 PASS on its placement was wrong.
- The `pr` step exits 0 without writing `number=` on the merged and closed paths (main.yml:466-474). The `automerge` step is gated on `steps.pr.outputs.number != ''` (main.yml:511), so it is skipped on the item-7 re-run. No merge-endpoint status can be recorded there.
- Where it can be observed: a re-run while the bump PR is still open. That prints `PR #N is open, reusing it` (main.yml:470) and writes `number=`, so `automerge` runs and POSTs the merge, and the log shows `auto-merge scheduled (HTTP 409|2xx)` (main.yml:537-538).
- Candidate window: after item 4 and before item 5. The old required contexts are still in force there, which plausibly keeps the PR open. That is my inference, not verified against the forge.
- Suggested fix: add a re-run step between 4 and 5 (or a sub-item under item 4) that expects `PR #N is open, reusing it` and records the merge status. That makes the `%2F` check observable there too. Item 7 keeps the merged-path check, expecting `PR #N is merged`.

### Fix-diff breakage
- None beyond the above. The main.yml:470 line reference is accurate. The deleted step-4 item is gone and no other text referenced it.

### Round-1 summary
- I1: NOT ADDRESSED (wrong expected log line).
- N1 (Important): the 409 check is unobservable at item 7. Move it to an open-PR re-run.

## Re-review round 2 (8c40631..e51e3d2)

Verdict: APPROVED_WITH_FINDINGS. I1 ADDRESSED. N1 ADDRESSED.

Scope: the 18-insertion/7-deletion diff to `rollout-checklist.md`, read against the `pr` and `automerge` steps in `.forgejo/workflows/main.yml`.

### Log lines checked against main.yml
- `PR #N is open, reusing it`: main.yml:470, and `number=` is written on the next line (471). Matches `rollout-checklist.md:52-53`.
- `PR #N is merged`: main.yml:473 is `echo "PR #$num is $state"`. The jq at 468 maps `.merged` to `merged`, so the text matches `rollout-checklist.md:79-80`. The `exit 0` follows with no `number=` write.
- `auto-merge scheduled (HTTP <code>)`: main.yml:541, reached only for 2xx or 409. Matches `rollout-checklist.md:57`.
- `automerge` gate is `steps.pr.outputs.number != ''` (main.yml:523), so it is skipped on the merged path. The statement at `rollout-checklist.md:81-83` is correct.

### I1: ADDRESSED
`rollout-checklist.md:79-83` now expects `PR #N is merged` with the correct line reference (473). The claim that the lookup's 200 still proves `%2F` decoding holds, because the lookup at main.yml:465 is the only call involved.

### N1: ADDRESSED
`rollout-checklist.md:50-62` places the 409/2xx observation on an open-PR re-run. That path logs `PR #N is open, reusing it` (main.yml:470) and writes `number=` (471). `automerge` then runs and POSTs the merge, and logs `auto-merge scheduled (HTTP <code>)` (541). The "merged first, record not observed" fallback matches the ruling. Item 7 no longer carries the 409 check and says where it went (`:82-83`).

### Non-blocking
- `rollout-checklist.md:50-51` "waiting on checks" is an inference. Before item 5 the old required contexts are still in force, which plausibly keeps the PR open, but I did not verify that against the forge. The "not observed" fallback covers the case where it is wrong.
- The item 4 sub-check now runs before item 5. Items 4, 5 and 6 still read in the brief's order. No other text references the moved checks.

### Fix-diff breakage
- None.

### Not evaluable
- Live-forge behaviour (whether the PR stays open at item 4, the actual merge-endpoint code) is the checklist's subject, not the review's.
