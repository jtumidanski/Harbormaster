# Review — task-006, Task 6: `bump-manifest` through a bot PR

verdict: APPROVED_WITH_FINDINGS
Spec compliance: ✅
Task quality: Approved with three Minor findings (none blocking).

Range: `e2ca346..6939fbe` (one commit, `6939fbe`). It touches only
`.forgejo/workflows/main.yml` (+192/-24), which matches the brief's file list.
Scope confirmed.

Inputs read: `.superpowers/sdd/plan/task-6-brief.md`, `task-6-report.md`, the
commit diff, design §2 and §4.1–§4.5, and `tools/ci/stale-guard.sh`. The stale
guard is included because the `pin` step depends on its stdout and exit-code
contract.

## Static checks re-run by the reviewer

- `actionlint .forgejo/workflows/pr.yml .forgejo/workflows/main.yml` returns one
  finding, `main.yml:209:15` (select-arm64-builder "invalid format"). This is the
  accepted baseline finding.
- `grep -n 'skip ci\|FORGE_PUSH_TOKEN}@\|no such rule\|set -x' main.yml` returns
  no output.
- Each `run:` block of `pin`, `supersede`, `pr` and `automerge` was extracted
  with yq and run through `shellcheck -s bash`. All four are clean.
- The extracted commit message has its continuation lines at column 0, so the
  block-scalar indent does not leak into it.
- `yq` reports the following, unchanged from before the commit:
  - `paths-ignore` = `deploy/kubernetes/deployment.yaml`
  - `concurrency` = `{group: bump-manifest, cancel-in-progress: false}`
  - `needs` = `[manifest]`
  - `if` = `github.ref == 'refs/heads/main'`

## Requirement by requirement

| Brief step | Result | Evidence |
|---|---|---|
| 1 Comments | PASS | `main.yml:6-8` now reads "its squash-merge commit". `main.yml:314-333`: the header states that main is protected, that required checks apply to pushes and a direct push is rejected, the `bot/image-bump-<7>` branch, auto-merge gated by `pr / pr-complete`, why FORGE_PUSH_TOKEN is used, that loop prevention is still paths-ignore, and links `docs/operator/image-bump.md`. That runbook is created by a later task (plan.md:55, :326). "no such rule" is removed. The 7-char tag paragraph is kept. |
| 2 Checkout | PASS | `main.yml:346-356` sets `ref: main`, `fetch-depth: 0`, `filter: blob:none` and `token: ${{ secrets.FORGE_PUSH_TOKEN }}`. The `ref: main` comment is kept, and `Reset toolchain caches` is unchanged. |
| 3 `pin` | PASS | `main.yml:359-388`: `set -euo pipefail`, then `stale="$(tools/ci/stale-guard.sh)"` (:369). `stale=true` writes `skip=true` and exits 0. The sed is unchanged (:379). `git diff --quiet` writes "already pinned" and `skip=true` (:381-385). Then the grep check (:387) and `skip=false`. Nothing is committed in this step. |
| 4 `api()` helper | PASS | Copied verbatim into `supersede` and `pr`. The env is `FORGE_TOKEN` + `API`. `BRANCH` is built from `${GITHUB_SHA::7}`, `resp` from `mktemp`. `ENC_BRANCH` is defined only in `pr`, which is the only step that uses it. |
| 5 `supersede` | PASS (deviation 1, approved) | `main.yml:389-435`. It pages `limit=50` until a page has fewer than 50 items, using the exact jq selector (:422). It sends `PATCH` with `200,201` and `{"state":"closed"}` (:430), then `DELETE /branches/${ref//\//%2F}` with `204,404` (:431). It logs `superseded #n (ref)` or `no older bump PRs`. |
| 6 `pr` | PASS (deviation 2, approved) | `main.yml:436-520`. It sends `GET /pulls/main/$ENC_BRANCH` with `200,404` (:465). On 200 it either writes `number` (open) or logs `PR #n is <state>`, then exits 0. It captures the `ls-remote --exit-code` rc with `\|\| rc=$?` (:481): 0 reuses the branch, 2 runs switch, the bot identity, add, commit and push, and any other rc fails. Commit message: no `[skip ci]`. The PR payload is built with `jq -n --arg` (:512-516). `POST /pulls` expects 201. `number` comes from `jq -r .number`. |
| 7 `automerge` | PASS | `main.yml:521-550`. `if: steps.pr.outputs.number != ''`. `N` comes in through env. The exact payload is used. 12 attempts with `sleep 10` between them. `2??\|409` counts as success. Every other code logs `attempt i/12` and `head -c 300`. The final failure prints `::error::` and exits 1. |
| Global: title / author / branch | PASS | Title `chore(deploy): pin harbormaster image to ${short}`, where `short=sha-${GITHUB_SHA::7}`. Author `harbormaster-image-bot <harbormaster-image-bot@git.tumidanski>`. |
| Global: token | PASS | All three API steps use `FORGE_TOKEN: ${{ secrets.FORGE_PUSH_TOKEN }}` and the `Authorization: token ${FORGE_TOKEN}` header. The push is a bare `git push origin "$BRANCH"` (:498) on credentials that checkout persisted from FORGE_PUSH_TOKEN. The old URL-embedded push was removed. There is no `set -x`, and no `${{ }}` inside any `run:` block. |
| Global: never force-push | PASS | No `--force` or `+refspec` anywhere in the job. |

