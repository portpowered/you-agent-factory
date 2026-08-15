---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/dashboard-ux-probe-and-fix
---

# Dashboard UX Probe And Fix Plan

> **Current status — 2026-08-15:** The original probe markings and plan
> progress in this historical document are not current delivery status. See
> the [audited dashboard UX delivery status](dashboard-ux-delivery-status.md)
> for the verified 1-30 matrix, story rollups, and prioritized remainder.

## Outcome

Customers can open or create a Factory, understand the exact active Factory
Session and stream state, inspect every concurrent Work and Worker Session, use
the Factory editor—including groups—as one predictable transaction, and arrange
responsive dashboard widgets without duplicated labels, clipped feedback,
phantom identities, stale cross-session data, or inaccessible controls.

## Customer problem

The initial ten-probe audit and thirty additional bento/functionality probes
found failures across every dashboard journey. The known duplicate chart label,
capped submission notice, phantom `~default` tab, collapsed workstation
sessions, broken grouping, and inconsistent inspection spacing are symptoms of
larger contract gaps:

- display labels and partial keys substitute for canonical session, dispatch,
  Work, Worker Session, and Provider Session identity;
- loading, known-empty, historical, stale, reconnecting, and failed states are
  inferred inconsistently from retained rows;
- editor topology, fields, layout, selection, groups, and viewport have
  incompatible history and save rules;
- bento instance policy, persisted geometry, removal, responsive projection,
  and keyboard manipulation are not one validated lifecycle; and
- dense inspection, per-card failure isolation, locale completeness, touch
  targets, and render budgets are incomplete.

The full evidence matrix is
`docs/internal/development/plans/dashboard-ux-30-probe-results.md`.

### Audit side effect

The initial audit created one local `idea` Work request named `UX layout probe`
while exercising submission validation. This plan does not authorize cleanup;
retain or remove it only by explicit operator choice. The additional thirty
probes were read-only.

## Scope

In scope:

- Factory Session identity, tabs, Open/New Factory, lifecycle/stream controls,
  replay mode, reconnect, and session switching;
- Work totals/outcomes, Submit Work, workstation activity/history, Terminal
  Work, Selected Work, Worker Sessions, Provider Sessions, and Trace;
- editor selection, topology/field/layout history, grouping transaction seam,
  connection/cancel, save/reset/discard, and observe-mode immutability;
- bento catalog, duplicate/singleton policy, add/remove, persistence, geometry,
  responsive projection, keyboard/touch behavior, and failure isolation; and
- accessibility, localization, safe payload presentation, bounded rendering,
  browser evidence, generated contracts, and performance budgets.

Out of scope:

- re-owning graph visual grammar, group-region styling, graph-node sizing, and
  packaged renderer parity from
  `docs/internal/development/plans/factory-graph-visual-ux.md`;
- treating Provider Sessions as Worker Session identity;
- persisting React Flow or React Grid Layout objects as domain state; and
- broad dashboard restyling unrelated to the probe matrix.

## Architecture and ownership

```text
Factory Definitions + Factory Sessions + Work + Worker Sessions
        | canonical service operations and authored OpenAPI
        v
Recordings Factory Events       Worker Sessions -> Events topics
durable replay/history          source-native retained/live observations
        |                                  |
        +--------------+-------------------+
                       v
typed UI APIs + identity-keyed timeline/stream stores
                       |
                       v
pure editor/activity/selection/layout projections
                       |
                       v
bento cards, React Flow, charts, tables, dialogs
disposable render state; never canonical product state
```

- Factory Sessions owns canonical session identity. `~default` is an accepted
  selector, not a second durable row.
- Recordings owns durable Factory Event replay and dispatch-to-Worker Session
  association. Worker Sessions owns Worker Session identity, attempts, and
  observations; Provider Sessions remains optional detail.
- Work owns admission/content. Workstation and Terminal Work views are
  identity-preserving projections, never label-keyed canonical state.
