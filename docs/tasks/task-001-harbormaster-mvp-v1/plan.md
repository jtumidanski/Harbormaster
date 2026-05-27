# Harbormaster MVP v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver Harbormaster v1 — a single Go binary embedding a React/TS SPA that provides browser-based admin for a single MinIO deployment, packaged as a multi-arch GHCR image with full CI/CD, deployment manifests, and supply-chain controls.

**Architecture:** One Go binary serves both `/api/v1/*` and the embedded SPA. State lives in a single SQLite file with AES-256-GCM-encrypted credential columns. Domain packages follow the seven-file pattern from `backend-dev-guidelines`. Six stacked sub-branches (M0–M6) land the work incrementally; each milestone is independently demoable.

**Tech Stack:** Go 1.24, chi v5, GORM + glebarez/sqlite (pure Go), golang-migrate + embed.FS, argon2id, AES-256-GCM, zerolog, madmin-go/v3, minio-go/v7, oklog/ulid/v2, viper, testify, testcontainers-go (integration only). Vite + React 18 + TS strict, React Query, React Router, react-hook-form + zod, @tanstack/react-virtual, shadcn/ui, Tailwind, sonner, lucide-react. Vitest + RTL + jsdom. Playwright (on-demand). Container: `gcr.io/distroless/static-debian12:nonroot` with `CGO_ENABLED=0`.

**Companion docs (load on demand per task):** `prd.md`, `design.md`, `api-contracts.md`, `data-model.md`, `risks.md`, `context.md` (this folder).

---

## Conventions used throughout this plan

- Paths are relative to the worktree root: `/Users/tumidanski/source/Harbormaster/.worktrees/task-001-harbormaster-mvp-v1/`.
- Backend tests use `testing` + `github.com/stretchr/testify/require`. Where a test body is shown, the package import block is implied (`testing`, `testify/require`, and what the body uses).
- Frontend tests use Vitest with React Testing Library. `describe`/`it`/`expect` are from `vitest`; `render`/`screen` from `@testing-library/react`.
- Run commands assume cwd = worktree root unless prefixed with `cd <subdir> &&`.
- "Verify" steps state the expected outcome. If a verify step fails, stop and fix before the next task.
- Each task ends with a commit. Use the exact message shown.
- Domain packages follow the seven-file pattern: `model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `administrator.go`, `resource.go`, `rest.go`. When this plan says "scaffold the seven-file pattern for X with [these fields]," the engineer reads `.claude/skills/backend-dev-guidelines/resources/file-responsibilities.md` and produces all eight files (plus tests) for that package. Each file's content is constrained by the fields/signatures the task lists.
- For brevity, `[skill scaffold]` in a Files block means: produce the standard eight files (seven canonical + test files) following the linked skill resource. The task lists only the *deviations and domain specifics* — never the boilerplate.

---

## Task index

- **M0 — Repo scaffolding & CI baseline** (T0.1 – T0.16)
- **M1 — Backend platform** (T1.1 – T1.18)
- **M2 — Setup wizard + auth + connection** (T2.1 – T2.18)
- **M3 — Buckets + objects + lifecycle** (T3.1 – T3.30)
- **M4 — Users + service accounts + policies** (T4.1 – T4.14)
- **M5 — Dashboard + activity** (T5.1 – T5.10)
- **M6 — Deployment, CI/CD, supply chain** (T6.1 – T6.14)

After each milestone: run `superpowers:requesting-code-review` and address findings before merging into `task-001-harbormaster-mvp-v1`.

---

## Milestone M0 — Repo scaffolding & CI baseline

Goal: produce a green CI on a repo that builds an empty container serving a placeholder. No application logic yet.

### Task T0.1: Top-level repo skeleton + LICENSE + README boilerplate

**Files:**
- Create: `LICENSE` (full AGPL-3.0 text from https://www.gnu.org/licenses/agpl-3.0.txt)
- Create: `README.md`
- Create: `.editorconfig`
- Create: `.gitignore`
- Modify: `CLAUDE.md` (update Build & Verification section once stack is in)

- [ ] **Step 1: Create LICENSE**

Download AGPL-3.0 text verbatim (no modifications). Save as `LICENSE` at the worktree root.

- [ ] **Step 2: Create `.editorconfig`**

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
indent_style = space
indent_size = 2
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab
indent_size = 4

[Makefile]
indent_style = tab
```

- [ ] **Step 3: Create `.gitignore`**

```gitignore
# Go
apps/backend/bin/
apps/backend/dist/
*.test
*.out
coverage.txt

# Node / Vite
apps/frontend/node_modules/
apps/frontend/dist/
apps/frontend/dist-ssr/
apps/frontend/.vite/

# Editors
.idea/
.vscode/*
!.vscode/extensions.json
!.vscode/settings.json.example

# OS
.DS_Store
Thumbs.db

# Worktree fallback (we live inside one)
.worktrees/

# Local secrets and data
*.env
*.env.local
data/
harbormaster.db
harbormaster.db-wal
harbormaster.db-shm
encryption.key

# Test artifacts
apps/frontend/playwright-report/
apps/frontend/test-results/
```

- [ ] **Step 4: Create `README.md`** (boilerplate; expanded in M6)

```markdown
# Harbormaster

> Self-hosted MinIO admin UI for homelab and small-cluster operators.

**Status:** in development — see `docs/tasks/task-001-harbormaster-mvp-v1/prd.md`.

## License

Copyright (C) 2026 Harbormaster contributors.

Harbormaster is free software: you can redistribute it and/or modify it under the
terms of the GNU Affero General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version. See `LICENSE` for the full text.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public License
for more details.
```

- [ ] **Step 5: Verify**

Run: `ls LICENSE README.md .editorconfig .gitignore`
Expected: all four files exist.
Run: `head -1 LICENSE`
Expected: `                    GNU AFFERO GENERAL PUBLIC LICENSE`

- [ ] **Step 6: Commit**

```bash
git add LICENSE README.md .editorconfig .gitignore
git commit -m "chore: bootstrap repo with AGPL-3.0 license, README boilerplate, editor config"
```

---

### Task T0.2: Directory shell for apps/, deploy/, docs/, scripts/

**Files:**
- Create: `apps/.gitkeep`, `deploy/.gitkeep`, `scripts/.gitkeep`, `docs/.gitkeep`

(All directories will be populated by later tasks; we commit the empty shell now so subsequent commits are diff-clean.)

- [ ] **Step 1: Create directory placeholders**

```bash
mkdir -p apps/backend apps/frontend deploy/docker deploy/kubernetes scripts docs/architecture docs/operator
touch apps/.gitkeep deploy/.gitkeep scripts/.gitkeep docs/.gitkeep
```

- [ ] **Step 2: Verify**

Run: `find apps deploy scripts docs -type d`
Expected: `apps`, `apps/backend`, `apps/frontend`, `deploy`, `deploy/docker`, `deploy/kubernetes`, `scripts`, `docs`, `docs/architecture`, `docs/operator`.

- [ ] **Step 3: Commit**

```bash
git add apps deploy scripts docs
git commit -m "chore: create top-level apps/deploy/scripts/docs directory shell"
```

---

### Task T0.3: Backend Go module initialization

**Files:**
- Create: `apps/backend/go.mod` (via `go mod init`)
- Create: `apps/backend/go.sum` (generated)
- Create: `apps/backend/cmd/harbormaster/main.go`
- Create: `apps/backend/cmd/harbormaster/main_test.go`

- [ ] **Step 1: Initialize the module**

```bash
cd apps/backend && go mod init github.com/jtumidanski/Harbormaster
```

- [ ] **Step 2: Write a failing test in `apps/backend/cmd/harbormaster/main_test.go`**

```go
package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunPrintsHello(t *testing.T) {
	var out bytes.Buffer
	err := run(&out, []string{"harbormaster"})
	require.NoError(t, err)
	require.Contains(t, out.String(), "harbormaster placeholder")
}
```

- [ ] **Step 3: Add testify dependency**

```bash
cd apps/backend && go get github.com/stretchr/testify@v1.9.0
```

- [ ] **Step 4: Run test — expect FAIL**

```bash
cd apps/backend && go test ./cmd/harbormaster/...
```
Expected: compilation failure (no `run` function yet).

- [ ] **Step 5: Implement `apps/backend/cmd/harbormaster/main.go`**

```go
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Stdout, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out io.Writer, _ []string) error {
	_, err := fmt.Fprintln(out, "harbormaster placeholder — M1 will replace this")
	return err
}
```

- [ ] **Step 6: Run test — expect PASS**

```bash
cd apps/backend && go test ./cmd/harbormaster/... -race
```
Expected: `PASS`.

- [ ] **Step 7: Verify build**

```bash
cd apps/backend && go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 8: Commit**

```bash
git add apps/backend/go.mod apps/backend/go.sum apps/backend/cmd
git commit -m "feat(backend): initialize Go module and placeholder cmd/harbormaster"
```

---

### Task T0.4: golangci-lint configuration

**Files:**
- Create: `apps/backend/.golangci.yml`

- [ ] **Step 1: Write `apps/backend/.golangci.yml`**

```yaml
run:
  timeout: 5m
  tests: true
  go: "1.24"

linters:
  disable-all: true
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - gocyclo
    - unparam
    - unused
    - bodyclose
    - noctx
    - forbidigo
    - gosec
    - ineffassign
    - misspell
    - nakedret
    - prealloc
    - whitespace

linters-settings:
  gocyclo:
    min-complexity: 18
  forbidigo:
    forbid:
      - p: "^fmt\\.Print(ln|f)?$"
        msg: "use the logger, not fmt.Print*"
      - p: "^panic$"
        msg: "do not panic in production code; return an error"
  revive:
    rules:
      - name: exported
        severity: warning
  gosec:
    excludes:
      - G115  # acceptable int conversions are reviewed manually

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - forbidigo
        - gosec
        - gocyclo
    - path: cmd/harbormaster/main\.go
      linters:
        - forbidigo  # main may print fatal errors via fmt.Fprintln(os.Stderr, …)
```

- [ ] **Step 2: Install golangci-lint locally (engineer's choice; v1.59+ recommended) and run**

```bash
cd apps/backend && golangci-lint run
```
Expected: clean (the placeholder main.go has `fmt.Fprintln` to stderr which is allow-listed; no other issues).

- [ ] **Step 3: Commit**

```bash
git add apps/backend/.golangci.yml
git commit -m "chore(backend): add golangci-lint config"
```

---

### Task T0.5: Frontend Vite + React + TS scaffold

**Files:**
- Create: `apps/frontend/package.json`
- Create: `apps/frontend/tsconfig.json`
- Create: `apps/frontend/tsconfig.node.json`
- Create: `apps/frontend/vite.config.ts`
- Create: `apps/frontend/index.html`
- Create: `apps/frontend/src/main.tsx`
- Create: `apps/frontend/src/App.tsx`
- Create: `apps/frontend/src/vite-env.d.ts`
- Create: `apps/frontend/.npmrc` (`save-exact=true`)

- [ ] **Step 1: Initialize via Vite template**

```bash
cd apps && npm create vite@5 frontend -- --template react-ts
```

- [ ] **Step 2: Pin save-exact and remove the demo content**

Add `apps/frontend/.npmrc`:
```
save-exact=true
```

Replace `apps/frontend/src/App.tsx` with:

```tsx
export default function App() {
  return (
    <main className="placeholder">
      <h1>Harbormaster</h1>
      <p>Setup wizard arrives in M2.</p>
    </main>
  );
}
```

Replace `apps/frontend/src/main.tsx` with:

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

Delete `apps/frontend/src/App.css`, `apps/frontend/src/index.css`, and `apps/frontend/src/assets/*`.

- [ ] **Step 3: Enforce strict TS in `apps/frontend/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "forceConsistentCasingInFileNames": true,
    "exactOptionalPropertyTypes": true,
    "noImplicitOverride": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src", "e2e"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

- [ ] **Step 4: Verify build**

```bash
cd apps/frontend && npm install && npm run build
```
Expected: `dist/index.html` produced, no errors.

- [ ] **Step 5: Commit**

```bash
git add apps/frontend
git commit -m "feat(frontend): initialize Vite + React + TS strict scaffold"
```

---

### Task T0.6: Tailwind + shadcn/ui + base libraries

**Files:**
- Modify: `apps/frontend/package.json` (add deps)
- Create: `apps/frontend/tailwind.config.ts`
- Create: `apps/frontend/postcss.config.cjs`
- Create: `apps/frontend/src/styles/index.css`
- Modify: `apps/frontend/src/main.tsx` (import styles)
- Create: `apps/frontend/components.json` (shadcn config)
- Create: `apps/frontend/src/lib/utils.ts`

- [ ] **Step 1: Install dependencies**

```bash
cd apps/frontend && npm install \
  react@18.3.1 react-dom@18.3.1 \
  @tanstack/react-query@5.51.0 \
  @tanstack/react-virtual@3.8.0 \
  react-router-dom@6.26.0 \
  react-hook-form@7.52.0 \
  zod@3.23.8 \
  @hookform/resolvers@3.9.0 \
  sonner@1.5.0 \
  lucide-react@0.412.0 \
  class-variance-authority@0.7.0 \
  clsx@2.1.1 \
  tailwind-merge@2.4.0 \
  tailwindcss-animate@1.0.7
```

```bash
cd apps/frontend && npm install -D \
  tailwindcss@3.4.7 \
  postcss@8.4.39 \
  autoprefixer@10.4.19 \
  @types/node@20.14.0
```

- [ ] **Step 2: Initialize Tailwind config**

`apps/frontend/postcss.config.cjs`:
```js
module.exports = {
  plugins: { tailwindcss: {}, autoprefixer: {} },
};
```

`apps/frontend/tailwind.config.ts`:
```ts
import type { Config } from "tailwindcss";
import animate from "tailwindcss-animate";

const config: Config = {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    container: {
      center: true,
      padding: "2rem",
      screens: { "2xl": "1400px" },
    },
    extend: {
      colors: {
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        primary: { DEFAULT: "hsl(var(--primary))", foreground: "hsl(var(--primary-foreground))" },
        secondary: { DEFAULT: "hsl(var(--secondary))", foreground: "hsl(var(--secondary-foreground))" },
        destructive: { DEFAULT: "hsl(var(--destructive))", foreground: "hsl(var(--destructive-foreground))" },
        muted: { DEFAULT: "hsl(var(--muted))", foreground: "hsl(var(--muted-foreground))" },
        accent: { DEFAULT: "hsl(var(--accent))", foreground: "hsl(var(--accent-foreground))" },
        card: { DEFAULT: "hsl(var(--card))", foreground: "hsl(var(--card-foreground))" },
      },
      borderRadius: { lg: "var(--radius)", md: "calc(var(--radius) - 2px)", sm: "calc(var(--radius) - 4px)" },
    },
  },
  plugins: [animate],
};

export default config;
```

`apps/frontend/src/styles/index.css`:
```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 222.2 84% 4.9%;
    --card: 0 0% 100%;
    --card-foreground: 222.2 84% 4.9%;
    --primary: 222.2 47.4% 11.2%;
    --primary-foreground: 210 40% 98%;
    --secondary: 210 40% 96.1%;
    --secondary-foreground: 222.2 47.4% 11.2%;
    --muted: 210 40% 96.1%;
    --muted-foreground: 215.4 16.3% 46.9%;
    --accent: 210 40% 96.1%;
    --accent-foreground: 222.2 47.4% 11.2%;
    --destructive: 0 84.2% 60.2%;
    --destructive-foreground: 210 40% 98%;
    --border: 214.3 31.8% 91.4%;
    --input: 214.3 31.8% 91.4%;
    --ring: 222.2 84% 4.9%;
    --radius: 0.5rem;
  }

  .dark {
    --background: 222.2 84% 4.9%;
    --foreground: 210 40% 98%;
    --card: 222.2 84% 4.9%;
    --card-foreground: 210 40% 98%;
    --primary: 210 40% 98%;
    --primary-foreground: 222.2 47.4% 11.2%;
    --secondary: 217.2 32.6% 17.5%;
    --secondary-foreground: 210 40% 98%;
    --muted: 217.2 32.6% 17.5%;
    --muted-foreground: 215 20.2% 65.1%;
    --accent: 217.2 32.6% 17.5%;
    --accent-foreground: 210 40% 98%;
    --destructive: 0 62.8% 30.6%;
    --destructive-foreground: 210 40% 98%;
    --border: 217.2 32.6% 17.5%;
    --input: 217.2 32.6% 17.5%;
    --ring: 212.7 26.8% 83.9%;
  }

  body { @apply bg-background text-foreground; }
}
```

Update `apps/frontend/src/main.tsx`:
```tsx
import "./styles/index.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [ ] **Step 3: Add `components.json` for shadcn CLI**

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "default",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "tailwind.config.ts",
    "css": "src/styles/index.css",
    "baseColor": "slate",
    "cssVariables": true
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils"
  }
}
```

- [ ] **Step 4: Add `apps/frontend/src/lib/utils.ts`**

```ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

- [ ] **Step 5: Wire `@/*` alias in Vite**

Update `apps/frontend/vite.config.ts`:
```ts
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  base: process.env.VITE_BASE_PATH ?? "/",
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: { port: 5173 },
  build: { sourcemap: mode !== "production" },
}));
```

- [ ] **Step 6: Verify**

```bash
cd apps/frontend && npm run build
```
Expected: clean build; `dist/assets/` contains hashed JS + CSS.

- [ ] **Step 7: Commit**

```bash
git add apps/frontend
git commit -m "feat(frontend): add Tailwind, shadcn config, base libs (react-query, router, rhf+zod, virtual)"
```

---

### Task T0.7: Multi-stage Dockerfile shell

**Files:**
- Create: `deploy/docker/Dockerfile`
- Create: `deploy/docker/.dockerignore`
- Create: `deploy/docker/docker-compose.yml`
- Create: `deploy/docker/.env.example`

- [ ] **Step 1: Write `deploy/docker/Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1.7

# ---- Stage 1: frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /src
COPY apps/frontend/package.json apps/frontend/package-lock.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

# ---- Stage 2: backend build ----
FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
# Embed the SPA into the binary
COPY --from=frontend /src/dist ./internal/server/spa-dist
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION:-dev}" -o /out/harbormaster ./cmd/harbormaster

# ---- Stage 3: runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/harbormaster /usr/local/bin/harbormaster
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/harbormaster"]
CMD ["serve"]
```

(Note: `internal/server/spa-dist` is a placeholder path; M1 wires the real `embed.FS`. For M0 we add an empty `apps/backend/internal/server/spa-dist/.gitkeep` so the COPY succeeds.)

- [ ] **Step 2: Add empty SPA dist placeholder**

```bash
mkdir -p apps/backend/internal/server/spa-dist && touch apps/backend/internal/server/spa-dist/.gitkeep
```

- [ ] **Step 3: Write `deploy/docker/.dockerignore`**

```
**/node_modules
**/dist
**/.vite
**/coverage
**/*.test
**/*.out
.git
.worktrees
.idea
.vscode
*.env.local
data/
```

- [ ] **Step 4: Write `deploy/docker/docker-compose.yml`** (M2 expands this; M0 just proves it boots)

```yaml
services:
  harbormaster:
    image: harbormaster:dev
    build:
      context: ../..
      dockerfile: deploy/docker/Dockerfile
    ports:
      - "8080:8080"
    environment:
      HARBORMASTER_LISTEN_ADDR: ":8080"
      HARBORMASTER_DATA_DIR: "/var/lib/harbormaster"
    volumes:
      - harbormaster-data:/var/lib/harbormaster
      # Uncomment to enable mc-alias import during first-run setup (read-only).
      # See README "Importing from mc config" for the trade-off.
      # - ${HOME}/.mc/config.json:/root/.mc/config.json:ro

volumes:
  harbormaster-data: {}
```

- [ ] **Step 5: Write `deploy/docker/.env.example`**

```dotenv
# Listening address (default :8080)
HARBORMASTER_LISTEN_ADDR=:8080

# Data directory (SQLite file + encryption key live here)
HARBORMASTER_DATA_DIR=/var/lib/harbormaster

# Optional: explicit DB path. Defaults to <data dir>/harbormaster.db.
# HARBORMASTER_DATABASE_PATH=/var/lib/harbormaster/harbormaster.db

# Logging
HARBORMASTER_LOG_LEVEL=info
HARBORMASTER_LOG_FORMAT=json

# Session lifetime (Go duration; default 8h)
HARBORMASTER_SESSION_TIMEOUT=8h
HARBORMASTER_SESSION_COOKIE_NAME=harbormaster_session

# Reverse-proxy support
HARBORMASTER_BASE_PATH=/
HARBORMASTER_TRUSTED_PROXIES=

# Object upload cap (default 100 MiB)
HARBORMASTER_UPLOAD_MAX_BYTES=104857600

# Share-link maximum TTL (Go duration; default 168h = 7 days)
HARBORMASTER_SHARE_LINK_MAX_TTL=168h

# Download mode: proxy (default) or direct
HARBORMASTER_DOWNLOAD_PROXY_MODE=proxy

# mc config path (only consulted while setup_completed=false)
HARBORMASTER_MC_CONFIG_PATH=/root/.mc/config.json

# Encryption key file (default <data dir>/encryption.key)
# HARBORMASTER_ENCRYPTION_KEY_FILE=

# Optional TLS
# HARBORMASTER_TLS_CERT_FILE=
# HARBORMASTER_TLS_KEY_FILE=

# Optional metrics (Prometheus)
HARBORMASTER_METRICS_ENABLED=false
HARBORMASTER_METRICS_LISTEN_ADDR=:9090

# Optional OTLP tracing
# HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT=
```

- [ ] **Step 6: Verify build**

```bash
docker buildx build --platform linux/amd64 -f deploy/docker/Dockerfile -t harbormaster:m0 .
```
Expected: builds successfully; image size is < 20 MB (static binary + scratch-ish distroless).

```bash
docker run --rm harbormaster:m0
```
Expected: prints `harbormaster placeholder — M1 will replace this` and exits 0.

- [ ] **Step 7: Commit**

```bash
git add deploy/docker apps/backend/internal/server/spa-dist
git commit -m "build(docker): multi-stage Dockerfile (frontend → static Go → distroless) and compose shell"
```

---

### Task T0.8: PR workflow — frontend lint/test/build jobs

**Files:**
- Create: `.github/workflows/pr.yml`
- Create: `apps/frontend/eslint.config.mjs`
- Create: `apps/frontend/.prettierrc`
- Create: `apps/frontend/.prettierignore`
- Modify: `apps/frontend/package.json` (scripts + devDeps)

- [ ] **Step 1: Add ESLint + Prettier + Vitest devDeps**

```bash
cd apps/frontend && npm install -D \
  eslint@9.7.0 \
  @eslint/js@9.7.0 \
  typescript-eslint@8.0.0 \
  eslint-plugin-react@7.35.0 \
  eslint-plugin-react-hooks@4.6.2 \
  eslint-plugin-jsx-a11y@6.9.0 \
  prettier@3.3.3 \
  vitest@2.0.4 \
  @vitest/coverage-v8@2.0.4 \
  @testing-library/react@16.0.0 \
  @testing-library/jest-dom@6.4.6 \
  @testing-library/user-event@14.5.2 \
  jsdom@24.1.0 \
  @vitejs/plugin-react@4.3.1 \
  @types/react@18.3.3 \
  @types/react-dom@18.3.0
```

- [ ] **Step 2: Write `apps/frontend/eslint.config.mjs`**

```js
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import jsxA11y from "eslint-plugin-jsx-a11y";

export default tseslint.config(
  { ignores: ["dist", "node_modules", "playwright-report", "test-results"] },
  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      parserOptions: {
        project: ["./tsconfig.json", "./tsconfig.node.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: { react, "react-hooks": reactHooks, "jsx-a11y": jsxA11y },
    settings: { react: { version: "detect" } },
    rules: {
      ...react.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      ...jsxA11y.configs.recommended.rules,
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      "@typescript-eslint/consistent-type-imports": "error",
    },
  },
);
```

- [ ] **Step 3: Write `apps/frontend/.prettierrc`**

```json
{
  "semi": true,
  "singleQuote": false,
  "trailingComma": "all",
  "printWidth": 100,
  "tabWidth": 2,
  "arrowParens": "always"
}
```

`apps/frontend/.prettierignore`:
```
dist
node_modules
playwright-report
test-results
```

- [ ] **Step 4: Update `apps/frontend/package.json` scripts**

Replace the `scripts` block with:
```json
"scripts": {
  "dev": "vite",
  "build": "tsc -b && vite build",
  "preview": "vite preview",
  "lint": "eslint .",
  "format": "prettier --check .",
  "format:fix": "prettier --write .",
  "test": "vitest run",
  "test:watch": "vitest",
  "test:e2e": "playwright test"
},
```

- [ ] **Step 5: Add a smoke Vitest test in `apps/frontend/src/App.test.tsx`**

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import App from "./App";

describe("App", () => {
  it("renders the placeholder heading", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: /harbormaster/i })).toBeInTheDocument();
  });
});
```

Add `apps/frontend/vitest.config.ts`:
```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  test: {
    environment: "jsdom",
    globals: false,
    setupFiles: ["./src/test/setup.ts"],
  },
});
```

`apps/frontend/src/test/setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
```

- [ ] **Step 6: Write `.github/workflows/pr.yml` (frontend jobs portion; backend jobs added in T0.9)**

```yaml
name: pr
on:
  pull_request:
    branches: [main, "task-*"]

permissions:
  contents: read

jobs:
  frontend-lint:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/frontend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-node@1e60f620b9541d16bece96c5465dc8ee9832be0b # v4.0.3
        with: { node-version: "20", cache: "npm", cache-dependency-path: apps/frontend/package-lock.json }
      - run: npm ci
      - run: npm run lint
      - run: npm run format

  frontend-test:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/frontend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-node@1e60f620b9541d16bece96c5465dc8ee9832be0b # v4.0.3
        with: { node-version: "20", cache: "npm", cache-dependency-path: apps/frontend/package-lock.json }
      - run: npm ci
      - run: npm test

  frontend-build:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/frontend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-node@1e60f620b9541d16bece96c5465dc8ee9832be0b # v4.0.3
        with: { node-version: "20", cache: "npm", cache-dependency-path: apps/frontend/package-lock.json }
      - run: npm ci
      - run: npm run build
```

- [ ] **Step 7: Run locally — expect all green**

```bash
cd apps/frontend && npm run lint && npm run format && npm test && npm run build
```

- [ ] **Step 8: Commit**

```bash
git add apps/frontend .github/workflows/pr.yml
git commit -m "ci(frontend): lint, prettier, vitest, build jobs in PR workflow"
```

---

### Task T0.9: PR workflow — backend lint/test/build + secret/dep scan jobs

**Files:**
- Modify: `.github/workflows/pr.yml`
- Create: `.trivyignore`
- Create: `tools/licenses/allowlist.yaml`
- Modify: `apps/backend/.golangci.yml` (no change yet; already in T0.4)

- [ ] **Step 1: Append backend jobs to `.github/workflows/pr.yml`**

Append (after the `frontend-build` job):

```yaml
  backend-lint:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/backend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
        with: { go-version: "1.24" }
      - uses: golangci/golangci-lint-action@aaa42aa0628b4ae2578232a66b541047968fac86 # v6.1.0
        with: { version: v1.59, working-directory: apps/backend }

  backend-test:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/backend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
        with: { go-version: "1.24" }
      - run: go test -race -count=1 ./...

  backend-build:
    runs-on: ubuntu-24.04
    defaults: { run: { working-directory: apps/backend } }
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
        with: { go-version: "1.24" }
      - run: CGO_ENABLED=0 go build ./...

  gitleaks:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
        with: { fetch-depth: 0 }
      - uses: gitleaks/gitleaks-action@ff98106e4c7b2bc287b24eaf42907196329070c7 # v2.3.6
        env: { GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} }

  dependency-scan:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: aquasecurity/trivy-action@a20de5420d57c4102486cdd9578b45609c99d7eb # v0.24.0
        with:
          scan-type: fs
          scan-ref: "."
          ignore-unfixed: "true"
          severity: "CRITICAL,HIGH"
          format: table
          exit-code: "1"
          trivyignores: ".trivyignore"

  license-allowlist:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
        with: { go-version: "1.24" }
      - run: |
          cd apps/backend
          go install github.com/google/go-licenses@v1.6.0
          go-licenses check ./... \
            --allowed_licenses=Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0,AGPL-3.0,AGPL-3.0-or-later,GPL-3.0,GPL-3.0-or-later \
            --ignore=$(yq -r '.ignore | join(",")' ../../tools/licenses/allowlist.yaml || echo "")
```

- [ ] **Step 2: Write `.trivyignore`**

```
# Trivy ignore file. Review monthly.
# Each entry must include: CVE/GHSA id, reason for ignoring, and a review date.
# CRITICAL CVEs are NEVER allowlisted here.
# HIGH CVEs require a justification comment.
#
# Example:
# CVE-YYYY-XXXX  # reason: not exploitable in our config; reviewed 2026-05-23

```

- [ ] **Step 3: Write `tools/licenses/allowlist.yaml`**

```yaml
# License allow-list overrides.
# Each entry MUST include: module path, license discovered, justification, reviewer, date.
ignore: []
# Example:
# ignore:
#   - module: github.com/example/foo
#     license: MPL-2.0
#     reason: bundled but never linked into the runtime
#     reviewer: jtumidanski
#     date: 2026-05-23
```

- [ ] **Step 4: Verify locally**

```bash
cd apps/backend && go vet ./... && go test -race ./...
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/pr.yml .trivyignore tools/licenses/allowlist.yaml
git commit -m "ci(backend): backend lint/test/build + gitleaks + trivy + license allowlist"
```

---

### Task T0.10: Renovate configuration

**Files:**
- Create: `renovate.json5`

- [ ] **Step 1: Write `renovate.json5`**

```json5
{
  $schema: "https://docs.renovatebot.com/renovate-schema.json",
  extends: ["config:base", ":dependencyDashboard", ":semanticCommits"],
  rangeStrategy: "pin",
  packageRules: [
    {
      matchManagers: ["gomod"],
      groupName: "go modules",
      minimumReleaseAge: "3 days",
    },
    {
      matchManagers: ["npm"],
      groupName: "npm dependencies",
      minimumReleaseAge: "3 days",
    },
    {
      matchManagers: ["github-actions"],
      groupName: "github actions",
      minimumReleaseAge: "3 days",
      automerge: true,
      automergeType: "pr",
      matchUpdateTypes: ["patch"],
    },
    {
      matchManagers: ["dockerfile"],
      groupName: "docker base images",
      minimumReleaseAge: "7 days",
    },
    {
      matchUpdateTypes: ["minor"],
      minimumReleaseAge: "7 days",
    },
    {
      matchUpdateTypes: ["major"],
      dependencyDashboardApproval: true,
    },
    // Never auto-merge runtime / security / auth libs even on patch
    {
      matchPackageNames: [
        "golang.org/x/crypto",
        "github.com/minio/minio-go/v7",
        "github.com/minio/madmin-go/v3",
        "github.com/go-chi/chi/v5",
        "gorm.io/gorm",
        "github.com/glebarez/sqlite",
        "github.com/golang-migrate/migrate/v4",
        "github.com/spf13/viper",
        "react",
        "react-dom",
        "@tanstack/react-query",
      ],
      automerge: false,
    },
  ],
  vulnerabilityAlerts: {
    labels: ["security"],
    minimumReleaseAge: "0",
  },
}
```

- [ ] **Step 2: Commit**

```bash
git add renovate.json5
git commit -m "ci: renovate config with minimum release ages and groupings"
```

---

### Task T0.11: Gitleaks config

**Files:**
- Create: `.gitleaks.toml`

- [ ] **Step 1: Write `.gitleaks.toml`**

```toml
title = "Harbormaster gitleaks config"

[extend]
useDefault = true

[allowlist]
description = "Allow obvious placeholders in docs/tests"
regexes = [
  '''(?i)(your[_-]?(access|secret)[_-]?key|AKIA(?:EXAMPLE|TEST))''',
  '''(?i)correct horse battery staple''',
]
paths = [
  '''docs/.*''',
  '''.*\.md$''',
  '''apps/frontend/src/test/.*''',
  '''apps/backend/.*_test\.go$''',
]
```

- [ ] **Step 2: Commit**

```bash
git add .gitleaks.toml
git commit -m "ci: gitleaks config with doc/test allowlist"
```

---

### Task T0.12: Update CLAUDE.md Build & Verification

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Replace the placeholder Build & Verification section**

Edit `CLAUDE.md` — find the heading `## Build & Verification` and replace its body with:

```markdown
## Build & Verification

A branch is "done" only when all of these are clean:

**Backend** (cwd = `apps/backend`):
- `go test -race -count=1 ./...`
- `go vet ./...`
- `golangci-lint run`
- `CGO_ENABLED=0 go build ./...`

**Backend integration tests** (Docker required, on-demand):
- `HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...`

**Frontend** (cwd = `apps/frontend`):
- `npm ci`
- `npm run lint`
- `npm run format`
- `npm test`
- `npm run build`

**Container** (cwd = worktree root):
- `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .`

**E2E** (on-demand, not per-PR; requires Docker Compose stack):
- `cd apps/frontend && npm run test:e2e`
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): record concrete build & verification commands"
```

---

### Task T0.13: Backend Makefile convenience

**Files:**
- Create: `apps/backend/Makefile`

- [ ] **Step 1: Write `apps/backend/Makefile`**

```make
.PHONY: lint test test-integration build vet tidy run

lint:
	golangci-lint run

test:
	go test -race -count=1 ./...

test-integration:
	HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build -o bin/harbormaster ./cmd/harbormaster

tidy:
	go mod tidy

run: build
	./bin/harbormaster serve
```

- [ ] **Step 2: Commit**

```bash
git add apps/backend/Makefile
git commit -m "chore(backend): add Makefile shortcuts (lint/test/build/run)"
```

---

### Task T0.14: Smoke test the entire M0 pipeline

- [ ] **Step 1: Run the full local verification matrix**

```bash
cd apps/backend && go test -race ./... && go vet ./... && golangci-lint run && CGO_ENABLED=0 go build ./...
cd ../frontend && npm run lint && npm run format && npm test && npm run build
cd ../.. && docker buildx build --platform linux/amd64 -f deploy/docker/Dockerfile -t harbormaster:m0 .
docker run --rm harbormaster:m0
```
Expected: all commands exit 0; container prints the placeholder.

- [ ] **Step 2: Push branch and open PR (manual)**

Per CLAUDE.md "Code Review Before PR," do not yet open the PR. Instead run `superpowers:requesting-code-review` over M0 first.

- [ ] **Step 3: Tag M0 internally**

```bash
git tag m0-complete
```

(The tag is a local checkpoint for stack rebases; it is not pushed.)

---

### Task T0.15: Run M0 code review

- [ ] **Step 1: Invoke `superpowers:requesting-code-review` on the M0 work**

The review dispatches `backend-guidelines-reviewer` + `frontend-guidelines-reviewer` + `plan-adherence-reviewer` in parallel. Findings land in `docs/tasks/task-001-harbormaster-mvp-v1/audit.md`.

- [ ] **Step 2: Address findings**

Fix anything flagged FAIL. Re-run the verification matrix from T0.14. Commit fixes with `fix(<scope>): <description>` per Conventional Commits.

---

### Task T0.16: M0 demo checkpoint

- [ ] **Step 1: Confirm demoability**

A reviewer should be able to:
1. Clone the worktree.
2. Run `docker buildx build -f deploy/docker/Dockerfile -t hm:demo .`
3. Run `docker run --rm hm:demo`.
4. See the placeholder output.
5. Open `.github/workflows/pr.yml`, see all seven jobs (frontend lint/test/build, backend lint/test/build, gitleaks, dependency-scan, license-allowlist).

- [ ] **Step 2: Commit any final M0 docs polish if needed**

If no further work: proceed to M1.

---

## Milestone M1 — Backend platform

Goal: produce a binary that boots, runs migrations, serves `/healthz` and `/readyz`, rejects every `/api/v1/*` request with `401 unauthenticated`, and exposes CLI subcommands for admin recovery. No domain features yet — but every cross-cutting library is in place.

### Task T1.1: Add core backend dependencies

**Files:**
- Modify: `apps/backend/go.mod`
- Modify: `apps/backend/go.sum`

- [ ] **Step 1: Add deps via `go get`**

```bash
cd apps/backend && go get \
  github.com/go-chi/chi/v5@v5.1.0 \
  github.com/spf13/viper@v1.19.0 \
  github.com/spf13/cobra@v1.8.1 \
  github.com/rs/zerolog@v1.33.0 \
  gorm.io/gorm@v1.25.11 \
  github.com/glebarez/sqlite@v1.11.0 \
  github.com/golang-migrate/migrate/v4@v4.17.1 \
  github.com/oklog/ulid/v2@v2.1.0 \
  github.com/google/uuid@v1.6.0 \
  golang.org/x/crypto@v0.25.0 \
  github.com/prometheus/client_golang@v1.20.0 \
  go.opentelemetry.io/otel@v1.28.0 \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.28.0 \
  go.opentelemetry.io/otel/sdk@v1.28.0 \
  github.com/minio/madmin-go/v3@v3.0.66 \
  github.com/minio/minio-go/v7@v7.0.74
```

(The `migrate/v4` `iofs` source and `sqlite3` database driver are sub-packages of the same module; they're pulled in by import.)

- [ ] **Step 2: Run `go mod tidy`**

```bash
cd apps/backend && go mod tidy
```

- [ ] **Step 3: Verify it still builds**

```bash
cd apps/backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add apps/backend/go.mod apps/backend/go.sum
git commit -m "feat(backend): pin core dependencies (chi, viper, zerolog, gorm, glebarez-sqlite, migrate, minio)"
```

---

### Task T1.2: `internal/config` — env/file/defaults loader

**Files:**
- Create: `apps/backend/internal/config/config.go`
- Create: `apps/backend/internal/config/config_test.go`

- [ ] **Step 1: Write failing test in `config_test.go`**

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.ListenAddr)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "json", cfg.LogFormat)
	require.Equal(t, 8*time.Hour, cfg.SessionTimeout)
	require.Equal(t, "harbormaster_session", cfg.SessionCookieName)
	require.Equal(t, "/", cfg.BasePath)
	require.Equal(t, int64(104857600), cfg.UploadMaxBytes)
	require.Equal(t, 168*time.Hour, cfg.ShareLinkMaxTTL)
	require.Equal(t, "proxy", cfg.DownloadProxyMode)
	require.False(t, cfg.MetricsEnabled)
}

