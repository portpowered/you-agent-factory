---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/factory-graph-visual-ux
---

# Factory Graph Visual UX Plan

## Outcome

Customers can read and edit a Factory graph without fighting narrow cards,
truncated names, visually flat runtime state, misleading workstation kinds,
disconnected Work states, or opaque grouping rectangles. The graph uses one
consistent visual grammar for entity identity, workstation type and scheduling
behavior, runtime status, selection, Work volume, node size, and authored
groups.

The intended visual direction is a dark, quiet canvas where inactive structure
recedes, active or terminal state is immediately legible, important nodes have
enough room for their names, dense Work becomes a large count, and groups read
as colored regions behind the graph rather than panels placed on top of it.
The first release uses well-rounded rectangular regions; an organic concave
hull or free-form blob renderer is not required.

## Customer problem

The current graph exposes the Factory topology but makes it hard to understand
at a glance:

- inactive and active objects do not follow one strong, predictable color
  hierarchy;
- active styling is often limited to a border or glow and does not carry
  enough visual weight at fit-to-view zoom;
- Work is rendered as rows or dots until counts are already high, making busy
  nodes visually noisy;
- fixed, narrow dimensions truncate authored names and paths;
- the graph collapses a workstation's runtime `type` and scheduling `behavior`
  into one optional `workstation_kind` field, so logical moves, classifiers,
  guarded loop breakers, and executable workstations can receive the wrong
  icon, shape, or size;
- package replay omits both legacy workstation-kind inputs and can therefore
  infer the special exhaustion presentation for an ordinary workstation;
- declared but disconnected Work states render as meaningful topology even
  though no admission path, workstation route, invocation return, or implemented
  lifecycle behavior can reach them;
- the portable `FactoryLayoutNode.size` field exists but the graph does not
  use it as an editor behavior;
- visual groups use opaque rectangular fills and a full-surface hit target, so
  they read as blocks and can compete with graph interaction; and
- graph-specific color and shape decisions are distributed across semantic
  node components rather than expressed as one reviewable visual contract.

## Customer ask

Make the Factory graph substantially easier to scan and edit:

- when a Work-bearing node represents more than three Work items, replace the
  item-level presentation with one large total count;
- give default objects standardized colors and shapes;
- give active objects a vivid, unmistakable treatment;
- replace block-like group rendering with presentable, colorable regions that
  make contrasting areas clear;
- make workstation runtime type, scheduling behavior, and guarded control role
  legible without conflating them;
- remove or implement disconnected states in first-party Factory definitions so
  every rendered Work state represents real customer behavior;
- prevent names from being needlessly truncated; and
- let users expand or resize nodes where the node family and content warrant
  it.

## Scope

This plan applies to the canonical semantic Factory graph used by the current
activity dashboard, observe/edit modes, and package-owned replay surfaces. It
does not create or improve a parallel generic topology renderer. Shared node
presentation belongs in `ui/packages/factory-graph`; the dashboard continues
to own event streaming, Factory save mutations, editor-session control, and
confirmation flows.

The plan includes:

- semantic graph color and shape roles;
- a lossless workstation presentation projection covering runtime type,
  scheduling behavior, and guarded logical-control role;
- first-party packaged Factory topology hygiene and generated-catalog drift;
- Work-volume presentation for work-state and workstation nodes;
- content fitting, authored node resizing, undo/redo, save, and reload;
- visual-group rendering, color selection, fit-to-members, and interaction;
- observe/edit/replay parity; and
- accessibility, reduced-motion, responsive, performance, package, and browser
  evidence.

## Product decisions

These decisions remove ambiguity from implementation:

1. **The threshold is strictly greater than three.** Counts of one through
   three retain an item-level presentation. A count of four or more renders a
   large total number.
2. **The number is the total represented Work count.** It is not an overflow
   value such as `+4`, and the numeric mode does not also render the first
   three items.
3. **Node identity and runtime status are separate visual layers.** A
   workstation remains recognizably a workstation when idle, active, selected,
   or failed. Selection must not erase the runtime status treatment.
4. **Inactive nodes are quiet; active nodes are vivid.** Inactive structure
   uses neutral surfaces and stable family icons. Active or processing nodes
   use the graph active role with a stronger tinted surface, border, and glow.
5. **Color is semantic and theme-backed.** No raw hex, arbitrary CSS color, or
   persisted custom color is introduced. All graph roles map to the supported
   palette and semantic token system.
6. **Shape means card family, not decorative geometry.** The graph keeps
   readable cards with predictable handles: compact support cards, standard
   state/resource cards, and larger operational workstation cards. Icons,
   labels, and status text supplement shape and color.
7. **Saved size is portable layout metadata.** The existing
   `FactoryLayoutNode.size` value is the authored source of truth. React Flow
   width, height, measurements, and resize previews remain disposable
   projections.
8. **Groups are regions behind the graph.** The first implementation uses
   translucent, generously rounded regions with an accent outline and label
   chip. It does not require an organic hull algorithm.
9. **Group interiors do not own the canvas.** Nodes, edges, selection, pan, and
   zoom remain operable through a group. In edit mode the group label/outline
   supplies selection and drag affordances; in observe mode the region is
   non-interactive.
10. **Motion is supplementary.** Active animation is disabled by reduced-motion
    preferences while the static active surface, icon/status text, and border
    remain legible.
