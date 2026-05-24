# Audit — task-001-harbormaster-mvp-v1

## M0 — Plan adherence audit (T0.1–T0.14)

**Reviewer:** plan-adherence-reviewer
**Date:** 2026-05-24
**Branch:** task-001-harbormaster-mvp-v1 @ m0-complete

### Verification commands

- [PASS] backend tests/vet/build — `go test -race ./...` (cached PASS for `cmd/harbormaster`), `go vet ./...` (no output), `CGO_ENABLED=0 go build ./...` (no output)
- [PASS] frontend lint/format/test/build — `npm run lint` clean, `npm run format` "All matched files use Prettier code style!", `npm test` 1/1 passed, `npm run build` produced `dist/index.html` + chunks
- [PASS] docker buildx — built `harbormaster:m0-audit` for `linux/amd64`, finished with `exporting manifest list sha256:25b018926ce5…`
- [PASS] docker run prints placeholder — `docker run --rm harbormaster:m0-audit` printed `harbormaster placeholder — M1 will replace this`
- [PASS] tag m0-complete exists — `git tag --list m0-complete` returns `m0-complete`
- [PASS] pr.yml jobs list — `['backend-build', 'backend-lint', 'backend-test', 'dependency-scan', 'frontend-build', 'frontend-lint', 'frontend-test', 'gitleaks', 'license-allowlist']` (exact match with expected)

### Per-task adherence (T0.1 … T0.14)

- **T0.1 — PASS.** `LICENSE` is 661 lines, starts `GNU AFFERO GENERAL PUBLIC LICENSE / Version 3, 19 November 2007`, ends with the canonical `<https://www.gnu.org/licenses/>` footer (LICENSE:1-5, 659-661). `README.md` carries the project tagline plus AGPL boilerplate (README.md:1-19). `.editorconfig:1-17` sets utf-8/LF/2-space defaults with Go tab + Makefile-tab overrides. `.gitignore:1-39` covers Go (`apps/backend/bin/`, `harbormaster`, `*.test`, `coverage.txt`), Node/Vite (`node_modules/`, `dist/`, `.vite/`), editors, OS, `.worktrees/`, local secrets, and Playwright artifacts. Commit `2feb572`.

- **T0.2 — PASS.** `apps/.gitkeep`, `deploy/.gitkeep`, `scripts/.gitkeep`, `docs/.gitkeep` all present. Commit `8a8693e`.

- **T0.3 — PASS.** `apps/backend/go.mod:1-4` sets module `github.com/jtumidanski/Harbormaster` and `go 1.24.4`, with `require github.com/stretchr/testify v1.9.0`. `go.sum` contains the matching v1.9.0 hashes. `cmd/harbormaster/main.go:1-22` provides `main`+`run` printing the placeholder; `cmd/harbormaster/main_test.go:1-15` uses `require` to assert the placeholder substring. Commit `89ae85e`.

- **T0.4 — PASS.** `apps/backend/.golangci.yml:1-49` enables the plan's linter set (errcheck/govet/staticcheck/revive/gocyclo/unparam/unused/bodyclose/noctx/forbidigo/gosec/ineffassign/misspell/nakedret/prealloc/whitespace), pins go 1.24 / 5m timeout / tests=true, forbids `fmt.Print*` and `panic` via `forbidigo`, and provides per-path exclusions for `_test.go` and `cmd/harbormaster/main.go`. Commit `ab74922`.

- **T0.5 — PASS (with documented deviations).** `apps/frontend/` holds the Vite 5.4.21 scaffold (`package.json`, `index.html`, `src/main.tsx`, `src/App.tsx`, `vite.config.ts`). `tsconfig.json:1-26` is strict with `noUnusedLocals`, `noUnusedParameters`, `exactOptionalPropertyTypes`, `noImplicitOverride`, and `@/*` path alias. `.npmrc:1` contains `save-exact=true`. The orphaned `tsconfig.app.json` was deleted in commit `6cd4729`; `tsconfig.node.json:1-24` is retained per plan and references `vite.config.ts` / `vitest.config.ts` / `tailwind.config.ts`. Smoke test passes (`npm run build` produced `dist/index.html`). Deviations documented below. Commits `ca5da96`, `6cd4729`.

