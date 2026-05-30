# PRD: Session-Aware Factory PNG Import

---
author: Codex
last modified: 2026, may, 30
status: draft
---

## Context

### Customer Ask

Customers want to import factories from a PNG on **any** factory workspace tab. Today import only behaves correctly on the first/default tab because activation submits to `POST /factories`, which creates and activates a named factory under the **service root** instead of updating the factory owned by the **currently selected live session**.

On a second (or later) tab, import still targets the default factory's directory, source directory, and runtime. The import button and drop flow must become **session-context aware**: derive activation context from the active factory session and submit to `PUT /factory-sessions/{session_id}/factory` so the running factory for that session is replaced (including portable bundled files) without mutating unrelated sessions.

Add functional tests that prove the request payload and endpoint are correct when importing from a non-default tab, and add integration tests that exercise the multi-tab import path end to end.

### Problem

The dashboard already tracks `selectedSessionID` in `useDashboardSessionStore` and uses it for export, saves, and work submission. Factory PNG import activation is wired through `useFactoryImportActivation`, which defaults to `createFactory` in `ui/src/api/named-factory/api.ts` and always `POST`s `/factories`.

That global named-factory activation path:

- Switches the durable current-factory pointer and materializes a new subdirectory under the service root.
- Ignores which workspace tab (live factory session) the operator had selected.
- Leaves other live sessions unchanged while appearing to "import" on the wrong tab from the operator's perspective.

Operators running multiple factory sessions (for example default `~default` plus an opened `session-beta` tab) expect import on the active tab to update **that** session's factory directory and runtime only.

### Solution

1. **Frontend activation seam:** Introduce a session-scoped import activation function that reads the current factory document for the selected session (`GET /factory-sessions/{session_id}/factory`), adapts the imported canonical `Factory` payload to the session's current factory name and version rules, and saves via `PUT /factory-sessions/{session_id}/factory` (reusing the existing current-factory-definition transport and error mapping where practical).
2. **Session wiring:** Thread `selectedSessionID` from the dashboard shell through `useCurrentActivityImportController` / `useFactoryImportActivation` the same way export already threads session into `getCurrentFactory`.
3. **Verification:** Add focused UI unit tests for endpoint, method, and payload per session; update the default-tab app-shell import test to expect the session route; add a backend functional test that imports on a non-default session and proves isolation from the default session's on-disk factory; add a browser integration test that selects a second tab and confirms import hits the session-scoped route.

## Project Acceptance Criteria

- [ ] Confirming a factory PNG import on the **default** session tab activates via `PUT /factory-sessions/~default/factory` (not `POST /factories`) using the imported canonical payload and the session's current factory version metadata.
- [ ] Confirming import on a **non-default** session tab activates via `PUT /factory-sessions/{that-session-id}/factory` and does not call `POST /factories`.
- [ ] After import on a non-default tab, the targeted session's factory definition and on-disk factory directory reflect the imported content; the default session's factory remains unchanged.
- [ ] Import preview, submitting, error, and success states continue to work with accessible dialog semantics and existing activation error codes where applicable.
- [ ] Unit tests prove the activation client sends the correct method, path, and body for default and non-default selected sessions.
- [ ] Backend functional and UI integration tests exercise the second-tab import path with direct behavioral evidence.
- [ ] Quality gate: backend and frontend typecheck, lint, and relevant tests pass before merge.

## Goals

- [ ] Make factory PNG import respect the operator's active factory session tab.
- [ ] Align import activation with the existing session-scoped current-factory API used by export and editable saves.
- [ ] Prevent cross-session mutation when multiple live factory sessions are open.
- [ ] Preserve the canonical imported `Factory` payload shape from PNG metadata without dashboard-side reshaping beyond required session save rules (name preservation, version echo/increment).
- [ ] Add regression tests at the API client, app-shell, service, and browser integration layers.

## User Stories

### US-001: Session-scoped factory import activation client

**Description:** As a dashboard operator, I want PNG import activation to save through my active session's current-factory endpoint so import updates the factory I am viewing.