11. **Workstation type and behavior are independent facts.** `type` answers what
    the workstation does (`AGENT_RUN`, `SCRIPT_RUN`, `LOGICAL_MOVE`,
    `CLASSIFIER_WORKSTATION`, and the other public types). `behavior` answers how
    it is scheduled (`STANDARD`, `REPEATER`, `CRON`, or `POLLER`). The graph must
    not store or infer both through one `kind` value.
12. **Guarded logical control is derived explicitly.** A loop breaker is a
    `LOGICAL_MOVE` with a supported guard such as `VISIT_COUNT`; absence of
    worker metadata is not evidence that a node is an exhaustion rule.
13. **Unknown semantics use a neutral fallback.** A lossy historical or trace
    source may render a generic workstation with an explicit unavailable
    semantic label. It must never guess `exhaustion`, `repeater`, or another
    special role from missing fields.
14. **First-party graphs contain only behavioral states.** A packaged Factory
    state must be reachable through submission/admission, workstation routing,
    invocation return, or an implemented and directly tested lifecycle bridge.
    A state that only appears in a diagram or future design note is not part of
    the shipped graph.

## Visual contract

### Node identity and shape

| Factory object | Shape family | Inactive presentation | Content behavior |
| --- | --- | --- | --- |
| Workstation | Large operational card | Neutral raised surface, solid semantic frame, workstation icon | Stable header plus Work body; both axes are resizable |
| Work state | Standard state card | Neutral surface with lifecycle icon and state label | Width is resizable; content-fit may increase height for wrapped names |
| Resource or constraint | Standard support card | Neutral surface with stable resource/constraint icon | Width is resizable; counts remain compact badges |
| Worker or Work type | Compact support card | Neutral surface with family icon and concise type label | Width is resizable; height follows the compact family |
| Document | Standard document card | Neutral surface with document icon | Both axes are resizable so label and path can wrap |

The exact canvas-unit minimum, default, and maximum sizes must live in one
package-owned size table. The values should be chosen through the graph
Storybook fixtures so representative customer names fit without returning to
the current narrow-card proportions.

### Runtime and interaction state

| State | Required treatment |
| --- | --- |
| Idle or inactive | Quiet neutral surface and border; family identity remains visible through icon and label |
| Initial or queued | Info/waiting role, with text or icon in addition to color |
| Active or processing | Vivid graph-active surface, high-contrast border, glow, and reduced-motion-safe activity cue |
| Terminal or completed | Success surface, border, and terminal/completed icon or label |
| Failed or rejected | Danger surface, border, and failed/rejected icon or label |
| Selected | Focus/selection ring layered over the current runtime state rather than replacing it |
| Validation error | Danger validation ring/message takes precedence, without changing the node family or hiding runtime content |
| Muted by active-flow focus | Reduced emphasis that preserves readable text and accessible contrast |

`active`, `processing`, `completed`, and `failed` are visual presentation
roles, not new backend state. The projection maps existing workstation
activity, active-flow highlights, and work-state categories into these roles.

### Workstation semantic identity

Workstation presentation has three independent axes:

| Axis | Canonical source | Required graph treatment |
| --- | --- | --- |
| Runtime type | `Factory.workstations[].type` | Primary icon and role label for inference, agent, script, poller, classifier, or logical move |
| Scheduling behavior | `Factory.workstations[].behavior`, defaulting to `STANDARD` | Secondary badge, border, or concise status treatment for standard, repeater, cron, or poller scheduling |
| Guarded control role | `Factory.workstations[].guards` together with runtime type | Compact logical-control card; a supported visit-count guard renders as a loop breaker with its limit available to sighted and assistive-technology users |

The authored Factory definition is canonical for all three axes. Selected-tick
runtime topology supplies stable identity and activity overlays, not a competing
semantic taxonomy. Adapters join the runtime workstation to the authored
workstation by durable public id, falling back to the authored name only for
legacy definitions without ids.

## Architecture decision

### Canonical state and projection boundaries

```text
FactoryDefinition
  workstations[].id + type + behavior + guards
  layout.nodes[].position + layout.nodes[].size
  layout.groups[].bounds + color + nodeIds
                    |
selected-tick runtime snapshot
  active routes + executions + Work/place counts + lifecycle state
                    |
                    v
Factory graph state / pending editor layout
  pure move, resize, fit, reset, group, undo, redo operations
                    |
                    v
@you-agent-factory/factory-graph projection
  workstation identity + semantic visual state + resolved node size
  + Work presentation
                    |
                    v
React Flow nodes, edges, measurements, resize preview, and group geometry
  disposable and reproducible UI-library state
```

The source-of-truth rules are:

- the Factory definition and selected-tick runtime snapshot remain canonical
  for entity and runtime facts;
- the Factory definition owns workstation type, scheduling behavior, and guard
  configuration; runtime topology may identify the node but may not overwrite
  those authored facts with a reduced `workstation_kind` heuristic;
- the controlled editor layout draft remains canonical for unsaved position,
  size, group, and viewport changes;
- feature operations own resize, fit, reset, group fit, group move, group
  resize, and save preparation;
- the semantic package projection owns the mapping from canonical facts to
  shape, surface, status, count mode, and React Flow dimensions;
- React components render the projection and dispatch compact operations; and
- components must not reconstruct Factory or layout state from measured DOM or
  React Flow nodes when saving.

### Size precedence

The projection resolves node size in this order:

