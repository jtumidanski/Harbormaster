# Bot-PR Image Bump Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy every successful `main` build again by having `bump-manifest` pin the image through an auto-merged bot PR instead of a direct push. `pr.yml` gets one fail-closed aggregate context, `pr-complete`, so a bump-only PR skips the suite.

**Architecture:** Three pure decisions become small, tested shell scripts under `tools/ci/`: the bump-only classifier, the aggregate verdict, and the stale-run guard. The workflows call them, and `tools/verify.sh` gains a `ci scripts` gate that runs their tests. The Forgejo API calls (supersede, find-or-create branch and PR, auto-merge) stay inline in `main.yml` the way atlas does it, because they can only be exercised against the forge.

**Tech Stack:** Forgejo Actions YAML (Forgejo `17.0.1+gitea-1.22.0`), bash, git, `curl`, `jq`, mikefarah `yq` v4, `actionlint`, `shellcheck`.

**Spec:** [`design.md`](design.md) (PRD: [`prd.md`](prd.md)). Where the design deliberately departs from the PRD (§4.2 stale guard, §4.4 supersede order, §5.2 extra conditions), **the design wins**.

## Global Constraints

- Bump branch: `bot/image-bump-${GITHUB_SHA::7}`. Image line regex (added line): `^[[:space:]]*image: registry\.tumidanski/jtumidanski/harbormaster:sha-[0-9a-f]{7}$`.
- Commit author `harbormaster-image-bot <harbormaster-image-bot@git.tumidanski>`. Commit and PR title `chore(deploy): pin harbormaster image to sha-<7>`. No `[skip ci]` anywhere.
- Every API call and the `git push` use `FORGE_PUSH_TOKEN`, never the run token. The token never appears in a URL or in the log. Header: `Authorization: token $FORGE_TOKEN`.
- `API` = `${{ github.server_url }}/api/v1/repos/${{ github.repository }}`.
- Auto-merge payload `{"Do":"squash","merge_when_checks_succeed":true,"delete_branch_after_merge":true}`. 12 attempts, 10 s apart. 2xx or 409 = success.
- Never force-push a bump branch.
- Aggregate job id `pr-complete`, with no `name:`. Expected context `pr / pr-complete (pull_request)`, **unconfirmed** until this task's own PR runs.
- The ten suite jobs, in this order: `frontend-lint frontend-test frontend-build backend-lint backend-test backend-build backend-integration-build gitleaks dependency-scan license-allowlist`.
- The `golangci-lint-action` `with: { version: v2.15.0, working-directory: apps/backend }` line in `pr.yml` stays byte-identical, because the `verify.sh` drift gate parses it.
- `concurrency: { group: bump-manifest, cancel-in-progress: false }`, `needs: [manifest]` and `if: github.ref == 'refs/heads/main'` on `bump-manifest` are unchanged. So is `main.yml`'s `paths-ignore`.
- Every new script starts `#!/usr/bin/env bash` + `set -euo pipefail`, is `chmod +x`, and is `shellcheck`-clean. Tests follow `tools/verify_test.sh`'s style: `assert_eq`/`assert_contains`, `ok   - …`/`FAIL - …` lines, a non-zero exit on any failure, hermetic temp git repos, and no network.
- Committed docs use repo-relative paths only.
- `actionlint .forgejo/workflows/pr.yml .forgejo/workflows/main.yml` baseline: exactly **one** pre-existing finding, `main.yml:208` (`https://git.tumidanski/ci/actions/select-arm64-builder@v1` "invalid format"). After each workflow task, that must still be the only finding.
- `git add` named paths only. Never `-A` or `.`.

## Review Focus

1. **The PR author controls the gate code.** `pull_request` runs `pr.yml` and `tools/ci/*.sh` from the PR's own tree, so a human PR that edits them can turn `pr-complete` green. This is not new (today a PR can delete jobs from `pr.yml`), but it is now concentrated in two scripts. Task 7's operator doc says so, and says that changes under `.forgejo/` and `tools/ci/` need human review. No test exists for this; it's policy.
2. **A rename or delete hiding behind the one-file check.** `git diff --name-only` with rename detection lists only the new path. The classifier uses `--no-renames`, so a delete or rename shows both paths. Task 1 tests a bot branch that deletes a second file.
3. **A non-pin edit made to look like a pin.** A bot-prefixed branch removes some other `deployment.yaml` line (e.g. `imagePullPolicy`) and adds an `image:` line. The design only constrains the *added* line. This plan also requires the *removed* line to be a `harbormaster` image line with the same leading whitespace (a fail-closed tightening of design §5.2 condition 4). Task 1 tests it.
4. **A garbled or case-variant `bump-only` output.** `TRUE`, `yes`, an empty value, or a missing `RESULT_*` env var must all mean full mode or fail, never pass. Task 2 tests these.
5. **An eleventh suite job added to `pr.yml` but not to `pr-complete`** (or the reverse), or a suite job that loses its `needs: [changes]`/`if:`. Task 5 adds a `yq` agreement test that compares the jobs in `pr.yml`, `pr-complete.needs`, `pr-complete`'s `RESULT_*` env keys, and `pr-complete.sh --list-jobs`.

