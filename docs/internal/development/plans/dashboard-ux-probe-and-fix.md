---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/dashboard-ux-probe-and-fix
---

# Dashboard UX Probe And Fix Plan

## Outcome

Customers can open or create a Factory, understand the active Factory Session,
inspect every concurrent Work and Worker Session, and use the Factory graph and
dashboard widgets without duplicated labels, clipped feedback, phantom tabs,
collapsed executions, misleading selection, or inaccessible controls.

This plan turns the current UX defects into independently reviewable behavior
stories. It is intentionally separate from
`docs/internal/development/plans/factory-graph-visual-ux.md`, which remains the
owner for the graph's visual grammar, node sizing, Work-count presentation,
group-region styling, workstation semantic identity, and first-party topology
hygiene.

## Customer problem

The dashboard currently exposes several kinds of misleading state:

- the Work outcome chart renders its Y-axis title twice;
- Submit Work feedback is narrower than its card and some validation feedback
  is duplicated;
- the header can show a real Factory Session and a second `~default` alias as
  if they were separate Factories;
- workstation activity can collapse multiple concurrent dispatches to one;
- Current Selection infers executions from Provider Sessions instead of
  enumerating authoritative Worker Sessions;
- the Worker Session event stream can expose Factory-event fallback records
  without making the provenance difference explicit;
- workstation Request history shows only the first Work in a multi-Work
  dispatch and suppresses executions without Provider Sessions;
- editor selection, undo, dragging, connection handles, and mobile toolbar
  behavior do not form one consistent operation model;
- persisted dashboard geometry accepts unsafe values and the 640–768 px layout
  is neither compact nor editable; and
- New Factory is hidden behind a failed-looking validation path.

## Probe evidence

The audit used an initial ten focused probes followed by thirty additional
bento/functionality probes, run in staged parallel batches because the
workspace supports four concurrent agents including the coordinator. The
initial audit included a live desktop inspection against the running backend;
the thirty-probe expansion used read-only source, contract, projection, and
focused-test inspection because the live browser was unavailable during that
batch.

The complete additional matrix and its execution limitations are recorded in
`docs/internal/development/plans/dashboard-ux-30-probe-results.md`.

| Priority | Finding | Direct evidence |
| --- | --- | --- |
| P1 | Duplicate Y-axis title | `work-chart.tsx` renders both a chart overlay label and a Recharts `YAxis` label; the live accessibility tree and geometry contained two `Work count` elements. |
| P1 | Submit feedback does not fill its card | `submit-work-status-panel.tsx` uses `max-w-xl`; at the measured desktop layout the parent was 589 px while the panel stopped at 576 px. |
| P1 | Phantom default session tab | The live header rendered a resolved UUID-backed session and a second `~default` entry. `session_listing.go` appends two live readers without identity reconciliation, and the UI preserves every row. |
| P1 | Concurrent workstation activity collapses | `projectRuntime.ts` builds `workstation_activity_by_node_id` with `Object.fromEntries` keyed by `transitionID`, so later dispatches replace earlier dispatches on the same workstation. |
| P1 | Worker Sessions are absent from the UI | The OpenAPI list/show/stream/transcript contract exists, but there is no handwritten Worker Sessions API module, hook, projection, or component consumer. |
| P1 | Durable Worker Session association is discarded | The generated event exists, but `ui/src/api/events/types.ts` and `replayWorldState.ts` omit `DISPATCH_WORKER_SESSION_ASSOCIATION`. |
| P1 | Worker Session stream provenance is misleading | The runtime-bound route can prefer dispatch-scoped Factory ledger records and label them `factory_event` instead of streaming the source-native Worker Sessions Events topic promised by the route. |
| P1 | Workstation history loses Work and attempts | `workstation-request-history-section.tsx` reads only `work_items[0]`; request presence suppresses fallback run history; attempts without Provider Session identity are filtered out. |
| P1 | Editor state is inconsistent | Undo/Redo only covers layout, observer selection can look selected while Delete reports no editor selection, and `nodesDraggable` is enabled outside edit mode. |
| P1 | Persisted bento geometry is unsafe | The layout normalizer accepts negative coordinates, non-positive dimensions, out-of-grid widths, overlap, and duplicate custom IDs. |
| P1 | Responsive/accessibility relationships are false or clipped | Session tabs control an empty `tabpanel`; the mobile editor toolbar clips trailing Save/Discard actions; 640–768 px keeps desktop geometry while disabling layout interaction. |
| P1 | Submit Work contract and client behavior diverge | OpenAPI describes `name` as optional while server and dashboard reject missing/blank names; same-tick duplicate activation is not synchronously guarded. |
| P1 | Drill-down context can combine unrelated identities | A directly selected trace is global bento state and can survive a Work change; direct request detail drops Provider Session selection callbacks; terminal rows use display labels instead of Work identity. |
| P1 | Graph projection failures are not safely contained | Replay does not validate edge endpoints or expose an error state, while current/trace React Flow handlers throw every warning as fatal without a card-local boundary. |
| P1 | Dense graph projections churn or scale quadratically | Package replay repeatedly scans occupancy/count/overlay arrays per node/edge, and live clock updates rebuild stable current-activity nodes and edges. |
| P0 | Runtime caches and status can cross Factory Session boundaries | Trace query identity omits Factory Session/stream generation, session changes can render the prior active snapshot for one frame, and late EventSource callbacks can overwrite the new session's global status. |
| P0 | Grouping is not an end-to-end editor feature | Groups disappear outside edit mode, opaque group bodies block graph interaction, reassignment undo loses prior membership, invalid geometry is accepted, and keyboard/cancel semantics are absent. |
| P0 | Duplicate Factory graph cards race singleton editor state | The catalog permits duplicate graph widgets although every instance mounts the same session-scoped editor/controller bridge; removing one can clear another's handlers. |
| P1 | Dashboard async state is neither shared nor truthful | Known-empty, retained-stale, reconnecting, recovery-failed, and historical states are unavailable or inconsistently inferred by individual cards. |
| P1 | Inspection surfaces are unbounded | Terminal Work, Current Selection histories, Provider transcripts, Trace graphs/tables, and payload/relationship views can mount all retained data without pagination or virtualization. |
| P1 | Advertised locale and accessibility contracts are incomplete | Japanese and Korean frequently fall back to English, many controls are below 44 px, layout editing is pointer-only, and no general per-widget error boundary exists. |

### Audit side effect

While probing the Submit Work validation path, one audit interaction created a
local `idea` Work request named `UX layout probe` with an empty optional text
item. No cleanup is authorized by this plan; retain or remove that Work only by
explicit operator choice.

## Scope

