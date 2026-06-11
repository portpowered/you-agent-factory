# PRD-005: Visual Groups

## Introduction

Add ComfyUI-style visual groups to the factory graph editor: labeled background
boxes that organize related nodes on the canvas without changing runtime
topology. Groups are presentation-only layout metadata stored in
`layout.groups[]`; they may contain member node ids, a label, bounds, style
metadata, and a reserved `parentGroupId` field for future nested groups.

The concrete problem is that large factories become hard to scan once several
work types, states, workstations, workers, guards, and resources share one
canvas. Authors need a way to mark zones visually, move those zones with their
member nodes, and save that organization without creating executable subgraphs
or changing factory behavior.

The high-level solution is to add first-version flat visual groups to the
website layout editor. Users can create, rename, style, assign nodes, move,
resize, delete, save, reload, and undo or redo group layout changes. The backend
continues to treat groups as non-executable layout metadata, pruning stale node
references and rejecting invalid geometry during save validation.

## Dependencies

- PRD-001: Stable IDs And Layout Contract.
- PRD-002: Layout Persistence And Validation.
- PRD-003: Website Layout Editing Foundation.
- PRD-004: Edge Waypoints should remain compatible but is not required for basic
  group creation.

## Goals

- Let factory authors create labeled background boxes that organize canvas
  regions.
- Persist group metadata in `layout.groups[]` with group-level `nodeIds`.
- Keep groups flat in v1 while preserving `parentGroupId` when present in saved
  layout data.
- Move a group background and member nodes together by default.
- Make group edits undoable, redoable, saveable, reloadable, and visible behind
  member nodes.
- Keep groups presentation-only and unable to alter execution topology,
  scheduling, generated edges, or runtime events.

## Project-Level Acceptance Criteria

- Users can create, rename, style, assign nodes to, move, resize, and delete
  visual groups from the graph editor.
- Group edits update only layout metadata, set `layoutDirty`, and use existing
  save/discard/undo/redo behavior.
- Saved factories persist `layout.groups[]`, prune stale `nodeIds` on save, and
  reject or report non-finite group bounds.
- Group rendering appears behind graph nodes and does not use nested card-like
  UI that implies executable containment.
- Deleting a group preserves its member nodes and all topology edges.
- Browser verification covers create, rename, assign, move, resize, delete,
  save, reload, and undo/redo for visual groups.
- Typecheck, lint, and relevant Go/UI tests pass.

## User Stories

### prd-better-editor-005-visual-groups-001: Preserve Visual Group Layout State

**Description:** As a maintainer, I want editable factory layout state to carry
visual groups so that group data can round-trip through editing, validation, and
save without affecting topology.

**Acceptance Criteria:**

- [x] Existing factories with `layout.groups[]` load into the editor with group
  id, label, bounds, `nodeIds`, color, locked state, and `parentGroupId`
  preserved.
- [x] Saving a factory writes group changes back to `layout.groups[]` without
  changing work types, work states, workstations, workers, resources, guards, or
  topology edges.
- [x] Empty groups remain valid and are saved unless the user deletes them.
- [x] Missing member node ids are pruned on save and reported through layout
  save outcomes.
- [x] Non-finite group bounds are rejected or reported as layout validation
  targets and do not corrupt the saved factory.
- [x] Typecheck passes.
- [x] Tests pass.

### prd-better-editor-005-visual-groups-002: Create, Rename, and Style Groups

**Description:** As a factory author, I want to create labeled, styled
background groups so that I can mark zones on a large factory canvas.

**Acceptance Criteria:**

- [ ] Users can create a new visual group on the canvas with finite default
  bounds and a stable group id.
- [ ] Users can rename a group, and the visible label updates without requiring
  a save first.
- [ ] Users can choose only approved group style options, and the selected style
  is persisted as layout metadata.
- [ ] Create, rename, and style changes update `layout.groups[]` and set
  `layoutDirty`.
- [ ] The group appears as a background region behind nodes, not as a foreground
  card or executable node.
- [ ] Loading, empty, error, and success states for group editing controls are
  explicit and keyboard reachable.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### prd-better-editor-005-visual-groups-003: Assign Nodes to Groups

**Description:** As a factory author, I want a visual group to remember which
nodes it contains so that factory sections stay organized after save and reload.

**Acceptance Criteria:**

- [ ] Users can add an existing canvas node to a visual group.
- [ ] Users can remove a node from a visual group without deleting the node.
- [ ] Group membership is stored on the group entry as `nodeIds`; node layout
  entries do not become the source of truth for membership.
- [ ] Assigning or removing a node from a group does not add, remove, or modify
  topology edges or runtime relationships.
- [ ] Nested group assignment is not exposed in v1, but existing
  `parentGroupId` values are preserved when saving unrelated group changes.
- [ ] Membership controls are accessible by keyboard and expose selected,
  unselected, empty, and error states clearly.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### prd-better-editor-005-visual-groups-004: Move, Resize, and Delete Groups

**Description:** As a factory author, I want to rearrange a visual group and its
member nodes together so that reorganizing a section is fast and reversible.

**Acceptance Criteria:**