- The Factory editor document owns editable topology, fields, grouping, and
  layout operations. React Flow state is reproducible projection state.
- The bento layout store owns a scoped, versioned operator preference. Compact
  React Grid Layout geometry is disposable and is not persisted.

## Delivery sequence

1. Land deterministic fixtures and the full regression matrix.
2. Stop cross-session stale state and make stream/history status truthful.
3. Reconcile Factory Session identity and preserve concurrent Work identity.
4. Expose authoritative Worker Sessions and coherent inspection navigation.
5. Unify editor transactions and complete the linked grouping work.
6. Sanitize bento lifecycle/persistence and isolate widget failures.
7. Land narrow chart/submission/spacing fixes with their honest contracts.
8. Bound dense data, complete locales/accessibility, run browser/CI gates, and
   merge each independently reviewable story.

## Work stories

### Story 0: Establish the deterministic dashboard regression probe

#### Problem statement

The defects depend on concurrency, replay, duplicate identities, malformed
persistence, dense data, and responsive geometry that happy-path fixtures omit.

#### Customer ask

Create one repeatable baseline that proves every known defect before and after
its owning fix.

#### Solution

Build fixture-driven pure, component, and browser scenarios containing every
canonical widget, permitted duplicates, duplicate session selectors, concurrent
dispatches, multi-Work requests, provider-less/retried Worker Sessions,
terminal states, grouped graphs, dense inspectors, and hostile stored layouts.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Add reusable builders under existing UI integration/component conventions;
  do not add production-only test seams or a second replay implementation.

##### Contracts

- Expected rows use immutable IDs, not display text.

##### Services

- Reuse `root.BuildProcess` and replace external effects only through
  `edges.Edges` for functional fixtures.

##### API changes

- None.

##### Tests

- Cover 320/390/640/768/desktop, 200% zoom, keyboard/touch, forced colors,
  reduced motion, en/zh-CN/ja/ko/pseudo-long copy, async states, session races,
  a deliberate card throw, duplicate widgets, and dense render budgets.

#### Acceptance criteria

- Every row in the thirty-probe matrix maps to a failing-before/passing-after
  assertion or a linked graph-visual-plan assertion.
- The fixture is deterministic, identity-keyed, CI-safe, and does not use sleeps
  as its primary synchronization mechanism.

### Story 1: Reconcile Factory Session identity and entry flows

#### Problem statement

A UUID session and `~default` alias can render as separate tabs; tab semantics
point at an empty panel; Open/New Factory and session controls misstate actions.

#### Customer ask

See one row per real Factory Session and intentionally open, create, inspect,
pause the stream, or switch replay mode without misleading labels.

#### Solution

Canonicalize session listing at the service boundary, make Open and New Factory
explicit select/confirm journeys, use navigation or truthful panel semantics,
and label stream pause separately from runtime lifecycle and historical mode.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update session listing, header tabs, Open/New dialogs, controls, focus,
  keyboard ordering, active-only status, and responsive overflow behavior.

##### Contracts

- `~default` remains a selector only; validation is non-mutating; stream pause
  and Factory lifecycle pause are separate actions; Live/Historical is explicit.

##### Services

- Factory Sessions owns alias reconciliation and returned canonical identity.

##### API changes

- Extend responses only if canonical selector/resolved identity cannot be
  expressed today; regenerate clients if changed.

##### Tests

- Alias plus UUID, singleton/multi-session, close/remap, keyboard tabs, Open
  existing, Create preview, broken path, cancel/focus, stream pause, and
  historical/current mode.

#### Acceptance criteria

- Each real session renders once and every action targets its canonical ID.
- Validation never claims to start a Factory; create previews the exact target.
- Tab/content/status semantics are truthful, keyboard operable, and non-color-only.

### Story 2: Make stream, replay, and session-switch state truthful

#### Problem statement

Zero-event sessions can load forever, retained data hides recovery failure,
historical mode is misdetected, and session A data/status can flash into B.

#### Customer ask

