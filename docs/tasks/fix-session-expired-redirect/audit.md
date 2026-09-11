# Frontend Audit — fix-session-expired-redirect

- **Audit Scope:** `apps/frontend/src` diff, `origin/main...HEAD` (merge commit `0d7b6b6`): `context/AuthContext.tsx`, `features/auth/LoginPage.tsx`, `features/buckets/useEmptyBucket.ts`, `features/objects/UploadDialog.tsx`, `lib/api/queryClient.ts` (new), `main.tsx`, `routes.tsx`, plus their test files.
- **Guidelines Source:** frontend-dev-guidelines skill
- **Date:** 2026-09-10
- **Build:** PASS (reported by requester: `npx tsc --noEmit`, `npm run build`)
- **Tests:** 27 files / 112 tests passing (reported by requester)
- **Overall:** PASS

## Build & Test Results

Not re-run per instructions — requester already verified `npx tsc --noEmit` clean, `npm test` (27 files / 112 tests), `npm run lint` (0 errors, 7 pre-existing warnings), `npm run format` clean, `npm run build` succeeds.

## File Inventory

- `apps/frontend/src/context/AuthContext.tsx` — Context (auth session hook + logout)
- `apps/frontend/src/context/AuthContext.test.tsx` — Test
- `apps/frontend/src/features/auth/LoginPage.tsx` — Component (feature page)
- `apps/frontend/src/features/auth/LoginPage.test.tsx` — Test
- `apps/frontend/src/features/buckets/useEmptyBucket.ts` — Hook (feature-local, not `lib/hooks/api`)
- `apps/frontend/src/features/buckets/useEmptyBucket.test.ts` — Test
- `apps/frontend/src/features/objects/UploadDialog.tsx` — Component
- `apps/frontend/src/features/objects/UploadDialog.test.tsx` — Test
- `apps/frontend/src/lib/api/queryClient.ts` — Other (new: QueryClient factory + centralized 401 handling)
- `apps/frontend/src/lib/api/queryClient.test.ts` — Test (new)
- `apps/frontend/src/main.tsx` — Other (root bootstrap)
- `apps/frontend/src/routes.tsx` — Other (route gate / declarative redirects)
- `apps/frontend/src/routes.test.tsx` — Test

## Semantic Verification of Conflict Resolutions

1. **`react-router-dom` → `react-router` rename (`routes.tsx`, `main.tsx`).** Confirmed `origin/main`'s `routes.tsx` and `main.tsx` (`git show origin/main:apps/frontend/src/routes.tsx` / `main.tsx`) are structurally identical to the branch's pre-merge base plus the package rename — no functionality from main's 130 commits was dropped in this file. `useLocation` (`routes.tsx:1`) and `QueryClientProvider`/`BrowserRouter` (`main.tsx:5-6`) correctly import from `react-router`/`@tanstack/react-query`, matching main's renamed import path. PASS.

2. **Dropping `useNavigate`/`await navigate("/buckets")` from `LoginPage.tsx` in favor of declarative `RedirectAfterLogin`.** Verified no other code depends on `LoginPage`'s imperative navigate: `grep -rn "navigate(\"/buckets\")"` across `apps/frontend/src` only matches `features/buckets/DeleteBucketDialog.tsx:50`, an unrelated component with its own independent `useNavigate()` call — not affected by this change. `git show origin/main:apps/frontend/src/features/auth/LoginPage.tsx` shows main's version is byte-identical to the branch's pre-conflict `onSuccess` handler except for the extra `await navigate("/buckets")` line — nothing else on main was layered on top of it. The declarative replacement is exercised end-to-end by three new `routes.test.tsx` cases (`routes.tsx:636-778`): redirect-to-login-and-back-to-origin, direct `/login` visit landing on `/buckets`, and the general 401→login case. Semantically correct — the comment at `routes.tsx:34-37` correctly explains the race the old imperative call had (still-mounted unauthenticated `Routes` swallow the navigate before the gate re-renders). PASS.

3. **Centralized 401 handling via `createQueryClient` (`lib/api/queryClient.ts`).** Traced the full flow: `isSessionExpiredError` keys on `status === 401 && code === "unauthenticated"` (`queryClient.ts:491-494`), which correctly excludes `invalid_credentials` (login/change-password failures) — confirmed by test `queryClient.test.ts:440-455`. `resetToSignedOut` seeds `me` null before removing other queries (`queryClient.ts:503-507`), preserving `authKeys.setupStatus()` so the route gate doesn't flash a loading state (`queryClient.ts:501-502`, test at `queryClient.test.ts:417-426`). `handleSessionExpiry` guards against re-firing when already signed out (`queryClient.ts:516-521`, test at `queryClient.test.ts:467-475`). `AuthContext`'s own `/auth/me` query catches its 401 locally and returns `null` instead of throwing (`AuthContext.tsx:23-28`), so it never re-enters the global `QueryCache`/`MutationCache` `onError` handler — no double-handling / infinite loop risk. Raw-fetch/XHR bypass paths (`useEmptyBucket.ts:258`, `UploadDialog.tsx:150`) correctly call `handleSessionExpiry` manually since they don't go through the shared `api` client's cache-integrated error path, and both skip local error-state rendering only when `handleSessionExpiry` returns `true`; non-expiry 401s and other statuses still fall through to existing local error handling (`UploadDialog.tsx:152-183`). `AuthContext.logout` now shares the exact same `resetToSignedOut` (`AuthContext.tsx:48`) — was duplicated logic (`qc.clear()` + `setQueryData`) before; now unified. PASS.

