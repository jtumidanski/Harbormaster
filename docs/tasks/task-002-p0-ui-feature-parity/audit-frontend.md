# Frontend Audit — task-002-p0-ui-feature-parity

- **Audit Scope:** TypeScript/React diff vs `main` (`git diff main...HEAD -- 'apps/frontend/**'`): lifecycle kind selector, objects version sheet, policies page + editor, users custom-policy attach, metrics page, `lib/api/{keys,errors}.ts`, `components/ui/textarea.tsx`, `AppShell.tsx`, `routes.tsx`, `test/setup.ts`.
- **Guidelines Source:** frontend-dev-guidelines skill (FE-* checklist)
- **Date:** 2026-06-05
- **Build:** PASS
- **Tests:** 93 passed, 0 failed (24 files)
- **Overall:** NEEDS-WORK (build + tests + lint + format all green; two non-blocking guideline divergences, both consistent with pre-existing codebase conventions)

> **Convention note:** The literal paths in the guideline (`services/api/` + `BaseService`, `lib/hooks/api/`, Jest) do **not** match this codebase. The actual architecture is: per-feature `features/<x>/api.ts` wrappers over the singleton `@/lib/api/client` (this *is* the service layer), query-key factories centralized in `src/lib/api/keys.ts`, and Vitest + RTL. FE-03/FE-11/FE-12/FE-16 are judged against the project's real architecture; the *principle* (components never touch the raw client; keys are `as const`; tests exist) is enforced.

## Build & Test Results

```
npm run build   → exit 0 (PASS)
npm test        → vitest run: Test Files 24 passed (24), Tests 93 passed (93), exit 0
npm run lint    → exit 0; 3 warnings (react-hooks/exhaustive-deps), all PRE-EXISTING and
                  NOT in net-new code (service-accounts/CreateServiceAccountDialog.tsx:92,
                  users/CreateUserDialog.tsx:87, users/EditPoliciesDialog.tsx:72 — the latter
                  warning is on an unchanged useMemo, not the +2 lines this PR added)
npm run format  → prettier --check . → exit 0 (PASS)
tsconfig.tsbuildinfo → restored via git checkout; working tree clean
```

## File Inventory

