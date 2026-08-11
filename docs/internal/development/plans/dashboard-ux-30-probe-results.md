---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/dashboard-ux-30-probe-results
---

# Dashboard UX 30-Probe Results

## Outcome

Thirty additional read-only probes decomposed the dashboard by bento card and
functional seam. The probes confirm that the reported grouping, spacing,
chart, submission, session-tab, and Worker Session problems are reproducible,
and they identify the identity, state, persistence, accessibility, and scale
contracts that must be fixed with them.

This evidence is an input to
`docs/internal/development/plans/dashboard-ux-probe-and-fix.md`. Graph visual
grammar, graph-node sizing, group-region styling, and packaged renderer parity
remain owned by
`docs/internal/development/plans/factory-graph-visual-ux.md`.

## Probe method and limitations

- Probes were executed in staged parallel batches against repository source,
  focused unit/component tests, OpenAPI fragments, service contracts, replay
  projections, and existing browser stories.
- Each probe was read-only. No repository source, Factory definition, Factory
  Session, Work, upload, selection, or external resource was mutated.
- Live browser execution was unavailable during this batch. Browser-sensitive
  findings are therefore marked for explicit 320/390/640/768/desktop browser
  acceptance rather than treated as visually proven.
- Some graph-consuming Bun component suites could not start because the
  checked-in components distribution references a missing `./graph-edge`
  artifact. Passing focused suites and blocked suites are recorded separately;
  a blocked suite is never counted as a product pass.

## Probe matrix

| # | Bento or function | Result | Highest-signal finding |
| --- | --- | --- | --- |
| 1 | Factory Session tabs | Fail | The backend can return a UUID session and its `~default` selector as separate rows; tabs also claim an empty `tabpanel` and reuse one active-stream status for every tab. |
| 2 | Open/New Factory | Fail | Open and create are conflated behind manual path validation; the primary action can validate without starting and target rows open immediately instead of supporting select-then-confirm. |
| 3 | Session controls | Fail | Pause is a client event-stream pause, not Factory Session lifecycle pause, and the UI does not truthfully distinguish Live from Historical mode. |
| 4 | Work totals | Fail | Labels mix current Work gauges, dispatch attempts, and Work failures; the fixed four-column layout does not reflow at narrow widths. |
| 5 | Factory graph observe mode | Fail | Nodes and viewport remain mutable in observe mode; concurrent Work can collapse by workstation; projection errors can escape without card-local recovery. |
| 6 | Factory graph grouping | Fail | Groups are empty-first, disappear in observe/replay, block click-through, do not restore prior membership on undo, accept invalid geometry, and lack keyboard move/resize. |
| 7 | Graph Add workflow | Fail | Unknown action IDs can create a Workstation, cancel leaves the tool active, errors lack field associations, and topology add is outside Undo/Redo. |
| 8 | Connect/disconnect | Fail | Pending connection cannot be explicitly canceled, duplicates are silent, and connect/disconnect are outside the editor command history. |
| 9 | Selection/delete | Fail | Observer/editor/group selection are inconsistent; successful cleanup can be applied after a failed confirmation; topology delete is not undoable. |
| 10 | Editor save/reset/discard/history | Fail | History is layout-only, layout-only saves bypass stale protection, session changes can discard dirty state, and field edits appear uncommitted. |
| 11 | Layout/move/resize/viewport | Fail | Observe-mode movement can commit authored positions, observe pan/zoom writes authored viewport, graph-node resize is unimplemented, and Fit ignores groups. |
| 12 | Graph legend/controls | Fail | Legend omits important semantics, controls miss the 44 px target, disabled reasons are unreachable, overlays collide, and expanded legend content can obscure narrow canvases. |
| 13 | Current Selection shell/spacing | Fail | The outer frame matches Trace, but internal sections use three incompatible spacing/divider contracts, fixed description columns, undersized controls, and weak narrow-width behavior. |
| 14 | Workstation summary/config | Fail | Saving during active Work has no real confirmation/enforcement; summary and form read different state; provider metadata can remain tied to the previous worker. |
| 15 | Workstation Active Work | Fail | Rows lose `(dispatch_id, work_id)` identity, can select the wrong concurrent attempt, use ambiguous counts/order, and keep aging in historical mode. |
| 16 | Workstation Request history/detail | Fail | Only the first Work is shown, requests suppress attempt history, Worker Session identity is missing, and direct request detail drops Provider Session navigation. |
| 17 | Selected Work/payload/relations | Fail | Work state/tags/lineage are incomplete, binary content has no safe retrieval path, malformed parts can disappear, relation navigation can silently no-op, and lists are unbounded. |
| 18 | Worker Sessions | Fail | The backend has list/replay/observation foundations, but the UI never consumes them; provider-less sessions, exact Worker Session identity, resumable cursors, retries, and back/forward are absent. |
| 19 | Provider Session | Fail | Query identity is sound, but origin context and entry points are incomplete; selection conflates content with dispatch origin; transcripts are unbounded and lack an explicit redaction/reveal policy. |
| 20 | Terminal Work | Fail | Terminal rows are keyed by display label, so duplicate-name Work can collapse or select together; canceled states, stable recency, paging, and Worker Session links are absent. |
| 21 | Factory outcomes chart | Fail | The Y-axis title is rendered twice, and the four series compare incompatible Work/dispatch populations under one `Work count` unit; no accessible data alternative exists. |
| 22 | Submit Work composer/validation | Fail | Simple drafts can cross Factory Session boundaries, simple success is silent, status width is capped, file staging is unbounded, and initial Work state is not a supported choice. |
| 23 | Submit Work files/status | Fail | No size/count policy, true upload progress, cancellation, idempotent receipt ledger, durable Work link, or opaque staging reference exists; whole files are base64-buffered in JSON. |
| 24 | Trace shell/states/spacing | Fail | Shared outer spacing is correct, but real loading/error states are mostly unreachable, stale/reconnect states are absent, query identity is incomplete, and embedded graphs overflow/trap scroll. |
| 25 | Trace relation graph | Fail | Sessionless caching can expose stale trace data, selected Work is not threaded into production, relation-only traces are hidden, relation types are visually indistinguishable, and no textual alternative exists. |
| 26 | Trace workstation path/table | Fail | Positional fallback invents causality for out-of-order or parallel dispatches; rows and graph are unsynchronized and omit Worker Session/provider/timing/failure detail. |
| 27 | Bento add/remove/duplicates | Fail | Duplicate Factory graph cards race singleton editor state; removal is immediately destructive; persisted duplicate IDs/singletons survive; instance IDs can be reused; duplicate limits are invisible. |
| 28 | Bento drag/resize/persistence | Fail | Persistence accepts invalid geometry and overlaps, the 640–768 px band keeps unusable desktop geometry, movement is pointer-only, storage failures are silent, and layout scope is global. |
| 29 | Cross-bento loading/reconnect/session switch | Fail | Retained data hides recovery failure, known-empty cannot be distinguished from loading, A-to-B switches can flash A data, historical mode is misdetected, and late stream callbacks can overwrite status. |
| 30 | Cross-bento accessibility/localization/performance | Fail | No general per-card error boundary or keyboard layout editing exists; targets are often below 44 px; ja/ko fall back to English; unrelated cards rerender each second; dense inspectors are unbounded. |

