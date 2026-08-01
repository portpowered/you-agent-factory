# Bun Unit Cohort Baseline: components-01

This record freezes the Vitest `dashboard-unit` before-state for the leased
four-file dashboard fixture/typography cohort. It is measurement evidence for
the migration contract; it does not invent product behavior.

## Workload

Exact leased paths (4 files, 11 named cases):

| File | Named cases |
| --- | ---: |
| `ui/src/components/dashboard/fixtures/public-api.test.ts` | 1 |
| `ui/src/components/dashboard/fixtures/runtime.test.ts` | 4 |
| `ui/src/components/dashboard/test-fixtures.test.ts` | 2 |
| `ui/src/components/dashboard/typography.test.ts` | 4 |

Totals: `1 + 4 + 2 + 4 = 11`.

### Named-case inventory

`fixtures/public-api.test.ts` (1):

1. exports documented fixture catalog entries for direct Storybook and Vitest imports

`fixtures/runtime.test.ts` (4):

1. applies active work overlays without mutating the base topology
2. builds retry, failure, and rejected snapshots with observable session outcomes
3. projects multimodal selected-work payload onto active work item refs
4. composes semantic overlays against one shared topology

`test-fixtures.test.ts` (2):

1. does not re-export the canonical twenty-node topology fixture
2. keeps twentyNodeDashboardSnapshot wired to the canonical topology fixture

`typography.test.ts` (4):

1. documents Material scale mappings for shared dashboard roles
2. documents the code extension and label roles beside Material families
3. retires the repeated dashboard-only size literals for the covered roles
4. raises body and supporting roles above the prior shared dashboard baseline

## Runtime and host

- Recorded: 2026-08-01 UTC
- Repository revision: `aabd48eb7` (`BUN-UNIT-components-01` after merging current `main`, including `BUN-UNIT-00-lane-foundation`)
- Bun: `1.3.12`
- Vitest: `4.1.3`
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-components-01`

## Matched Vitest command

From `ui/`:

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/components/dashboard/fixtures/public-api.test.ts `
  src/components/dashboard/fixtures/runtime.test.ts `
  src/components/dashboard/test-fixtures.test.ts `
  src/components/dashboard/typography.test.ts
```

Wrapper wall-clock is measured around that command with
`[System.Diagnostics.Stopwatch]`.

## Comparable baseline runs

| Run | Wrapper wall | Vitest wall | Transform | Import | Tests | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 (first focused) | 1293ms | 384ms | 502ms | 645ms | 15ms | 4 files / 11 tests passed |
| 2 | 1128ms | 473ms | 502ms | 644ms | 15ms | 4 files / 11 tests passed |
| 3 | 1081ms | 389ms | 497ms | 639ms | 15ms | 4 files / 11 tests passed |
| 4 | 1166ms | 386ms | 516ms | 654ms | 16ms | 4 files / 11 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`1128ms`, Vitest wall `389ms`. All runs reported exit code `0`.

Representative raw reporter output (run 3):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-components-01/ui
Test Files  4 passed (4)
Tests  11 passed (11)
Start at  12:08:53
Duration  389ms (transform 497ms, setup 0ms, import 639ms, tests 15ms, environment 0ms)
[timing] elapsed_ms=1081
[timing] exit_code=0
```

## After-state placeholder

Focused Bun after-state wall-clock for the same four files will be recorded in
story `BUN-UNIT-components-01-004` under conditions comparable to this baseline.
