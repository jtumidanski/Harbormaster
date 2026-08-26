# Task 15: Final assertion sweep and parity report

**AC-20 / AC-21 blocking check, run first:** AC-20's grep printed nothing (empty
after excluding `docs/process-parity.md`, exit 1 as expected). AC-21's `git diff
--name-only main...HEAD | grep -c '^apps/'` printed `0`. Neither is a blocking
failure. The sweep below proceeded.

## 1. Mechanical results

### AC-1 — no `atlas-` in hooks

```
$ grep -l 'atlas-' .claude/hooks/*.sh; echo "exit: $?"
exit: 1
```
**PASS** (no filenames, exit 1).

### AC-2 — wait-loop-guard test

```
$ ./.claude/hooks/wait-loop-guard_test.sh; echo "exit: $?"
== denied: no-op turns ==
== denied: sleeping to wait ==
== denied: broad process listing as a wait ==
== allowed: legitimate process debugging ==
== allowed: ordinary work ==
== allowed: explicitly justified ==

passed: 33  failed: 0
exit: 0
```
**PASS.**

### AC-3 — format-on-write rebound

```
$ grep -n 'apps/frontend\|toolchain.versions\|apps/backend/.golangci.yml' .claude/hooks/format-on-write.sh
28:        # shellcheck source=../../tools/toolchain.versions
29:        source "$ROOT/tools/toolchain.versions" 2>/dev/null || exit 0
38:        (cd "$moddir" && "$GOLANGCI" fmt -c "$ROOT/apps/backend/.golangci.yml" "$fp") >/dev/null 2>&1 || true
40:    */apps/frontend/*.ts|*/apps/frontend/*.tsx)
41:        (cd "$ROOT/apps/frontend" && npx --no-install prettier --write "$fp") >/dev/null 2>&1 || true
```
**PASS** — rebind targets present.

### AC-4 — pin matches CI

```
$ grep GOLANGCI_LINT_VERSION tools/toolchain.versions
GOLANGCI_LINT_VERSION=v2.12.2

$ grep -n -A2 'golangci-lint-action' .github/workflows/pr.yml
48:      - uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0
49-        with: { version: v2.12.2, working-directory: apps/backend }
50-
```
Pinned `v2.12.2` matches CI's `version: v2.12.2`. **PASS.**

### AC-5 — the three scripts exist and are executable

```
$ test -x tools/verify.sh && echo "ok   tools/verify.sh"
ok   tools/verify.sh
$ test -x tools/task-numbers.sh && echo "ok   tools/task-numbers.sh"
ok   tools/task-numbers.sh
$ test -x tools/task-brief.sh && echo "ok   tools/task-brief.sh"
ok   tools/task-brief.sh
```
**PASS.**

### AC-6 — task-numbers test

```
$ ./tools/task-numbers_test.sh; echo "exit: $?"
ok   - history number 002 reported as used
ok   - present number 001 reported as used
ok   - present number 004 reported as used
ok   - next returns true smallest free gap
ok   - check is clean (no false history collision)
ok   - next survives a >64KB used-set (no SIGPIPE short-circuit)
all task-numbers.sh tests passed
exit: 0
```
**PASS.**

### AC-12 — agents present, the four pre-existing untouched

```
$ ls .claude/agents/
backend-guidelines-reviewer.md  9.8K
frontend-guidelines-reviewer.md  7.4K
plan-adherence-reviewer.md  4.6K
service-documentation.md  3.2K
task-implementer.md  11.7K
task-reviewer.md  5.8K
task-verifier.md  3.9K
todo-scanner.md  3.9K

$ git diff --stat main...HEAD -- .claude/agents/backend-guidelines-reviewer.md \
    .claude/agents/frontend-guidelines-reviewer.md \
    .claude/agents/plan-adherence-reviewer.md \
    .claude/agents/todo-scanner.md
(empty)
```
Empty diff == byte-unchanged. **PASS.**

### AC-13 — commands present

```
$ ls .claude/commands/fix-pr-bug.md .claude/commands/service-doc.md
.claude/commands/fix-pr-bug.md  4.7K
.claude/commands/service-doc.md  932B
```
**PASS.**

### AC-14 — settings

```
$ jq -e '.disableBundledSkills == true' .claude/settings.json
true

$ jq -r '.hooks.PreToolUse[].matcher' .claude/settings.json
Write|Edit
Agent
Bash
*

$ jq -r '.hooks.PostToolUse[].matcher' .claude/settings.json
Write|Edit
*
Bash

$ jq -r '.hooks | keys[]' .claude/settings.json
PostToolUse
PreToolUse
SessionStart
UserPromptSubmit

$ jq -e '.enabledPlugins["superpowers@claude-plugins-official"] == true' .claude/settings.json
true
```
Matches Task 6's evidence. **PASS.**

### AC-15 — ten owner documents

```
ok   docs/agent-dispatch.md
ok   docs/verification.md
ok   docs/superpowers-integration.md
ok   docs/review-protocol.md
ok   docs/post-implementation.md
ok   docs/codemod-vs-agents.md
ok   docs/slice-first.md
ok   docs/tooling-conventions.md
ok   docs/git-workflow.md
ok   docs/observability.md
```
**PASS.**

### AC-16 — every docs/ link resolves

```
$ grep -ohE '\]\((\.\./)*docs/[A-Za-z0-9._/-]+\)' CLAUDE.md docs/*.md \
  | sed -E 's/^\]\(//; s/\)$//; s|^(\.\./)+||' | sort -u
docs/agent-dispatch.md
docs/codemod-vs-agents.md
docs/git-workflow.md
docs/observability.md
docs/post-implementation.md
docs/review-protocol.md
docs/slice-first.md
docs/superpowers-integration.md
docs/tooling-conventions.md
docs/verification.md
```
All ten resolve to files that exist (confirmed above). No `MISSING` lines.
**PASS.**

### AC-18 / AC-19 — CLAUDE.md shape

```
$ grep -n '^#\{1,2\} ' CLAUDE.md
1:# Harbormaster
16:## Never do this
30:## Evidence & grounding
39:## Development workflow
66:## Done means verified
86:## Dispatching agents
103:## Handing off context
113:## Repository conventions
128:## Where the procedures live

$ grep -niE 'unscaffolded|only .README\.md. exists' CLAUDE.md; echo "exit: $?"
exit: 1
```
Eight `##` headings present in order (Never do this / Evidence & grounding /
Development workflow / Done means verified / Dispatching agents / Handing off
context / Repository conventions / Where the procedures live). AC-19 exit 1
(no leftover "unscaffolded" boilerplate). **Both PASS.**

### AC-20 — process-parity check 3

```
$ git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'; echo "exit: $?"
exit: 1
```
(Raw `git grep` matched only `docs/process-parity.md`, which the second grep
excludes; the final result is empty, exit 1.) **PASS. Not blocking.**

### AC-21 — no app changes

```
$ git diff --name-only main...HEAD | grep -c '^apps/'
0
```
**PASS. Not blocking.**

### AC-22 — no home/absolute paths under docs/

```
$ grep -rnE '/home/|/Users/|(^|[^A-Za-z0-9_.])~/' docs/ --include='*.md'; echo "exit: $?"
exit: 0
```

This criterion, worded exactly this way, **cannot pass on this repo** — the
grep matches 20 lines, all pre-existing and none introduced to launder the
result. Full output:

```
docs/tooling-conventions.md:99:literal home or absolute paths like `/Users/<name>/...` or
docs/tooling-conventions.md:100:`/home/<name>/...` — a committed absolute path is not reproducible on
docs/tasks/task-001-harbormaster-mvp-v1/audit.md:32:...commented-out `~/.mc/config.json:ro` mount...
docs/tasks/task-001-harbormaster-mvp-v1/prd.md:121:...Operators bind-mount their host `~/.mc/config.json` here...
docs/tasks/task-001-harbormaster-mvp-v1/prd.md:142:...operators bind-mount their host `~/.mc/config.json` to enable this...
docs/tasks/task-001-harbormaster-mvp-v1/prd.md:546:...the host's `~/.mc/config.json`...
docs/tasks/task-004-process-parity-harness/context.md:12:| This worktree | `/home/tumidanski/source/Harbormaster/.worktrees/task-004-process-parity-harness` |
docs/tasks/task-004-process-parity-harness/context.md:14:| Main repo | `/home/tumidanski/source/Harbormaster` |
docs/tasks/task-004-process-parity-harness/context.md:15:| **`$ATLAS`** — the source worktree, read-only | `/home/tumidanski/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename` |
docs/tasks/task-004-process-parity-harness/plan.md:16:  `/home/tumidanski/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename`.
docs/tasks/task-004-process-parity-harness/plan.md:21:  `/home/tumidanski/source/Harbormaster/.worktrees/task-004-process-parity-harness`
docs/tasks/task-004-process-parity-harness/plan.md:1053:cd <worktree> && printf ... "see /home/someone/x"} ...
docs/tasks/task-004-process-parity-harness/plan.md:1827:cd <worktree> && grep -rnE '/home/|/Users/|~/' docs/*.md
docs/tasks/task-004-process-parity-harness/plan.md:2545:cd <worktree> && grep -n '/Users/\|/home/' .claude/commands/spec-task.md
docs/tasks/task-004-process-parity-harness/plan.md:2644:cd <worktree> && grep -n '/Users/\|/home/' .claude/commands/*.md; echo "exit: $?"
docs/tasks/task-004-process-parity-harness/plan.md:2752:grep -rnE '/home/|/Users/|(^|[^A-Za-z0-9_.])~/' docs/ --include='*.md'; echo "exit: $?"
docs/tasks/task-001-harbormaster-mvp-v1/plan.md:17:- Paths are relative to the worktree root: `/Users/tumidanski/source/Harbormaster/.worktrees/task-001-harbormaster-mvp-v1/`.
docs/operator/security.md:101:The first-run setup wizard can read your host `~/.mc/config.json` to
docs/operator/security.md:102:pre-fill the MinIO connection form. **The secret key in `~/.mc/config.json`
docs/operator/configuration.md:42:| `HARBORMASTER_MC_CONFIG_PATH` ... Bind-mount your host `~/.mc/config.json` here to opt in. |
```

Classification of every hit:

**(a) `~/.mc/config.json`** in `docs/operator/security.md`,
`docs/operator/configuration.md`, and `docs/tasks/task-001-.../{prd,audit}.md`
— legitimate operator documentation of a real host path an operator
bind-mounts. `.claude/hooks/block-home-paths-in-docs.sh` (the enforcing hook,
per `.superpowers/sdd/plan/task-5-report.md`) does not police `~/` at all —
its refusal fires only on `/home/<user>/...`. Not a hook-relevant match.

**(b) `/Users/tumidanski/...`** in `docs/tasks/task-001-.../plan.md:17` —
historical, already on `main` before this branch existed. Out of this task's
scope to touch, and not introduced here.

**(c) `/home/tumidanski/...`** in this task's own `plan.md` and `context.md`
— committed before `block-home-paths-in-docs.sh` existed on this branch, and
the implementers' only pointer to `$ATLAS`, the source worktree used to port
files. Editing these to satisfy this grep would erase the provenance record
plan.md/context.md exist to preserve, and the task brief for this step
explicitly forbids editing them "to make the grep quieter."

**Expected hit in a file this branch added:** `docs/tooling-conventions.md:99-100`
quotes the forbidden path SHAPE — `/home/<name>/...` — inside the rule that
*forbids* that shape. The hook's real regex is
`/home/[A-Za-z0-9_.-]+/` (from `.claude/hooks/block-home-paths-in-docs.sh`);
`<` and `>` fall outside that character class, so `/home/<name>/` cannot
match the hook's actual regex. Nothing leaked — this is documentation
correctly describing the rule it enforces, using a placeholder token the
enforcement regex itself would reject as a real path.

**Verdict: AC-22 as literally worded fails on this repo, for reasons
independent of this branch (pre-existing operator docs, pre-existing
cross-repo historical paths, and this branch's own required provenance
record). No file was edited to quiet this grep.**

## 2. `verify.sh` re-run (Step 2)

### `verify_test.sh`

```
$ ./tools/verify_test.sh
ok   - flagless selects 11 gates
ok   - flagless includes docker buildx
ok   - flagless names the race-enabled test gate
ok   - flagless skips nothing
ok   - --no-docker selects 10 gates
ok   - --no-docker skips exactly one gate
ok   - --no-docker skips docker buildx
ok   - --quick skips docker buildx
ok   - --quick names the un-raced test gate
ok   - --quick does not name a race-enabled test gate
ok   - --quick --no-docker skips docker buildx
ok   - --quick --no-docker does not name a race-enabled test gate
ok   - --help exits 0
ok   - --help names the integration suite
ok   - --help names the e2e suite
ok   - --help says a flagged run is not done
ok   - an unknown flag exits 2
ok   - SELECTED is appended in exactly one place
ok   - that one place is inside step()
ok   - SKIPPED is appended in exactly one place
ok   - that one place is inside skip()
ok   - no writing formatter or linter variants
ok   - verify.sh exports GOTOOLCHAIN derived from GO_VERSION
ok   - excluded suites appear only in comments and --help
ok   - a failing gate exits non-zero
ok   - the failing gate is named on the verdict line
ok   - the failing gate announced itself
ok   - the run stopped before the next gate

verify_test.sh: all assertions passed
```
Exit 0. **PASS** (AC-11's structural assertion — see §3 below).

### `--quick`

```
$ ./tools/verify.sh --quick 2>&1 | tail -3
- docker buildx (skipped: --quick)

VERIFY: PARTIAL — all selected gates passed, but 2 were skipped (npm ci, docker buildx). This does NOT count as done.
```
**PASS** (`VERIFY: PARTIAL`, does not count as done, as expected).

### `--no-docker`

```
$ ./tools/verify.sh --no-docker 2>&1 | tail -3
- docker buildx (skipped: --no-docker)

VERIFY: PARTIAL — all selected gates passed, but 1 were skipped (docker buildx). This does NOT count as done.
```
**PASS.**

### Flagless run — AC-7 is environmentally blocked

The brief's Step 2 expects the flagless run to end in one `VERIFY: DONE`.
**It did not, and this report does not pretend otherwise.** The flagless run
was executed once, to completion, with a 900000 ms timeout, and its actual
output is pasted below verbatim.

```
$ time ./tools/verify.sh
[... toolchain drift: OK ...]
── golangci-lint
0 issues.
── go build
[OK]
── npm ci
[OK]
── npm run lint
[OK]
── npm run format
[OK]
── npm test
[OK]
── npm run build
✓ 2732 modules transformed.
✓ built in 2.19s
── docker buildx
#24 [linux/amd64 frontend 4/6] RUN npm ci
#24 ...
#25 [linux/arm64 frontend 4/6] RUN npm ci
#25 0.790 exec /bin/sh: exec format error
#25 ERROR: process "/bin/sh -c npm ci" did not complete successfully: exit code: 255
#24 [linux/amd64 frontend 4/6] RUN npm ci
#24 CANCELED
------
 > [linux/arm64 frontend 4/6] RUN npm ci:
0.790 exec /bin/sh: exec format error
------
Dockerfile:7
--------------------
   5 |     WORKDIR /src
   6 |     COPY apps/frontend/package.json apps/frontend/package-lock.json ./
   7 | >>> RUN npm ci
   8 |     COPY apps/frontend/ ./
   9 |     RUN npm run build
--------------------
ERROR: failed to build: failed to solve: process "/bin/sh -c npm ci" did not complete successfully: exit code: 255
✗ docker buildx FAILED
VERIFY: FAILED — docker buildx

./tools/verify.sh > /tmp/hm-verify-flagless-full.log 2>&1  209.77s user 73.81s system 282% cpu 1:40.45 total
EXIT_CODE=1
```

**Every gate before `docker buildx` — toolchain drift, `go test -race`,
`go vet`, `golangci-lint` (0 issues), `go build`, `npm ci`, `npm run lint`,
`npm run format`, `npm test`, `npm run build` — passed cleanly.** Only the
container gate's arm64 leg fails, at `npm ci`, with `exec format error`.

Verified environmental, not a gate defect:

```
$ ls /proc/sys/fs/binfmt_misc/
WSLInterop  0B
WSLInterop-late  0B
register  0B
status  0B
```

No `qemu-aarch64` handler is registered on this machine — only WSLInterop.
`docker buildx build --platform linux/amd64,linux/arm64` cannot execute any
arm64 binary, including `npm`, without one.

**Remedy** (deliberately NOT run — it registers binfmt handlers system-wide
on the user's machine, outside this task's authority to change host state):

```
docker run --privileged --rm tonistiigi/binfmt --install arm64
```

**Reported verdict for AC-7:** blocked by this machine's missing arm64 binfmt
handler, with the remedy above. All ten non-docker gates pass. The strongest
verdict obtainable from this repository is `--no-docker` reaching
`VERIFY: PARTIAL` cleanly (shown above) — which it does.

### Tree state (AC-10)

```
$ git status --porcelain > /tmp/hm-final-before.txt   # (before the flagless run; already clean)
$ ./tools/verify.sh ...                                # (flagless run above)
$ git status --porcelain > /tmp/hm-final-after.txt
$ diff /tmp/hm-final-before.txt /tmp/hm-final-after.txt
0a1
>  M apps/frontend/tsconfig.tsbuildinfo
```

The `--quick`, `--no-docker`, and flagless runs each invoke `npm run build`,
which runs `tsc -b` and rewrites the TRACKED file
`apps/frontend/tsconfig.tsbuildinfo` (confirmed tracked: it exists on `main`
via `git ls-files`). `verify.sh` itself runs no writing command — the dirt is
a side effect of the real `tsc -b` build tool, not of the verification
script. Each time this happened during this task's runs, it was restored:

```
$ git restore apps/frontend/tsconfig.tsbuildinfo
$ git status --porcelain
(clean)
```

**AC-10 verdict: met for `verify.sh`'s own behaviour, with this single
pre-existing exception named.** Untracking the file would require
`git rm --cached apps/frontend/tsconfig.tsbuildinfo`, which touches a path
under `apps/` — AC-21 forbids that absolutely in this task. Listed under
Known follow-ups below.

## 3. AC-11, asserted structurally

A passing run cannot prove a command did *not* run. `verify_test.sh` carries
a structural assertion for this instead:

```
$ ./tools/verify_test.sh 2>&1 | grep 'excluded suites'
ok   - excluded suites appear only in comments and --help
```

This asserts that the on-demand suites (`HARBORMASTER_INTEGRATION=1 go test
-tags=integration` and `npm run test:e2e`) are named only in `verify.sh`'s
comments and `--help` text — never inside a `step()`/`skip()` call that would
actually execute them as part of any mode. **PASS.**

## 4. AC-23 — process-parity.md §7 check 1 (hook byte-identity)

Harbormaster's current `sha256sum` for the eight hook files, re-run in this
task (identical to Task 5's recorded values, confirming no drift since
porting):

```
a98f2959146ca0d5e7574926f5cd8c81aa33ad83c1ed2525367d6d3ad7573591  .claude/hooks/wait-loop-guard.sh
3ee10948cce01b293f4a2c31a230ff046bd09203284706aa9f6271019db9dd2f  .claude/hooks/wait-loop-guard_test.sh
d42cfae89f747d2a2622e0f085a8b44627545fb20d1f729bf468319878b17a15  .claude/hooks/block-home-paths-in-docs.sh
0214454609b4d6c8a7a287afd86fab819f6c8e6215d87f0691039c79194903f5  .claude/hooks/turn-budget.sh
ce88cae21f6c2ea3ea28f72c5c8727bc6f032cae3317dda285de176cfbae9376  .claude/hooks/turn-budget-guard.sh
65896f4c463e1643347036fbd1846225ff99f1389f3655fd6db283560e23e6f2  .claude/hooks/fork-dispatch-guard.sh
3642046a73e6e41fff91c2960acb8af5014a7559135d4ee9c862efba22a14d65  .claude/hooks/commit-boundary.sh
44f166b9502651b49888c6d8dea77de77b37fc0c204e74f61872234792168dcf  .claude/hooks/task-num-collision-detector.sh
```

The pairwise byte-identity comparison across atlas, home-hub, MyFleet and
Harbormaster **cannot be performed from this repository.** These are
Harbormaster's hashes only.

One divergence is known and deliberate:
`.claude/hooks/wait-loop-guard_test.sh` differs from the atlas original in
two allow-list fixture strings — `kubectl get pods -n atlas-pr-1370` →
`harbormaster-pr-1370` and `journalctl -u atlas` → `journalctl -u
harbormaster`. `process-parity.md` §7 check 1 (byte-identity) and this
task's AC-1 / FR-1 (`grep -l 'atlas-' .claude/hooks/*.sh` prints nothing)
cannot both hold for this file. AC-1 won: it is mechanically checkable here
and is a hard criterion, check 1 cannot be evaluated from this repository at
all, and §5.2 explicitly authorises genericizing examples. The guard itself,
`wait-loop-guard.sh`, is byte-identical. Whoever runs check 1 should expect
seven of eight to match.

## 5. AC-24 — process-parity.md §7 checks 4, 5, 6, Harbormaster's side only

**Cross-repo comparison is not performed here.** Only Harbormaster's own
shape is reported.

**Check 4 (hook set and events wired):** nine hooks present in
`.claude/hooks/` (`wait-loop-guard.sh` + `_test.sh`,
`block-home-paths-in-docs.sh`, `turn-budget.sh`, `turn-budget-guard.sh`,
`fork-dispatch-guard.sh`, `commit-boundary.sh`,
`task-num-collision-detector.sh`, `format-on-write.sh`). Wired in
`.claude/settings.json`:
- `PreToolUse`: matchers `Write|Edit`, `Agent`, `Bash`, `*` →
  `block-home-paths-in-docs.sh`, `fork-dispatch-guard.sh`,
  `wait-loop-guard.sh`, `turn-budget-guard.sh`.
- `PostToolUse`: matchers `Write|Edit`, `*`, `Bash` →
  `format-on-write.sh`, `turn-budget.sh`, `commit-boundary.sh`.
- Hook event keys present: `PostToolUse`, `PreToolUse`, `SessionStart`,
  `UserPromptSubmit`.
(`task-num-collision-detector.sh` is invoked by `tools/task-numbers.sh`, not
directly wired as a Claude Code hook — consistent with Task 5/6's evidence.)

**Check 5 (CLAUDE.md heading list and trigger table targets):** `CLAUDE.md`'s
eight `##` headings, in order: Never do this / Evidence & grounding /
Development workflow / Done means verified / Dispatching agents / Handing
off context / Repository conventions / Where the procedures live. Skill
trigger table lives in `.claude/skills/skill-rules.json`, targeting the
`backend-dev-guidelines` and `frontend-dev-guidelines` skills per this
project's `CLAUDE.md` §"Code Review Pattern".

**Check 6 (ten owner documents):** all ten present and confirmed above —
`docs/agent-dispatch.md`, `docs/verification.md`,
`docs/superpowers-integration.md`, `docs/review-protocol.md`,
`docs/post-implementation.md`, `docs/codemod-vs-agents.md`,
`docs/slice-first.md`, `docs/tooling-conventions.md`,
`docs/git-workflow.md`, `docs/observability.md`.

## 6. AC-17 — reviewer-judged, not self-asserted

**This report does not mark AC-17 pass.** It requires a reviewer comparing
each ported document against its atlas original for a rule dropped along
with its example. Documents with substantive rebinds, for the reviewer to
check:

- `docs/verification.md` — whole flag surface rebound to Harbormaster's
  `verify.sh`/`verify_test.sh`.
- `docs/observability.md` — rewritten; sections describing pipeline
  mechanics were dropped because Harbormaster's pipeline differs from
  atlas's.
- `docs/slice-first.md` — atlas's `doc-slice.sh` replaced with
  Harbormaster's equivalent tooling.
- `docs/agent-dispatch.md` — agent roster rebound to Harbormaster's set.
- `docs/codemod-vs-agents.md` — worked examples rebound.
- The three agent files (`backend-guidelines-reviewer.md`,
  `frontend-guidelines-reviewer.md`, `plan-adherence-reviewer.md`) — ported
  with Harbormaster-specific checklists.

For the reviewer's benefit: every one of these documents WAS reviewed
against its atlas source during this build, and `docs/observability.md`'s
dropped sections specifically were independently re-read and judged
justified by a reviewer at that time. That prior review is not a
self-certification of AC-17 by this task — a reviewer must still make the
final call here.

## 7. Known follow-ups

- **eslint / `npm run lint` strictness.** 7 eslint warnings currently pass
  because `npm run lint` does not use `--max-warnings 0`. Whether to tighten
  this is PRD §9 Q1 — explicitly out of scope for this task.
- **Four CI-only gates**, recorded in `docs/verification.md`, not
  reproducible from a local `verify.sh` run.
- **`untrack and gitignore apps/frontend/tsconfig.tsbuildinfo`.** It is a
  tracked build artifact that `tsc -b` rewrites on every `npm run build`;
  doing so requires `git rm --cached` under `apps/`, which AC-21 forbids
  absolutely in this task.
- **`plan-adherence-reviewer` overwrites `audit.md`.**
  `.claude/agents/plan-adherence-reviewer.md:64` says "Write the report to
  `<task-folder>/audit.md` (overwriting any existing audit)", while
  `backend-guidelines-reviewer.md:119` and `frontend-guidelines-reviewer.md:114`
  both APPEND. A plan-adherence run after the guideline reviewers destroys
  their output. Not fixable here: those four agent files are required by
  AC-12 to stay byte-unchanged. `docs/review-protocol.md` already documents
  the hazard.
- **Dead metrics configuration.** `apps/backend/internal/config/config.go:35-37`
  and `:77-79` declare and populate `MetricsEnabled`, `MetricsListenAddr` and
  `OTELExporterOTLPEndpoint`, but no non-test Go consumes them. Meanwhile
  `docs/operator/configuration.md:46-48` documents `METRICS_ENABLED` as
  enabling a Prometheus listener on a separate `http.Server`, which does not
  exist. Out of scope here (the code fix is under `apps/`, which AC-21
  forbids).
- **Minor documentation nits deferred during review**, each one line:
  `docs/observability.md` says "both deploy manifests set"
  `HARBORMASTER_LOG_FORMAT` when only `deploy/kubernetes/deployment.yaml`
  does; `docs/observability.md` says "the pipeline is five files", omitting
  `processor.go`; `docs/observability.md`'s deploy smoke test lost the
  source's cardinality-verification step (the rule survives elsewhere);
  `docs/git-workflow.md` demotes `gh` token-env clearing from a habit to a
  troubleshooting tip; `CLAUDE.md` drops the tail clause "document those
  directly via a brainstorming session" from the skip-`/spec-task` rule;
  `tools/verify.sh`'s drift-check awk does not match a QUOTED version scalar
  (`version: "v2.12.2"`) — it fails loudly rather than vacuously, but a YAML
  reformat of `pr.yml` would red the gate for a non-drift reason.
