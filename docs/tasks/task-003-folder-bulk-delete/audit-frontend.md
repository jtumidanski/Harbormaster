# Frontend Audit — task-003-folder-bulk-delete

- **Audit Scope:** Changed TS/TSX in range `818f5ed..HEAD` — `components/ui/checkbox.tsx`, `features/objects/api.ts`, `features/objects/BulkDeleteDialog.tsx`, `features/objects/BulkDeleteDialog.test.tsx`, `features/objects/VirtualizedObjectList.tsx`, `features/objects/ObjectBrowserPage.tsx`, `features/objects/ObjectBrowserPage.test.tsx`
- **Guidelines Source:** frontend-dev-guidelines skill
- **Date:** 2026-06-12
- **Build:** PASS
- **Tests:** 98 passed, 0 failed (25 files)
- **Overall:** PASS

## Build & Test Results

Toolchain: node v22.22.2 (cwd `apps/frontend`).

- `npm run lint` — PASS (0 errors; 3 pre-existing `react-hooks/exhaustive-deps` warnings in unrelated `service-accounts` / `users` dialogs, none in scope).
- `npm run format` (`prettier --check .`) — PASS ("All matched files use Prettier code style!").
- `npx tsc --noEmit` — PASS (no output, exit 0).
- `npm test` — PASS (25 files, 98 tests). In-scope suites: `ObjectBrowserPage.test.tsx` (5), `BulkDeleteDialog.test.tsx` (3).
- `npm run build` — PASS (exit 0; pre-existing >500 kB chunk-size advisory only, not an error).

## File Inventory

- `components/ui/checkbox.tsx` — Component (ui/ shadcn-style primitive, native input)
- `features/objects/api.ts` — Service (feature-local API client functions)
- `features/objects/BulkDeleteDialog.tsx` — Component (feature container; useQuery + useMutation)
- `features/objects/BulkDeleteDialog.test.tsx` — Test
- `features/objects/VirtualizedObjectList.tsx` — Component (presentational list/rows)
- `features/objects/ObjectBrowserPage.tsx` — Page-equivalent container (selection state, toolbar, dialog wiring)
- `features/objects/ObjectBrowserPage.test.tsx` — Test