- **T0.6 — PASS (with documented version bump).** `apps/frontend/package.json:17-31` pins all expected base libraries: `@hookform/resolvers@3.9.0`, `@tanstack/react-query@5.100.14` (deviation), `@tanstack/react-virtual@3.8.0` (plan version), `class-variance-authority@0.7.0`, `clsx@2.1.1`, `lucide-react@0.412.0`, `react-hook-form@7.52.0`, `react-router-dom@6.26.0`, `sonner@1.5.0`, `tailwind-merge@2.4.0`, `tailwindcss-animate@1.0.7`, `zod@3.23.8`. Tailwind config (`tailwind.config.ts:1-43`) defines shadcn theme tokens and registers `tailwindcss-animate`. `components.json:1-16` configures shadcn (style=default, baseColor=slate, css=`src/styles/index.css`, alias `@/lib/utils`). `postcss.config.cjs` and `src/lib/utils.ts` present. Commits `9b577ca`, `e004159`.

- **T0.7 — PASS.** `deploy/docker/Dockerfile:1-31` implements 3-stage build: `node:20-alpine` frontend → `golang:1.24-alpine` backend with `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=…"` → `gcr.io/distroless/static-debian12:nonroot`. Frontend output is wired into `internal/server/spa-dist` via `COPY --from=frontend /src/dist ./internal/server/spa-dist`. `.dockerignore:1-13` excludes node_modules/dist/.vite/.git/.worktrees/data/. `docker-compose.yml:1-15` exposes 8080 with `harbormaster-data` volume and a commented-out `~/.mc/config.json:ro` mount. `.env.example:1-46` lists `HARBORMASTER_LISTEN_ADDR`, `HARBORMASTER_DATA_DIR`, session, base path, proxies, upload cap, share TTL, download mode, mc config path, encryption key file, TLS, metrics, OTLP. `apps/backend/internal/server/spa-dist/.gitkeep` exists. `main.go:9` declares `var version = "dev"` per documented deviation. Commit `7d54c21`.

- **T0.8 — PASS (with documented ESLint plugin bumps).** `eslint.config.mjs:1-34` is a flat config with `js.configs.recommended`, `typescript-eslint` recommended-type-checked (project: tsconfig.json + tsconfig.node.json), `react`, `react-hooks`, `jsx-a11y` plugins, `consistent-type-imports`, `_`-prefix unused exception, `react/react-in-jsx-scope: off`. `.prettierrc:1-8` (semi, double quotes, trailing comma, printWidth 100). `.prettierignore:1-4`. `vitest.config.ts:1-13` uses jsdom + setupFiles=`./src/test/setup.ts` + `@` alias. `src/test/setup.ts` imports `@testing-library/jest-dom/vitest`. `src/App.test.tsx:1-10` is the smoke test (`heading /harbormaster/i`). `.github/workflows/pr.yml` lines 9-39 add `frontend-lint`/`frontend-test`/`frontend-build`. ESLint plugin pins bumped per documented deviation. Commit `d8a4adb`.

