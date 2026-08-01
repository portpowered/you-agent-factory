# Bun Unit Cohort Timing: features-trace-drilldown-02

This record captures the Vitest `dashboard-unit` baseline for the leased
trace-drilldown relation factory-graph cohort, the lane exclusivity audit after
partial migration, and the matched focused before/after wall-clock comparison.
The measurements are local diagnostic evidence for the migration contract; they
must not be converted into a repository-wide speedup claim.

## Workload

- Cohort work item: `BUN-UNIT-features-trace-drilldown-02`
- Leased Vitest files (before migration):
  - `ui/src/features/trace-drilldown/lib/trace-relation-factory-graph.test.ts`
  - `ui/src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts`
- File count: `2`
- Passing tests: `14`
- Named cases:

### `trace-relation-factory-graph.test.ts` (7)

1. `projectTraceRelationsToFactoryGraph endpoint nodes > maps relation endpoints to work-state nodes when state and work type are known`
2. `projectTraceRelationsToFactoryGraph endpoint nodes > derives work-state nodes from display-name work types when required state is present`
3. `projectTraceRelationsToFactoryGraph endpoint nodes > uses work_type_id from optional work items when projecting work-state nodes`
4. `projectTraceRelationsToFactoryGraph endpoint nodes > uses work-type nodes when relations have no required state`
5. `projectTraceRelationsToFactoryGraph edges and overlays > emits work-type-state edges with localized aria labels`
6. `projectTraceRelationsToFactoryGraph edges and overlays > aggregates relation metadata onto one node per endpoint`
7. `projectTraceRelationsToFactoryGraph edges and overlays > keeps canonical factory labels separate from trace overlay display labels`

### `trace-relation-factory-graph-flow.test.ts` (7)

1. `buildTraceRelationFactoryGraphFlow > projects batch relations into shared work nodes and clean edges`
2. `buildTraceRelationFactoryGraphFlow > registers only shared work graph React Flow node types`
3. `buildTraceRelationFactoryGraphFlow relation styling > renders relations with a shared clean edge style`
4. `buildTraceRelationFactoryGraphFlow relation styling > keeps required-state relations on the same clean edge style`
5. `buildTraceRelationFactoryGraphFlow repeated DEPENDS_ON > renders distinct dependency edges and nodes for each relation instance`
6. `buildTraceRelationFactoryGraphFlow selection > marks relation nodes selectable when onSelectWorkID is provided`
7. `buildTraceRelationFactoryGraphFlow selection > marks the selected work node as active and non-selectable`

## Runtime and host

- Recorded: 2026-08-01 UTC.
- Repository revision for the Vitest baseline: `7c573b37e`.
- Repository revision for lane audit + focused after: `a167aee35`.
- Bun: `1.3.12`.
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11, version `10.0.26200`; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-trace-drilldown-02`.

## Matched commands and raw results

### Vitest baseline

Command:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/trace-drilldown/lib/trace-relation-factory-graph.test.ts src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts
```