- [ ] Dragging a group moves the group bounds and all member nodes by the same
  delta.
- [ ] Resizing a group changes only group bounds and does not move nodes unless
  the user also performs a move operation.
- [ ] Group move, resize, membership, style, rename, create, and delete
  operations have undo and redo behavior.
- [ ] Deleting a group removes only the group entry from `layout.groups[]`;
  member nodes and topology edges remain.
- [ ] Group interaction remains usable at mobile, tablet, and desktop
  breakpoints without overlapping labels, handles, or graph controls.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### prd-better-editor-005-visual-groups-005: Save, Reload, and Inspect Groups

**Description:** As a factory author, I want visual groups to restore after save
and reload so that my canvas organization remains durable across sessions.

**Acceptance Criteria:**

- [ ] After creating or editing groups and saving, reloading the factory restores
  the same group labels, bounds, styles, membership, and preserved
  `parentGroupId` values.
- [ ] Save summaries and validation feedback distinguish recoverable layout
  group warnings from blocking topology errors.
- [ ] Stale group members are pruned during save without deleting the group
  itself.
- [ ] Invalid group bounds produce visible, bounded error feedback and do not
  leave the editor in a permanently dirty or unsaveable state after correction.
- [ ] Browser verification covers create, rename, assign, move, resize, delete,
  save, reload, and undo/redo in one representative factory.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

## High-Level Technical Design

The canonical durable model is the factory layout contract, specifically
`layout.groups[]`. Editor components and React Flow projections should treat
groups as a derived visual layer over the canonical editable layout state, not
as executable nodes. Operations that mutate groups should write canonical group
layout entries first, then project those entries into background regions and
interaction handles.

Group membership is group-owned through `nodeIds`. Dragging a group should reuse
the existing layout node movement operation for member nodes so node positions,
dirty state, undo/redo, and save behavior stay consistent with normal node
movement. Resizing a group updates group bounds only. Deleting a group removes
the layout group entry but leaves all factory resources and topology edges
unchanged.

The backend/API boundary should continue to accept groups as non-executable
layout metadata. Validation and save pruning should reject or report invalid
geometry, prune stale member node ids, preserve empty groups, and avoid
introducing runtime events or scheduling behavior for groups. If OpenAPI
fragments or generated clients are touched, the authored OpenAPI contract must
be regenerated rather than editing generated files directly.

## Functional Requirements

1. `FR-1`: The editor must render visual groups behind member nodes.
2. `FR-2`: Visual groups must never alter runtime topology, scheduling, work
   dispatch, generated edges, or execution semantics.
3. `FR-3`: Group metadata must persist in `layout.groups[]` with id, label,
   bounds, `nodeIds`, optional color/style, optional locked state, and optional
   `parentGroupId`.
4. `FR-4`: Group membership must be stored on group entries as `nodeIds`.
5. `FR-5`: Dragging a group must move the group bounds and member node layout
   positions by the same delta.
6. `FR-6`: Resizing a group must change group bounds without changing topology.
7. `FR-7`: Deleting a group must not delete member nodes or topology edges.
8. `FR-8`: Group operations must participate in existing layout dirty,
   save/discard, undo/redo, and validation flows.
9. `FR-9`: Save validation must prune stale member references and reject or
   report invalid group bounds.
10. `FR-10`: The v1 UI must not expose nested group creation, even though
   `parentGroupId` is preserved for future compatibility.

## Non-Goals

- No nested group UI in the first version.
- No collapsed groups in the first version.
- No executable subgraphs.
- No topology edge creation, deletion, or waypoint behavior changes.
- No automatic group creation based on node proximity.
- No broad graph editor redesign outside the visual group behavior.

## Supporting Technical and UX Considerations

- Prefer existing editor controls, dialogs, popovers, action buttons, and status
  treatments for group actions.
- Group labels and resize affordances must remain readable and reachable at
  mobile, tablet, and desktop sizes.
- Keyboard users must be able to create, select, rename, style, assign, move
  where existing keyboard movement patterns support it, delete, undo, and redo
  groups.
- Group style choices should use a small approved vocabulary that works with
  existing theme colors and keeps nodes visually dominant.
- Projection/component tests should verify z-ordering so groups render behind
  nodes.
- Operation tests should verify command inverses for create, rename, style,
  assign, unassign, move, resize, and delete.
- Browser verification should use a representative large factory fixture with
  enough nodes to prove grouping improves scanability without layout overlap.

## Success Metrics

- A factory author can create a labeled group and assign at least two nodes in
  one editing session without modifying topology.
- Group move operations reduce section rearrangement from multiple node drags to
  one group drag.
- Saved groups restore consistently after reload with no manual repair.
- Validation catches invalid bounds and stale member references before they can
  corrupt durable layout.
- Visual group interactions do not regress existing node movement, edge display,
  save/discard, or undo/redo behavior.

## Open Questions

- What exact group style vocabulary should be approved for v1: color swatches
  only, or color plus a small set of border treatments?
- Should group membership be primarily controlled by explicit selection actions,
  drag-to-contain behavior, or both in the first implementation?
