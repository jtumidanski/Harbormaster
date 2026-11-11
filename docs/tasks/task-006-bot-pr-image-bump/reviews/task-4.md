# Review: task-006 Task 4 (ci scripts gate), 377e25c..97c7ae3

Spec compliance: PASS. Task quality: Approved (Minor findings only).

Scope: tools/verify.sh, tools/verify_test.sh, docs/verification.md in the range.

## Evidence
- Label `ci scripts`, registered between `npm run build` and the docker block: tools/verify.sh:317; unconditional (no skip). `--list` prints 12 lines with it before `docker buildx`.
- Glob `tools/ci/*_test.sh`: tools/verify.sh:249-261. Unmatched glob leaves the literal, `[ -e ]` fails, `found` stays 0, so it prints `no tools/ci/*_test.sh found` and returns 1. No vacuous pass.
- First failure returns 1: `bash "$t" || return 1` (verify.sh:255). File name echoed first.
- Usage line added at verify.sh:92.
- verify_test.sh: counts 12 / 11, `ci scripts` asserted for flagless and --quick (lines 53-67). `bash tools/verify_test.sh` passes.
- docs: "eleven" -> "twelve" at all 3 sites (verification.md:81, 93, 379); no remaining "eleven". Known drift: "three things" (:348), `ci scripts` explained (:362-365). Repo-relative paths only.

## Findings
- Minor, docs/verification.md:362: the `ci scripts` sentence is appended to a line that is still over-long ("...next local run. The third is **`ci scripts`**, which runs"). Rewrap, ideally as its own paragraph.
- Minor, docs/verification.md:358 (CI-only table row for `go vet -tags=integration`): says "a twelfth gate was scope that task did not authorise". A twelfth gate now exists, so this reads as stale/confusing. Reword to the original-gate-list wording.
- Minor, docs/verification.md:327: "Adding a gate" step 1 says a gate runs one command in a subshell. `gate_ci_scripts` is a loop, not a subshell. Harmless; the procedure is slightly out of step.
- Minor, tools/verify_test.sh: no test pins the no-match failure or the position (before docker). Not required by the brief.

Not evaluable: gate-executing behaviour (the no-match and first-failure paths) was not run, per instructions. Read from source only.