---

## File map

| File | Status | Responsibility |
|---|---|---|
| `tools/ci/classify-pr.sh` | create | bump-only decision (design §5.2) |
| `tools/ci/classify-pr_test.sh` | create | table tests on throwaway repos |
| `tools/ci/pr-complete.sh` | create | aggregate verdict (design §5.4) |
| `tools/ci/pr-complete_test.sh` | create | verdict table + workflow agreement check |
| `tools/ci/stale-guard.sh` | create | "newer code on main?" (design §4.2) |
| `tools/ci/stale-guard_test.sh` | create | throwaway-repo tests |
| `tools/verify.sh` | modify | `ci scripts` gate |
| `tools/verify_test.sh` | modify | gate counts 11→12 / 10→11 |
| `docs/verification.md` | modify | gate count, drift table |
| `.forgejo/workflows/pr.yml` | modify | trigger, `changes`, ten `needs`/`if`, `pr-complete`, header |
| `.forgejo/workflows/main.yml` | modify | `bump-manifest` rewrite, comments |
| `docs/operator/image-bump.md` | create | runbook (design §7) |
| `deploy/kubernetes/README.md` | modify | machine-written callout (lines 12–14) |
| `docs/tasks/task-006-bot-pr-image-bump/rollout-checklist.md` | create | live checks handed to the operator |

---

### Task 1: Bump-only classifier

**Files:**
- Create: `tools/ci/classify-pr.sh`
- Test: `tools/ci/classify-pr_test.sh`

**Interfaces:**
- Produces: `tools/ci/classify-pr.sh`, run with cwd = a git checkout that has `origin/$BASE_REF`. Env inputs: `HEAD_REF`, `HEAD_REPO`, `REPO`, `BASE_REF`. **Stdout: exactly one line**, `bump-only=true` or `bump-only=false`, so it can be appended straight to `$GITHUB_OUTPUT`. All diagnostics go to stderr: each condition with its value, and on false, `classified full: <reason>`. Exit 0 whenever it classifies. Non-zero only on an unexpected crash.

- [ ] **Step 1: Write the failing test** `tools/ci/classify-pr_test.sh`.

  Fixture: `mktemp -d` repo, `git init -q -b main`, local `user.name`/`user.email`. Commit `deploy/kubernetes/deployment.yaml` with these lines:
  ```yaml
  spec:
    template:
      spec:
        containers:
          - name: harbormaster
            image: registry.tumidanski/jtumidanski/harbormaster:sha-780b019
            imagePullPolicy: IfNotPresent
  ```
  plus a `README.md`. Then `git update-ref refs/remotes/origin/main HEAD`. Each case does `git switch -q -c <branch> origin/main`, makes its edit, commits, and runs the script with `HEAD_REF=<branch> HEAD_REPO=jtumidanski/Harbormaster REPO=jtumidanski/Harbormaster BASE_REF=main` unless the case says otherwise. Assert stdout equals exactly:

  | Case | Edit / env | Expect |
  |---|---|---|
  | bot pin | branch `bot/image-bump-abc1234`, `sed` tag → `sha-abc1234` | `bump-only=true` |
  | human pin | branch `task-007-x`, same pin | `bump-only=false` |
  | bot + second file | pin + edit `README.md` | `false` |
  | bot deletes a file | pin + `git rm README.md` | `false` |
  | bot non-pin edit | `IfNotPresent` → `Always` only | `false` |
  | bot pin + extra line | pin + `IfNotPresent` → `Always` | `false` |
  | removed line not an image | delete the `imagePullPolicy` line, add a second `image: …:sha-abc1234` line | `false` |
  | short tag | tag → `sha-abc12` | `false` |
  | uppercase tag | tag → `sha-ABC1234` | `false` |
  | fork | bot pin, `HEAD_REPO=mallory/Harbormaster` | `false` |
  | lookalike prefix | branch `xbot/image-bump-abc1234`, pin | `false` |
  | empty HEAD_REF | bot pin, `HEAD_REF=` | `false` |
  | empty HEAD_REPO | bot pin, `HEAD_REPO=` | `false` |
  | empty BASE_REF | bot pin, `BASE_REF=` | `false` |
  | missing base | bot pin, `BASE_REF=nope` | `false` |

  Also assert, for every case, that stdout has exactly 1 line and that the exit code is 0. For the `false` cases, assert stderr contains `classified full:`.