1. a valid pending/persisted `layout.nodes[].size` authored by the user;
2. a deterministic content-fit size for the node family and authored label;
3. the centralized family default.

Invalid, non-finite, non-positive, or out-of-family sizes are ignored for
rendering and removed or normalized during save preparation. Older layouts
without `size` continue to render from content-fit/default dimensions. No
Factory layout schema-version bump or new OpenAPI shape is required because
`FactoryLayoutNode.size` is already part of version 1.

Runtime Work updates must not change a node's outer dimensions. Crossing from
three to four Work items changes the body presentation, not the card geometry,
so live updates do not cause graph jitter or edge churn.

### Package and host ownership

`ui/packages/factory-graph` owns:

- the node-family shape and size vocabulary;
- the protocol-neutral workstation presentation contract and safe unknown
  fallback;
- graph visual-state roles and semantic class resolution;
- Work-volume presentation rules;
- semantic node rendering and resize affordance primitives;
- read-only group-region presentation; and
- package-level fixtures and stories for the public graph contract.

The dashboard Factory graph features own:

- pending layout state and undo/redo history;
- node resize/fit/reset and group operations;
- React Flow interaction wiring and ephemeral drag/resize previews;
- Factory save/reload and query-cache synchronization;
- localized editor controls and notices; and
- event/timeline selection and current detail-panel destinations.

## Non-goals

- Replacing Factory events, snapshots, or Factory definitions as canonical
  state.
- Changing scheduling, dispatch, Work, worker, or workstation semantics.
- Inventing a new backend workstation taxonomy or changing the existing public
  `WorkstationType` and `WorkstationKind` enums.
- Redesigning Work-relation or unrelated graph surfaces. Trace views that render
  Factory workstation nodes are in scope only for semantic parity or a neutral
  fallback; their trace-specific topology is not redesigned.
- Adding custom hex/RGB group colors or a free-form color picker.
- Building a concave hull, freehand group outline, nested group model, or
  topology-aware container node in the first release.
- Rewriting the layered layout algorithm or edge-routing model.
- Rendering every Work item inside a high-volume node.
- Persisting hover, selection, animation phase, DOM measurements, or React Flow
  internal state.
- Adding avatars, worker portraits, or other reference-image decoration that
  is not required by the customer ask.

## Work stories

### Story 1: Standardize node shapes, colors, and runtime emphasis

#### Problem statement

Customers cannot reliably distinguish stable object identity from runtime
activity because shape, base color, active flow, lifecycle, selection, and
validation styles are distributed and sometimes compete.

#### Customer ask

Give default objects standardized colors and shapes, and make active objects
vibrantly colored.

#### Solution

Publish one graph visual-state resolver and one node-family shape/size
vocabulary in the canonical Factory graph package. Render inactive cards on
quiet neutral surfaces and layer queued, active, completed, failed, selected,
validation, and muted treatments through documented precedence.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Consolidate node visual-state mapping in
  `ui/packages/factory-graph/src/semantic-node-style.ts` or a narrowly named
  package-owned visual-state module.
- Centralize node-family shape and default dimension roles currently split
  between `semantic-node-shell.tsx`, semantic node components, and
  `ui/src/features/factory-graph-editor/lib/editor/factory-graph-editor-layout.ts`.
- Update `semantic-workstation-node.tsx`, `semantic-place-nodes.tsx`,
  `semantic-support-nodes.tsx`, `semantic-doc-node.tsx`, and
  `work-state-presentation.ts` to consume the shared resolver.
- Add graph-specific semantic roles to `ui/src/styles.css` only where existing
  surface/status roles cannot express the required active strength. Map every
  new role to the existing palette foundations.
- Keep semantic icons from the existing `GraphSemanticIcon` vocabulary and
  keep typed handles in `FactoryGraphNodeShell`.

##### Contracts

- Define a package-owned, protocol-neutral visual-state value covering base
  family, lifecycle/runtime status, selection, validation, active flow, and
  muted state.
- Define and test precedence so validation and selection remain visible while
  active/completed/failed state is still recognizable.
- Preserve the current node/handle projection contract; no Factory domain
  schema is added.

##### Services

- No backend service changes. Existing runtime and Factory definition facts
  are projected into visual roles.

##### API changes

- None. Do not edit generated OpenAPI clients.

##### Tests

- Package unit tests for every node family and visual-state precedence pair.
- Component tests proving active, completed, failed, selected, validation, and
  muted states are visible through class/attribute and semantic content.
- Storybook states covering all node families across supported palette/theme
  presets and reduced motion.
- Contrast and keyboard-focus checks for active and selected nodes.

#### Acceptance criteria

- An idle graph uses a consistent quiet surface hierarchy and still identifies
  each object family through shape, icon, and label.
- Active/processing nodes are visibly stronger than idle nodes at normal and
  fit-to-view zoom through surface, border, and glow—not a subtle border alone.
- Initial/queued, active/processing, terminal/completed, and failed/rejected
  states map consistently to info/waiting, graph-active, success, and danger
  roles.
- Selection and keyboard focus remain visible on every runtime state.
- Reduced-motion users receive the same static state information without an
  infinite animation.
- Color is never the only state signal, and supported palettes meet the
  repository's WCAG 2.2 AA target.

### Story 2: Collapse high-volume Work into a large total count

#### Problem statement