Know whether data is hydrating, empty, live, historical, stale, reconnecting, or
failed, while keeping retained data visible and safe.

#### Solution

Key stream state by backend/session/logical session/generation, add replay-ready
and freshness metadata, gate bento rendering on committed identity, guard late
callbacks, and pass one canonical state model to every dependent card.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update event-stream lifecycle, timeline/world-view selection, bento snapshots,
  Trace/Provider cache keys, stale banners, live regions, and retry actions.

##### Contracts

- State declares identity, hydrating, ready-known-empty, live, historical,
  stale, reconnecting, gap, recovery-failed, last event, and retryability.

##### Services

- Factory Sessions/Events provides replay-ready or watermark semantics.

##### API changes

- Add a stream-ready/replay-watermark envelope if existing events cannot signal
  completion; regenerate stream types.

##### Tests

- Zero-event readiness, retained offline/recovery, gap, live-history-live,
  A-to-B same IDs, late A event/status, stale Trace/Provider state, and retry.

#### Acceptance criteria

- Retained data is visibly stale and never suppresses retry or failure.
- Historical mode cannot mutate the Current Factory.
- A switch cannot render, cache, select, or announce prior-session data.

### Story 3: Define honest Work metrics and chart behavior

#### Problem statement

The outcomes chart repeats its Y-axis title and compares incompatible Work and
dispatch populations; Work totals repeats the ambiguity and stays four columns.

#### Customer ask

Read one clear, comparable, localized set of Work metrics in totals and chart.

#### Solution

Define each metric's subject, aggregation, unit, and included outcomes; remove
the duplicate label/grid, add localized axis/tooltip formatting and a textual
data alternative, and reflow totals at narrow widths.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update outcome/totals projections, chart axis/legend/tooltip/summary,
  keyboard inspection/zoom, indexed point building, and responsive totals.

##### Contracts

- Metrics declare Work versus dispatch, gauge versus cumulative, non-negative
  integer units, and treatment of accepted/continue/rejected/failed/retry/cancel.

##### Services

- Factory Visualization owns a typed aggregate only if canonical events are
  insufficient for one client projection.

##### API changes

- None unless backend-owned aggregates are required; then regenerate clients.

##### Tests

- Exactly one axis title/grid, metric truth table, zero/outlier/dense history,
  selected tick, localized tooltip, accessible table, keyboard zoom, full-width
  chart, and totals 2x2/1-column reflow.

#### Acceptance criteria

- Every number has one unambiguous subject, aggregation, and unit.
- The chart fills its card and has one axis title plus an accessible data view.
- Controls are keyboard operable, non-color-dependent, and at least 44 px.

### Story 4: Make Submit Work session-safe, bounded, and traceable

#### Problem statement

Simple drafts can cross sessions, simple success is silent, rich status is
capped by `max-w-xl`, file staging is unbounded whole-file base64 JSON, and
accepted/duplicate/runtime state is not retained.

#### Customer ask

Submit to a visible destination, attach content safely, avoid duplicate retries,
and keep a trustworthy receipt linking to Work and Trace.

#### Solution

Scope simple/rich draft and mutation state by exact session, make status full
width, align validation and form IDs, enforce published upload limits with
streaming/cancel, and keep a session-scoped idempotent receipt ledger.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update simple/rich composers, status panel, destination/receipt UI, form IDs,
  file picker/drop/paste, progress/cancel/retry, and safe previews.

##### Contracts

- Align required name/state semantics; define types, size/count/total limits,
  opaque staging tokens, cleanup, idempotency, and receipt lifecycle.

##### Services

- Work owns admission receipts and bounded content staging; host paths never
  enter client references.

##### API changes

- Author changes in component fragments, replace unbounded JSON staging with
  multipart/stream/chunks, and regenerate Go/TypeScript clients.

##### Tests

- A-to-B drafts and late completions, double submit, validation associations,
  duplicate widget IDs, full-width status, file limits/types/sniffing,
  progress/cancel/cleanup, ambiguous retry, receipt remount, and Work/Trace link.

