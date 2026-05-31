# PRD: UI Session Factory Client

---
author: Codex
last modified: 2026-05-30
status: draft
---

## Introduction

Dashboard factory editing, saving, and PNG import today use overlapping API surfaces: `current-factory-definition` (session GET/PUT without save modes), `named-factory` (`createFactory` → `POST /factories`), and `factory-definition` (normalization only). Session-scoped import already PUTs to `/factory-sessions/{session_id}/factory` for replace-in-place, but create/activate semantics still depend on the legacy POST path in some flows.

Backend [session factory save modes](../prd-session-factory-save-modes.md) (S1) unifies submission behind one session PUT with `{ mode, factory }` and removes `POST /factories`. This PRD delivers the **UI transport and operator flows** that match that contract: one GET/PUT client, editor saves that only replace the current factory, and an import confirm dialog that chooses replace-current vs create-new-named.

**Intent:** Operators and maintainers get predictable, session-scoped factory I/O with no duplicate HTTP paths; implementers get one module to own transport, error mapping, and save-mode selection.

## Context

### Customer ask

One GET/PUT session factory client with `mode` + `factory`; import confirm dialog; remove `POST /factories` from dashboard paths. Complete backend S1 (`prd-session-factory-save-modes`) before UI implementation.

### Problem

- Factory definition writes are split across modules with different body shapes and error types.
- Import cannot offer “create new named factory” without `POST /factories`.
- Save-mode semantics (`REPLACE_CURRENT` vs `UPSERT_NAMED_AND_ACTIVATE`) are not represented in the UI client, so the dashboard cannot align with the unified backend contract.
- Tests and Storybook mocks may still assert `POST /factories`, hiding regressions.

### Solution

Introduce a dedicated session factory API client, migrate existing get/save call sites through thin adapters, wire import confirm UX to the two save modes, delete `createFactory`, and centralize error mapping—while keeping canonical shape normalization in `factory-definition`.

## Project-level acceptance criteria

- [ ] All dashboard factory definition writes use `GET` / `PUT` on `/factory-sessions/{session_id}/factory` only (no `POST /factories`).
- [ ] PUT bodies use `{ mode?: "REPLACE_CURRENT" | "UPSERT_NAMED_AND_ACTIVATE", factory: Factory }` with `version` inside `factory` when required by mode.
- [ ] Factory editor Save replaces the current factory in the active tab (`REPLACE_CURRENT` or omitted mode); no editor path uses `UPSERT_NAMED_AND_ACTIVATE`.
- [ ] Import confirm dialog on every session tab offers **Replace current factory** (default) and **Create new named factory**, with correct mode, name, and version rules per choice.
- [ ] `selectedSessionID` from the dashboard session store reaches save, import, and export-equivalent factory I/O for the active tab.
- [ ] Canonical factory normalization remains in `factory-definition`; session transport does not reshape topology.
- [ ] Quality gate: UI typecheck, lint, and affected unit/integration tests pass.

## Goals

- Single client for session factory GET and PUT with explicit save `mode` and `factory` payload.
- Editor save uses `REPLACE_CURRENT` only (or omitted default mode).
- Import confirm dialog: replace current vs create new named; suffixed name (`name-2`, `name-3`, …) on conflict for create path.
- Remove `createFactory` and all dashboard `POST /factories` usage.
- Keep `factory-definition` as pure normalization; map API errors in one place for save and import.

## User Stories

### ui-session-factory-client-001: Session factory transport client

**Description:** As a frontend maintainer, I want one module that performs session factory GET/PUT against the regenerated OpenAPI contract so all features share one HTTP shape.

**Acceptance Criteria:**

- [ ] After S1 lands, regenerate `ui/src/api/generated/openapi.ts` so PUT accepts `mode` and `factory` per backend contract.
- [ ] New module under `ui/src/api/session-factory/` exports `getSessionFactory(sessionID)` and `saveSessionFactory({ sessionID, mode?, factory })`.
- [ ] PUT request body is `{ mode?: "REPLACE_CURRENT" | "UPSERT_NAMED_AND_ACTIVATE", factory: Factory }`; `factory.version` is set on the wire when callers supply it.
- [ ] GET and PUT target `currentFactorySessionPath(sessionID)` from `session-routing.ts` and use shared `transport.ts` helpers.
- [ ] Unit tests assert HTTP method, path, and JSON body for default (`~default`) and a non-default session ID for GET and for PUT with each save mode.
- [ ] Typecheck passes.
- [ ] Tests pass.

### ui-session-factory-client-002: Current factory document delegates to session client

**Description:** As a feature developer, I want existing `getCurrentFactoryDocument` / `saveCurrentFactoryDocument` to call the session factory client so hooks do not duplicate fetch logic.

**Acceptance Criteria:**