Busy work-state and workstation nodes spend space on repeated dots, rows, and
overflow markers, making the graph noisy and understating the actual Work
volume.

#### Customer ask

When Work is beyond three items, make the graph node body a large number.

#### Solution

Create one pure Work-volume presentation rule shared by work-state and
workstation node renderers. Counts one through three keep the useful item-level
view. Counts of four or more replace the entire Work body with a visually
dominant total count while preserving the node header, identity, status,
handles, selection, and detail navigation.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Replace the current `DOT_LIMIT = 10` behavior in
  `ui/packages/factory-graph/src/semantic-place-nodes.tsx` with the shared
  threshold contract.
- Replace the current workstation `VISIBLE_WORK_ITEM_LIMIT` plus `+N`/dots
  overflow in `semantic-workstation-node.tsx` with the same presentation rule.
- Add a reusable large numeric Work marker owned by the Factory graph package;
  do not introduce a dashboard-only duplicate.
- Keep total-count and detail-action copy in the owning graph message catalog
  for every supported locale.
- Keep resource/constraint token badges out of this rule unless they represent
  customer Work; resource capacity is not Work volume.

##### Contracts

- Define a pure presentation result for `empty`, `items`, and `total` modes.
- Count unique represented Work IDs for workstation activity. Work-state nodes
  use the canonical represented Work/place count already supplied by the
  runtime projection.
- The accessible label must state the total, such as `12 active items`, even
  when the visual body is only `12`.

##### Services

- No backend changes. The rule consumes existing selected-tick runtime counts
  and active executions.

##### API changes

- None.

##### Tests

- Pure boundary tests for counts 0, 1, 2, 3, 4, and a large count.
- Component tests for work-state and workstation nodes proving that four does
  not render three items plus an overflow marker.
- Tests proving the visible number and accessible label contain the total,
  not the remainder.
- Tests proving the large-count body still activates the owning node/detail
  action and leaves semantic handles intact.

#### Acceptance criteria

- Counts one through three render one through three item indicators or rows.
- A count of four or more renders one large total number and no item rows,
  dots, avatar-like markers, or `+N` suffix.
- The Work node's name/icon/header remains visible so the number never loses
  its Factory context.
- Screen readers receive the total count and Work status in meaningful text.
- Moving between three and four items during live execution does not change
  the outer node dimensions or move connected edges.

### Story 3: Fit long names and persist accessible node resizing

#### Problem statement

Fixed narrow node dimensions force names and document paths into truncation,
and users cannot use the existing portable node-size contract to create a
layout appropriate for their Factory.

#### Customer ask

Allow graph nodes to expand or resize as appropriate so authored names are
readable.

#### Solution

Resolve a content-fit default for each node family, wrap labels safely, and
add edit-mode resize, fit-to-content, and reset-size operations. Persist the
result through the existing `layout.nodes[].size` field, include it in
undo/redo and save/reload, and keep React Flow measurement changes as preview
state only.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Add a pure package-owned node-size resolver with per-family minimum, default,
  and maximum bounds.
- Update semantic title/label components to use wrapping and
  `overflow-wrap:anywhere` where appropriate instead of unconditional
  single-line `truncate`.
- Provide a reusable selected-node resize affordance that can be enabled by the
  edit-mode host without making the package own editor state.
- Refresh node internals after committed size changes so every edge endpoint
  remains attached to a handle rendered by the resized node.

##### Contracts

- Reuse the existing OpenAPI-generated `FactoryLayoutNode.size` contract as
  the only durable node-size value.
- Add pure layout operations for resize, fit-to-content, and reset-size under
  `ui/src/features/factory-graph-editor/lib/layout/`.
- Extend layout history commands so each committed size operation has one
  inverse and participates in undo/redo.
- Extend frontend layout validation/save preparation to reject or normalize
  non-finite, non-positive, and out-of-family dimensions before projection or
  persistence.
- Resolve dimensions from pending layout state in
  `current-activity-factory-graph-layout.ts` and the React Flow projection;
  do not recover authored size from React Flow measurements during save.

##### Services

- The existing Factory definition save operation remains authoritative for
  persisted layout metadata. No new service is introduced.

##### API changes

- No schema-shape change. `FactoryLayoutNode.size` already exists in
  `api/components/schemas/data-models/FactoryLayoutNode.yaml` and generated Go
  and TypeScript contracts.
- If implementation discovers contract drift rather than merely consuming the
  existing field, author the correction in `api/components/` and run
  `make generate-api`; generated files must not be hand-edited.

##### Tests

- Pure tests for size precedence, per-family clamping, fit, reset, and invalid
  saved values.
- Layout history tests for resize undo/redo and for preserving unrelated node
  position, group membership, and metadata.
- Projection tests proving resolved width/height become the React Flow node's
  width, height, initial size, and measurement contract.
- Component tests for pointer resize plus keyboard-accessible `Fit to content`
  and `Reset size` actions.
- Save/reload tests proving size round-trips through the existing Factory
  layout without topology changes.
- Long-label stories for ordinary words, long unbroken identifiers, localized
  labels, and document paths.

#### Acceptance criteria

- Representative workstation, Work-state, resource, worker, Work-type, and
  document names are readable at their fitted/default size without the current
  unconditional ellipsis.
- A selected node in edit mode exposes pointer resize handles only on the axes
  allowed by its family: workstation/document cards support both axes, while
  compact support and state/resource cards prioritize horizontal sizing and
  content-fit height.
