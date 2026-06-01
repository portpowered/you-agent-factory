# PRD: Warn When Current-Selection Save Conflicts With Unsaved Graph Draft

## Introduction

Operators can edit the factory graph in editor mode (pending nodes, edges, and removals in the graph draft) while also editing entity configuration in the current-selection panel. When they save a **topology-affecting** change from current selection, the running factory document updates immediately, but the graph draft may still reflect an older baseline. Today that mismatch is easy to miss: the save can succeed while the graph draft stays dirty, and the operator only discovers the problem later when graph save is blocked or the canvas looks wrong.

This project adds an explicit **warning toast** at the moment a topology-affecting current-selection save succeeds while the graph editor still has unsaved draft changes. The warning uses the same global notification path as other factory save feedback and tells the operator to review or discard the graph draft before continuing.

## Context

### Customer ask

If graph edit mode has unsaved draft changes and current selection saves topology, warn via global notification rather than silently merging or discarding.

### Problem

- The factory graph editor keeps a local `FactoryGraphDraft` with explicit save/discard controls (`hasChanges` / “Pending graph changes”).
- Current-selection saves (workstation, worker, resource, work type, work state) persist through `useScopedFactoryDocumentSave` and can change graph-relevant factory structure.
- When a topology-affecting external save lands while the graph draft is dirty, `syncFactoryGraphDraftSession` updates `latestDocument` but preserves the draft (see `issues3-current-selection-graph-refresh-topology`). The graph becomes **stale relative to the saved factory** without a clear, immediate signal at save time.
- Graph-initiated stale draft feedback exists inline on the graph surface and is intentionally **not** toasted (`STALE_FACTORY_GRAPH_DRAFT_WARNING` is suppressed in `resolveGraphDocumentSaveToastNotification`). Current-selection saves are a different entry point and need their own warning at the moment of conflict.

### High-level solution

1. **Expose graph draft dirtiness to current selection** — Publish whether the graph editor has pending draft changes through a small dashboard bridge (extend `useFactoryGraphTopologyEditorBridge` or an adjacent store) registered from the workflow-activity graph editor hook.
2. **Detect conflict on topology-affecting save success** — After a successful current-selection save classified as topology-affecting, if the bridge reports `graphDraftHasPendingChanges`, treat it as a graph-draft conflict.
3. **Resolve and deliver a warning toast** — Map the conflict to localized warning title/description copy and deliver via the global Sonner toaster using the same `save-notification-delivery-policy` dedupe keys as current-selection save notifications.
4. **Preserve draft semantics** — Do not auto-discard, auto-merge, or block the current-selection save; only warn. Graph refresh and stale-inline notice behavior remain as defined in sibling work.

## Goals

- Surface an immediate warning when a topology-affecting current-selection save succeeds while the graph draft is still dirty.
- Explain that the graph draft may be stale or needs review (discard, reconcile, or refresh before graph save).
- Route the warning through the global notification component, consistent with graph-editor and current-selection save toasts.
- Avoid warning noise on non-topology saves or when the graph draft is clean.
- Keep graph draft data intact; no silent merge or discard.

## Project-level acceptance criteria

- [ ] After a successful **topology-affecting** save from current selection (workstation, worker, resource, work type, or work state), when the graph editor has unsaved draft changes, a **warning** global toast appears.
- [ ] The warning title and description state that the graph draft may be stale or should be reviewed before saving the graph (discard or refresh intent), using localized copy in all supported locales.
- [ ] The warning is delivered through the app-wide Sonner toaster (`AppNotificationToaster`) with the same duration/dedupe policy as other save notifications—not inline in the current-selection form body.
- [ ] When the graph draft has no pending changes, topology-affecting current-selection saves do **not** show the conflict warning.
- [ ] Non-topology current-selection saves (for example workstation prompt-only edits) do **not** show the conflict warning even if the graph draft is dirty.
- [ ] The current-selection save still completes successfully; the graph draft is not silently discarded or merged.
- [ ] Typecheck, lint, and tests pass for touched UI packages.

## User Stories

### issues3-current-selection-save-warn-graph-draft-001: Publish graph draft pending state to the dashboard bridge

