# Review: task-006 Task 3, stale-run guard (2c6de8e..377e25c)

Spec compliance: PASS
Task quality: Approved (2 Minor)

## Evidence
- Shebang, `set -euo pipefail`, chmod 755, shellcheck clean on both files (re-run): stale-guard.sh:1,13; ls -l shows 755.
- Empty GITHUB_SHA exits 2 with stderr: stale-guard.sh:15-18.
- rc captured explicitly: stale-guard.sh:20-21. 0 continue, 1 stale=true exit 0 (24-28), else exit 2 (29-32).
- Diff uses `--no-renames --name-only "$GITHUB_SHA" HEAD -- . ':(exclude)deploy/kubernetes/deployment.yaml'`: line 35, as briefed.
- Stdout is exactly one line on every exit-0 path (26, 38, 43); reasons go to stderr with the briefed wording.
- All 7 table cases present (test:62-91); re-ran, 21 ok lines, exit 0. Each case uses a fresh hermetic repo, GIT_CONFIG_GLOBAL/SYSTEM neutralised (test:23), no network. Style matches assert_eq / `ok   -` / `FAIL -`, non-zero exit on failure (test:93).
- Test honesty: "bump commits only" fails if the exclude pathspec is dropped. "A not on main" (test:77-85) touches only deployment.yaml on the sibling, so only the ancestry branch can yield stale=true; this is the fixture the mutation check used. Code cases fail if the diff check is removed. Unknown SHA (40 x f) gives git rc 128, so exit 2.

## Findings
Minor 1 - stale-guard.sh:35: if `git diff` itself fails (e.g. a shallow clone missing the tree), `set -e` aborts inside the assignment with git's exit code (128), not the documented 2. It still fails loudly with git's stderr, so no deploy is lost silently. Optional: `|| { echo ...; exit 2; }`.
Minor 2 - stale-guard.sh:37: the reason message ends with a trailing space from `tr '\n' ' '`. Cosmetic.

## Process note (not a defect in the artifact)
No RED run was captured (Step 2 skipped). The mutation check later showed the ancestor-1 case is guarded by the revised sibling fixture, so the final tests are not vacuous. I did not re-run the mutation.

## Not evaluable
Behaviour in the real workflow checkout (fetch depth, blobless clone) is outside this unit; the guard assumes the history from GITHUB_SHA to HEAD is present.