#### Acceptance criteria

- Session A drafts/results never submit to or annotate B.
- Status fills the card and every path shows one localized success/error state.
- Staging is bounded, cancellable, path-opaque, and receipt identity is durable.

### Story 5: Preserve exact concurrent and terminal Work identity

#### Problem statement

Workstation activity collapses by transition; Active Work and Terminal Work
lose dispatch/Work identity, duplicate labels select together, canceled states
are absent, and history is unbounded or ambiguously ordered.

#### Customer ask

See every concurrent and terminal Work item once and open its exact execution.

#### Solution

Project Work participation by `(dispatch_id, work_id)`, Terminal Work by
`workId` plus terminal event, retain exact status/timing/failure/association,
and index/sort/page projections deterministically.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update replay/runtime activity, Active Work, Terminal Work, Current Selection
  actions, ordering, historical clocks, paging, and selection/history keys.

##### Contracts

- Labels are presentation only; completed, failed, canceled/interrupted, and
  terminated remain explicit terminal outcomes.

##### Services

- Consume canonical Work, dispatch, recordings, and Worker Session associations.

##### API changes

- Extend terminal projection with IDs/status/timing/associations if current
  label arrays cannot be replaced client-side; regenerate clients.

##### Tests

- Same workstation concurrency, duplicate names, one sibling completion,
  cancellation, historical scrub, exact row navigation, ordering, pagination,
  stale/loading/error, long labels, and Worker Session links.

#### Acceptance criteria

- Every participation/terminal Work appears once by immutable identity.
- Completing one sibling does not remove another and history freezes elapsed time.
- Rows open the exact Work, dispatch, trace, and Worker Sessions.

### Story 6: Expose authoritative Worker Sessions end to end

#### Problem statement

The backend has Worker Session observations, replay, lifecycle, and optional
provider identity, but the dashboard reconstructs lossy Provider Session rows
and cannot inspect provider-less sessions or retries.

#### Customer ask

Enumerate and inspect every Worker Session for selected Work and the Current
Factory Session, including attempts without Provider Sessions.

#### Solution

Make `workerSessionId` primary, expose exact detail and resumable events, model
attempt chronology explicitly, add UI APIs/hooks/cards/history, and keep Provider
Session as optional linked detail.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Extend Worker Sessions service/HTTP/mapping, recordings association replay,
  generated clients, UI adapters, selection kind/history, list/detail/events.

##### Contracts

- Detail includes Work, dispatch attempts, workstation, timing, lifecycle,
  failure, optional provider, cursor, gap/truncation, and replay completion.

##### Services

- Worker Sessions owns identity/attempts/observations; Events owns source-native
  live retention; Recordings owns durable associations.

##### API changes

- Add exact ID-based detail/events and cursor-paged Factory Session list; keep
  provider-reference lookup as compatibility; regenerate interfaces.

##### Tests

- Provider-less, concurrent sessions, multi-retry/provider switch, all terminal
  states, live/replay, resume/gap, selected Work and session-wide lists,
  back/forward, pagination, narrow/desktop.

#### Acceptance criteria

- Every Worker Session appears once by stable ID regardless of provider.
- Attempt chronology preserves every dispatch and association.
- Reconnect loses/duplicates no event and exposes gaps/known-empty/terminal state.

### Story 7: Make workstation and inspection history complete

#### Problem statement

Request history shows only the first Work, suppresses attempts, omits Worker
Session identity and output/mutation detail, and selected Work payload/relation
views can silently lose or misnavigate data.

#### Customer ask

Inspect one coherent request/dispatch/Work/Worker Session timeline with safe
payload and relation navigation.

#### Solution

Build a unified dispatch timeline with all Work, attempts, outputs, mutations,
failures, Worker/Provider links, exact identity, typed async states, and bounded
safe payload/relationship presentation.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update Current Selection projectors, request/detail sections, workstation
  summary/config draft source, Active Work, payload/content rendering, relation
  list/graph navigation, copy/download controls, and focus/history.

