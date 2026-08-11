---
author: Agent Factory Team
last-modified: 2026-08-10
doc-id: agent-factory/plans/dashboard-ux-30-probe-results
---

# Dashboard UX 30-Probe Results

## Outcome

Thirty additional read-only probes decomposed the dashboard by bento card and
functional seam. They confirm the reported grouping, spacing, chart,
submission, session-tab, and Worker Session problems and identify the identity,
state, persistence, accessibility, and scale contracts that must be fixed with
them.

This evidence feeds
`docs/internal/development/plans/dashboard-ux-probe-and-fix.md`. Graph visual
grammar, graph-node sizing, group-region styling, and packaged renderer parity
remain owned by
`docs/internal/development/plans/factory-graph-visual-ux.md`.

## Method and limitations

- Probes ran in staged parallel batches over source, focused unit/component
  tests, OpenAPI fragments, services, replay projections, and browser stories.
- The batch was read-only: no source, Factory, Factory Session, Work, upload,
  selection, or external resource was mutated.
- Live browser execution was unavailable during this batch. Browser-sensitive
  findings remain explicit acceptance work at 320/390/640/768/desktop.
- Some graph-consuming Bun suites could not start because the checked-in
  components distribution references a missing `./graph-edge` artifact. A
  blocked suite is not counted as a product pass.

## Probe matrix

| # | Bento or function | Result | Highest-signal finding |
| --- | --- | --- | --- |
| 1 | Factory Session tabs | Fail | A UUID session and its `~default` selector can render as two rows; tabs also claim an empty panel and reuse one stream status for all tabs. |
| 2 | Open/New Factory | Fail | Open and create are conflated behind manual validation, and the primary action can validate without starting. |
| 3 | Session controls | Fail | Pause controls the client event stream rather than Factory lifecycle, and Live versus Historical mode is not truthful. |
| 4 | Work totals | Fail | Labels mix Work gauges, dispatch attempts, and Work failures; four fixed columns do not reflow. |
| 5 | Factory graph observe | Fail | Nodes/viewport remain mutable, concurrent Work collapses by workstation, and graph errors can escape the card. |
| 6 | Grouping | Fail | Groups are empty-first, disappear outside edit mode, block click-through, lose prior membership on undo, accept invalid geometry, and lack keyboard/cancel semantics. |
| 7 | Graph Add | Fail | Unknown actions fall through to Workstation, cancel leaves Add active, errors lack associations, and add is outside Undo/Redo. |
| 8 | Connect/disconnect | Fail | Pending connection lacks explicit cancel, duplicates are silent, and topology edits are outside history. |
| 9 | Selection/delete | Fail | Observer/editor/group selection disagree, cleanup can run after failed confirmation, and delete is not undoable. |
| 10 | Editor save/reset/history | Fail | History is layout-only, layout-only saves bypass stale protection, and session changes can discard dirty state. |
| 11 | Move/resize/viewport | Fail | Observe movement/viewport can persist, graph-node resize is absent, cancel is incomplete, and Fit ignores groups. |
| 12 | Legend/controls | Fail | Semantics are incomplete, targets are undersized, disabled reasons are unreachable, and overlays collide on narrow canvases. |
| 13 | Current Selection spacing | Fail | The outer frame matches Trace, but inner sections use three incompatible spacing/divider contracts and fixed description columns. |
| 14 | Workstation summary/config | Fail | Active-Work save has no real confirmation, summary and form read different state, and provider metadata can remain tied to the old worker. |
| 15 | Active Work | Fail | Rows lose `(dispatch_id, work_id)` identity, can select the wrong attempt, and keep aging in history. |
| 16 | Request history/detail | Fail | Only the first Work is shown, requests suppress attempt history, Worker Session identity is missing, and direct request detail drops Provider navigation. |
| 17 | Selected Work/payload/relations | Fail | State/tags/lineage are incomplete, binary retrieval is undefined, malformed parts can disappear, and relation navigation can silently no-op. |
| 18 | Worker Sessions | Fail | Backend foundations exist, but the UI does not consume them; provider-less identity, attempts, resumable cursors, detail, and history are absent. |
| 19 | Provider Session | Fail | Origin context and entry points are incomplete; selection conflates content with dispatch; transcript rendering is unbounded and lacks a redaction policy. |
| 20 | Terminal Work | Fail | Rows are keyed by display label, so duplicate-name Work can collapse/select together; canceled states, paging, and Worker links are absent. |
| 21 | Factory outcomes chart | Fail | The Y-axis title renders twice, and series compare incompatible Work/dispatch populations under one unit. |
| 22 | Submit composer | Fail | Simple drafts cross sessions, simple success is silent, status width is capped, and file staging is unbounded. |
| 23 | Submit files/status | Fail | No size/count policy, progress/cancel, idempotent receipt ledger, durable Work link, or opaque staging reference exists. |
| 24 | Trace shell/states | Fail | Real loading/error states are mostly unreachable, stale/reconnect is absent, identity is incomplete, and graphs overflow/trap scroll. |
| 25 | Trace relation graph | Fail | Sessionless caching can show stale data, selected Work is not wired through, relation-only traces are hidden, and no textual alternative exists. |
| 26 | Trace workstation path/table | Fail | Positional fallback invents causality for out-of-order/parallel dispatches; rows and graph are unsynchronized and omit execution detail. |
| 27 | Bento add/remove/duplicates | Fail | Duplicate graph cards race singleton editor state; removal is destructive; invalid duplicate IDs/singletons survive; instance IDs can be reused. |
| 28 | Bento drag/resize/persistence | Fail | Invalid geometry/overlap survives, 640–768 keeps cramped desktop geometry, movement is pointer-only, and storage scope/errors are undefined. |
| 29 | Loading/reconnect/session switch | Fail | Retained data hides failure, known-empty is ambiguous, A-to-B can flash A data, history is misdetected, and late callbacks can overwrite status. |
| 30 | Accessibility/localization/performance | Fail | No general card boundary or keyboard layout editing exists; targets miss 44 px; ja/ko fall back; unrelated cards tick; dense inspectors are unbounded. |