## Focus areas

- **Token leakage.** Clean. No `set -x`. curl gets the header through an argv
  array, and nothing echoes it. The URL is `${API}${p}`, which carries no
  credential. `curl -sS` error text does not include headers. The push uses the
  persisted credential.
- **Fail-closed behaviour.**
  - A stale-guard exit 2 propagates. A failing command substitution in a plain
    assignment fails the step under `set -e` (`main.yml:369`).
  - An unexpected HTTP code in `api()` prints `::error::` and the body, then
    runs `exit 1` (:408, :455).
  - A curl transport error inside `API_CODE="$(curl …)"` also fails the step,
    through `set -e`.
  - On the last attempt, `[ "$i" -lt 12 ] && sleep 10` returns 1 inside an
    `&&` list, which `set -e` ignores. Control then reaches `::error::` and
    `exit 1` as intended.
- **Idempotence.**
  - PR open: the step reuses it and writes `number`, and the 409 from automerge
    counts as success.
  - PR closed or merged: the step exits 0 with no number, so automerge is
    skipped. A merged re-run normally stops even earlier, at "already pinned".
  - Branch exists with no PR: no push, then `POST /pulls`.
  - `supersede` excludes `$BRANCH`, so a re-run never closes its own PR.
- **`pin` working tree vs. switch/reuse.** `git switch -c` carries the unstaged
  `deployment.yaml` change onto the new branch, and `git add` and commit pick
  it up. On the reuse path the dirty working tree is simply discarded, and the
  remote branch already holds the earlier attempt's commit. Correct.
- **Step `if:` conditions.** `supersede` and `pr` have
  `if: steps.pin.outputs.skip != 'true'`. `skip` is always written on every
  successful pin path, and if pin fails the implicit `success()` stops the later
  steps. `automerge` keys on a non-empty `number`. Correct.
- **jq-built JSON.** The PR payload uses `jq -n --arg`. The two remaining JSON
  literals are constants with no interpolation: `{"state":"closed"}`, which the
  brief mandates, and the auto-merge payload, which Global Constraints gives
  verbatim. Acceptable.

## Deviations

1. **`supersede` collects all pages before closing anything.** Approved. With
   `state=open`, closing PRs while paging removes them from the result set, so
   later pages shift and some matches would be skipped. Collecting first is the
   correct reading of the design intent. Empty jq output only adds blank lines,
   and `[ -n "$num" ] || continue` (:429) skips them.
2. **A merged PR is logged as "merged".** Approved. This is display only
   (:468). The control flow is the same as for any non-open state: exit 0, no
   `number`.

## Findings (non-blocking)

- **Minor — `main.yml:370`.** Only `stale=true` is tested. Any other stdout from
  the guard falls through as "not stale". `stale-guard.sh` guarantees exactly
  one line, `stale=true` or `stale=false`, so this cannot be reached today. A
  `case` with a `*) ::error:: …; exit 1` arm would make the step fail closed if
  that contract ever drifts.
- **Minor — `main.yml:536-538`.** `code="$(curl …)"` runs under `set -e`. A curl
  transport failure (non-zero exit, `000`) therefore aborts the step on attempt
  1 instead of using the retry loop. The failure is loud, so this is not a
  safety issue, but a transient connection blip costs a manual re-run.
  `|| true` on the curl call would keep it inside the retry loop.
- **Minor, pre-existing and in the brief-mandated order — `main.yml:381-387`.**
  If the `sed` matches nothing (for example, the image line was renamed),
  `git diff --quiet` is true. The step then reports "already pinned" and
  `skip=true`, the job goes green, and the `grep` sanity check never runs. Before
  this commit the order was the same (`e2ca346:main.yml:367,372`), and brief
  Step 3 prescribes it. This is not a regression from this task. A follow-up
  could run the grep before the "already pinned" exit.

## Not evaluable

These cannot be checked statically. Task 8's live checklist covers them.

1. Whether `POST /pulls/{n}/merge` returns 409 for an auto-merge that is already
   scheduled on Forgejo 17.0.1. The design asserts it and the report flags it as
   unverified.
2. Whether `%2F` in `GET /pulls/main/{head}` and `DELETE /branches/{branch}` is
   decoded as the design §2 states.
3. Whether `actions/checkout@v7.0.1`'s persisted FORGE_PUSH_TOKEN credential is
   honoured by `git ls-remote` and `git push` on the Forgejo runner.
4. Whether the server's maximum page size is at least 50. If it is lower, a full
   page looks short and paging stops after page 1.
