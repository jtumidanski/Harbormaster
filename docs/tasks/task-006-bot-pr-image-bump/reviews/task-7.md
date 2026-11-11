# Review: task-006 Task 7 (6939fbe..ceffa17)

Scope: `docs/operator/image-bump.md` (new), `deploy/kubernetes/README.md` callout. Checked against the Task 7 brief, design 4.4/6/7, `tools/ci/{classify-pr,pr-complete,stale-guard}.sh`, `.forgejo/workflows/pr.yml` (`changes`, `pr-complete`) and `.forgejo/workflows/main.yml` (`pin`, `supersede`, `pr`, `automerge`).

Spec compliance: PASS (see Important finding for the one design-inherited weakness).
Task quality: findings below.

## Spec checks (PASS with evidence)
- Six design 7 sections in order: image-bump.md:12, 58, 104, 114, 128, 155. Trust boundary note at :93 (under section 2). Says pull_request runs pr.yml and tools/ci from the PR tree, and that `.forgejo/` and `tools/ci/` need human review (:95-100).
- Required context line (:106) is byte-identical to the brief.
- Token: `write:repository`, 403 body in the `bump-manifest` log (:116-124).
- Five classify checks match classify-pr.sh: 36-38 (empty/prefix), 45 (fork), 54 (file set), 80 (1 removed/1 added), 67-72 (positional header skip), 83-92 (added regex, removed regex, whitespace). Fail-closed matches :32/:49/:58. pr-complete: only exact `true` selects bump-only (pr-complete.sh:24); offender/FAIL wording matches :58; `if: always()` at pr.yml:319.
- Flow matches main.yml: stale-guard exit 2 (stale-guard.sh:31); supersede collects all pages before closing, same-repo filter, excludes own branch, DELETE accepts 204/404; `pr` reuses open, logs merged/closed and exits 0, never force-pushes; `automerge` 12x10s, 409/2xx OK; FORGE_PUSH_TOKEN via checkout token. paths-ignore claim true (main.yml:11-12).
- Rollout steps 1-5 and rollback match design 6.
- README: `.github` reference gone, text says "auto-merged bot PR (`bot/image-bump-<short-sha>`)", link `../../docs/operator/image-bump.md` resolves from `deploy/kubernetes/`. No absolute paths in the doc. The design link (:8) is a code span, not a link, so nothing to break.

## Findings

### Important
1. docs/operator/image-bump.md:162, :163, :172-174 — the primary recovery ("close the PR, delete its branch, and re-run `bump-manifest`") is presented as working, but the doc itself says at :37 and :164 that the `pr` step does not reopen a closed PR. main.yml `pr` step looks up `GET /pulls/main/<branch>` first and on 200 with state closed logs and `exit 0` (see the `exit 0` after the state check). The branch name is `bot/image-bump-<short-sha>` of the same commit, so after closing and deleting the stuck PR, a re-run finds the closed PR (if the forge lookup returns closed PRs; unverified) and does nothing. This is the implementer's concern and it is real. The caveat appears only in row 3 and contradicts rows 1, 2 and the residual-race paragraph. Also :173-174 says "the new branch is built from the current tip", which cannot happen for the same SHA if the closed-PR lookup hits. Fix the runbook: either say plainly that after a close the only reliable ways forward are merge by hand or wait for the next push to `main`, marking the lookup behaviour unverified, or note the recovery is only valid when the lookup returns 404. (Code behaviour is out of Task 7 scope; flag for the controller as a design 4.4/7 defect.)

### Minor
2. docs/operator/image-bump.md:164 — "re-run is a no-op for that branch" is stated as fact; it depends on unverified forge lookup behaviour. Hedge it as unverified.
3. docs/operator/image-bump.md:120-123 — the 403 "HTTP body (first 300 bytes)" does not apply to the branch push, which fails as a git error, not an API 403 body. Reword.
4. docs/operator/image-bump.md:123 — says `automerge` "prints the status of each attempt"; it also prints up to 300 bytes of body (main.yml, `head -c 300`). The 403 body is therefore visible there too.

## Not evaluable
- Whether the forge's `pulls/{base}/{head}` returns closed PRs, whether `write:repository` is sufficient, and the real `pr-complete` context string. All unverifiable from the repo (the doc marks the context as unconfirmed, correctly).

## Re-review round 1 (ceffa17..9c7e7af)

Verdict: ADDRESSED. Finding 1 (Important) and its Minor sibling 2 are resolved.

Evidence, docs/operator/image-bump.md:
- :162 (row 1) no longer says close and delete. It offers merge by hand, or wait for the next push to `main`. The closed-PR re-run is marked "expected to do nothing (unverified forge behaviour)".
- :163 (row 2) says the reliable ways are merge by hand while open, or wait for the next push. Same unverified hedge on the re-run.
- :164 (row 3) now says "expected to do nothing" with the assumption stated (forge lookup by branch returns closed PRs), replacing the former flat "is a no-op". This closes Minor 2.
- :172-175 (residual race) matches rows 1-3. The false "new branch built from the current tip" claim is gone. A grep for "delete its branch", "current tip" and "new branch" returns nothing.
- Ruling check: the workflow is unchanged, since the diff touches only image-bump.md. The runbook states merge by hand or wait for the next push as the reliable recovery, and marks the same-SHA no-op as expected but unverified.

New breakage within the fix diff: none. Rows 1 and 2 cross-reference "the third row", which exists at :164. :166-167 (merged re-run exits 0) is consistent.

Out of scope, unchanged from round 0 and still open: Minors 3 and 4 (:120-123, 403 wording). The underlying `pr`-step behaviour on closed PRs is a design 4.4/7 matter for the controller, as already flagged.