Raw result:

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-trace-drilldown-02/ui
Test Files  2 passed (2)
Tests  14 passed (14)
Start at 12:22:44
Duration  927ms (transform 666ms, setup 0ms, import 935ms, tests 32ms, environment 0ms)
[timing] elapsed_ms=1870
[timing] exit_code=0
```

### Bun after (migrated file)

Migrated file command:

```text
bun test src/features/trace-drilldown/lib/trace-relation-factory-graph.bun.unit.test.ts
```

Migrated file raw result (story `004` audit, revision `a167aee35`):

```text
bun test v1.3.12 (700fc117)
src\features\trace-drilldown\lib\trace-relation-factory-graph.bun.unit.test.ts:
(pass) ... 7 named cases ...
7 pass
0 fail
16 expect() calls
Ran 7 tests across 1 file. [63.00ms]
[timing] elapsed_ms=101
[timing] exit_code=0
```

### Retained Vitest exception: `trace-relation-factory-graph-flow.test.ts`

Decision (story `003`, confirmed in story `004`): keep this leased file on
Vitest. A filename/`bun:test` migration attempt fails before any case runs
because Bun resolves `@you-agent-factory/factory-graph` →
`@you-agent-factory/components/graphs` through package `exports` into
`dist/graphs/index.d.ts`, then cannot load `./graph-edge` from that types entry.
Vitest continues to resolve the same imports through dashboard source aliases.

Bun incompatibility probe (temporary `.bun.unit.test.ts` + `bun:test` /
`mock(() => {})` adaptation of the leased file; not retained in tree):

```text
bun test v1.3.12 (700fc117)
src\features\trace-drilldown\lib\_tmp-flow.bun.unit.test.ts:
# Unhandled error between tests
error: Cannot find module './graph-edge' from '.../components/dist/graphs/index.d.ts'
0 pass
1 fail
1 error
Ran 1 test across 1 file. [106.00ms]
```

Direct Bun probe of the retained Vitest path (story `004`, same failure class):

```text
bun test src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts
error: Cannot find module './graph-edge' from '.../components/dist/graphs/index.d.ts'
0 pass
1 fail
1 error
Ran 1 test across 1 file. [114.00ms]
```

Retained Vitest execution evidence (story `004` focused after, revision
`a167aee35`; 7 named cases unchanged):

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts
Test Files  1 passed (1)
Tests  7 passed (7)
Duration  818ms (transform 448ms, setup 0ms, import 656ms, tests 17ms, environment 0ms)
[timing] elapsed_ms=1425
[timing] exit_code=0
```

No broad Bun shim, assertion weakening, or production-source change was
introduced. Fixing this requires Bun unit-lane package/source resolution owned
by the foundation lane, not this leased cohort. Follow-up idea:
`tasks/ideas-to-review/ui/bun-unit-lane-workspace-package-source-resolution.md`.

## Lane exclusivity audit (story 004)

| Leased file | Lane ownership | Bun execution | Vitest `dashboard-unit` |
| --- | --- | --- | --- |
| `trace-relation-factory-graph.bun.unit.test.ts` | Bun unit | 7/7 pass once (`elapsed_ms=101`) | No test files found (excluded via `src/**/*.bun.unit.test.ts`) |
| `trace-relation-factory-graph-flow.test.ts` | Vitest retention | Fails before cases (`./graph-edge` from `dist/graphs/index.d.ts`) — not claimed as Bun ownership | 7/7 pass once (`elapsed_ms=1425`) |
| `trace-relation-factory-graph.test.ts` (pre-migration path) | Removed / renamed | n/a | No test files found (file absent) |

Audit conclusions:

- Migrated projection file executes exactly once under Bun and is not selected by
  Vitest `dashboard-unit`.
- Retained flow file executes exactly once under Vitest with the evidence-backed
  Bun package-resolution incompatibility above.
- No double-lane ownership for either leased case set.

## Focused before/after wall-clock comparison

Comparable host and runner versions. Before = single Vitest command over both
leased files. After = sequential Bun migrated file + Vitest retained exception
(same 14 named cases).

| Field | Vitest baseline | Focused after (story 004) |
| --- | --- | --- |
| Migrated files | 0 | 1 (`trace-relation-factory-graph`) |
| Retained Vitest files | 2 | 1 (`trace-relation-factory-graph-flow`) |
| Passing tests | 14 | 14 (7 Bun + 7 Vitest) |
| Wall-clock `elapsed_ms` | 1870 | 1512 (Bun then Vitest sequential) |
| Vitest duration summary | 927ms (2 files) | retained file alone 818ms / 1425ms wall |
| Bun duration summary | n/a | migrated file 63ms / 101ms wall |
| Patch size vs `7c573b37e` | n/a | 3 files, +186 / −1 lines (under ~1,000) |

Focused after command sequence:

```text
bun test src/features/trace-drilldown/lib/trace-relation-factory-graph.bun.unit.test.ts
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.test.ts
```

Focused after raw wall-clock:

```text
[timing] cohort_after_elapsed_ms=1512
[timing] bun_exit_code=0
[timing] vitest_exit_code=0
```

Observation for this cohort only: focused wall-clock improved from `1870ms` to
`1512ms` under comparable local conditions while preserving all 14 named cases
and assertion strength. The retained Vitest file still dominates after latency
because of the foundation package-resolution gap.