In scope:

- Factory Session listing, identity reconciliation, singleton header context,
  and explicit Open/New Factory flows;
- Work outcome chart and Submit Work feedback/validation behavior;
- selected-workstation concurrent activity, Work/Request enumeration, Worker
  Session association, list, live updates, and optional Provider Session links;
- graph editor operation history, selection semantics, observe-mode geometry,
  connection workflow, and responsive toolbar behavior;
- bento layout validation, minimum geometry, compact projection, duplicate
  instance identity, keyboard manipulation, and persistence scope;
- truthful tab semantics, stream status, reduced motion, touch targets, and
  narrow trace visualization behavior; and
- regression fixtures and browser evidence tying these surfaces together.

Out of scope:

- redesigning the graph visual grammar already owned by
  `factory-graph-visual-ux.md`;
- replacing Factory Events, Factory definitions, Work, or Worker Sessions as
  canonical state;
- persisting React Flow or React Grid Layout objects as domain state;
- broad dashboard restyling, unrelated copy sweeps, or new backend topology;
- treating Provider Sessions as Worker Session identity; and
- making process-local Worker Events durable Factory replay history.

## Architecture and state boundaries

```text
Factory definitions + Factory Sessions + Work + Worker Sessions
        | canonical service operations and authored OpenAPI
        v
Recordings Factory Events       Worker Sessions -> Events topics
durable replay/history          source-native retained/live observations
        |                                  |
        +--------------+-------------------+
                       v
typed UI API modules + event stores
                       |
                       v
pure replay/activity/selection/editor/layout projections
                       |
                       v
React Flow, Recharts, React Grid Layout, tables, cards, and dialogs
disposable UI-library state; never the domain source of truth
```

Required ownership decisions:

- Factory Sessions owns reconciliation of live session identities. `~default`
  remains an accepted selector, not a second durable client identity.
- Recordings owns durable Factory Event replay, including the dispatch-to-Worker
  Session association.
- Worker Sessions owns Worker Session identity and lifecycle; Events owns its
  bounded source-native observation topic; Provider Sessions is optional detail.
- Work owns Work and Work Request admission. Dashboard workstation requests are
  dispatch-keyed read projections, not Work Request identity.
- The Factory document and graph editor operation stream own editable graph
  state. React Flow nodes, handles, edges, and measurements are reproducible
  projections.
- The bento layout store owns browser-local layout preference. React Grid Layout
  and the compact one-column form are projections.

## Delivery sequence

1. Land the deterministic regression fixture and probe matrix.
2. Land narrow presentation and contract corrections with no dependency on the
   Worker Session work.
3. Correct canonical session/activity/association projections.
4. Expose and render authoritative Worker Sessions and complete workstation
   history.
5. Repair editor and bento operation models after their canonical boundaries
   are covered by pure tests.
6. Bound and isolate inspection surfaces, then complete locale coverage.
7. Complete responsive, accessibility, browser, generated-contract, CI, review,
   conflict-resolution, and merge gates.

## Work stories

### Story 0: Establish the deterministic UX regression probe

#### Problem statement

The reported defects span live events, replay, concurrent dispatch, multiple
identity types, graph-library behavior, and responsive layouts; isolated happy
path fixtures cannot prove the fixes.

#### Customer ask

Create one repeatable probe that reproduces the known problems and provides a
stable baseline for every fix story.

#### Solution

Add a fixture-driven browser scenario containing duplicate default-session
inputs, two concurrent dispatches on one workstation, a multi-Work dispatch,
multiple Worker Sessions including one without a Provider Session, terminal and
failed/canceled executions, a large grouped graph, dense inspector data, every
canonical dashboard widget, and a permitted duplicate instance of each
duplicate-capable widget.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Add the replay/browser fixture under the existing `ui/integration/` fixture
  conventions and reuse feature-local test builders for pure projections.
- Do not add production-only test seams or a second replay engine.

##### Contracts

- Define expected identities by `factorySessionId`, `dispatch_id`, `work_id`,
  and `workerSessionId`; do not key expected behavior by display text.

##### Services

- Reuse public service operations and existing edge mocks; no new service.

##### API changes

- None.

##### Tests

- Probe 320, 390, 640, 768, and desktop widths; 200% zoom; keyboard and touch;
  forced colors and reduced motion; en/zh-CN/ja/ko and pseudo-long copy; live,
  historical, reconnect, recovery failure, known-empty, and session-switch
  races; deliberate single-widget throw; dense-data and rerender budgets.

#### Acceptance criteria

- The fixture reproduces the duplicate chart label, capped Submit status,
  phantom session entry, collapsed concurrent activity, broken group journey,
  incomplete history, stale cross-session state, unsafe duplicate graph card,
  and Worker Session gap before fixes.
- Expected row counts, stable ordering, navigation targets, accessible names,
  and document-level overflow are asserted directly.
- The fixture is deterministic in CI and uses no sleeps as its primary
  synchronization strategy.

### Story 1: Render one Work outcome axis and one plot grid

#### Problem statement

The Work outcome chart repeats `Work count` and renders redundant grid layers.

#### Customer ask

Make the chart visually and accessibly unambiguous.

#### Solution

Choose one Y-axis-title owner and one grid contract inside the canonical Work
chart component; keep chart data and series behavior unchanged.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update `ui/src/features/work-outcome/components/work-chart/work-chart.tsx`
  and its component/Storybook contracts.

##### Contracts

- Recharts remains a disposable rendering projection; no outcome model change.

##### Services

- None.

##### API changes

- None.

##### Tests

- Component assertion for exactly one visible/accessibility-tree Y-axis title
  and one intended grid; browser geometry at narrow and desktop card sizes.

#### Acceptance criteria

- `Work count` is presented exactly once without clipping ticks or legend.
- The chart remains readable at compact and desktop widths, with all series
  toggle and zoom behavior unchanged.

### Story 2: Make Submit Work feedback full-width, singular, and session-safe

#### Problem statement

Submit feedback is capped below card width, item errors can appear twice, and a
late response from a previous session can overwrite the current form.

#### Customer ask

Keep submission feedback aligned with the form and scoped to the submission
that produced it.

#### Solution

Remove the status width cap, give each validation error one authoritative
location/association, scope both simple and rich drafts/mutations by exact
Factory Session, show the destination and returned receipt, synchronously guard
activation, and ignore or cancel responses whose initiating session generation
is no longer active.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update `submit-work-status-panel.tsx`, `submit-work-card.tsx`, simple composer,
  `use-submit-work-widget.ts`, and bento instance/form IDs; reuse shared status
  and form primitives.

##### Contracts

