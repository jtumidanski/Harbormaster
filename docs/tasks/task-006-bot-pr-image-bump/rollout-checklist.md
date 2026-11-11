# task-006 live rollout checklist

Operator checklist. Each item names the PRD section 10 criterion it confirms
(AC = the acceptance-criteria bullets in order: AC1 bump PR opened with
auto-merge; AC2 ten jobs skipped, aggregate passes; AC3 protection with the
single context, bump PR merges, branch deleted; AC4 pin on `main`, no new
`main.yml` run; AC5 re-run is idempotent; AC6 older PRs superseded; AC7 stale
run exits 0; AC8 human PR runs the full suite; AC9 bot-prefixed branch with
other files runs the full suite; AC10 comment removed and operator doc
complete; AC11 `VERIFY: DONE`).

Several behaviours below could not be verified without a live Forgejo. Each is
a checkbox under the step where it is observed. Record the answer, and where
noted, update `docs/operator/image-bump.md`.

## 1. Token scope (precondition for AC1, AC3)

- [ ] `FORGE_PUSH_TOKEN` exists as a repository secret and belongs to a user
      allowed to push branches and to merge past protection.
- [x] `FORGE_PUSH_TOKEN` carries the `write:repository` scope (confirmed by
      the operator, 2026-11-11).
- [ ] Scope check: `write:repository` alone is sufficient for every call
      (create PR, list/patch PRs, merge, delete branch, push). If any call
      returns 403 in steps 4, 7 or 8, record which call and which extra scope
      it needed, and update section 4 of `docs/operator/image-bump.md`.
- [ ] Page size: the server honours `limit=50` on the list-PRs call, i.e.
      `MAX_RESPONSE_ITEMS` is at least 50 (check the instance `app.ini`
      `[api]` section, or count items in a response when more than the
      default page of PRs exist). If it is lower, `supersede` stops after page
      1 and must be changed to page by the server's cap.

## 2. Open the PR (AC10)

- [ ] Copy the exact `pr-complete` context string from the PR status list
      into `docs/operator/image-bump.md` (line "Required context"), remove the
      "unconfirmed" wording, and commit to the same branch.
- [ ] In the `changes` job log, `HEAD_REF` and `HEAD_REPO` are non-empty
      (design section 2 unknowns).

## 3. Merge (AC10)

- [ ] Merge the PR.

## 4. First bump PR (AC1, AC2)

- [ ] The first `bot/image-bump-*` PR opened, changing only `deployment.yaml`,
      with auto-merge (squash, delete branch) set.
- [ ] Its ten suite jobs are skipped and `pr-complete` is green.
- [ ] Push credential: `git ls-remote` and `git push` in `bump-manifest`
      authenticated with the credential `actions/checkout` persisted from
      `FORGE_PUSH_TOKEN` (no auth failure in the log).
- [ ] Re-run `bump-manifest` once while this first bump PR is still open and
      waiting on checks (it must happen before the PR merges). Confirm:
      - the `pr` step logs `PR #N is open, reusing it` (the `echo` at
        `.forgejo/workflows/main.yml:470`). The lookup only returns 200 when
        Forgejo decodes the `%2F`-encoded branch name, so this proves `%2F`
        decoding and that no duplicate PR was created. A first run cannot show
        it, because the lookup returns 404 when no PR exists yet.
      - the `automerge` step logs `auto-merge scheduled (HTTP <code>)`.
      - record that `<code>`: the status the merge endpoint returns when
        auto-merge is already scheduled (expected 409 or 2xx; the script
        treats both as success).
      If the PR merges before the re-run, record "not observed" and retry on a
      later bump.

## 5. Branch protection (AC3)

- [ ] Replace the required contexts with the single `pr-complete` context,
      keeping `apply_to_admins` on.

## 6. Auto-merge outcome (AC3, AC4)

- [ ] The bump PR merged by itself and its branch was deleted.
- [ ] `deployment.yaml` on `main` names `sha-<short-sha>` of the build.
- [ ] The merge commit started no new `main.yml` run.
- [ ] Argo synced the new pin.

## 7. Re-run `bump-manifest` (AC5)

- [ ] Re-running the same job fails nothing and opens no duplicate PR.
- [ ] By now the PR has merged (item 6) and `main` carries the pin, so the
      re-run checks out `main` and the `pin` step logs
      `already pinned to sha-<7> — nothing to bump`
      (`.forgejo/workflows/main.yml:382`) and sets `skip=true`. The job is
      green and no new PR is opened; the `pr` step never runs. This does not
      exercise the PR lookup: `%2F` decoding is proven by item 4's open-PR
      re-run.
- [ ] Closed PRs: record whether `GET /pulls/main/<branch>` returns an
      already-closed PR. Hand-close a bump PR, re-run its `bump-manifest`, and
      note whether it is a no-op or opens a new PR. Then replace each
      "unverified forge behaviour" statement in `docs/operator/image-bump.md`
      (failure table rows 1, 2 and 3, and the residual-race paragraph) with the
      observed result.

## 8. Two quick merges (AC6, AC7)

- [ ] Two pushes to `main` in quick succession: the older bump PR is closed
      and its branch deleted; only the newer remains.
- [ ] `%2F`: `DELETE /branches/<enc-branch>` deleted the older branch (not a
      404 swallowed as "accepted"). Confirm in the log.
- [ ] The stale run (main moved on) exits 0 without opening a PR.

## 9. Human PR (AC8, AC9)

- [ ] A human PR touching only `deployment.yaml` runs all ten jobs and
      `pr-complete` reflects the result.
- [ ] A `bot/image-bump-*` branch that also changes another file runs the full
      suite.

## 10. Repository gate (AC11)

- [ ] `tools/verify.sh` prints `VERIFY: DONE` (run by the verifier, not the
      operator).
