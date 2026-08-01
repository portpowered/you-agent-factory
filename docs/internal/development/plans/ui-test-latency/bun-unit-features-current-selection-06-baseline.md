# Bun Unit Cohort Baseline: features-current-selection-06

Vitest baseline for the leased selected-work relationship graph/relations unit
cohort before Bun migration. Recorded so later stories can prove named-case
parity and comparable focused wall-clock change. No test files were changed for
this measurement.

## Workload

Leased files (4):

- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-graph.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-relations.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.test.ts`

Passing tests: **9** across **4** files.

### Named case inventory (parity baseline)

1. `buildSelectedWorkRelationshipGraph > builds the full connected relationship graph around the selected work item`
2. `buildSelectedWorkRelationshipGraph > returns an explicit empty graph when no supported relationships exist`
3. `buildSelectedWorkRelationshipGraph > returns an explicit error state when relationship data is unavailable`
4. `buildSelectedWorkRelationshipGraph repeated DEPENDS_ON > preserves every distinct DEPENDS_ON relationship from the selected work item`
5. `factory-batch-local-agent-cli-runtime relationship graph > preserves every DEPENDS_ON relation from the loopback work item in the smoke test fixture`
6. `projectSelectedWorkRelationshipGraphToDashboardRelations > projects ready relationship graphs from direct relations when available`
7. `projectSelectedWorkRelationshipGraphToDashboardRelations > returns no relations for loading, error, empty, or missing graphs`
8. `projectSelectedWorkRelationshipGraphToDashboardRelations > projects repeated dependency edges when direct relations are unavailable`
9. `projectSelectedWorkRelationshipGraphToDashboardRelations repeated DEPENDS_ON > projects every dependency relation instance from a ready selected-work graph`

## Runtime and host

- Recorded: 2026-08-01 UTC.
- Repository revision: `7c573b37e`.
- Bun: `1.3.12` (used only to invoke `bunx vitest` for this baseline).
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-selection-06`.

## Matched Vitest command and raw result

Comparable conditions: Vitest `dashboard-unit` project, `--maxWorkers=4`,
`--retry=0`, focused file list only (no coverage, no file parallelism changes).
Wall-clock is process elapsed time around the command (same pattern as
`bun-unit-seed-timing.md`).

Command:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/current-selection/work-selection/lib/selected-work-relationship-graph.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.test.ts
```

Raw result (primary timing sample):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-current-selection-06/ui
Test Files  4 passed (4)
Tests  9 passed (9)
Start at 12:25:58
Duration  389ms (transform 189ms, setup 0ms, import 281ms, tests 48ms, environment 0ms)
[timing] elapsed_ms=1360
[timing] exit_code=0
```

Focused wall-clock baseline used for later before/after comparison:
**1360 ms** process elapsed (`[timing] elapsed_ms`).

Verbose confirmation of the nine named cases was re-run immediately after under
the same command with `--reporter=verbose`; all nine cases listed above passed
(`Duration 352ms` Vitest-internal; not used as the wall-clock baseline).

## After migration — lane audit and latency report

Recorded for story `BUN-UNIT-features-current-selection-06-004` on the same
host class as the Vitest baseline (Windows 11 Home `10.0.26200`, i7-13700K,
24 logical processors; Bun `1.3.12`). Repository revision for these
measurements: `42b45e12f`.

### Migrated / retained counts

| Lane ownership | Files | Named tests |
| --- | ---: | ---: |
| Migrated to Bun (exclusive) | 4 | 9 |
| Retained on Vitest | 0 | 0 |

Migrated files:

- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-graph.bun.unit.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.bun.unit.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-relations.bun.unit.test.ts`
- `ui/src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.bun.unit.test.ts`

No retained exceptions. The leased Vitest `*.test.ts` paths no longer exist in
the tree after rename.

### Lane exclusivity audit

Focused Bun invocation (exactly four files / nine named cases):

```text
bun test src/features/current-selection/work-selection/lib/selected-work-relationship-graph.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.bun.unit.test.ts
```

Result:

```text
bun test v1.3.12 (700fc117)
src\features\current-selection\work-selection\lib\selected-work-relationship-graph.bun.unit.test.ts:
(pass) buildSelectedWorkRelationshipGraph > builds the full connected relationship graph around the selected work item
(pass) buildSelectedWorkRelationshipGraph > returns an explicit empty graph when no supported relationships exist
(pass) buildSelectedWorkRelationshipGraph > returns an explicit error state when relationship data is unavailable
src\features\current-selection\work-selection\lib\selected-work-relationship-graph.instances.bun.unit.test.ts:
(pass) buildSelectedWorkRelationshipGraph repeated DEPENDS_ON > preserves every distinct DEPENDS_ON relationship from the selected work item
(pass) factory-batch-local-agent-cli-runtime relationship graph > preserves every DEPENDS_ON relation from the loopback work item in the smoke test fixture
src\features\current-selection\work-selection\lib\selected-work-relationship-relations.bun.unit.test.ts:
(pass) projectSelectedWorkRelationshipGraphToDashboardRelations > projects ready relationship graphs from direct relations when available
(pass) projectSelectedWorkRelationshipGraphToDashboardRelations > returns no relations for loading, error, empty, or missing graphs
(pass) projectSelectedWorkRelationshipGraphToDashboardRelations > projects repeated dependency edges when direct relations are unavailable
src\features\current-selection\work-selection\lib\selected-work-relationship-relations.instances.bun.unit.test.ts:
(pass) projectSelectedWorkRelationshipGraphToDashboardRelations repeated DEPENDS_ON > projects every dependency relation instance from a ready selected-work graph
 9 pass
 0 fail
 26 expect() calls
Ran 9 tests across 4 files. [72.00ms]
```

Vitest `dashboard-unit` selection of the migrated paths (must be zero):

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/current-selection/work-selection/lib/selected-work-relationship-graph.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-graph.instances.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.bun.unit.test.ts src/features/current-selection/work-selection/lib/selected-work-relationship-relations.instances.bun.unit.test.ts
```

Result:

```text
No test files found, exiting with code 1
filter: ...selected-work-relationship-*.bun.unit.test.ts
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename. Conclusion: each leased suite
executes exactly once under Bun and zero times under Vitest.

### Focused after wall-clock (comparable warm-deps)

Command (same four migrated paths as the exclusivity Bun invocation above).

Warm re-runs after one discarded warm-up (same host / Bun version as baseline):

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 1 | 111ms | 76ms | 4 files / 9 tests passed |
| 2 | 108ms | 77ms | 4 files / 9 tests passed |
| 3 | 105ms | 73ms | 4 files / 9 tests passed |

Median warm wrapper wall-clock after migration: **108 ms**.

### Before / after comparison

| Metric | Vitest baseline | Bun after |
| --- | ---: | ---: |
| Focused wrapper wall | 1360 ms | 108 ms (warm median) |
| Files | 4 | 4 |
| Named tests | 9 | 9 |
| Expect calls (Bun after) | — | 26 |

These are matched four-file focused observations only; they are not a
repository-wide speedup claim. The Vitest baseline uses the primary process
elapsed sample recorded in story 001 (`elapsed_ms=1360`); the Bun after value
is the median of three warm wrapper samples under the same host class.

### Changed-line budget

Against `origin/main` including this lane-audit section the cohort patch is
**5 files / 184 insertions / 4 deletions** (baseline+report note + rename/import
of the four leased tests). Well within the ~1,000 changed-line budget; no split
required.