- Keep local draft/mutation state separate from returned canonical
  `requestId`/`traceId`.

##### Services

- Work admission behavior is unchanged for this story.

##### API changes

- None for width, error presentation, same-tick guard, or session scoping.

##### Tests

- Width measurement, one-error announcement, duplicate-widget IDs,
  double-click/Enter+click, simple and rich A-to-B draft isolation, visible
  destination, simple success/trace receipt, late success/error after session
  switch, retry, long locale/200% zoom, and narrow layout coverage.

#### Acceptance criteria

- Guidance, pending, success, validation, and error panels fill the card and
  wrap without horizontal overflow.
- Each error is announced once and is programmatically associated with its
  recovery target.
- One activation produces one client mutation; late session-A results do not
  clear or annotate session B.

### Story 3: Align the Submit Work public name contract

#### Problem statement

Generated clients allow an omitted request name while server and dashboard
behavior reject missing, empty, and whitespace-only names.

#### Customer ask

Make public, server, and dashboard validation agree.

#### Solution

Make `SubmitWorkRequest.name` required with matching semantic validation and
regenerate every contract consumer.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update authored OpenAPI fragments and matching Work admission/UI validation.

##### Contracts

- `name` is required and blank-only input is invalid across all entry points.
- Preserve the existing, intentional header-only Work behavior: optional blank
  text rows are omitted rather than transmitted as blank items.

##### Services

- `pkg/services/work` remains the canonical admission owner.

##### API changes

- Run `make generate-api` or `make interfaces-all` as required; never hand-edit
  bundled OpenAPI or generated Go/TypeScript clients.

##### Tests

- API contract, transport, generated-client, UI helper, and browser validation
  cases for missing/empty/whitespace names and optional empty text content.

#### Acceptance criteria

- Every public client rejects or cannot construct the invalid name cases, and
  server/UI outcomes remain equivalent.
- Generated artifacts are drift-free and `make api-smoke` passes.

### Story 4: Return and render each live Factory Session once

#### Problem statement

Two live-session readers can describe the same default session as a UUID and
`~default`, and the header renders both rows as separate tabs.

#### Customer ask

Show one navigation entry for each real live Factory Session and no phantom
Factory.

#### Solution

Reconcile live summaries by canonical session identity in the Factory Sessions
owner, then render singleton context without exposing a routing alias as a
second resource.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Correct `pkg/services/factory_sessions/transports/http/session_listing.go`
  and the header session projection/components.

##### Contracts

- Preserve `~default` as an input selector; publish the resolved UUID when
  known. Do not deduplicate by Factory name or path because distinct sessions
  may target the same Factory.

##### Services

- Factory Sessions owns list reconciliation and session selection.

##### API changes

- No schema change.

##### Tests

- Exact duplicate, alias+UUID, one default, default+named, two sessions for one
  Factory, close-to-single, and server response-count regressions.

#### Acceptance criteria

- One real default session yields one header context; default plus named yields
  two; two distinct sessions targeting one Factory remain two.
- Exactly one session uses concise current context rather than a misleading
  one-item tab strip; Open another session remains reachable.

### Story 5: Make Open Factory and New Factory explicit paths

#### Problem statement

Creation is available only after an action labelled Start Factory performs a
validation-only request and produces a failed-looking state.

#### Customer ask

Let users intentionally open an existing Factory or create a new one.

#### Solution

Present explicit Open existing and Create new paths using the existing
validation and `initNewFactory` operations, with destination preview and
operation-specific errors.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update `dashboard-session-tabs-open-dialog.tsx`,
  `use-dashboard-session-tabs-state.ts`, and localized messages.

##### Contracts

- Keep `Factory` creation distinct from opening/selecting the resulting
  `Factory Session` and its `Current Factory`.

##### Services

- Factory Sessions owns discovery/open; Factory Definitions owns scaffold
  persistence.

##### API changes

- Reuse existing `POST /factory-sessions` fields; no new route or schema.

##### Tests

- Existing target, missing target, invalid/broken config, create preview,
  create success, validation/scaffold/open failures, cancel, and keyboard flow.

#### Acceptance criteria

- Validation is visibly non-mutating and never described as starting a Factory.
- Create previews the exact `<folder>/factory` destination, and success opens
  and selects the returned session.
- A broken existing Factory is never offered an overwrite-as-new shortcut.

### Story 6: Preserve every concurrent workstation dispatch

#### Problem statement

Multiple active dispatches on the same workstation collapse to the last entry
in `workstation_activity_by_node_id`.

#### Customer ask

Show all active Work at a workstation without unstable ordering or loss.

#### Solution

Aggregate activity by workstation into a deterministic dispatch-keyed
collection rather than replacing entries by `transitionID`.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Correct `ui/src/features/timeline/state/timeline/projectRuntime.ts` and
  downstream graph/current-selection projections.

##### Contracts

- Use `(dispatch_id, work_id)` for Work participation and preserve stable
  workstation identity mapping across node ID, transition ID, and name fallback.

##### Services

- No service mutation; consume canonical Factory Events.

##### API changes

- None.

##### Tests

- Same-workstation concurrency, one sibling completion, equal timestamps,
  replay order, node/transition divergence, and graph component proof.

#### Acceptance criteria

- Every concurrent dispatch remains visible and deterministically ordered.
- Completing one dispatch removes only that dispatch; siblings remain.
- Live and selected-tick replay produce the same projection.

### Story 7: Project durable dispatch-to-Worker Session associations

#### Problem statement

The UI discards `DISPATCH_WORKER_SESSION_ASSOCIATION`, preventing durable
correlation of Factory activity with Worker Session identity.

#### Customer ask

Keep the association needed to inspect the real execution session.

#### Solution

Add the generated event to typed UI handling and replay it into a durable
dispatch-keyed association projection without copying source-native Worker
observations into Factory replay state.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update UI event types, `replayWorldState.ts`, runtime projections, and shared
  replay package contracts only if the package owns this event vocabulary.

##### Contracts

- Association is durable Factory history; Worker observation records remain on
  the Worker Session Events topic.

##### Services

- Recordings remains the Factory Event owner; Worker Sessions owns observation.

##### API changes

- None; generated payload already exists.

##### Tests

- Retained replay, reconnect, out-of-order canonicalization, duplicate
  idempotency, terminal history, and conflicting-association diagnostics.

#### Acceptance criteria

- Every known association survives live delivery and replay and is addressable
  by dispatch ID.
- No Worker Session observation is silently promoted into durable Factory state.

### Story 8: Stream truthful Worker Session Events

#### Problem statement

The current runtime-bound Worker Session route can prefer Factory ledger events
while the public route promises the source-native Worker Session Events topic.