> Note on architecture: this codebase uses a feature-folder layout (`features/objects/*`) with co-located feature-local `api.ts` rather than the `pages/` + `services/api/BaseService` + `lib/hooks/api/` layout the guideline resources describe. Several guideline checks (FE-11 BaseService, FE-12 hook-file key factory, FE-09 `createErrorFromUnknown`) describe an idiom this repo does not use. Scored against the **established repo convention** the changed code follows; deviations from the literal guideline that match repo norm are flagged informational, not blocking.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | grep `: any`/`as any` over all 5 source files — no matches. `unknown` + narrowing used instead: `BulkDeleteDialog.tsx:70` (`err: unknown`), `VirtualizedObjectList.tsx:96` (`ret: unknown`). |
| FE-02 | No manual class concatenation | PASS | No `className={... + ...}` or template-literal className. All conditional classes use `cn()`: `checkbox.tsx:16`. Other files use static class strings. |
| FE-03 | No direct API client in components | INFO | `BulkDeleteDialog.tsx:14` and `ObjectBrowserPage.tsx:23` import from `./api` (feature service module), not `@/lib/api/client`. Only `features/objects/api.ts:1` imports the raw client — that file *is* this feature's service layer. Matches repo convention (cf. `ShareLinkDialog`, `ObjectVersionsSheet`). |
| FE-04 | No inline Zod schemas | PASS | No `z.*(` in any changed component (no forms introduced; bulk-delete is a confirm dialog, not a data-entry form). |
| FE-05 | No spinners for content loading | PASS | No `animate-spin` anywhere in scope. Loading states use text (`BulkDeleteDialog.tsx:87` "Counting objects…", `:114` "Deleting…"; `VirtualizedObjectList.tsx:251` "Loading…"; `ObjectBrowserPage.tsx:155` "Loading…"). No raw spinner in content. |
| FE-06 | No hardcoded colors | PASS | Semantic tokens only: `bg-background`, `bg-accent/30`, `bg-accent/40`, `text-muted-foreground`, `text-destructive`, `text-primary`, `border-input`, `accent-primary`. grep for `bg-white/black/gray-N/red-N`, `text-gray-N` — no matches. |
| FE-07 | No state mutation | PASS | Selection updates are immutable Set copies: `ObjectBrowserPage.tsx:50-54` and `:59-63` (`new Set(prev)` then add/delete, return new). `.sort()` at `BulkDeleteDialog.tsx:46-47` operates on `[...keys]`/`[...prefixes]` copies, not props/state. |
| FE-08 | No default exports for components | PASS | grep `export default` — none. Named exports: `checkbox.tsx:28`, `BulkDeleteDialog.tsx:33`, `VirtualizedObjectList.tsx:61`, `ObjectBrowserPage.tsx:33`. |
| FE-09 | Error handling surfaced to user | PASS (repo idiom) | Repo has no `createErrorFromUnknown` helper (grep: 0 hits in `src/`). Established pattern is `AppError` + `instanceof AppError` + toast — followed exactly at `BulkDeleteDialog.tsx:70-73` and `ObjectBrowserPage.tsx:99-102` (matches `ObjectVersionsSheet.tsx:139-140`, `ShareLinkDialog.tsx:91-92`). Both query and mutation error paths surface to the user (mutation → toast; list query error → `ObjectBrowserPage.tsx:157-160`; preview error → `BulkDeleteDialog.tsx:89-90` + disabled Delete `:111`). |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS (where applicable) | List/version models keep `{id, attributes}` (consumed at `VirtualizedObjectList.tsx:138,173`). Bulk-delete request/response are **action-endpoint plain-JSON** types (`api.ts:131-144`), consistent with the file's existing plain-JSON actions (`restoreVersion` `:96`, `undeleteObject` `:122`, `createShareLink` body `:52`). See FE-typing note below. |
| FE-11 | Service extends BaseService | N/A (repo idiom) | Repo does not use `BaseService`; feature-local `api.ts` functions call `api.*` directly — the documented "direct client" alternative. `bulkDelete`/`previewBulkDelete` (`api.ts:148-168`) follow the file's existing style. |
| FE-12 | Query key uses `as const` | PASS (mutation key) / INFO (inline preview key) | List invalidation uses the central factory `objectsKeys.list(bucket, listPrefix)` which is `as const` (`keys.ts:20`), referenced at `BulkDeleteDialog.tsx:59` and `ObjectBrowserPage.tsx:95`. The dry-run preview uses an **inline** array key `["objects", bucket, "bulk-delete-preview", sortedKeys, sortedPrefixes]` (`BulkDeleteDialog.tsx:51`) not added to `objectsKeys`. Functionally stable (sorted → reorder-insensitive) but not centralized — informational. |
| FE-13 | Forms use rhf + zodResolver | N/A | No form introduced; destructive confirm dialog only. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schema introduced. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Native `<button>`/`<a>` get pointer from browser (all row actions and toolbar buttons are `<Button>`/`<button>`: `VirtualizedObjectList.tsx:151,160,186,198,207,216,225,244`; `ObjectBrowserPage.tsx` toolbar `:133,146`). The one custom interactive surface — the native checkbox — explicitly sets `cursor-pointer` (`checkbox.tsx:17`) and `disabled:cursor-not-allowed` (`:19`). No clickable `<div>`/row-as-trigger introduced. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `BulkDeleteDialog.test.tsx` exercises real behavior: dry-run count render (`:45-54`), truncated `10,000+` (`:56-63`), and partial-failure toast + `onDeleted` callback driven by inspecting the request body for `"dry_run":false` (`:65-83`). `ObjectBrowserPage.test.tsx` adds selection-toolbar-after-checkbox + dialog open (`:304-341`) and folder-row trash → dialog (`:343-374`). Assertions verify rendered counts and DOM, not mock call shape only. Checkbox is a trivial primitive (no standalone test — acceptable). |
| FE-17 | Mocks updated when services changed | PASS | Tests stub `fetch` directly (no `__mocks__/` service-mock layer in this repo). New bulk-delete endpoint is covered by per-test fetch stubs that branch on `dry_run` (`BulkDeleteDialog.test.tsx:66-74`; `ObjectBrowserPage.test.tsx:307-324,344-361`). |

## Summary

### Blocking (must fix)
- None. Build, lint, format, tsc, and all 98 tests pass; no FE-* check FAILs.

### Non-Blocking (should fix)
- **FE-12 (informational):** The dry-run preview query key is an inline literal in `BulkDeleteDialog.tsx:51` rather than a member of the `objectsKeys` factory in `lib/api/keys.ts`. Consider adding `objectsKeys.bulkDeletePreview(bucket, sortedKeys, sortedPrefixes)` for consistency and to make targeted invalidation possible later. Current form is stable and correct.
- **Query-key vs queryFn arg consistency (informational):** the queryKey is built from `sortedKeys`/`sortedPrefixes` (`:51`) while the `queryFn` passes the **unsorted** `keys`/`prefixes` (`:52`). Harmless today (server is order-insensitive; identical selection yields identical key), but the asymmetry is a latent footgun if the backend ever became order-sensitive. Passing the sorted arrays to `previewBulkDelete` (and to `bulkDelete` at `:57`) would remove the ambiguity.
- **FE-09 / FE-11 / FE-12 (idiom drift, repo-wide, not introduced here):** this feature follows the repo's `AppError` + feature-local `api.ts` + inline-key convention rather than the skill's `createErrorFromUnknown` / `BaseService` / hook-file-factory idiom. Not a regression — the changed code is consistent with surrounding `features/objects/*`. Flagged only so the guideline/codebase divergence is on record.