func TestLoadOverridesFromEnv(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_LISTEN_ADDR", ":9090")
	t.Setenv("HARBORMASTER_BASE_PATH", "/harbormaster/")
	t.Setenv("HARBORMASTER_DOWNLOAD_PROXY_MODE", "direct")
	t.Setenv("HARBORMASTER_UPLOAD_MAX_BYTES", "52428800")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.ListenAddr)
	require.Equal(t, "/harbormaster", cfg.BasePath, "trailing slash should normalize off")
	require.Equal(t, "direct", cfg.DownloadProxyMode)
	require.Equal(t, int64(52428800), cfg.UploadMaxBytes)
}

func TestLoadRejectsInvalidBasePath(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_BASE_PATH", "harbormaster")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_BASE_PATH must begin with /")
}

func TestLoadRejectsInvalidDownloadMode(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_DOWNLOAD_PROXY_MODE", "stream")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_DOWNLOAD_PROXY_MODE")
}

func TestLoadRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_LOG_FORMAT", "logfmt")
	_, err := Load()
	require.ErrorContains(t, err, "HARBORMASTER_LOG_FORMAT")
}

func TestLoadRejectsTLSPartial(t *testing.T) {
	t.Setenv("HARBORMASTER_DATA_DIR", t.TempDir())
	t.Setenv("HARBORMASTER_TLS_CERT_FILE", "/tmp/cert.pem")
	_, err := Load()
	require.ErrorContains(t, err, "TLS_CERT_FILE and HARBORMASTER_TLS_KEY_FILE must both be set or both be empty")
}
```

- [ ] **Step 2: Run test — expect FAIL** (`go test ./internal/config/...`)

- [ ] **Step 3: Implement `apps/backend/internal/config/config.go`**

```go
// Package config loads Harbormaster configuration from env vars, an optional
// config file, and defaults. The returned Config value is immutable.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the resolved Harbormaster configuration. Pass by value.
type Config struct {
	ListenAddr                string
	DataDir                   string
	DatabasePath              string
	LogLevel                  string
	LogFormat                 string
	SessionTimeout            time.Duration
	SessionCookieName         string
	BasePath                  string
	TrustedProxies            []string
	UploadMaxBytes            int64
	ShareLinkMaxTTL           time.Duration
	DownloadProxyMode         string
	McConfigPath              string
	TLSCertFile               string
	TLSKeyFile                string
	EncryptionKeyFile         string
	MetricsEnabled            bool
	MetricsListenAddr         string
	OTELExporterOTLPEndpoint  string
	AuditRetention            time.Duration
}

// Load reads configuration in priority order: env (HARBORMASTER_*) > file > defaults.
func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("HARBORMASTER")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	defaults(v)

	if p := v.GetString("CONFIG"); p != "" {
		v.SetConfigFile(p)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("reading config file %s: %w", p, err)
		}
	}

	cfg := Config{
		ListenAddr:               v.GetString("LISTEN_ADDR"),
		DataDir:                  v.GetString("DATA_DIR"),
		DatabasePath:             v.GetString("DATABASE_PATH"),
		LogLevel:                 v.GetString("LOG_LEVEL"),
		LogFormat:                v.GetString("LOG_FORMAT"),
		SessionTimeout:           v.GetDuration("SESSION_TIMEOUT"),
		SessionCookieName:        v.GetString("SESSION_COOKIE_NAME"),
		BasePath:                 normalizeBasePath(v.GetString("BASE_PATH")),
		TrustedProxies:           splitCSV(v.GetString("TRUSTED_PROXIES")),
		UploadMaxBytes:           v.GetInt64("UPLOAD_MAX_BYTES"),
		ShareLinkMaxTTL:          v.GetDuration("SHARE_LINK_MAX_TTL"),
		DownloadProxyMode:        v.GetString("DOWNLOAD_PROXY_MODE"),
		McConfigPath:             v.GetString("MC_CONFIG_PATH"),
		TLSCertFile:              v.GetString("TLS_CERT_FILE"),
		TLSKeyFile:               v.GetString("TLS_KEY_FILE"),
		EncryptionKeyFile:        v.GetString("ENCRYPTION_KEY_FILE"),
		MetricsEnabled:           v.GetBool("METRICS_ENABLED"),
		MetricsListenAddr:        v.GetString("METRICS_LISTEN_ADDR"),
		OTELExporterOTLPEndpoint: v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		AuditRetention:           v.GetDuration("AUDIT_RETENTION"),
	}

	if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "harbormaster.db")
	}
	if cfg.EncryptionKeyFile == "" {
		cfg.EncryptionKeyFile = filepath.Join(cfg.DataDir, "encryption.key")
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults(v *viper.Viper) {
	v.SetDefault("LISTEN_ADDR", ":8080")
	v.SetDefault("DATA_DIR", "/var/lib/harbormaster")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")
	v.SetDefault("SESSION_TIMEOUT", 8*time.Hour)
	v.SetDefault("SESSION_COOKIE_NAME", "harbormaster_session")
	v.SetDefault("BASE_PATH", "/")
	v.SetDefault("UPLOAD_MAX_BYTES", int64(104857600))
	v.SetDefault("SHARE_LINK_MAX_TTL", 168*time.Hour)
	v.SetDefault("DOWNLOAD_PROXY_MODE", "proxy")
	v.SetDefault("MC_CONFIG_PATH", "/root/.mc/config.json")
	v.SetDefault("METRICS_ENABLED", false)
	v.SetDefault("METRICS_LISTEN_ADDR", ":9090")
	v.SetDefault("AUDIT_RETENTION", 90*24*time.Hour)
}

func validate(c Config) error {
	if !strings.HasPrefix(c.BasePath, "/") {
		return errors.New("HARBORMASTER_BASE_PATH must begin with /")
	}
	if c.LogFormat != "json" && c.LogFormat != "console" {
		return fmt.Errorf("HARBORMASTER_LOG_FORMAT must be json or console (got %q)", c.LogFormat)
	}
	if c.DownloadProxyMode != "proxy" && c.DownloadProxyMode != "direct" {
		return fmt.Errorf("HARBORMASTER_DOWNLOAD_PROXY_MODE must be proxy or direct (got %q)", c.DownloadProxyMode)
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return errors.New("HARBORMASTER_TLS_CERT_FILE and HARBORMASTER_TLS_KEY_FILE must both be set or both be empty")
	}
	if c.UploadMaxBytes <= 0 {
		return errors.New("HARBORMASTER_UPLOAD_MAX_BYTES must be positive")
	}
	return nil
}

func normalizeBasePath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return strings.TrimRight(p, "/")
	}
	return p
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd apps/backend && go test ./internal/config/... -race
```

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/config
git commit -m "feat(backend/config): viper-backed loader with env/file/default precedence and validation"
```

---

### Task T1.3: `internal/observability` — zerolog setup + request ID + log wrapper

**Files:**
- Create: `apps/backend/internal/observability/log/log.go`
- Create: `apps/backend/internal/observability/log/log_test.go`
- Create: `apps/backend/internal/observability/middleware.go`

- [ ] **Step 1: Write failing test in `log_test.go`**

```go
package log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewJSONWriter(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewWith("info", "json", &buf)
	require.NoError(t, err)
	logger.Info().Str("key", "value").Msg("hello")
	require.Contains(t, buf.String(), `"key":"value"`)
	require.Contains(t, buf.String(), `"message":"hello"`)
	require.Contains(t, buf.String(), `"level":"info"`)
}

func TestCtxRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := NewWith("debug", "json", &buf)
	ctx := WithLogger(context.Background(), logger)
	FromCtx(ctx).Info().Msg("ping")
	require.True(t, strings.Contains(buf.String(), `"message":"ping"`))
}
```

- [ ] **Step 2: Run test — expect FAIL**

- [ ] **Step 3: Implement `apps/backend/internal/observability/log/log.go`**

```go
// Package log wraps zerolog with a context-bound logger usable across the codebase.
// All emit sites should go through this package (or via FromCtx) so log format
// and level remain consistent and so secret-scrubbing lint can statically locate
// every log call.
package log

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog"
)

type ctxKey struct{}

// NewWith builds a logger from level ("debug" | "info" | "warn" | "error") and
// format ("json" | "console") writing to w. Caller owns w's lifecycle.
func NewWith(level, format string, w io.Writer) (zerolog.Logger, error) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Nop(), fmt.Errorf("invalid log level %q: %w", level, err)
	}
	var out io.Writer = w
	if format == "console" {
		out = zerolog.ConsoleWriter{Out: w}
	}
	return zerolog.New(out).Level(lvl).With().Timestamp().Caller().Logger(), nil
}

// New is a convenience that writes to stderr.
func New(level, format string) (zerolog.Logger, error) {
	return NewWith(level, format, os.Stderr)
}

// WithLogger attaches the logger to the context.
func WithLogger(ctx context.Context, l zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromCtx returns the logger attached to ctx, or a Nop logger if none.
func FromCtx(ctx context.Context) zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}
```

- [ ] **Step 4: Write `apps/backend/internal/observability/middleware.go`**

```go
package observability

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	hmlog "github.com/jtumidanski/Harbormaster/internal/observability/log"
)

// Logger emits one structured log line per HTTP request.
func Logger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			rid := chimw.GetReqID(r.Context())
			l := base.With().Str("request_id", rid).Logger()
			ctx := hmlog.WithLogger(r.Context(), l)
			start := chimw.GetReqID // sentinel only — actual timing below
			_ = start
			t := chimw.NewWrapResponseWriter
			_ = t
			startTime := nowFn()
			next.ServeHTTP(ww, r.WithContext(ctx))
			l.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("latency", nowFn().Sub(startTime)).
				Int("bytes", ww.BytesWritten()).
				Msg("http_request")
		})
	}
}
```

Add the small `nowFn` indirection so tests can freeze time later (defined at file top: `var nowFn = time.Now`). Import `time`.

Final correct version (replace the stub above):

```go
package observability

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	hmlog "github.com/jtumidanski/Harbormaster/internal/observability/log"
)

var nowFn = time.Now

// Logger emits one structured log line per HTTP request.
func Logger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			rid := chimw.GetReqID(r.Context())
			l := base.With().Str("request_id", rid).Logger()
			ctx := hmlog.WithLogger(r.Context(), l)
			start := nowFn()
			next.ServeHTTP(ww, r.WithContext(ctx))
			l.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("latency", nowFn().Sub(start)).
				Int("bytes", ww.BytesWritten()).
				Msg("http_request")
		})
	}
}
```

- [ ] **Step 5: Tests pass + commit**

```bash
cd apps/backend && go test ./internal/observability/... -race
git add apps/backend/internal/observability
git commit -m "feat(backend/observability): zerolog wrapper with ctx-bound logger and request-log middleware"
```

---

### Task T1.4: `internal/db` — SQLite open, PRAGMA, retry-on-busy, migration runner harness

**Files:**
- Create: `apps/backend/internal/db/db.go`
- Create: `apps/backend/internal/db/retry.go`
- Create: `apps/backend/internal/db/db_test.go`
- Create: `apps/backend/migrations/.gitkeep`

- [ ] **Step 1: Write failing test `db_test.go`**

```go
package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
)

func TestOpenCreatesWALMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sql, err := Open(cfg)
	require.NoError(t, err)
	defer sql.Close()
	var mode string
	require.NoError(t, gdb.Raw("PRAGMA journal_mode;").Scan(&mode).Error)
	require.Equal(t, "wal", mode)
	var fk int
	require.NoError(t, gdb.Raw("PRAGMA foreign_keys;").Scan(&fk).Error)
	require.Equal(t, 1, fk)
}

func TestSingleConnectionLimits(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	_, sql, err := Open(cfg)
	require.NoError(t, err)
	defer sql.Close()
	stats := sql.Stats()
	require.Equal(t, 1, stats.MaxOpenConnections)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `apps/backend/internal/db/db.go`**

```go
// Package db opens the SQLite database with PRAGMAs and connection limits
// appropriate for a single-writer workload, and runs forward-only migrations
// at startup.
package db

import (
	"database/sql"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/config"
)

// Open returns a configured *gorm.DB wrapping an open *sql.DB.
// Callers must close the returned *sql.DB on shutdown.
func Open(cfg config.Config) (*gorm.DB, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)",
		cfg.DatabasePath,
	)
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	sdb, err := gdb.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("unwrap sql.DB: %w", err)
	}
	sdb.SetMaxOpenConns(1)
	sdb.SetMaxIdleConns(1)
	sdb.SetConnMaxLifetime(0)

	if err := registerBusyRetry(gdb); err != nil {
		return nil, nil, fmt.Errorf("register busy-retry plugin: %w", err)
	}
	return gdb, sdb, nil
}
```

- [ ] **Step 4: Implement `apps/backend/internal/db/retry.go`**

```go
package db

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// registerBusyRetry installs a GORM callback that retries SQLITE_BUSY on
// write operations with exponential backoff (max 5 attempts, 1s ceiling).
func registerBusyRetry(gdb *gorm.DB) error {
	hook := func(name string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			if tx.Error == nil {
				return
			}
			if !isBusy(tx.Error) {
				return
			}
			backoff := 25 * time.Millisecond
			for i := 0; i < 4; i++ {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > time.Second {
					backoff = time.Second
				}
				tx.Error = nil
				tx.Statement.SQL.Reset()
				tx.Statement.Vars = nil
				switch name {
				case "create":
					tx.Callback().Create().Execute(tx)
				case "update":
					tx.Callback().Update().Execute(tx)
				case "delete":
					tx.Callback().Delete().Execute(tx)
				case "raw":
					tx.Callback().Raw().Execute(tx)
				}
				if tx.Error == nil || !isBusy(tx.Error) {
					return
				}
			}
		}
	}
	for _, n := range []string{"create", "update", "delete", "raw"} {
		if err := gdb.Callback().Create().After("gorm:" + n).Register("hm:busy_retry_"+n, hook(n)); err != nil {
			return err
		}
	}
	return nil
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	var msg string
	msg = err.Error()
	if strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked") {
		return true
	}
	return errors.Is(err, gorm.ErrInvalidTransaction)
}
```

(Note: GORM's plugin signature for retry isn't a literal "before/after" replay — the hook above is illustrative. The executing engineer may simplify this to a wrapper function `WithRetry(ctx, fn)` if GORM's callback model doesn't fit; the user-visible behavior is what matters. Tests in T1.4 cover the steady-state path; a separate task adds a failure-injection test once we have a real write path in M1.)

- [ ] **Step 5: Run tests — expect PASS**

```bash
cd apps/backend && go test ./internal/db/... -race
```

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/db apps/backend/migrations
git commit -m "feat(backend/db): SQLite open with PRAGMAs and single-conn limits"
```

---

### Task T1.5: `internal/crypto` — AES-256-GCM cipher + key loader + fingerprint check

**Files:**
- Create: `apps/backend/internal/crypto/cipher.go`
- Create: `apps/backend/internal/crypto/keyloader.go`
- Create: `apps/backend/internal/crypto/cipher_test.go`
- Create: `apps/backend/internal/crypto/keyloader_test.go`

- [ ] **Step 1: Write failing tests in `cipher_test.go`**

```go
package crypto

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := New(key)
	require.NoError(t, err)
	ct, err := c.Encrypt([]byte("hello world"))
	require.NoError(t, err)
	pt, err := c.Decrypt(ct)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), pt)
}

func TestNonceUniqueness(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := New(key)
	a, _ := c.Encrypt([]byte("x"))
	b, _ := c.Encrypt([]byte("x"))
	require.NotEqual(t, a, b, "same plaintext must produce different ciphertext (random nonce)")
}

func TestRejectShortKey(t *testing.T) {
	_, err := New(make([]byte, 16))
	require.ErrorContains(t, err, "32 bytes")
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `apps/backend/internal/crypto/cipher.go`**

```go
// Package crypto wraps AES-256-GCM symmetric encryption for at-rest credential
// columns. Ciphertext storage envelope: base64( nonce(12B) || ct || tag(16B) ).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Cipher performs AES-256-GCM encryption with random 12-byte nonces.
type Cipher struct {
	aead cipher.AEAD
}

// New constructs a Cipher from a 32-byte key.
func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns a base64-encoded envelope of (nonce || ct || tag).
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ct := c.aead.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Decrypt reverses Encrypt. Returns an error if the envelope is malformed or
// authentication fails.
func (c *Cipher) Decrypt(envelope string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(envelope)
	if err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return nil, errors.New("envelope too short")
	}
	nonce, ct := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return pt, nil
}
```

- [ ] **Step 4: Write key-loader tests in `keyloader_test.go`**

```go
package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadGeneratesKeyOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	key, fp, err := LoadKey(path)
	require.NoError(t, err)
	require.Len(t, key, 32)
	expected := sha256.Sum256(key)
	require.Equal(t, hex.EncodeToString(expected[:]), fp)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadRejectsWorldReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	require.NoError(t, os.WriteFile(path, make([]byte, 32), 0o644))
	_, _, err := LoadKey(path)
	require.ErrorContains(t, err, "world-readable")
}

func TestLoadReturnsExistingKeyAndFingerprint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encryption.key")
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	key, fp, err := LoadKey(path)
	require.NoError(t, err)
	require.Equal(t, raw, key)
	expected := sha256.Sum256(raw)
	require.Equal(t, hex.EncodeToString(expected[:]), fp)
}
```

- [ ] **Step 5: Implement `apps/backend/internal/crypto/keyloader.go`**

```go
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// LoadKey returns (32-byte key, hex SHA-256 fingerprint).
// If the file does not exist, a new key is generated and written with 0600.
// Permission rules: world-readable bits set → fatal error.
// Group-readable bits set → returned alongside the key; caller logs a warning.
func LoadKey(path string) ([]byte, string, error) {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return generate(path)
	case err != nil:
		return nil, "", fmt.Errorf("stat key file: %w", err)
	}
	if info.Mode().Perm()&0o004 != 0 {
		return nil, "", fmt.Errorf("encryption key file %s is world-readable (perm %v); refusing to start", path, info.Mode().Perm())
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read key file: %w", err)
	}
	if len(key) != 32 {
		return nil, "", fmt.Errorf("encryption key file %s is %d bytes; expected 32", path, len(key))
	}
	sum := sha256.Sum256(key)
	return key, hex.EncodeToString(sum[:]), nil
}

func generate(path string) ([]byte, string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, "", fmt.Errorf("read random bytes: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, "", fmt.Errorf("write key file: %w", err)
	}
	sum := sha256.Sum256(key)
	return key, hex.EncodeToString(sum[:]), nil
}
```

- [ ] **Step 6: Run tests — expect PASS**

```bash
cd apps/backend && go test ./internal/crypto/... -race
```

- [ ] **Step 7: Commit**

```bash
git add apps/backend/internal/crypto
git commit -m "feat(backend/crypto): AES-256-GCM cipher + 0600-enforced key loader with SHA-256 fingerprint"
```

---

### Task T1.6: `internal/jsonapi` — hand-rolled encoder/decoder + error envelope

**Files:**
- Create: `apps/backend/internal/jsonapi/encoder.go`
- Create: `apps/backend/internal/jsonapi/decoder.go`
- Create: `apps/backend/internal/jsonapi/errors.go`
- Create: `apps/backend/internal/jsonapi/types.go`
- Create: `apps/backend/internal/jsonapi/encoder_test.go`

- [ ] **Step 1: Write failing tests**

```go
package jsonapi_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

type bucket struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (b bucket) ResourceType() string { return "buckets" }
func (b bucket) ResourceID() string   { return b.Name }

func TestEncodeSingle(t *testing.T) {
	var buf bytes.Buffer
	enc := jsonapi.NewEncoder()
	err := enc.Single(&buf, bucket{Name: "photos", CreatedAt: "2026-05-23T14:00:00Z"}, nil)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	data := got["data"].(map[string]any)
	require.Equal(t, "buckets", data["type"])
	require.Equal(t, "photos", data["id"])
	attrs := data["attributes"].(map[string]any)
	require.Equal(t, "photos", attrs["name"])
}

func TestEncodeCollection(t *testing.T) {
	var buf bytes.Buffer
	enc := jsonapi.NewEncoder()
	items := []jsonapi.Resource{
		bucket{Name: "a", CreatedAt: "t"},
		bucket{Name: "b", CreatedAt: "t"},
	}
	err := enc.Collection(&buf, items, &jsonapi.Meta{Page: &jsonapi.Page{Number: 1, Size: 50, TotalRecords: 2, TotalPages: 1}}, nil)
	require.NoError(t, err)
	require.True(t, strings.Contains(buf.String(), `"total_records":2`))
}

func TestEncodeError(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, jsonapi.WriteError(&buf, jsonapi.Error{
		Status: 422, Code: "invalid_bucket_name", Title: "Invalid bucket name",
		Detail: "must be lowercase", Pointer: "/data/attributes/name",
	}))
	require.Contains(t, buf.String(), `"status":"422"`)
	require.Contains(t, buf.String(), `"pointer":"/data/attributes/name"`)
}

func TestDecodeSingle(t *testing.T) {
	body := `{"data":{"type":"buckets","attributes":{"name":"photos","created_at":"2026-05-23T14:00:00Z"}}}`
	var out bucket
	err := jsonapi.NewDecoder().Single(strings.NewReader(body), &out)
	require.NoError(t, err)
	require.Equal(t, "photos", out.Name)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `apps/backend/internal/jsonapi/types.go`**

```go
// Package jsonapi is a minimal hand-rolled JSON:API encoder/decoder tailored to
// the v1 endpoint surface. It supports single and collection resource documents,
// the errors[] envelope, and simple request decoding into a typed attrs struct.
package jsonapi

// Resource is implemented by every domain model exposed via the JSON:API
// transport. ResourceType returns the canonical type name (plural noun);
// ResourceID returns the string ID used in /<type>/<id> URLs.
type Resource interface {
	ResourceType() string
	ResourceID() string
}

// Meta holds optional metadata such as pagination.
type Meta struct {
	Page *Page `json:"page,omitempty"`
}

// Page describes pagination state.
type Page struct {
	Number       int    `json:"number,omitempty"`
	Size         int    `json:"size,omitempty"`
	TotalRecords int    `json:"total_records"`
	TotalPages   int    `json:"total_pages"`
	NextToken    string `json:"next_token,omitempty"`
}

// Links holds top-level document links.
type Links struct {
	Self string `json:"self,omitempty"`
	Next string `json:"next,omitempty"`
	Prev string `json:"prev,omitempty"`
}
```

- [ ] **Step 4: Implement `apps/backend/internal/jsonapi/encoder.go`**

```go
package jsonapi

import (
	"encoding/json"
	"io"
)

// Encoder writes JSON:API documents.
type Encoder struct{}

// NewEncoder constructs an Encoder.
func NewEncoder() *Encoder { return &Encoder{} }

type resourceDoc struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes any    `json:"attributes"`
}

type singleDoc struct {
	Data  resourceDoc `json:"data"`
	Meta  *Meta       `json:"meta,omitempty"`
	Links *Links      `json:"links,omitempty"`
}

type collectionDoc struct {
	Data  []resourceDoc `json:"data"`
	Meta  *Meta         `json:"meta,omitempty"`
	Links *Links        `json:"links,omitempty"`
}

// Single writes a single-resource JSON:API document.
// The `r` value's serialized JSON form (via encoding/json) becomes `attributes`,
// minus the resource ID (which is excluded by convention by tagging it `json:"-"`
// on the model struct).
func (e *Encoder) Single(w io.Writer, r Resource, links *Links) error {
	doc := singleDoc{Data: resourceDoc{Type: r.ResourceType(), ID: r.ResourceID(), Attributes: r}, Links: links}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

// Collection writes a collection JSON:API document.
func (e *Encoder) Collection(w io.Writer, rs []Resource, meta *Meta, links *Links) error {
	docs := make([]resourceDoc, len(rs))
	for i, r := range rs {
		docs[i] = resourceDoc{Type: r.ResourceType(), ID: r.ResourceID(), Attributes: r}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(collectionDoc{Data: docs, Meta: meta, Links: links})
}
```

- [ ] **Step 5: Implement `apps/backend/internal/jsonapi/decoder.go`**

```go
package jsonapi

import (
	"encoding/json"
	"io"
)

// Decoder reads JSON:API request documents.
type Decoder struct{}

// NewDecoder constructs a Decoder.
func NewDecoder() *Decoder { return &Decoder{} }

type singleRequest struct {
	Data struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Attributes json.RawMessage `json:"attributes"`
	} `json:"data"`
}

// Single decodes `{ "data": { "type": ..., "attributes": {...} } }` into the
// struct pointed to by out by unmarshaling attributes via encoding/json.
func (d *Decoder) Single(r io.Reader, out any) error {
	var req singleRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return err
	}
	return json.Unmarshal(req.Attributes, out)
}
```

- [ ] **Step 6: Implement `apps/backend/internal/jsonapi/errors.go`**

```go
package jsonapi

import (
	"encoding/json"
	"io"
	"strconv"
)

// Error is the JSON:API errors[] item shape used by Harbormaster.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Detail  string `json:"detail,omitempty"`
	Pointer string `json:"-"`
}

type wireError struct {
	Status string         `json:"status"`
	Code   string         `json:"code"`
	Title  string         `json:"title"`
	Detail string         `json:"detail,omitempty"`
	Source *wireErrorSrc  `json:"source,omitempty"`
}

type wireErrorSrc struct {
	Pointer string `json:"pointer,omitempty"`
}

type errorDoc struct {
	Errors []wireError `json:"errors"`
}

// WriteError writes one or more errors[] entries to w.
func WriteError(w io.Writer, errs ...Error) error {
	wires := make([]wireError, len(errs))
	for i, e := range errs {
		wires[i] = wireError{
			Status: strconv.Itoa(e.Status),
			Code:   e.Code,
			Title:  e.Title,
			Detail: e.Detail,
		}
		if e.Pointer != "" {
			wires[i].Source = &wireErrorSrc{Pointer: e.Pointer}
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(errorDoc{Errors: wires})
}
```

- [ ] **Step 7: Run tests — expect PASS**

```bash
cd apps/backend && go test ./internal/jsonapi/... -race
```

- [ ] **Step 8: Commit**

```bash
git add apps/backend/internal/jsonapi
git commit -m "feat(backend/jsonapi): hand-rolled encoder/decoder + errors[] envelope"
```

---

### Task T1.7: `internal/apierror` + `internal/sse`

**Files:**
- Create: `apps/backend/internal/apierror/apierror.go`
- Create: `apps/backend/internal/apierror/apierror_test.go`
- Create: `apps/backend/internal/sse/writer.go`
- Create: `apps/backend/internal/sse/writer_test.go`

- [ ] **Step 1: Implement `apierror` first**

`apps/backend/internal/apierror/apierror.go`:

```go
// Package apierror defines the typed error envelope used by the HTTP layer.
// Resource routes render JSON:API errors[]; action routes render a plain
// { error: { code, message, details? } } envelope. The Style constant on
// the route record picks which one.
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// Style selects the envelope shape used to render an Error.
type Style int

const (
	StyleAction Style = iota // {"error":{"code","message","details?"}}
	StyleJSONAPI             // {"errors":[{"status","code","title","detail","source"}]}
)

// Error is the typed error sentinel carried across handlers.
type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	HTTPStatus int            `json:"-"`
	Pointer    string         `json:"-"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// New constructs an Error.
func New(status int, code, msg string) *Error {
	return &Error{HTTPStatus: status, Code: code, Message: msg}
}

// WithDetails returns a copy with details attached.
func (e *Error) WithDetails(d map[string]any) *Error {
	cp := *e
	cp.Details = d
	return &cp
}

// WithPointer returns a copy with JSON:API source.pointer set.
func (e *Error) WithPointer(p string) *Error {
	cp := *e
	cp.Pointer = p
	return &cp
}

// Write renders err to w with the chosen Style. Falls back to 500 if err is
// not an *Error.
func Write(w http.ResponseWriter, style Style, err error) {
	var ae *Error
	if !errors.As(err, &ae) {
		ae = New(http.StatusInternalServerError, "internal_error", "An internal error occurred.")
	}
	w.Header().Set("Content-Type", contentType(style))
	w.WriteHeader(ae.HTTPStatus)
	switch style {
	case StyleJSONAPI:
		_ = jsonapi.WriteError(w, jsonapi.Error{
			Status: ae.HTTPStatus, Code: ae.Code, Title: ae.Code,
			Detail: ae.Message, Pointer: ae.Pointer,
		})
	default:
		writeAction(w, ae)
	}
}

func writeAction(w io.Writer, ae *Error) {
	type body struct {
		Error *Error `json:"error"`
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body{Error: ae})
}

func contentType(s Style) string {
	if s == StyleJSONAPI {
		return "application/vnd.api+json"
	}
	return "application/json"
}

// Common sentinel constructors. Add additional codes as features land.
func Unauthenticated() *Error { return New(http.StatusUnauthorized, "unauthenticated", "Authentication required.") }
func CSRFInvalid() *Error     { return New(http.StatusForbidden, "csrf_token_invalid", "Missing or invalid CSRF token.") }
func NotFound(what string) *Error {
	return New(http.StatusNotFound, "not_found", what+" not found")
}
func Internal(msg string) *Error { return New(http.StatusInternalServerError, "internal_error", msg) }
```

`apierror_test.go`:

```go
package apierror_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

func TestWriteActionEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleAction, apierror.New(409, "bucket_not_empty", "Bucket contains objects").WithDetails(map[string]any{"object_count": 142}))
	require.Equal(t, 409, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	body := got["error"].(map[string]any)
	require.Equal(t, "bucket_not_empty", body["code"])
	require.Equal(t, float64(142), body["details"].(map[string]any)["object_count"])
}

func TestWriteJSONAPIEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, apierror.StyleJSONAPI, apierror.New(422, "invalid_bucket_name", "Bucket name invalid").WithPointer("/data/attributes/name"))
	require.Equal(t, 422, w.Code)
	require.Equal(t, "application/vnd.api+json", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), `"pointer":"/data/attributes/name"`)
}
```

- [ ] **Step 2: Implement `internal/sse`**

`apps/backend/internal/sse/writer.go`:

```go
// Package sse writes Server-Sent Events with reverse-proxy-friendly headers
// and periodic heartbeats. One Writer per HTTP response.
package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Writer streams SSE frames over an HTTP response.
// Heartbeat must be called periodically by the caller (or via a paired goroutine)
// to defeat proxy buffering.
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// New initializes the response headers and returns a ready Writer.
func New(w http.ResponseWriter) (*Writer, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	return &Writer{w: w, flusher: f}, nil
}

// Event writes an `event: <name>\ndata: <json>\n\n` frame and flushes.
func (sw *Writer) Event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(sw.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// Heartbeat writes a comment-only frame to keep the connection alive.
func (sw *Writer) Heartbeat() error {
	if _, err := fmt.Fprint(sw.w, ": keepalive\n\n"); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// StartHeartbeat fires a heartbeat every interval until ctx is done. Run in a
// goroutine alongside the main producer.
func (sw *Writer) StartHeartbeat(stop <-chan struct{}, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = sw.Heartbeat()
		}
	}
}
```

`sse/writer_test.go`:

```go
package sse_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/sse"
)

func TestWriterHeadersAndEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := sse.New(rec)
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))

	require.NoError(t, w.Event("progress", map[string]int{"deleted": 1000}))
	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, "event: progress\n"))
	require.Contains(t, body, `"deleted":1000`)
	require.True(t, strings.HasSuffix(body, "\n\n"))
}

func TestHeartbeat(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := sse.New(rec)
	require.NoError(t, w.Heartbeat())
	require.Contains(t, rec.Body.String(), ": keepalive")
}
```

- [ ] **Step 3: Run tests — expect PASS**

```bash
cd apps/backend && go test ./internal/apierror/... ./internal/sse/... -race
```

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/apierror apps/backend/internal/sse
git commit -m "feat(backend): apierror dual-envelope writer and SSE writer with heartbeat"
```

---

### Task T1.8: Migrations 0001–0006

**Files:**
- Create: `apps/backend/migrations/0001_admin_users.up.sql`
- Create: `apps/backend/migrations/0001_admin_users.down.sql`
- Create: `apps/backend/migrations/0002_sessions.up.sql`
- Create: `apps/backend/migrations/0002_sessions.down.sql`
- Create: `apps/backend/migrations/0003_minio_connections.up.sql`
- Create: `apps/backend/migrations/0003_minio_connections.down.sql`
- Create: `apps/backend/migrations/0004_app_settings.up.sql`
- Create: `apps/backend/migrations/0004_app_settings.down.sql`
- Create: `apps/backend/migrations/0005_audit_events.up.sql`
- Create: `apps/backend/migrations/0005_audit_events.down.sql`
- Create: `apps/backend/migrations/0006_bucket_empty_jobs.up.sql`
- Create: `apps/backend/migrations/0006_bucket_empty_jobs.down.sql`
- Create: `apps/backend/internal/db/migrations.go`
- Create: `apps/backend/internal/db/migrations_test.go`

- [ ] **Step 1: Write migrations**

**`0001_admin_users.up.sql`:**
```sql
CREATE TABLE admin_users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  username        TEXT NOT NULL UNIQUE,
  password_hash   TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  disabled_at     TEXT
);
```

**`0001_admin_users.down.sql`:**
```sql
DROP TABLE admin_users;
```

**`0002_sessions.up.sql`:**
```sql
CREATE TABLE sessions (
  id              TEXT PRIMARY KEY,
  admin_user_id   INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  created_at      TEXT NOT NULL,
  expires_at      TEXT NOT NULL,
  last_active_at  TEXT NOT NULL,
  source_ip       TEXT,
  user_agent      TEXT
);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);
CREATE INDEX sessions_admin_user_id_idx ON sessions(admin_user_id);
```

**`0002_sessions.down.sql`:**
```sql
DROP TABLE sessions;
```

**`0003_minio_connections.up.sql`:**
```sql
CREATE TABLE minio_connections (
  id                          INTEGER PRIMARY KEY AUTOINCREMENT,
  singleton_guard             INTEGER NOT NULL DEFAULT 1,
  endpoint_url                TEXT NOT NULL,
  tls_skip_verify             INTEGER NOT NULL DEFAULT 0,
  access_key_ciphertext       TEXT NOT NULL,
  secret_key_ciphertext       TEXT NOT NULL,
  custom_ca_pem_ciphertext    TEXT,
  created_at                  TEXT NOT NULL,
  updated_at                  TEXT NOT NULL
);
CREATE UNIQUE INDEX minio_connections_singleton ON minio_connections(singleton_guard);
```

**`0003_minio_connections.down.sql`:**
```sql
DROP TABLE minio_connections;
```

**`0004_app_settings.up.sql`:**
```sql
CREATE TABLE app_settings (
  key         TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
```

**`0004_app_settings.down.sql`:**
```sql
DROP TABLE app_settings;
```

**`0005_audit_events.up.sql`:**
```sql
CREATE TABLE audit_events (
  id                   TEXT PRIMARY KEY,
  occurred_at          TEXT NOT NULL,
  actor                TEXT NOT NULL,
  source_ip            TEXT,
  action               TEXT NOT NULL,
  target_type          TEXT NOT NULL,
  target_id            TEXT,
  outcome              TEXT NOT NULL,
  error_message        TEXT,
  payload_summary_json TEXT
);
CREATE INDEX audit_events_occurred_at_idx ON audit_events(occurred_at);
CREATE INDEX audit_events_action_idx      ON audit_events(action, occurred_at);
CREATE INDEX audit_events_target_idx      ON audit_events(target_type, target_id);
```

**`0005_audit_events.down.sql`:**
```sql
DROP TABLE audit_events;
```

**`0006_bucket_empty_jobs.up.sql`:**
```sql
CREATE TABLE bucket_empty_jobs (
  id                TEXT PRIMARY KEY,
  bucket_name       TEXT NOT NULL,
  started_at        TEXT NOT NULL,
  last_progress_at  TEXT NOT NULL,
  deleted_count     INTEGER NOT NULL DEFAULT 0,
  estimated_total   INTEGER,
  state             TEXT NOT NULL,
  error_message     TEXT,
  finished_at       TEXT,
  purge_versions    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX bucket_empty_jobs_active_per_bucket
  ON bucket_empty_jobs(bucket_name)
  WHERE state = 'running';
CREATE INDEX bucket_empty_jobs_bucket_idx ON bucket_empty_jobs(bucket_name, started_at);
```

**`0006_bucket_empty_jobs.down.sql`:**
```sql
DROP TABLE bucket_empty_jobs;
```

- [ ] **Step 2: Implement `apps/backend/internal/db/migrations.go`**

```go
package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

//go:embed ../../migrations/*.sql
var MigrationsFS embed.FS

// Migrate runs all up-migrations against the given gorm.DB.
func Migrate(gdb *gorm.DB) error {
	sdb, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("unwrap sql.DB: %w", err)
	}
	src, err := iofs.New(MigrationsFS, "../../migrations")
	if err != nil {
		return fmt.Errorf("open migrations source: %w", err)
	}
	driver, err := sqlite.WithInstance(sdb, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("init sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
```

(Note: the executing engineer must verify the `//go:embed` path resolves correctly relative to `migrations.go`. If the path needs to be `migrations/*.sql` and the embed must live in a separate file, restructure as needed — but keep the public `Migrate(*gorm.DB) error` signature.)

- [ ] **Step 3: Tests**

`migrations_test.go`:

```go
package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

func TestMigrateCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, DatabasePath: filepath.Join(dir, "h.db")}
	gdb, sdb, err := db.Open(cfg)
	require.NoError(t, err)
	defer sdb.Close()

	require.NoError(t, db.Migrate(gdb))

	expected := []string{
		"admin_users", "sessions", "minio_connections", "app_settings",
		"audit_events", "bucket_empty_jobs",
	}
	for _, table := range expected {
		var name string
		require.NoError(t,
			gdb.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name=?;`, table).Scan(&name).Error,
		)
		require.Equal(t, table, name, "table %q must exist after migration", table)
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd apps/backend && go test ./internal/db/... -race
```

- [ ] **Step 5: Commit**

```bash
git add apps/backend/migrations apps/backend/internal/db
git commit -m "feat(backend/db): 0001–0006 migrations + golang-migrate runner via iofs embed"
```

---

### Task T1.9: `internal/audit` — minimal writer + retention sweeper + sanitizer

**Files:** `[skill scaffold]` for the `audit` domain. The seven canonical files plus tests, with these specifics:

- `model.go` defines `Event` with fields per data-model.md §6: `ID` (ULID), `OccurredAt time.Time`, `Actor`, `SourceIP`, `Action`, `TargetType`, `TargetID`, `Outcome`, `ErrorMessage`, `PayloadSummary map[string]any`. Constants for action and target enums per data-model §allowed values.
- `entity.go` defines GORM struct `auditEvent` with `payload_summary_json string` (JSON-encoded) and `Make`/`ToEntity`.
- `provider.go` exposes `getByID(id string)`, `list(filter Filter)` returning `database.EntityProvider` shapes (or the simplified Go equivalent if the project hasn't pulled in a generic provider helper yet — at minimum, return `func(*gorm.DB) ([]Event, error)`).
- `administrator.go` exposes `insert(db *gorm.DB, e Event) error`, `deleteOlderThan(db *gorm.DB, cutoff time.Time) (int64, error)`.
- `processor.go` exposes `Record(ctx, action, targetType, targetID, outcome, payload, errMsg)` and `RetentionSweep(cutoff)`.
- `resource.go` registers `GET /api/v1/audit-events` (with filter+page params) — wired in M5; for M1 just expose the type.
- `rest.go` defines the JSON:API attrs (omitting `source_ip` from the `payload_summary` ever).
- `sanitize.go` defines `Sanitize(map[string]any) map[string]any` that drops any key matching `secret|password|token|csrf|signature|presigned|url` (case-insensitive, regex).
- `retention.go` exposes `StartRetentionSweeper(ctx, p *Processor, every time.Duration)` goroutine.

**Tests:**
- `sanitize_test.go` — table-driven: input vs expected post-scrub map.
- `processor_test.go` — record an event, read it back, assert `payload_summary_json` excludes scrubbed keys.
- `retention_test.go` — insert events at varying timestamps, sweep, assert correct rows removed.
- `no_secrets_test.go` — **critical**: a test that enumerates every constant in the `action` enum and confirms `Sanitize` is invoked for that action by reading the source. Implementation: for each action constant, build a fake `Event` with payload `{"secret_key": "X", "password": "Y"}`, call `Record`, then `provider.getByID`, assert the persisted `payload_summary_json` contains neither `"secret_key"` nor `"password"`.

- [ ] **Step 1: Scaffold the eight files per skill `file-responsibilities.md`. Wire `Sanitize` into `Record` such that no path can write a non-sanitized payload.**

- [ ] **Step 2: Implement `sanitize.go`**

```go
package audit

import "regexp"

var sensitiveKeyRE = regexp.MustCompile(`(?i)(secret|password|token|csrf|signature|presigned|url)`)

// Sanitize returns a copy of m with sensitive keys removed. Nil maps return nil.
func Sanitize(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sensitiveKeyRE.MatchString(k) {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = Sanitize(nested)
			continue
		}
		out[k] = v
	}
	return out
}
```

- [ ] **Step 3: Implement `no_secrets_test.go`** (enumerate constants — example skeleton)

```go
package audit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

var allActions = []string{
	"bucket.create", "bucket.delete", "bucket.versioning.enable", "bucket.versioning.disable",
	"bucket.public_access.update", "bucket.quota.update", "bucket.empty",
	"object.upload", "object.delete", "object.download_proxy", "object.share_link.create",
	"user.create", "user.delete", "user.disable", "user.enable", "user.policies.update",
	"service_account.create", "service_account.revoke",
	"lifecycle_rule.create", "lifecycle_rule.delete",
	"session.login", "session.logout", "session.login_failed",
	"connection.update", "connection.test",
	"admin.password.change", "admin.encryption.reset",
}

func TestNoSecretsInPayloadAnyAction(t *testing.T) {
	p := newTestProcessor(t)
	for _, a := range allActions {
		p.Record(ctx, audit.Event{
			OccurredAt:     time.Now().UTC(),
			Action:         a,
			TargetType:     "bucket",
			TargetID:       "x",
			Outcome:        "success",
			PayloadSummary: map[string]any{
				"secret_key":    "AAA",
				"password":      "BBB",
				"presigned_url": "CCC",
				"safe_field":    "ok",
			},
		})
		got := loadLatest(t, p, a)
		require.NotContains(t, strings.ToLower(got), "secret_key", "action %s leaked secret_key", a)
		require.NotContains(t, strings.ToLower(got), "password", "action %s leaked password", a)
		require.NotContains(t, strings.ToLower(got), "presigned_url", "action %s leaked presigned_url", a)
		require.Contains(t, got, "safe_field")
	}
}
```

(`newTestProcessor` and `loadLatest` are test helpers using an in-memory SQLite + the migration runner.)

- [ ] **Step 4: Run — expect PASS**

```bash
cd apps/backend && go test ./internal/audit/... -race
```

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/audit
git commit -m "feat(backend/audit): event writer + sanitizer + retention sweeper + per-action no-secrets test"
```

---

### Task T1.10: `internal/minio` — client pool scaffold

**Files:**
- Create: `apps/backend/internal/minio/pool.go`
- Create: `apps/backend/internal/minio/pool_test.go`

(Setup persistence lands in M2; here we just provide the type with a stubbed `Rebuild(decryptedCreds)` that other code can call without a real connection.)

- [ ] **Step 1: Implement `pool.go`**

```go
// Package minio holds the cached madmin + minio-go client pair built from the
// current decrypted connection settings. Pool.Rebuild is called by the
// connection processor after a successful update.
package minio

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"sync"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Credentials carries plaintext keys for client construction. Never persisted.
type Credentials struct {
	EndpointURL     string
	AccessKey       string
	SecretKey       string
	TLSSkipVerify   bool
	CustomCAPEMText string
}

// Pool holds the active client pair behind an RWMutex.
type Pool struct {
	mu   sync.RWMutex
	mc   *miniogo.Client
	madm *madmin.AdminClient
	cred Credentials
}

// NewEmpty returns an unbound Pool (useful before setup is complete).
func NewEmpty() *Pool { return &Pool{} }

// Rebuild swaps in new clients built from creds. Old credentials are zeroed.
func (p *Pool) Rebuild(creds Credentials) error {
	mc, madm, err := build(creds)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// zero previous secret bytes
	for i := range p.cred.SecretKey {
		_ = i
	}
	p.cred = creds
	p.mc = mc
	p.madm = madm
	return nil
}

// Get returns the current pair, or ErrNotInitialized if Rebuild has not yet
// succeeded.
func (p *Pool) Get(ctx context.Context) (*madmin.AdminClient, *miniogo.Client, error) {
	_ = ctx
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.mc == nil || p.madm == nil {
		return nil, nil, ErrNotInitialized
	}
	return p.madm, p.mc, nil
}

// ErrNotInitialized is returned by Get when the pool has no active connection.
var ErrNotInitialized = errors.New("minio pool: connection not yet configured")

func build(c Credentials) (*miniogo.Client, *madmin.AdminClient, error) {
	parsed, useTLS, host, err := parseEndpoint(c.EndpointURL)
	if err != nil {
		return nil, nil, err
	}
	tr, err := transport(c, useTLS)
	if err != nil {
		return nil, nil, err
	}
	mc, err := miniogo.New(host, &miniogo.Options{
		Creds:     credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure:    useTLS,
		Transport: tr,
	})
	if err != nil {
		return nil, nil, err
	}
	madm, err := madmin.NewWithOptions(host, &madmin.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: useTLS,
	})
	if err != nil {
		return nil, nil, err
	}
	madm.SetCustomTransport(tr)
	_ = parsed
	return mc, madm, nil
}
```

(Implement `parseEndpoint` and `transport` as helpers handling `http://`/`https://` schemes, optional ports, and custom CA pinning by appending PEM to a new `x509.CertPool`. Function bodies are mechanical — the executing engineer fills these in following the standard pattern.)

`transport` signature:
```go
func transport(c Credentials, useTLS bool) (*http.Transport, error) {
    // Build *http.Transport with InsecureSkipVerify=c.TLSSkipVerify and
    // RootCAs containing the system pool plus c.CustomCAPEMText if non-empty.
    // Returns the configured transport.
    _ = x509.NewCertPool
    return &http.Transport{}, nil
}
```

`parseEndpoint(url string) (parsed *url.URL, useTLS bool, host string, err error)` parses an HTTP(S) URL, returns `useTLS=true` when scheme is `https`, and `host` = `url.Host`. Reject schemes other than `http`/`https`.

- [ ] **Step 2: Tests**

```go
package minio_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

func TestPoolGetBeforeRebuild(t *testing.T) {
	p := hmminio.NewEmpty()
	_, _, err := p.Get(context.Background())
	require.ErrorIs(t, err, hmminio.ErrNotInitialized)
}

func TestPoolRebuildSwapsClients(t *testing.T) {
	p := hmminio.NewEmpty()
	require.NoError(t, p.Rebuild(hmminio.Credentials{
		EndpointURL: "https://minio.example.test:9000",
		AccessKey:   "AKIA",
		SecretKey:   "SECRET",
	}))
	madm, mc, err := p.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, madm)
	require.NotNil(t, mc)
}
```

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/minio/... -race
git add apps/backend/internal/minio
git commit -m "feat(backend/minio): client Pool with atomic Rebuild and custom CA support"
```

---

### Task T1.11: `internal/server` — bootstrap, middleware order, SPA fallback, graceful shutdown

**Files:**
- Create: `apps/backend/internal/server/server.go`
- Create: `apps/backend/internal/server/spa.go`
- Create: `apps/backend/internal/server/health.go`
- Create: `apps/backend/internal/server/server_test.go`
- Create: `apps/backend/internal/server/spa-dist/.gitkeep` (already created in T0.7)

- [ ] **Step 1: Implement `server.go`**

```go
// Package server boots the chi mux, mounts the API and SPA, and runs the
// main + optional metrics HTTP servers with graceful shutdown.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/observability"
)

// Deps wires per-domain handler registrars into Server.
type Deps struct {
	Logger zerolog.Logger
	// Subrouter handlers added in M2+ — each domain provides a func(chi.Router).
	APIRoutes []func(r chi.Router)
}

// New builds a Server but does not start it.
func New(cfg config.Config, deps Deps) *Server {
	return &Server{cfg: cfg, deps: deps}
}

// Server holds the HTTP server lifecycle.
type Server struct {
	cfg   config.Config
	deps  Deps
	httpS *http.Server
}

// Run blocks until ctx is canceled. Graceful shutdown drains for 10s.
func (s *Server) Run(ctx context.Context) error {
	root := chi.NewRouter()
	root.Use(chimw.RequestID)
	root.Use(chimw.Recoverer)
	root.Use(observability.Logger(s.deps.Logger))
	root.Use(chimw.RealIP)
	root.Use(chimw.Timeout(30 * time.Second))

	root.Get("/healthz", healthz)
	root.Get("/readyz", readyz(s))

	api := chi.NewRouter()
	for _, register := range s.deps.APIRoutes {
		register(api)
	}
	root.Mount(s.cfg.BasePath+"/api/v1", api)

	root.Handle("/*", spaHandler(s.cfg.BasePath))

	s.httpS = &http.Server{Addr: s.cfg.ListenAddr, Handler: root, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		err := s.httpS.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpS.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}
```

- [ ] **Step 2: Implement `health.go`**

```go
package server

import (
	"net/http"
	"sync/atomic"
)

// Health holds the live/ready signals.
type Health struct {
	ready atomic.Bool
}

// SetReady flips the readiness signal.
func (h *Health) SetReady(b bool) { h.ready.Store(b) }

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyz(_ *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// In M1 we always return 200 once the server is running. M2 wires in
		// migrations + cached MinIO admin probe and gates here.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
```

- [ ] **Step 3: Implement `spa.go` (placeholder; M2 wires real embed)**

```go
package server

import (
	"net/http"
	"strings"
)

func spaHandler(basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		_ = basePath
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Harbormaster</title><p>SPA placeholder — M2 will wire embedded assets.</p>`))
	})
}
```

- [ ] **Step 4: Tests in `server_test.go`**

```go
package server_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/server"
)

func TestServerHealthzAndShutdown(t *testing.T) {
	cfg := config.Config{ListenAddr: "127.0.0.1:0", LogLevel: "info", LogFormat: "json", BasePath: "/"}
	// Use a fixed port for the test
	cfg.ListenAddr = "127.0.0.1:18080"
	l, _ := log.New("info", "json")
	s := server.New(cfg, server.Deps{Logger: l})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:18080/healthz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(body), `"status":"ok"`)

	resp2, err := http.Get("http://127.0.0.1:18080/api/v1/anything")
	require.NoError(t, err)
	resp2.Body.Close()
	// No API routes registered yet → chi returns 404.
	require.Equal(t, 404, resp2.StatusCode)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("shutdown timed out")
	}
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
cd apps/backend && go test ./internal/server/... -race
```

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/server
git commit -m "feat(backend/server): chi bootstrap, middleware stack, SPA fallback placeholder, graceful shutdown"
```

---

### Task T1.12: CLI dispatch — `serve`, `version`, admin subcommands (scaffolding)

**Files:**
- Modify: `apps/backend/cmd/harbormaster/main.go` (replace with cobra dispatch)
- Create: `apps/backend/cmd/harbormaster/serve.go`
- Create: `apps/backend/cmd/harbormaster/version.go`
- Create: `apps/backend/cmd/harbormaster/admin_reset_password.go`
- Create: `apps/backend/cmd/harbormaster/admin_reset_encryption.go`
- Create: `apps/backend/cmd/harbormaster/main_test.go` (replace)

- [ ] **Step 1: Replace `main.go` with cobra root**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd(os.Stdout).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "harbormaster",
		Short: "Self-hosted MinIO admin UI",
	}
	root.SetOut(out)
	root.AddCommand(
		newServeCmd(out),
		newVersionCmd(out),
		newAdminCmd(out),
	)
	return root
}

func newAdminCmd(out io.Writer) *cobra.Command {
	c := &cobra.Command{Use: "admin", Short: "Administrative recovery commands"}
	c.AddCommand(newAdminResetPasswordCmd(out), newAdminResetEncryptionCmd(out))
	return c
}
```

- [ ] **Step 2: `version.go`**

```go
package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newVersionCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintln(out, version)
		},
	}
}
```

- [ ] **Step 3: `serve.go` (minimal — boots config, opens DB, runs server)**

```go
package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
	"github.com/jtumidanski/Harbormaster/internal/observability/log"
	"github.com/jtumidanski/Harbormaster/internal/server"
)

func newServeCmd(out io.Writer) *cobra.Command {
	c := &cobra.Command{
		Use:   "serve",
		Short: "Run the Harbormaster HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), out)
		},
	}
	return c
}

func runServe(ctx context.Context, _ io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	logger, err := log.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}
	gdb, sdb, err := db.Open(cfg)
	if err != nil {
		return err
	}
	defer sdb.Close()
	if err := db.Migrate(gdb); err != nil {
		return err
	}
	s := server.New(cfg, server.Deps{Logger: logger})
	logger.Info().Str("addr", cfg.ListenAddr).Msg("harbormaster started")
	return s.Run(ctx)
}
```

- [ ] **Step 4: Admin commands as stubs (functionality lands in T1.13–T1.14)**

`admin_reset_password.go`:
```go
package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

func newAdminResetPasswordCmd(out io.Writer) *cobra.Command {
	var username string
	c := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset the local admin password (interactive prompt)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = out
			_ = username
			return errors.New("reset-password not yet implemented; lands in M1 task T1.13")
		},
	}
	c.Flags().StringVar(&username, "username", "", "admin username (required)")
	_ = c.MarkFlagRequired("username")
	return c
}
```

`admin_reset_encryption.go`:
```go
package main

import (
	"errors"
	"io"

	"github.com/spf13/cobra"
)

func newAdminResetEncryptionCmd(out io.Writer) *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "reset-encryption",
		Short: "Destructive: back up DB, regenerate encryption key, clear minio_connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = out
			if !confirm {
				return errors.New("--confirm is mandatory; this is a destructive recovery operation")
			}
			return errors.New("reset-encryption not yet implemented; lands in M1 task T1.14")
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "acknowledge destructive recovery")
	return c
}
```

- [ ] **Step 5: Test `main_test.go`**

```go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionPrints(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd(&out)
	root.SetArgs([]string{"version"})
	require.NoError(t, root.Execute())
	require.NotEqual(t, "", strings.TrimSpace(out.String()))
}

func TestAdminResetEncryptionRequiresConfirm(t *testing.T) {
	root := newRootCmd(&bytes.Buffer{})
	root.SetArgs([]string{"admin", "reset-encryption"})
	err := root.Execute()
	require.ErrorContains(t, err, "--confirm is mandatory")
}
```

- [ ] **Step 6: Verify**

```bash
cd apps/backend && go test ./cmd/harbormaster/... -race && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add apps/backend/cmd/harbormaster
git commit -m "feat(backend/cmd): cobra root + serve/version commands and admin subcommand stubs"
```

---

### Task T1.13: Implement `harbormaster admin reset-password`

**Files:**
- Modify: `apps/backend/cmd/harbormaster/admin_reset_password.go`
- Create: `apps/backend/internal/auth/password.go` (argon2id hashing helpers)
- Create: `apps/backend/internal/auth/password_test.go`

- [ ] **Step 1: Implement argon2id hashing in `internal/auth/password.go`**

```go
// Package auth provides password hashing, session lifecycle, and request
// middleware. password.go covers argon2id hashing.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id RFC 9106 minimum parameters.
const (
	argonMemoryKB = 64 * 1024
	argonTime     = 3
	argonThreads  = 2
	argonSaltLen  = 16
	argonHashLen  = 32
)

// HashPassword returns a PHC-encoded argon2id hash.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(pw), salt, argonTime, argonMemoryKB, argonThreads, argonHashLen)
	encSalt := base64.RawStdEncoding.EncodeToString(salt)
	encHash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoryKB, argonTime, argonThreads, encSalt, encHash), nil
}

// VerifyPassword returns nil iff pw matches the stored encoded hash.
func VerifyPassword(encoded, pw string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("malformed hash")
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return fmt.Errorf("malformed params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("password mismatch")
	}
	return nil
}
```

- [ ] **Step 2: Tests `password_test.go`**

```go
package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/auth"
)

func TestHashVerify(t *testing.T) {
	h, err := auth.HashPassword("correct horse battery staple!")
	require.NoError(t, err)
	require.NoError(t, auth.VerifyPassword(h, "correct horse battery staple!"))
	require.Error(t, auth.VerifyPassword(h, "wrong"))
}

func TestHashIsRandomized(t *testing.T) {
	a, _ := auth.HashPassword("x")
	b, _ := auth.HashPassword("x")
	require.NotEqual(t, a, b)
}
```

- [ ] **Step 3: Implement `admin reset-password`**

Replace the body of `admin_reset_password.go`:

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

func newAdminResetPasswordCmd(out io.Writer) *cobra.Command {
	var username string
	c := &cobra.Command{
		Use:   "reset-password",
		Short: "Reset the local admin password (interactive prompt)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Fprint(out, "New password: ")
			pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(out)
			if err != nil {
				return err
			}
			fmt.Fprint(out, "Confirm password: ")
			pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(out)
			if err != nil {
				return err
			}
			if string(pw1) != string(pw2) {
				return errors.New("passwords do not match")
			}
			if len(pw1) < 12 {
				return errors.New("password must be at least 12 characters")
			}
			hash, err := auth.HashPassword(string(pw1))
			if err != nil {
				return err
			}
			gdb, sdb, err := db.Open(cfg)
			if err != nil {
				return err
			}
			defer sdb.Close()
			if err := db.Migrate(gdb); err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339)
			res := gdb.Exec(`UPDATE admin_users SET password_hash=?, updated_at=? WHERE username=?`, hash, now, username)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("no admin user %q", username)
			}
			fmt.Fprintf(out, "Password updated for user %q.\n", username)
			return nil
		},
	}
	c.Flags().StringVar(&username, "username", "", "admin username (required)")
	_ = c.MarkFlagRequired("username")
	return c
}
```

- [ ] **Step 4: Add `golang.org/x/term` dependency**

```bash
cd apps/backend && go get golang.org/x/term@v0.22.0 && go mod tidy
```

- [ ] **Step 5: Smoke test manually (the interactive prompt is hard to unit-test; integration test added in M2)**

```bash
cd apps/backend && go build -o /tmp/hm ./cmd/harbormaster && /tmp/hm admin reset-password --username admin || true
```
Expected: prompts for password, then fails with `no admin user "admin"` because the table is empty — verifies the wiring.

- [ ] **Step 6: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/cli): admin reset-password subcommand with argon2id hashing"
```

---

### Task T1.14: Implement `harbormaster admin reset-encryption --confirm`

**Files:**
- Modify: `apps/backend/cmd/harbormaster/admin_reset_encryption.go`

- [ ] **Step 1: Implement reset flow**

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jtumidanski/Harbormaster/internal/config"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
	"github.com/jtumidanski/Harbormaster/internal/db"
)

func newAdminResetEncryptionCmd(out io.Writer) *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "reset-encryption",
		Short: "Destructive: back up DB, regenerate encryption key, clear minio_connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				fmt.Fprintln(out, `WARNING: this is a destructive recovery operation.