##### Contracts

- Dispatch/Work/Worker/Provider identities remain distinct; content exposes
  bounded/redacted metadata and authorized retrieval rather than raw file URLs.

##### Services

- Work owns content/lineage; Worker/Provider Sessions own their details;
  Factory Visualization may aggregate read-only timeline rows.

##### API changes

- Add missing worker session, tags/state/lineage, output/mutation, truncation, or
  authorized artifact fields where projections cannot recover them.

##### Tests

- Multi-Work requests, request plus attempts, provider-less attempt, retries,
  active-save guard, worker/provider change, malformed/unknown content, safe
  artifact retrieval, unresolved/cyclic relations, typed states, and paging.

#### Acceptance criteria

- No request or attempt suppresses another; every Work participation is visible.
- Summary and form use one draft source and active mutation has enforced policy.
- Payload/relation views are bounded, safe, accessible, and never silently no-op.

### Story 8: Keep Trace, Work, Worker, and Provider inspection coherent

#### Problem statement

Trace cache identity omits session generation, relation-only traces are hidden,
positional fallback invents causality, selected Work is not wired through, and
Provider Session content/context/navigation are inconsistent and unbounded.

#### Customer ask

Move through exact Work, dispatch, Trace, Worker Session, and Provider Session
context with truthful causality and reliable back/forward.

#### Solution

Key Trace by full stream identity, use canonical predecessor dispatch IDs,
thread selected Work/dispatch through graph/table, provide textual relations,
and separate Provider content identity from optional origin context with bounded
redacted transcript inspection.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update Trace query/selection, causal projection, relation/path graphs and
  table sync, Provider selection/entry points, retry/cache/abort, transcript,
  focus/history, and narrow graph shells.

##### Contracts

- Trace exposes canonical predecessor IDs, timing/status/provider/worker,
  unresolved/truncated metadata; Provider content key is `{provider,kind,id}`
  and origin is optional context.

##### Services

- Recordings owns causal Factory history; Worker/Provider Sessions own their
  normalized details and redaction.

##### API changes

- Add predecessor/worker/truncation/redaction pagination fields if missing;
  regenerate affected clients.

##### Tests

- A-to-B same IDs, relation-only trace, parallel/out-of-order/repeated paths,
  missing predecessor gap, row/graph sync, textual relation list, every Provider
  entry point, shared provider session, retry/abort, redaction, huge transcript.

#### Acceptance criteria

- Session switches cannot show old Trace/Provider content.
- The UI never invents sequential causality for independent dispatches.
- Graph and textual/table views share selection, identity, order, and context.

### Story 9: Unify editor transactions and complete grouping

#### Problem statement

Editor history covers layout only; topology/fields/groups use different rules;
observe mode can mutate authored geometry; group creation, visibility,
click-through, membership undo, validation, cancel, and keyboard behavior fail.

#### Customer ask

Treat every edit—including grouping—as one predictable draft with complete
Undo/Redo, save protection, cancel, and observe-mode immutability.

#### Solution

Create one editor document and domain command stream for topology, fields,
layout, group membership, and viewport intent; guard dirty exits/session changes
and stale saves. Implement group visuals/fit/sizing through the linked graph
visual plan rather than duplicating renderer ownership here.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update editor document/history/operations, selection bridge, add/connect/delete,
  group membership transaction, save/reset/discard, viewport policy, and guards.

##### Contracts

- Commands store domain transitions, not React Flow objects; all portable
  changes set dirty/stale protection; group bounds are finite and positive.

##### Services

- Factory Definitions remains save/validation owner.

##### API changes

- None unless server validation lacks required group geometry constraints.

##### Tests

- Mixed topology/field/layout/group history, reassignment undo, duplicate/cycle,
  cancel/pointercancel, failed confirmation, stale layout save, dirty session
  switch, observe immutability, save/reload, and linked group browser journeys.

#### Acceptance criteria

