# Review: task-006 Task 2 (pr-complete aggregate verdict), range 73933bc..aaff5de

Spec compliance: PASS
Task quality: Approved with findings (1 Important, 2 Minor)

Scope: tools/ci/pr-complete.sh, tools/ci/pr-complete_test.sh. Ran the test (all ok, exit 0), shellcheck (clean), both files mode 755.

## Spec evidence
- Fail-closed bump-only: pr-complete.sh:24 uses exact `= "true"`; anything else (TRUE, yes, empty, unset) is full mode, expected=success (:22-27). Tested at test:69-71.
- Missing RESULT_* var: `${!var:-}` (:50) yields empty, which != expected, so it is an offender (:51-52). Tested at test:72 (gitleaks=UNSET).
- changes != success fails, including empty/unset CHANGES_RESULT (:44-46). Tested at test:67-68.
- All offenders collected and named in the FAIL line (:43-58). Output order: Mode, table of eleven with changes first (:31-40), verdict.
- `--list-jobs` prints the ten ids in constraint order from the single SUITE_JOBS array (:11-17); tested at test:77-80 against an independent JOBS list.
- All 13 table cases, --list-jobs and the step-summary header are present. env -i is used (:49, :85). Hermetic, no network, `ok   -`/`FAIL -` style, non-zero exit on failure.
- `set -euo pipefail`, shebang, chmod +x, shellcheck clean.

## Findings
### Important
- tools/ci/pr-complete_test.sh:62-66, 72, 74: the `contains <job>` assertions are vacuous. The script always prints every job name in the results table (pr-complete.sh:36-40), so the substring is present whether or not the job is named in the FAIL reason. A regression that dropped offender names from the reason, or that named only the first offender, would still pass "full one cancelled", "missing result var" and "two offenders". The brief's "reason names every offending job" is therefore untested. Fix: assert against the `pr-complete: FAIL` line only, e.g. extract it with `grep '^pr-complete: FAIL'` and check names there.

### Minor
- tools/ci/pr-complete_test.sh:67-68, 69-71: the exit-1 cases do not assert the FAIL verdict line in all cases, and "empty/TRUE/yes" check only "Mode: full", so the exit 1 could come from a script crash. Add `pr-complete: FAIL` to those checks.
- tools/ci/pr-complete.sh:57: the comma join then `sed 's/,/, /g'` would mangle any value containing a comma. Not reachable with GitHub result values; cosmetic only.

## Not evaluable
- The implementer's red-run only proves the script was absent, not that the verdict logic can fail (per the note). This review exercised the green run only; I did not run mutation checks.

## Re-review round 1 (aaff5de..2c6de8e)

Verdict: ADDRESSED

Finding 1 (Important, vacuous offender assertions): fixed.
- tools/ci/pr-complete_test.sh:63 `fail_line` extracts only `^pr-complete: FAIL`; `check_fail` (:69-77) asserts exit 1, that the FAIL line exists, and that each offender is named on that line. The six single-offender cases and "two offenders" now use it (:88-92, 96-98).
- :99 adds a negative assertion (healthy `gitleaks` not blamed on the FAIL line).
- Mutation check re-run on a temp copy (worktree untouched). Baseline: 0 failures. Dropping `$reason` from the FAIL line (pr-complete.sh:58): 8 failures, matching the report. Naming only `offenders[0]`: "two offenders ... names backend-lint" fails, so the all-offenders requirement is now tested.

New breakage in the fix diff: none.

Out-of-scope / still open (not part of this round):
- Earlier Minor: exit-1 cases at test:93-95 ("changes failed", "changes skipped", empty/TRUE/yes) still use `check`, so "Mode: full" is asserted against full output and the FAIL line is not required for the mode cases.
- Earlier Minor: comma-join/sed at pr-complete.sh:57 mangles values containing commas (unreachable).