## Cross-cutting conclusions

### Carry canonical identity end to end

Use Factory Session plus stream generation for runtime caches,
`(dispatch_id, work_id)` for workstation activity, `workerSessionId` for Worker
Session inspection, and `workId` for Terminal Work. Keep Provider Session
content identity separate from optional origin context.

### Model async state rather than infer it from rows

Hydrating, known-empty, live, historical, stale, reconnecting, gap, and
recovery-failed are distinct. Retained data must remain visible but marked
stale, and mutation controls must remain disabled in historical mode.

### Use one transactional editor document

Topology, fields, layout, selection, grouping, and viewport need one dirty,
stale-save, cancel, validation, and Undo/Redo model. Group visual behavior stays
in the graph-visual plan; the transaction seam belongs in the dashboard plan.

### Give bento persistence a lifecycle contract

Persisted layouts need versioning, scope, identity/geometry sanitization,
collision repair, instance limits, dirty-aware removal, focus restoration, and
error recovery. Factory graph must be singleton until ownership is per instance.

### Bound and isolate inspection

Terminal Work, Current Selection, Provider transcripts, Trace, and payloads
need pagination/windowing, safe retrieval/redaction, focus continuity, card-local
error recovery, and measured render budgets.

## Required regression matrix

- Every canonical bento type and every permitted duplicate.
- 320/390/640/768/desktop, 200% zoom, keyboard, touch, forced colors, and
  reduced motion.
- en/zh-CN/ja/ko and pseudo-long copy.
- Empty, loading, known-empty, stale, reconnecting, recovery-failed,
  historical, current, and session-switch races.
- Duplicate labels, concurrent dispatches, provider-less Worker Sessions,
  retries, canceled Work, relation-only traces, malformed replay, and hostile
  persisted layouts.
- Dense graph/history/trace/transcript/payload and many-widget budgets.