**Description:** As a maintainer, I need current-selection save flows to read whether the graph editor has unsaved draft changes so conflict detection does not reach into graph-editor hooks directly.

**Acceptance Criteria:**

- [ ] The workflow-activity graph editor registers `graphDraftHasPendingChanges` (derived from `draftState.hasChanges`) on the existing factory graph topology editor bridge (or a dedicated adjacent store) whenever the editor mounts or draft dirtiness changes.
- [ ] Registration clears on unmount or session scope change so stale `true` values do not leak across dashboard sessions.
- [ ] Current-selection code can read the published flag without importing graph-editor React hooks.
- [ ] Unit tests cover register/update/clear behavior and scope reset.
- [ ] Typecheck passes
- [ ] Tests pass

### issues3-current-selection-save-warn-graph-draft-002: Add localized copy for graph-draft conflict warnings

**Description:** As an operator, I want clear warning text when my current-selection save may have invalidated my graph draft so I know to review the graph editor next.

**Acceptance Criteria:**

- [ ] Message keys exist for a warning title and description explaining the graph draft may be stale or needs review (discard or refresh before graph save).
- [ ] Copy is added to all supported locale catalogs (`en`, `ja`, `ko`, `zh-CN`) following existing factory-graph-editor / current-selection message patterns.
- [ ] Wording does not claim the graph draft was discarded or merged.
- [ ] Typecheck passes

### issues3-current-selection-save-warn-graph-draft-003: Resolve graph-draft conflict warning toast payloads

**Description:** As a maintainer, I need a pure resolver that decides when a topology-affecting current-selection save should emit a graph-draft conflict warning toast.

**Acceptance Criteria:**

- [ ] A pure `resolveCurrentSelectionGraphDraftConflictNotification` (or equivalent) returns `{ kind: "warning", title, description, key } | null`.
- [ ] Returns a warning payload when `saveSucceeded`, `isTopologyAffectingSave`, and `graphDraftHasPendingChanges` are all true.
- [ ] Returns null when the graph draft is clean, the save did not succeed, or the save was not topology-affecting.
- [ ] Uses the localized message keys from story 002 for title and description.
- [ ] Unit tests cover positive conflict, clean draft, non-topology save, and failed save cases.
- [ ] Typecheck passes
- [ ] Tests pass

### issues3-current-selection-save-warn-graph-draft-004: Deliver graph-draft conflict warnings via global notifications

**Description:** As an operator, I want the conflict warning in the same global toaster as other save feedback so I notice it even when the current-selection panel is scrolled.

**Acceptance Criteria:**

- [ ] Warning delivery uses `toast.warning` with `GLOBAL_TOAST_DURATION_MS` and existing `buildSaveNotificationDeliveryKey` / `shouldDeliverSaveNotification` dedupe (same pattern as `CurrentActivityGraphSaveNotifications` and `CurrentSelectionSaveNotifications`).
- [ ] Delivery is wired into the current-selection save notification component (or a sibling render-null notifier mounted from `current-selection-widget.tsx`) and depends on save notification routing from `issues3-current-selection-save-notifications` when that work is present.
- [ ] A successful entity save toast and the graph-draft conflict warning can both appear for the same save attempt when applicable (warning does not replace success).
- [ ] Unit tests mock Sonner and assert warning title, description, and dedupe across rerenders vs new `saveAttemptRevision`.
- [ ] Typecheck passes
- [ ] Tests pass

### issues3-current-selection-save-warn-graph-draft-005: Emit conflict warning after topology-affecting current-selection saves

**Description:** As a factory operator editing the graph and current selection together, I want an immediate warning when my panel save updates the factory while I still have unsaved graph draft edits.

**Acceptance Criteria:**

- [ ] After a successful topology-affecting save from workstation, worker, resource, work type, or work state current selection, the save flow checks the bridge flag and invokes conflict warning delivery when the graph draft is dirty.
- [ ] Topology-affecting classification reuses the shared classifier from `issues3-current-selection-graph-refresh-topology` when available; otherwise this story includes the same classifier contract so behavior matches graph refresh.
- [ ] Non-topology saves never trigger the conflict warning.
- [ ] Graph draft state is not reset or merged by this warning path.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: enter graph editor, add a pending edge, save a topology-affecting workstation change from current selection, confirm a warning toast appears and the graph draft remains dirty

