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

## After-migration placeholder

Bun after timings, migrated/retained counts, and lane-exclusivity proof belong
in story `BUN-UNIT-features-current-selection-06-004` once migration completes.
They must use the same host class, worker/retry settings for any remaining
Vitest retained files, and a focused Bun invocation of the migrated
`.bun.unit.test.ts` files under comparable warm-deps conditions.