- Keyboard users can fit and reset a selected node without operating a pointer
  drag handle.
- Resizing is clamped, does not invert or collapse a node, updates edge handles,
  and is undoable/redoable as one editor command.
- Save, reload, observe mode, and replay use the authored size. Legacy layouts
  without size use deterministic fitted/default dimensions.
- Runtime status or Work-count updates do not overwrite an authored size.

### Story 4: Replace block groups with colored, non-blocking graph regions

#### Problem statement

Authored visual groups currently render as opaque rectangular blocks with an
interactive full surface. They compete with node/edge contrast and can obstruct
normal canvas gestures instead of helping users understand related areas.

#### Customer ask

Make grouping visually presentable and colorable so contrasting parts of the
Factory are obvious without requiring organic blob geometry.

#### Solution

Render saved groups as low-opacity, well-rounded regions behind nodes and
edges, with a semantic accent outline/glow and a floating label chip. Make the
interior click-through. In edit mode use the label and outline/resize affordance
for selection and movement, support create-from-selection and fit-to-members,
and persist the existing bounds/color/member contract.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Add or move a read-only semantic group-region presentation into
  `ui/packages/factory-graph` so observe, edit, and replay do not invent
  separate group visuals.
- Refactor
  `factory-graph-visual-group-layer.tsx` to render group background beneath
  nodes/edges and separate decorative surface from the edit hit targets.
- Update `factory-graph-visual-group-controls.tsx` with `Fit to members` and
  keyboard-operable size/fit behavior using existing shared action controls.
- Add the new fit action, semantic color names, tooltips, and accessible labels
  to `ui/src/features/factory-graph-editor/messages/editor.ts` rather than
  hardcoding production copy in the group components.
- Keep group label, color, membership, delete, move, and resize behavior in the
  existing visual-group editor hook and pure layout operation boundary.

##### Contracts

- Continue using `layout.groups[].bounds`, `color`, and `nodeIds`; groups remain
  presentation metadata, not topology/container nodes.
- Standardize newly authored colors as neutral, primary, info, success,
  warning, and danger semantic choices. Preserve the existing `outline` value
  as a neutral read alias, and render unsupported legacy strings through a safe
  neutral fallback rather than raw CSS.
- Add a pure `fit group to members` operation that computes bounds from resolved
  node sizes plus standardized padding.
- When nodes are selected, `Create group` initializes membership and fitted
  bounds from that selection. With no selected nodes, it retains the current
  empty-at-viewport-center behavior.

##### Services

- No backend service changes. Existing Factory layout save/reload behavior
  remains the persistence boundary.

##### API changes

- None. `FactoryLayoutGroup.color`, `bounds`, and `nodeIds` already support the
  behavior. Do not add a topology relationship or custom-color field.

##### Tests

- Pure operation tests for fit-to-members with mixed node sizes, padding,
  empty membership, missing legacy members, move, resize, and undo/redo.
- Component tests proving the group interior is click-through while the label
  and resize affordances are keyboard/pointer operable in edit mode.
- Component tests proving observe-mode regions are non-interactive and render
  beneath nodes and edges.
- Palette tests for every supported semantic group color, `outline` alias, and
  unsupported legacy fallback.
- Extend the existing visual-group Storybook browser save check to cover
  create-from-selection, fit, recolor, save, reload, and ordinary node/canvas
  interaction through the region.

#### Acceptance criteria

- A saved group reads as a subtle colored region with a visible label and
  accent boundary, not as an opaque card or panel.
- Nodes, handles, edges, edge labels, selection boxes, pan, and zoom remain
  usable inside the group region.
- Saved groups are visible in observe and replay modes; edit-only controls are
  not.
- Users can select/move a group through its label/outline, resize it with a
  pointer, and fit it to members through a keyboard-operable control.
- Creating a group from selected nodes produces fitted bounds and membership
  in one action.
- Every newly authored group color uses a semantic token, survives save/reload,
  and remains distinguishable through label/border as well as fill color.

### Story 5: Preserve the new graph UX across live updates and host surfaces

#### Problem statement

A graph improvement is incomplete if observe/edit mode, historical replay,
save/reload, package consumers, or high-volume live updates fall back to
different renderers or visibly unstable geometry.

#### Customer ask

Keep the improved graph understandable while the Factory runs and wherever the
canonical graph is embedded.

#### Solution

Wire every canonical host through the same package-owned visual-state,
Work-volume, size, and read-only group presentation. Preserve node identity,
viewport, selection, and authored geometry while runtime overlays update, and
prove the result against the existing large-editor fixtures.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Update `FactoryGraphReplaySurface`, current-activity, editor, and applicable
  trace adapters to consume the same package visual contracts rather than
  copying style/count or workstation-taxonomy logic.
- Keep one semantic node registry and one set of handles across observe, edit,
  and replay.
- Add meaningful package/website Storybook states for inactive, active,
  completed, failed, 1/3/4/large Work counts, long labels, authored sizes, and
  colored groups, plus every workstation type/behavior/control-role family.
- Extend renderer-path boundary checks if needed to prevent a dashboard-only
  fallback implementation.

##### Contracts

- Runtime overlays may change status/count content but may not mutate authored
  node positions, sizes, group bounds, or group membership.
- Switching observe/edit mode or selected timeline tick preserves stable
  semantic node IDs and handle IDs.