**Acceptance Criteria:**
- [ ] A new session-aware activation function (or extension of the import activation seam) reads the current factory document for a supplied `sessionID`, sets the imported payload's `name` to the session's current factory name, includes incremented `version` metadata consistent with `saveCurrentFactoryDocument`, and `PUT`s to `/factory-sessions/{session_id}/factory`.
- [ ] Activation for `~default` uses `/factory-sessions/~default/factory`; activation for `session-beta` uses `/factory-sessions/session-beta/factory`.
- [ ] Activation does not call `POST /factories` for session-scoped import.
- [ ] Structured API failures (`FACTORY_NOT_IDLE`, `STALE_FACTORY_VERSION`, `INVALID_FACTORY`, etc.) surface through the existing import activation error state.
- [ ] API client unit tests assert method, path, and JSON body for default and non-default session IDs.
- [ ] Typecheck passes
- [ ] Tests pass

### US-002: Wire selected session into dashboard import controller

**Description:** As a dashboard operator, I want the import drop zone and preview confirm action on whichever tab is selected to activate against that tab's session.

**Acceptance Criteria:**
- [ ] `useCurrentActivityImportController` (and `DashboardBento`) pass `selectedSessionID` from `useDashboardSessionStore` into import activation, mirroring `useCurrentActivityExport` / `getCurrentFactory({ sessionID })` session threading.
- [ ] Switching the selected session tab before confirming import activates against the newly selected session without requiring a page reload.
- [ ] `useFactoryImportActivation` uses the session-scoped activation function by default instead of `createFactory`.
- [ ] Hook-level tests cover session ID propagation into the activation dependency.
- [ ] Typecheck passes
- [ ] Tests pass

### US-003: Update default-tab app-shell import test for session PUT

**Description:** As a maintainer, I need the primary dashboard import test to reflect the corrected default-session activation route so regressions back to `POST /factories` are caught.

**Acceptance Criteria:**
- [ ] `App.import.test.tsx` (and any other app-shell import tests that assert `/factories`) expect `PUT /factory-sessions/~default/factory` with the imported canonical factory payload and version field after a current-factory read (mocked or spied as needed).
- [ ] The test still covers preview dialog confirm, submitting state, and successful activation refresh behavior.
- [ ] Typecheck passes
- [ ] Tests pass

### US-004: Backend functional test for non-default session import isolation

**Description:** As a maintainer, I need proof that session-scoped import replacement updates only the targeted session's factory directory and runtime.

**Acceptance Criteria:**
- [ ] A functional test under `tests/functional/runtime_api/factory_transformation/` (or `tests/functional/bootstrap_portability/` if better aligned) opens a second live factory session, `PUT`s an imported-style factory payload to `/factory-sessions/{session_id}/factory`, and asserts the saved definition is returned from that session's `GET`.
- [ ] The default session's current factory name, work types, and on-disk factory directory remain unchanged after the non-default session import.
- [ ] When the payload includes portable `supportingFiles.bundledFiles` with inline content, materialized files are written under the **target session's** factory directory, not the default session root.
- [ ] Typecheck passes
- [ ] Tests pass

### US-005: Browser integration test for import on a second session tab

**Description:** As a dashboard operator, I want confidence that selecting a second factory tab and confirming import exercises the session-scoped network path in the real UI shell.

**Acceptance Criteria:**
- [ ] A new or extended test under `ui/integration/` starts the dashboard with at least two session tabs, selects the non-default tab, completes the import preview confirm flow (using harness mocks for PNG read and API), and asserts the activation request targets `/factory-sessions/{non-default-session-id}/factory` with `PUT`.
- [ ] The test asserts no `POST /factories` request occurs during that flow.
- [ ] Loading, preview, and post-confirm success or refresh behavior remain observable (no silent failure).
- [ ] Typecheck passes
- [ ] Verify in browser using dev-browser skill

## Functional Requirements