#### Customer ask

Make live Worker Session progress truthful about source, ordering, and history
availability.

#### Solution

For live sessions, stream retained-then-live records from the exact Worker
Session Events topic. For historical/restarted sessions, return an explicitly
documented provenance/fallback or a typed source-history-unavailable outcome.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Correct the runtime Worker Session observation adapter and HTTP handler tests.

##### Contracts

- Define terminal, replay, source-failure, retention-gap, cancellation, and
  restart semantics; never present Factory Events and source-native Events as
  identical provenance.

##### Services

- Worker Sessions publishes to Events; Events supplies retained/live aggregate
  order; Recordings remains historical Factory facts.

##### API changes

- No shape change if current delivery/provenance values can express the result.
  Add a cursor/gap contract only if reconnect requirements prove it necessary.

##### Tests

- Interleaved sibling streams, terminal close, already-terminal replay, source
  failure, retention gap, cancellation, restart, and server-bound SSE proof.

#### Acceptance criteria

- Sibling records never cross streams; terminal closes only the matching
  session; complete retained records survive source failure.
- The client can distinguish live source-native records from historical
  Factory-event projection or explicit unavailability.

### Story 9: Render every Worker Session for selected Work

#### Problem statement

Current Selection renders inferred Provider Session attempts and cannot show
starting, script, concurrent, or terminal Worker Sessions independently.

#### Customer ask

Enumerate the authoritative Worker Sessions associated with selected Work.

#### Solution

Add a typed UI API/hook/feature using the existing work-scoped list and exact
stream/show/transcript operations. Key rows by `workerSessionId` and expose a
Provider Session link only when available.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Add `ui/src/api/worker-sessions/` and a focused Worker Sessions feature;
  integrate it into selected Work/Current Selection without feature-wide
  barrels.

##### Contracts

- Worker Session is execution identity; Provider Session is optional detail.
  Stream state is keyed by `workerSessionId`, never only dispatch/workstation.

##### Services

- Reuse Worker Sessions and Provider Sessions public operations.

##### API changes

- No change for selected-Work enumeration. Factory-session-wide enumeration is
  a separate follow-up unless a proven journey requires it.

##### Tests

- Multiple concurrent sessions, STARTING without provider, terminal
  completed/failed/canceled, loading/empty/not-found/error/retry, interleaved
  streams, reconnect dedupe, selection change, and unmount cleanup.

#### Acceptance criteria

- Every returned Worker Session is a separate stable row/card in API order.
- One stream failure does not erase siblings; terminal rows remain inspectable.
- Optional Provider Session/transcript actions never hide the Worker Session.

### Story 10: Enumerate all Work and execution history for a workstation

#### Problem statement

Request history shows only `work_items[0]`, request presence suppresses other
attempts, and attempts without Provider Sessions disappear.

#### Customer ask

Show complete active and historical workstation participation with reliable
navigation.

#### Solution

Project Work participation by `(dispatch_id, work_id)`, deduplicate input/output
overlap, join request summaries with authoritative Worker Sessions without
mutual suppression, and add progressive history rendering if the measured
fixture requires it.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update Current Selection request helpers, workstation detail/history/active
  components, selection history, and feature messages.

##### Contracts

- Preserve Work, dispatch request, Worker Session, and Provider Session
  identities separately; label consumed and produced Work relationships.

##### Services

- Reuse Recordings projections, paginated Work, and work-scoped Worker Sessions.

##### API changes

- None unless the scale probe proves existing composition incomplete or an
  unacceptable N+1 path.

##### Tests

- Multi-Work dispatch, input/output overlap, repeated Work across dispatches,
  multiple Worker Sessions/retries, missing provider, deterministic order,
  forward/back navigation, loading/error/empty, and large history.

#### Acceptance criteria

- Every distinct Work participation and Worker Session attempt is visible once
  in the correct context.
- Work and request actions open exact identities and preserve selection history.
- Partial projection data is never presented as a definitive empty state.

### Story 11: Unify graph editor command history and selection semantics

#### Problem statement

Generic Undo/Redo covers layout only, observer selection can look like editor
selection while editor actions see nothing selected, and observer nodes remain
draggable.

#### Customer ask

Make graph editing predictable across topology, layout, mode, and selection.

#### Solution

Represent topology, fields, and layout changes as one ordered editor document
history; make runtime-inspection, group, and editor selection explicit; disable
node and viewport persistence in observe mode. Group visual behavior, group
fit-to-members, node sizing, and group-region styling remain owned by Stories
3–4 of `factory-graph-visual-ux.md`; this story owns their shared transaction,
dirty-state, stale-save, and Undo/Redo seam.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update editable graph hooks, graph operations/history, selection gestures,
  controls, view model, and viewport wiring.

##### Contracts

- History records domain commands/document transitions, never React Flow
  objects. Save sets a baseline; Discard restores server state and clears
  history.

##### Services

- Current Factory document save remains the mutation owner.

##### API changes

- None.

##### Tests

- Mixed add/connect/delete/field/move/group-membership/reset command order,
  compound group reassignment undo, stale layout-only save rejection, dirty
  session-change guards, redo invalidation, save/discard, mode transitions,
  Escape/additive selection, and observe/editor drag browser proof.

#### Acceptance criteria

- Undo/Redo restores the complete prior/next editable Factory document in LIFO
  order and button state matches available history.
- Entering edit mode never leaves a node looking editor-selected while Delete
  reports no selection.
- Observer nodes cannot move; editor drag dispatches one layout operation.

### Story 12: Make connection editing and editor controls progressive

#### Problem statement

Edit mode places every connection handle into a huge interaction surface with
no clear Connect workflow, and the floating toolbar clips/overlays content.

#### Customer ask

Provide a discoverable, keyboard-operable connection flow and reachable draft
actions at every supported viewport.

#### Solution

Activate connection targets only after an explicit source/focus intent, expose
valid targets and cancellation, and give the editor toolbar safe canvas insets
plus responsive wrapping/scroll/overflow behavior.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update editor connection projection/controllers, React Flow wiring, controls,
  floating surface, localized tooltips, and fit-view inset logic.

##### Contracts

- Every edge endpoint references a handle rendered by the same canonical
  projection; connection completion dispatches a graph operation.

##### Services

- None beyond existing Factory document save.

##### API changes

- None.

##### Tests

- Valid/invalid targets, keyboard source/target/cancel, handle projection,
  390/768/1280 toolbar geometry, dirty Save/Discard visibility, focus order,
  and real React Flow browser interaction.

#### Acceptance criteria

- Handles do not all enter tab order before connection intent.
- Escape cancels without changing the draft; successful connection updates the
  canonical draft and never persists raw React Flow edges.
