# PRD: UI Factory Document vs Snapshot Data Planes

## Introduction

The dashboard maintains two overlapping sources of factory-related UI state:

1. **Factory document plane** — session-scoped `GET/PUT /factory-sessions/{session_id}/factory` via React Query (`useCurrentFactoryDocument`, `useSaveCurrentFactory`). This is the canonical definition for edit, save, and export.
2. **Dashboard snapshot plane** — SSE events → timeline store → world view → `DashboardSnapshot` (`useDashboardSnapshot`). This is the live runtime projection at the selected timeline tick (topology overlay, in-flight counts, timeline selection).

Today these planes are mixed in the graph editor and related surfaces: code sometimes treats `snapshot.factory` as structure for edits or saves when the loaded document differs, which mirrors backend confusion between “live world view” and “editable definition” and causes subtle bugs (stale saves, wrong export payloads, cross-session flashes).

**Intent:** Make responsibilities explicit and enforce observable behavior so edit/save/export always use the document plane while observe/runtime overlays use the snapshot plane only where appropriate.

**Contributor guide:** [`docs/internal/development/development.md`](../docs/internal/development/development.md#factory-document-vs-dashboard-snapshot) (U4 in UI Wave 3).

## Context

### Customer ask

Separate **editable factory document** responsibilities from **event snapshot** responsibilities across the dashboard so operators edit and save what the server considers authoritative, while live runtime overlays remain driven by streamed events.

### Problem

- Graph editor and activity card paths can prefer `DashboardSnapshot.factory` when document and snapshot diverge.
- Session tab switches can briefly show another session’s factory or retain a dirty draft keyed to the wrong scope.
- After save, document cache and timeline snapshot can drift without a documented convergence contract.
- Export or save payloads built only from snapshot topology can ship the wrong factory to disk or the server.

### Solution

Document the two planes for contributors, then align dashboard features so:

- **Document plane** owns structure for edit, save, export, draft baseline, and `baseVersion`.
- **Snapshot plane** owns timeline, selected-tick world view, runtime counts, and read-only observe overlays.
- **Combined guards** use document `version` plus snapshot `in_flight_dispatch_count` (not snapshot factory version alone) for stale-save and idle blocking.

## Project-level acceptance criteria

- [ ] Contributors can read a single dev-guide section that maps each UI concern (edit, save, graph structure, export, runtime counts, timeline, stale warnings) to the authoritative plane.
- [ ] Graph editor edit/save paths build structure and PUT payloads from the loaded factory document; snapshot factory is not the sole structural source when a document is available.
- [ ] Switching dashboard session tabs does not show the previous session’s factory definition or retain that session’s dirty graph draft in the new session.
- [ ] After a successful factory save, the document cache updates immediately and the live snapshot converges via the existing event stream without redundant document GET storms.
- [ ] Factory PNG export uses the session-scoped factory document GET, not snapshot-only topology.
- [ ] Observe-mode activity graph continues to reflect the selected-tick snapshot; edit-mode graph reflects the document (including pending draft), even when the two planes diverge in tests.
- [ ] Quality gate: UI typecheck, lint, and affected unit/integration tests pass before merge.

## Goals

- **Authoritative for save/edit/export:** session factory document (React Query).
- **Authoritative for live runtime overlay:** event snapshot / world view at selected tick.
- Graph editor and activity card **project** from document + optional snapshot overlay (in-flight work, live counts).
- Explicit query invalidation and draft scope rules on session switch and after save.
- Align with website standards: React Flow graph state is a projection, not source of truth.

## User stories

### ui-factory-document-snapshot-planes-001: Publish factory document vs snapshot planes guide

**Description:** As a contributor, I want written rules and a data-flow diagram for the two planes so I know which hook to use for each dashboard concern.

**Acceptance Criteria:**

- [ ] `docs/internal/development/development.md` includes a **Factory document vs dashboard snapshot** section with a concern→plane table covering edit, save, graph structure, export, runtime counts, timeline selection, and stale-save warnings.
- [ ] The section includes a mermaid or ASCII diagram showing React Query document flow vs SSE/timeline snapshot flow.
- [ ] The section states the save rule: do not build save payloads from `DashboardSnapshot.factory` alone when a document is loaded.
- [ ] Typecheck passes

### ui-factory-document-snapshot-planes-002: Graph editor uses document for structure and save

**Description:** As a graph editor user, I want the editable graph and save payload to reflect the loaded factory document, using snapshot data only for runtime guards—not for topology when the document differs.

**Acceptance Criteria:**

- [ ] `useEditableFactoryGraph` bases `baseDocument`, `latestDocument`, and pending graph structure on `useCurrentFactoryDocument` (or injected document), not on `snapshot.factory` alone.
- [ ] Save calls send `baseVersion` from the document and a factory definition derived from the document plane; divergent snapshot-only workstations never appear in the saved payload when only the snapshot differs.
- [ ] Save is blocked when snapshot runtime reports in-flight dispatches (`activeWorkCount` / `in_flight_dispatch_count`); stale-draft detection uses document version, not snapshot factory version alone.
- [ ] Unit tests use fixtures where document and snapshot topology intentionally diverge and assert document authority for projection and save.
- [ ] Typecheck passes
- [ ] Tests pass

### ui-factory-document-snapshot-planes-003: Activity card observe vs edit plane behavior

**Description:** As a dashboard operator, I want the current-activity graph to show live snapshot topology in observe mode and the factory document (plus my draft) in edit mode so I do not edit stale snapshot structure.

**Acceptance Criteria:**

- [ ] In **observe mode**, the activity card graph layout uses the dashboard snapshot at the selected tick (existing live behavior preserved).
- [ ] In **edit mode**, graph layout and editor interactions use document-plane definitions (`pendingFactoryDefinition` → `latestDocument` → `baseDocument`); snapshot-only workstations absent from the document do not appear as editable nodes.
- [ ] Classifier/unsupported-workstation detection in edit mode evaluates the document definition, not snapshot-only topology.
- [ ] Integration or hook tests cover divergent document vs snapshot fixtures for both modes.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

### ui-factory-document-snapshot-planes-004: Session tab switch isolates document cache and graph draft

**Description:** As an operator switching workspace tabs, I want the factory document query and graph draft to reset for the new session so I never edit another session’s factory.

**Acceptance Criteria:**

- [ ] On `sessionID` change, dashboard lifecycle resets timeline and selection (existing behavior) and clears or refetches session-scoped factory document queries per documented policy (`removeQueries` or equivalent).
- [ ] `useEditableFactoryGraph` / `useFactoryGraphDraftState` reset dirty draft when `factoryDocumentScopeKey` (normalized session id) changes so a prior tab’s pending edits cannot seed the new session.
- [ ] After switching from session A to session B, the graph editor does not render session A’s factory name or workstations while session B’s document GET is loading (loading/empty state instead of stale flash).
- [ ] Session lifecycle tests assert factory-definition query keys are cleared once per session switch.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

### ui-factory-document-snapshot-planes-005: After-save document and snapshot convergence

**Description:** As a user saving factory edits, I want the saved definition and live dashboard to converge without manual reload or redundant network churn.

**Acceptance Criteria:**

- [ ] Successful save writes the PUT response (including `version`) into the document React Query cache without an automatic document GET refetch on success.
- [ ] Graph draft clears pending edits after save and re-syncs `latestDocument` from the updated cache on the next render.
- [ ] Live snapshot updates when the session event stream processes `FACTORY_CHANGE` (or equivalent); observe overlays and runtime counts refresh without requiring `refreshToken` increment on each save.
- [ ] `syncCurrentFactoryDefinition` updates or invalidates the document cache from streamed events per documented rules (set when version metadata present; single invalidate when absent)—no double-fetch storm on save success.
- [ ] Dev guide **After-save convergence** subsection documents the immediate vs event-driven steps.
- [ ] Typecheck passes
- [ ] Tests pass

### ui-factory-document-snapshot-planes-006: Export uses session factory document

**Description:** As an operator exporting the factory, I want the PNG/metadata payload to match the session factory document I would save, not a divergent snapshot projection.

**Acceptance Criteria:**

- [ ] `useCurrentFactoryExport` loads export payload via session-scoped factory document GET for `useDashboardSession().sessionID`; export stays disabled when `rawSessionID` is null.
- [ ] When document and snapshot topology diverge in tests, exported factory definition matches the document plane (including `version` metadata rules for hybrid export).
- [ ] Export error and loading states remain explicit (unavailable, load failed, preparing).
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

## High-level technical design

### Two planes (canonical vs projection)

| Plane | Storage | Primary hooks | Authoritative for |
| --- | --- | --- | --- |
| Factory document | React Query | `useCurrentFactoryDocument`, `useSaveCurrentFactory` | Edit, save, export, graph draft baseline, `baseVersion` |
| Dashboard snapshot | Zustand timeline + world view | `useDashboardSnapshot`, `useFactoryTimelineStore` | Timeline tick, runtime counts, observe-mode topology, in-flight guards |

React Flow nodes/edges are **projections** rebuilt from document topology (edit) or snapshot/world view (observe), plus optional runtime overlays.

### Session switch lifecycle

`useDashboardSessionLifecycle` → `resetDashboardSessionScopedState`:

- Reset timeline and stream state.
- `removeQueries` for `current-factory-definition` prefix (all sessions).
- Graph draft hooks key off `factoryDocumentScopeKey` === normalized `sessionID`.

Depends on **U2** (`prd-ui-session-scope-context`) for `useDashboardSession()` and session-keyed query keys.

### After-save flow

1. PUT success → `setQueryData` on document (and definition) keys.
2. Clear graph pending state.
3. SSE `FACTORY_CHANGE` → timeline replay → updated `DashboardSnapshot`; optional `syncCurrentFactoryDefinition` bridges version into document cache.

Depends on **U1/U3** for session factory client and shared save hook behavior.

### Feature touchpoints (implementation map, not story boundaries)

- `ui/src/features/current-factory-definition/` — document queries and save mutation cache writes.
- `ui/src/features/factory-graph-editor/` — draft state, editable graph, save controller.
- `ui/src/features/workflow-activity/` — activity card editor and graph layout.
- `ui/src/features/dashboard/` — session lifecycle, event stream sync.
- `ui/src/features/export/` — current factory export hook.

## Functional requirements

- **FR-1:** Features must not build save payloads from `DashboardSnapshot` alone when a factory document is loaded.
- **FR-2:** `normalizeFactoryDefinition` (or equivalent) applies to the document plane; snapshot may use separate normalization only for display projection.
- **FR-3:** React Query keys for the factory document include normalized `sessionID` from `useDashboardSession()`.
- **FR-4:** World view / `DashboardSnapshot` remains in the timeline store, not React Query.
- **FR-5:** Stale-save and save-blocked-while-busy UX combine document `version` with snapshot `runtime.in_flight_dispatch_count`.
- **FR-6:** Observe-mode surfaces may read `snapshot.factory`; edit-mode surfaces must prefer the document plane when available.

## Non-goals

- Changing backend event schema or world-view projection.
- Merging timeline and document into one store.
- Real-time collaborative editing.
- Replacing or renaming `DashboardSnapshot` wholesale.
- Broad unrelated dashboard refactors (covered by U5/U8 PRDs).

## Supporting technical and UX considerations

- **Upstream:** U1 session factory client, U2 session scope context, U3 factory document save hook.
- **Downstream:** U5 graph editor orchestration split benefits from clear plane inputs.
- **Accessibility:** Loading/empty/error states on session switch and document fetch must remain screen-reader friendly (existing patterns).
- **Testing:** Use divergent document vs snapshot fixtures; avoid meta-tests that only scan file lists or route registration.
- **Website standards:** See `docs/internal/standards/code/general-website-standards.md` — canonical state vs React Flow projection.

## Success metrics

- Zero production paths that assign `snapshot.factory` into a save payload without merging from the document plane when a document is loaded.
- Operators can switch tabs and save without seeing another session’s factory or exporting snapshot-only topology.
- Documented plane rules referenced in graph-editor and dashboard PR reviews.

## Open questions

None — convergence policy (immediate document cache + SSE snapshot) is specified in the dev guide; implementers should follow that contract unless backend event metadata changes.