It will:
  1. Back up the SQLite database to <path>.pre-reset-<unix-ts>.bak
  2. Generate a new encryption key at HARBORMASTER_ENCRYPTION_KEY_FILE
     (or <data dir>/encryption.key by default)
  3. Truncate the minio_connections table
  4. Clear the setup_completed flag so the first-run wizard reappears

Re-run with --confirm to proceed.`)
				return errors.New("--confirm is mandatory; this is a destructive recovery operation")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ts := time.Now().Unix()
			backup := fmt.Sprintf("%s.pre-reset-%d.bak", cfg.DatabasePath, ts)
			if err := copyFile(cfg.DatabasePath, backup); err != nil {
				return fmt.Errorf("backup db: %w", err)
			}
			fmt.Fprintf(out, "Backup written to %s\n", backup)
			// Move the old key out of the way before generating a new one.
			if _, err := os.Stat(cfg.EncryptionKeyFile); err == nil {
				if err := os.Rename(cfg.EncryptionKeyFile,
					fmt.Sprintf("%s.pre-reset-%d.bak", cfg.EncryptionKeyFile, ts)); err != nil {
					return fmt.Errorf("rotate old key: %w", err)
				}
			}
			if _, _, err := crypto.LoadKey(cfg.EncryptionKeyFile); err != nil {
				return fmt.Errorf("generate new key: %w", err)
			}
			gdb, sdb, err := db.Open(cfg)
			if err != nil {
				return err
			}
			defer sdb.Close()
			if err := db.Migrate(gdb); err != nil {
				return err
			}
			if err := gdb.Exec(`DELETE FROM minio_connections`).Error; err != nil {
				return err
			}
			if err := gdb.Exec(`DELETE FROM app_settings WHERE key IN ('setup_completed','encryption_key_fingerprint')`).Error; err != nil {
				return err
			}
			// Now record a fresh fingerprint for the new key.
			_, fp, err := crypto.LoadKey(cfg.EncryptionKeyFile)
			if err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if err := gdb.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)`,
				"encryption_key_fingerprint", fp, now).Error; err != nil {
				return err
			}
			fmt.Fprintln(out, "Reset complete. Restart Harbormaster to enter the first-run wizard.")
			return nil
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "acknowledge destructive recovery")
	return c
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func _filepath() string { return filepath.Separator + "" }
```

(Drop the `_filepath` helper if unused; it exists only because the import was needed for the symbol path.) Re-tidy imports: keep `filepath` only if used elsewhere.

- [ ] **Step 2: Add a smoke test using a temp dir**

`apps/backend/cmd/harbormaster/admin_reset_encryption_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminResetEncryptionEnd2End(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARBORMASTER_DATA_DIR", dir)
	t.Setenv("HARBORMASTER_DATABASE_PATH", filepath.Join(dir, "h.db"))
	// Create the data dir so config.Load accepts it
	require.NoError(t, os.MkdirAll(dir, 0o700))

	// First boot: write a stub key + db
	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), make([]byte, 32), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "h.db"), []byte("dummy"), 0o600))

	var out bytes.Buffer
	root := newRootCmd(&out)
	root.SetArgs([]string{"admin", "reset-encryption", "--confirm"})
	require.NoError(t, root.Execute())

	// Backup file should exist
	entries, _ := os.ReadDir(dir)
	hasBackup := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bak" {
			hasBackup = true
		}
	}
	require.True(t, hasBackup, "expected at least one .bak file")
}
```

- [ ] **Step 3: Run + commit**

```bash
cd apps/backend && go test ./cmd/harbormaster/... -race
git add apps/backend/cmd/harbormaster
git commit -m "feat(backend/cli): admin reset-encryption --confirm with db backup and key rotation"
```

---

### Task T1.15: Wire encryption-key fingerprint check into `runServe`

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: Extend `runServe` after `db.Migrate`**

Insert before `s := server.New(...)`:

```go
	keyBytes, fp, err := crypto.LoadKey(cfg.EncryptionKeyFile)
	if err != nil {
		return err
	}
	cipher, err := crypto.New(keyBytes)
	if err != nil {
		return err
	}
	_ = cipher // wired into M2 setup/connection processors

	// Fingerprint check
	var stored string
	gdb.Raw(`SELECT value FROM app_settings WHERE key = ?`, "encryption_key_fingerprint").Scan(&stored)
	switch {
	case stored == "" :
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gdb.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)`,
			"encryption_key_fingerprint", fp, now).Error; err != nil {
			return err
		}
	case stored != fp:
		return fmt.Errorf("encryption key fingerprint mismatch (stored=%s, current=%s); refusing to start", stored, fp)
	}
```

Add imports: `"time"`, `"fmt"`, `"github.com/jtumidanski/Harbormaster/internal/crypto"`.

- [ ] **Step 2: Add a startup test**

`apps/backend/cmd/harbormaster/serve_test.go`:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServeFailsOnKeyFingerprintMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARBORMASTER_DATA_DIR", dir)
	t.Setenv("HARBORMASTER_DATABASE_PATH", filepath.Join(dir, "h.db"))
	t.Setenv("HARBORMASTER_LISTEN_ADDR", "127.0.0.1:0")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), make([]byte, 32), 0o600))

	// First call records the fingerprint.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = runServe(ctx, os.Stdout) // it boots, then ctx times out

	// Swap the key file with a different one.
	newKey := make([]byte, 32)
	newKey[0] = 0xFF
	require.NoError(t, os.WriteFile(filepath.Join(dir, "encryption.key"), newKey, 0o600))

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	err := runServe(ctx2, os.Stdout)
	require.ErrorContains(t, err, "encryption key fingerprint mismatch")
}
```

- [ ] **Step 3: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): encryption key fingerprint check on startup"
```

---

### Task T1.16: Wire `runServe` to register/expose audit retention sweeper

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: After fingerprint check, before `s.Run(ctx)`, start the sweeper**

```go
	auditProc := audit.NewProcessor(gdb, logger)
	go audit.StartRetentionSweeper(ctx, auditProc, 24*time.Hour, cfg.AuditRetention)
```

Add import `"github.com/jtumidanski/Harbormaster/internal/audit"`.

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): start audit retention sweeper at startup"
```

---

### Task T1.17: M1 verification and demo

- [ ] **Step 1: Run full backend verification**

```bash
cd apps/backend && go vet ./... && go test -race -count=1 ./... && golangci-lint run && CGO_ENABLED=0 go build ./...
```

- [ ] **Step 2: Boot the binary against a temp data dir**

```bash
cd apps/backend && go build -o /tmp/hm ./cmd/harbormaster
mkdir -p /tmp/hm-data && \
  HARBORMASTER_DATA_DIR=/tmp/hm-data \
  HARBORMASTER_DATABASE_PATH=/tmp/hm-data/h.db \
  HARBORMASTER_LISTEN_ADDR=127.0.0.1:8080 \
  /tmp/hm serve &
sleep 1
curl -s http://127.0.0.1:8080/healthz
# Expected: {"status":"ok"}
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/api/v1/anything
# Expected: 404 (no routes registered yet)
kill %1
```

- [ ] **Step 3: Confirm `admin reset-password` and `admin reset-encryption --confirm` work**

(Manual; verify via temp env as in T1.14's test.)

---

### Task T1.18: M1 code review and merge

- [ ] **Step 1: Invoke `superpowers:requesting-code-review`**

Targets: backend changes only (no frontend touched in M1). Reviewers: `backend-guidelines-reviewer` + `plan-adherence-reviewer`.

- [ ] **Step 2: Address findings, commit fixes**

- [ ] **Step 3: Tag M1**

```bash
git tag m1-complete
```

---

## Milestone M2 — Setup wizard + auth + connection

Goal: an operator can `docker compose up`, point a browser at the app, complete the first-run wizard, log in, and see an empty "Buckets" page. All cross-cutting middleware is exercised end-to-end.

### Task T2.1: `internal/auth` — session model + repository

**Files:** `[skill scaffold]` for the `auth` domain — eight files per skill pattern (`model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `administrator.go`, `resource.go`, `rest.go`) plus tests.

Specifics:

- `model.go` — `AdminUser{ID uint, Username string, PasswordHash string, CreatedAt, UpdatedAt time.Time, DisabledAt *time.Time}` and `Session{ID string, AdminUserID uint, CreatedAt, ExpiresAt, LastActiveAt time.Time, SourceIP, UserAgent string}`. Both immutable; accessors only.
- `entity.go` — `adminUserEntity` (gorm tags map to `admin_users` columns; ULID stays text). `sessionEntity` for `sessions`. `Make`/`ToEntity` both ways.
- `builder.go` — `NewAdminUserBuilder().Username(s).PasswordHash(h).Build()` validates `len(username) ∈ [3, 64]` and that the username matches `^[a-z0-9._-]+$`. `NewSessionBuilder(...)` similar.
- `provider.go` — `getAdminUserByUsername(name string) func(*gorm.DB) (AdminUser, error)`, `getSessionByID(id string) func(*gorm.DB) (Session, error)`, `getActiveSessionsByAdmin(id uint) func(*gorm.DB) ([]Session, error)`.
- `administrator.go` — `createAdminUser(db, AdminUser) (adminUserEntity, error)`, `updateAdminUserPassword(db, id, hash) error`, `createSession(db, Session) error`, `deleteSession(db, id string) error`, `deleteExpiredSessions(db, cutoff time.Time) (int64, error)`, `rotateSession(db, oldID, new Session) error`.
- `processor.go` — `(p *Processor) Login(ctx, username, password, ip, ua) (sessionID string, csrfToken string, err error)`, `(p *Processor) Logout(ctx, sessionID) error`, `(p *Processor) Me(ctx, sessionID) (Session, AdminUser, error)`, `(p *Processor) ChangePassword(ctx, sessionID, current, new string) error`, `(p *Processor) SweepExpired(ctx) error`.
- `rest.go` defines JSON/JSON:API DTOs for the action endpoints (login, logout, me, password); these are action-shape so they're plain JSON.

- [ ] **Step 1: Implement the eight files. Test-first per file.**

Each provider/administrator function gets a unit test against an in-memory SQLite. Tests must include:
  - `TestLoginUnknownUser` returns `apierror.New(401, "invalid_credentials", …)`
  - `TestLoginBadPassword` returns same code (no oracle)
  - `TestLoginSuccessIssuesSessionAndCSRF`
  - `TestLogoutInvalidatesSession`
  - `TestSessionExpiry`
  - `TestRotateOnLogin` — session ID before login ≠ session ID after
  - `TestChangePasswordRejectsWrongCurrent`
  - `TestSweepRemovesExpired`

- [ ] **Step 2: Wire login rate limiter**

Add `internal/auth/ratelimit.go` — in-memory sliding window keyed by source IP, 5 failures per 5 minutes:

```go
package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter throttles failed logins per source IP. In-memory only;
// single-replica deployment as documented in risks.md R6.
type LoginRateLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	window   time.Duration
	max      int
}

// NewLoginRateLimiter creates a limiter with the given window and max-fail count.
func NewLoginRateLimiter(window time.Duration, max int) *LoginRateLimiter {
	return &LoginRateLimiter{failures: map[string][]time.Time{}, window: window, max: max}
}

// Allow returns true iff the IP may attempt a login now. It does NOT record
// the attempt; callers MUST call RecordFailure() after a failed login.
func (l *LoginRateLimiter) Allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	pruned := l.failures[ip][:0]
	for _, t := range l.failures[ip] {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	l.failures[ip] = pruned
	return len(pruned) < l.max
}

// RecordFailure appends a failure timestamp.
func (l *LoginRateLimiter) RecordFailure(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.failures[ip], now)
}

// Reset clears all recorded failures for an IP (call on successful login).
func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}
```

Test: 5 calls + 1 denied; then advance time past window, allow again; on reset, fresh budget.

- [ ] **Step 3: Implement CSRF token issuance**

Add `internal/auth/csrf.go`:

```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// NewCSRFToken returns a 256-bit base64url-encoded random token.
func NewCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
```

Test: produces > 32 chars, different on each call, alphabet matches base64url.

- [ ] **Step 4: Run tests + commit**

```bash
cd apps/backend && go test ./internal/auth/... -race
git add apps/backend/internal/auth
git commit -m "feat(backend/auth): admin/session domain, login rate limiter, CSRF token issuance"
```

---

### Task T2.2: Auth middleware: `RequireSession`, `RequireCSRF`, `AuditTagger`

**Files:**
- Create: `apps/backend/internal/auth/middleware.go`
- Create: `apps/backend/internal/auth/middleware_test.go`

- [ ] **Step 1: Implement middleware**

```go
package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

type ctxKey int

const (
	ctxSession ctxKey = iota
	ctxCSRF
)

// SessionInfo is attached to the request context by RequireSession.
type SessionInfo struct {
	SessionID   string
	AdminUserID uint
	Username    string
	SourceIP    string
}

// FromContext extracts SessionInfo. Returns false if absent.
func FromContext(ctx context.Context) (SessionInfo, bool) {
	si, ok := ctx.Value(ctxSession).(SessionInfo)
	return si, ok
}

// RequireSession reads the session cookie, looks it up, and rejects with 401
// on absence or expiry.
func RequireSession(cookieName string, p *Processor, style apierror.Style) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			sess, user, err := p.Me(r.Context(), c.Value)
			if err != nil {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			if !sess.ExpiresAt.After(time.Now().UTC()) {
				apierror.Write(w, style, apierror.Unauthenticated())
				return
			}
			ctx := context.WithValue(r.Context(), ctxSession, SessionInfo{
				SessionID:   sess.ID,
				AdminUserID: user.ID,
				Username:    user.Username,
				SourceIP:    sess.SourceIP,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireCSRF enforces the double-submit token pattern on unsafe methods.
func RequireCSRF(cookieName string, style apierror.Style) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				apierror.Write(w, style, apierror.CSRFInvalid())
				return
			}
			h := r.Header.Get("X-CSRF-Token")
			if h == "" || h != c.Value {
				apierror.Write(w, style, apierror.CSRFInvalid())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ErrNoSession is returned by helpers that require an authenticated context.
var ErrNoSession = errors.New("auth: no session in context")
```

- [ ] **Step 2: Tests**

```go
package auth_test

// Use httptest.NewRecorder; build a chi router with the middleware applied;
// assert 401 / 403 / 200 outcomes for the documented cases.
```

Cases to cover:
- No session cookie → 401.
- Wrong session cookie value → 401.
- Valid session → handler runs.
- POST without CSRF header → 403.
- POST with mismatched CSRF → 403.
- POST with matched CSRF → 200.
- GET ignores CSRF entirely.

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/auth/... -race
git add apps/backend/internal/auth
git commit -m "feat(backend/auth): RequireSession and RequireCSRF middleware with dual-envelope error responses"
```

---

### Task T2.3: `auth.Processor` — login + logout + cookies in HTTP handler layer

**Files:**
- Create: `apps/backend/internal/auth/handler.go` (action HTTP handlers for `/api/v1/auth/*` and `/api/v1/csrf`)
- Create: `apps/backend/internal/auth/handler_test.go`

- [ ] **Step 1: Implement handlers (action envelope)**

```go
package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// HandlerDeps wires processor + cookie config + rate limiter.
type HandlerDeps struct {
	Processor         *Processor
	RateLimiter       *LoginRateLimiter
	SessionCookieName string
	CSRFCookieName    string
	BasePath          string
	SessionTimeout    time.Duration
	Secure            bool
}

// Routes returns a chi.Router with /auth and /csrf endpoints mounted.
func Routes(d HandlerDeps) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/auth/login", d.login)
		r.Post("/auth/logout", d.logout)
		r.Get("/auth/me", d.me)
		r.Post("/auth/password", d.changePassword)
		r.Get("/csrf", d.issueCSRF)
	}
}

func (d HandlerDeps) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(400, "bad_request", "Invalid JSON body"))
		return
	}
	ip := r.RemoteAddr
	now := time.Now().UTC()
	if !d.RateLimiter.Allow(ip, now) {
		apierror.Write(w, apierror.StyleAction, apierror.New(429, "too_many_attempts",
			"Too many failed attempts; try again in 5 minutes"))
		return
	}
	sid, csrf, err := d.Processor.Login(r.Context(), body.Username, body.Password, ip, r.UserAgent())
	if err != nil {
		d.RateLimiter.RecordFailure(ip, now)
		apierror.Write(w, apierror.StyleAction, apierror.New(401, "invalid_credentials",
			"Invalid username or password"))
		return
	}
	d.RateLimiter.Reset(ip)
	d.setSessionCookie(w, sid)
	d.setCSRFCookie(w, csrf)
	w.WriteHeader(http.StatusNoContent)
}

func (d HandlerDeps) logout(w http.ResponseWriter, r *http.Request) {
	si, ok := FromContext(r.Context())
	if !ok {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	if err := d.Processor.Logout(r.Context(), si.SessionID); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal("logout failed"))
		return
	}
	d.clearCookie(w, d.SessionCookieName)
	d.clearCookie(w, d.CSRFCookieName)
	w.WriteHeader(http.StatusNoContent)
}

func (d HandlerDeps) me(w http.ResponseWriter, r *http.Request) {
	si, _ := FromContext(r.Context())
	sess, user, err := d.Processor.Me(r.Context(), si.SessionID)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Unauthenticated())
		return
	}
	resp := struct {
		Username           string    `json:"username"`
		SessionExpiresAt   time.Time `json:"session_expires_at"`
	}{Username: user.Username, SessionExpiresAt: sess.ExpiresAt}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (d HandlerDeps) changePassword(w http.ResponseWriter, r *http.Request) {
	si, _ := FromContext(r.Context())
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(400, "bad_request", "Invalid JSON body"))
		return
	}
	if len(body.NewPassword) < 12 {
		apierror.Write(w, apierror.StyleAction, apierror.New(422, "weak_password", "Password must be at least 12 characters"))
		return
	}
	if err := d.Processor.ChangePassword(r.Context(), si.SessionID, body.CurrentPassword, body.NewPassword); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(401, "invalid_credentials", "Current password incorrect"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d HandlerDeps) issueCSRF(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(d.CSRFCookieName); err != nil {
		token, _ := NewCSRFToken()
		d.setCSRFCookie(w, token)
	}
	c, _ := r.Cookie(d.CSRFCookieName)
	resp := struct {
		CSRFToken string `json:"csrf_token"`
	}{CSRFToken: c.Value}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (d HandlerDeps) setSessionCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.SessionCookieName,
		Value:    sid,
		Path:     d.cookiePath(),
		Expires:  time.Now().UTC().Add(d.SessionTimeout),
		MaxAge:   int(d.SessionTimeout.Seconds()),
		HttpOnly: true,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (d HandlerDeps) setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     d.CSRFCookieName,
		Value:    token,
		Path:     d.cookiePath(),
		Expires:  time.Now().UTC().Add(d.SessionTimeout),
		MaxAge:   int(d.SessionTimeout.Seconds()),
		HttpOnly: false,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (d HandlerDeps) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: d.cookiePath(), MaxAge: -1})
}

func (d HandlerDeps) cookiePath() string {
	if d.BasePath == "" {
		return "/"
	}
	return d.BasePath
}
```

- [ ] **Step 2: Handler tests**

Use `httptest.NewServer` with the chi router. Cases:
- `POST /api/v1/auth/login` with valid creds returns 204 + sets `harbormaster_session` (HttpOnly+Secure+SameSite=Lax) + sets `harbormaster_csrf` (not HttpOnly).
- Bad creds return 401 with `invalid_credentials`.
- 6th failure within 5 minutes returns 429 `too_many_attempts`.
- `GET /api/v1/auth/me` with session cookie returns username.
- `POST /api/v1/auth/logout` clears cookies and invalidates the session.
- `POST /api/v1/auth/password` requires correct `current_password`.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/auth
git commit -m "feat(backend/auth): /auth/* and /csrf handlers with cookie issuance and rate limiting"
```

---

### Task T2.4: `internal/setup` — first-run setup wizard backend

**Files:** `[skill scaffold]` for `setup` domain — but this is closer to a small action-style domain. Use only the minimum files (`processor.go`, `resource.go`, `rest.go`) plus tests. Also create:
- `apps/backend/internal/setup/mcconfig.go` — mc config parser (`version: "10"` only)
- `apps/backend/internal/setup/mcconfig_test.go`

- [ ] **Step 1: Implement `mcconfig.go`**

```go
// Package setup handles the first-run wizard (admin user + MinIO connection
// validation + persist), and the mc-config import helper.
package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// McAlias is a public projection of an mc config alias. Secret keys are
// never embedded here.
type McAlias struct {
	Name           string `json:"name"`
	Endpoint       string `json:"endpoint"`
	AccessKey      string `json:"access_key"`
	TLSSkipVerify  bool   `json:"tls_skip_verify"`
}

// ReadMcAliases returns the parsed aliases from the given path. Missing or
// unreadable files return ([], nil). Versions other than "10" return ([], nil)
// with a record of the version encountered in encounteredVersion.
func ReadMcAliases(path string) (aliases []McAlias, encounteredVersion string, err error) {
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		if errors.Is(rerr, fs.ErrNotExist) {
			return nil, "", nil
		}
		return nil, "", nil // best-effort
	}
	var raw struct {
		Version string `json:"version"`
		Aliases map[string]struct {
			URL           string `json:"url"`
			AccessKey     string `json:"accessKey"`
			SecretKey     string `json:"secretKey"`
			Insecure      bool   `json:"insecure"`
		} `json:"aliases"`
	}
	if uerr := json.Unmarshal(data, &raw); uerr != nil {
		return nil, "", nil
	}
	encounteredVersion = raw.Version
	if raw.Version != "10" {
		return nil, encounteredVersion, nil
	}
	out := make([]McAlias, 0, len(raw.Aliases))
	for name, a := range raw.Aliases {
		out = append(out, McAlias{Name: name, Endpoint: a.URL, AccessKey: a.AccessKey, TLSSkipVerify: a.Insecure})
	}
	return out, encounteredVersion, nil
}

// ReadMcAliasSecret returns the secret key for the named alias, or "" if the
// alias does not exist. Used only at setup-submit time.
func ReadMcAliasSecret(path, alias string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var raw struct {
		Version string `json:"version"`
		Aliases map[string]struct {
			SecretKey string `json:"secretKey"`
		} `json:"aliases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", err
	}
	if raw.Version != "10" {
		return "", errors.New("unsupported mc config version")
	}
	a, ok := raw.Aliases[alias]
	if !ok {
		return "", errors.New("alias not found")
	}
	return a.SecretKey, nil
}
```

- [ ] **Step 2: Tests for the parser**

```go
package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/setup"
)

func TestReadMcAliasesV10(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(p, []byte(`{
		"version": "10",
		"aliases": {
			"myminio": {"url": "https://minio.lan:9000", "accessKey": "AKIA", "secretKey": "SECRET", "insecure": false}
		}
	}`), 0o600))
	aliases, ver, err := setup.ReadMcAliases(p)
	require.NoError(t, err)
	require.Equal(t, "10", ver)
	require.Len(t, aliases, 1)
	require.Equal(t, "myminio", aliases[0].Name)
	require.Equal(t, "AKIA", aliases[0].AccessKey)
}

func TestReadMcAliasesWrongVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":"9","aliases":{"x":{}}}`), 0o600))
	aliases, ver, err := setup.ReadMcAliases(p)
	require.NoError(t, err)
	require.Equal(t, "9", ver)
	require.Empty(t, aliases)
}

func TestReadMcAliasesMissingFile(t *testing.T) {
	a, _, err := setup.ReadMcAliases("/nonexistent/path")
	require.NoError(t, err)
	require.Empty(t, a)
}
```

- [ ] **Step 3: Implement processor and HTTP handlers**

`internal/setup/processor.go`:

```go
package setup

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	"github.com/jtumidanski/Harbormaster/internal/crypto"
)

// Processor coordinates first-run setup.
type Processor struct {
	DB         *gorm.DB
	Cipher     *crypto.Cipher
	AuthProc   *auth.Processor
	ConnProc   *connection.Processor
	McPath     string
}

// ErrAlreadyInitialized is returned by SetupSubmit when setup_completed is true.
var ErrAlreadyInitialized = errors.New("setup already completed")

// SetupRequest carries the form fields submitted to POST /api/v1/setup.
type SetupRequest struct {
	Admin struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"admin"`
	MinIO connection.SubmitInput `json:"minio"`
}

// Submit performs the first-run sequence and is idempotent — it returns
// ErrAlreadyInitialized on the second call.
func (p *Processor) Submit(ctx context.Context, req SetupRequest, sourceIP string) error {
	if p.isInitialized(ctx) {
		return ErrAlreadyInitialized
	}
	if req.MinIO.FromMcAlias != "" {
		secret, err := ReadMcAliasSecret(p.McPath, req.MinIO.FromMcAlias)
		if err != nil {
			return errors.New("mc_alias_not_found")
		}
		req.MinIO.SecretKey = secret
		aliases, _, _ := ReadMcAliases(p.McPath)
		for _, a := range aliases {
			if a.Name == req.MinIO.FromMcAlias {
				req.MinIO.EndpointURL = a.Endpoint
				req.MinIO.AccessKey = a.AccessKey
				if req.MinIO.TLSSkipVerify == nil {
					v := a.TLSSkipVerify
					req.MinIO.TLSSkipVerify = &v
				}
				break
			}
		}
	}
	if err := p.ConnProc.Validate(ctx, req.MinIO); err != nil {
		return err
	}
	hash, err := auth.HashPassword(req.Admin.Password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tx := p.DB.WithContext(ctx).Begin()
	if err := tx.Exec(`INSERT INTO admin_users (username, password_hash, created_at, updated_at) VALUES (?,?,?,?)`,
		req.Admin.Username, hash, now, now).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := p.ConnProc.PersistInTx(ctx, tx, req.MinIO); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES (?,?,?)`,
		"setup_completed", "true", now).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	_ = sourceIP // audit hook added once audit.Processor is wired here
	return nil
}

func (p *Processor) isInitialized(ctx context.Context) bool {
	var v string
	p.DB.WithContext(ctx).Raw(`SELECT value FROM app_settings WHERE key = ?`, "setup_completed").Scan(&v)
	return v == "true"
}

// Status returns whether setup has been completed.
func (p *Processor) Status(ctx context.Context) bool {
	return p.isInitialized(ctx)
}
```

- [ ] **Step 4: Handlers `resource.go`** — `GET /api/v1/setup/status`, `GET /api/v1/setup/mc-aliases`, `POST /api/v1/setup`. Action-envelope errors. The two GETs are unauthenticated; POST is unauthenticated but returns 409 if already initialized.

- [ ] **Step 5: Tests**

Cover: status before/after submit, mc-aliases path with v10/v9/missing, submit with explicit creds, submit with alias form, submit-twice returns 409.

- [ ] **Step 6: Commit**

```bash
cd apps/backend && go test ./internal/setup/... -race
git add apps/backend/internal/setup
git commit -m "feat(backend/setup): first-run wizard endpoints + mc-config v10 parser"
```

---

### Task T2.5: `internal/connection` — minio_connections persistence + validation

**Files:** `[skill scaffold]` for `connection` domain (eight files + tests). Specifics:

- `model.go` — `Connection{ID uint, EndpointURL string, TLSSkipVerify bool, AccessKeyMasked string, SecretKeyPresent bool, CustomCAPEMPresent bool}` (the masked/`*Present` versions are what GET returns; ciphertexts are entity-only).
- `entity.go` — gorm struct for `minio_connections`. `Make`/`ToEntity` go through `Cipher.Decrypt`/`Cipher.Encrypt`.
- `provider.go` — `getSingleton(db) (Connection, encryptedCreds, error)` (returns plaintext via Cipher only inside the package).
- `administrator.go` — `upsertSingleton(db, encrypted) error`.
- `processor.go` — `Validate(ctx, SubmitInput) error` (runs the three probes), `PersistInTx(ctx, tx, SubmitInput) error`, `Update(ctx, SubmitInput) error` (Validate + Persist + Pool.Rebuild), `Get(ctx) (Connection, error)`, `Test(ctx, SubmitInput) (TestResult, error)`.
- `rest.go` — DTOs for `GET /api/v1/connection`, `PUT /api/v1/connection`, `POST /api/v1/connection/test` (all action shape).
- `resource.go` — chi route registration.

Add:
- `apps/backend/internal/connection/probe.go` — performs the TCP/list-buckets/admin-ping checks. Returns a typed `*apierror.Error` for failures with codes per `api-contracts.md` (`minio_unreachable`, `minio_invalid_credentials`, `minio_not_admin`).

- [ ] **Step 1: Implement `probe.go`**

```go
package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// SubmitInput is the body shape for setup and connection-update endpoints.
type SubmitInput struct {
	EndpointURL  string `json:"endpoint_url"`
	AccessKey    string `json:"access_key"`
	SecretKey    string `json:"secret_key"`
	TLSSkipVerify *bool `json:"tls_skip_verify,omitempty"`
	CustomCAPEM  string `json:"custom_ca_pem,omitempty"`
	FromMcAlias  string `json:"from_mc_alias,omitempty"`
}

// TestResult mirrors the api-contracts.md shape.
type TestResult struct {
	TCPConnect   any    `json:"tcp_connect"`
	ListBuckets  any    `json:"list_buckets"`
	AdminPing    any    `json:"admin_ping"`
	MinIOVersion string `json:"minio_version,omitempty"`
}

// Probe performs the three checks. Returns a structured TestResult plus an
// optional apierror.Error when any step fails (caller decides whether to return
// a 422 with the apierror, or a 200 with the result mix as in POST /test).
func Probe(ctx context.Context, in SubmitInput) (TestResult, *apierror.Error) {
	out := TestResult{}
	u, err := url.Parse(in.EndpointURL)
	if err != nil {
		return out, apierror.New(422, "minio_unreachable", "Endpoint URL is malformed").WithDetails(map[string]any{"underlying": err.Error()})
	}
	useTLS := u.Scheme == "https"
	host := u.Host
	// TCP connect with 3s timeout
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, derr := dialer.DialContext(ctx, "tcp", host)
	if derr != nil {
		out.TCPConnect = map[string]string{"failed": derr.Error()}
		return out, apierror.New(422, "minio_unreachable", "TCP connect failed").WithDetails(map[string]any{"underlying": derr.Error()})
	}
	_ = conn.Close()
	out.TCPConnect = "ok"

	skipVerify := false
	if in.TLSSkipVerify != nil {
		skipVerify = *in.TLSSkipVerify
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: skipVerify} //nolint:gosec // operator-configurable
	if in.CustomCAPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(in.CustomCAPEM)) {
			return out, apierror.New(422, "minio_unreachable", "Custom CA PEM is not valid")
		}
		tlsCfg.RootCAs = pool
	}

	mc, err := miniogo.New(host, &miniogo.Options{
		Creds:  credentials.NewStaticV4(in.AccessKey, in.SecretKey, ""),
		Secure: useTLS,
	})
	if err != nil {
		return out, apierror.New(422, "minio_unreachable", "client init failed").WithDetails(map[string]any{"underlying": err.Error()})
	}
	if _, err := mc.ListBuckets(ctx); err != nil {
		// Distinguish auth errors
		msg := err.Error()
		switch {
		case contains(msg, "InvalidAccessKeyId"), contains(msg, "SignatureDoesNotMatch"):
			out.ListBuckets = map[string]string{"failed": msg}
			return out, apierror.New(422, "minio_invalid_credentials", "MinIO rejected the provided keys")
		default:
			out.ListBuckets = map[string]string{"failed": msg}
			return out, apierror.New(422, "minio_unreachable", "list buckets failed").WithDetails(map[string]any{"underlying": msg})
		}
	}
	out.ListBuckets = "ok"

	madm, err := madmin.NewWithOptions(host, &madmin.Options{Creds: credentials.NewStaticV4(in.AccessKey, in.SecretKey, ""), Secure: useTLS})
	if err != nil {
		return out, apierror.New(422, "minio_unreachable", "admin client init failed").WithDetails(map[string]any{"underlying": err.Error()})
	}
	info, err := madm.ServerInfo(ctx)
	if err != nil {
		msg := err.Error()
		if contains(msg, "AccessDenied") {
			out.AdminPing = map[string]string{"failed": msg}
			return out, apierror.New(422, "minio_not_admin", "Provided MinIO keys lack admin capability")
		}
		out.AdminPing = map[string]string{"failed": msg}
		return out, apierror.New(422, "minio_unreachable", "admin ping failed").WithDetails(map[string]any{"underlying": msg})
	}
	out.AdminPing = "ok"
	out.MinIOVersion = info.Mode
	return out, nil
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

var _ = errors.New
var _ = fmt.Errorf
```

(`ServerInfo` returns rich info but its fields vary by madmin version; the executing engineer should pick a real field for `MinIOVersion` — e.g., `info.MinioVersion` or `info.Mode` — adjusting after reading the `madmin-go` source. The tests cover the happy path.)

- [ ] **Step 2: Implement the rest of the seven-file pattern with `Cipher` integration**

`processor.go::Validate` calls `Probe` and returns the error; `PersistInTx` encrypts and writes; `Update` runs both then rebuilds `*minio.Pool`.

- [ ] **Step 3: Tests**

Unit-test `Probe` against:
- Bad endpoint URL → 422 minio_unreachable.
- TCP dial failure → 422 minio_unreachable.
- (Integration-only `//go:build integration`): real MinIO testcontainer for the happy path.

Unit-test `processor.PersistInTx`: encrypt round-trip; assert ciphertext in row, plaintext on read.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/connection
git commit -m "feat(backend/connection): minio_connections persistence, validation probe, dual-envelope error codes"
```

---

### Task T2.6: Mount auth + setup + connection routes in `serve`

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: Build the route registrar list and pass it into `server.Deps.APIRoutes`**

In `runServe`, after constructing the cipher, audit processor, and minio pool:

```go
	authProc := auth.NewProcessor(gdb, logger)
	limiter := auth.NewLoginRateLimiter(5*time.Minute, 5)
	pool := hmminio.NewEmpty()
	connProc := connection.NewProcessor(gdb, cipher, pool, logger)
	setupProc := setup.NewProcessor(gdb, cipher, authProc, connProc, cfg.McConfigPath, logger)

	style := apierror.StyleAction

	publicRoutes := func(r chi.Router) {
		r.Group(func(g chi.Router) {
			g.Get("/setup/status", setupProc.HandleStatus)
			g.Get("/setup/mc-aliases", setupProc.HandleMcAliases)
			g.Post("/setup", setupProc.HandleSubmit)
			g.Post("/auth/login", auth.HandlerDeps{
				Processor: authProc, RateLimiter: limiter,
				SessionCookieName: cfg.SessionCookieName, CSRFCookieName: "harbormaster_csrf",
				BasePath: cfg.BasePath, SessionTimeout: cfg.SessionTimeout, Secure: true,
			}.LoginHandler())
		})
	}

	protectedRoutes := func(r chi.Router) {
		r.Use(auth.RequireSession(cfg.SessionCookieName, authProc, style))
		r.Use(auth.RequireCSRF("harbormaster_csrf", style))
		r.Mount("/auth", authProtectedRouter(authProc, cfg))
		r.Mount("/connection", connProc.Routes())
		// further mounts added in M3+
	}

	deps := server.Deps{
		Logger:    logger,
		APIRoutes: []func(chi.Router){publicRoutes, protectedRoutes},
	}
```

(The handler factory naming above is illustrative; the executing engineer aligns it with the actual exported names from auth/setup/connection packages from T2.3–T2.5. The intent is: setup + login are public; everything else is gated.)

- [ ] **Step 2: Tests**

End-to-end test in `apps/backend/cmd/harbormaster/serve_test.go`:
- Boot server with temp data dir.
- GET `/api/v1/setup/status` → `{"initialized":false}`.
- POST `/api/v1/setup` with valid stub creds (use a httptest MinIO if needed, or temporarily stub Probe via dep injection) → 201.
- GET `/api/v1/setup/status` → `{"initialized":true}`.
- POST `/api/v1/auth/login` with the new admin → 204, gets cookies.
- GET `/api/v1/auth/me` with cookies → 200, returns the username.
- GET `/api/v1/connection` without cookies → 401.
- GET `/api/v1/connection` with cookies → 200.
- POST `/api/v1/connection/test` without CSRF header → 403.
- POST `/api/v1/connection/test` with CSRF header → 200 (probe result).

For Probe stubbing: introduce a `connection.Prober` interface (function-typed field on `Processor`) so tests can inject a fake. Document this in the package comment.

- [ ] **Step 3: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): wire setup, auth, and connection routes into the HTTP server"
```

---

### Task T2.7: `/readyz` reflects migration + MinIO probe status

**Files:**
- Modify: `apps/backend/internal/server/server.go` (accept a readiness function)
- Modify: `apps/backend/internal/server/health.go`
- Modify: `apps/backend/cmd/harbormaster/serve.go` (provide the probe)

- [ ] **Step 1: Extend `Deps` with `Ready func(ctx) (ok bool, reason string)`**

`readyz` calls it and returns `503 {error:{code:"not_ready", message:reason}}` when false.

- [ ] **Step 2: In `runServe`, attach a 10-s ticker that probes `madmin.ServerInfo` and caches the last-OK timestamp; `Ready` returns true iff cache is fresh (< 30 s).**

If `setup_completed` is false, `Ready` returns true (no MinIO bound yet).

- [ ] **Step 3: Tests**

End-to-end: `/readyz` returns 200 before setup, 503 if probe stale, 200 once probe succeeds.

- [ ] **Step 4: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/server): /readyz reports migration + MinIO probe status"
```

---

### Task T2.8: Frontend — base API client + CSRF injection + dual envelope handling

**Files:**
- Create: `apps/frontend/src/lib/api/client.ts`
- Create: `apps/frontend/src/lib/api/errors.ts`
- Create: `apps/frontend/src/lib/api/keys.ts`
- Create: `apps/frontend/src/lib/api/client.test.ts`

- [ ] **Step 1: Write failing test for the client (use msw or a manual fetch shim)**

Use a fetch shim:

```ts
import { describe, it, expect, beforeEach, vi } from "vitest";
import { api } from "./client";

describe("api", () => {
  beforeEach(() => {
    document.cookie = "harbormaster_csrf=test-token; Path=/";
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      return new Response(JSON.stringify({ data: { type: "buckets", id: "x", attributes: { name: "x" } } }), {
        status: 200,
        headers: { "Content-Type": "application/vnd.api+json" },
      });
    }));
  });

  it("sends CSRF header on POST", async () => {
    await api.post("/api/v1/buckets", { data: {} });
    const call = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    const init = call[1] as RequestInit;
    expect((init.headers as Record<string,string>)["X-CSRF-Token"]).toBe("test-token");
  });

  it("does not send CSRF header on GET", async () => {
    await api.get("/api/v1/buckets");
    const init = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit;
    expect((init.headers as Record<string,string>)["X-CSRF-Token"]).toBeUndefined();
  });
});
```

- [ ] **Step 2: Implement `client.ts`**

```ts
import { AppError, parseErrorResponse } from "./errors";

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

const UNSAFE = new Set<Method>(["POST", "PUT", "PATCH", "DELETE"]);

function readCsrfCookie(): string {
  const m = document.cookie.match(/(?:^|;\s*)harbormaster_csrf=([^;]+)/);
  return m ? decodeURIComponent(m[1]) : "";
}

async function request<T>(method: Method, path: string, body?: unknown, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    Accept: "application/vnd.api+json, application/json",
    ...(init?.headers as Record<string, string> | undefined),
  };
  let bodyInit: BodyInit | undefined;
  if (body !== undefined) {
    if (body instanceof FormData) {
      bodyInit = body;
    } else {
      headers["Content-Type"] = "application/json";
      bodyInit = JSON.stringify(body);
    }
  }
  if (UNSAFE.has(method)) {
    const t = readCsrfCookie();
    if (t) headers["X-CSRF-Token"] = t;
  }
  const res = await fetch(path, {
    method,
    credentials: "include",
    body: bodyInit,
    headers,
    ...init,
  });
  if (!res.ok) {
    throw await parseErrorResponse(res);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  const ct = res.headers.get("Content-Type") ?? "";
  if (ct.includes("text/event-stream")) {
    return res as unknown as T;
  }
  return (await res.json()) as T;
}

export const api = {
  get:    <T>(path: string, init?: RequestInit) => request<T>("GET", path, undefined, init),
  post:   <T>(path: string, body?: unknown, init?: RequestInit) => request<T>("POST", path, body, init),
  put:    <T>(path: string, body?: unknown, init?: RequestInit) => request<T>("PUT", path, body, init),
  patch:  <T>(path: string, body?: unknown, init?: RequestInit) => request<T>("PATCH", path, body, init),
  delete: <T>(path: string, body?: unknown, init?: RequestInit) => request<T>("DELETE", path, body, init),
};

export type { AppError };
```

- [ ] **Step 3: Implement `errors.ts`**

```ts
export type AppErrorDetails = Record<string, unknown>;

export class AppError extends Error {
  status: number;
  code: string;
  details?: AppErrorDetails;
  pointer?: string;

  constructor(opts: { status: number; code: string; message: string; details?: AppErrorDetails; pointer?: string }) {
    super(opts.message);
    this.status = opts.status;
    this.code = opts.code;
    this.details = opts.details;
    this.pointer = opts.pointer;
  }
}

export async function parseErrorResponse(res: Response): Promise<AppError> {
  let body: unknown = {};
  try { body = await res.json(); } catch { /* ignore */ }
  const b = body as Record<string, unknown>;
  if (Array.isArray(b.errors) && b.errors.length > 0) {
    const e = b.errors[0] as Record<string, unknown>;
    return new AppError({
      status: res.status,
      code: String(e.code ?? "unknown"),
      message: String(e.detail ?? e.title ?? res.statusText),
      pointer: ((e.source as Record<string, unknown> | undefined)?.pointer) as string | undefined,
    });
  }
  if (b.error && typeof b.error === "object") {
    const e = b.error as Record<string, unknown>;
    return new AppError({
      status: res.status,
      code: String(e.code ?? "unknown"),
      message: String(e.message ?? res.statusText),
      details: (e.details as AppErrorDetails | undefined),
    });
  }
  return new AppError({ status: res.status, code: "unknown", message: res.statusText });
}
```

- [ ] **Step 4: Implement `keys.ts` (key factories — minimal for now)**

```ts
export const authKeys = {
  me: () => ["auth", "me"] as const,
  setupStatus: () => ["setup", "status"] as const,
  mcAliases: () => ["setup", "mc-aliases"] as const,
  csrf: () => ["auth", "csrf"] as const,
};