- Save, Discard, and Leave remain visible/reachable and the toolbar does not
  cover selectable graph content.

### Story 13: Recover, project, and manipulate bento layouts safely

#### Problem statement

Unsafe persisted geometry and duplicate IDs can reach React Grid Layout; tablet
widths retain cramped desktop columns; layout manipulation is pointer-only.

#### Customer ask

Keep the dashboard usable after reload and across viewport/input modes.

#### Solution

Normalize stored layouts into unique bounded geometry with content-specific
minimums, define and enforce singleton/duplicate limits, codify the chosen
operator/backend/factory preference scope, project one column across the
unusable tablet gap without persisting compact geometry, and add keyboard
reorder/resize plus dirty-aware add/remove operations.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update bento schema/persistence/mutations/store, `agent-bento.tsx`, the typed
  widget catalog, duplicate widget labels, remove/undo/focus behavior, and
  layout action primitives. Treat Factory graph as singleton until editor
  controller ownership is instance-scoped.

##### Contracts

- Browser-local layout uses a versioned envelope with explicit scope, monotonic
  instance identity, unique IDs, singleton counts, positive integer bounds,
  deterministic collision repair, and a recover/reset path. Widget contents
  and selections remain Factory Session-scoped. React Grid Layout and compact
  geometry remain disposable.

##### Services

- No backend service.

##### API changes

- None.

##### Tests

- Malformed/unsafe storage, duplicate IDs/singletons, overlap repair,
  idempotent migration, capped duplicates, dirty remove confirmation, undo,
  focus restoration and live announcements, pointer/keyboard/touch
  move/resize/cancel, storage failure, two scopes, desktop to compact to
  desktop, and every card at 320/390/640/768/1280.

#### Acceptance criteria

- Reload always yields unique IDs, positive bounded dimensions, no off-grid
  cards, one add-widget card, and meaningful content minimums.
- Compact projection is full-width, deterministic, non-persisting, and covers
  the current 640–768 gap.
- Keyboard users can move/resize and hear the resulting position/size; duplicate
  widget instances have distinct visible and accessible names.

### Story 14: Make session, trace, and motion accessibility truthful

#### Problem statement

Session tabs control an empty panel, all tabs share a color-only selected-stream
status, tab reordering is pointer-only, narrow trace graphs can overflow, and
shared loading/overlay motion ignores reduced-motion preference.

#### Customer ask

Make dashboard navigation and visualization usable by keyboard, screen reader,
touch, zoom, and reduced-motion users.

#### Solution

Use truthful tab-panel or navigation semantics, scope status to the active
session unless per-session state exists, add text/shape and keyboard reorder,
make trace diagrams fit their cards with operable resizing, and gate shared
motion primitives.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update session tab components/messages, trace graph shells, shared feedback
  and overlay primitives, and undersized touch controls.

##### Contracts

- Inactive session status is neutral unless real per-session stream state is
  projected. Status is not communicated by color alone.

##### Services

- None.

##### API changes

- None for honest active-only stream status.

##### Tests

- Arrow/Home/End/keyboard reorder and announcements, NVDA/VoiceOver spot check,
  reduced-motion emulation, 200% zoom/reflow, 320/390 trace overflow, labelled
  pan/zoom/resize, and minimum 44 px touch hit areas for dense actions.

#### Acceptance criteria

- No tab controls an empty panel; selected content has a truthful programmatic
  relationship or the selector uses navigation semantics.
- Session state is accessible without color and does not falsely apply one
  stream's status to inactive sessions.
- Trace cards do not cause page-level overflow, and all essential interaction
  has keyboard/touch equivalents.
- Nonessential pulse, ping, dialog, popover, and skeleton motion is suppressed
  under reduced motion while state remains perceivable.

### Story 15: Keep Work, trace, request, Worker Session, and Provider Session inspection coherent

#### Problem statement

Trace selection can outlive its Work, direct request detail cannot open the same
Provider Session exposed elsewhere, back/forward restores only base selection,
and duplicate terminal Work labels can resolve to the wrong Work/dispatch.

#### Customer ask

Make every drill-down step preserve exact identity and return to a coherent
inspection context.

#### Solution

Validate or clear trace/session sub-selection atomically when Work changes,
route Provider Session selection through every attempt surface, store or
deterministically reset inspection context in selection history, and project
terminal outcomes by Work plus terminal dispatch/event identity rather than
display label.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update selection history, trace selection, direct request detail, Provider
  Session correlation, terminal-work projection/list identity, and trace state
  ordering.

##### Contracts

- Work uses `work_id`; request/execution uses `dispatch_id`; Worker Session uses
  `workerSessionId`; Provider Session content uses `(provider, kind, id)` while
  dispatch/attempt is navigation context; trace comes from Factory Event replay.

##### Services

- Work, Worker Sessions, Provider Sessions, and Recordings retain their existing
  read ownership.

##### API changes

- Extend Factory Visualization/dashboard terminal outcome data with Work-ID and
  terminal dispatch/event references if the existing label-only projection
  cannot provide exact identity; regenerate all contract artifacts if changed.

##### Tests

- Work A → trace A2 → Work B, duplicate terminal labels, completed-to-failed
  transition, direct request → Provider Session, identical Provider Session from
  two dispatches, back/forward, replay removal, and trace loading/absent/error.

#### Acceptance criteria

- Current Selection and trace/session detail never show identities from
  different Work contexts.
- Every inference attempt with an available Provider Session offers the same
  keyboard-operable inspection action from Work and request paths.
- Terminal rows open the exact Work and dispatch even when labels repeat.
- Back/Forward either restores the full valid inspection step or intentionally
  returns to its base entity; focus moves to the restored heading.

### Story 16: Validate, contain, and scale every Factory graph projection

#### Problem statement

Replay forwards unchecked node/handle endpoints, current/trace graphs treat all
React Flow diagnostics as fatal, merged trace nodes can discard required
handles, and dense/live projections perform avoidable full scans and rebuilds.

#### Customer ask

Keep observe, replay, and trace graphs truthful, recoverable, and responsive on
large live Factories.

#### Solution

Share endpoint validation across graph modes, union trace handles by stable ID,
classify and contain fatal projection failures within the affected card, index
runtime overlays once, and preserve stable node/edge identities across
clock-only updates. Keep visual grammar work in `factory-graph-visual-ux.md`.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update package replay projection/surface, dashboard and trace endpoint error
  classification/boundaries, trace relation merge, current-activity view-model
  memoization, replay empty state, and viewport precedence.

##### Contracts

