# Factory Graph Refactor Plan

## Objective

Make the existing website Factory graph the canonical implementation, extract
it intact into a reusable package, and have every host supply state to that one
implementation. The generic topology renderer should be retired rather than
enhanced in parallel.

The key regression is the observe-mode branch in
`ui/src/features/workflow-activity/components/react-flow-current-activity-card-surface.tsx`:
observe mode uses `HostedTopologyReplay`, while edit mode still uses the
original semantic viewport. The intended canonical-state and projection
boundaries are already documented in
`docs/internal/development/current-activity-graph-data-model.md`.

## Target Architecture

Establish `@you-agent-factory/factory-graph` as the canonical package. This is
a shared package boundary, not another renderer:

```text
FactoryDefinition + selected-tick runtime + saved/draft layout
                              |
                              v
                 @you-agent-factory/factory-graph
          semantic projection -> nodes/edges -> graph viewport
                              |
              +---------------+----------------+
              |               |                |
              v               v                v
       Hosted dashboard   Recording/replay   Editor controller
       state adapter      state adapter      + save adapter
```

The package owns:

- The original worker, workstation, resource, work type, work state,
  constraint, and document node presentations.
- Canonical graph IDs, handles, edges, active-flow decoration, layout
  application, waypoints, visual groups, legend, visibility controls, and
  viewport.
- The toolbar and controlled edit-mode presentation.
- Pure graph projection and editor operations that do not require transport.
- Explicit loading, empty, failure, observe, and edit states.

Hosts own:

- Event streaming and timeline selection.
- Factory persistence, save mutations, imports, and cache synchronization.
- Current selection destinations.
- Controlled editor session state and confirmation workflows.
- Translation from hosted or replay runtime state into the package's
  normalized runtime contract.

React Flow nodes and edges remain disposable UI projections.
`FactoryDefinition`, selected-tick runtime state, and the controlled editor
draft remain canonical.

## Implementation Plan

### 1. Restore the website baseline immediately

Before extraction, remove the observe-mode `HostedTopologyReplay` early return
and restore the pre-`a29f39900` behavior where observe and edit modes both
render `CurrentActivityGraphViewport`.

Also restore the observe-mode factory-selection logic removed by `cdb0ded6c`,
so the graph again uses the event-computed `snapshot.factory`, including saved
layout and bundled documents.

Acceptance criteria:

- Observe mode renders through the same semantic viewport and `NODE_TYPES` as
  edit mode.
- Dedicated node classes, icons, resource counts, work-state phases,
  workstation activity rows, saved layout, legend, visibility controls, and
  selection styling return.
- Historical timeline selection decorates the semantic graph from the selected
  snapshot.
- Observe mode does not query a second Factory source.
- Editing continues to enter from the same graph without remounting a different
  renderer.

This is the recovery PR and establishes the executable baseline for
extraction.

### 2. Lock parity with recovered regression coverage

Restore or port the behavioral coverage deleted in `cdb0ded6c`, especially the
former
`react-flow-current-activity-card-graph-semantics.test.tsx`.

The parity suite should cover:

- Every semantic node family.
- Node selection and selected styling.
- Semantic icons and readable identifiers.
- Active, failed, terminal, and inactive flow styling.
- Resource occupancy across selected ticks.
- Workstation active-work rows.
- Saved layout, edge waypoints, visual groups, and viewport.
- Bundled document nodes.
- All edge endpoints resolving to handles actually rendered by their nodes.
- Toolbar, legend, visibility, localization, accessibility, and responsive
  behavior.
- Observe-to-edit continuity.

These tests become the package extraction contract. They should test visible
behavior, not merely that renderer names are registered.

### 3. Introduce the canonical package input contract

Define a lossless package-owned input rather than continuing with the reduced
`FactoryTopologyProjection`.

A suitable shape is conceptually:

```ts
interface FactoryGraphSource {
  factory: FactoryDefinition;
  runtime: FactoryGraphRuntimeProjection;
  selectedTick: number;
  layout?: FactoryVisualizationLayout;
}

type FactoryGraphMode =
  | { kind: "observe"; controls: FactoryGraphObserveControls }
  | { kind: "edit"; controls: FactoryGraphEditorControls };
```

`FactoryGraphRuntimeProjection` should normalize only what the original
renderer consumes: active executions, work items, place occupancy, resource
use, workstation status, and active routes. Both the dashboard snapshot and
replay state must be able to produce it.

Acceptance criteria:

- The full `FactoryDefinition` remains available to semantic renderers;
  workstation and worker metadata are not discarded.