export const connectionKeys = {
  detail: () => ["connection", "detail"] as const,
};
```

- [ ] **Step 5: Run tests + commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src/lib
git commit -m "feat(frontend/api): typed fetch client with CSRF injection and dual error envelope"
```

---

### Task T2.9: Frontend — React Query setup + AuthContext + ThemeProvider + AppShell

**Files:**
- Create: `apps/frontend/src/main.tsx` (rewrite)
- Create: `apps/frontend/src/App.tsx` (rewrite)
- Create: `apps/frontend/src/routes.tsx`
- Create: `apps/frontend/src/context/AuthContext.tsx`
- Create: `apps/frontend/src/context/ThemeProvider.tsx`
- Create: `apps/frontend/src/components/AppShell.tsx`
- Create: `apps/frontend/src/lib/hooks/api/useMe.ts`
- Create: `apps/frontend/src/lib/hooks/api/useSetupStatus.ts`

- [ ] **Step 1: Rewrite `main.tsx`**

```tsx
import "./styles/index.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import App from "./App";
import { ThemeProvider } from "./context/ThemeProvider";
import { AuthProvider } from "./context/AuthContext";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, err) => {
        const status = (err as { status?: number }).status;
        if (status && status >= 400 && status < 500) return false;
        return failureCount < 2;
      },
    },
  },
});

const basename = (document.querySelector("base")?.getAttribute("href") ?? "/").replace(/\/$/, "") || "/";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter basename={basename === "/" ? undefined : basename}>
          <AuthProvider>
            <App />
            <Toaster position="top-right" richColors closeButton />
          </AuthProvider>
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
```

- [ ] **Step 2: `context/ThemeProvider.tsx`**

```tsx
import { createContext, useContext, useEffect, useState, type PropsWithChildren } from "react";

type Theme = "light" | "dark" | "system";
type Ctx = { theme: Theme; setTheme: (t: Theme) => void; resolved: "light" | "dark" };

const ThemeCtx = createContext<Ctx | null>(null);

export function ThemeProvider({ children }: PropsWithChildren) {
  const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem("theme") as Theme) ?? "system");
  const [resolved, setResolved] = useState<"light" | "dark">("light");

  useEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      const r = theme === "system" ? (mql.matches ? "dark" : "light") : theme;
      document.documentElement.classList.toggle("dark", r === "dark");
      setResolved(r);
    };
    apply();
    mql.addEventListener("change", apply);
    return () => mql.removeEventListener("change", apply);
  }, [theme]);

  useEffect(() => { localStorage.setItem("theme", theme); }, [theme]);

  return <ThemeCtx.Provider value={{ theme, setTheme, resolved }}>{children}</ThemeCtx.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeCtx);
  if (!ctx) throw new Error("useTheme outside ThemeProvider");
  return ctx;
}
```

- [ ] **Step 3: `context/AuthContext.tsx`**

```tsx
import { createContext, useContext, useMemo, type PropsWithChildren } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";

type Me = { username: string; session_expires_at: string };
type Ctx = {
  me: Me | null;
  isLoading: boolean;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
};

const AuthCtx = createContext<Ctx | null>(null);

export function AuthProvider({ children }: PropsWithChildren) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: authKeys.me(),
    queryFn: async () => {
      try {
        return await api.get<Me>("/api/v1/auth/me");
      } catch (e) {
        if ((e as { status?: number }).status === 401) return null;
        throw e;
      }
    },
  });

  const value = useMemo<Ctx>(() => ({
    me: data ?? null,
    isLoading,
    refresh: async () => { await qc.invalidateQueries({ queryKey: authKeys.me() }); },
    logout: async () => {
      try { await api.post("/api/v1/auth/logout"); } finally { qc.clear(); }
    },
  }), [data, isLoading, qc]);

  return <AuthCtx.Provider value={value}>{children}</AuthCtx.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthCtx);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
```

- [ ] **Step 4: `App.tsx` and `routes.tsx`**

```tsx
// App.tsx
import { AppRoutes } from "./routes";
export default function App() { return <AppRoutes />; }
```

```tsx
// routes.tsx
import { Navigate, Route, Routes } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";
import { useAuth } from "@/context/AuthContext";
import { AppShell } from "@/components/AppShell";
import { LoginPage } from "@/features/auth/LoginPage";
import { SetupWizard } from "@/features/setup/SetupWizard";
import { BucketsPlaceholder } from "@/features/buckets/BucketsPlaceholder";

function useSetupStatus() {
  return useQuery({
    queryKey: authKeys.setupStatus(),
    queryFn: () => api.get<{ initialized: boolean }>("/api/v1/setup/status"),
  });
}

export function AppRoutes() {
  const { me, isLoading: meLoading } = useAuth();
  const status = useSetupStatus();
  if (status.isLoading || meLoading) return <div className="p-8">Loading…</div>;
  if (!status.data?.initialized) {
    return (
      <Routes>
        <Route path="/setup" element={<SetupWizard />} />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    );
  }
  if (!me) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route path="/" element={<Navigate to="/buckets" replace />} />
        <Route path="/buckets" element={<BucketsPlaceholder />} />
        <Route path="*" element={<div className="p-8">Not found</div>} />
      </Route>
    </Routes>
  );
}
```

- [ ] **Step 5: `components/AppShell.tsx`** — sidebar + header + outlet (Buckets / Users / Activity placeholders). Use shadcn primitives (`Button`, `Card`) added via shadcn CLI in T2.10 once needed; for M2 a plain JSX layout is acceptable.

- [ ] **Step 6: `features/buckets/BucketsPlaceholder.tsx`** — renders `<div className="p-8"><h1>Buckets</h1><p>Bucket list lands in M3.</p></div>`.

- [ ] **Step 7: Frontend tests**

`AuthContext.test.tsx`: mock `/auth/me` returning 401 → `me === null`. Mock returning 200 → `me.username` available.

`routes.test.tsx`: with `initialized=false`, all paths render `<SetupWizard/>`. With `initialized=true` + no me, all paths render `<LoginPage/>`. With both, `/` redirects to `/buckets` and renders the placeholder.