- Every edge endpoint must reference a handle rendered by its projected node.
  React Flow diagnostics are classified; only proven integrity failures are
  fatal. A valid authored viewport wins over automatic fit.

##### Services

- Recordings and Factory Visualization remain canonical history/presentation
  sources; no service mutation.

##### API changes

- None proven.

##### Tests

- Missing node/source handle/target handle, duplicate node ID, trace handle
  union/conflict, card-local fatal recovery, non-fatal warning logging, empty
  replay, authored viewport/fallback fit/tick change, and sparse/dense 500-node
  performance/render-count fixtures.

#### Acceptance criteria

- Invalid topology produces a localized actionable graph error without blanking
  unrelated dashboard cards; non-fatal warnings do not crash the graph.
- Trace endpoint merging preserves every valid handle and diagnoses conflicts.
- Dense replay projection is approximately linear in nodes, edges, and overlay
  entries; clock-only updates preserve unaffected node/edge identity, viewport,
  selection, and layout.
- A valid authored viewport is honored, selected-tick changes preserve user
  navigation, and zero-node replay presents an explicit empty state.

### Story 17: Enumerate Worker Sessions for the Current Factory Session

#### Problem statement

Story 9 exposes Worker Sessions only through selected Work, while operators
also need to understand every starting, concurrent, provider-less, retried, and
terminal Worker Session in the Current Factory Session.

#### Customer ask

Provide a complete Factory Session-wide Worker Sessions view without requiring
the operator to discover and select each Work first.

#### Solution

Add a Factory Session-scoped Worker Session list and exact detail journey keyed
by `workerSessionId`, with attempts, Work and dispatch associations, lifecycle,
optional Provider Session links, resumable observations, and stable navigation.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Extend Worker Sessions service contracts, recordings-backed projections, HTTP
  transport/mapping, generated clients, and add a dashboard API module, query
  hooks, list/detail components, and `worker-session` selection-history kind.

##### Contracts

- Worker Session ID is primary identity. Attempts preserve every dispatch and
  provider association or absence; Work, workstation, timing, state, failure,
  cursor, truncation, and replay-complete metadata are explicit.

##### Services

- Worker Sessions owns identity, attempts, lifecycle, observations, and
  transcript availability. Recordings supplies durable Factory associations;
  Provider Sessions stays optional detail.

##### API changes

- Add exact Worker Session detail/events endpoints and Factory Session-scoped
  cursor pagination. Keep provider-reference lookup only as compatibility.
- Regenerate bundled OpenAPI plus Go and TypeScript clients.

##### Tests

- Provider-less sessions, two concurrent sessions for one Work, multi-attempt
  retries/provider switches, every terminal state, live/replay ordering,
  resumable cursor/gap, back/forward, pagination, and narrow/desktop journeys.

#### Acceptance criteria

- Every Worker Session in the Current Factory Session appears once by stable
  ID, regardless of Provider Session availability.
- Selecting a row restores its exact attempt/Work/dispatch context and survives
  back/forward without substituting Provider Session identity.
- Reconnect resumes without loss or duplication and explicitly reports gaps,
  known-empty, terminal, unavailable, and replay-complete states.

### Story 18: Make Provider Session inspection contextual, safe, and bounded

#### Problem statement

Provider Session content can be fetched reliably, but users cannot see its
origin context, reach it from every attempt, recover a failed fetch, or safely
inspect a large/raw transcript.

#### Customer ask

Open the same Provider Session consistently from Work, request, dispatch, and
Worker Session history with enough context to understand and safely navigate it.

#### Solution

Separate Provider Session content identity from optional origin context; route
all entry points through one inspector with breadcrumbs, history, retry,
stale-while-refresh, bounded transcript presentation, and explicit server-side
redaction/reveal rules.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update Provider Session selection keys/state, request and Worker Session
  entry points, detail panel, transcript renderer, cache/retry behavior, focus
  restoration, and safe copy/reveal controls.

##### Contracts

- Content identity is `{provider, kind, id}`. Origin context is optional
  `{factorySessionId, dispatchID, workID, workerSessionID}` and does not create
  duplicate content identities.
- Transcript payloads carry redaction/truncation metadata; ciphertext and raw
  secret-bearing tool data are not displayed by default.

##### Services

- Provider Sessions owns normalized/redacted transcript inspection and bounded
  pagination; Worker Sessions supplies optional origin association.

##### API changes

- Add cursor/truncation/redaction fields only where the existing detail contract
  cannot provide bounded safe reads; regenerate affected interfaces.

##### Tests

- Every entry point, one Provider Session shared by two dispatches, retry and
  abort, stale refresh, back/forward, redacted and revealable payloads, huge
  transcript paging/windowing, 44 px controls, and narrow long-content layout.

#### Acceptance criteria

- The inspector always shows Provider Session identity and available origin
  context without clearing equivalent content when only the origin changes.
- Large transcripts do not mount or reveal all raw data by default; authorized
  reveal and copy actions are explicit, audited by tests, and redaction-safe.
- Fetch errors are recoverable and session switches cannot show prior-session
  content.

### Story 19: Give dashboard Work metrics coherent, accessible semantics

#### Problem statement

Removing the duplicate Y-axis label does not fix a chart that compares queued
Work, active dispatches, accepted dispatches, and ever-failed Work under one
ambiguous unit. The Work totals card repeats the ambiguity with four labels
whose gauges and cumulative counts use different subjects.

#### Customer ask

Make every chart series and summary total comparable, understandable,
localizable, and operable without relying on color or pointer hover.

#### Solution

Define each series' subject, aggregation, unit, and included outcomes; project
honest metrics, expose a textual latest/selected-tick data alternative, localize
numbers and tooltips, and provide keyboard-equivalent inspection and range
controls.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update materialized outcome and totals projections, Work totals labels/reflow,
  chart model/legend/tooltip/axis, accessible summary/table, keyboard range
  control, point indexing, and optional display-only downsampling.

##### Contracts

- Every metric declares Work versus dispatch subject, gauge versus cumulative
  aggregation, non-negative integer unit, and treatment of ACCEPTED, CONTINUE,
  REJECTED, FAILED, retry, and cancellation outcomes.

##### Services

- Keep projection client-side if canonical events are sufficient; otherwise
  Factory Visualization owns a typed outcome aggregate rather than duplicating
  metric policy in Recharts components.

##### API changes

- None unless a backend-owned materialized aggregate is selected; any new
  schema must use explicit metric definitions and generated clients.

##### Tests

- One axis title, chart/totals metric truth table, zero/negative/decimal
  rejection, 1e9 outlier, 512-sample dense history, selected historical tick,
  localized tooltip/ticks, keyboard inspection/zoom, accessible data
  alternative, Work totals 2x2/1-column reflow, and exact card/chart width at
  320/390/640/desktop.