- **Page** — `features/metrics/MetricsPage.tsx` (new), `features/policies/PoliciesPage.tsx` (new, replaces PolicyTemplatesPage.tsx), `features/users/UserDetailPage.tsx`
- **Component** — `features/lifecycle/CreateRuleDialog.tsx`, `features/lifecycle/LifecycleRulesTab.tsx`, `features/objects/ObjectVersionsSheet.tsx` (new), `features/objects/ObjectBrowserPage.tsx`, `features/objects/VirtualizedObjectList.tsx`, `features/policies/PolicyEditorDialog.tsx` (new), `features/users/EditCustomPoliciesDialog.tsx` (new), `features/users/EditPoliciesDialog.tsx`, `features/buckets/BucketDetailPage.tsx`, `components/AppShell.tsx`, `components/ui/textarea.tsx` (new shadcn primitive)
- **Service (feature api.ts)** — `features/lifecycle/api.ts`, `features/objects/api.ts`, `features/metrics/api.ts`, `features/policies/policiesApi.ts` (new), `features/users/api.ts`
- **Schema** — inline (CreateRuleDialog discriminated union, PolicyEditorDialog editorSchema)
- **Type** — `features/objects/types.ts`, `features/metrics/types.ts`, `features/policies/types.ts` (new), `features/users/types.ts`, `lib/api/errors.ts` (AppErrorDetails path)
- **Keys** — `lib/api/keys.ts` (objectsKeys.versions, policiesKeys, metricsKeys)
- **Other** — `routes.tsx`, `test/setup.ts` (ResizeObserver shim)

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grep of all non-test changed `.ts`/`.tsx` for `: any` / `as any` / `<any>` → zero matches. Dynamic data is handled with `unknown` + narrowing (e.g. `policiesApi` `document: unknown`; PoliciesPage.tsx:71 `err.details as {…} \| undefined`; MetricsPage.tsx:36 `isMetricsWindow` type guard). |
| FE-02 | No manual class concatenation | PASS | Grep for template-string / `+` className concat → zero matches. Conditional classes use `cn()` (LifecycleRulesTab.tsx:52; textarea.tsx:10). |
| FE-03 | No raw API client in components | PASS | No changed `.tsx` imports `@/lib/api/client`. All client access is behind feature `api.ts` wrappers; components import `listPolicies`, `createRule`, `fetchMetrics`, `listVersions`, etc. |
| FE-04 | No inline Zod in components | WARN (non-blocking) | Inline schemas at PolicyEditorDialog.tsx:30 (`editorSchema`) and CreateRuleDialog.tsx:42 (`ruleSchema` discriminated union). Guideline says schemas live in `lib/schemas/`. **Mitigating:** this is the established codebase pattern — CreateRuleDialog/CreateServiceAccountDialog/CreateUserDialog already inline schemas on `main`. The CreateRuleDialog union is form-specific (coercion + per-branch fields) and arguably falls under the documented cross-field/`.refine()` exception. |
| FE-05 | No spinners for content loading | PASS | Grep for `animate-spin` in changed files → zero matches. Loading states use text/empty placeholders (MetricsPage.tsx:374, ObjectVersionsSheet.tsx:196, LifecycleRulesTab.tsx:125). Submit buttons use text ("Creating…"/"Saving…") not spinners. |
| FE-06 | No hardcoded colors | WARN (non-blocking) | Palette colors used for status accents with dark-mode variants: CreateRuleDialog.tsx:219 (amber warning), LifecycleRulesTab.tsx:40/42/44 (blue/violet/orange kind badges), ObjectVersionsSheet.tsx:179-180/247 (amber delete-marker). Guideline (anti-patterns §6) mandates semantic CSS vars. **Mitigating:** identical amber/blue palette usage is pre-existing on `main` (BucketDetailPage.tsx:58/72, unchanged by this PR) — this is an accepted convention for multi-state status accents the semantic token set (`destructive`/`muted`/`primary`) cannot express. Default text/surfaces correctly use `text-muted-foreground`/`bg-background`/`text-destructive`. |
| FE-07 | No state mutation | PASS | Immutable updates throughout: EditCustomPoliciesDialog.tsx:72-80 (`new Set(prev)` then return); UserDetailPage.tsx:54 optimistic `{...prev, status}`; MetricsPage.tsx:107 `{...existing}`. No `.push`/`.splice`/`.sort` on state (MetricsPage.tsx:110 sorts a freshly-built `Array.from(map.values())`, not state). |
| FE-08 | No default exports for components | PASS | Grep `export default` in changed files → zero matches. All components named-exported. |
| FE-09 | Error handling via AppError + toast/error state | PASS | Every mutation `onError` narrows with `err instanceof AppError` and surfaces via `toast.error` or `form.setError`: PoliciesPage.tsx:68-91 (incl. `policy_in_use`/`policy_read_only`), PolicyEditorDialog.tsx:100-114 (pointer→field), CreateRuleDialog.tsx:149-163 (422 pointer→field), EditCustomPoliciesDialog.tsx:58-68 (`unknown_policy`), ObjectVersionsSheet.tsx:138-167, UserDetailPage.tsx:61-68 (rollback + toast). Queries render `err instanceof AppError ? err.message : fallback`. *(Project convention uses `AppError` narrowing rather than the guideline's `createErrorFromUnknown`, which does not exist in this codebase.)* |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | Wire types use `{ type, id, attributes }`: objects/types.ts:18-20,45-49; policies/types.ts:14-32; lifecycle/api.ts:30-35. api.ts wrappers unwrap `res.data.attributes` (policiesApi.ts:6, objects/api.ts:54). Domain-facing shapes flatten to plain records intentionally (e.g. `Policy`, `MetricsView`), which is consistent with the existing feature-local style rather than the `types/models` layout. |
| FE-11 | Service layer (project pattern) | PASS | No `BaseService`/`services/api/` exists in this repo. New/changed services follow the established direct-client wrapper pattern: policiesApi.ts, metrics/api.ts, objects/api.ts additions, lifecycle/api.ts, users/api.ts all `import { api } from "@/lib/api/client"` and export typed functions. |
| FE-12 | Query keys `as const` | PASS | keys.ts:21 `objectsKeys.versions`, :42-46 `policiesKeys.{all,list,detail}`, :56 `metricsKeys.view` all end `as const`. Centralized in `lib/api/keys.ts` (project's key-factory home). Hooks consume them: PoliciesPage.tsx:57, PolicyEditorDialog.tsx:65, ObjectVersionsSheet.tsx:94, MetricsPage.tsx:365. |
| FE-13 | Forms use react-hook-form + zodResolver | PASS | PolicyEditorDialog.tsx:57-61 and CreateRuleDialog.tsx:130-134 both `useForm({ resolver: zodResolver(...) })` with shadcn `<Form>` field rendering. (EditCustomPoliciesDialog uses a controlled `Set` + native checkboxes — acceptable for a non-validated multi-select toggle; no Zod needed.) |
| FE-14 | Schema paired with inferred type | PASS (with FE-04 caveat) | Both inline schemas pair with `z.infer`: PolicyEditorDialog.tsx:35 `type FormValues = z.infer<typeof editorSchema>`; CreateRuleDialog.tsx:77 `type FormValues = z.infer<typeof ruleSchema>`. Location (component vs `lib/schemas/`) is the FE-04 WARN, not a type-inference miss. |
| — | Infinite query w/ opaque next_token | PASS | ObjectVersionsSheet.tsx:93-103 `useInfiniteQuery` with `initialPageParam: ""`, `getNextPageParam` returning `meta.page.next_token` (opaque) or `undefined` to stop; "Load more" gated on `hasNextPage` (:302). |
| — | refetchInterval (live metrics) | PASS | MetricsPage.tsx:367 `refetchInterval: 30_000`. |
| — | Invalidation correctness | PASS | Mutations invalidate the right keys: PolicyEditorDialog.tsx:95-96 (list + detail), PoliciesPage.tsx:64 (list), ObjectVersionsSheet.tsx:123-128 (versions + parent object list), EditCustomPoliciesDialog.tsx:53-54 (user detail + list), UserDetailPage.tsx:70-71. |
| — | `policy_in_use` details/meta path | PASS | errors.ts:41 maps `errors[0].meta` → `AppError.details`; consumed at PoliciesPage.tsx:70-79 (`attached_to.users/groups`) and rendered in the delete dialog (:223-253) with the delete button disabled while in-use (:266). Covered by errors.test.ts:37 (meta→details), :70 (absent meta → undefined). |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | All new clickable surfaces are `<Button>` (CVA supplies `cursor-pointer`) or `<a>`: VirtualizedObjectList.tsx:184 version trigger is a `<Button>`; ObjectVersionsSheet download is `<Button asChild><a>` (:257-271); PoliciesPage row actions are `<Button>` (:159-173); UserDetailPage edit triggers are `<Button>`. The native `<input type="checkbox">` toggles (EditCustomPoliciesDialog.tsx:113, UserDetailPage.tsx:107) get a browser pointer by default. No bare clickable `<div>`/non-button `render`-prop triggers introduced. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | New/changed components have real-behavior tests: PoliciesPage.test.tsx (editable-gating, policy_in_use flow), PolicyEditorDialog.test.tsx (invalid-JSON guard, create payload), CreateRuleDialog.test.tsx (7 tests across all 3 union branches + 422 pointer mapping), ObjectVersionsSheet.test.tsx (5 tests: badges, restore/delete/undelete), EditCustomPoliciesDialog.test.tsx (custom-only filtering, toggle/save), MetricsPage.test.tsx (paused banner vs charts). Assertions query by role/text/label, not implementation. |
| FE-17 | Mocks updated when services changed | PASS | errors.test.ts:37-105 exercises the new `meta`→`details` and `error.details` branches added to errors.ts. test/setup.ts:58-66 adds the `ResizeObserver` shim required by Recharts' `ResponsiveContainer` so MetricsPage tests run. Feature tests mock their `api.ts` wrappers (`vi.mock`), so no separate `__mocks__/` dir to drift. |

## Summary

### Blocking (must fix)
- None. Build, tests (93/93), lint (0 errors), and format are all clean; no FE check is an outright FAIL.

### Non-Blocking (should fix)
- **FE-04** — Inline Zod schemas in `PolicyEditorDialog.tsx:30` and `CreateRuleDialog.tsx:42`. Consider extracting to `src/lib/schemas/` for consistency with `auth.ts`/`connection.ts`/`setup.ts`. Currently matches the pre-existing feature-local convention, so low priority; if extracting, the CreateRuleDialog discriminated union is the higher-value candidate.
- **FE-06** — Hardcoded palette colors for status accents in `CreateRuleDialog.tsx:219`, `LifecycleRulesTab.tsx:40-44`, `ObjectVersionsSheet.tsx:179-180,247`. Matches existing `main` usage (BucketDetailPage). If a "warning"/"info" semantic token set is ever added to the theme, migrate these; not theme-breaking today because each palette class ships an explicit `dark:` variant.

### Notable strengths
- Discriminated-union form (CreateRuleDialog) with per-branch validation, JSON:API pointer→field error mapping, and exhaustive branch tests.
- `policy_in_use` end-to-end: backend `meta` → `AppError.details` → in-dialog attached-users/groups display with delete disabled, plus unit coverage.
- Infinite version history with opaque `next_token` pagination and correct cross-query invalidation (versions + parent listing).