- FR-1: Factory PNG import activation must use the active dashboard `selectedSessionID` (defaulting to `~default` when unset) to choose the session-scoped factory endpoint.
- FR-2: Activation must `GET` the current factory for that session before `PUT` so version metadata and factory name constraints are satisfied.
- FR-3: The `PUT` body must be the imported canonical `Factory` payload with `name` matching the session's current factory and `version` advanced per existing save rules.
- FR-4: `POST /factories` must not be used for dashboard PNG import activation after this change.
- FR-5: Import preview, drag/drop, and error presentation remain unchanged except for activation targeting and any new stale-version messaging mapped from current-factory save errors.
- FR-6: Non-default session import must not change another session's factory definition, pointer, or on-disk layout.
- FR-7: Tests must validate endpoint, payload, session isolation, and the second-tab UI path without meta-inventory checks.

## Non-Goals

- Changing PNG metadata parsing, schema version handling, or preview rendering.
- Replacing `POST /factories` for non-dashboard clients or CLI flows that intentionally create new named factories.
- Reworking factory session tab open/close UX or multi-session layout.
- Allowing import to rename the active session's factory to the embedded PNG name when the session save contract requires name preservation (operators keep the session's current factory name; imported topology replaces content in place).
- Broad refactors of `named-factory` API ownership beyond what is required for session-scoped import activation.
- Migrating historical factories on disk.

## High-Level Technical Design

**UI import seam (`ui/src/features/import`, `ui/src/features/workflow-activity/hooks/current-activity-import-controller.ts`, `ui/src/features/bento/components/dashboard-bento.tsx`):**

- Add `activateImportedFactoryForSession(sessionID, importedFactory)` alongside or instead of default `createFactory`, built on `getCurrentFactoryDocument` + `saveCurrentFactoryDocument` from `ui/src/api/current-factory-definition/api.ts` and `currentFactorySessionPath` from `ui/src/api/session-routing.ts`.
- Map `CurrentFactoryDefinitionError` codes into `NamedFactoryAPIError` (or extend activation error typing) so the preview dialog keeps working.
- Pass `useDashboardSessionStore((s) => s.selectedSessionID)` into `useCurrentActivityImportController` the same way `use-current-factory-export.ts` does.

**Payload adaptation:**

- After reading the session's current factory document, set `factory.name` to `current.name` before save so `prepareEditableFactoryDefinitionSave` name-preservation rules succeed.
- Echo/increment `version` using the same helper path as editable workstation saves.

**Backend (verification only unless gaps found):**

- `SaveCurrentFactoryForSession` in `pkg/service/runtime_sessions.go` already replaces only the targeted session runtime and persists via `replaceEditableFactoryDefinition` / `ReplaceNamedFactoryWithReport`, which materializes portable bundled files.
- Functional tests should use existing helpers such as `openNamedFactorySession`, `getCurrentFactoryForSession`, and `saveCurrentFactoryForSession` from `api_current_factory_put_test.go`.

**Integration testing:**

- Extend `ui/integration/browser-test-harness.mjs` request recording if needed to assert PUT paths per session, following `dashboard-session-tabs.integration.test.mjs` patterns.

## Supporting Technical and UX Considerations

- Normalize null/empty `selectedSessionID` to `~default` via `DEFAULT_FACTORY_SESSION_ID` for routing consistency.
- If the runtime is not idle, preserve existing `FACTORY_NOT_IDLE` operator messaging.
- On `STALE_FACTORY_VERSION`, surface a recoverable error (refresh current factory, retry import) using existing dashboard error patterns.
- After successful activation, keep `onFactoryActivated` / `incrementRefreshToken` behavior so the graph reloads for the active session.
- Reuse existing import preview copy; only activation targeting changes.

## Success Metrics

- Operators can import on any open factory tab and see the graph refresh for that tab's factory without affecting other tabs.
- Zero errant `POST /factories` calls from dashboard PNG import in test harnesses.
- CI functional and integration tests cover default and second-session import paths.

## Open Questions

None. Session-scoped `PUT` replacement, name preservation, and verification expectations are specified by the customer ask and existing current-factory save contract.