##### Services

- No service changes.

##### API changes

- None.

##### Tests

- Package-consumer tests proving the public Factory graph package renders the
  same node state/count/size/group contract as the dashboard.
- Current-activity graph semantic tests for live status transitions without
  remount, geometry loss, or missing handles.
- Existing large-editor performance tests extended to cover numeric Work mode,
  authored mixed sizes, and multiple translucent groups.
- Browser checks at mobile, tablet, and desktop sizes for pan/zoom, selection,
  resize, group interaction, focus visibility, and reduced motion.
- Packed-package verification to catch reliance on dashboard aliases or
  unshipped styles.

#### Acceptance criteria

- Observe, edit, selected-tick replay, and the public package use the same
  semantic node, workstation identity, state, count, size, and group
  presentation.
- Entering or leaving edit mode does not replace the graph renderer or lose
  viewport, selection, authored size, or group display.
- Live transitions between idle, active, completed, and failed update visual
  state without moving unaffected nodes or disconnecting edges.
- Large graphs remain within the existing repository performance budgets and
  do not allocate one rendered marker per Work item once the count is above
  three.
- The package builds and works from a packed clean consumer, not only through
  dashboard source aliases.

### Story 6: Preserve workstation type, behavior, and guarded-control meaning

#### Problem statement

Customers cannot reliably tell what a workstation does or how it is scheduled.
The current dashboard and package contracts overload `workstation_kind` with
scheduling behavior, discard authored workstation `type`, and use missing
worker/kind fields as an exhaustion heuristic. In the built-in goal Factory this
makes the guarded `goal-loop-breaker` look like a large ordinary workstation,
while replay sources can mislabel ordinary workstations as exhaustion rules.

#### Customer ask

Give every Factory graph enough workstation-kind behavior to distinguish
execution, classification, logical routing, guarded loop breaking, repeating,
cron, and polling wherever that graph is rendered.

#### Solution

Create one package-owned workstation semantic projection with independent
runtime-type, scheduling-behavior, and guard-role fields. Build it from the full
Factory definition and merge runtime activity by stable workstation identity.
Use runtime type as the primary role, scheduling behavior as a secondary
treatment, and supported guard configuration to derive compact control nodes.
Remove every inference that turns absent metadata into a special role.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Replace the overloaded semantic input in
  `ui/packages/factory-graph/src/semantic-workstation-presentation.ts` with a
  protocol-neutral value that carries authored workstation type, scheduling
  behavior, and derived guarded-control role separately. Retain a temporary
  read adapter for old `workstation_kind` package inputs only if a packed
  consumer requires it; new projections must not write the compatibility shape.
- Add primary icon/role presentations for the canonical workstation families:
  inference run, agent run, script run, poller run, classifier, logical move,
  and unknown. Keep legacy `MODEL_INVOKE` and `MODEL_WORKSTATION` values mapped
  through the repository's existing taxonomy helpers rather than creating a
  second enum table.
- Render scheduling behavior as a secondary badge, border, or concise label so
  `REPEATER`, `CRON`, and `POLLER` remain visible without replacing the primary
  runtime-type identity.
- Render guarded logical moves as compact control cards. A `VISIT_COUNT` guard
  names the guarded workstation and visit limit in accessible text; an ordinary
  `LOGICAL_MOVE` remains a logical router and is not styled as failure.
- Make the unknown fallback explicit and neutral. Delete the heuristic in which
  missing workstation kind plus missing worker type means `exhaustion`.
- Ensure the semantic header cooperates with the Story 3 size and wrapping
  resolver so a role badge never forces `execute-goal`, `goal-loop-breaker`, or
  longer localized names back into ellipsis-only presentation.

##### Contracts

- Extend the package's `FactoryGraphWorkstationRef` replacement with separate
  `workstationType`, `schedulingBehavior`, and `controlRole` semantics. The
  exact names may follow package conventions, but a single `kind` string must
  not represent more than one axis.
- Project these fields from `Factory.workstations[]`. Join selected-tick runtime
  topology/activity using authored workstation `id`; use `name` only as the
  documented legacy fallback when the definition has no id.
- Update current-activity, editor, replay/emulator, and trace workstation-node
  adapters that use the shared semantic node. A trace source that cannot recover
  the Factory definition supplies `unknown`, not a fabricated standard or
  exhaustion role.
- Keep React Flow node data disposable. Components render the semantic
  projection and never recover authored type/behavior/guards from icon names,
  CSS classes, or DOM measurements.

##### Services

- No runtime scheduling or service mutation changes are required. The
  event-computed `snapshot.factory` and package `FactoryGraphSource.factory`
  already carry the authored definition used by current activity and replay.
- Do not extend the backend world-view contract merely to duplicate full
  Factory-definition data. If a user-visible topology-only host is discovered
  that cannot receive the definition, document that boundary and use the
  neutral fallback in this lane.

##### API changes

- None. Reuse the existing generated `WorkstationType`, `WorkstationKind`,
  `Workstation.type`, `Workstation.behavior`, and `Workstation.guards`
  contracts. Do not hand-edit generated clients.

##### Tests

- Pure package tests for every supported runtime type crossed with `STANDARD`,
  `REPEATER`, `CRON`, and `POLLER`, including lowercase legacy input,
  undefined values, and unknown future strings.