#### Acceptance criteria

- Every plotted value has one unambiguous subject, aggregation, unit, and
  accessible textual representation.
- The chart has exactly one Y-axis title and one intentional grid, fills its
  content width, and remains usable for outliers and dense history.
- Interactive controls are outside `role=img` semantics, keyboard operable,
  non-color-dependent, and at least 44 px.

### Story 20: Isolate widgets and standardize asynchronous recovery

#### Problem statement

Individual cards infer loading, empty, stale, reconnecting, historical, and
failed state differently; a retained snapshot can hide recovery failure, and a
single rendering exception can remove the whole dashboard.

#### Customer ask

Keep available data visible and honest during hydration or recovery, and keep
one broken widget from taking down its siblings.

#### Solution

Publish identity-keyed stream freshness to all cards, add a shared async-state
presentation contract and per-widget error boundary, and synchronously gate
session/trace/provider state before rendering a new Factory Session.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update stream/timeline/session lifecycle state, world-view selection, bento
  snapshot props, Trace and Provider cache keys, shared status panels/live
  regions, retry actions, and `DashboardWidgetErrorBoundary`.

##### Contracts

- Stream state is keyed by backend, Factory Session, logical session, and
  generation and declares hydrating, ready-known-empty, live, historical,
  stale, reconnecting, gap, recovery-failed, last-event time, and retryability.

##### Services

- Factory Sessions/Events expose a replay-ready or watermark signal sufficient
  to distinguish known-empty from still-hydrating without guessing from rows.

##### API changes

- Add a stream-ready/replay-watermark envelope if the existing stream cannot
  signal replay completion; regenerate stream types and clients.

##### Tests

- Zero-event readiness, retained offline/reconnect/recovery failure, cursor
  gap, live-to-historical-to-live, A-to-B same IDs, late A event/status after B,
  stale Trace/Provider selection, retry, and deliberate child throw isolation.

#### Acceptance criteria

- Retained data remains visible with an explicit stale/degraded label and retry;
  it never appears live or hides terminal recovery failure.
- Historical mode is derived from timeline state and cannot submit or edit the
  Current Factory.
- A Factory Session switch cannot render, select, cache, or announce data from
  the previous identity, and one widget exception leaves siblings operational.

### Story 21: Bound dense inspection and isolate render churn

#### Problem statement

Terminal Work, Current Selection, Provider transcripts, Trace graphs/tables,
and payload/relationship views can render all retained data, while the global
one-second clock reconstructs cards that do not depend on time.

#### Customer ask

Keep the dashboard responsive with long-running sessions, large traces,
transcripts, histories, payloads, and many widget instances.

#### Solution

Define product-level page/window/preview thresholds, incremental data contracts,
and render budgets; index repeated joins; subscribe only time-dependent cards
to the clock; and virtualize or progressively disclose dense inspectors.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Add bounded list/transcript/table models, virtualization or pagination,
  truncation diagnostics, indexed Work/attempt/relation joins, memoized card
  boundaries, scoped clocks, occupancy-grid bento placement, and performance
  fixtures.

##### Contracts

- Bounded reads expose total count, returned range/cursor, truncation, and
  continuation. Rendering downsampling never changes accessible values or
  canonical stored history.

##### Services

- Owning services provide cursor pagination for data that cannot be bounded
  safely in the client; Events retention and Recordings durability remain
  distinct.

##### API changes

- Add cursor/limit/total/truncation fields for Worker/Provider Sessions or other
  inspectors only where local retained state is not the canonical full set.

##### Tests

- Twenty widgets plus 1,000 Terminal/Trace/Provider/history entries, long text
  and binary metadata, 512 outcome samples, pagination/window focus continuity,
  cancellation, memory bounds, commit-time budgets, and proof that clock ticks
  do not rerender unrelated cards.

#### Acceptance criteria

- No dense inspector mounts an unbounded retained collection or blocks the main
  thread with whole-file/stringify/quadratic projection work.
- Pagination/window changes preserve exact identity, ordering, selection,
  focus, stale state, and accessible count/range copy.
- Time-independent widgets do not rerender on the one-second dashboard clock.

### Story 22: Complete advertised dashboard locales and long-copy geometry

#### Problem statement

The product advertises English, Simplified Chinese, Japanese, and Korean, but
many bento catalogs provide only English and Chinese, formatters sometimes
ignore locale, and long-copy geometry is not systematically tested.

#### Customer ask

See complete, correctly formatted dashboard copy in every advertised locale
without clipped controls, overlapping labels, or English fallback surprises.

#### Solution

Require complete message catalogs for every advertised locale, route all
numbers/durations/outcomes through locale-aware formatters, and add pseudo-long
geometry fixtures across every card and breakpoint.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Complete ja/ko catalogs, strengthen locale completeness checks, remove
  hardcoded runtime copy, pass locale to Trace/chart formatters, and adopt
  wrapping/reflow recipes for headings, legends, totals, controls, and forms.

##### Contracts

- Every selectable locale is required for every customer-facing dashboard
  catalog. Technical/backend errors are normalized before presentation.

##### Services

- None unless server-provided diagnostics require stable localization keys.

##### API changes

- None for authored UI copy; use typed diagnostic codes instead of localizing
  raw server strings if backend errors enter customer UI.

##### Tests

- Catalog completeness for en/zh-CN/ja/ko, locale-aware outcomes/durations/
  numbers, pseudo-long strings, 320/390/640/768/desktop, 200% zoom, duplicate
  cards, and error/loading/recovery copy.

#### Acceptance criteria

- No advertised locale falls back to English for authored dashboard messages.
- Dates, durations, counts, outcomes, axes, tooltips, and statuses use the
  active locale consistently.
- Every bento card reflows without unintended page overflow or unreachable
  actions under pseudo-long strings and 200% zoom.

### Story 23: Bound attachment staging and preserve submission receipts

#### Problem statement

Submit Work buffers and base64-encodes whole files into JSON without size,
count, concurrency, or type policy; staging references expose encoded host
paths, and accepted/duplicate/runtime progress disappears with the component.

#### Customer ask

Attach supported content safely, understand upload and admission progress, and
retain a trustworthy link to the submitted Work even across retries or remounts.

#### Solution

Publish and enforce upload policy at every boundary, use opaque streamed staging
with bounded concurrency/progress/cancel, and keep a session-scoped submission
receipt ledger keyed by a client request identity.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update Work content staging, HTTP transport, generated clients, file picker,
  drag/drop/paste, progress/cancel/retry, safe preview metadata, receipt/status
  UI, Work/Trace navigation, and localized validation.

