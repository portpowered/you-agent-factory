# Chaining Trace Contract Data Model

This artifact records the contract-first decisions for `US-001` of the
`agent-factory-chaining-trace-propagation` PRD. It defines the shared chaining
lineage vocabulary, the deterministic fan-in rule, and the surfaces that later
stories must update without reinterpreting lineage differently at each
boundary.

## Change

- PRD, design, or issue: `prd.json` story `US-001`
- Owner: Agent Factory maintainers
- Packages or subsystems: `pkg/interfaces`, `pkg/factory`, `pkg/api`,
  `pkg/replay`, projections, generated contracts, and dashboard consumers
- Canonical architecture document to update before completion:
  `docs/processes/agent-factory-development.md`

## Trigger Check

- [x] Shared noun or domain concept
- [x] Shared identifier or resource name
- [ ] Lifecycle state or status value
- [ ] Shared configuration shape
- [x] Inter-package contract or payload
- [x] API, generated, persistence, or fixture schema
- [x] Scheduler, dispatcher, worker, or event payload
- [x] Package-local struct that another package must interpret

## Shared Vocabulary

| Name | Kind | Meaning | Canonical owner | Evidence |
| --- | --- | --- | --- | --- |
| `currentChainingTraceId` | contract field | The chain identifier declared for the work item or dispatch currently being represented. | Future public and runtime boundary fields; contract rule owned by `pkg/interfaces` lineage helpers and this artifact. | `pkg/interfaces/chaining_trace.go`, this artifact |
| `previousChainingTraceIds` | contract field | The explicit predecessor chain or chains that directly caused the current work or dispatch. | Future public and runtime boundary fields; deterministic ordering rule owned by `pkg/interfaces.CanonicalChainingTraceIDs(...)`. | `pkg/interfaces/chaining_trace.go`, focused contract tests |
| chaining fan-out | behavior rule | One consumed input chain leading to multiple outputs preserves the single predecessor chain on every output. | Runtime emission and event or projection adapters | `pkg/interfaces/chaining_trace_test.go` |
| chaining fan-in | behavior rule | Multiple consumed input chains preserve every unique predecessor chain instead of collapsing to one. | Runtime emission and event or projection adapters | `pkg/interfaces/chaining_trace_test.go` |

## Deterministic Fan-In Rule

`previousChainingTraceIds` must be constructed by:

1. Collecting every predecessor chaining trace from the consumed work inputs.
2. Dropping empty values.
3. Deduping repeated predecessor IDs.
4. Sorting the remaining IDs lexicographically ascending.

This rule is shared by runtime emission, canonical event serialization, and
world-state or dashboard projections. Input ordering must not affect the
published predecessor lineage.

## Surface Inventory

| Surface family | Fields required by the approved contract | Planned owner |
| --- | --- | --- |
| Submitted work boundaries | `currentChainingTraceId` on public submit payloads and normalized runtime submit or work records | `api/openapi-main.yaml`, generated `factoryapi.Work`, `interfaces.Work`, `interfaces.SubmitRequest` |
| Generated work boundaries | `currentChainingTraceId` and `previousChainingTraceIds` on generated work items and runtime work-history shapes | `interfaces.FactoryWorkItem`, generated `factoryapi.Work`, replay artifacts |
| Dispatch request boundaries | `currentChainingTraceId` for the dispatch chain plus `previousChainingTraceIds` for consumed predecessor chains | `interfaces.WorkDispatch`, dispatch event payloads, workstation-request projections |
| Canonical event boundaries | Explicit chaining fields on request, dispatch, and produced-work event payloads | generated factory event payloads and `pkg/factory/event_history.go` |
| Projection boundaries | Explicit chaining fields preserved without collapsing multi-input predecessors | `pkg/factory/projections`, `pkg/interfaces/factory_world_state.go`, dashboard consumers |

## Representative Examples

| Scenario | Inputs | Output `previousChainingTraceIds` |
| --- | --- | --- |
| Single-input fan-out | one consumed input with chain `trace-parent` | `["trace-parent"]` on every produced output |
| Multi-input fan-in | consumed inputs with chains `trace-z`, `trace-a`, `trace-z` | `["trace-a","trace-z"]` |

## Shared Package Or Package-Local Decision

- Shared interface, generated schema, contract package, or equivalent
  selected: `pkg/interfaces/chaining_trace.go` owns the deterministic
  predecessor-lineage helper used by runtime emission and projections.
- Package-local model selected: existing `trace_id`, `trace_ids`, and
  `parent_lineage` fields remain the current branch-local bridge until later
  stories rename or expand the public contract fields.
- Reason: `US-001` defines one contract and rule first so later boundary
  stories can thread the same semantics through API, runtime, event, replay,
  and world projections without inventing competing ordering rules.
- Translation boundary: later stories may keep legacy `trace_id` fields
  temporarily for compatibility, but any predecessor-chain list must be built
  through the shared helper.

## Reviewer Notes

- Applicable data-model construction artifact: this file.
- Package responsibility artifact:
  `docs/architecture/package-responsibilities.md`.
- Package interaction artifact: `docs/architecture/package-interactions.md`.
- Approved exceptions: none.