- [ ] **Step 8: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend): React Query, AuthContext, ThemeProvider, AppShell, routing guards"
```

---

### Task T2.10: Frontend — shadcn primitives needed for M2

**Files:** `apps/frontend/src/components/ui/*.tsx` (Button, Input, Card, Label, Form, Tabs, Dialog, Tooltip, Toast, Select). Use the shadcn CLI:

- [ ] **Step 1: Install primitives**

```bash
cd apps/frontend && npx shadcn-ui@latest add button input card label form tabs dialog tooltip select alert
```

- [ ] **Step 2: Verify build**

```bash
cd apps/frontend && npm run build && npm test
```

- [ ] **Step 3: Commit**

```bash
git add apps/frontend
git commit -m "feat(frontend): add shadcn primitives (button, input, card, form, dialog, tabs, tooltip, select, alert)"
```

---

### Task T2.11: Frontend — Setup wizard

**Files:**
- Create: `apps/frontend/src/features/setup/SetupWizard.tsx`
- Create: `apps/frontend/src/features/setup/AdminStep.tsx`
- Create: `apps/frontend/src/features/setup/MinIOStep.tsx`
- Create: `apps/frontend/src/features/setup/api.ts`
- Create: `apps/frontend/src/lib/schemas/setup.ts`
- Create: `apps/frontend/src/features/setup/SetupWizard.test.tsx`

- [ ] **Step 1: Zod schemas in `lib/schemas/setup.ts`**

```ts
import { z } from "zod";

export const adminSchema = z.object({
  username: z
    .string()
    .min(3, "must be at least 3 characters")
    .max(64)
    .regex(/^[a-z0-9._-]+$/, "lowercase letters, digits, dot, underscore, hyphen"),
  password: z.string().min(12, "must be at least 12 characters"),
  passwordConfirm: z.string(),
}).refine((v) => v.password === v.passwordConfirm, {
  path: ["passwordConfirm"],
  message: "passwords must match",
});

export const minioSchema = z.object({
  fromMcAlias: z.string().optional(),
  endpointUrl: z.string().url(),
  accessKey: z.string().min(1),
  secretKey: z.string().min(1),
  tlsSkipVerify: z.boolean().default(false),
  customCaPem: z.string().optional(),
});

export type AdminInput = z.infer<typeof adminSchema>;
export type MinIOInput = z.infer<typeof minioSchema>;
```

- [ ] **Step 2: Implement `SetupWizard.tsx`**

Two-step state machine: Step 1 (admin) → Step 2 (MinIO). On submit, POST `/api/v1/setup` with combined payload; on success, invalidate `authKeys.setupStatus` and redirect to `/login`.

- [ ] **Step 3: Implement `MinIOStep.tsx`**

Fetches `/api/v1/setup/mc-aliases`. If non-empty, renders a `<Select>` of alias names at the top; selecting one pre-fills the form. All fields remain editable. Pass `from_mc_alias` only if the operator did not edit endpoint/access key after selecting.

- [ ] **Step 4: Tests with msw**

Add msw to devDeps if not present:
```bash
cd apps/frontend && npm install -D msw@2.3.0
```

Test cases:
- Renders admin step first; submit advances to MinIO step.
- mc-aliases dropdown shows when API returns aliases.
- Submit POSTs to `/api/v1/setup` with combined payload; on 422 with `minio_unreachable`, shows toast and stays on Step 2.
- On 201, calls `queryClient.invalidateQueries` and navigates to `/login`.

- [ ] **Step 5: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/setup): two-step first-run wizard with mc-alias import"
```

---

### Task T2.12: Frontend — Login page + change-password page

**Files:**
- Create: `apps/frontend/src/features/auth/LoginPage.tsx`
- Create: `apps/frontend/src/features/auth/api.ts`
- Create: `apps/frontend/src/lib/schemas/auth.ts`
- Create: `apps/frontend/src/features/auth/LoginPage.test.tsx`
- Create: `apps/frontend/src/features/auth/ChangePasswordPage.tsx`
- Modify: `apps/frontend/src/routes.tsx` (add `/settings/account` route)

- [ ] **Step 1: Login schema and page**

`lib/schemas/auth.ts`:
```ts
import { z } from "zod";
export const loginSchema = z.object({
  username: z.string().min(1, "required"),
  password: z.string().min(1, "required"),
});
export type LoginInput = z.infer<typeof loginSchema>;
```

`LoginPage.tsx`: react-hook-form + zodResolver, posts to `/api/v1/auth/login` (which sets cookies), then calls `useAuth().refresh()` and `navigate("/buckets")`.

- [ ] **Step 2: ChangePasswordPage**

Form with `current_password`, `new_password`, `new_password_confirm`. POST `/api/v1/auth/password`. On 401, shows "current password incorrect."

- [ ] **Step 3: Tests**

- Bad credentials → renders the error toast and stays on `/login`.
- Rate-limited → renders the 429 message.
- Successful login redirects.
- Change password validates confirm-match client-side.

- [ ] **Step 4: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/auth): login and change-password pages"
```

---

### Task T2.13: Frontend — Connection settings page

**Files:**
- Create: `apps/frontend/src/features/connection/ConnectionSettingsPage.tsx`
- Create: `apps/frontend/src/features/connection/api.ts`
- Create: `apps/frontend/src/lib/schemas/connection.ts`
- Modify: `apps/frontend/src/routes.tsx` (add `/settings/connection`)
- Modify: `apps/frontend/src/components/AppShell.tsx` (settings menu link)

- [ ] **Step 1: Implement page**

Loads `GET /api/v1/connection` and shows: endpoint, TLS skip flag, masked access key, secret-present indicator. "Edit" button opens a form (same shape as MinIO step in setup) but routed through `PUT /api/v1/connection`. "Test connection" button calls `POST /api/v1/connection/test` and renders the three booleans + MinIO version.

- [ ] **Step 2: Tests + commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/connection): settings page with edit + test flows"
```

---

### Task T2.14: Backend — embed real SPA assets and rewrite `<base href>`

**Files:**
- Modify: `apps/backend/internal/server/spa.go`
- Create: `apps/backend/internal/server/embed.go`

- [ ] **Step 1: Implement embed.go**

```go
package server

import "embed"

//go:embed all:spa-dist
var spaFS embed.FS
```

- [ ] **Step 2: Implement spa.go** that serves `/assets/*`, `/favicon.*`, `/manifest.webmanifest` from `spaFS`, and falls back to `index.html` for GET + `Accept: text/html`. Rewrites `<base href="/" />` to the configured base path.

```go
package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func spaHandler(basePath string) http.Handler {
	dist, err := fs.Sub(spaFS, "spa-dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "SPA bundle missing", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(dist))
	index, _ := fs.ReadFile(dist, "index.html")
	indexWithBase := rewriteBase(index, basePath)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		clean := path.Clean(r.URL.Path)
		// Serve known asset paths directly
		if isAssetPath(clean) {
			fileServer.ServeHTTP(w, r)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(indexWithBase)
	})
}

func isAssetPath(p string) bool {
	switch {
	case strings.HasPrefix(p, "/assets/"):
		return true
	case p == "/favicon.ico", p == "/favicon.svg":
		return true
	case p == "/manifest.webmanifest":
		return true
	}
	return false
}

func rewriteBase(html []byte, basePath string) []byte {
	if basePath == "" || basePath == "/" {
		return html
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return bytes.ReplaceAll(html, []byte(`<base href="/"`), []byte(`<base href="`+basePath+`"`))
}
```

- [ ] **Step 3: Adjust `index.html`**

In `apps/frontend/index.html`, add a `<base href="/" />` element inside `<head>` before script tags.

- [ ] **Step 4: Wire the Docker build to populate the embed dir**

Already configured in `Dockerfile` T0.7 (`COPY --from=frontend /src/dist ./internal/server/spa-dist`). Confirm.

- [ ] **Step 5: Verify**

```bash
docker buildx build -f deploy/docker/Dockerfile -t hm:m2 .
docker run --rm -p 8080:8080 -v $(pwd)/data:/var/lib/harbormaster hm:m2 &
sleep 2
curl -s http://localhost:8080/ -H "Accept: text/html" | grep "<title>"
# Expected: <title>Harbormaster</title>
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/setup/status
# Expected: 200
kill %1
```

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/server apps/frontend/index.html
git commit -m "feat(backend/server): embed SPA via go:embed, rewrite <base href> on subpath deployments"
```

---

### Task T2.15: M2 end-to-end smoke test

- [ ] **Step 1: Bring up the stack**

```bash
docker buildx build -f deploy/docker/Dockerfile -t hm:m2 .
cd deploy/docker && docker compose up
```

- [ ] **Step 2: In a browser, walk the wizard**

1. Visit `http://localhost:8080/` → SetupWizard.
2. Complete admin step.
3. Use a throwaway MinIO docker container as the target:
   ```bash
   docker run --rm -p 9000:9000 -p 9001:9001 \
     -e MINIO_ROOT_USER=admin -e MINIO_ROOT_PASSWORD=admin12345 \
     minio/minio:RELEASE.2025-01-01T00-00-00Z server /data --console-address ":9001"
   ```
4. Enter `http://host.docker.internal:9000` + creds.
5. Submit → redirected to `/login`.
6. Log in → land on `/buckets` placeholder.
7. Visit `/settings/connection` → see masked access key.

- [ ] **Step 3: Tear down + commit any fixes uncovered**

```bash
docker compose down
git status
```

---

### Task T2.16: Audit-event integration in setup/auth/connection

**Files:**
- Modify: `apps/backend/internal/setup/processor.go`
- Modify: `apps/backend/internal/auth/processor.go`
- Modify: `apps/backend/internal/connection/processor.go`

- [ ] **Step 1: Wire `audit.Processor` into each domain processor**

- Setup's `Submit` writes `session.login` on the implicit first login (none yet — but writes nothing; the explicit login records it).
- Auth's `Login` writes `session.login` (outcome=success) or `session.login_failed` (failure).
- Auth's `Logout` writes `session.logout`.
- Auth's `ChangePassword` writes `admin.password.change`.
- Connection's `Update` writes `connection.update`; `Test` writes `connection.test` only on failure (success is silent — too noisy otherwise).

- [ ] **Step 2: Tests assert that the expected rows are written and contain no secrets**

- [ ] **Step 3: Commit**

```bash
git add apps/backend
git commit -m "feat(backend): wire audit events into setup/auth/connection flows"
```

---

### Task T2.17: M2 verification

- [ ] **Step 1: Full local matrix**

```bash
cd apps/backend && go vet ./... && go test -race ./... && golangci-lint run && CGO_ENABLED=0 go build ./...
cd ../frontend && npm run lint && npm run format && npm test && npm run build
cd ../.. && docker buildx build -f deploy/docker/Dockerfile -t hm:m2 .
```

- [ ] **Step 2: Code review via `superpowers:requesting-code-review`**

Reviewers: backend + frontend + plan-adherence. Address findings.

---

### Task T2.18: Tag M2

```bash
git tag m2-complete
```

---

## Milestone M3 — Buckets + objects + lifecycle

Goal: full bucket and object workflows end-to-end against a real MinIO instance, including the persistent empty-bucket job with SSE progress and the lifecycle rule builder.

### Task T3.1: `internal/buckets` domain scaffold

**Files:** `[skill scaffold]` for `buckets` domain. Specifics:

- `model.go`:
  ```go
  type Bucket struct {
      Name                string
      CreatedAt           time.Time
      EstimatedBytes      int64
      ObjectCount         int64
      VersioningEnabled   bool
      HasLifecycleRules   bool
      PublicAccess        PublicAccess
      Quota               *Quota
  }
  type PublicAccess string
  const (
      PublicAccessPrivate         PublicAccess = "private"
      PublicAccessPublicRead      PublicAccess = "public-read"
      PublicAccessPublicReadWrite PublicAccess = "public-read-write"
  )
  type Quota struct {
      Kind      QuotaKind // "hard" | "fifo"
      Bytes     int64
      UsedBytes int64
  }
  type QuotaKind string
  ```

- `entity.go` — not a DB-backed table; buckets live in MinIO. `Bucket` is built directly from `madmin`/`minio-go` results. The seven-file skill pattern still applies but `entity.go` and `administrator.go` are minimal (no SQL writes for the bucket domain — only the MinIO clients are used). Per the skill exception ("If the action is simple, fold into the parent domain's processor"), the bucket processor calls the MinIO Pool directly. `provider.go` exposes read functions that take `(*madmin.AdminClient, *minio.Client, ctx)` and `administrator.go` exposes write functions with the same shape.

- `builder.go` — `NewBucketBuilder().Name(s).Build()` validates per MinIO rules (3–63 chars; lowercase alnum + `.-`; no leading/trailing `.`; no adjacent `..`; not an IP address).

- `processor.go` — methods:
  - `List(ctx) ([]Bucket, error)` — fans out `ListBuckets` + per-bucket `BucketInfo` (admin) + `GetBucketVersioning` + `GetBucketLifecycle` + `GetBucketPolicy` + `GetBucketQuota`. Use a small parallel fan-out with `errgroup`. Limit concurrency to 10.
  - `Get(ctx, name) (Bucket, error)` — same info for a single bucket.
  - `Create(ctx, name, opts) (Bucket, error)` — uses `MakeBucket` then optionally enables versioning, sets public-access, sets quota, applies a lifecycle template.
  - `Delete(ctx, name, confirmName) error` — re-checks emptiness via `ListObjectsV2` with `MaxKeys=1`; returns `apierror` `409 bucket_not_empty` otherwise. No force flag in v1.
  - `SetVersioning(ctx, name, enabled bool) error`.
  - `SetPublicAccess(ctx, name, mode, confirmName) error`. Materializes one of three canned bucket policies (`policies.PolicyPrivate`/`PolicyPublicRead`/`PolicyPublicReadWrite`); `private` removes the policy.
  - `SetQuota(ctx, name, kind, bytes) error`. Enforces `fifo_requires_versioning_off`.

- `resource.go` — chi router with the bucket REST endpoints (JSON:API for collection/single, action style for `/versioning`, `/public-access`, `/quota`, `/empty`).

- `rest.go` — `BucketResource` implementing `jsonapi.Resource`; request DTOs (`CreateRequest`, `PublicAccessRequest`, `QuotaRequest`, `DeleteRequest`, `VersioningRequest`).

- [ ] **Step 1: Scaffold all eight files.**

- [ ] **Step 2: Tests** (unit-only; integration handled separately):
  - `TestBuilderRejectsInvalidNames` (table-driven; includes `192.168.0.1`, `Photos`, `a`, `..a`, `a..b`, 64-char names).
  - `TestSetQuotaRejectsFifoWithVersioning` — uses a stubbed admin client.
  - `TestDeleteReturnsConflictOnNonEmpty` — stubbed S3 client returns one key for `ListObjectsV2(MaxKeys=1)`.

The MinIO clients in tests use a small interface (`type adminAPI interface { ... }`, `type s3API interface { ... }`) so they're mockable. The integration suite uses the real clients (T3.28).

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/buckets/... -race
git add apps/backend/internal/buckets
git commit -m "feat(backend/buckets): bucket domain (list/get/create/delete/versioning/public-access/quota) with validation"
```

---

### Task T3.2: Public-access canned policy templates

**Files:**
- Create: `apps/backend/internal/policies/bucket_canned.go`
- Create: `apps/backend/internal/policies/bucket_canned_test.go`

(`internal/policies` is also used by M4 for user policies; here we add the bucket-canned-policy helpers.)

- [ ] **Step 1: Implement**

```go
// Package policies defines IAM and bucket-canned policy templates and renders
// them into the JSON shapes that MinIO accepts.
package policies

import (
	"encoding/json"
	"fmt"
)

// BucketPolicyFor returns the JSON for the canned bucket policy matching the
// public-access mode. Returns ("", nil) for "private" — callers should call
// SetBucketPolicy(name, "") (or RemoveBucketPolicy) instead.
func BucketPolicyFor(bucket, mode string) (string, error) {
	switch mode {
	case "private":
		return "", nil
	case "public-read":
		return render(bucket, []string{"s3:GetObject", "s3:ListBucket"})
	case "public-read-write":
		return render(bucket, []string{"s3:GetObject", "s3:ListBucket", "s3:PutObject", "s3:DeleteObject"})
	}
	return "", fmt.Errorf("unknown public-access mode %q", mode)
}

func render(bucket string, actions []string) (string, error) {
	type stmt struct {
		Effect    string   `json:"Effect"`
		Principal any      `json:"Principal"`
		Action    []string `json:"Action"`
		Resource  []string `json:"Resource"`
	}
	type doc struct {
		Version   string `json:"Version"`
		Statement []stmt `json:"Statement"`
	}
	d := doc{
		Version: "2012-10-17",
		Statement: []stmt{{
			Effect:    "Allow",
			Principal: map[string]any{"AWS": []string{"*"}},
			Action:    actions,
			Resource:  []string{"arn:aws:s3:::" + bucket, "arn:aws:s3:::" + bucket + "/*"},
		}},
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 2: Tests round-trip the three modes; `private` returns empty string; unknown mode errors**

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/policies/... -race
git add apps/backend/internal/policies
git commit -m "feat(backend/policies): canned bucket policies for private/public-read/public-read-write"
```

---

### Task T3.3: Bucket REST handlers wire-up

**Files:**
- Complete `apps/backend/internal/buckets/resource.go`
- Tests in `apps/backend/internal/buckets/resource_test.go`

- [ ] **Step 1: Handler list with style annotations**

Resource endpoints (StyleJSONAPI): `GET /buckets`, `POST /buckets`, `GET /buckets/{name}`, `DELETE /buckets/{name}`.

Action endpoints (StyleAction): `PUT /buckets/{name}/versioning`, `PUT /buckets/{name}/public-access`, `PUT /buckets/{name}/quota`, `POST /buckets/{name}/empty` (SSE — added in T3.7), `GET /buckets/{name}/lifecycle-rules` (StyleJSONAPI — actually resource; added in T3.20).

- [ ] **Step 2: Handler-level tests** verify the dual envelopes, audit-event writes per action, and error codes per api-contracts.md.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/buckets
git commit -m "feat(backend/buckets): JSON:API + action HTTP handlers with envelope-correct errors"
```

---

### Task T3.4: `internal/jobs/bucketempty` — worker, single-flight, persistent state

**Files:**
- Create: `apps/backend/internal/jobs/bucketempty/service.go`
- Create: `apps/backend/internal/jobs/bucketempty/worker.go`
- Create: `apps/backend/internal/jobs/bucketempty/repo.go`
- Create: `apps/backend/internal/jobs/bucketempty/service_test.go`

- [ ] **Step 1: Implement `repo.go`**

GORM-backed CRUD for `bucket_empty_jobs`. Methods:
- `InsertRunning(db, name, purgeVersions bool) (id string, err error)` — INSERT with `state='running'`; relies on the partial unique index to enforce single-flight (any duplicate INSERT returns the unique-constraint error).
- `UpdateProgress(db, id, deleted int64, estimatedTotal int64) error`.
- `MarkDone(db, id, deletedTotal int64) error`.
- `MarkError(db, id, msg string) error`.
- `FindRunning(db, bucket string) (job, error)` — returns the active row if any.
- `OrphanRunningAtStartup(db) (orphaned []job, err error)` — called by `runServe` to flip stale rows to `error="orphaned by restart"`.

- [ ] **Step 2: Implement `service.go`**

```go
package bucketempty

import (
	"context"
	"errors"
	"sync"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// Progress is emitted to SSE subscribers per RemoveObjects batch.
type Progress struct {
	Deleted        int64 `json:"deleted"`
	EstimatedTotal int64 `json:"estimated_total"`
}

// Result terminates a stream.
type Result struct {
	DeletedTotal int64
	DurationMS   int64
	ErrorMessage string
}

// Service coordinates empty-bucket jobs. One Service per process.
type Service struct {
	db     *gorm.DB
	pool   PoolGetter
	audit  AuditRecorder
	mu     sync.Mutex
	active map[string]*subscription
}

// PoolGetter returns the active MinIO client pair.
type PoolGetter interface {
	Get(ctx context.Context) (*madmin.AdminClient, *miniogo.Client, error)
}

// AuditRecorder writes the terminal bucket.empty audit event.
type AuditRecorder interface {
	Record(ctx context.Context, action, target, outcome string, payload map[string]any, errMsg string)
}

type subscription struct {
	jobID    string
	bucket   string
	progress chan Progress
	done     chan Result
	closeMu  sync.Once
	subs     []chan Progress
}

// New constructs a Service. Call OrphanRunningAtStartup separately.
func New(db *gorm.DB, pool PoolGetter, audit AuditRecorder) *Service {
	return &Service{db: db, pool: pool, audit: audit, active: map[string]*subscription{}}
}

// StartOrAttach starts a new job for the bucket, or attaches the caller to an
// in-flight job. progressCh emits per-batch updates; doneCh emits the terminal
// Result exactly once.
func (s *Service) StartOrAttach(ctx context.Context, bucket string, purgeVersions bool) (progressCh <-chan Progress, doneCh <-chan Result, err error) {
	s.mu.Lock()
	if sub, ok := s.active[bucket]; ok {
		pc := make(chan Progress, 16)
		sub.subs = append(sub.subs, pc)
		s.mu.Unlock()
		return pc, sub.done, nil
	}
	id, ierr := insertRunning(s.db, bucket, purgeVersions)
	if ierr != nil {
		s.mu.Unlock()
		return nil, nil, ierr
	}
	sub := &subscription{
		jobID:    id,
		bucket:   bucket,
		progress: make(chan Progress, 16),
		done:     make(chan Result, 1),
		subs:     []chan Progress{make(chan Progress, 16)},
	}
	s.active[bucket] = sub
	s.mu.Unlock()
	go s.run(context.Background(), sub, purgeVersions) // detached from request ctx
	return sub.subs[0], sub.done, nil
}

// run executes the empty operation and fans out progress to subscribers.
func (s *Service) run(ctx context.Context, sub *subscription, purgeVersions bool) {
	start := time.Now()
	madm, mc, err := s.pool.Get(ctx)
	if err != nil {
		s.terminate(sub, Result{ErrorMessage: err.Error()}, start, 0, purgeVersions)
		return
	}
	_ = madm
	versioned, _ := isVersioned(ctx, mc, sub.bucket)
	var (
		deleted int64
		batchN  = 1000
	)
	if purgeVersions && versioned {
		err = drainVersions(ctx, mc, sub.bucket, batchN, func(n int64) { deleted += n; s.broadcast(sub, Progress{Deleted: deleted}) })
	} else {
		err = drainObjects(ctx, mc, sub.bucket, batchN, func(n int64) { deleted += n; s.broadcast(sub, Progress{Deleted: deleted}) })
	}
	res := Result{DeletedTotal: deleted, DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		res.ErrorMessage = err.Error()
	}
	s.terminate(sub, res, start, deleted, purgeVersions)
}

func (s *Service) broadcast(sub *subscription, p Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range sub.subs {
		select { case ch <- p: default: }
	}
	_ = updateProgress(s.db, sub.jobID, p.Deleted, p.EstimatedTotal)
}

func (s *Service) terminate(sub *subscription, r Result, start time.Time, deleted int64, purgeVersions bool) {
	s.mu.Lock()
	delete(s.active, sub.bucket)
	for _, ch := range sub.subs {
		close(ch)
	}
	sub.done <- r
	close(sub.done)
	s.mu.Unlock()
	if r.ErrorMessage != "" {
		_ = markError(s.db, sub.jobID, r.ErrorMessage)
	} else {
		_ = markDone(s.db, sub.jobID, r.DeletedTotal)
	}
	s.audit.Record(context.Background(), "bucket.empty", sub.bucket,
		outcome(r.ErrorMessage), map[string]any{
			"deleted_count":               r.DeletedTotal,
			"duration_ms":                 r.DurationMS,
			"purge_versions":              purgeVersions,
			"versioning_enabled_at_start": deleted >= 0 && purgeVersions,
		}, r.ErrorMessage)
}

func outcome(msg string) string {
	if msg != "" {
		return "failure"
	}
	return "success"
}

// (drainObjects / drainVersions implementations call mc.ListObjectsV2 /
// mc.ListObjectVersions, batch into mc.RemoveObjects with 1000-key chunks,
// and invoke onBatch(deletedCount) per batch.)

var (
	errExisting = errors.New("an empty-bucket job is already running")
)
```

- [ ] **Step 3: Implement `worker.go` with `drainObjects` and `drainVersions`**

Use `minio.Client.ListObjectsV2` paginated; collect 1000 keys, build a `chan minio.ObjectInfo` for `RemoveObjects`. For versions, use `ListObjectVersions` and the versioned remove form. Each batch invokes `onBatch(int64)` to report increment.

- [ ] **Step 4: Tests with a fake S3 client**

Cases:
- New job → INSERT row, run completes, MarkDone called.
- Concurrent `StartOrAttach` → second call returns the same channels (no second job started).
- Mid-flight error → MarkError, audit `outcome=failure`.
- `OrphanRunningAtStartup` on a DB with one `running` row flips it to `error`.

- [ ] **Step 5: Commit**

```bash
cd apps/backend && go test ./internal/jobs/bucketempty/... -race
git add apps/backend/internal/jobs
git commit -m "feat(backend/jobs): persistent empty-bucket worker with single-flight and SSE-friendly fanout"
```

---

### Task T3.5: Bucket-empty SSE handler

**Files:**
- Create: `apps/backend/internal/buckets/empty_handler.go`
- Create: `apps/backend/internal/buckets/empty_handler_test.go`

- [ ] **Step 1: Implement handler**

```go
package buckets

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/sse"
)

// EmptyHandler streams SSE progress for the empty-bucket operation.
type EmptyHandler struct{ Service *bucketempty.Service }

// ServeHTTP wires up the SSE response and pumps progress events.
func (h EmptyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		ConfirmName   string `json:"confirm_name"`
		PurgeVersions bool   `json:"purge_versions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(400, "bad_request", "Invalid JSON body"))
		return
	}
	if body.ConfirmName != name {
		apierror.Write(w, apierror.StyleAction, apierror.New(403, "confirm_name_mismatch", "Provided confirm_name does not match bucket name"))
		return
	}
	sw, err := sse.New(w)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal("SSE not supported"))
		return
	}
	progress, done, err := h.Service.StartOrAttach(r.Context(), name, body.PurgeVersions)
	if err != nil {
		_ = sw.Event("error", map[string]string{"message": err.Error()})
		return
	}
	stop := make(chan struct{})
	defer close(stop)
	go sw.StartHeartbeat(stop, 15*time.Second)
	for {
		select {
		case p, ok := <-progress:
			if !ok {
				select {
				case res := <-done:
					if res.ErrorMessage != "" {
						_ = sw.Event("error", map[string]string{"message": res.ErrorMessage})
					} else {
						_ = sw.Event("done", map[string]any{"deleted_total": res.DeletedTotal, "duration_ms": res.DurationMS})
					}
					return
				case <-time.After(time.Minute):
					_ = sw.Event("error", map[string]string{"message": "terminal state lost"})
					return
				}
			}
			_ = sw.Event("progress", p)
		case <-r.Context().Done():
			return
		}
	}
}
```

- [ ] **Step 2: Tests**

Use `httptest.NewServer`; assert event sequence (`progress` × N, then `done`). Confirm `confirm_name_mismatch` returns plain JSON (not SSE) when validation fails.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/buckets
git commit -m "feat(backend/buckets): /empty SSE handler with confirm-name gate and heartbeats"
```

---

### Task T3.6: Wire bucket routes + orphan reset at startup

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: Construct the `bucketempty.Service`, call `OrphanRunningAtStartup`, mount bucket routes under protectedRoutes**

```go
	bucketEmpty := bucketempty.New(gdb, pool, auditProc)
	if err := bucketempty.OrphanRunningAtStartup(gdb, auditProc); err != nil {
		logger.Warn().Err(err).Msg("orphan-running cleanup failed")
	}
	bucketsProc := buckets.NewProcessor(pool, bucketEmpty, auditProc, logger)
	// ...
	protectedRoutes := func(r chi.Router) {
		r.Use(auth.RequireSession(...))
		r.Use(auth.RequireCSRF(...))
		r.Mount("/buckets", bucketsProc.Routes())
		// ...
	}
```

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): mount /buckets routes + orphan cleanup of stale empty-bucket jobs"
```

---

### Task T3.7: `internal/objects` — list (paginated continuation tokens)

**Files:** `[skill scaffold]` for `objects` domain. Specifics:

- `model.go`:
  ```go
  type Entry struct {
      Key          string
      Size         int64
      LastModified time.Time
      ContentType  string
      ETag         string
  }
  type Prefix struct { Name string }
  type ListResult struct {
      Entries   []Entry
      Prefixes  []Prefix
      NextToken string
  }
  ```

- `processor.go` methods (M3.7 covers list only; other methods in subsequent tasks):
  - `List(ctx, bucket, prefix, delimiter, pageSize int, pageToken string) (ListResult, error)`.

- `rest.go` defines two resource types: `object_entries` and `object_prefixes`, sharing the `data: []` array. Use a custom encoder path that emits two-type resources (the encoder in T1.6 already supports any `[]Resource` so just have both types implement `jsonapi.Resource`).

- [ ] **Step 1: Scaffold + implement List**

```go
func (p *Processor) List(ctx context.Context, bucket, prefix, delimiter string, pageSize int, pageToken string) (ListResult, error) {
    _, mc, err := p.pool.Get(ctx)
    if err != nil { return ListResult{}, err }
    if pageSize <= 0 || pageSize > 1000 { pageSize = 100 }
    opts := miniogo.ListObjectsOptions{
        Prefix:               prefix,
        Recursive:            delimiter == "",
        MaxKeys:              pageSize,
        StartAfter:           "",
    }
    // The MinIO Go SDK exposes ListObjectsV2 via the ListObjects channel; for
    // an explicit continuation-token shape we instead use the lower-level
    // ListObjectsV2Result via mc.Core. (Use core.ListObjectsV2 to retain the
    // continuation token directly.)
    core := miniogo.Core{Client: mc}
    res, err := core.ListObjectsV2(bucket, prefix, pageToken, "", delimiter, pageSize)
    if err != nil { return ListResult{}, err }
    out := ListResult{NextToken: res.NextContinuationToken}
    for _, o := range res.Contents {
        if strings.HasSuffix(o.Key, "/") { continue }
        out.Entries = append(out.Entries, Entry{Key: o.Key, Size: o.Size, LastModified: o.LastModified, ContentType: o.ContentType, ETag: o.ETag})
    }
    for _, p := range res.CommonPrefixes {
        out.Prefixes = append(out.Prefixes, Prefix{Name: p.Prefix})
    }
    return out, nil
}
```

- [ ] **Step 2: Handler emits both resource types**

```go
// GET /api/v1/buckets/{name}/objects
func (p *Processor) handleList(w http.ResponseWriter, r *http.Request) {
    bucket := chi.URLParam(r, "name")
    prefix := r.URL.Query().Get("prefix")
    delim := r.URL.Query().Get("delimiter")
    pageSize, _ := strconv.Atoi(r.URL.Query().Get("page[size]"))
    pageToken := r.URL.Query().Get("page[token]")
    res, err := p.List(r.Context(), bucket, prefix, delim, pageSize, pageToken)
    if err != nil { apierror.Write(w, apierror.StyleJSONAPI, apierror.Internal(err.Error())); return }
    items := make([]jsonapi.Resource, 0, len(res.Entries)+len(res.Prefixes))
    for _, e := range res.Entries { items = append(items, entryRes(e)) }
    for _, pf := range res.Prefixes { items = append(items, prefixRes(pf)) }
    enc := jsonapi.NewEncoder()
    w.Header().Set("Content-Type", "application/vnd.api+json")
    _ = enc.Collection(w, items, &jsonapi.Meta{Page: &jsonapi.Page{Size: pageSize, NextToken: res.NextToken}}, nil)
}
```

`entryRes` and `prefixRes` are small adapter types implementing `jsonapi.Resource`.

- [ ] **Step 3: Tests** with a stubbed `core.ListObjectsV2`. Cases: empty bucket, single page, paginated (token round-trip), delimiter set returns prefixes.

- [ ] **Step 4: Commit**

```bash
cd apps/backend && go test ./internal/objects/... -race
git add apps/backend/internal/objects
git commit -m "feat(backend/objects): paginated list with continuation tokens and prefix/entry rendering"
```

---

### Task T3.8: `internal/objects` — upload (multipart, capped)

**Files:**
- Modify: `apps/backend/internal/objects/processor.go`
- Modify: `apps/backend/internal/objects/resource.go`

- [ ] **Step 1: Implement `Upload(ctx, bucket, key string, body io.Reader, contentType string) (Entry, error)`**

```go
func (p *Processor) Upload(ctx context.Context, bucket, key string, body io.Reader, contentType string) (Entry, error) {
    _, mc, err := p.pool.Get(ctx)
    if err != nil { return Entry{}, err }
    info, err := mc.PutObject(ctx, bucket, key, body, -1, miniogo.PutObjectOptions{ContentType: contentType})
    if err != nil { return Entry{}, err }
    return Entry{Key: info.Key, Size: info.Size, LastModified: info.LastModified, ContentType: contentType, ETag: info.ETag}, nil
}
```

- [ ] **Step 2: Handler enforces upload cap**

```go
func (p *Processor) handleUpload(w http.ResponseWriter, r *http.Request) {
    bucket := chi.URLParam(r, "name")
    r.Body = http.MaxBytesReader(w, r.Body, p.cfg.UploadMaxBytes)
    if err := r.ParseMultipartForm(8 << 20); err != nil {
        var mbe *http.MaxBytesError
        if errors.As(err, &mbe) {
            apierror.Write(w, apierror.StyleJSONAPI, apierror.New(413, "upload_too_large",
                fmt.Sprintf("Upload exceeds the configured per-request cap of %d bytes", p.cfg.UploadMaxBytes)).WithDetails(map[string]any{"limit_bytes": p.cfg.UploadMaxBytes}))
            return
        }
        apierror.Write(w, apierror.StyleJSONAPI, apierror.New(400, "bad_request", err.Error()))
        return
    }
    key := r.FormValue("key")
    if key == "" { /* 422 */ return }
    f, fh, err := r.FormFile("file")
    if err != nil { /* 422 */ return }
    defer f.Close()
    ct := r.FormValue("content_type")
    if ct == "" { ct = fh.Header.Get("Content-Type") }
    e, err := p.Upload(r.Context(), bucket, key, f, ct)
    if err != nil { apierror.Write(w, apierror.StyleJSONAPI, apierror.Internal(err.Error())); return }
    // ... JSON:API encode and audit object.upload
}
```

- [ ] **Step 3: Tests** with httptest + a fake S3 client. Cases: under cap → 201; over cap → 413 with `details.limit_bytes`.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/objects
git commit -m "feat(backend/objects): multipart upload with HARBORMASTER_UPLOAD_MAX_BYTES enforcement"
```

---

### Task T3.9: `internal/objects` — delete + download (proxy + direct modes)

- [ ] **Step 1: Delete (`DELETE /api/v1/buckets/{name}/objects?key=...`)**

`Delete(ctx, bucket, key) error` calls `mc.RemoveObject(ctx, bucket, key, RemoveObjectOptions{})`. Handler returns 204. Audit `object.delete` with `{bucket, key}` (key is not sensitive — it's the object path).

- [ ] **Step 2: Download (`GET /api/v1/buckets/{name}/objects/download?key=...`)**

Two branches based on `cfg.DownloadProxyMode`:
- `proxy`: `obj, err := mc.GetObject(ctx, bucket, key, GetObjectOptions{})`. Set `Content-Type` (sniff), `Content-Length` (from `obj.Stat`), `Content-Disposition: attachment; filename="<basename>"`, `Cache-Control: private, no-store`. `io.Copy` to response. Audit `object.download_proxy` on completion (not on abort).
- `direct`: `url, err := mc.PresignedGetObject(ctx, bucket, key, 5*time.Minute, nil)`. `http.Redirect(w, r, url.String(), 307)`. No audit.

- [ ] **Step 3: Tests**

- Proxy mode: HTTP test server returns the bytes with expected headers.
- Direct mode: returns 307 with `Location` set; no body read.
- Audit table contains exactly one `object.download_proxy` event in proxy mode and zero in direct mode.

- [ ] **Step 4: Commit**

```bash
cd apps/backend && go test ./internal/objects/... -race
git add apps/backend/internal/objects
git commit -m "feat(backend/objects): delete + dual-mode download (proxy/direct)"
```

---

### Task T3.10: `internal/objects` — share-link minting

- [ ] **Step 1: Implement**

```go
func (p *Processor) MintShareLink(ctx context.Context, bucket, key string, expiresSeconds int) (string, time.Time, error) {
    _, mc, err := p.pool.Get(ctx)
    if err != nil { return "", time.Time{}, err }
    if expiresSeconds < 30 { expiresSeconds = 30 }
    if d := int(p.cfg.ShareLinkMaxTTL.Seconds()); expiresSeconds > d { expiresSeconds = d }
    expiry := time.Duration(expiresSeconds) * time.Second
    u, err := mc.PresignedGetObject(ctx, bucket, key, expiry, nil)
    if err != nil { return "", time.Time{}, err }
    return u.String(), time.Now().Add(expiry), nil
}
```

Handler shape per `api-contracts.md`. Audit `object.share_link.create` with `{bucket, key, expires_seconds}` — never the URL.

- [ ] **Step 2: Tests assert: (a) clamp behavior at 30 s lower bound; (b) clamp at upper bound = `ShareLinkMaxTTL`; (c) audit row does not contain the URL substring.**

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/objects
git commit -m "feat(backend/objects): share-link minting with server-side TTL clamp and audit-without-URL"
```

---

### Task T3.11: Mount object routes

**Files:** Modify `serve.go` to mount `/buckets/{name}/objects/*` and `/buckets/{name}/objects/share-links` under protected routes.

- [ ] **Step 1: Wire + commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): mount object routes under protected /buckets/{name}/objects/*"
```

---

### Task T3.12: `internal/lifecycle` — managed/unmanaged classifier + create/delete

**Files:** `[skill scaffold]` for `lifecycle` domain.

- `model.go`:
  ```go
  type Rule struct {
      ID       string
      Managed  bool
      Kind     string // "expiration" for managed v1
      Days     int
      Prefix   string
      Summary  string // for unmanaged
  }
  ```

- `processor.go`:
  - `List(ctx, bucket) ([]Rule, error)` — calls `mc.GetBucketLifecycle`, classifies each rule.
  - `Create(ctx, bucket, days int, prefix string) (Rule, error)`.
  - `Delete(ctx, bucket, ruleID string) error`.

- Classification:
  ```go
  var managedIDRE = regexp.MustCompile(`^harbormaster-expire-\d+d(-[a-z0-9.-]+)?$`)

  func classify(r lifecycle.Rule) Rule {
      if managedIDRE.MatchString(r.ID) && len(r.Transitions) == 0 &&
         r.AbortIncompleteMultipartUpload.DaysAfterInitiation == 0 &&
         r.Expiration.Days > 0 &&
         hasNoTagFilters(r) {
          return Rule{ID: r.ID, Managed: true, Kind: "expiration",
                      Days: int(r.Expiration.Days), Prefix: r.RuleFilter.Prefix}
      }
      return Rule{ID: r.ID, Managed: false, Summary: summarize(r)}
  }

  func summarize(r lifecycle.Rule) string {
      var parts []string
      if r.Expiration.Days > 0 { parts = append(parts, "Expiration") }
      if len(r.Transitions) > 0 { parts = append(parts, "Transition") }
      if r.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
          parts = append(parts, "AbortIncompleteMultipart")
      }
      tagN := countTagFilters(r)
      summary := fmt.Sprintf("Unmanaged rule (created outside Harbormaster) — %d actions: %s",
          len(parts), strings.Join(parts, ", "))
      if tagN > 0 {
          summary += fmt.Sprintf("; scoped to %d tag filter(s)", tagN)
      }
      return summary
  }
  ```

- `Create` generates ID `harbormaster-expire-<days>d[-<prefix-slug>]`, writes the rule via `mc.SetBucketLifecycle` (merge with existing).

- [ ] **Step 1: Scaffold + implement.**

- [ ] **Step 2: Tests** (table-driven classifier):
  - Managed: `harbormaster-expire-30d-uploads` with `Expiration{Days:30}` + prefix filter, no tag → managed.
  - Unmanaged: same ID but with a `Transition` → unmanaged.
  - Unmanaged: random ID → unmanaged.
  - Summary never contains tag values, only counts.

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/lifecycle/... -race
git add apps/backend/internal/lifecycle
git commit -m "feat(backend/lifecycle): managed/unmanaged classifier + simple expiration create/delete"
```

---

### Task T3.13: Mount lifecycle routes

- [ ] **Step 1: Wire under `/buckets/{name}/lifecycle-rules/*`**

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): mount lifecycle-rule routes under /buckets/{name}/lifecycle-rules"
```

---

### Task T3.14: Frontend — bucket list page

**Files:**
- Create: `apps/frontend/src/features/buckets/BucketListPage.tsx`
- Create: `apps/frontend/src/features/buckets/api.ts`
- Create: `apps/frontend/src/features/buckets/types.ts`
- Create: `apps/frontend/src/lib/api/keys.ts` (extend)
- Create: `apps/frontend/src/features/buckets/BucketListPage.test.tsx`

- [ ] **Step 1: Add bucket key factory**

`keys.ts` extend:
```ts
export const bucketsKeys = {
  all: () => ["buckets"] as const,
  list: (params: { page: number; size: number; sort: string }) =>
    ["buckets", "list", params] as const,
  detail: (name: string) => ["buckets", "detail", name] as const,
};
```

- [ ] **Step 2: `api.ts`** with `listBuckets`, `getBucket`, `createBucket`, `deleteBucket`. Decode JSON:API by extracting `.data[].attributes`.

- [ ] **Step 3: Page UI**

A sortable, paginated table (shadcn `Table`). Columns: name, created, size, object count, versioning, lifecycle, public-access, quota. "Create bucket" button opens a dialog. Row click navigates to `/buckets/{name}`. Empty state: "No buckets yet — create one to get started."

- [ ] **Step 4: Create-bucket modal**

Zod-validated form: name (server + client validate), versioning checkbox, public-access select, lifecycle template select (`none` | `read-only` | bundled templates from M4 — for M3 leave `none` as the only option and note follow-up), optional quota.

- [ ] **Step 5: Tests** mock the API with msw, render list, assert columns, exercise create flow on success and on 422 (`invalid_bucket_name`).

- [ ] **Step 6: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/buckets): list page with create modal, sortable columns, pagination"
```

---

### Task T3.15: Frontend — bucket detail page (versioning, quota, public-access, delete, empty)

**Files:**
- Create: `apps/frontend/src/features/buckets/BucketDetailPage.tsx`
- Create: `apps/frontend/src/features/buckets/EditPublicAccessDialog.tsx`
- Create: `apps/frontend/src/features/buckets/EditQuotaDialog.tsx`
- Create: `apps/frontend/src/features/buckets/DeleteBucketDialog.tsx`
- Create: `apps/frontend/src/features/buckets/EmptyBucketDialog.tsx`
- Create: `apps/frontend/src/features/buckets/api.ts` (extend)

- [ ] **Step 1: Detail layout**

Tabs: Overview / Objects / Lifecycle. Overview tab shows the same bucket fields as the list row plus a quota progress bar (amber ≥80 %, red ≥95 %). Toggles: versioning. Buttons: Edit public access, Edit quota, Empty bucket, Delete bucket. Disable Delete when `object_count > 0` and link to Empty modal.

- [ ] **Step 2: Public-access dialog**

Select: private / public-read / public-read-write. If switching into public-read-write (write-allowing), require typed `<bucket-name>` confirmation field. Submit calls `PUT /buckets/{name}/public-access`.

- [ ] **Step 3: Quota dialog**

Form: kind (hard / fifo / none), value, unit (MiB/GiB/TiB). Disable FIFO option when `versioning_enabled=true` with tooltip "FIFO quotas require versioning to be off; disable versioning in bucket settings first." On 422 `fifo_requires_versioning_off`, show the same copy as a toast.

- [ ] **Step 4: Delete dialog**

Typed bucket-name confirmation. Disabled if `object_count > 0`; instead shows the link "Empty this bucket first." On 409 `bucket_not_empty`, navigate to the Empty modal.

- [ ] **Step 5: Empty-bucket dialog with SSE**

- Typed bucket-name confirmation.
- Versioned bucket: checkbox "Also permanently delete all object versions and delete-markers" (default off). Copy: "Recoverable via version restore" / "Permanent — no recovery."
- Non-versioned bucket: checkbox hidden.
- Submit opens `EventSource("/api/v1/buckets/{name}/empty")` … but `POST` is required. Use `fetch` with `ReadableStream` for SSE-over-POST instead:

```ts
import { useState } from "react";

export function useEmptyBucket(name: string) {
  const [progress, setProgress] = useState(0);
  const [done, setDone] = useState<{ deletedTotal: number; durationMs: number } | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [stalled, setStalled] = useState(false);

  async function start(confirmName: string, purgeVersions: boolean) {
    const res = await fetch(`/api/v1/buckets/${name}/empty`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": readCsrfCookie(),
        Accept: "text/event-stream",
      },
      body: JSON.stringify({ confirm_name: confirmName, purge_versions: purgeVersions }),
    });
    if (!res.ok) {
      const err = await parseErrorResponse(res);
      throw err;
    }
    const reader = res.body!.getReader();
    const dec = new TextDecoder();
    let buf = "";
    let lastEvent = Date.now();
    const stallTimer = setInterval(() => {
      if (Date.now() - lastEvent > 30_000) setStalled(true);
    }, 5_000);
    try {
      while (true) {
        const { done: end, value } = await reader.read();
        if (end) break;
        buf += dec.decode(value, { stream: true });
        const frames = buf.split("\n\n");
        buf = frames.pop() ?? "";
        for (const frame of frames) {
          if (!frame.trim() || frame.startsWith(":")) continue;
          lastEvent = Date.now();
          setStalled(false);
          const lines = frame.split("\n");
          const event = lines.find((l) => l.startsWith("event:"))?.slice(6).trim();
          const data = lines.find((l) => l.startsWith("data:"))?.slice(5).trim();
          if (!event || !data) continue;
          const parsed = JSON.parse(data);
          if (event === "progress") setProgress(parsed.deleted as number);
          if (event === "done") setDone({ deletedTotal: parsed.deleted_total, durationMs: parsed.duration_ms });
          if (event === "error") setErrorMsg(parsed.message as string);
        }
      }
    } finally { clearInterval(stallTimer); }
  }

  return { start, progress, done, errorMsg, stalled };
}
```

(`readCsrfCookie` extracted to `lib/api/csrf.ts` so it's reusable.)

The dialog renders progress, a stall banner when `stalled`, and the terminal "Done" or "Error" state. On done, invalidate `bucketsKeys.detail(name)` and `objectsKeys.list(name, "")`.

- [ ] **Step 6: Tests**

- Unit-test the SSE parser with a manual `ReadableStream` of mock frames.
- Integration: mount the dialog, simulate frames, assert progress updates.

- [ ] **Step 7: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/buckets): detail page with versioning, quota, public-access, delete, and SSE empty"
```

---

### Task T3.16: Frontend — object browser with virtualization + 90% auto-load + manual fallback

**Files:**
- Create: `apps/frontend/src/features/objects/ObjectBrowserPage.tsx`
- Create: `apps/frontend/src/features/objects/VirtualizedObjectList.tsx`
- Create: `apps/frontend/src/features/objects/api.ts`
- Create: `apps/frontend/src/features/objects/Breadcrumb.tsx`
- Create: `apps/frontend/src/features/objects/ObjectBrowserPage.test.tsx`

- [ ] **Step 1: useInfiniteObjects hook**

```ts
export function useInfiniteObjects(bucket: string, prefix: string) {
  return useInfiniteQuery({
    queryKey: objectsKeys.list(bucket, prefix),
    initialPageParam: "",
    queryFn: async ({ pageParam }) => {
      const sp = new URLSearchParams({ prefix, delimiter: "/", "page[size]": "100" });
      if (pageParam) sp.set("page[token]", String(pageParam));
      return api.get<ObjectListResponse>(`/api/v1/buckets/${bucket}/objects?${sp}`);
    },
    getNextPageParam: (last) => last.meta?.page?.next_token || undefined,
  });
}
```

- [ ] **Step 2: Virtualized list component**

Use `@tanstack/react-virtual`. 36px estimated row height. `onScroll` handler:

```ts
const fetchingRef = useRef(false);
function onScroll(e: React.UIEvent<HTMLDivElement>) {
  const el = e.currentTarget;
  const ratio = (el.scrollTop + el.clientHeight) / el.scrollHeight;
  if (ratio >= 0.9 && !fetchingRef.current && q.hasNextPage) {
    fetchingRef.current = true;
    q.fetchNextPage().finally(() => { fetchingRef.current = false; });
  }
}
```

Manual "Load more" button at the bottom always present (does nothing while `fetchNextPage` in flight). One outstanding request at a time enforced by `fetchingRef` AND `q.isFetchingNextPage`.

- [ ] **Step 3: Breadcrumb navigation**

Reads `prefix` from the URL search string (`?prefix=2025/01/`). Each segment is a clickable link that drops trailing segments.

- [ ] **Step 4: Row actions (download / delete / share-link)** open per-row menus. Download uses a `<a download>` link to `/api/v1/buckets/{name}/objects/download?key=...`. Delete opens a confirmation. Share-link opens a modal.

- [ ] **Step 5: Tests**

- Renders 10 entries.
- Scroll past 90 % triggers `fetchNextPage` exactly once.
- Scrolling past 90 % again while a fetch is pending does NOT call again.
- Manual "Load more" calls `fetchNextPage` once.

- [ ] **Step 6: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/objects): virtualized browser with 90% auto-load, one-outstanding cap, manual fallback"
```

---

### Task T3.17: Frontend — upload modal

**Files:**
- Create: `apps/frontend/src/features/objects/UploadDialog.tsx`
- Create: `apps/frontend/src/features/objects/UploadDialog.test.tsx`

- [ ] **Step 1: Implement**

- Drag-and-drop + file picker (`<input type="file">`).
- Read selected file's size; if > server cap (passed via `useConfigSummary` query or hard-coded for v1 default 100 MiB), reject client-side with the explanatory message: "This file exceeds the configured cap (100 MiB). Use `mc cp` or another direct S3 client."
- POST to `/api/v1/buckets/{bucket}/objects` with FormData. Use `XMLHttpRequest` to drive a progress bar.
- On 413 response, show the same explanatory message.
- On success, invalidate the current `objectsKeys.list(bucket, prefix)` query.

- [ ] **Step 2: Tests + commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/objects): drag-and-drop upload dialog with progress and 413 handling"
```

---

### Task T3.18: Frontend — share-link modal

**Files:**
- Create: `apps/frontend/src/features/objects/ShareLinkDialog.tsx`

- [ ] **Step 1: Implement**

- TTL input (default 7 days, options: 30 m / 1 h / 24 h / 7 d / custom seconds).
- Submit POST `/api/v1/buckets/{name}/objects/share-links`.
- On success: render the URL with a copy-to-clipboard button, the `expires_at` formatted human-readable, and a prominent "Cannot be revoked" warning block (with copy from `risks.md` R17).

- [ ] **Step 2: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/objects): share-link modal with copy-to-clipboard and revocation warning"
```

---

### Task T3.19: Frontend — object preview (image/text/json/pdf)

**Files:**
- Create: `apps/frontend/src/features/objects/PreviewPane.tsx`

- [ ] **Step 1: Implement**

- Open by clicking a non-folder row.
- For images: `<img src={previewURL}>`.
- For PDFs: `<embed type="application/pdf" src={previewURL}>` (browser-native).
- For text: fetch first 1 MiB (Range header), render in a `<pre>` with monospace.
- For JSON: same as text but pretty-printed via `JSON.stringify(JSON.parse(body), null, 2)`.
- Anything else: "No preview available — download to view" placeholder.

`previewURL` uses the proxy-download endpoint with an inline disposition override: backend may accept `?inline=1` to swap `Content-Disposition: attachment` for `inline`. If not implementing the inline flag in v1, the preview pane fetches the bytes itself and constructs a blob URL.

- [ ] **Step 2: Decision**

The simplest cut: client-side blob URLs only. The preview pane calls `api.get<Blob>(...)` for the relevant byte range and renders.

- [ ] **Step 3: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/objects): inline preview pane for image/pdf/text/json with 1 MiB text cap"
```

---

### Task T3.20: Frontend — lifecycle rules tab

**Files:**
- Create: `apps/frontend/src/features/lifecycle/LifecycleRulesTab.tsx`
- Create: `apps/frontend/src/features/lifecycle/CreateRuleDialog.tsx`
- Create: `apps/frontend/src/features/lifecycle/api.ts`

- [ ] **Step 1: Implement**

- List shows managed rules (editable: delete only; "edit" = delete + recreate) and unmanaged rules (read-only with `summary` string).
- Create form: days (1–10000), prefix (optional). No tag filter input.
- Delete confirmation.

- [ ] **Step 2: Tests + commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/lifecycle): rules tab with managed/unmanaged split + create-expiration form"
```

---

### Task T3.21: Backend — bucket lifecycle template application on create

**Files:**
- Modify: `apps/backend/internal/buckets/processor.go`

- [ ] **Step 1: Extend `Create(ctx, name, opts)` to accept an optional lifecycle template**

```go
type CreateOptions struct {
    VersioningEnabled bool
    PublicAccess      string
    Quota             *Quota
    LifecycleTemplate string // "" or "expire-30d" / "expire-90d" — small enum bundled in policies
}
```

Bundled templates (Go literals, not DB-stored):
- `expire-30d`: days=30, no prefix.
- `expire-90d`: days=90, no prefix.

(Document that the template enum is intentionally minimal. The frontend `CreateBucketDialog` exposes the same enum.)

- [ ] **Step 2: Tests + commit**

```bash
cd apps/backend && go test ./internal/buckets/... -race
git add apps/backend
git commit -m "feat(backend/buckets): apply optional lifecycle template on bucket create"
```

---

### Task T3.22: Backend — integration tests against MinIO testcontainer

**Files:**
- Create: `apps/backend/internal/integration/buckets_integration_test.go`
- Create: `apps/backend/internal/integration/objects_integration_test.go`
- Create: `apps/backend/internal/integration/empty_integration_test.go`
- Create: `apps/backend/internal/integration/lifecycle_integration_test.go`
- Create: `apps/backend/internal/integration/helper.go`

All files start with the build tag:
```go
//go:build integration
```

- [ ] **Step 1: `helper.go`** boots a MinIO testcontainer (using `testcontainers-go/modules/minio` at `RELEASE.2025-01-01T00-00-00Z`), constructs the `Pool`, returns ready clients.

- [ ] **Step 2: bucket tests**

- Create, list, delete (empty), versioning toggle, public-access transitions verified via `mc anonymous get`-equivalent (direct admin API), quota set/clear, FIFO with versioning → 422.

- [ ] **Step 3: object tests**

- Upload 5 MiB file via processor.Upload, download via proxy and verify byte-equality.
- Direct mode returns a presigned URL that returns 200.
- Share-link respects clamp (test min and max).

- [ ] **Step 4: empty test**

- Upload 5000 objects (use parallel goroutines for speed).
- Run `Service.StartOrAttach`; collect progress events; assert final `deleted_total == 5000`.
- Run again with `purge_versions=true` on a versioned bucket containing delete-markers; assert versions gone.

- [ ] **Step 5: lifecycle test**

- Create managed rule, fetch and confirm `Managed=true`.
- Pre-create an unmanaged rule via raw `mc` lifecycle XML, fetch, confirm `Managed=false`.

- [ ] **Step 6: Run locally**

```bash
cd apps/backend && HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...
```

- [ ] **Step 7: Commit**

```bash
git add apps/backend/internal/integration
git commit -m "test(backend/integration): MinIO testcontainer-driven tests for buckets/objects/empty/lifecycle"
```

---

### Task T3.23: Audit-event wiring across M3

- [ ] **Step 1: Verify every action handler `defer`s `audit.Record`**

For each handler, the deferred call records the appropriate action constant from `data-model.md`:
- `bucket.create`, `bucket.delete`, `bucket.versioning.enable`, `bucket.versioning.disable`, `bucket.public_access.update`, `bucket.quota.update`, `bucket.empty`.
- `object.upload`, `object.delete`, `object.download_proxy`, `object.share_link.create`.
- `lifecycle_rule.create`, `lifecycle_rule.delete`.

- [ ] **Step 2: Add a meta-test that, for each action constant, verifies a corresponding handler exists and writes the event**

This is the per-action no-secrets test from T1.9 expanded with **per-action coverage**: load each action constant and assert that running the appropriate handler against a fake-MinIO produces exactly one audit row with that action.

- [ ] **Step 3: Commit**

```bash
git add apps/backend
git commit -m "test(backend/audit): exhaustive per-action coverage test for M3 handlers"
```

---

### Task T3.24: Backend — bucket.empty audit payload completeness

- [ ] **Step 1: Verify `bucket.empty` payload includes `{deleted_count, duration_ms, purge_versions, versioning_enabled_at_start}`**

Add a focused integration test in M3.22 that performs an empty operation and reads the resulting audit row, asserting all four payload keys are present and correct.

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "test(backend/audit): assert bucket.empty payload completeness"
```

---

### Task T3.25: Backend — reverse-proxy buffering smoke test

- [ ] **Step 1: Add a tiny stand-alone test boot for SSE in front of a buffering reverse proxy**

Easier alternative: a unit test for the `internal/sse` writer that confirms `X-Accel-Buffering: no` is set. That's already in T1.7. Add a docs test (in T6.10) describing how to verify with nginx in front. Skip code work here.

---

### Task T3.26: Frontend — recent failures widget integration (deferred to M5)

Note: the dashboard widget itself ships in M5; the supporting `/api/v1/audit-events?filter=...&from=...` query is needed first. That query handler is wired in M5.

---

### Task T3.27: Frontend — settings page hooks for upload cap and download mode

**Files:**
- Modify: `apps/frontend/src/features/objects/UploadDialog.tsx`

- [ ] **Step 1: Read cap from a `GET /api/v1/config-summary` endpoint** (new — see T3.28).

Or alternatively: hard-code the default and rely on the backend 413. The hard-code approach keeps the surface smaller. Decision: **don't add a config-summary endpoint**. Frontend uses the default; backend remains the source of truth via 413 response details.

- [ ] **Step 2: Commit if any changes; otherwise skip.**

---

### Task T3.28: Frontend — surface cap dynamically from 413 response

**Files:**
- Modify: `apps/frontend/src/features/objects/UploadDialog.tsx`

- [ ] **Step 1: When a 413 returns `details.limit_bytes`, format that number into the rejection message: "Upload exceeds the configured cap (X MiB). Use mc cp or another direct S3 client."**

- [ ] **Step 2: Tests + commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/objects): show backend-reported upload cap in 413 message"
```

---

### Task T3.29: M3 verification

- [ ] **Step 1: Full local matrix**

```bash
cd apps/backend && go vet ./... && go test -race ./... && golangci-lint run && CGO_ENABLED=0 go build ./...
cd apps/backend && HARBORMASTER_INTEGRATION=1 go test -tags=integration ./...
cd ../frontend && npm run lint && npm test && npm run build
cd ../.. && docker buildx build -f deploy/docker/Dockerfile -t hm:m3 .
```

- [ ] **Step 2: Manual smoke**

Bring the stack up with a real MinIO. Walk through: create bucket, toggle versioning, upload 1 GiB (expect 413), upload 50 MiB (expect success), download, share-link, lifecycle rule, empty-bucket on a 5k-object bucket, delete bucket.

- [ ] **Step 3: Code review via `superpowers:requesting-code-review`** and fix findings.

---

### Task T3.30: Tag M3

```bash
git tag m3-complete
```

---

## Milestone M4 — Users, service accounts, policy templates

Goal: an operator can create IAM users with bundled policy templates, manage service accounts, and see externally-attached policies (including `consoleAdmin`) read-only.

### Task T4.1: `internal/policies` — bundled IAM templates

**Files:**
- Create: `apps/backend/internal/policies/templates.go`
- Create: `apps/backend/internal/policies/templates_test.go`

- [ ] **Step 1: Implement**

```go
package policies

import (
	"encoding/json"
	"fmt"
)

// Template represents a bundled IAM policy template.
type Template struct {
	Name         string
	Description  string
	Render       func(params map[string]string) (string, error)
	ParamsSchema json.RawMessage
}

// All returns the bundled v1 templates in deterministic order.
func All() []Template {
	return []Template{readOnly(), readWrite(), backupTarget()}
}

// Find returns the named template or ok=false.
func Find(name string) (Template, bool) {
	for _, t := range All() {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

func readOnly() Template {
	return Template{
		Name:        "read-only",
		Description: "Read-only across all buckets",
		Render: func(_ map[string]string) (string, error) {
			return renderDoc([]string{"s3:GetObject", "s3:ListBucket"}, []string{"arn:aws:s3:::*", "arn:aws:s3:::*/*"})
		},
	}
}