- Undo/Redo restores the entire prior/next document in LIFO order.
- Groups render in edit/observe/replay, do not block member/edge interaction,
  validate geometry, and support pointer/keyboard cancel and manipulation.
- Observe mode never persists node/group/viewport changes.

### Story 10: Make graph interaction progressive and contained

#### Problem statement

Add/connect/delete/selection modes are inconsistent, pending connections cannot
cancel, legends/controls omit semantics or collide, and React Flow failures can
escape without local recovery.

#### Customer ask

Use discoverable graph actions with clear eligibility, keyboard/touch parity,
safe overlays, and recovery that does not blank the dashboard.

#### Solution

Centralize typed graph actions and selection, expose progressive connection
intent/cancel, union/validate endpoints, reserve overlay insets, complete legend
semantics, and classify failures inside the graph card.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update action registry, dialogs/errors/focus, connection controller, handles,
  selection/delete confirmation, toolbar/legend/controls, Fit/group bounds,
  endpoint validation, and graph error boundary.

##### Contracts

- Unknown actions fail closed; every edge endpoint exists; duplicate/cycle
  eligibility is explicit; only proven integrity diagnostics are fatal.

##### Services

- Existing Factory document save/validation remains authoritative.

##### API changes

- None.

##### Tests

- Unknown/add/cancel/focus, valid/duplicate/cycle connect, Escape/pointer cancel,
  failed delete, keyboard/touch, 44 px controls, overlay geometry, complete
  legend, empty/loading/error/retry, large graph adapter budget.

#### Acceptance criteria

- Actions never silently fall through and pending gestures always cancel safely.
- Toolbar, legend, controls, and notices remain reachable without covering data.
- One graph projection/render failure stays inside its card with localized retry.

### Story 11: Make bento lifecycle and persistence safe

#### Problem statement

Duplicate graph cards race singleton editor state, removal loses dirty work,
persisted invalid/duplicate geometry survives, instance IDs are reused, layout
scope/errors are silent, and 640–768 retains unusable desktop geometry.

#### Customer ask

Add, remove, move, resize, reload, and reflow widgets without losing work,
breaking identity, or requiring pointer input.

#### Solution

Centralize widget metadata/policy, enforce graph singleton and instance caps,
use versioned scoped persistence with sanitization/collision repair/monotonic
IDs, add dirty-aware remove+undo/focus announcements, and provide keyboard/touch
move/resize/cancel plus coherent compact layout through 768 px.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Update typed catalog, add/remove UI, layout schema/store/migrations/mutations,
  AgentBento projection, focus/live region, collision policy, reduced motion,
  storage errors/reset, and high-card-count placement.

##### Contracts

- Envelope defines version/scope/items/counter; IDs are unique and monotonic;
  geometry is integer, positive, bounded, collision-repaired, and widget-specific.

##### Services

- None unless preferences move to operator settings after scope decision.

##### API changes

- None for local preference; use Operator Settings only by explicit product choice.

##### Tests

- Hostile JSON/IDs/geometry/overlap, migrations, storage failure, singleton/caps,
  dirty remove/undo/focus, add announcement, pointer/keyboard/touch/cancel,
  visual/DOM order, 320/390/640/768/desktop, reduced motion, and 100 cards.

#### Acceptance criteria

- Reload yields unique IDs, valid non-overlapping geometry, one Add card, and
  enforced singleton/caps with a recover/reset notice when sanitation occurs.
- Removal cannot silently lose dirty editor/form/staged state and restores focus.
- Compact layout is full width and usable through 768 without persisting it.

### Story 12: Standardize widget spacing, accessibility, locales, and scale

#### Problem statement

Current Selection and Trace share an outer frame but use inconsistent inner
rhythm; many controls miss 44 px; no general card error boundary exists; ja/ko
often fall back to English; dense cards are unbounded; unrelated cards rerender
on the one-second clock.

#### Customer ask

Use a coherent, resilient dashboard at every supported viewport, input mode,
locale, zoom level, and session size.