##### Contracts

- Define accepted MIME/content types, sniffing policy, per-file/total/count
  limits, expiry and cleanup, opaque staging tokens, idempotency identity,
  accepted-versus-duplicate outcome, and submission/runtime status vocabulary.

##### Services

- Work owns admission receipts and content staging policy. Staging never exposes
  absolute server paths and cleans canceled/replaced/expired content.

##### API changes

- Replace unbounded base64 JSON staging with multipart, streaming, or bounded
  chunked upload; expose upload limits, progress-compatible identity, opaque
  references, idempotent request identity, and receipt fields. Regenerate all
  affected contracts and clients.

##### Tests

- File/text/JSON/image/audio/binary policy, MIME mismatch, size/count/aggregate
  limits, bounded parallelism, progress/cancel/cleanup, clipboard/drop,
  ambiguous retry/idempotency, accepted/duplicate/runtime outcomes, remount,
  session switch, multiple widgets, safe filename/token, and 320-to-desktop.

#### Acceptance criteria

- Unsupported or oversized content is rejected before expensive buffering and
  at the server boundary with one localized, field-associated error.
- Staging is cancellable, bounded, path-opaque, and does not copy whole large
  files through byte-array, binary-string, base64, and JSON representations.
- Every submission produces a session-scoped receipt showing request/Work/Trace
  identity and accepted, duplicate, queued/running, succeeded, or failed state.

### Story 24: Standardize Current Selection and Trace content rhythm

#### Problem statement

Current Selection and Trace share the same outer widget frame, but their inner
sections mix incompatible margins, gaps, dividers, nested panels, fixed
description columns, and minimum widths, producing visibly inconsistent rhythm
and narrow-card clipping.

#### Customer ask

Make inspection cards feel like one coherent system while preserving readable
hierarchy, scrolling, and compact reflow.

#### Solution

Keep the shared frame and introduce one inspection-section primitive and
description-list responsive recipe with measurable spacing, divider, nesting,
scroll, title, and touch-target rules used by Current Selection and Trace.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Add/refine shared inspection section, expandable section, nested surface, and
  responsive description-list primitives; migrate Current Selection and Trace
  shells/sections and remove duplicated utility-selector contracts.

##### Contracts

- Direct sections use 16 px block separation and 10 px internal gap; sections
  after the first use one divider plus 16 px top spacing; nested surfaces use
  12 px padding and the shared outline/radius; description lists stack below
  640 px and use a 136 px label column above it; controls are at least 44 px.

##### Services

- None.

##### API changes

- None.

##### Tests

- Empty/loading/error/ready content, one and many sections, nested expansion,
  long IDs/localized labels, scroll-owner assertions, focus continuity,
  duplicate widgets, and exact geometry at 320/390/640/768/desktop and 200% zoom.

#### Acceptance criteria

- Current Selection and Trace use the same outer and inner spacing primitives;
  no feature-local margin/divider override recreates a competing rhythm.
- Titles, description values, graphs, tables, and actions reflow without
  unintended page overflow, nested vertical scrolling, or clipped focus rings.
- Visual hierarchy, semantic heading order, touch targets, and focus behavior
  remain consistent across empty, loading, error, and dense ready states.

## Project-level acceptance criteria

- Every known regression and each failure in the thirty-probe matrix is either
  directly disproved by the owning story or explicitly linked to the owning
  `factory-graph-visual-ux.md` acceptance criterion.
- Canonical Factory Session, Work, dispatch, Worker Session, Provider Session,
  Factory Event, and Events-topic identities remain distinct at every layer.
- Complex graph/editor/bento surfaces keep canonical state, pure operations,
  projections, and component wiring separately testable.
- Network-backed surfaces expose identity-keyed hydrating, known-empty, ready,
  historical, stale, reconnecting, gap, recovery-failed, retry, and success
  states where applicable; retained data is never presented as fresh by
  omission.
- One widget failure does not blank sibling widgets; duplicated permitted
  widgets keep unique DOM/form/disclosure identity, while singleton and maximum
  instance policies are enforced before render.
- Group creation, membership reassignment, observe-mode rendering, click-through,
  invalid geometry rejection, cancel, save/reload, and compound Undo/Redo meet
  the linked graph-visual and editor-transaction acceptance criteria.
- No generated OpenAPI or client artifact is hand-edited; all contract changes
  regenerate and pass drift checks.
- New user-facing copy uses feature-owned complete en/zh-CN/ja/ko catalogs and
  shared controls; locale-aware formatting and pseudo-long geometry pass.
- Every canonical widget passes 320/390/640/768/desktop, 200% zoom, keyboard,
  touch, screen-reader, forced-colors, reduced-motion, focus restoration, live
  announcement, and selected-tick/session-switch evidence.
- Dense/many-widget fixtures meet explicit render, memory, and pagination
  budgets; the one-second clock does not rerender unrelated widgets.
- Delivery continues through required CI until it is terminal and passing;
  blocking review feedback is explicitly addressed, conflicts and generated
  drift are resolved, and each PR is actually merged. Opening or approving a PR
  is not completion.

## Verification plan

Use the narrowest focused tests named in each story, then run the relevant final
gates:

```text
cd ui && bun run check
cd ui && bun run check:localized-copy
cd ui && bun run tsc
cd ui && bun run test:unit
cd ui && bun run test:component
cd ui && bun run test:integration
cd ui && bun run test:performance
cd ui && bun run test-storybook
cd ui && bun run storybook:responsive-check
make api-smoke                    # stories that change OpenAPI
make verify-fast
make verify-pr
```

Before relying on graph-consuming component suites, regenerate/build the
components distribution and prove that the `graph-edge` export resolves; a
suite that fails to start is a blocked verification lane, not a pass.

For Worker Session and Factory Session service changes, also run focused Go
service/transport tests and the appropriate functional server path. Functional
tests must construct through `root.BuildProcess` and `Process.Execute` by
default, prefer CLI for ordinary customer flows, replace external effects only
through `edges.Edges`, and use deterministic observation rather than sleeps.

## Definition of done

The program is done when a customer sees one real current Factory Session,
opens or creates a Factory through an intentional path, reads one correct Work
outcome chart, receives full-width session-safe submission feedback, sees every
concurrent Work and Worker Session without Provider Session substitution, can
trace exact Work/request/execution identities, edits the graph with coherent
selection, grouping, save protection, and undo behavior, and uses isolated,
bounded, localized dashboard widgets and layout navigation at supported
viewports and input modes. All required tests and generated artifacts are
current, CI and blocking review are resolved, conflicts are cleared, and the
implementation PRs are merged.