### issues3-current-selection-save-warn-graph-draft-006: Regression coverage for conflict warning boundaries

**Description:** As a maintainer, I want automated tests at the widget/hook level so conflict warnings cannot regress silently across entity types.

**Acceptance Criteria:**

- [ ] Tests cover at least one topology-affecting save per entity family (workstation, worker, resource, work type, work state) with a dirty graph draft asserting `toast.warning` is called with the conflict copy.
- [ ] Tests cover a non-topology workstation save with a dirty graph draft asserting no conflict warning toast.
- [ ] Tests cover a topology-affecting save with a clean graph draft asserting no conflict warning toast.
- [ ] Tests use behavioral assertions (toast calls, message text) rather than source inventories.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** When a topology-affecting current-selection save succeeds and `graphDraftHasPendingChanges` is true, the system MUST show a global warning notification.
- **FR-2:** Warning copy MUST state that the graph draft may be stale or needs review before graph save.
- **FR-3:** Warning delivery MUST use the global Sonner notification component and shared save-notification dedupe policy.
- **FR-4:** Non-topology current-selection saves MUST NOT emit the graph-draft conflict warning.
- **FR-5:** Current-selection saves MUST NOT be blocked, and graph drafts MUST NOT be silently discarded or merged when the warning fires.
- **FR-6:** The conflict warning MAY appear alongside the normal current-selection save success toast for the same save attempt.

## Non-Goals

- Blocking or cancelling current-selection saves when the graph draft is dirty.
- Auto-discarding or auto-merging graph drafts on external save.
- Changing graph-editor inline stale-draft notice behavior or graph-initiated save toasts.
- Routing non-conflict save outcomes (success/error/stale-version) — covered by `issues3-current-selection-save-notifications`.
- Refreshing graph layout/topology after save — covered by `issues3-current-selection-graph-refresh-topology`.
- Backend or OpenAPI changes.

## High-level technical design

**Bridge**

Extend `useFactoryGraphTopologyEditorBridge` (or add `useFactoryGraphDraftConflictBridge`) so `useCurrentActivityGraphEditor` publishes `graphDraftHasPendingChanges: boolean` whenever `draftState.hasChanges` changes. Current-selection save hooks read this flag after successful saves.

**Topology detection**

Reuse the topology-affecting classifier from `issues3-current-selection-graph-refresh-topology` (graph-relevant canonical factory diff). Conflict warnings only fire when that classifier returns true for the save that just succeeded.

**Notification pipeline**

```
topology save success
  → read bridge.graphDraftHasPendingChanges
  → resolveCurrentSelectionGraphDraftConflictNotification(...)
  → CurrentSelectionSaveNotifications (or sibling) → toast.warning
```

Align stable identity keys with `save-notification-delivery-policy` so rerenders do not duplicate warnings within one save attempt revision.

**Coordination**

- Depends on global current-selection save notification routing (`issues3-current-selection-save-notifications`) for mounting and `saveAttemptRevision`.
- Complements graph refresh (`issues3-current-selection-graph-refresh-topology`), which updates `latestDocument` without discarding dirty drafts; this PRD adds operator-visible feedback at save time.

## Supporting technical and UX considerations

- Follow `docs/internal/standards/code/general-website-standards.md` for UI and testing.
- Warning toasts auto-dismiss with `GLOBAL_TOAST_DURATION_MS`; they are informational, not persistent errors.
- Do not steal focus from the current-selection panel after save.
- If graph editor is not mounted (bridge handlers null), treat `graphDraftHasPendingChanges` as false.
- Accessibility: warnings inherit Sonner/toaster semantics already used app-wide.

## Success Metrics

- Operators with simultaneous graph draft edits and topology current-selection saves see a warning on the first successful save, without reloading the page.
- Zero reports of “silent” graph draft invalidation after current-selection topology saves in manual QA.
- No increase in warning toasts on non-topology saves in automated tests.

## Open Questions

None. Warning is informational only; graph draft preservation and refresh semantics are defined by sibling PRDs.
