# Graph primitives (`@you-agent-factory/components/graphs`)

Generic React Flow presentation primitives for graph nodes, edges, handles, and
viewport chrome. The graph category is domain-free: it does not import factory,
workstation, work-state, generated OpenAPI, dashboard API, or dashboard feature
types.

Host applications own graph data fetching, durable graph state, domain node
renderers, layout projection, copy, and business workflows. Pass generic props
and callbacks into these components; keep product-specific graph logic in host
feature code.

## Install and import

Graph primitives import from the graphs category entrypoint:

```ts
import {
  GraphNodeShell,
  GraphNodeButton,
  GraphEdge,
  GRAPH_EDGE_TYPES,
  GraphViewportSurface,
  GraphNodeHandleBadge,
  buildGraphEdgePathThroughWaypoints,
  type GraphNodeHandle,
  type GraphNodeState,
} from "@you-agent-factory/components/graphs";
```

Import package styles once in the host application:

```css
@import "@you-agent-factory/components/styles.css";
```

## React Flow dependency

`@xyflow/react` is a direct dependency of `@you-agent-factory/components`, not
a dashboard-only peer. Graph primitives that integrate with React Flow resolve
it from the component package dependency graph.

| Package surface | React Flow boundary |
| --- | --- |
| `GraphNodeHandleBadge` | Wraps React Flow `Handle` for source/target affordances |
| `GraphEdge` | Registered as a React Flow custom edge type (`graphEdge`) |
| `GraphInteractiveExample` | Composes `ReactFlow`, `Controls`, and package node shells |
| `GraphNodeShell`, `GraphNodeButton`, `GraphViewportSurface` | Plain React markup; mount inside React Flow node renderers or viewport chrome |

When composing a full canvas, wrap node renderers in `ReactFlowProvider` and
register `GRAPH_EDGE_TYPES` (or a host-specific alias that forwards to
`GraphEdge`) on the `ReactFlow` instance. Import `@xyflow/react/dist/style.css`
in components that render `ReactFlow` controls or backgrounds.

## Component contracts

### `GraphNodeShell`

Presentation shell for a graph node body and handle rails.

- **Props:** `handles` (required), `children`, optional `nodeKind`, `state`,
  `stateLabel`, `contentInset`, `showStateIndicator` (default `true`), and
  standard `article` attributes. `contentInset` defaults to `default`; use
  `compact` for `0.5rem` content insets without handle rails and `1.25rem`
  on sides occupied by handle rails.
- **States:** `default`, `selected`, `disabled`, `loading`, `error`.
- **Accessibility:** Selected shells expose `aria-selected`; error shells expose
  `aria-invalid`; loading shells expose `aria-busy`; disabled shells expose
  `aria-disabled`. Selected and error shells use border weight/style and shadow
  in addition to color.
- **Layout:** Loading and loaded shells reserve a fixed-height state indicator
  row (`GRAPH_NODE_CONTENT_MIN_HEIGHT_CLASS`) to avoid layout shift.

```tsx
<GraphNodeShell
  handles={[
    { id: "in", label: "Input", side: "left", type: "target" },
    { id: "out", label: "Output", side: "right", type: "source" },
  ]}
  nodeKind="example"
  state="selected"
  stateLabel="Selected node"
>
  <GraphNodeButton graphState="selected">Example node</GraphNodeButton>
</GraphNodeShell>
```

### `GraphNodeButton`

Interactive control inside a node shell. Uses `graphState` (not `state`) to
avoid clashing with native button state.

- **States:** same `GraphNodeState` union as the shell.
- **Disabled behavior:** `disabled`, `loading`, and native `disabled` block
  activation while preserving readable labels.
- **Accessibility:** `aria-pressed` for selected buttons; `aria-busy`,
  `aria-invalid`, and `aria-disabled` mirror shell semantics.

```tsx
<GraphNodeButton
  graphState="loading"
  stateLabel="Loading node"
  onClick={() => activateNode()}
>
  Run step
</GraphNodeButton>
```

Shared state helpers (`graphNodeShellStateClassName`,
`graphNodeButtonStateAttributes`, `defaultGraphNodeStateLabel`, and related
exports) are available when host wrappers need the same contract.

### `GraphEdge`

Custom React Flow edge renderer. Accepts standard `EdgeProps` plus optional
`edgeClassName` and `labelClassName` for host styling.

- **Data:** optional `label`, `alwaysShowLabel`, and `waypoints` on edge `data`.
- **Geometry:** uses `buildGraphEdgePathThroughWaypoints` when waypoints are
  present; otherwise falls back to React Flow `getBezierPath`.
- **Registration:** export `GRAPH_EDGE_TYPES` or alias `graphEdge` in host edge
  type maps.

```tsx
const edgeTypes = { graphEdge: GraphEdge };

<ReactFlow edgeTypes={edgeTypes} /* ... */ />
```

### `GraphViewportSurface`

Semantic viewport chrome (`section` with `role="region"` by default) for graph
frames. Accepts children (typically `ReactFlow`) and standard section attributes.