- [ ] `getCurrentFactoryDocument` and `saveCurrentFactoryDocument` delegate to `getSessionFactory` / `saveSessionFactory` (or remain thin wrappers with identical observable behavior).
- [ ] `SaveCurrentFactoryInput.baseVersion` continues to map to `factory.version` on the PUT body only; callers do not pass a parallel top-level version field.
- [ ] React Query keys and hook signatures in `useCurrentFactoryDefinition` / `useSaveCurrentFactory` are unchanged unless a breaking change is documented in the PR.
- [ ] Existing unit tests for current-factory-definition still pass or are updated to mock the session client without changing operator-visible behavior.
- [ ] Typecheck passes.
- [ ] Tests pass.

### ui-session-factory-client-003: Editor save always replaces current factory

**Description:** As a factory editor, I want Save to update the factory I am editing in the active session without switching named factories or save modes.

**Acceptance Criteria:**

- [ ] `useSaveCurrentFactory` (and any direct `saveCurrentFactoryDocument` call from editor paths) sends `mode: "REPLACE_CURRENT"` or omits `mode`.
- [ ] No graph editor, activity card, worker, or workstation save path sends `UPSERT_NAMED_AND_ACTIVATE`.
- [ ] Stale version (`STALE_FACTORY_VERSION`) and not-idle (`FACTORY_NOT_IDLE`) errors still surface in the same UI states as before migration.
- [ ] Save on a non-default session tab PUTs to that session’s factory endpoint when `selectedSessionID` is set.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill: edit and save on default and second session tabs; confirm network PUT uses session path and replace mode only.

### ui-session-factory-client-004: Import confirm dialog with replace vs create choice

**Description:** As a dashboard operator, I want to choose on import confirm whether to replace the current factory or create a new named factory, with accessible defaults and loading/error states.

**Acceptance Criteria:**

- [ ] Import preview confirm dialog appears on all session tabs and defaults to **Replace current factory**.
- [ ] Operator can select **Create new named factory** before confirming; choice is keyboard-accessible and announced to assistive tech (radio group or equivalent).
- [ ] Dialog shows the resolved factory name for the create path once derived (including numeric suffix when applicable).
- [ ] Confirm button shows submitting state during activation; errors render in the existing error panel pattern.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill: open import preview, switch choices, confirm disabled/submitting behavior matches selection.

### ui-session-factory-client-005: Import activation uses session PUT save modes

**Description:** As a dashboard operator, I want import confirm to persist via the correct save mode so replace keeps my current name and create adds or upserts a new named factory in this session.

**Acceptance Criteria:**

- [ ] **Replace current:** GET current factory for `sessionID` → set imported payload `factory.name` to current session name (ignore PNG embedded name) → PUT `REPLACE_CURRENT` with `factory.version` from GET.
- [ ] **Create new named:** derive writable `factory.name` from PNG metadata → if name exists under session, allocate first free suffixed name (`base`, `base-2`, `base-3`, …) using client-side factory name list when available, otherwise documented probe strategy → PUT `UPSERT_NAMED_AND_ACTIVATE`; omit `factory.version` when creating a new name, include version when upserting an existing name the client pre-read.
- [ ] `useFactoryImportActivation` uses session factory client only; no `POST /factories`.
- [ ] `App.import.test.tsx` and `factory-import-second-session.integration.test.mjs` assert correct PUT mode and body per dialog choice and session.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill: replace on default tab; create-new-named on second tab; no `POST /factories` in network log.

### ui-session-factory-client-006: Remove legacy named-factory create path

**Description:** As a maintainer, I want dead `POST /factories` client surface removed so the dashboard cannot regress to global create.

**Acceptance Criteria:**

- [ ] `createFactory` is removed from `ui/src/api/named-factory/api.ts` (or deleted module after call-site migration).
- [ ] No remaining dashboard import or save code references `POST /factories`.
- [ ] `openapi.named-factory.test.ts` (or successor contract tests) expect session PUT only for former create flows.
- [ ] Storybook `fetchMocks` for current-factory and import stories use session factory paths and modes.
- [ ] Typecheck passes.
- [ ] Tests pass.

### ui-session-factory-client-007: Unified session factory error mapping

**Description:** As a dashboard operator, I want consistent, actionable error messages in save and import dialogs when the session factory API rejects a request.

**Acceptance Criteria:**

- [ ] Session factory client maps API codes (`STALE_FACTORY_VERSION`, `FACTORY_NOT_IDLE`, `INVALID_FACTORY`, `INVALID_FACTORY_NAME`, etc.) to a single error type consumed by save and import features (e.g. `SessionFactoryAPIError`), or thin adapters preserve existing `CurrentFactoryDefinitionError` / `NamedFactoryAPIError` shapes without duplicate switch logic in hooks.
- [ ] Import preview shows stale-version guidance when the API returns `STALE_FACTORY_VERSION` on replace or upsert paths.
- [ ] Network and 5xx failures show the same recoverable copy patterns as today.
- [ ] Unit tests cover at least stale, not-idle, and invalid-factory mappings from mocked HTTP responses.
- [ ] Typecheck passes.
- [ ] Tests pass.