- [ ] **Step 2: Run it and confirm it fails.** `bash tools/ci/classify-pr_test.sh`. Expected: FAIL (script missing).

- [ ] **Step 3: Implement `tools/ci/classify-pr.sh`.**

  Check in this order, stopping at the first failure (`echo bump-only=false`, reason on stderr):
  1. Empty `HEAD_REF`, `HEAD_REPO`, `REPO` or `BASE_REF`.
  2. `HEAD_REF` doesn't start with `bot/image-bump-` (use `case`, not regex).
  3. `HEAD_REPO != REPO`.
  4. `files=$(git diff --no-renames --name-only "origin/${BASE_REF}...HEAD")`. A failing git command means false with the reason `git diff failed`. Guard it with `if ! files=$(…)`, because `set -e` must not turn it into a crash. `files` must equal exactly `deploy/kubernetes/deployment.yaml`.
  5. Take content lines from `git diff --no-renames -U0 "origin/${BASE_REF}...HEAD" -- deploy/kubernetes/deployment.yaml`. These are lines starting with `+` or `-`, excluding the `+++ `/`--- ` headers. There must be exactly one `-` and one `+`. The added text must match the Global Constraints regex (`grep -Eq`). The removed text must match `^[[:space:]]*image: registry\.tumidanski/jtumidanski/harbormaster:[^[:space:]]+$`. The leading whitespace of the added and removed lines must be identical.

  If all pass, print `bump-only=true`. Before deciding, log each input (`HEAD_REF=…`, etc.) and the changed-file list to stderr.

- [ ] **Step 4: Run the tests and confirm they pass.** `bash tools/ci/classify-pr_test.sh` then `shellcheck tools/ci/classify-pr.sh tools/ci/classify-pr_test.sh`. Expected: all `ok`, a final pass line, exit 0, and no shellcheck output.

- [ ] **Step 5: Commit.**
  ```bash
  git add tools/ci/classify-pr.sh tools/ci/classify-pr_test.sh
  git commit -m "feat(ci): bump-only PR classifier with table tests"
  ```

### Task 2: Aggregate verdict

**Files:**
- Create: `tools/ci/pr-complete.sh`
- Test: `tools/ci/pr-complete_test.sh`

**Interfaces:**
- Produces: `tools/ci/pr-complete.sh`. Env inputs: `CHANGES_RESULT`, `BUMP_ONLY`, and one `RESULT_<JOB>` per suite job, where `<JOB>` is the job id upper-cased with `-`→`_` (e.g. `RESULT_BACKEND_INTEGRATION_BUILD`). Exit 0 = pass, 1 = fail. Output goes through `say() { printf '%s\n' "$*" | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"; }`: a `Mode: full|bump-only` line, then a `| Job | Result |` table of all eleven (`changes` first), then either `pr-complete: PASS` or `pr-complete: FAIL — <reason>`, where the reason names every offending job.
- Produces: `tools/ci/pr-complete.sh --list-jobs` prints the ten job ids, one per line, from the single `SUITE_JOBS=(…)` array (Global Constraints order) and exits 0. Task 5 relies on this.

