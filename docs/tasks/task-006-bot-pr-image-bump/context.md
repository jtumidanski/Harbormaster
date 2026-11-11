# task-006-bot-pr-image-bump — Context

Companion to [`plan.md`](plan.md). Spec: [`design.md`](design.md), PRD: [`prd.md`](prd.md).

## Problem in one paragraph

`main` has required status checks enforced for admins, and Forgejo applies them
to direct pushes. `bump-manifest` (`.forgejo/workflows/main.yml:328-382`) pushes
the image pin straight to `main`, so every push since 2026-10-23 has been rejected
and nothing has deployed since `sha-780b019`. The fix routes the pin through a bot
PR with auto-merge (atlas's pattern), and adds a single aggregate context,
`pr-complete`, to `pr.yml`.

## Key files

| File | Today | After |
|---|---|---|
| `.forgejo/workflows/main.yml` | `bump-manifest` does the stale check `tip == GITHUB_SHA`, runs `sed`, and pushes with the token in the URL (line 382). The comment at 317–321 falsely says there's no protection rule. | Checkout with `FORGE_PUSH_TOKEN`; steps `pin` → `supersede` → `pr` → `automerge`. |
| `.forgejo/workflows/pr.yml` | Ten independent jobs; `paths-ignore: deployment.yaml` (lines 5–13). | Adds a `changes` job; the ten jobs get `needs: [changes]` plus an `if:`; adds `pr-complete` (`if: always()`). |
| `tools/ci/` | does not exist | `classify-pr.sh`, `pr-complete.sh`, `stale-guard.sh` plus a `*_test.sh` for each |
| `tools/verify.sh` | 11 gates; no shell-test gate | 12 gates; `ci scripts` sits after `npm run build` and before `docker buildx` |
| `tools/verify_test.sh` | asserts 11 gates (flagless) and 10 (`--no-docker`) | 12 and 11 |
| `docs/verification.md` | says "eleven" at lines 81, 93 and 376; drift table | "twelve"; the script-only paragraph mentions `ci scripts` |
| `deploy/kubernetes/README.md:12-14` | points at `.github/workflows/main.yml` | `.forgejo/…`, says "bot PR", links the runbook |
| `deploy/kubernetes/deployment.yaml:61` | `          image: registry.tumidanski/jtumidanski/harbormaster:sha-780b019` (the only `image:` line) | rewritten by the bot only |
| `docs/operator/image-bump.md` | — | the new runbook |

## Decisions (where the plan or design departs from the PRD)

1. **Stale guard** = "`GITHUB_SHA` is on `main` and nothing after it changed any
   path except `deployment.yaml`", not "tip == SHA" (design §4.2). Bump merges
   move the tip now.
2. **Supersede runs before our branch is pushed** (design §4.4). This narrows the
   window in which two bump PRs conflict.
3. **Bump-only needs all five of these:** the bot prefix, the same repo, exactly
   one changed file (`--no-renames`), exactly one `-` line and one `+` line, an
   added line shaped like a pin, **and** a removed line that is a harbormaster
   image line with identical indentation. That last condition is the plan's own
   tightening of design §5.2 condition 4.
4. **Change detection uses git diff against the merge base**
   (`origin/$BASE_REF...HEAD`), not the files API.
5. **Logic lives in scripts with fixed stdout contracts:**
   `classify-pr.sh` → `bump-only=true|false`; `stale-guard.sh` →
   `stale=true|false` (exit 2 on error); `pr-complete.sh` → exit 0/1, with
   `--list-jobs` printing its `SUITE_JOBS` array.
6. **Result variables:** `RESULT_<JOB_ID_UPPER_UNDERSCORED>`, plus
   `CHANGES_RESULT` and `BUMP_ONLY`.
7. **A `yq` agreement test** (in `pr-complete_test.sh`) keeps `pr.yml`'s job
   list, `pr-complete.needs`, its env keys and the script's array in lockstep.
8. **`ci scripts` is a script-only gate** (CI runs the scripts but not their
   tests), and the drift table in `docs/verification.md` records that.
9. **The API helper is inline and duplicated** in `supersede` and `pr`, with
   the signature `api METHOD PATH EXPECTED_CODES [JSON]`. Auto-merge uses its
   own retry loop.

## Verified facts (from design §2)

The Forgejo API paths, the merge payload fields and the response codes were
checked against `swagger.v1.json` on the live forge (`17.0.1+gitea-1.22.0`).
The `head` query parameter on the list-PRs call is an exact match, so the prefix
filter runs client-side with `jq`. The branch name in a URL needs `/`→`%2F`.

## Still unverified (resolved during rollout, not by implementation)

- `FORGE_PUSH_TOKEN` scopes (`write:repository`).
- The exact `pr-complete` context string. It's expected to be
  `pr / pr-complete (pull_request)`, and the operator doc marks it "unconfirmed"
  until the PR run.
- Whether `github.head_ref` and `pull_request.head.repo.full_name` are populated
  on Forgejo. If either is empty, the classifier says `false` (fails closed).
- Whether Forgejo treats a skipped required context as passing. Nothing relies
  on it.

## Local tooling

`actionlint` 1.7.7, `shellcheck`, `yq` v4.44.3 (mikefarah), `jq`. The
`actionlint` baseline on the two workflows is one pre-existing finding:
`main.yml:208`, the Forgejo URL-form `uses:`.

## Dependencies between tasks

Tasks 1, 2 and 3 are independent. Task 4 needs 1–3 to exist, because its gate
fails if no tests are found. Task 5 needs Tasks 1 and 2. Task 6 needs Task 3.
Task 7 needs 1–6 for names. Task 8 comes last.