## Critical cross-cutting findings

### Canonical identity must be carried end to end

The highest-risk defects repeatedly replace immutable identity with display
labels or partial keys. Required keys include Factory Session and stream
generation for cached runtime state, `(dispatch_id, work_id)` for workstation
activity, `workerSessionId` for Worker Session inspection, and `workId` for
Terminal Work and relation navigation. Provider Session content identity must
remain separate from optional dispatch/Work/Worker Session origin context.

### Loading, empty, historical, stale, and failed are distinct states

The dashboard currently infers state from the presence of cached rows. This can
make retained offline data look live, keep zero-event sessions loading forever,
hide recovery actions, and enable mutations while inspecting history. The
stream identity must publish replay-ready/known-empty, freshness, gap,
reconnecting, and retryable failure state to every dependent card.

### Editor operations need one transactional document model

Layout, topology, fields, selection, grouping, and viewport currently have
different mutation and history rules. A single editor document and operation
stream must define dirty state, stale-save protection, Undo/Redo, cancel,
validation, session-change guards, and observe-mode immutability. Group visual
behavior and node sizing stay in the graph-visual plan; the shared transaction
seam belongs in the dashboard plan.

### Bento persistence and lifecycle need an explicit contract

Persisted layouts require versioned envelopes, identity and geometry
sanitization, deterministic collision repair, documented operator/factory
scope, error reporting, instance caps, and a dirty-aware add/remove lifecycle.
Factory graph must be singleton until editor ownership is instance-scoped.

### Inspection surfaces need bounded, safe presentation

Terminal Work, Current Selection histories, Provider Session transcripts,
Trace rows/graphs, and Work payloads can all render without meaningful bounds.
They need pagination, virtualization, progressive disclosure, safe redaction
and retrieval contracts, focus continuity, and measured render budgets.

## Verification matrix generated by the probes

The implementation plan must verify:

- every canonical bento type, including two instances where policy permits;
- 320, 390, 640, 768, and desktop widths, plus 200% zoom;
- keyboard-only and touch operation, 44 px targets, reduced motion, forced
  colors, focus restoration, and live announcements;
- English, Simplified Chinese, Japanese, Korean, and a pseudo-long locale;
- empty, loading, known-empty, stale, reconnecting, recovery-failed,
  historical, current, and session-switch races;
- duplicate display labels, concurrent dispatches, provider-less Worker
  Sessions, multiple retries, canceled Work, relation-only traces, malformed
  retained records, and duplicate persisted widget IDs; and
- dense fixtures for graphs, terminal histories, traces, transcripts, Work
  payloads, and many dashboard widgets with explicit time/memory budgets.

## Immediate sequencing recommendation

1. Fix canonical session/stream/history detection and prevent cross-session
   stale rendering or mutation.
2. Establish authoritative Worker Session identity, attempts, associations,
   resumable observations, and dashboard consumers.
3. Unify editor operation history and guard grouping/topology/layout saves.
4. Add bento sanitization, singleton/duplicate policy, dirty-aware removal, and
   per-card error isolation.
5. Land the narrow Y-axis and Submit status width fixes while defining their
   honest metric/submission-state contracts.
6. Complete bounded inspection, responsive/accessibility, localization, and
   measured performance work.