- **T0.9 — PASS.** `.github/workflows/pr.yml` lines 41-103 add `backend-lint` (golangci-lint v1.59), `backend-test` (race+count=1), `backend-build` (CGO_ENABLED=0), `gitleaks`, `dependency-scan` (Trivy CRITICAL,HIGH, ignore-unfixed=true, trivyignores=`.trivyignore`, exit-code=1), `license-allowlist` (`go-licenses check` with the plan's allowed_licenses list, reading ignore set from `tools/licenses/allowlist.yaml`). `.trivyignore:1-7` exists with the review-policy header. `tools/licenses/allowlist.yaml:1-10` is the empty `ignore: []` shell. All third-party actions are SHA-pinned. Commit `e603eaf`.

- **T0.10 — PASS.** `renovate.json5:1-59` extends `config:base`/`:dependencyDashboard`/`:semanticCommits`, `rangeStrategy: pin`, per-manager groupings with `minimumReleaseAge` (gomod 3d, npm 3d, github-actions 3d patch-automerge, dockerfile 7d, minor 7d, major requires dashboard approval). Auto-merge is explicitly disabled for runtime / security / auth libs (`golang.org/x/crypto`, `minio-go/v7`, `madmin-go/v3`, chi/v5, gorm, glebarez/sqlite, migrate/v4, viper, react, react-dom, react-query). `vulnerabilityAlerts.minimumReleaseAge: 0`. Commit `e5c8586`.

- **T0.11 — PASS.** `.gitleaks.toml:1-17` extends defaults and allow-lists obvious doc/test placeholders (`your_*_key`, `AKIAEXAMPLE`/`AKIATEST`, `correct horse battery staple`) and paths (`docs/.*`, `*.md`, `apps/frontend/src/test/.*`, `apps/backend/.*_test\.go$`). Commit `265a0a0`.

- **T0.12 — PASS.** Worktree `CLAUDE.md` now contains an explicit `## Build & Verification` section listing the concrete backend (`go test -race -count=1`, `go vet`, `golangci-lint run`, `CGO_ENABLED=0 go build`), backend-integration (`HARBORMASTER_INTEGRATION=1`/`-tags=integration`), frontend (`npm ci`/`lint`/`format`/`test`/`build`), container (`docker buildx build --platform linux/amd64,linux/arm64`), and on-demand E2E commands. Commit `c0bc56c`.

- **T0.13 — PASS.** `apps/backend/Makefile:1-22` exposes `lint`, `test`, `test-integration`, `vet`, `build` (writes to `bin/harbormaster`), `tidy`, `run` with the canonical commands matching CLAUDE.md and the CI workflow. Commit `6902085`.

- **T0.14 — PASS.** Full verification matrix re-run during this audit (see Verification commands above) — backend race tests, vet, CGO=0 build, frontend lint/format/vitest/build, multi-stage docker build, container `docker run` producing the placeholder string, and `git tag --list m0-complete` confirm the milestone is reproducible end-to-end. Tag `m0-complete` is at HEAD (`6902085 chore(backend): add Makefile shortcuts`).

**T0.15 (this review)** and **T0.16 (manual demo checkpoint)** are out of scope for code-evidence verification.

### Recorded intentional deviations

- **T0.5 — Vite 5 tsconfig layout.** Vite 5 scaffold writes three tsconfigs (root, `.app`, `.node`). The plan only mentions root + `.node`. The orphaned `tsconfig.app.json` was deleted in follow-up commit `6cd4729`. The root `tsconfig.json` does NOT reference `tsconfig.node.json` because Vite's default `tsconfig.node.json` has `noEmit: true`, which conflicts with `composite: true` required for `tsc -b` project references. `tsconfig.node.json` is therefore not referenced from root but is still present per plan, and was extended in T0.8 to include `vite.config.ts`, `vitest.config.ts`, `tailwind.config.ts` so ESLint typed rules can resolve them.
- **T0.6 — `@tanstack/react-query` 5.100.14.** Installed at 5.100.14 instead of the plan's 5.51.0 because 5.51.0 returns 404 on npm. `@tanstack/react-virtual` is pinned to plan version 3.8.0.
- **T0.7 — `var version = "dev"` in `cmd/harbormaster/main.go`.** Added so the Dockerfile's `-X main.version=` ldflag has a symbol to overwrite. Forward-compatible with T6.10.
- **T0.8 — ESLint plugin pins.** Plan pins for `eslint-plugin-react@7.35.0`, `eslint-plugin-react-hooks@4.6.2`, `eslint-plugin-jsx-a11y@6.9.0` were bumped to the closest minor that supports the ESLint 9 peer-dep (`7.37.5`, `5.0.0`, `6.10.2`).
- **Extra hygiene commits beyond the flat task list.** `67a9e06` (chore: ignore the stray `apps/backend/harbormaster` build output) and `e004159` (fix: pin `@tanstack/react-virtual` to plan version 3.8.0). Both tighten plan adherence rather than diverge from it.

### Other observations

- All third-party GitHub Actions in `pr.yml` are pinned to commit SHAs with version comments (e.g. `actions/checkout@692973e3…  # v4.1.7`), matching the plan's security baseline.
- `renovate.json5` correctly disables auto-merge on the high-blast-radius runtime/security/auth libs called out in plan §M0.
- `.gitignore` already ignores the `apps/backend/harbormaster` build artifact and `harbormaster.db*` / `encryption.key` files, anticipating M1 secrets policy.
- `Dockerfile` runs as `nonroot:nonroot` on `gcr.io/distroless/static-debian12:nonroot`, which is the plan's hardened runtime baseline.
- `eslint.config.mjs` correctly references both `tsconfig.json` and `tsconfig.node.json` in `parserOptions.project`, which is what lets `npm run lint` resolve types for `vite.config.ts` / `vitest.config.ts` / `tailwind.config.ts`.
- One pre-existing build artifact (`apps/backend/harbormaster`) and a build directory (`apps/backend/bin/`) are present in the working tree but gitignored, so they don't pollute the commit graph.

### Verdict

**PASS.** All 14 substantive M0 tasks (T0.1–T0.14) implemented with file-level evidence; full verification matrix re-run during this audit produced zero failures; documented deviations are minor, justified, and forward-compatible. Tag `m0-complete` correctly marks the milestone HEAD. M0 is ready to merge.