func readWrite() Template {
	return Template{
		Name:        "read-write",
		Description: "Read/write across all buckets, no admin operations",
		Render: func(_ map[string]string) (string, error) {
			return renderDoc(
				[]string{"s3:GetObject", "s3:ListBucket", "s3:PutObject", "s3:DeleteObject"},
				[]string{"arn:aws:s3:::*", "arn:aws:s3:::*/*"})
		},
	}
}

func backupTarget() Template {
	schema := json.RawMessage(`{"type":"object","required":["bucket"],"properties":{"bucket":{"type":"string","minLength":3,"maxLength":63}}}`)
	return Template{
		Name:         "backup-target",
		Description:  "Read/write/delete in a specific bucket",
		ParamsSchema: schema,
		Render: func(p map[string]string) (string, error) {
			b := p["bucket"]
			if b == "" {
				return "", fmt.Errorf("backup-target requires param 'bucket'")
			}
			return renderDoc(
				[]string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:ListBucket"},
				[]string{"arn:aws:s3:::" + b, "arn:aws:s3:::" + b + "/*"})
		},
	}
}

func renderDoc(actions, resources []string) (string, error) {
	type stmt struct {
		Effect   string   `json:"Effect"`
		Action   []string `json:"Action"`
		Resource []string `json:"Resource"`
	}
	type doc struct {
		Version   string `json:"Version"`
		Statement []stmt `json:"Statement"`
	}
	out, err := json.Marshal(doc{Version: "2012-10-17", Statement: []stmt{{Effect: "Allow", Action: actions, Resource: resources}}})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// MaterializedName returns the deterministic policy name Harbormaster uses on
// the MinIO side when attaching this template.
func MaterializedName(template string, params map[string]string) string {
	switch template {
	case "backup-target":
		return fmt.Sprintf("harbormaster-%s-%s", template, params["bucket"])
	default:
		return fmt.Sprintf("harbormaster-%s", template)
	}
}
```

- [ ] **Step 2: Tests**

- `TestAllRendersValidJSON` — each template's `Render({})` (`backup-target` with `{"bucket":"x"}`) returns valid JSON.
- `TestBackupTargetRequiresBucket` — `Render({})` errors.
- `TestMaterializedName` — deterministic per template/params.
- `TestNoAdminTemplate` — `Find("administrator")` returns false.

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/policies/... -race
git add apps/backend/internal/policies
git commit -m "feat(backend/policies): bundled read-only, read-write, backup-target templates"
```

---

### Task T4.2: `internal/policies` — materializer

**Files:**
- Create: `apps/backend/internal/policies/materializer.go`
- Create: `apps/backend/internal/policies/materializer_test.go`

- [ ] **Step 1: Implement**

```go
package policies

import (
	"context"

	madmin "github.com/minio/madmin-go/v3"
)

// Materializer ensures a Harbormaster-managed policy exists on MinIO with the
// expected canonical content, creating or overwriting as needed.
type Materializer struct {
	Admin func(context.Context) (*madmin.AdminClient, error)
}

// EnsurePolicy creates or overwrites the named canned policy with the rendered
// document. Returns the canonical policy name.
func (m *Materializer) EnsurePolicy(ctx context.Context, template string, params map[string]string) (string, error) {
	t, ok := Find(template)
	if !ok {
		return "", ErrUnknownTemplate
	}
	body, err := t.Render(params)
	if err != nil {
		return "", err
	}
	name := MaterializedName(template, params)
	admin, err := m.Admin(ctx)
	if err != nil {
		return "", err
	}
	if err := admin.AddCannedPolicy(ctx, name, []byte(body)); err != nil {
		return "", err
	}
	return name, nil
}
```

(`ErrUnknownTemplate` is a package-level sentinel.)

- [ ] **Step 2: Tests** with a stubbed admin client. Assertions: idempotent on repeated invocation; backup-target with the same bucket twice creates one policy; with different buckets creates two.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/policies
git commit -m "feat(backend/policies): materializer creates/upserts canonical Harbormaster policies on MinIO"
```

---

### Task T4.3: `internal/users` — IAM user domain

**Files:** `[skill scaffold]` for `users` domain (data flows through `madmin`, not SQLite, so `entity.go` is again minimal). Specifics:

- `model.go`:
  ```go
  type User struct {
      AccessKey         string
      Status            string // "enabled" | "disabled"
      AttachedTemplates []TemplateRef
      OtherPolicies     []string // MinIO policies not produced by Harbormaster
  }
  type TemplateRef struct { Name string; Params map[string]string }
  ```

- `processor.go` methods:
  - `List(ctx) ([]User, error)` — `admin.ListUsers` + per-user `admin.GetUserInfo` to fetch attached policies; reverse-map policies whose names match `harbormaster-<template>-<bucket?>` into `AttachedTemplates`; everything else → `OtherPolicies`.
  - `Create(ctx, accessKey, templates []TemplateRef) (User, secret string, err error)` — generates secret via CSPRNG (40 char [a-zA-Z0-9]), `admin.AddUser`, materializes each template via `Materializer`, attaches via `admin.AttachPolicy`.
  - `SetStatus(ctx, accessKey, enabled bool) error` — `admin.SetUserStatus(accessKey, enabled)`.
  - `Delete(ctx, accessKey, confirmKey string) error` — verifies confirmation, `admin.RemoveUser`.
  - `UpdatePolicies(ctx, accessKey string, templates []TemplateRef) error` — diff current attached templates vs requested; detach removed, attach added.

- `rest.go` — JSON:API DTOs. **Critical**: `CreateUserResponse` includes `secret_key` exactly once; the type used by `List` does **not** have that field.

- [ ] **Step 1: Scaffold + implement.**

Secret generator helper in `internal/users/secret.go`:

```go
package users

import (
	"crypto/rand"
	"math/big"
)

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateSecret returns a 40-character base62 secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 40)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}
```

- [ ] **Step 2: Tests** (unit) with mocked admin client:
  - `TestCreateGeneratesSecretAndAttaches`
  - `TestCreateMaterializesPolicies`
  - `TestDeleteRequiresExactAccessKeyMatch`
  - `TestUpdatePoliciesDiff` — detach removed, attach added, leave unchanged

- [ ] **Step 3: Commit**

```bash
cd apps/backend && go test ./internal/users/... -race
git add apps/backend/internal/users
git commit -m "feat(backend/users): IAM user domain (list/create/status/delete/policies) with secret-shown-once"
```

---

### Task T4.4: Service accounts

**Files:**
- Create: `apps/backend/internal/users/serviceaccounts.go`
- Create: `apps/backend/internal/users/serviceaccounts_test.go`

- [ ] **Step 1: Implement**

```go
package users

import (
	"context"

	madmin "github.com/minio/madmin-go/v3"
)

// ServiceAccount describes a child credential of a parent user.
type ServiceAccount struct {
	AccessKey        string
	ParentUser       string
	Name             string
	Description      string
	AttachedTemplate *TemplateRef // optional policy override
}

// ServiceAccountProcessor methods.
type ServiceAccountProcessor struct {
	Admin func(ctx context.Context) (*madmin.AdminClient, error)
	Mat   *policies.Materializer
	Audit AuditRecorder
}

func (p *ServiceAccountProcessor) List(ctx context.Context, parent string) ([]ServiceAccount, error) { /* admin.ListServiceAccounts */ }
func (p *ServiceAccountProcessor) Create(ctx context.Context, parent, name, description string, override *TemplateRef) (ServiceAccount, string, error) { /* admin.AddServiceAccount + optional policy attach */ }
func (p *ServiceAccountProcessor) Revoke(ctx context.Context, accessKey string) error { /* admin.DeleteServiceAccount */ }
```

(`madmin` package shape varies — the executing engineer verifies field names against `madmin-go/v3` at the pinned version. The user-visible behaviors are what the tests pin.)

- [ ] **Step 2: Tests** (mocked admin client):
  - Create returns the secret exactly once and never on List.
  - Override with a `backup-target` template materializes the canonical bucket-scoped policy.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/users
git commit -m "feat(backend/users): service-accounts subhandler (list/create/revoke) with optional template override"
```

---

### Task T4.5: REST handlers + mount

**Files:**
- Complete `apps/backend/internal/users/resource.go`
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: Mount users routes**

- `GET    /api/v1/users` — list (JSON:API).
- `POST   /api/v1/users` — create (JSON:API). Response includes one-time `secret_key`.
- `GET    /api/v1/users/{access_key}` — detail.
- `PUT    /api/v1/users/{access_key}/status` — action.
- `DELETE /api/v1/users/{access_key}` — body `{"confirm_access_key": "..."}`.
- `PUT    /api/v1/users/{access_key}/policies` — action.
- `GET    /api/v1/users/{access_key}/service-accounts` — JSON:API.
- `POST   /api/v1/users/{access_key}/service-accounts` — JSON:API. Includes one-time `secret_key`.
- `DELETE /api/v1/service-accounts/{access_key}` — action.
- `GET    /api/v1/policy-templates` — JSON:API.

Implement each handler. Tests assert envelope shape per api-contracts.md.

- [ ] **Step 2: Commit**

```bash
cd apps/backend && go test ./internal/users/... ./internal/policies/... -race
git add apps/backend
git commit -m "feat(backend/users): mount /users, /service-accounts, /policy-templates routes"
```

---

### Task T4.6: Backend integration tests for users + service accounts

**Files:**
- Create: `apps/backend/internal/integration/users_integration_test.go`
- Create: `apps/backend/internal/integration/serviceaccounts_integration_test.go`

(`//go:build integration` tag.)

- [ ] **Step 1: Test scenarios**

- Create user with `read-write` template → verify can list buckets via the new creds against the testcontainer.
- Create user with `backup-target{bucket:"x"}` → can write into `x`, cannot list other buckets.
- Disable user → operations fail with `AccessDenied`.
- Create service account under user → operates with the parent user's authority unless override is set.
- Externally attach `consoleAdmin` via raw `admin.AttachPolicy`; reload user; verify `OtherPolicies` contains `consoleAdmin`.

- [ ] **Step 2: Commit**

```bash
git add apps/backend/internal/integration
git commit -m "test(backend/users): integration coverage against MinIO testcontainer"
```

---

### Task T4.7: Frontend — Users list

**Files:**
- Create: `apps/frontend/src/features/users/UserListPage.tsx`
- Create: `apps/frontend/src/features/users/api.ts`
- Create: `apps/frontend/src/features/users/types.ts`
- Modify: `apps/frontend/src/components/AppShell.tsx` (sidebar nav link)
- Modify: `apps/frontend/src/routes.tsx` (mount `/users` + `/users/:accessKey`)

- [ ] **Step 1: Implement**

Table columns: access key, status, attached templates (chips). Search input filters locally on access key. "New user" button opens the create dialog (T4.9).

- [ ] **Step 2: Tests + commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/users): list page with status, attached templates, search"
```

---

### Task T4.8: Frontend — User detail page

**Files:**
- Create: `apps/frontend/src/features/users/UserDetailPage.tsx`
- Create: `apps/frontend/src/features/users/EditPoliciesDialog.tsx`
- Create: `apps/frontend/src/features/users/DeleteUserDialog.tsx`

- [ ] **Step 1: Detail layout**

- Header: access key, status toggle.
- Attached templates section: list with edit + remove. "Add template" opens a small subform.
- "Other attached policies" section (read-only): renders `OtherPolicies` strings (e.g., `consoleAdmin`).
- Service accounts section: nested list with create/revoke.
- Delete user: opens a typed-access-key confirmation dialog.

- [ ] **Step 2: Tests + commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/users): detail page with templates editor, other-policies readout, SA section"
```

---

### Task T4.9: Frontend — Create-user dialog (one-time secret reveal)

**Files:**
- Create: `apps/frontend/src/features/users/CreateUserDialog.tsx`
- Create: `apps/frontend/src/features/users/SecretRevealCard.tsx`

- [ ] **Step 1: Implement**

- Form: access key, template selector (multi-select with optional params per template — backup-target asks for bucket).
- Submit `POST /api/v1/users`.
- On 201, the dialog body switches to a "Your secret key" reveal card:
  - "Reveal" button (defaults hidden); when revealed shows the secret in a monospace field.
  - "Copy to clipboard" button.
  - Warning copy: "This is the only time you'll see this secret. Store it now."
  - "I've saved it" closes the dialog and refreshes the list.

- [ ] **Step 2: Tests assert**:
  - Secret never visible by default.
  - Copy button copies the rendered string (uses `navigator.clipboard.writeText`).
  - Closing the dialog before "I've saved it" is acknowledged still works but the secret is unrecoverable.

- [ ] **Step 3: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/users): create-user dialog with one-time secret reveal card"
```

---

### Task T4.10: Frontend — Service accounts UI

**Files:**
- Create: `apps/frontend/src/features/service-accounts/CreateServiceAccountDialog.tsx`
- Create: `apps/frontend/src/features/service-accounts/api.ts`
- (Embedded inside `UserDetailPage` from T4.8 — only the dialog is separate.)

- [ ] **Step 1: Implement**

- Create form: optional name, optional description, optional template override (same shape as user template attach UI).
- On 201, same one-time secret reveal card as in T4.9.
- Revoke action with confirmation.

- [ ] **Step 2: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/service-accounts): create dialog with one-time secret + revoke confirmation"
```

---

### Task T4.11: Frontend — Policy templates page (informational)

**Files:**
- Create: `apps/frontend/src/features/policies/PolicyTemplatesPage.tsx`
- Modify: `apps/frontend/src/routes.tsx` (add `/policies`)

- [ ] **Step 1: Implement**

A small reference page listing the bundled templates with their descriptions and (for `backup-target`) the params schema rendered as a small form preview. Includes copy explaining that `consoleAdmin` is intentionally not bundled and pointing operators to `mc admin policy attach`.

- [ ] **Step 2: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/policies): bundled-template reference page"
```

---

### Task T4.12: Confirm `risks.md` R17 — share-link audit content

Already wired in M3. M4 only verifies no `presigned`-flavored fields appear in any user-flow audit row.

- [ ] **Step 1: Add a test that creates a service account, lists, then a share link, and inspects every audit row's `payload_summary_json` for the substring `https://`. Assert none.**

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "test(backend/audit): assert no presigned URL leaks into any audit payload across user flows"
```

---

### Task T4.13: M4 verification

- [ ] **Step 1: Full local matrix (backend + frontend + container)**

- [ ] **Step 2: Manual smoke** — create user with `backup-target{bucket:foo}`, copy secret, attempt `mc cp` into `foo` (works) and into another bucket (fails), revoke service account, delete user.

- [ ] **Step 3: `superpowers:requesting-code-review`** + address findings.

---

### Task T4.14: Tag M4

```bash
git tag m4-complete
```

---

## Milestone M5 — Dashboard + activity feed

Goal: the `/dashboard` aggregate renders within the 2 s p95 SLO; `/activity` provides a filtered/paginated audit log view; the recent-failures widget on the dashboard works with selectable 24h/7d/30d windows.

### Task T5.1: `internal/dashboard` — aggregate processor

**Files:**
- Create: `apps/backend/internal/dashboard/processor.go`
- Create: `apps/backend/internal/dashboard/rest.go`
- Create: `apps/backend/internal/dashboard/resource.go`
- Create: `apps/backend/internal/dashboard/processor_test.go`

Dashboard is the documented exception that **may call multiple processors directly** (per `backend-dev-guidelines/resources/architecture-overview.md` "Cross-Domain Orchestration"). No DDD entity/builder needed.

- [ ] **Step 1: Implement**

```go
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/buckets"
)

// Window represents the supported failures-window query values.
type Window string

const (
	Window24h Window = "24h"
	Window7d  Window = "7d"
	Window30d Window = "30d"
)

// Parse converts an input string into a Window, defaulting to 7d.
func Parse(s string) (Window, error) {
	switch s {
	case "":
		return Window7d, nil
	case "24h", "7d", "30d":
		return Window(s), nil
	}
	return "", errors.New("invalid_failures_window")
}

