# Bun Unit Cohort Baseline: features-current-factory-definition-02

This record freezes the Vitest `dashboard-unit` baseline for the leased
`current-factory-definition` workstation-helper cohort before Bun migration.
Later stories compare Bun focused timings against this matched workload.

## Workload

Leased files (5):

| File | Source `it` blocks | Vitest expanded tests |
| --- | ---: | ---: |
| `ui/src/features/current-factory-definition/lib/workstation-progress-outcome-routes.test.ts` | 20 | 20 |
| `ui/src/features/current-factory-definition/lib/workstation-worker-assignment.test.ts` | 4 | 4 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-editable-resolution.test.ts` | 5 | 5 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-model-invoke.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-type.test.ts` | 4 | 4 |
| **Total** | **41** | **41** |

There are no `it.each` expansions in this cohort, so the PRD inventory and the
executable Vitest total both match at **5 files / 41 passing tests**. Migration
must preserve all 41 named cases below.

## Named cases that must remain

### `workstation-progress-outcome-routes.test.ts` (20)

- `workstationSupportsProgressOutcomeRoutes > returns false for a standard model processor without stopWords`
- `workstationSupportsProgressOutcomeRoutes > returns false for a standard model processor with empty or whitespace-only stopWords`
- `workstationSupportsProgressOutcomeRoutes > returns true for a standard model processor with a trimmed stop word entry`
- `workstationSupportsProgressOutcomeRoutes > returns true for a repeater workstation without stopWords`
- `workstationSupportsProgressOutcomeRoutes > returns false for classifier workstations`
- `workstationSupportsProgressOutcomeRoutes > returns false for logical-move workstations`
- `workstationSupportsProgressOutcomeRoutes > returns true for a standard model processor with a trimmed worker stopToken`
- `workstationSupportsProgressOutcomeRoutes > returns false for a standard model processor with empty or whitespace-only worker stopToken`
- `workstationSupportsProgressOutcomeRoutes > returns true when both workstation stopWords and worker stopToken are configured`
- `workstationSupportsProgressOutcomeRoutes > returns false when neither workstation stopWords nor worker stopToken are configured`
- `workstationSupportsProgressOutcomeFailureRoute > returns false for logical-move workstations`
- `workstationSupportsProgressOutcomeFailureRoute > returns true for a standard model processor without stopWords`
- `workstationSupportsProgressOutcomeFailureRoute > returns true for a standard model processor with stopWords`
- `workstationHasZAxisIncompleteForConnections > returns true for a standard model processor without stopWords`
- `workstationHasZAxisIncompleteForConnections > returns false for a standard model processor with configured stopWords`
- `workstationHasZAxisIncompleteForConnections > returns false for a repeater workstation without stopWords`
- `workstationHasZAxisIncompleteForConnections > returns false for classifier workstations`
- `workstationHasZAxisIncompleteForConnections > returns false for a standard model processor with a trimmed worker stopToken`
- `workstationHasZAxisIncompleteForConnections > returns true when neither workstation stopWords nor worker stopToken are configured`
- `workstationHasZAxisIncompleteForConnections > returns false when both workstation stopWords and worker stopToken are configured`

### `workstation-worker-assignment.test.ts` (4)

- `workstationRequiresWorkerAssignment > returns false for LOGICAL_MOVE workstations even when a worker field is present`
- `workstationRequiresWorkerAssignment > returns true for MODEL_WORKSTATION fixtures used in graph-editor tests`
- `workstationRequiresWorkerAssignment > returns true for MODEL_INVOKE and CLASSIFIER_WORKSTATION fixtures`
- `workstationRequiresWorkerAssignment > defaults omitted workstation type to MODEL_WORKSTATION and requires a worker`

### `workstation/workstation-editable-resolution.test.ts` (5)

- `workstation editable resolution lookups > resolves canonical workstations by transition id or workstation name`
- `workstation editable resolution lookups > exposes worker catalog helpers for editable workstation forms`
- `workstation editable resolution lookups > returns no shared worker workstations when the selected workstation has no worker`
- `workstation editable resolution lookups > skips workstations without workers when building shared worker maps`
- `workstation editable resolution projections > projects editable guards, inputs, and worker type lookups`

### `workstation/workstation-model-invoke.test.ts` (8)

- `workstation model invoke type helpers > detects model invoke workstation types`
- `workstation model invoke type helpers > resolves compatible model workers and operations from the factory`
- `workstation model invoke binding projection > round-trips editable model invoke bindings`
- `workstation model invoke binding projection > round-trips config and default content bindings`
- `workstation model invoke binding validation > omits empty optional bindings from canonical projection`
- `workstation model invoke binding validation > accepts valid model-invoke bindings without validation errors`
- `workstation model invoke binding validation > validates required slots and duplicate bindings`
- `workstation model invoke binding validation > syncs binding rows to the selected operation input slots`

### `workstation/workstation-type.test.ts` (4)

- `workstation type helpers > defaults missing workstation types to AGENT_RUN`
- `workstation type helpers > limits LOGICAL_MOVE workstations to their current type`
- `workstation type helpers > allows conversion between AGENT_RUN and INFERENCE_RUN`
- `workstation type helpers > preserves legacy runnable workstation types in conversion options`

## Runtime and host

- Recorded: 2026-08-01T18:47:15Z (Vitest start) / wall-clock wrapper measured immediately after.
- Repository revision: `ccce5c32e` (branch after merging `origin/main`, including merged Bun unit foundation).
- Bun: `1.3.12`.
- Node: `v24.5.0`.
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-factory-definition-02`.

## Matched Vitest baseline command and raw results

Command (from `ui/`):

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 \
  src/features/current-factory-definition/lib/workstation-progress-outcome-routes.test.ts \
  src/features/current-factory-definition/lib/workstation-worker-assignment.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-editable-resolution.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-model-invoke.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-type.test.ts
```

Raw result:

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-current-factory-definition-02/ui
Test Files  5 passed (5)
Tests  41 passed (41)
Start at  11:47:15
Duration  715ms (transform 489ms, setup 0ms, import 656ms, tests 34ms, environment 1ms)
[timing] elapsed_ms=2417
[timing] exit_code=0
```

## Baseline summary

- Files: 5
- Passing Vitest tests (expanded): 41
- Source-level named cases: 41
- Vitest reported duration: 715ms
- Wrapper wall-clock elapsed: 2417ms
- Exit code: 0