- [ ] **Step 1: Write the failing test** `tools/ci/pr-complete_test.sh`. A helper sets all ten `RESULT_*` to one value, with optional per-job overrides, and runs the script under `env -i PATH="$PATH" …` so the caller's environment can't leak in. Cases:

  | Case | CHANGES_RESULT | BUMP_ONLY | Results | Expect |
  |---|---|---|---|---|
  | full all success | success | false | all `success` | exit 0, `pr-complete: PASS` |
  | bump all skipped | success | true | all `skipped` | exit 0 |
  | full one cancelled | success | false | `gitleaks=cancelled` | exit 1, output contains `gitleaks` |
  | full one failure | success | false | `backend-test=failure` | exit 1, contains `backend-test` |
  | full one skipped | success | false | `frontend-build=skipped` | exit 1 |
  | bump one success | success | true | `dependency-scan=success` | exit 1, contains `dependency-scan` |
  | bump one failure | success | true | `license-allowlist=failure` | exit 1 |
  | changes failed | failure | true | all `skipped` | exit 1 |
  | changes skipped | skipped | false | all `success` | exit 1 |
  | empty output, all skipped | success | `` | all `skipped` | exit 1, `Mode: full` |
  | `TRUE` is not true | success | TRUE | all `skipped` | exit 1, `Mode: full` |
  | missing result var | success | false | all success, `RESULT_GITLEAKS` unset | exit 1, contains `gitleaks` |
  | two offenders named | success | false | `frontend-lint=failure`, `backend-lint=cancelled` | output contains both |

  Plus: `--list-jobs` prints exactly the ten ids in Global Constraints order. In the full-success case, a `GITHUB_STEP_SUMMARY` temp file receives the `| Job | Result |` header.

- [ ] **Step 2: Run it and confirm it fails.** `bash tools/ci/pr-complete_test.sh`. Expected: FAIL.

- [ ] **Step 3: Implement `tools/ci/pr-complete.sh`.** Logic in design §5.4 order: print the mode and the table, then `CHANGES_RESULT != success` → fail; mode = bump-only iff `BUMP_ONLY` is exactly `true`; expected per-job value is `success` (full) or `skipped` (bump-only); collect every job whose value differs (indirect expansion `${!var:-}`), and fail naming all of them. Use `set -euo pipefail`. Unset vars are read with `:-`.

- [ ] **Step 4: Run the tests and confirm they pass.** `bash tools/ci/pr-complete_test.sh` and `shellcheck tools/ci/pr-complete.sh tools/ci/pr-complete_test.sh`. Expected: all `ok`, exit 0.

- [ ] **Step 5: Commit.**
  ```bash
  git add tools/ci/pr-complete.sh tools/ci/pr-complete_test.sh
  git commit -m "feat(ci): fail-closed pr-complete aggregate verdict with tests"
  ```

### Task 3: Stale-run guard

**Files:**
- Create: `tools/ci/stale-guard.sh`
- Test: `tools/ci/stale-guard_test.sh`

**Interfaces:**
- Produces: `tools/ci/stale-guard.sh`, run with cwd = a checkout whose `HEAD` is `main`'s tip. Env input `GITHUB_SHA` (full SHA). **Stdout: exactly one line**, `stale=true` or `stale=false`. The reason goes to stderr (`<sha> is not on main` / `main has newer code changes: <files>` / `no newer code on main`). Exit 0 when decided. Exit 2 with a stderr message when `GITHUB_SHA` is empty, or when `git merge-base --is-ancestor` returns anything other than 0 or 1 (unknown object). A silent skip would lose a deploy, so this fails loudly instead.

- [ ] **Step 1: Write the failing test** `tools/ci/stale-guard_test.sh`. Throwaway repo with `deploy/kubernetes/deployment.yaml` and `apps/x.txt`. Commit `A` is the built SHA. Cases (each in a fresh repo or a reset branch):

  | Case | History after A (HEAD) | Expect |
  |---|---|---|
  | tip is A | none | `stale=false`, exit 0 |
  | bump commits only | two commits editing only `deployment.yaml` | `stale=false` |
  | code commit | one commit editing `apps/x.txt` | `stale=true` |
  | bump then code | bump commit, then code commit | `stale=true` |
  | A not on main | HEAD on a sibling commit that doesn't descend from A | `stale=true` |
  | unknown SHA | `GITHUB_SHA=` 40 × `f` | exit 2 |
  | empty SHA | `GITHUB_SHA=` | exit 2 |

  Assert one stdout line in every exit-0 case.

- [ ] **Step 2: Run it and confirm it fails.** `bash tools/ci/stale-guard_test.sh`. Expected: FAIL.

- [ ] **Step 3: Implement `tools/ci/stale-guard.sh`** following the design §4.2 snippet. The diff is `git diff --no-renames --name-only "$GITHUB_SHA" HEAD -- . ':(exclude)deploy/kubernetes/deployment.yaml'`. Capture the `--is-ancestor` code explicitly (`rc=0; git merge-base … || rc=$?`): 0 → continue, 1 → `stale=true`, else exit 2.

- [ ] **Step 4: Run the tests and confirm they pass.** `bash tools/ci/stale-guard_test.sh`, then `shellcheck` on both files. Expected: all `ok`, exit 0.

