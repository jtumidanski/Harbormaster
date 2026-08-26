# Brief — Process Parity Phase 3 (Harbormaster)

**Read `docs/process-parity.md` first.** It is the canonical specification for this
work and was written to be self-contained: you do not need any prior conversation.
This brief only tells you which parts apply to Harbormaster and in what order.

## Provenance

`docs/process-parity.md` here is a verbatim copy pinned at atlas commit
`e75c2a168`, on the unmerged branch `task-266-process-parity-agent-rename`. Atlas
phase 1 is complete and gated green but not yet merged.

Set `ATLAS` to the path of the atlas worktree
`.worktrees/task-266-process-parity-agent-rename` on your machine. Before copying
anything, confirm the copy is still current:

```sh
diff "$ATLAS/docs/process-parity.md" docs/process-parity.md
```

If that diff is non-empty, the atlas PR changed the spec after this brief was
written. Stop and re-sync before proceeding — do not merge the two by hand.

## Your task

Execute `docs/process-parity.md` §6 step 2 for Harbormaster, using this repo's own
four-phase flow (`/spec-task` → `/design-task` → `/plan-task` → `/execute-task`).

Full parity is the scope: the portable hooks, the agent trio, the verify
entrypoint, the owner documents, `/fix-pr-bug`, the `service-documentation` agent
plus `/service-doc`, the `.claude/settings.json` hook wiring, and the `CLAUDE.md`
restructure.

**Do not skip `/design-task`.** Harbormaster is one of two repos where this is
genuine design work rather than porting, because `tools/verify.sh` has to be built
from a prose checklist and the `--quick` / `--no-docker` tiering has to be decided.

## What Harbormaster already has

Do not re-create these:

- the four phase commands, `/audit-plan`, `/review-todos`
- `backend-guidelines-reviewer`, `frontend-guidelines-reviewer`,
  `plan-adherence-reviewer`, `todo-scanner`
- `backend-dev-guidelines` / `frontend-dev-guidelines` skills, `skill-rules.json`,
  the `skill-activation-prompt` hook

## What Harbormaster is missing that MyFleet and atlas have

- `tools/task-numbers.sh` — port it from `$ATLAS/tools/`, together with its test
  `task-numbers_test.sh` and the `task-num-collision-detector.sh` SessionStart
  hook. Without it there is no collision-safe task numbering.
- `tools/task-brief.sh` — port from `$ATLAS/tools/`. `commit-boundary.sh`
  references it.
- `service-documentation` agent and `/service-doc` command.

## Harbormaster's binding row (`docs/process-parity.md` §4)

| Binding | Value |
|---|---|
| Verify entrypoint | **create** `tools/verify.sh` |
| Backend gates (cwd `apps/backend`) | `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build ./...` |
| Frontend gates (cwd `apps/frontend`) | `npm ci`, `npm run lint`, `npm run format`, `npm test`, `npm run build` |
| Container gate (cwd repo root) | `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .` |
| `--no-docker` | skips the buildx step |
| Excluded from the flagless run | `HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...` and `npm run test:e2e` — both on-demand, not per-PR |
| Go layout | no `go.work`; single module at `apps/backend` |
| Frontend path | `apps/frontend` |

The gate list above is lifted from the current `CLAUDE.md` "Build & Verification"
section. Moving it into `tools/verify.sh` is the point: the prose checklist cannot
be run by a verifier agent, and a ten-command checklist is exactly the kind of
thing that gets partially run and then reported as green.

`tools/verify.sh` must match atlas's contract: **flagless run exits 0 means the
branch may be called done**; `--quick` / `--no-docker` also exit 0 but skip the
slow gates and do NOT count as done. Decide during `/design-task` which gates
`--quick` drops — at minimum the buildx step and `-race`.

## Check this before you start

`CLAUDE.md` currently states the repository "is currently unscaffolded — only
`README.md` exists" and asks that the file be updated once service layout is
settled. That is now stale: `apps/backend` and `apps/frontend` both exist. Confirm
the current state and correct that section as part of the `CLAUDE.md` restructure.
Do not carry the stale claim forward into the new rule-list shape.

## Copying the portable files

From `$ATLAS`, copy verbatim into `.claude/hooks/`:

`wait-loop-guard.sh`, `wait-loop-guard_test.sh`, `block-home-paths-in-docs.sh`,
`turn-budget.sh`, `turn-budget-guard.sh`, `fork-dispatch-guard.sh`,
`commit-boundary.sh`, `task-num-collision-detector.sh`

These contain no atlas-specific strings as of `e75c2a168`. Verify after copying:

```sh
grep -l 'atlas-' .claude/hooks/*.sh   # must print nothing
```

`format-on-write.sh` must NOT be copied verbatim. Atlas's version hardcodes
`services/atlas-ui` for prettier and sources `tools/toolchain.versions`. Rebind it
to `apps/frontend`, and to whatever pins `golangci-lint` here — if nothing does,
that is a decision for `/design-task`, not something to leave dangling.

The agent trio (`task-implementer`, `task-verifier`, `task-reviewer`) and the owner
documents copy from `$ATLAS/.claude/agents/` and `$ATLAS/docs/`. The owner docs
need the §5.2 genericization pass — replace atlas-specific examples (packet work,
WZ data, IDA) with Harbormaster equivalents or neutral ones. **Do not delete a rule
because its example does not transfer; find a new example.**

Do not port: anything under `docs/packets/`, `docs/reverse-engineering.md`.

## The one carve-out that differs from atlas

`docs/process-parity.md` §7 check 3 exempts two files. In Harbormaster **only
`docs/process-parity.md` is exempt** — the `docs/agent-dispatch.md` exemption is
atlas-only, because atlas is the only repo that ever used the `atlas-*` names.
Confirmed 2026-08-26: this repo has zero such references today. Your check is:

```sh
git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'
```

It must print nothing.

## Done

`docs/process-parity.md` §7 lists the six checks. Checks 1, 4, 5, and 6 are
cross-repo and cannot be fully evaluated from Harbormaster alone — report your side
of them and say plainly that the pairwise comparison is not evaluable here. Checks
2 and 3 are fully checkable in this repo and must pass.

Report back what you could not verify rather than asserting it.