4. **`tsconfig.tsbuildinfo`.** Took main's copy and let the build regenerate it — out of `apps/frontend/src` scope and not part of the FE-* checklist; not evaluated further. This file being tracked in git at all predates this branch.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -n ": any\|as any"` across all 7 in-scope source files: zero matches. |
| FE-02 | No manual class concatenation | PASS | No `className={"..." +` or template-string concatenation in the diff; only pre-existing static `className="p-8"` strings untouched (`routes.tsx:46`). |
| FE-03 | No direct API client calls in components | PASS | `LoginPage.tsx`, `UploadDialog.tsx`, `useEmptyBucket.ts` import only `handleSessionExpiry`/`readCsrfCookie`/service functions, not `@/lib/api/client`. `AuthContext.tsx:3` imports `api` from `@/lib/api/client`, but this is a context/provider (auth boundary), the established pre-existing pattern, unchanged by this diff. |
| FE-04 | No inline Zod schemas in components | PASS | No `z.object(`/`z.string(` added anywhere in the diff. |
| FE-05 | No spinners for content loading | PASS | `grep -n "animate-spin"` in `routes.tsx` and `LoginPage.tsx`: zero matches. No spinner added elsewhere in the diff. |
| FE-06 | No hardcoded colors | PASS | No `bg-white`/`bg-gray-*`/etc. added in the diff. |
| FE-07 | No state mutation | PASS | All new state updates are `setQueryData`/`removeQueries`/`setState` with fresh values (e.g. `queryClient.ts:504-506`); no `.push()`/`.splice()`/in-place mutation. |
| FE-08 | No default exports for components | PASS | `grep -n "export default function"` in `routes.tsx`, `AuthContext.tsx`, `LoginPage.tsx`, `queryClient.ts`: zero matches. All new functions (`RedirectToLogin`, `RedirectAfterLogin`, `createQueryClient`, `resetToSignedOut`, `handleSessionExpiry`, `isSessionExpiredError`) are named exports/declarations. |
| FE-09 | Error handling with `createErrorFromUnknown` | NON-BLOCKING | `useEmptyBucket.ts:163-169` and `UploadDialog.tsx:171-183` set local error state directly from `err instanceof AppError`/`Error` checks rather than routing through `createErrorFromUnknown()` — but this is **pre-existing** code, not touched by this diff except for the new `handleSessionExpiry` short-circuit inserted above it. `queryClient.ts:518` calls `toast.error()` directly with a fixed string (not from a caught error object), which is consistent with the guideline's "toast for user feedback" rule but doesn't invoke `createErrorFromUnknown` since there is no per-call error object to transform (the trigger is a boolean check, not a catch). Not a regression introduced by this branch. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | No new model types added in this diff. |
| FE-11 | Service extends `BaseService` | N/A | No new service classes added; `queryClient.ts` is infrastructure (React Query client factory), not a service. |
| FE-12 | Query key factory uses `as const` | PASS | `authKeys` (`lib/api/keys.ts:1-6`, pre-existing, referenced by the new code) uses `as const` on every branch. No new key factories added. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `LoginPage.tsx` retains `useForm({ resolver: zodResolver(loginSchema) })` — unchanged by this diff aside from removing the navigate call. |
| FE-14 | Schema in `lib/schemas/` with inferred type | PASS | `LoginPage.tsx` still imports `loginSchema`/`LoginInput` from `@/lib/schemas/auth` — unchanged. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | N/A | No new interactive non-`<button>`/`<a>` elements introduced by this diff (`RedirectToLogin`/`RedirectAfterLogin` render `<Navigate>`, not interactive markup). |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | Every changed source file has a corresponding updated/new test: `AuthContext.test.tsx` (new logout test, `:9-66`), `LoginPage.test.tsx` (updated assertion, `:110-125`), `useEmptyBucket.test.ts` (new 401 test, `:184-225`), `UploadDialog.test.tsx` (new 401 test, `:287-323`), `queryClient.test.ts` (new file, full coverage of `createQueryClient`/`resetToSignedOut`/`handleSessionExpiry`), `routes.test.tsx` (3 new end-to-end redirect tests, `:636-778`). |
| FE-17 | Mocks updated when services changed | N/A | No `__mocks__/` directory exists in this codebase (`find src -iname "__mocks__"` returns nothing) — this project doesn't use the Jest manual-mock pattern; not applicable. |

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- FE-09: `useEmptyBucket.ts:163-169` and `UploadDialog.tsx:171-183` still build local error messages via manual `instanceof` checks rather than `createErrorFromUnknown()`. Pre-existing pattern, not introduced by this branch, but worth a follow-up cleanup since this branch touched both call sites anyway.