#### Solution

Adopt shared inspection-section/description-list/async-state primitives,
per-card boundaries, complete locale catalogs, 44 px and keyboard/touch rules,
responsive reflow/forced-color/reduced-motion recipes, bounded inspectors, and
scoped clock subscriptions with measured budgets.

#### Original document

`C:\Users\andre\work\portos\infinite-you\docs\internal\development\plans\dashboard-ux-probe-and-fix.md`

#### Changes

##### Package changes

- Migrate Current Selection/Trace sections, nested surfaces and descriptions;
  add card boundary/status primitives; complete ja/ko and formatters; paginate or
  virtualize Terminal/Trace/Provider/history; memoize cards and scope clocks.

##### Contracts

- Direct sections use 16 px block separation and 10 px internal gap; later
  sections use one divider plus 16 px top; nested surfaces use 12 px padding;
  descriptions stack below 640 and use 136 px labels above. Bounded reads expose
  total/range/cursor/truncation.

##### Services

- Owning services provide cursor paging where client retention is not the
  canonical complete set; technical diagnostics use stable codes.

##### API changes

- Add cursor/total/truncation/redaction fields only where required; regenerate
  clients. Authored UI copy remains client-owned.

##### Tests

- Every widget twice, unique IDs/headings, deliberate throw isolation, spacing
  geometry, 44 px/tab/focus/live roles, forced colors/reduced motion, all locales
  plus pseudo-long, 200% zoom, 20 widgets plus 1,000-row inspectors, rerender and
  memory/commit budgets.

#### Acceptance criteria

- Current Selection and Trace use one measurable inner rhythm and scroll owner.
- Every advertised locale is complete and every widget reflows without
  unintended page overflow or unreachable actions.
- One widget failure leaves siblings usable; dense data is bounded; clock ticks
  do not rerender time-independent cards.

## Project-level acceptance criteria

- Every probe failure is directly disproved by an owning story or linked
  graph-visual-plan criterion.
- Canonical Factory Session, stream generation, Work, dispatch, Worker Session,
  Provider Session, Factory Event, and Events-topic identities remain distinct.
- Network-backed cards expose truthful hydrating, known-empty, ready,
  historical, stale, reconnecting, gap, recovery-failed, retry, and success.
- Grouping passes create/membership/observe/click-through/validation/cancel/
  save-reload/compound-history journeys with linked graph visual acceptance.
- Every widget survives deliberate sibling failure, duplicate permitted
  instances have unique accessible identity, and singleton/cap rules are
  enforced before render.
- Every widget passes 320/390/640/768/desktop, 200% zoom, keyboard, touch,
  screen-reader, forced-colors, reduced-motion, focus restoration, live status,
  en/zh-CN/ja/ko, and pseudo-long copy.
- Generated OpenAPI/clients are regenerated from authored fragments; public
  contract changes pass smoke/drift checks.
- Dense/many-widget fixtures meet explicit render/memory/page budgets and
  unrelated cards do not rerender on the global clock.
- Delivery continues through terminal passing CI, blocking review resolution,
  conflict/generated-drift resolution, and merged PRs.

## Verification plan

Run story-focused tests first, then the relevant gates:

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

Before relying on graph-consuming component suites, rebuild the components
distribution and prove the `graph-edge` export resolves; a suite that cannot
start is blocked, not passing. For service changes, run focused Go
service/transport tests and functional paths through `root.BuildProcess` and
`Process.Execute`, preferring deterministic observation over sleeps.

## Definition of done

The program is done when a customer sees one canonical Current Factory Session,
opens/creates intentionally, reads honest Work metrics, receives full-width
session-safe submission receipts, sees every concurrent Work and Worker Session,
traces exact causal identity, edits and groups through one protected history,
and uses isolated, bounded, localized widgets and layouts at every supported
viewport/input mode. Required generated artifacts and tests are current, CI and
blocking review are resolved, conflicts are cleared, and implementation PRs are
merged.
