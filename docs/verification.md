# Verification

[`tools/verify.sh`](../tools/verify.sh) is the pre-PR gate. It must exit 0 —
**and print the `DONE` verdict** — before a branch is called "done," "ready for
PR," or handed to `superpowers:finishing-a-development-branch`.

```sh
./tools/verify.sh              # full gate — what you run before opening a PR
./tools/verify.sh --quick      # inner loop: no docker, no -race, maybe no npm ci
./tools/verify.sh --no-docker  # everything except the container build
./tools/verify.sh --list       # print the gates these flags would select, run none
./tools/verify.sh --help       # the flag surface and the gate list
```

That is the entire flag surface. An unrecognised flag is a usage error and exits
`2` — it is never ignored, because a typo'd flag that silently ran the wrong
subset would produce a report nobody could trust.

Only the **flagless** invocation counts as verified. `--quick` and `--no-docker`
also exit 0; they say so on their verdict line. Never claim verified from a
subset.

The script mirrors what `.github/workflows/pr.yml` runs, plus a toolchain drift
check and an untagged `go vet ./...`. **CI is the authority; the script is the
bug.** If the two disagree, fix `tools/verify.sh` to match CI — never loosen CI
to match the script. The four places CI deliberately runs more than the script
are enumerated under
[Known drift between this gate and CI](#known-drift-between-this-gate-and-ci),
so they are a recorded gap rather than a rediscovery.

This document explains *why* each gate exists and what breaks when it is
skipped. The list of gates lives in the script's registry, not here — do not
maintain a second copy of it in `CLAUDE.md` or anywhere else. `./tools/verify.sh
--help` prints the current list.

---

## Asking the gate what it selected — `--list`

Before investigating why a run skipped something, ask it:

```sh
./tools/verify.sh --list --quick
```

```text
toolchain drift
go test
go vet
golangci-lint
go build
- npm ci (skipped: --quick, node_modules current)
npm run lint
npm run format
npm test
npm run build
- docker buildx (skipped: --quick)
```

Pass the same flags you would really run — the answer reflects them, so
`--list --quick` reports what `--quick` would do. Skipped gates are announced in
`--list` *and* in a real run, prefixed with `- `, because a truncated transcript
must still make clear what did not execute.

**It cannot drift from a real run.** `--list` does not re-implement the
selection logic: it runs the script's real body and neuters `step()`, so each
gate that the real predicates reach records its label instead of executing. It
is the real run with the work removed, not a second description of it. When you
add a gate, add it to the registry — there is no separate list printer to keep
in sync, and `tools/verify_test.sh` asserts that a gate label can only originate
inside `step()`.

Use it instead of reading the script. If you find yourself grepping
`tools/verify.sh` to work out what a set of flags will select, you are deriving
a fact the script will hand you in under a second. See
[`tooling-conventions.md`](tooling-conventions.md) — "Ask for a fact rather than
deriving it."

## Why this gate stops at the first failure

`tools/verify.sh` walks eleven gates in registry order and **exits at the first
one that fails**, naming it on stderr. It does not continue and summarise.

It also has **no change detection of any kind**, and no `--base`-style flag for
scoping a run to a diff; like any unrecognised flag, one would exit 2. That was
an explicit design rejection, not an omission. Do not add one, and do not read
its absence as an incomplete port of a sibling repository's gate.

Both choices are deliberate, and both differ from the atlas gate that this
document was ported from:

> Atlas's 80-module fan-out makes a full picture worth the wall time, so its
> gate runs everything and summarises. Harbormaster has eleven gates over one Go
> module and one frontend, and the first failure is almost always the whole
> story. A verifier agent asked for "the first failing block" wants the run to
> have stopped there — otherwise it has to work out which of several failures
> came first.

The same argument disposes of change gating. With one Go module and one
frontend, "which surfaces changed" collapses to "backend, frontend, or both" —
a distinction worth roughly one `npm ci`. The machinery to compute it correctly
(merge-base resolution, a fan-out rule for shared files, a way to narrow the
base per task, and a way to explain what it selected) is a standing source of
silent under-verification for that saving. `--quick` buys the inner-loop speed
instead, and buys it honestly: it says on the verdict line that it did less.

Launch the gate in the background and keep working; never idle waiting on it.
See [`tooling-conventions.md`](tooling-conventions.md) — "Waiting on processes."

## The three verdicts

All of `DONE` and `PARTIAL` **exit 0**. That is deliberate — `--quick` has to
stay usable in an `&&` chain in the inner loop. The distinction lives in the
terminal `VERIFY:` line, and nowhere else:

- `VERIFY: DONE — all gates passed; the branch may be called done.`
- `VERIFY: PARTIAL — all selected gates passed, but <N> were skipped (<labels>). This does NOT count as done.`
- `VERIFY: FAILED — <label>` — on stderr, exit 1.

**Quote the verdict line.** "verify.sh exited 0" is not a report; it is
consistent with a run that skipped the container build and half the backend's
race detection. A report that omits the verdict line is under-reporting, and a
reviewer is entitled to treat it as unverified.

`--list` prints no verdict at all and always exits 0. It ran nothing, so it has
nothing to attest.

## The `--quick` trade

`--quick` does three things:

1. Skips the container build.
2. Drops `-race` from the backend tests (the gate label changes from
   `go test -race` to `go test`, so the transcript shows which one ran).
3. Skips `npm ci` when `apps/frontend/node_modules/.package-lock.json` is not
   older than `apps/frontend/package-lock.json`.

That third condition compares mtimes rather than checking for a `node_modules`
directory, because npm writes `.package-lock.json` on every install as a record
of the installed tree. Bare directory presence would let a stale tree mask a
lockfile change — a dependency bump followed by `--quick` would report green on
packages that were never installed.

What a `--quick` pass has therefore *not* established: that the backend is
race-clean, that the container builds on either architecture, and — when the
skip fired — that the installed dependency tree matches the lockfile. That is
exactly why it reports `PARTIAL`. Run it as many times as you like in the inner
loop; run the flagless gate before you call anything done.

`--no-docker` skips the container build and nothing else. It is the right flag
when the container gate is failing for an environmental reason (see below) —
and it still yields `PARTIAL`, not `DONE`.

## The Go layer

From `apps/backend`: `go test -race -count=1 ./...`, `go vet ./...`,
`golangci-lint run`, `CGO_ENABLED=0 go build ./...`, in that order. Tests before
vet before lint before build: the cheapest signal that is most likely to be the
real problem goes first, and the build is last because a tree that vets and
lints clean almost always builds.

`go vet` is run separately from `golangci-lint` even though the linter bundles a
`govet` analyzer. This gate is **script-only** — CI has no untagged `go vet
./...` job to mirror. Its `pr.yml` jobs are `backend-lint` (`golangci-lint-action`),
`backend-test` (`go test -race -count=1 ./...`, which runs `go vet`'s default
subset as a prelude), `backend-build` (`CGO_ENABLED=0 go build ./...`, which
does not vet), and `backend-integration-build` (`go vet -tags=integration
./...`, scoped to integration-tagged files only). Running the untagged vet up
front buys a cheap, fast check — full package-level `govet` coverage — before
paying for the slower `golangci-lint` gate, without waiting on it.

### The Go toolchain is pinned, on purpose

`tools/verify.sh` sources [`tools/toolchain.versions`](../tools/toolchain.versions)
and exports `GOTOOLCHAIN=go$GO_VERSION` (currently `1.25.12`) once, before any
Go gate — including the `golangci-lint` gate and the `go install` that
bootstraps it.

CI's Go jobs take their version from `apps/backend/go.mod` via `setup-go`'s
`go-version-file`. A developer machine may have anything on PATH. Without the
pin, the local gate lints and tests under a toolchain CI never uses, and the two
can disagree for reasons that have nothing to do with the code.

This is not hypothetical. On a machine with go1.27.0 on PATH, golangci-lint
v2.12.2's bundled `honnef.co/go/tools` panics building IR for the go1.27
standard library. Unpinned, a branch that is green in CI goes red locally with a
stack trace from inside the linter. Pinned, both sides run go1.25.12 and agree.

That is the "CI is the authority; the script is the bug" rule doing its work:
the disagreement was resolved by moving the *script* onto CI's toolchain, not by
relaxing anything. The same class of problem is noted in `pr.yml`'s
`license-allowlist` job, whose comment records that a module-cache toolchain
breaks `go-licenses` on std packages.

If you bump Go, bump `apps/backend/go.mod` and `tools/toolchain.versions`
together. Gate 0 does not check the Go pin — it checks the golangci-lint pin —
so this one is on you.

### The golangci-lint bootstrap

The script is the **only** thing that provisions golangci-lint. It compiles the
pinned version into `.cache/tools/bin/golangci-lint-<version>` on first use and
reuses it thereafter. `.claude/hooks/format-on-write.sh` reads the same path and
never creates it: a PostToolUse hook that compiled a linter would stall every
`Write` for a minute on a cold cache.

Consequences worth knowing:

- The first gate run in a **fresh worktree** pays that compile — up to a minute
  — and says so on stdout. It is not a hung lint.
- `.cache/` is per-worktree, so each new task worktree pays it once.
- Bumping `GOLANGCI_LINT_VERSION` changes the cached filename, so the old binary
  is simply unused rather than stale-hit.

The config is at **`apps/backend/.golangci.yml`**, not the repo root. Looking
for it at the root and concluding there is none is a recurring mistake.

### Gate 0: toolchain drift

The golangci-lint pin exists in two places — `tools/toolchain.versions` (which
the gate and the format hook read) and `.github/workflows/pr.yml` (which CI
reads). If they disagree, every downstream lint result is suspect, so this runs
first and cheapest.

It fails loudly when it cannot find the version key at all, rather than matching
nothing and passing. **A check that can pass vacuously is worse than no check**,
because it converts an unknown into a false assurance. That rule generalises:
every gate here either runs or announces that it did not.

## The container layer

`docker buildx build --platform linux/amd64,linux/arm64 -f
deploy/docker/Dockerfile .` from the repo root. **This is mandatory, not
optional**, and it is the gate most often skipped for the worst reasons.

`go build` and `npm run build` run against the working tree and will not catch a
missing `COPY` line, a wrong workdir, or a stage that installs the wrong Node
version in `deploy/docker/Dockerfile`. Only the image build will. CI catches it,
but each round-trip wastes a CI cycle and turns "verified" into a lie.

When `docker` or `docker buildx` is absent, the gate **fails** and names
`--no-docker` in the error. It does not silently downgrade to a pass: reporting
success for a gate that did not run is the single failure mode this script
exists to prevent.

### Known local prerequisite: arm64 emulation

Because the build is two-platform, the `linux/arm64` leg needs a `qemu-aarch64`
binfmt handler on a `linux/amd64` machine. Without one, the arm64 leg fails
partway through — typically at `npm ci` — with `exec format error`. That is an
environmental prerequisite, not a gate defect, and not a reason to weaken the
gate to a single platform: the project ships both.

Install the handler once:

```sh
docker run --privileged --rm tonistiigi/binfmt --install arm64
```

If you cannot or will not install it, `./tools/verify.sh --no-docker` is the
correct fallback — while remembering that it yields `PARTIAL`, not `DONE`, and
that the container build still has to pass in CI before merge.

## Lint & format

- `npm run lint` is bare `eslint .`. The tree currently carries **7 warnings and
  0 errors**. The gate passes on warnings by design: **do not** tighten it with
  `--max-warnings 0` as a drive-by. Clearing the warnings and raising the bar is
  its own task, and raising the bar before clearing them would land a gate that
  fails on `main`.
- `npm run format` is `prettier --check .` — it **verifies**, it does not
  rewrite. To fix formatting, run `npm run format:fix` yourself. The gate
  never mutates the tree on your behalf (with one exception, below), so a green
  run followed by a dirty diff is never something the gate did to you.
- `golangci-lint run` uses `apps/backend/.golangci.yml`; Go formatting is
  enforced there and by `.claude/hooks/format-on-write.sh` as files are written.

## The gate does not mutate the tree — with one known exception

No gate command in `tools/verify.sh` is a writing command: the formatters run in
check mode, the container build is a build, and the tests do not write into the
tree.

**But a full run does leave `git status --porcelain` non-empty.**
`apps/frontend/tsconfig.tsbuildinfo` is a tracked file, and the `npm run build`
gate runs `tsc -b`, which rewrites it. So after a green flagless run you will
find exactly one modified path, always that one. It is expected; it is not
evidence that a gate wrote to your source.

Recommended follow-up, deliberately not done here because it requires a change
under `apps/`: untrack `apps/frontend/tsconfig.tsbuildinfo` and add it to
`.gitignore`. A build artifact in version control has no upside and this is its
only visible cost.

If you see *other* modified paths after a run, that is a real finding — treat it
as a gate defect and report it.

## Suites the gate never runs

Two suites are named in `--help` and run by nobody automatically. They are not
per-PR gates and they **never affect this script's exit code or its verdict**.
Run them by hand when the change warrants it:

```sh
cd apps/backend  && HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...
cd apps/frontend && npm run test:e2e
```

The backend one needs Docker (it starts MinIO); the frontend one needs the
Compose stack up.

`.github/workflows/nightly.yml` runs the integration suite nightly — `go test
-race -count=1 -tags=integration ./...` with `HARBORMASTER_INTEGRATION=1`,
against both the supported MinIO floor and `latest`. So integration breakage is
caught, but up to a day after merge, not before it. If your change touches the
MinIO client surface, run it yourself.

Their exclusion is asserted **textually**, not behaviourally:
`tools/verify_test.sh` greps `tools/verify.sh` and requires that neither
`HARBORMASTER_INTEGRATION` nor `test:e2e` appears outside a comment or the
`--help` heredoc. This is the right shape of assertion, and the reason is
general: **a passing run cannot prove a command did not run.** Any invariant of
the form "X never happens" needs a structural check, not a green test.

## Adding a gate

1. Write a `gate_<name>` function next to its peers. It runs one command in a
   subshell and returns its status.
2. Register it with `step '<label>' gate_<name>` in the registry block at the
   bottom, in the position you want it walked. `step()` is the only place a
   *selected* gate may be recorded or executed — that invariant is what makes
   `--list` trustworthy, and `tools/verify_test.sh` enforces it.
3. If the gate is conditional, use `skip '<label>' '<reason>'` in the else
   branch. A conditional gate that vanishes silently defeats the transcript.
4. Add the CI job that mirrors it — or, if you consciously choose not to, record
   the asymmetry in the table below. An unrecorded asymmetry gets rediscovered
   at cost.
5. Extend `tools/verify_test.sh`. It runs the script with a stubbed `go` on
   PATH to prove the first-failure behaviour hermetically, and asserts the
   `--help`/`--list` surface; a new gate that no assertion mentions is a gate
   the harness cannot notice regressing.

## Known drift between this gate and CI

Tracked here so it is visible rather than folklore. **CI is the authority for
everything it runs.** Neither side is a strict superset, but the asymmetry is
one-directional today: CI runs four things the script does not, and the script
runs two things CI does not.

| CI-only gate | Where | Why it is not in the script |
|---|---|---|
| `gitleaks` | `pr.yml`, `gitleaks/gitleaks-action` | Needs its own toolchain and a `GITHUB_TOKEN`; a local miss is caught in CI before merge, and no credential handling may be introduced into the local gate. |
| Trivy filesystem scan | `pr.yml`, `aquasecurity/trivy-action` with `.trivyignore` | Same: its own toolchain, a vulnerability database to fetch, and network. Local runs would be slow and non-deterministic across a stale DB. |
| `go-licenses` allowlist | `pr.yml`, `go-licenses check ./...` against an allowed-licenses list plus [`tools/licenses/allowlist.yaml`](../tools/licenses/allowlist.yaml) | Installs a tool and needs `yq`. It also depends on the directly-installed Go rather than a module-cache toolchain, which the job's own comment records as a trap. |
| `go vet -tags=integration ./...` | `pr.yml`, `backend-integration-build` | Cheap and genuinely tempting. The `//go:build integration` files are invisible to every other backend job, so signature drift between production code and the integration tests stays green on PRs and only surfaces in the nightly run — which is exactly why CI has this compile-only guard. It is absent locally because the gate list was fixed by the task that created the script and a twelfth gate was scope that task did not authorise. Recorded here so it is a known gap. |

Script-only: the **toolchain drift** check (gate 0) and untagged **`go vet
./...`**. CI has no equivalent for either — CI *is* one of the two sources the
toolchain check compares, so it cannot check itself, and CI's only vet coverage
is `go test`'s default subset plus the integration-tagged vet, never an
untagged `go vet ./...` job. A pin mismatch landed without ever running
`tools/verify.sh` locally would go unnoticed until the next local run.

Also nightly-only: the real integration suite
(`.github/workflows/nightly.yml`). It is neither a local gate nor a PR gate.

When the script and CI disagree on something they both claim to check, the
resolution is always the same: **CI is the authority; the script is the bug.**
Fix `tools/verify.sh`. The Go toolchain pin above is the worked example — the
fix was to make the local gate run under CI's toolchain, not to explain away
CI's result.

## When a gate fails

1. **Quote the verdict line**, including the gate label. `VERIFY: FAILED — go
   vet` tells a reader which of eleven things broke; "verify failed" does not.
2. **Report the first failing block, not the whole transcript.** The run stopped
   at the first failure, so the tail of the output is the failure. See
   [`slice-first.md`](slice-first.md) for reading long output without paying for
   all of it.
3. **Check the environmental list first** when the failure is `docker buildx`
   (arm64 binfmt) or `golangci-lint` (toolchain). Both have known non-code
   causes documented above, and both have cost people a debugging session.
4. **Do not re-run with fewer flags to get a green.** `--no-docker` after a
   docker failure is a legitimate *fallback for a machine that cannot run the
   gate*, and an illegitimate way to make a real container-build failure
   disappear. The verdict line says `PARTIAL` either way; the difference is
   whether you are honest about which one you were doing.
5. **Run the gate in a clean context, not inside the implementer.** A fresh
   session running `./tools/verify.sh` and reporting the verdict line costs a
   fraction of the context that re-reading a long build transcript into an
   implementer's window costs, and it will not rationalise its own code. The
   `task-verifier` agent (`.claude/agents/task-verifier.md`) exists for this.
6. **A fix for a failure found here is a normal commit on the branch.** See
   [`git-workflow.md`](git-workflow.md). A failure found *after* merge, in CI or
   in review, goes through `/fix-pr-bug`
   (`.claude/commands/fix-pr-bug.md`) instead.