## Functional Requirements

- FR-1: All dashboard factory definition writes use `PUT /factory-sessions/{session_id}/factory` only.
- FR-2: Request body includes optional `mode` (default `REPLACE_CURRENT`) and `factory` conforming to OpenAPI `Factory`.
- FR-3: `selectedSessionID` from `dashboardSessionStore` must reach save and import activation (same threading as export).
- FR-4: Editor saves use `REPLACE_CURRENT` only; `factory.name` must match session current name from last GET.
- FR-5: Import replace path preserves session current name and requires version from GET.
- FR-6: Import create path uses `UPSERT_NAMED_AND_ACTIVATE` with PNG-derived name and client-side suffix allocation on conflict.
- FR-7: Normalization of canonical factory shape stays in `factory-definition/api.ts`.
- FR-8: Generated OpenAPI types are regenerated when S1 changes the save request schema.

## Non-Goals

- PNG parsing, thumbnail preview, or import drag-drop UX redesign (only confirm-dialog actions and activation).
- CLI or backend handler changes (covered by S1).
- New HTTP endpoints beyond S1.
- Renaming all hooks under `features/current-factory-definition` (optional follow-up in U3/U7 PRDs).
- `SessionScope` context extraction (separate `prd-ui-session-scope-context`).

## High-level technical design

```text
┌─────────────────────────────────────────────────────────────┐
│  Features: editor save, import, selection saves             │
└───────────────────────────┬─────────────────────────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
  useSaveCurrentFactory   import activation   (future U3 hook)
         │                  │
         ▼                  ▼
  current-factory-definition (thin adapters, stable hook API)
         │
         ▼
  session-factory/client  ──GET/PUT──►  /factory-sessions/{id}/factory
         │                                    { mode, factory }
         ▼
  factory-definition (normalizeFactoryDefinition only)
```

**Layers:**

| Layer | Responsibility |
|-------|----------------|
| `session-factory` | HTTP transport, save mode on PUT, response parse, error code mapping |
| `current-factory-definition` | `CurrentFactoryDocument` type, `baseVersion` adapter, React Query hooks |
| `factory-definition` | Canonical topology normalization (no I/O) |
| Import feature | Dialog UX + mode/name/version orchestration before calling save |

**Prerequisite:** Do not merge UI transport typed against production until S1 is merged and `api/openapi.yaml` + `ui` generated client include save `mode`. Development may use a short-lived branch that tracks S1 OpenAPI.

**Suffix allocation:** Prefer listing named factories under the session when an HTTP list exists; otherwise iterate suffixed names with PUT validation or documented client-side name set from session metadata—document chosen approach in story 005 implementation notes.

## Supporting technical and UX considerations

- **Loading / empty / error / success:** Import dialog must show submitting on confirm, preserve preview while idle, and surface API errors without dismissing preview until operator cancels.
- **Accessibility:** Replace vs create controls use a single selectable group; confirm/cancel remain in dialog footer with focus trap.
- **Responsive:** Dialog layout unchanged from current import preview; new controls fit existing two-column preview grid.
- **Session isolation:** Non-default tab flows must be covered by integration test already in repo; extend for create-new-named mode.
- **Coordination:** [`prd-ui-api-module-cleanup.md`](../prd-ui-api-module-cleanup.md) may delete `named-factory` after this client lands; [`prd-ui-factory-document-save-hook.md`](../prd-ui-factory-document-save-hook.md) consumes the same transport later.

## Dependencies

| Upstream | Relationship |
|----------|----------------|
| `prd-session-factory-save-modes` (S1) | **Hard block:** OpenAPI `mode` + `factory` body and removal of `POST /factories` |

| Downstream | Relationship |
|------------|----------------|
| `prd-ui-factory-document-save-hook` | Consumes session client via current-factory adapters |
| `prd-ui-api-module-cleanup` | Removes `named-factory` after migration |

## Success metrics

- Zero `POST /factories` requests in UI test network assertions for save and import flows.
- One session-factory module imported by both save and import activation paths.
- Import on non-default session tab integration test green for both replace and create modes.
- Operators can complete replace import in one confirm with default selection unchanged from today’s mental model.

## Open Questions

None blocking—suffix allocation fallback is an implementation detail documented in story 005 if no list endpoint exists at S1 merge time.

## Related documents

- [`dependence-graph-for-ui-prds.md`](../dependence-graph-for-ui-prds.md)
- [`prd-session-factory-save-modes.md`](../prd-session-factory-save-modes.md)
- [`prd-ui-api-module-cleanup.md`](../prd-ui-api-module-cleanup.md)
- [`prd-ui-factory-document-save-hook.md`](../prd-ui-factory-document-save-hook.md)
