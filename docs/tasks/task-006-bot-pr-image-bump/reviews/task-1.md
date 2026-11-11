# Review: task-006 Task 1 (96a35fe..b9a8c87)

verdict: CHANGES_REQUIRED
Spec compliance: PASS on all 15 table rows and the stdout/exit/stderr contract (test run: PASS; shellcheck clean; both files mode 755).
Task quality: 1 Critical.

## Blocking
- Critical, tools/ci/classify-pr.sh:65 - the header skip `"+++ "* | "--- "*` also matches real content lines. A deleted line whose text starts with `-- ` (diff line `--- ...`) or an added line starting with `++ ` (diff line `+++ ...`) is dropped from the count. Reproduced: base has `-- evil` above the image line; a bot branch deletes `-- evil` and bumps the pin; classifier prints `bump-only=true` (removed=1, added=1) even though a non-pin line was deleted. This fails open, against the stated "any doubt -> false" requirement. Fix: with `-U0` and a single file, skip headers positionally (drop everything before the first `@@`), or run `git diff --numstat` / count via `--unified=0` after the first hunk header. Add test cases for a deleted `-- x` line and an added `++ x` line.

## Non-blocking
- Minor, tools/ci/classify-pr_test.sh: no case pins the rename/delete-of-deployment.yaml bypass that `--no-renames` exists for (brief table did not require one). Consider a case renaming deployment.yaml.
- Minor, classify-pr.sh:49,58: `2>/dev/null` hides git's error text; the reason line says only "git diff failed". Acceptable.

## Evidence (PASS)
- Order of checks 1-5 matches brief (lines 36-85); `case` prefix not regex (40-43); `if ! files=$(...)` guard (49, 58).
- Regexes identical to the global constraint (15) and brief (16); whitespace equality (81-85).
- Test hermetic: temp repo, GIT_CONFIG_GLOBAL/SYSTEM=/dev/null (33), no network; exits non-zero on failure (120-123).

## Re-review round 1 (b9a8c87..73933bc)

Verdict: APPROVED

1. Header skip swallowing content lines: ADDRESSED.
   - tools/ci/classify-pr.sh:62-71 now uses an `in_hunk` flag. Every line before the first `@@` is skipped by position, and the `"+++ "* | "--- "*` text match is gone. Content lines such as `--- x` or `+++ x` now reach the `+*` / `-*` arms.
   - tools/ci/classify-pr_test.sh:109-110 adds the two required cases: a deleted `-- x` line and an added `++ x` line. Both expect `false`.
   - The base fixture gets a `-- evil` line at tools/ci/classify-pr_test.sh:50. It is untouched by `sed 's/harbormaster:$OLD_TAG/.../'`, so the other cases are unaffected. The whole suite prints `PASS: all classify-pr cases`.

Test honesty check (the implementer had not run this):
- I copied the b9a8c87 classify-pr.sh and the new test file into a temp dir and ran the test there.
- The result was `FAIL: 4 assertion(s) failed`. Both new cases got `bump-only=true` where `false` was wanted, and neither stderr contained "classified full:".
- So the new tests fail against the old classifier and pass against the fix.

New breakage in the fix diff: none found.

Out-of-scope observations: none.
