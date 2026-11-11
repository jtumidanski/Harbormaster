# Review: task-006 Task 5 (e2ca346) — pr.yml changes / gated suite / pr-complete

Scope: `git diff 97c7ae3..e2ca346` (pr.yml, tools/ci/pr-complete_test.sh), plus
`tools/ci/pr-complete.sh` and `tools/ci/classify-pr.sh` for their contracts.
Brief: `.superpowers/sdd/plan/task-5-brief.md`. Design §5.1–§5.5.

## Verdicts
- Spec compliance: PASS
- Task quality: Approved (Minor notes only)

## Evidence
- `changes` job (pr.yml, first job): `ubuntu-24.04`, checkout `fetch-depth: 0, filter: blob:none` (same pinned SHA), cache reset, `id: classify`, env HEAD_REF/HEAD_REPO/REPO/BASE_REF, `set -euo pipefail` + `classify-pr.sh | tee -a "$GITHUB_OUTPUT"`. `bump-only` output wired. A classifier crash fails the step under pipefail, so the job fails and `pr-complete` rejects on `CHANGES_RESULT != success` (pr-complete.sh, `changes` offender check).
- fetch-depth 0 fetches all branches, so `origin/$BASE_REF` resolves. The classifier falls to `bump-only=false` if the diff fails, which is safe.
- Ten suite jobs: each has `needs: [changes]` and `if: needs.changes.outputs.bump-only != 'true'` directly under the id. The diff shows no other line changed in them. The golangci `with:` line (pr.yml:155) has 0 diff lines matching golangci.
- `pr-complete`: no `name:`, `if: always()`, needs = changes + the ten in the constraint order, one step with `CHANGES_RESULT`, `BUMP_ONLY` and 10 `RESULT_*` env entries, `run: tools/ci/pr-complete.sh`. No `${{ }}` inside any added `run:`.
- Sparse checkout: pr-complete.sh is mode 100755, so it is directly executable. It only needs bash and its own SUITE_JOBS array. It reads nothing else under tools/ and nothing outside tools/ci. `sparse-checkout: tools/ci` therefore suffices.
- `paths-ignore` and its comment are removed, and the header paragraph per §5.5 is in place.
- Reran `bash tools/ci/pr-complete_test.sh`: all passed. `actionlint pr.yml main.yml`: only the baseline main.yml:208 finding.
- Agreement test (pr-complete_test.sh, new section) covers (a)–(f). Missing yq fails the test. An eleventh job in pr.yml but not in the script/needs/env trips (a), (b) and (c). The reverse trips (a) as well. Losing `needs` or `if` on a suite job trips (d), per job. The implementer's mutation runs (b) and (d) are consistent with the code.

## Findings
- Minor — pr-complete_test.sh (d): `(.needs // []) | join(",")` would make yq error if `needs` were a scalar string. It still fails the test (empty != "changes"), so it fails closed, just with a poor message.
- Minor — The test does not pin the `changes` job itself: `fetch-depth: 0` on its checkout, the classify step env, or `outputs.bump-only`. Removing `fetch-depth: 0` would turn every bump PR into "classified full". That is safe but silently defeats the feature. No test would notice.
- Minor — The test does not assert that no `${{ }}` appears inside `run:` blocks. This is enforced only by review.

## Not evaluable
- The Forgejo context name `pr / pr-complete (pull_request)` is still unconfirmed until this task's own PR runs.
- Runtime behaviour of `actions/checkout` `sparse-checkout` on the Forgejo runner was not run here. Reading the script and the checkout config shows nothing that would break it.
- Design §5.4 says "no checkout" while the brief specifies a sparse checkout. The brief governs, and the sparse checkout is required for the script to exist.