- Regression tests proving only a guarded `LOGICAL_MOVE` becomes a loop breaker
  and missing metadata never becomes exhaustion.
- Current-activity projection/component tests using the authored goal Factory
  to prove `execute-goal` is an agent repeater and `goal-loop-breaker` is a
  compact guarded logical control with full accessible names.
- Replay package tests proving semantic metadata is resolved from
  `FactoryGraphSource.factory`, and trace tests proving a lossy source uses the
  neutral fallback.
- Storybook/browser coverage at fit-to-view zoom for a mixed Factory containing
  classifier, logical move, guarded loop breaker, inference, agent, script,
  repeater, cron, and poller workstations.

#### Acceptance criteria

- Every user-visible Factory workstation node communicates runtime type through
  an icon plus text or accessible label, and scheduling behavior through a
  distinct secondary treatment.
- `execute-goal` is visibly and accessibly both an agent-run workstation and a
  repeater; neither fact overwrites the other.
- `goal-loop-breaker` renders as a compact guarded logical-control node and
  exposes its `VISIT_COUNT` target and limit without requiring selection.
- Ordinary logical moves and classifiers are not presented as executable agent
  workstations, and missing metadata never produces an exhaustion/loop-breaker
  presentation.
- Current activity observe/edit, selected-tick replay, the public package, and
  every trace view using the shared workstation node either agree on semantics
  or show the documented neutral fallback.
- No backend scheduling behavior, public enum, or OpenAPI shape changes.

### Story 7: Remove disconnected Work states from first-party graphs

#### Problem statement

The semantic graph faithfully renders every authored Work state, including
states that no submitted Work, workstation route, invocation return, or
implemented lifecycle bridge can reach. These nodes look operational and add
edges/layout pressure even though they do not describe runtime behavior. The
baseline audit found `goal:execute`; `loop-controller:stopped` and
`scheduled-execution:skipped`; and `task:complete` plus `quorum-merge:init` in
the quorum Factory as disconnected candidates requiring reconciliation.

#### Customer ask

Remove redundant operational states from the goal graph and apply the same
hygiene rule to every first-party packaged Factory so rendered topology stays
minimal and truthful.

#### Solution