- Layout and bundled-document information survive the boundary.
- Node handles and edge endpoints derive from the same semantic model.
- The package contract contains no dashboard stores, React Query objects,
  transport clients, or persistence assumptions.
- Edit and save are unavailable when the caller supplies only lossy replay
  data.

### 4. Move the original projection and semantic renderers

Move, rather than rewrite, the website's canonical code into the package:

- Layout and topology projection.
- Semantic node-model construction.
- Dedicated node views currently registered in
  `ui/src/features/flowchart/components/current-activity-nodes.tsx`.
- Factory edges, semantic handles, icons, active-flow decoration, and selection
  presentation.
- Relevant localized messages and styles.

The dashboard must switch its imports to the moved package code in the same
PR. Avoid leaving copied versions under both `ui/src/features` and
`ui/packages`.

Acceptance criteria:

- The recovered parity suite passes against package exports.
- Package and website Storybook renders are visually and semantically
  equivalent.
- There is one implementation of each Factory node family.
- The package works from a packed clean consumer, not only through Vite source
  aliases.

### 5. Extract the viewport and controlled editor interface

Move the original `CurrentActivityGraphViewport`, toolbar, legend, visibility
controls, waypoint and group layers, and mode-specific interaction wiring into
the package.

Expose compact controlled interfaces for:

- Selection.
- Mode switching.
- Add, connect, and delete.
- Node and group movement.
- Layout undo, redo, and reset.
- Save and discard capability and status.
- Validation and notices.

Move pure draft, layout, and connection operations where they are required for
consistent graph semantics. Keep session persistence and API mutations in the
dashboard adapter.

Allow explicit host overlay regions for dashboard-owned import state, save
notifications, and confirmation dialogs, but do not make semantic node
rendering or standard graph chrome host-supplied slots.

Acceptance criteria:

- Observe and edit are modes of one mounted graph surface.
- Switching modes preserves layout, selection, viewport, and semantic node
  identity.
- The dashboard supplies its existing `useCurrentActivityGraphState`
  controller through the package interface.
- Replay consumers omit editor controls and receive the same graph in read-only
  mode.
- The package does not depend on dashboard, Zustand, React Query, SSE, or
  browser persistence.

### 6. Migrate replay and emulator compositions

Change `FactoryTopologyReplay` and `FactoryRecordingTopologyReplay` to prepare
`FactoryGraphSource` and render the canonical graph.

The timeline scrubber and Work-progress visualizer can remain composition
components, but they must surround the canonical graph rather than select
another graph implementation.

Because the current public package is version `0.0.0`, make the topology
contract intentionally lossless instead of retaining a generic visual
fallback. If an API transition is necessary, the old export may temporarily
delegate to the new graph, but it must never retain its own nodes, edges, or
layout algorithm.

Acceptance criteria:

- Hosted, recording, and emulator examples use the same node implementations
  and graph viewport.
- Saved layout is honored whenever supplied.
- Embedded sizing is controlled by the host container; the standalone
  fixed-height assumption is removed.
- Current and historical replay update semantic node contents without
  replacing the graph implementation.

### 7. Delete the bespoke renderer and enforce the boundary

Remove:

- `factory-topology-replay-nodes.tsx`.
- The generic `factoryTopologyNode` type.
- The fixed column-by-kind layout.
- Generic topology-specific CSS and chrome that duplicate the original graph.
- The workflow-activity `HostedTopologyReplay` renderer branch.
- Any private duplicate semantic renderers left in the website.

Update package-boundary checks. The current check explicitly rejects editor or
graph-editor code; replace that rule with architectural checks that forbid
transport, storage, dashboard stores, React Query, and hidden durable state.

Add a source guard ensuring the retired generic node and observe-mode split
cannot return.

## Verification Gates

For each slice, run focused projection and component tests first. Before
completion, run:

- `cd ui && bun run typecheck`
- `cd ui && bun run lint`
- `cd ui && bun run test:unit`
- Relevant Factory graph editor and session-switch integration tests.
- `cd ui && bun run test-storybook`
- `cd ui && bun run storybook:responsive-check`
- `cd ui && bun run verify:public-packages`
- `make verify-fast`
- `make verify-pr` for the final consolidation PR.

Browser verification should cover observe and edit modes at mobile, tablet,
and desktop widths, keyboard operation, accessible graph controls, session
switching, selected-tick replay, large graphs, and React Flow missing-handle
failures.

## Definition of Done

The migration is complete only when the website, hosted replay, recording
viewer, and emulator all render through the same semantic Factory graph
package; observe and edit modes share one viewport; and the bespoke generic
topology renderer no longer exists.

No backend or OpenAPI changes should be required.