- [ ] **Step 5: Commit.**
  ```bash
  git add tools/ci/stale-guard.sh tools/ci/stale-guard_test.sh
  git commit -m "feat(ci): code-newest stale-run guard for bump-manifest with tests"
  ```

### Task 4: `ci scripts` gate in `tools/verify.sh`

No existing gate runs shell tests (`tools/verify.sh --list` shows 11 Go/npm/docker gates), so this adds one, following `docs/verification.md` "Adding a gate".

**Files:**
- Modify: `tools/verify.sh` (add `gate_ci_scripts` near the other gates; usage heredoc "Gates, in order"; registry: `step 'ci scripts' gate_ci_scripts` between `npm run build` and the docker block)
- Modify: `tools/verify_test.sh` (lines ~53–63)
- Modify: `docs/verification.md` (lines 81, 93, 376: "eleven" → "twelve"; the "Known drift" section)

**Interfaces:**
- Consumes: `tools/ci/*_test.sh` from Tasks 1–3 (and Task 5's extension).
- Produces: gate label `ci scripts`. It runs in every mode, never skipped.

- [ ] **Step 1: Update the tests first.** In `tools/verify_test.sh`, change the counts: `flagless selects 12 gates`, `--no-docker selects 11 gates`. Add `assert_contains "flagless includes ci scripts" "ci scripts" "$flagless"` and the same for `$quick`. Run `bash tools/verify_test.sh`. Expected: FAIL on the counts and the `ci scripts` asserts.

- [ ] **Step 2: Implement `gate_ci_scripts()`.** It globs `"$ROOT"/tools/ci/*_test.sh`. If the glob matches nothing, it returns 1 with `no tools/ci/*_test.sh found` (no vacuous pass). It runs each one with `bash`, prints the file name first, and returns 1 at the first failure. Add the usage line `  ci scripts        tools/ci/*_test.sh (classifier, pr-complete, stale guard)`.

- [ ] **Step 3: Run the tests and confirm they pass.** `bash tools/verify_test.sh` → all ok. `tools/verify.sh --list` → 12 lines, including `ci scripts` before `docker buildx`.

- [ ] **Step 4: Docs.** In `docs/verification.md`, change "eleven" to "twelve" at the three sites. In "Known drift", change "the script runs two things CI does not" to three. Add a sentence to the script-only paragraph: `ci scripts` runs `tools/ci/*_test.sh`, which CI doesn't run, because those scripts *are* the CI gate's logic and a PR could edit both the scripts and their tests together. CI exercises them by running them, not by testing them.

- [ ] **Step 5: Commit.**
  ```bash
  git add tools/verify.sh tools/verify_test.sh docs/verification.md
  git commit -m "feat(verify): ci scripts gate runs tools/ci shell tests"
  ```

### Task 5: `pr.yml`: `changes`, gated suite, `pr-complete`

**Files:**
- Modify: `.forgejo/workflows/pr.yml` (lines 1–13 trigger/header; each of the ten jobs; append `changes` before `frontend-lint` and `pr-complete` at the end)
- Modify: `tools/ci/pr-complete_test.sh` (add the agreement section)

**Interfaces:**
- Consumes: `tools/ci/classify-pr.sh` (Task 1) and `tools/ci/pr-complete.sh` + `--list-jobs` (Task 2), with exactly their env/stdout contracts.
- Produces: job `changes` with `outputs: { bump-only: ${{ steps.classify.outputs.bump-only }} }`, and job `pr-complete`, the single future required context.

- [ ] **Step 1: Write the failing agreement test.** Append a section to `tools/ci/pr-complete_test.sh` that reads `$ROOT/.forgejo/workflows/pr.yml` with `yq` (fail the test if `yq` is missing) and asserts:
  - (a) the sorted list of all job ids minus `changes` and `pr-complete` equals the sorted `pr-complete.sh --list-jobs`;
  - (b) `.jobs."pr-complete".needs` equals `changes` plus that same set;
  - (c) the `RESULT_*` keys in `pr-complete`'s step `env`, mapped back (lower-case, `_`→`-`), equal the same set, and that env also has `CHANGES_RESULT` and `BUMP_ONLY`;
  - (d) every one of the ten has `needs` = `[changes]` and `if` = `needs.changes.outputs.bump-only != 'true'`;
  - (e) `.jobs."pr-complete".if` = `always()` and `.jobs."pr-complete".name` is null;
  - (f) `.on.pull_request."paths-ignore"` is null.

  Run `bash tools/ci/pr-complete_test.sh`. Expected: FAIL on (a)–(f).

- [ ] **Step 2: Trigger and header.** Delete lines 5–13 (the comment and `paths-ignore`). Insert the design §5.5 paragraph as a comment above `permissions:`. It says that bump PRs now run this workflow; that `changes` classifies them; that the suite is skipped only for a same-repo `bot/image-bump-*` branch whose sole change is the image pin (`tools/ci/classify-pr.sh`); that `pr-complete` is the single required context; and that it fails closed. Point to `docs/operator/image-bump.md`.

- [ ] **Step 3: `changes` job.** `runs-on: ubuntu-24.04`. Checkout (same pinned SHA as the other jobs) with `fetch-depth: 0` and `filter: blob:none`, then the `Reset toolchain caches` step. Then step `id: classify` with env `HEAD_REF: ${{ github.head_ref }}`, `HEAD_REPO: ${{ github.event.pull_request.head.repo.full_name }}`, `REPO: ${{ github.repository }}`, `BASE_REF: ${{ github.base_ref }}`, run `tools/ci/classify-pr.sh | tee -a "$GITHUB_OUTPUT"`, with `set -euo pipefail` so a crash fails the job.

- [ ] **Step 4: Ten suite jobs.** Add `needs: [changes]` and `if: needs.changes.outputs.bump-only != 'true'` directly under each job id. Change nothing else, and keep the golangci-lint `with:` line untouched.

- [ ] **Step 5: `pr-complete` job.** `if: always()`, `needs: [changes, <ten in Global Constraints order>]`, `runs-on: ubuntu-24.04`. Checkout with `sparse-checkout: tools/ci`. One step `env:` with `CHANGES_RESULT: ${{ needs.changes.result }}`, `BUMP_ONLY: ${{ needs.changes.outputs.bump-only }}`, and the ten `RESULT_*: ${{ needs.<job>.result }}`, `run: tools/ci/pr-complete.sh`. No `${{ }}` inside `run:`.

- [ ] **Step 6: Verify.**
  - `bash tools/ci/pr-complete_test.sh` → all ok.
  - `actionlint .forgejo/workflows/pr.yml .forgejo/workflows/main.yml` → only the baseline `main.yml:208` finding.
  - The drift gate still finds the pin: `bash tools/verify_test.sh`. Its first-failure section runs the real drift gate before the stubbed `go` fails, and asserts the verdict names `go test`. That assertion only holds if drift passed. Expected: all ok.

- [ ] **Step 7: Commit.**
  ```bash
  git add .forgejo/workflows/pr.yml tools/ci/pr-complete_test.sh
  git commit -m "feat(ci): pr.yml changes classifier and pr-complete aggregate"
  ```

### Task 6: `main.yml`: `bump-manifest` through a bot PR

**Files:**
- Modify: `.forgejo/workflows/main.yml` (lines 6–9 the `paths-ignore` comment; lines 313–382 the comment block and job)

**Interfaces:**
- Consumes: `tools/ci/stale-guard.sh` (Task 3): stdout `stale=true|false`, exit 2 on error.
- Produces: step ids `pin` (output `skip`), `supersede`, `pr` (output `number`), `automerge`.

No local test can exercise the API calls. Verification is static, plus Task 8's live checklist.

- [ ] **Step 1: Comments.** Lines 6–9: "its commit" → "its squash-merge commit" (design §4.5). Replace lines 313–321 with the design §4.5 content. It covers: `main` is protected (required checks, admins enforced) and Forgejo applies required checks to pushes, so a direct push is rejected; the job opens `bot/image-bump-<7>` and schedules auto-merge, and `pr / pr-complete` gates it; it uses `FORGE_PUSH_TOKEN` because a PR opened by the run token triggers no `pull_request` workflow; loop prevention is still the `paths-ignore` above. Remove "the forge repository carries no such rule". Keep the 7-char tag paragraph (lines 323–327). Link `docs/operator/image-bump.md`.

- [ ] **Step 2: Checkout.** `ref: main`, `fetch-depth: 0`, `filter: blob:none`, `token: ${{ secrets.FORGE_PUSH_TOKEN }}`. Keep the existing `ref: main` comment. Keep the `Reset toolchain caches` step.

- [ ] **Step 3: `pin` step.** `set -euo pipefail`. Run `stale="$(tools/ci/stale-guard.sh)"`. If it's `stale=true`, write `skip=true` and exit 0. Then apply today's `sed` (line 365, unchanged) to the tip's `deployment.yaml`. If `git diff --quiet` → `already pinned to <tag>`, `skip=true`, exit 0. Then today's post-rewrite `grep -q` sanity check, then `skip=false`. Each `skip=` line is appended to `$GITHUB_OUTPUT`. No commit here.

- [ ] **Step 4: Shared API helper.** Use this exact shape. It is duplicated verbatim at the top of the `supersede` and `pr` steps (design §4.1, builder's call: stay inline unless a second function appears):
  ```sh
  # api METHOD PATH EXPECTED_CODES [JSON] — EXPECTED_CODES is comma-separated.
  # Sets API_CODE; body in "$resp". Unexpected code: print ≤300 bytes, exit 1.
  api() {
    local m="$1" p="$2" ok="$3" data="${4:-}"
    local args=(-sS -o "$resp" -w '%{http_code}' -X "$m"
                -H "Authorization: token ${FORGE_TOKEN}" -H 'Content-Type: application/json')
    [ -n "$data" ] && args+=(--data "$data")
    API_CODE="$(curl "${args[@]}" "${API}${p}")"
    echo "$m $p → HTTP $API_CODE"
    case ",$ok," in *",$API_CODE,"*) return 0 ;; esac
    echo "::error::$m $p returned HTTP $API_CODE"; head -c 300 "$resp"; echo; exit 1
  }
  ```
  Step env (both steps): `FORGE_TOKEN: ${{ secrets.FORGE_PUSH_TOKEN }}` and `API: ${{ github.server_url }}/api/v1/repos/${{ github.repository }}`. Inside each step, compute `BRANCH="bot/image-bump-${GITHUB_SHA::7}"` (shell, not `${{ github.sha }}`, which is the full SHA), `ENC_BRANCH="${BRANCH//\//%2F}"`, and `resp="$(mktemp)"`.

- [ ] **Step 5: `supersede` step** (`if: steps.pin.outputs.skip != 'true'`). Page `api GET "/pulls?state=open&limit=50&page=$n" 200` from `n=1` until a page has fewer than 50 items. Select with `jq -r --arg me "$BRANCH" '.[] | select((.head.ref | startswith("bot/image-bump-")) and .head.repo.full_name == .base.repo.full_name and .head.ref != $me) | "\(.number) \(.head.ref)"'`. For each: `api PATCH "/pulls/$n" 200,201 '{"state":"closed"}'`, then `api DELETE "/branches/<encoded ref>" 204,404`. Log `superseded #n (<ref>)`. If nothing matches, log `no older bump PRs`.

- [ ] **Step 6: `pr` step** (`id: pr`, `if: steps.pin.outputs.skip != 'true'`). Follow design §4.3 exactly:
  1. `api GET "/pulls/main/$ENC_BRANCH" 200,404`. On 200: if `.state == "open"`, write `number=<n>` and exit 0. Otherwise log `PR #n is <state>`, write no number, and exit 0.
  2. Run `git ls-remote --exit-code --heads origin "$BRANCH"` and capture the rc. 0 → `branch exists, reusing (no push)`. 2 → `git switch -c "$BRANCH"`, then `git config user.name/user.email` (bot identity), `git add deploy/kubernetes/deployment.yaml`, a commit with today's message (lines 377–381) and no `[skip ci]`, then `git push origin "$BRANCH"`. Any other rc → fail.
  3. `api POST /pulls 201` with the body built by `jq -n --arg …` (no hand-quoted JSON). It contains `head`, `base: "main"`, the title from Global Constraints, and a body naming `main.yml run ${GITHUB_RUN_ID}`, `${GITHUB_SHA}`, the tag, Argo CD `harbormaster-main`, and "auto-merges once `pr / pr-complete` passes". Write `number=$(jq -r .number "$resp")` to `$GITHUB_OUTPUT`.

- [ ] **Step 7: `automerge` step** (`if: steps.pr.outputs.number != ''`). This is atlas's retry loop: 12 attempts, `sleep 10` between them. `curl` the merge endpoint `POST "$API/pulls/$N/merge"` with the Global Constraints payload, `-o "$resp" -w '%{http_code}'`, and the auth header. On 2xx or 409, log `auto-merge scheduled (HTTP …)` and exit 0. On anything else, log `attempt i/12: HTTP <code>` + `head -c 300`. After 12 failures: `::error::could not schedule auto-merge for PR #N`, exit 1.

- [ ] **Step 8: Verify (static).**
  - `actionlint .forgejo/workflows/pr.yml .forgejo/workflows/main.yml` → only the baseline finding (shellcheck runs on the `run:` blocks too).
  - `grep -n 'FORGE_PUSH_TOKEN}@\|skip ci\|no such rule' .forgejo/workflows/main.yml` → no output.
  - `yq '.jobs."bump-manifest".concurrency' .forgejo/workflows/main.yml` → `group: bump-manifest`, `cancel-in-progress: false`.
  - `yq '.on.push."paths-ignore"' .forgejo/workflows/main.yml` → `deploy/kubernetes/deployment.yaml`.

- [ ] **Step 9: Commit.**
  ```bash
  git add .forgejo/workflows/main.yml
  git commit -m "fix(ci): bump-manifest pins the image through an auto-merged bot PR"
  ```

### Task 7: Operator doc and deploy README callout

**Files:**
- Create: `docs/operator/image-bump.md`
- Modify: `deploy/kubernetes/README.md:12-14`

**Interfaces:**
- Consumes: the job ids, step names, scripts and context string from Tasks 1–6.

- [ ] **Step 1: Write `docs/operator/image-bump.md`.** Use the six sections from design §7, in order: Flow; Bump-only classification (all **five** checks as implemented in Task 1, including the removed-line tightening, and why it fails closed); Required context; Token precondition (`write:repository`; a missing scope shows up as an HTTP 403 body in the `bump-manifest` log); Rollout order (design §6, steps 1–5, plus rollback); Stuck-PR recovery (three symptoms → fixes, plus the §4.4 residual race). Add a short "Trust boundary" note (Review Focus 1). In Required context, write it exactly as: ``Required context: `pr / pr-complete (pull_request)` — **unconfirmed**: replace this line with the string from this task's PR status list before merge.`` Use repo-relative paths and links only.

- [ ] **Step 2: README callout.** Replace `.github/workflows/main.yml` with `.forgejo/workflows/main.yml`, and change the text to say the job rewrites the tag "through an auto-merged bot PR (`bot/image-bump-<short-sha>`)". Add a link to `../../docs/operator/image-bump.md`.

- [ ] **Step 3: Verify.** `grep -rn '/home/' docs/operator/image-bump.md` → nothing. `grep -n '.github/workflows' deploy/kubernetes/README.md` → nothing. Every relative link resolves (`ls` each target).

- [ ] **Step 4: Commit.**
  ```bash
  git add docs/operator/image-bump.md deploy/kubernetes/README.md
  git commit -m "docs: image-bump runbook and corrected deploy README callout"
  ```

### Task 8: Full verification and operator hand-off

**Files:**
- Create: `docs/tasks/task-006-bot-pr-image-bump/rollout-checklist.md`

- [ ] **Step 1: Write `rollout-checklist.md`.** This is a checkbox list for the operator, ordered as follows:
  1. Confirm `FORGE_PUSH_TOKEN` scopes.
  2. Open the PR. From its status list, copy the exact `pr-complete` context string into `docs/operator/image-bump.md`, remove the "unconfirmed" wording, and commit to the same branch. Also confirm in the `changes` job log that `HEAD_REF` and `HEAD_REPO` are non-empty (design §2 unknowns).
  3. Merge.
  4. Confirm the first `bot/image-bump-*` PR opened with auto-merge set, its ten jobs skipped, and `pr-complete` green.
  5. Switch protection to the single context, keeping `apply_to_admins` on.
  6. Confirm the merge, the deleted branch, the pin on `main`, no new `main.yml` run, and the Argo sync.
  7. Re-run that `bump-manifest` job (no duplicate, no failure).
  8. Make two quick merges (the older bump PR is closed and its branch deleted).
  9. Open a human PR touching only `deployment.yaml` (full suite runs, `pr-complete` reflects it).

  Map each item to its PRD §10 criterion.

- [ ] **Step 2: Full gate.** Dispatch `task-verifier` with the plain flagless `tools/verify.sh` (don't run it in the implementer). Expected terminal line: `VERIFY: DONE — all gates passed; the branch may be called done.` and `ci scripts` in the transcript.

- [ ] **Step 3: Static workflow check.** `actionlint .forgejo/workflows/pr.yml .forgejo/workflows/main.yml` → only the baseline finding.

- [ ] **Step 4: Commit.**
  ```bash
  git add docs/tasks/task-006-bot-pr-image-bump/rollout-checklist.md
  git commit -m "docs(task-006): live rollout checklist for the operator"
  ```
