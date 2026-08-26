# Tooling Conventions

This document owns three conventions for how commands and edits are run in
this repo: locating Go module source, waiting on long-running processes, and
general shell/editing hygiene.

## Locating Go module source

Never sweep the filesystem to locate a Go dependency's source. Ask the
toolchain instead:

```sh
go list -m -f '{{.Dir}}' <module>
```

This prints the directory in ~0.02s, whether the module resolves to the
module cache or (were one in play) a local `replace`. The same applies to
`go doc <pkg>` for a symbol and `go list -m all` for the version set.

`apps/backend` is a single Go module with no `go.work` and no local
`replace` directives, so every import outside `internal/` resolves into the
module cache — `find /` or `find $GOPATH` to locate, say, `gorm.io/gorm`'s
source takes minutes and requires guessing the module cache's
case-escaping (`gorm.io/gorm@v1.2.3` on disk, not the import path verbatim);
`go list -m -f '{{.Dir}}' gorm.io/gorm` answers directly. `find` is for
paths you own, rooted at a directory you name — never at `/`.

## Waiting on processes

Never spend inference turns waiting for a process. Launch it once with a
bound — `run_in_background: true`, or `Monitor` with an until-loop — and do
something else or hand back.

Repeated `sleep` / `ps aux | grep` / `echo waiting` / `for i in $(seq …); do
sleep` calls are the anti-pattern: each one re-reads the whole context to
learn nothing, and they cluster late in a session where that is most
expensive. If the process exceeds its bound, kill it and fall back; do not
keep polling. When a tool has a known hang mode, the fallback belongs in
that tool's agent doc, not in a longer wait.

The same holds for **waiting on a child agent**. There is no wait primitive
because none is needed: completions arrive as notifications, so do other work
or end the turn and be re-invoked. Emitting `true` to stay alive is the worst
version of this — measured at 30 such calls inside one agent, 36% of its
entire cost, for zero information.

`.claude/hooks/wait-loop-guard.sh` makes this machine-checked rather than
advisory, the way `.claude/hooks/fork-dispatch-guard.sh` did for forks. It
refuses bare no-ops, sleep-driven polls, and broad `ps`/`pgrep` sweeps. It
deliberately allows real process debugging — `ps -p <pid>`, `kill`/`pkill`,
`kubectl`, `docker ps`, `top -b -n1` — and anything prefixed
`POLL-JUSTIFIED: <reason>`, mirroring `FORK-JUSTIFIED:`. A considered wait
costs one sentence; the reflexive one is blocked.

## Ask for a fact rather than deriving it

Mechanical repository facts have deterministic sources. Use them:

| Question | Ask |
|---|---|
| Is local lint drifting from CI's toolchain | `tools/verify.sh` gate `toolchain drift` — compares `tools/toolchain.versions` against `.github/workflows/pr.yml`'s pinned actions and fails loudly if they disagree, rather than a reviewer noticing a version mismatch by eye |
| The next unused `task-NNN` number, or whether one collides | `tools/task-numbers.sh next` / `tools/task-numbers.sh check` |
| Which worktree/branch a task lives on, what artifacts it has | `git worktree list`, then `ls docs/tasks/<task>/` in the matching worktree — there is no `task-facts.sh` in this repo; these two commands are the manual equivalent |
| What gates a plain `tools/verify.sh` run will execute | `tools/verify.sh --list` — prints the gate list and runs none. There is no `--base`/`--facts` change-detection mode; `verify.sh` always runs the full selected gate set |
| Which files a branch touched | `git diff --name-only main...HEAD` — there is no `change-surfaces.sh` in this repo |
| One task out of a plan | `tools/task-brief.sh <plan> <N> [outfile]` |
| One section or a few rows of a large document | no `doc-slice.sh` in this repo — see [slice-first.md](slice-first.md) for the `grep`/`sed`/`Read`-offset equivalents |

Do not probe for toolchain availability (`command -v`, `--version`, `which`)
when a script already reports it — e.g. `tools/verify.sh`'s toolchain-drift
gate prints the pinned `GO_VERSION`, `NODE_VERSION`, and
`GOLANGCI_LINT_VERSION` from `tools/toolchain.versions` on every run; that is
the environment stating the fact rather than a probe rediscovering it.

**A deterministic tool defeated by a wrapper is a net loss.** When a
token-optimizing shell wrapper swallows a script's stdout, the saved bytes
cost a whole extra turn to recover the output. Any such wrapper must pass
`tools/*.sh` output through unfiltered; these scripts already emit compact,
purpose-built output and have nothing worth trimming.

## Shell and editing conventions

Prefer portable POSIX shell; avoid zsh/direnv-specific constructs and batch
patch loops that can produce garbled or unapplied output. For a multi-file
edit, prefer per-file Edit/Write over a shell patch loop.

Quote glob arguments in shell tool calls — `--include='*.go'`, not
`--include=*.go` — zsh expands an unquoted glob before `grep` sees it,
producing `no matches found` and a wasted retry.

Preserve line endings when editing — do not normalize CRLF→LF as a side
effect; it inflates diffs with spurious changes.

Always use repo-relative paths or placeholders in committed files; never
literal home or absolute paths like `/Users/<name>/...` or
`/home/<name>/...` — a committed absolute path is not reproducible on
another machine. `.claude/hooks/block-home-paths-in-docs.sh` enforces this
for every write under `docs/`.