Audit each authored packaged Factory through its parsed canonical definition.
For every declared Work state, prove one concrete behavioral role: external
admission, a workstation input or output route, an explicit invocation return,
or a real lifecycle bridge backed by implementation and tests. Connect a state
when the shipped product already promises that behavior; otherwise remove the
state and update docs/fixtures. Add a semantic catalog check so disconnected
states cannot silently return.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\factory-graph-visual-ux.md`

#### Changes

##### Package changes

- Remove the unreferenced `goal:execute` state from
  `packages/packaged-factories/factories/goal/factory.yaml`; the actual repeater
  path remains `goal:init -> execute-goal -> goal:init|complete|blocked|failed`.
- Reconcile every other disconnected candidate found by the catalog audit. A
  promised loop stop/skip or quorum completion state is connected only when an
  existing runtime operation already produces it and direct behavioral tests
  prove that promise; otherwise delete it instead of leaving speculative graph
  topology.
- Regenerate `packages/packaged-factories/generated/` and its manifest through
  `make packaged-factory-catalog-generate`; never edit generated catalog files
  by hand.
- Update `docs/reference/authoring-factories.md`, the goal docs smoke markers,
  and affected packaged-family diagrams or examples so documentation lists only
  the shipped state paths.

##### Contracts

- Define a catalog-only graph-state hygiene rule over canonical parsed Factory
  definitions. A state is accounted for when it is an admitted initial state,
  appears in workstation `inputs`, `outputs`, `onContinue`, `onRejection`,
  `onFailure`, or `classificationRoutes[].outputs`, is selected by
  `invocationReturn`, or is reached by a named implemented lifecycle bridge.
- Do not make this a blanket public validation error for customer Factories in
  this lane. First-party catalog minimality is stricter than the general
  authoring contract, and JavaScript or externally controlled Factory models
  may require a separate reachability policy.
- The check reports the Factory slug and exact `workType:state` for each
  disconnected state. Any lifecycle exception must point to a direct behavioral
  test; comment-only allowlists are not accepted.

##### Services

- No service changes are expected for removing states that are proven
  unreachable.
- If the audit shows a documented first-party lifecycle state is supposed to be
  produced but currently is not, stop that item from riding as graph cleanup and
  plan the missing runtime behavior as a separate vertical product story. Do
  not fake reachability in the projection or hide the state only in the UI.

##### API changes

- No schema shape changes. Generated packaged Factory artifacts and hashes must
  be refreshed from authored sources.

##### Tests

- A focused catalog test over every
  `packages/packaged-factories/factories/*/factory.yaml` that fails with exact
  disconnected-state diagnostics.
- Goal definition and docs tests proving `goal:execute` is absent while
  `goal:init`, `goal:complete`, `goal:blocked`, and `goal:failed` remain and the
  repeater routes are unchanged.
- Existing invocation/runtime smoke coverage for each changed packaged Factory
  to prove accepted, continued, blocked, failed, and explicit-return behavior is
  unchanged.
- Generated catalog drift and package-consumer checks proving authored YAML,
  flattened JSON/YAML, manifest hashes, embedded Go assets, and published npm
  artifacts agree.
- A goal-Factory graph projection or browser fixture asserting that no
  `work-state:goal:execute` node is rendered after the catalog update.

#### Acceptance criteria

- The built-in goal graph contains only `goal:init`, `goal:complete`,
  `goal:blocked`, and `goal:failed`; the repeater and loop-breaker routes still
  produce the same accepted, continue, blocked, and failed outcomes.
- Every state in every first-party graph Factory has a machine-checked,
  implemented behavioral role; the catalog check names and rejects a newly
  introduced disconnected state.
- State cleanup happens in the canonical authored Factory definition, not in a
  UI visibility filter, so dashboard, editor, replay, CLI inspection, and
  published package consumers agree.
- Authored definitions, generated catalog artifacts, public reference docs, and
  smoke expectations contain the same state inventory.
- No unrelated runtime lifecycle feature is implemented as part of topology
  cleanup.

## Project-level acceptance criteria

- Every Factory object family follows the documented shape and inactive color
  grammar, with no parallel dashboard-only styling table.
- Active/processing state is vivid and accessible across supported palettes;
  completed and failed state remain semantically distinct.
- Work-bearing nodes use item mode for one through three and a large total
  number for four or more.
- Long names can be fitted and nodes can be resized within family constraints;
  size survives undo/redo, save, reload, observe mode, and replay.
- Workstation runtime type, scheduling behavior, and guarded-control role remain
  separate through canonical definition, package projection, and rendering;
  missing data uses a neutral fallback.
- The goal graph identifies `execute-goal` as an agent repeater, renders
  `goal-loop-breaker` as compact guarded logical control, and contains no
  disconnected `goal:execute` state.
- Every first-party graph Factory passes the parsed disconnected-state catalog
  check, and generated artifacts/docs match the cleaned authored definitions.
- Groups render as translucent labeled regions behind the graph, remain
  colorable with semantic tokens, and do not block node, edge, pan, zoom, or
  selection interaction.
- Canonical Factory/runtime/layout state, pure editor operations, package
  projection, and React Flow component wiring remain distinct and directly
  testable.
- No new OpenAPI shape, backend runtime state, alternate renderer, or unrelated
  graph redesign is introduced.
- Localization, semantic-color, Tailwind-token, accessibility, reduced-motion,
  responsive, package-boundary, and performance checks pass.
- Delivery continues through required CI until it is terminal and passing;
  blocking review feedback is addressed, conflicts are resolved, generated or
  package drift is reconciled, and the pull request is actually merged. An open
  or approved PR is not completion.

## Verification plan

Use focused package, operation, projection, and component tests within each
story. Before merge, run the applicable final gates:

```text
cd ui && bun run --cwd packages/factory-graph verify
cd ui && bun run check
cd ui && bun run tsc
cd ui && bun run test:unit
cd ui && bun run test:component
cd ui && bun run test:integration
cd ui && bun run test:performance
cd ui && bun run test-storybook
cd ui && bun run storybook:responsive-check
cd ui && bun run storybook:factory-graph-visual-group-save-check
cd ui && bun run verify:public-packages
make packaged-factory-catalog-generate
make packaged-factory-catalog-check
make packaged-factory-package-verify
make docs-reference-smoke
make verify-fast
make verify-pr
```

Add one focused Factory graph visual-UX browser check if the existing visual
group and responsive checks cannot directly prove:

- the 3-to-4 Work threshold;
- vivid active state with reduced-motion fallback;
- long-name fit and pointer/keyboard resize;
- workstation type/behavior/guard-role parity and the unknown fallback;
- the cleaned goal graph without `goal:execute`;
- resize save/reload;
- group click-through, fit, recolor, and save/reload; and
- observe/edit continuity at desktop and touch breakpoints.

Visual QA must use representative large Factory fixtures rather than only
isolated nodes. Capture at least idle, mixed-runtime-state, dense-Work,
long-label, resized-layout, and multi-group states across the supported palette
presets. Reviewers should compare information hierarchy and interaction, not
pixel-match the supplied concept image.

## Delivery order

1. Clean the authored goal topology and land the first-party disconnected-state
   catalog check so visual fixtures start from truthful canonical definitions.
2. Land the package-owned workstation type/behavior/control-role contract and
   remove missing-metadata special-role inference.
3. Land the shared visual-state and node-family contract before changing every
   semantic node.
4. Land Work-volume compaction on the shared package contract.
5. Land size resolution and label wrapping before exposing resize controls.
6. Land resize operations, history, component wiring, and save/reload as one
   vertical behavior slice.
7. Land read-only group-region presentation before edit-only group controls.
8. Land create-from-selection, fit-to-members, semantic colors, and group
   persistence.
9. Complete observe/edit/replay/trace parity, large-graph performance,
   responsive and accessibility evidence, packaged-factory/catalog/docs drift
   checks, public-package verification, blocking review resolution, terminal
   green CI, conflict resolution, and merge.

## Definition of done

The work is complete when a customer can open a large Factory graph, understand
which workstation type runs and how it is scheduled, distinguish guarded
logical controls from executable workstations, trust that every rendered Work
state is part of real shipped behavior, understand inactive versus
active/completed/failed structure at a glance, see four or more Work items as a
large total, read or fit long names, resize appropriate nodes and retain those
sizes, and use colored groups that visually organize the canvas without
obstructing it. The behavior must come from cleaned canonical Factory
definitions, the canonical Factory graph package, and the controlled editor
layout path; survive save/reload and replay; pass the listed quality gates; and
be merged.