Pass an explicit height (`h-*`, `min-h-*`) for Storybook or standalone examples.
Wrap interactive examples in an explicit width (`w-[48rem]`, `w-80`, etc.) when
Storybook uses centered layout; `w-full` alone can collapse to a few pixels wide.
Hosts that fill flex parents should add `h-full` (and usually `min-h-0`) via
`className`; the primitive does not force `h-full` so explicit heights are not
capped by a collapsed parent.

```tsx
<GraphViewportSurface className="h-[480px] border-outline">
  <ReactFlow /* ... */ />
</GraphViewportSurface>
```

### `GraphNodeHandleBadge`

Renders a single source or target handle with accessible labels and tone-based
dot styling. Usually composed by `GraphNodeShell`; import directly when building
custom node layouts.

- **Props:** `handle: GraphNodeHandle` with `id`, `label`, `side`, `type`, and
  optional tone, variant, validation, and button interaction fields.
- **Accessibility:** `aria-label` from `buttonAriaLabel` or `label`; invalid
  handles expose `aria-invalid`.

### `buildGraphEdgePathThroughWaypoints`

Pure edge-path helper for waypoint-routed geometry. Returns `{ path, labelX,
labelY }` for representative straight, stepped, curved, and bezier-fallback
cases. Safe to call from host layout code without importing dashboard models.

```ts
const routed = buildGraphEdgePathThroughWaypoints({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  waypoints: [{ x: 120, y: 80 }],
});
```

## Storybook examples

Package Storybook discovers stories under `src/graphs/*.stories.tsx` without
dashboard providers. Run from `ui/packages/components`:

```bash
bun run build-storybook
bunx http-server storybook-static -p 6017 -a 127.0.0.1 -s
```

Open `http://127.0.0.1:6017` and use the story IDs below.

### Node states (`Graphs/GraphNodeStates`)

| Story | Storybook ID | What it shows |
| --- | --- | --- |
| Selected | `graphs-graphnodestates--selected` | Selected shell and button affordances |
| Disabled | `graphs-graphnodestates--disabled` | Non-activating disabled node |
| Loading | `graphs-graphnodestates--loading` | Stable loading dimensions and `aria-busy` |
| Loaded | `graphs-graphnodestates--loaded` | Default loaded node after loading |
| Error | `graphs-graphnodestates--error-state` | Error border style and `aria-invalid` |

### Edges and handles (`Graphs/GraphEdgesAndHandles`)

| Story | Storybook ID | What it shows |
| --- | --- | --- |
| Bezier edge | `graphs-graphedgesandhandles--bezier-edge` | Default bezier edge path |
| Waypoint edge | `graphs-graphedgesandhandles--waypoint-edge` | Routed path through waypoints |
| Source/target handles | `graphs-graphedgesandhandles--source-target-handles` | Accessible handle labels |
| Selected node handles | `graphs-graphedgesandhandles--selected-node-handles` | Handle placement on selected shell |
| Desktop viewport | `graphs-graphedgesandhandles--desktop-viewport` | Desktop-width edge/handle layout |
| Narrow viewport | `graphs-graphedgesandhandles--narrow-viewport` | Narrow-width readability |

### Interactive and responsive (`Graphs/GraphInteractiveExamples`)

| Story | Storybook ID | What it shows |
| --- | --- | --- |
| Interactive | `graphs-graphinteractiveexamples--interactive` | Pointer/keyboard node selection in a React Flow canvas |
| Selected | `graphs-graphinteractiveexamples--selected` | Selected state panel |
| Disabled | `graphs-graphinteractiveexamples--disabled` | Disabled state panel |
| Loading | `graphs-graphinteractiveexamples--loading` | Visible loading state without hover |
| Error | `graphs-graphinteractiveexamples--error-state` | Visible error state without hover |
| Desktop viewport | `graphs-graphinteractiveexamples--desktop-viewport` | Desktop canvas with controls |
| Narrow viewport | `graphs-graphinteractiveexamples--narrow-viewport` | Narrow canvas without horizontal overflow |

Loading, error, disabled, and selected states are visible in the story canvas
without opening addon panels.

## Host responsibilities

Keep these concerns in host application code, not in the graph package:

- Fetching graph topology, work items, and runtime facts from APIs
- Durable editor state, undo/redo, and save/discard workflows
- Domain node renderers that map product models onto `GraphNodeShell` /
  `GraphNodeButton` props
- Layout projection, edge routing policy beyond generic waypoints, and product copy
- React Flow `nodes`, `edges`, `onNodesChange`, and business interaction handlers

The dashboard follows this boundary: `features/graphs/public/` re-exports package
primitives and adds domain wrappers (for example activity-graph shells and
factory edge class names) without forking generic presentation logic.

## Development checks

From `ui/packages/components`:

```bash
bun run typecheck
bun run test
bun run build-storybook
bun run verify:storybook-browser
```

Graph tests use `ReactFlowProvider` and, for full-canvas cases,
`installReactFlowTestShims()` from `@you-agent-factory/components/testing`.