// Duration returns the time.Duration this Window covers.
func (w Window) Duration() time.Duration {
	switch w {
	case Window24h:
		return 24 * time.Hour
	case Window30d:
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// View is the full dashboard payload.
type View struct {
	Server         ServerInfo      `json:"server"`
	Totals         Totals          `json:"totals"`
	Nodes          []NodeStatus    `json:"nodes"`
	Warnings       []string        `json:"warnings"`
	RecentActivity []audit.Event   `json:"recent_activity"`
	RecentFailures FailuresWidget  `json:"recent_failures"`
}

type ServerInfo struct {
	Version        string `json:"version"`
	DeploymentMode string `json:"deployment_mode"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}
type Totals struct {
	Buckets        int64 `json:"buckets"`
	EstimatedBytes int64 `json:"estimated_bytes"`
	Objects        int64 `json:"objects"`
}
type NodeStatus struct {
	Endpoint string `json:"endpoint"`
	State    string `json:"state"`
	Drives   struct {
		Total     int `json:"total"`
		Healthy   int `json:"healthy"`
		Unhealthy int `json:"unhealthy"`
	} `json:"drives"`
}
type FailuresWidget struct {
	Window  Window         `json:"window"`
	Count   int64          `json:"count"`
	Entries []audit.Event  `json:"entries"`
}

// Processor fans out reads across MinIO admin and local audit.
type Processor struct {
	BucketsProc *buckets.Processor
	AuditProc   *audit.Processor
	MinIOPool   PoolGetter
}

// PoolGetter abstracts the MinIO Pool dependency (mockable in tests).
type PoolGetter interface {
	ServerInfo(ctx context.Context) (ServerInfo, []NodeStatus, []string, error)
}

// Build returns the aggregate.
func (p *Processor) Build(ctx context.Context, w Window) (View, error) {
	var (
		out  View
		grp  errgroup.Group
		bks  []buckets.Bucket
		ra   []audit.Event
		failW FailuresWidget
	)
	grp.Go(func() error {
		s, n, warn, err := p.MinIOPool.ServerInfo(ctx)
		out.Server = s
		out.Nodes = n
		out.Warnings = warn
		return err
	})
	grp.Go(func() error {
		var err error
		bks, err = p.BucketsProc.List(ctx)
		return err
	})
	grp.Go(func() error {
		var err error
		ra, err = p.AuditProc.Recent(ctx, 25)
		return err
	})
	grp.Go(func() error {
		cutoff := time.Now().UTC().Add(-w.Duration())
		count, entries, err := p.AuditProc.FailuresSince(ctx, cutoff, 10)
		failW = FailuresWidget{Window: w, Count: count, Entries: entries}
		return err
	})
	if err := grp.Wait(); err != nil {
		return View{}, fmt.Errorf("dashboard fan-out: %w", err)
	}
	for _, b := range bks {
		out.Totals.Buckets++
		out.Totals.EstimatedBytes += b.EstimatedBytes
		out.Totals.Objects += b.ObjectCount
	}
	out.RecentActivity = ra
	out.RecentFailures = failW
	return out, nil
}
```

`PoolGetter` is implemented by `internal/minio.Pool` via a small wrapper that calls `madmin.ServerInfo`, parses fields, returns warnings (e.g., "decommissioning in progress" if MinIO surfaces it).

- [ ] **Step 2: Add `golang.org/x/sync` dependency**

```bash
cd apps/backend && go get golang.org/x/sync@v0.7.0 && go mod tidy
```

- [ ] **Step 3: Tests**

Use mock implementations of `PoolGetter`, `BucketsProc`, `AuditProc`. Cases:
- 12 buckets aggregate to the documented totals.
- `failures_window=24h`/`7d`/`30d` resolve to expected cutoffs (use `time.Now` stub).
- Invalid window string → `Parse` returns `invalid_failures_window`.
- A fan-out error propagates.

- [ ] **Step 4: Commit**

```bash
cd apps/backend && go test ./internal/dashboard/... -race
git add apps/backend/internal/dashboard
git commit -m "feat(backend/dashboard): aggregate processor + parallel fan-out"
```

---

### Task T5.2: Audit query support — `Recent`, `FailuresSince`, `List(filter, page)`

**Files:**
- Modify: `apps/backend/internal/audit/processor.go`
- Modify: `apps/backend/internal/audit/provider.go`
- Modify: `apps/backend/internal/audit/resource.go`

- [ ] **Step 1: Add provider functions**

```go
func recent(limit int) func(*gorm.DB) ([]Event, error) {
    return func(db *gorm.DB) ([]Event, error) {
        var rows []auditEventEntity
        if err := db.Order("occurred_at DESC").Limit(limit).Find(&rows).Error; err != nil {
            return nil, err
        }
        return mapEntities(rows)
    }
}

func failuresSince(cutoff time.Time, limit int) func(*gorm.DB) (int64, []Event, error) {
    return func(db *gorm.DB) (int64, []Event, error) {
        var count int64
        if err := db.Model(&auditEventEntity{}).
            Where("outcome = ? AND occurred_at >= ?", "failure", cutoff.Format(time.RFC3339)).
            Count(&count).Error; err != nil { return 0, nil, err }
        var rows []auditEventEntity
        if err := db.Where("outcome = ? AND occurred_at >= ?", "failure", cutoff.Format(time.RFC3339)).
            Order("occurred_at DESC").Limit(limit).Find(&rows).Error; err != nil { return 0, nil, err }
        events, err := mapEntities(rows)
        return count, events, err
    }
}

func listFiltered(f Filter, page Page) func(*gorm.DB) ([]Event, int64, error) {
    return func(db *gorm.DB) ([]Event, int64, error) {
        q := db.Model(&auditEventEntity{})
        if f.Action != "" { q = q.Where("action = ?", f.Action) }
        if f.TargetType != "" { q = q.Where("target_type = ?", f.TargetType) }
        if f.TargetID != "" { q = q.Where("target_id = ?", f.TargetID) }
        if f.Outcome != "" { q = q.Where("outcome = ?", f.Outcome) }
        if !f.From.IsZero() { q = q.Where("occurred_at >= ?", f.From.UTC().Format(time.RFC3339)) }
        if !f.To.IsZero() { q = q.Where("occurred_at <= ?", f.To.UTC().Format(time.RFC3339)) }
        var total int64
        if err := q.Count(&total).Error; err != nil { return nil, 0, err }
        var rows []auditEventEntity
        if err := q.Order("occurred_at DESC").
            Offset((page.Number - 1) * page.Size).Limit(page.Size).
            Find(&rows).Error; err != nil { return nil, 0, err }
        events, err := mapEntities(rows)
        return events, total, err
    }
}
```

- [ ] **Step 2: Processor methods**

```go
func (p *Processor) Recent(ctx context.Context, limit int) ([]Event, error)
func (p *Processor) FailuresSince(ctx context.Context, cutoff time.Time, limit int) (int64, []Event, error)
func (p *Processor) List(ctx context.Context, f Filter, page Page) ([]Event, int64, error)
```

- [ ] **Step 3: REST handler — `GET /api/v1/audit-events`**

JSON:API collection; query params per api-contracts.md. Tests assert filter combinations and pagination.

- [ ] **Step 4: Commit**

```bash
cd apps/backend && go test ./internal/audit/... -race
git add apps/backend/internal/audit
git commit -m "feat(backend/audit): query handler with action/target/outcome/date filters and pagination"
```

---

### Task T5.3: Wire dashboard + audit-events routes

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`

- [ ] **Step 1: Mount under protected**

- `GET /api/v1/dashboard?failures_window=...` — action style.
- `GET /api/v1/audit-events?filter[...]&page[...]` — JSON:API style.

- [ ] **Step 2: Commit**

```bash
git add apps/backend
git commit -m "feat(backend/serve): mount /dashboard and /audit-events routes"
```

---

### Task T5.4: Frontend — Dashboard page

**Files:**
- Create: `apps/frontend/src/features/dashboard/DashboardPage.tsx`
- Create: `apps/frontend/src/features/dashboard/RecentFailuresWidget.tsx`
- Create: `apps/frontend/src/features/dashboard/api.ts`
- Create: `apps/frontend/src/features/dashboard/DashboardPage.test.tsx`
- Modify: `apps/frontend/src/routes.tsx` (add `/dashboard` and redirect `/` → `/dashboard`)
- Modify: `apps/frontend/src/components/AppShell.tsx` (sidebar item)

- [ ] **Step 1: Implement page**

Three rows:
1. Server card: version, deployment mode, uptime.
2. Totals row: buckets, total bytes (human), object count.
3. Node grid: per-node status badges.

Plus the `RecentFailuresWidget` and the "Recent activity" feed (last 25 from the response).

- [ ] **Step 2: Implement `RecentFailuresWidget`**

```tsx
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { api } from "@/lib/api/client";
import { dashboardKeys } from "@/lib/api/keys";
import { Link } from "react-router-dom";

const WINDOW_KEY = "harbormaster:dashboard:failuresWindow";
type W = "24h" | "7d" | "30d";

export function RecentFailuresWidget() {
  const [window, setWindow] = useState<W>(() => (localStorage.getItem(WINDOW_KEY) as W) ?? "7d");
  useEffect(() => { localStorage.setItem(WINDOW_KEY, window); }, [window]);
  const q = useQuery({
    queryKey: dashboardKeys.failures(window),
    queryFn: () => api.get<{ count: number; window: string; entries: Array<{...}> }>(`/api/v1/dashboard?failures_window=${window}`),
    select: (data) => data.recent_failures,
  });
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Recent failures</CardTitle>
        <Select value={window} onValueChange={(v) => setWindow(v as W)}>
          <SelectTrigger className="w-24"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="24h">24h</SelectItem>
            <SelectItem value="7d">7d</SelectItem>
            <SelectItem value="30d">30d</SelectItem>
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent>
        {q.data?.count === 0 && <p className="text-sm text-muted-foreground">No failures.</p>}
        {/* ...list... */}
        <Link to={`/activity?outcome=failure&from=...&to=...`} className="text-sm underline">See all</Link>
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 3: Tests** — mock the API and assert the widget reads `localStorage`-persisted window, re-fetches on change, and renders the "See all" link with the right query params.

- [ ] **Step 4: Commit**

```bash
cd apps/frontend && npm test
git add apps/frontend/src
git commit -m "feat(frontend/dashboard): page with server info, totals, nodes, recent activity, failures widget"
```

---

### Task T5.5: Frontend — Activity feed page

**Files:**
- Create: `apps/frontend/src/features/activity/ActivityFeedPage.tsx`
- Create: `apps/frontend/src/features/activity/api.ts`
- Modify: `apps/frontend/src/routes.tsx` (add `/activity`)

- [ ] **Step 1: Implement**

- URL-encoded filters (`?action=...&target_type=...&outcome=...&from=...&to=...&page[number]=...`).
- Paginated table with action, target, outcome, source IP, occurred_at, error_message (truncated).
- Filter sidebar with: action multi-select (use the same enum from policies file), target_type multi-select, outcome dropdown, date-range picker.

- [ ] **Step 2: Tests + commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/activity): filtered + paginated audit feed page"
```

---

### Task T5.6: Frontend — Theme toggle in AppShell

**Files:**
- Modify: `apps/frontend/src/components/AppShell.tsx`

- [ ] **Step 1: Add a sun/moon icon toggle (lucide-react) bound to `useTheme()`.** Persists to localStorage automatically via `ThemeProvider`.

- [ ] **Step 2: Commit**

```bash
git add apps/frontend/src
git commit -m "feat(frontend/shell): theme toggle in app header"
```

---

### Task T5.7: SLO smoke test for dashboard p95 < 2 s

**Files:**
- Create: `apps/backend/internal/integration/dashboard_slo_integration_test.go`

(`//go:build integration` tag.)

- [ ] **Step 1: Test scenario**

- Seed the testcontainer MinIO with 100 buckets containing 10 objects each.
- Hit `GET /api/v1/dashboard?failures_window=7d` 50 times sequentially; collect latencies; assert p95 < 2 s.

Use the `httptest.NewServer` driven by the same handler chain as production, with the real `madmin` against the testcontainer.

- [ ] **Step 2: Commit**

```bash
cd apps/backend && HARBORMASTER_INTEGRATION=1 go test -tags=integration ./internal/integration/...
git add apps/backend/internal/integration
git commit -m "test(backend/integration): dashboard p95 SLO with 100-bucket fixture"
```

---

### Task T5.8: Object listing p95 < 3 s for 10k-key prefix

**Files:**
- Create: `apps/backend/internal/integration/objects_slo_integration_test.go`

- [ ] **Step 1: Seed a bucket with 10k objects under `prefix/`; measure list page time; assert single-page p95 < 3 s.**

- [ ] **Step 2: Commit**

```bash
git add apps/backend/internal/integration
git commit -m "test(backend/integration): object listing p95 SLO for 10k-object prefix"
```

---

### Task T5.9: M5 verification

- [ ] **Step 1: Full local matrix; code review; address findings.**

---

### Task T5.10: Tag M5

```bash
git tag m5-complete
```

---

## Milestone M6 — Deployment, CI/CD, supply chain

Goal: a tagged release produces a signed multi-arch image on GHCR + a GitHub Release page; `docker compose up` from a clean clone walks through wizard → login → buckets end-to-end. Operator docs cover security, configuration, and recovery.

### Task T6.1: Finalize Dockerfile

**Files:**
- Modify: `deploy/docker/Dockerfile`

- [ ] **Step 1: Tighten the existing Dockerfile**

- Add HEALTHCHECK pinged at `/healthz`:
  ```dockerfile
  HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/harbormaster", "version"]
  ```
  (Distroless has no `curl`; using `version` as a process-aliveness check is sufficient since the binary's `version` subcommand exits 0 without touching the network.)
- Bake `version` via ldflags from `--build-arg VERSION`:
  ```dockerfile
  ARG VERSION=dev
  ENV VERSION=$VERSION
  ```

- [ ] **Step 2: Commit**

```bash
git add deploy/docker
git commit -m "build(docker): healthcheck + VERSION build-arg threaded through ldflags"
```

---

### Task T6.2: Finalize docker-compose.yml

**Files:**
- Modify: `deploy/docker/docker-compose.yml`

- [ ] **Step 1: Final compose**

```yaml
services:
  harbormaster:
    image: ghcr.io/jtumidanski/harbormaster:${HARBORMASTER_TAG:-latest}
    build:
      context: ../..
      dockerfile: deploy/docker/Dockerfile
    restart: unless-stopped
    ports:
      - "${HARBORMASTER_PORT:-8080}:8080"
    environment:
      HARBORMASTER_LISTEN_ADDR: ":8080"
      HARBORMASTER_DATA_DIR: "/var/lib/harbormaster"
      HARBORMASTER_LOG_LEVEL: "${HARBORMASTER_LOG_LEVEL:-info}"
    volumes:
      - harbormaster-data:/var/lib/harbormaster
      # Uncomment to enable mc-alias import during first-run setup (read-only).
      # See README "Importing from mc config" for the trade-off.
      # - ${HOME}/.mc/config.json:/root/.mc/config.json:ro
    healthcheck:
      test: ["CMD", "/usr/local/bin/harbormaster", "version"]
      interval: 30s
      timeout: 3s
      retries: 3

  # Optional: bundled MinIO for local testing. Comment out for production.
  minio:
    image: minio/minio:RELEASE.2025-01-01T00-00-00Z
    profiles: ["with-minio"]
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: "admin"
      MINIO_ROOT_PASSWORD: "admin12345"
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio-data:/data

volumes:
  harbormaster-data: {}
  minio-data: {}
```

- [ ] **Step 2: Verify**

```bash
cd deploy/docker && docker compose --profile with-minio config
docker compose --profile with-minio up --build
```

- [ ] **Step 3: Commit**

```bash
git add deploy/docker
git commit -m "build(docker): production-ready compose with optional MinIO profile and healthcheck"
```

---

### Task T6.3: Reverse-proxy snippet docs

**Files:**
- Create: `docs/operator/reverse-proxy.md`
- Create: `deploy/docker/nginx.conf.example`
- Create: `deploy/docker/caddy.example.Caddyfile`

- [ ] **Step 1: Provide nginx and Caddy snippets that handle SSE properly (proxy_buffering off, proxy_read_timeout 1h, etc.) and base-path support**

`nginx.conf.example`:

```nginx
upstream harbormaster_upstream {
    server harbormaster:8080;
}

server {
    listen 80;
    server_name harbormaster.example.com;

    # The empty-bucket endpoint streams SSE; buffering would batch progress.
    location ~ ^/api/v1/buckets/.+/empty$ {
        proxy_pass http://harbormaster_upstream;
        proxy_buffering off;
        proxy_read_timeout 1h;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location / {
        proxy_pass http://harbormaster_upstream;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        client_max_body_size 200M;  # match HARBORMASTER_UPLOAD_MAX_BYTES + headroom
    }
}
```

`caddy.example.Caddyfile`:

```caddy
harbormaster.example.com {
    reverse_proxy harbormaster:8080 {
        flush_interval -1
        transport http {
            response_header_timeout 1h
        }
    }
}
```

- [ ] **Step 2: Write `docs/operator/reverse-proxy.md`** explaining the SSE + upload trade-offs, base-path setup, and TLS termination options.

- [ ] **Step 3: Commit**

```bash
git add docs deploy/docker
git commit -m "docs(operator): reverse-proxy config snippets (nginx, Caddy, Traefik notes)"
```

---

### Task T6.4: Kubernetes manifests

**Files:**
- Create: `deploy/kubernetes/deployment.yaml`
- Create: `deploy/kubernetes/service.yaml`
- Create: `deploy/kubernetes/ingress.example.yaml`
- Create: `deploy/kubernetes/secret.example.yaml`
- Create: `deploy/kubernetes/pvc.yaml`
- Create: `deploy/kubernetes/README.md`

- [ ] **Step 1: Write `deployment.yaml`**

```yaml
# Harbormaster v1 — minimal Kubernetes Deployment.
#
# Knobs operators routinely change (top of file for visibility):
#   - image tag (default: ghcr.io/jtumidanski/harbormaster:v1)
#   - storage class on the PVC (defaults to "" → cluster default)
#   - ingress hostname + TLS in ingress.example.yaml
#   - MinIO endpoint URL (configured at first-run via the wizard)
#
# Important:
#   - This Deployment is intentionally single-replica with strategy Recreate
#     because the login rate limiter, the empty-bucket worker, and the audit
#     retention sweeper are in-process. Multi-replica is not supported in v1.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: harbormaster
  labels: { app.kubernetes.io/name: harbormaster }
spec:
  replicas: 1
  strategy: { type: Recreate }
  selector: { matchLabels: { app.kubernetes.io/name: harbormaster } }
  template:
    metadata:
      labels: { app.kubernetes.io/name: harbormaster }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
      containers:
        - name: harbormaster
          image: ghcr.io/jtumidanski/harbormaster:v1
          imagePullPolicy: IfNotPresent
          args: ["serve"]
          ports:
            - { containerPort: 8080, name: http }
          env:
            - { name: HARBORMASTER_LISTEN_ADDR, value: ":8080" }
            - { name: HARBORMASTER_DATA_DIR, value: "/var/lib/harbormaster" }
            - { name: HARBORMASTER_LOG_FORMAT, value: "json" }
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            initialDelaySeconds: 3
            periodSeconds: 5
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            initialDelaySeconds: 10
            periodSeconds: 10
          resources:
            requests: { cpu: 100m, memory: 128Mi }
            limits:   { cpu: 1000m, memory: 512Mi }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
          volumeMounts:
            - { name: data, mountPath: /var/lib/harbormaster }
      volumes:
        - name: data
          persistentVolumeClaim: { claimName: harbormaster-data }
```

- [ ] **Step 2: Write `service.yaml`, `pvc.yaml`, `ingress.example.yaml`, `secret.example.yaml`**

Each file's top 30 lines list the knobs operators tune.

- [ ] **Step 3: Verify `kubectl apply --dry-run=client -f deploy/kubernetes/`**

```bash
kubectl apply --dry-run=client -f deploy/kubernetes/deployment.yaml
kubectl apply --dry-run=client -f deploy/kubernetes/service.yaml
kubectl apply --dry-run=client -f deploy/kubernetes/pvc.yaml
```

- [ ] **Step 4: Write `deploy/kubernetes/README.md`**

Cover: how to apply, where to set the image tag, replicas:1 rationale, PVC sizing, ingress integration, exec one-liners for `harbormaster admin reset-password` and `reset-encryption --confirm`.

- [ ] **Step 5: Commit**

```bash
git add deploy/kubernetes
git commit -m "build(k8s): raw manifests (deployment, service, ingress, pvc, secret) with operator README"
```

---

### Task T6.5: Main-branch workflow — multi-arch build, trivy, GHCR push, cosign sign

**Files:**
- Create: `.github/workflows/main.yml`

- [ ] **Step 1: Write `main.yml`**

```yaml
name: main
on:
  push:
    branches: [main]
    tags: ["v*.*.*"]

permissions:
  contents: read
  packages: write
  id-token: write  # for cosign keyless OIDC

jobs:
  build-and-publish:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
        with: { fetch-depth: 0 }
      - uses: docker/setup-qemu-action@49b3bc8e6bdd4a60e6116a5414239cba5943d3cf # v3.2.0
      - uses: docker/setup-buildx-action@988b5a0280414f521da01fcc63a27aeeb4b104db # v3.6.1
      - uses: docker/login-action@9780b0c442fbb1117ed29e0efdff1e18412f7567 # v3.3.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - id: meta
        uses: docker/metadata-action@8e5442c4ef9f78752691e2d8f8d19755c6f78e81 # v5.5.1
        with:
          images: ghcr.io/jtumidanski/harbormaster
          tags: |
            type=ref,event=branch
            type=semver,pattern={{version}}
            type=sha,prefix=sha-,format=short
            type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}
      - id: build
        uses: docker/build-push-action@5cd11c3a4ced054e52742c5fd54dca954e0edd85 # v6.7.0
        with:
          context: .
          file: deploy/docker/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: false
          load: false
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: VERSION=${{ steps.meta.outputs.version }}
          outputs: type=oci,dest=/tmp/image.tar
      - name: Trivy image scan
        uses: aquasecurity/trivy-action@a20de5420d57c4102486cdd9578b45609c99d7eb # v0.24.0
        with:
          input: /tmp/image.tar
          ignore-unfixed: "true"
          severity: "CRITICAL,HIGH"
          exit-code: "1"
          trivyignores: ".trivyignore"
      - name: Push image
        uses: docker/build-push-action@5cd11c3a4ced054e52742c5fd54dca954e0edd85 # v6.7.0
        with:
          context: .
          file: deploy/docker/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: VERSION=${{ steps.meta.outputs.version }}
      - uses: sigstore/cosign-installer@4959ce089c160fddf62f7b42464195ba1a56d382 # v3.6.0
      - name: cosign sign
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
        run: |
          for tag in ${TAGS//,/ }; do
            cosign sign --yes "ghcr.io/jtumidanski/harbormaster@${DIGEST}"
          done
        # cosign uses GitHub OIDC; no key material required.
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/main.yml
git commit -m "ci(main): multi-arch buildx, trivy image scan, GHCR push, cosign keyless sign"
```

---

### Task T6.6: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`
- Create: `CHANGELOG.md` (empty seed)

- [ ] **Step 1: Write `release.yml`**

```yaml
name: release
on:
  release:
    types: [published]

permissions:
  contents: write

jobs:
  attach-notes:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
        with: { fetch-depth: 0 }
      - name: Generate notes from CHANGELOG.md
        id: notes
        run: |
          tag="${GITHUB_REF##*/}"
          awk "/^## ${tag}$/{flag=1; next} /^## /{flag=0} flag" CHANGELOG.md > /tmp/notes.md
          [ -s /tmp/notes.md ] || echo "No CHANGELOG entry for ${tag}; see commit history." > /tmp/notes.md
          echo "tag=$tag" >> "$GITHUB_OUTPUT"
      - uses: softprops/action-gh-release@c062e08bd532815e2082a85e87e3ef29c3e6d191 # v2.0.8
        with:
          body_path: /tmp/notes.md
          append_body: true
          files: |
            CHANGELOG.md
```

- [ ] **Step 2: Seed `CHANGELOG.md`**

```markdown
# Changelog

All notable changes to this project will be documented in this file. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v1.0.0

- Initial Harbormaster MVP release. See README for the feature list.
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml CHANGELOG.md
git commit -m "ci(release): attach changelog notes on GitHub Release publish"
```

---

### Task T6.7: Nightly integration workflow

**Files:**
- Create: `.github/workflows/nightly.yml`

- [ ] **Step 1: Write nightly**

```yaml
name: nightly
on:
  schedule:
    - cron: "0 3 * * *"  # 03:00 UTC daily
  workflow_dispatch: {}

permissions:
  contents: read

jobs:
  integration:
    strategy:
      matrix:
        minio: ["RELEASE.2025-01-01T00-00-00Z", "latest"]
    runs-on: ubuntu-24.04
    services: {}  # testcontainers starts MinIO itself
    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
        with: { go-version: "1.24" }
      - env:
          HARBORMASTER_INTEGRATION: "1"
          HARBORMASTER_MINIO_IMAGE: minio/minio:${{ matrix.minio }}
        run: cd apps/backend && go test -race -count=1 -tags=integration ./...
```

- [ ] **Step 2: Read `HARBORMASTER_MINIO_IMAGE` in `internal/integration/helper.go`** so the matrix actually exercises the floor + latest.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/nightly.yml apps/backend/internal/integration
git commit -m "ci(nightly): matrix integration suite (MinIO floor + latest) via testcontainers-go"
```

---

### Task T6.8: README finalization

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace the M0 boilerplate README with the full v1 README**

Sections:
1. **What is Harbormaster** — one-paragraph project overview.
2. **Quick start (Docker Compose)** — three-step path to a working setup.
3. **Production deployment** — Docker, Kubernetes, reverse-proxy notes.
4. **Configuration reference** — every `HARBORMASTER_*` env var with default and meaning.
5. **Security model** — encryption at rest, sessions, CSRF, audit, supported MinIO floor.
6. **Importing from mc config** — bind-mount instructions + opt-in rationale.
7. **Recovery** — `admin reset-password`, `admin reset-encryption --confirm` one-liners (Docker + Kubernetes).
8. **GHCR first-publish reminder** — flip package visibility to public after the first `main.yml` run.
9. **License** — AGPL boilerplate.
10. **Project goals + non-goals** — copied verbatim from PRD §2 for quick alignment.
11. **Links** — PRD location, architecture overview, security guide.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: finalize README with quick-start, config reference, security model, recovery"
```

---

### Task T6.9: Operator documentation

**Files:**
- Create: `docs/architecture/overview.md`
- Create: `docs/operator/configuration.md`
- Create: `docs/operator/security.md`
- Create: `docs/operator/recovery.md`
- (Already created: `docs/operator/reverse-proxy.md`.)

- [ ] **Step 1: Architecture overview**

A condensed walkthrough of the codebase: top-level layout, bounded contexts (per `design.md` §2), cross-cutting concerns, request lifecycle diagram (textual). Should match the implemented code, not the design speculation — verify by walking each section against the actual `apps/backend/internal` packages.

- [ ] **Step 2: Configuration reference**

Tabular env-var reference with name, default, valid values, effect. Source of truth is `internal/config/config.go`. Include a small section on the precedence order.

- [ ] **Step 3: Security guide**

- Threat model summary.
- Encryption key handling (backup advice, fingerprint check, perm requirements, recovery via `reset-encryption`).
- Session + CSRF behavior; cookie attributes.
- Single-replica deployment note (R6).
- Share-link non-revocability (R17).
- mc config exposure caveat (R18).
- AGPL implications for hosters.

- [ ] **Step 4: Recovery guide**

Step-by-step for both `reset-password` and `reset-encryption --confirm`, with explicit Docker and Kubernetes invocations. Cover what's preserved (admin account, audit history) vs lost (MinIO connection settings, custom CA) after `reset-encryption`.

- [ ] **Step 5: Commit**

```bash
git add docs
git commit -m "docs(operator): architecture, configuration, security, recovery guides"
```

---

### Task T6.10: Backend `version` subcommand reads ldflag

**Files:**
- Modify: `apps/backend/cmd/harbormaster/version.go`
- Modify: `apps/backend/Makefile` (add `-ldflags="-X main.version=...")

- [ ] **Step 1: Build with ldflags so `harbormaster version` prints the tag**

Already wired in the Dockerfile (T6.1). For local builds, update the Makefile `build` target:

```make
VERSION ?= $(shell git describe --tags --always --dirty)

build:
	CGO_ENABLED=0 go build -ldflags="-X main.version=$(VERSION)" -o bin/harbormaster ./cmd/harbormaster
```

- [ ] **Step 2: Commit**

```bash
git add apps/backend/Makefile
git commit -m "chore(backend): thread VERSION ldflag into local makefile builds"
```

---

### Task T6.11: Playwright smoke test

**Files:**
- Create: `apps/frontend/playwright.config.ts`
- Create: `apps/frontend/e2e/smoke.spec.ts`
- Modify: `apps/frontend/package.json` (test:e2e already added in T0.8)

- [ ] **Step 1: Install Playwright**

```bash
cd apps/frontend && npm install -D @playwright/test@1.46.0 && npx playwright install --with-deps chromium
```

- [ ] **Step 2: `playwright.config.ts`**

```ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  use: {
    baseURL: process.env.HARBORMASTER_E2E_URL ?? "http://localhost:8080",
    headless: true,
    viewport: { width: 1280, height: 800 },
  },
  retries: 1,
});
```

- [ ] **Step 3: `e2e/smoke.spec.ts`**

```ts
import { test, expect } from "@playwright/test";

test("setup → login → buckets golden path", async ({ page }) => {
  await page.goto("/");
  // Setup wizard
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password", { exact: true }).fill("correct horse battery staple!");
  await page.getByLabel("Confirm password").fill("correct horse battery staple!");
  await page.getByRole("button", { name: /next/i }).click();
  await page.getByLabel("Endpoint URL").fill("http://minio:9000");
  await page.getByLabel("Access key").fill("admin");
  await page.getByLabel("Secret key").fill("admin12345");
  await page.getByRole("button", { name: /finish/i }).click();
  // Login
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password", { exact: true }).fill("correct horse battery staple!");
  await page.getByRole("button", { name: /sign in/i }).click();
  // Buckets
  await expect(page.getByRole("heading", { name: /buckets/i })).toBeVisible();
});
```

- [ ] **Step 4: Manually run against `docker compose --profile with-minio up`**

```bash
cd apps/frontend && HARBORMASTER_E2E_URL=http://localhost:8080 npm run test:e2e
```

- [ ] **Step 5: Commit**

```bash
git add apps/frontend
git commit -m "test(frontend/e2e): Playwright golden-path smoke for setup→login→buckets"
```

---

### Task T6.12: Final code review

- [ ] **Step 1: Run `superpowers:requesting-code-review` over the whole task branch**

Dispatch in parallel:
- `plan-adherence-reviewer` (full plan.md)
- `backend-guidelines-reviewer` (all Go)
- `frontend-guidelines-reviewer` (all TS)

Findings land in `docs/tasks/task-001-harbormaster-mvp-v1/audit.md`.

- [ ] **Step 2: Address findings; fix until all FAIL items are PASS.**

---

### Task T6.13: Full acceptance criteria walkthrough

- [ ] **Step 1: Walk the PRD §10 acceptance criteria checklist top to bottom**

For each unchecked item in PRD §10, manually verify and check it off. Items that aren't verifiable without the live stack are deferred to the manual smoke pass.

- [ ] **Step 2: Manual smoke against a real stack**

```bash
docker compose -f deploy/docker/docker-compose.yml --profile with-minio up --build
```

Walk through every PRD §10 item end-to-end. Document any deviations in `docs/tasks/task-001-harbormaster-mvp-v1/audit.md`.

- [ ] **Step 3: Commit final docs touch-ups uncovered by the walkthrough**

---

### Task T6.14: Open the PR

- [ ] **Step 1: Push the task branch**

```bash
git push -u origin task-001-harbormaster-mvp-v1
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat: Harbormaster MVP v1" --body "$(cat <<'EOF'
## Summary

Implements the complete Harbormaster v1 MVP per `docs/tasks/task-001-harbormaster-mvp-v1/prd.md`.

Six internal milestones landed as stacked sub-branches (now squashed/merged into this single PR):

- **M0** — Repo scaffolding & CI baseline
- **M1** — Backend platform (config, db, crypto, audit, server, CLI)
- **M2** — Setup wizard + auth + connection
- **M3** — Buckets + objects + lifecycle (largest)
- **M4** — Users + service accounts + policies
- **M5** — Dashboard + activity
- **M6** — Deployment, CI/CD, supply chain

## Test plan

- [ ] `cd apps/backend && go vet ./... && go test -race ./... && golangci-lint run`
- [ ] `cd apps/backend && HARBORMASTER_INTEGRATION=1 go test -tags=integration ./...`
- [ ] `cd apps/frontend && npm run lint && npm test && npm run build`
- [ ] `docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .`
- [ ] PR workflow passes (frontend lint/test/build, backend lint/test/build, gitleaks, trivy fs scan, license allowlist).
- [ ] Manual smoke against `docker compose --profile with-minio up`: wizard, login, bucket CRUD, object upload/download/share-link, empty-bucket SSE, lifecycle rule, user with backup-target template, service account, dashboard + recent failures widget, activity feed.

## License

AGPL-3.0-or-later.
EOF
)"
```

- [ ] **Step 3: Verify CI passes; address review feedback; merge.**

---

## Self-Review Summary

A spec-coverage scan of `design.md` against this plan:

- §1.1 stack — locked across T0–T1 dep installs.
- §1.2 open-question decisions — every one is reflected in a concrete task (e.g., #14 → T3.4 persistent table; #15 → T3.12 tag-count summary; #10 → T6.5 cosign).
- §2 bounded contexts — each context has a `[skill scaffold]` task: `auth` T2.1, `setup` T2.4, `connection` T2.5, `buckets` T3.1, `objects` T3.7–T3.10, `users` T4.3, `policies` T4.1–T4.2, `lifecycle` T3.12, `dashboard` T5.1, `audit` T1.9 + T5.2.
- §3 cross-cutting concerns — config T1.2, db+retry T1.4, crypto T1.5, auth+CSRF T2.2, audit sanitizer T1.9, empty-job T3.4, MinIO pool T1.10, error envelope T1.7, SPA serving T2.14, health T2.7, frontend cross-cutting T2.8–T2.10.
- §4 milestone plan — M0–M6 line up exactly with the design's six-phase shape.
- §5 component-detail notes — encoder shape T1.6, middleware order T1.11, SSE wire format T3.5, object browser data flow T3.16, upload pipeline T3.8, download modes T3.9, share-link T3.10 / T3.18, lifecycle readout T3.12, policy materialization T4.2, frontend route map T2.9 / T5.4 / T5.5.
- §7 build & verification — codified in T0.12 (`CLAUDE.md`) and re-asserted in T0.14 / T1.17 / T2.17 / T3.29 / T4.13 / T5.9 / T6.13.

Risk-coverage scan against `risks.md`:

- R1 — milestone subdivision is the spine of this plan.
- R2 — T1.5 (perms + fingerprint), T1.14 (`reset-encryption`), T6.9 (recovery docs).
- R3 — T1.1 (pure-Go SQLite), T0.7 (`CGO_ENABLED=0`).
- R4 — T3.5 (Empty SSE with confirmation), T3.15 (Empty dialog with versioning-aware copy), T3.24 (audit payload completeness).
- R5 — T3.16 (virtualized + 90 % auto-load + one-outstanding + manual button), T5.8 (10k-prefix SLO test).
- R6 — T6.4 (`replicas:1` + `Recreate`), T6.8 (README + security doc).
- R7 — T6.4 (heavily commented manifests).
- R8 — T1.13, T1.14 (CLIs), T6.9 (recovery doc).
- R9 — T2.8 (`lib/api/client.ts` dual envelope), T1.7 (`apierror` Style).
- R10 — T0.9 (`.trivyignore`), T6.5 (image-scan exit on CRITICAL/HIGH).
- R11 — T6.8 (README publish-public reminder).
- R12 — T2.14 (SPA registration order test).
- R13 — T1.1 / T0.9 (split + tag), T6.7 (nightly).
- R14 — T0.9 (license allowlist job), T6.8 (README AGPL boilerplate).
- R15 — T6.7 (matrix MinIO floor + latest), T6.8 (README support window).
- R16 — T1.7 (`X-Accel-Buffering` + heartbeat), T6.3 (reverse-proxy snippets).
- R17 — T3.10 (audit-without-URL), T3.18 (modal copy).
- R18 — T2.4 (gated on `setup_completed=false`), T0.7 (commented-out compose mount), T2.4 tests (no persistence).

Placeholder scan: every code block contains the concrete identifier the engineer needs to type or reference. The `[skill scaffold]` shorthand is defined under "Conventions" — it isn't a placeholder, it's a pointer to a reusable cross-file pattern.

Type-consistency scan: `Bucket`, `Quota`, `PublicAccess`, `Rule`, `User`, `TemplateRef`, `Event`, `SessionInfo`, `Window`, `Progress`, `Result` keep stable names from first introduction onward. The audit `Event` type stays consistent between M1 (write path), M5 (query path), and dashboard's `RecentActivity`/`RecentFailures` fields.

---

## Execution Handoff

Plan complete and saved to `docs/tasks/task-001-harbormaster-mvp-v1/plan.md`. Companion context at `context.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration. Required sub-skill: `superpowers:subagent-driven-development`.

**2. Inline Execution** — Execute tasks in this session using `superpowers:executing-plans`, batched with checkpoints between milestones.

Given M3 alone is ~30 tasks (and the whole plan is ~125), subagent-driven is the only realistic path. Each milestone (M0–M6) is also a natural review checkpoint — run `superpowers:requesting-code-review` between them rather than at the very end.

**Next phase:** `/clear`, then `/execute-task task-001` from inside this worktree.







