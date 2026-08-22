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

## After migration — lane audit and latency report

Recorded for story `BUN-UNIT-components-01-004` on the same host class as the
Vitest baseline (Windows 11 Home `10.0.26200`, i7-13700K, 24 logical
processors; Bun `1.3.12`). Repository revision for these measurements:
`2d1cff0a7`.

### Migrated / retained counts

| Lane ownership | Files | Named tests |
| --- | ---: | ---: |
| Migrated to Bun (exclusive) | 4 | 11 |
| Retained on Vitest | 0 | 0 |

Migrated files (exactly once under Bun):

| Migrated path | Named cases |
| --- | ---: |
| `ui/src/components/dashboard/fixtures/public-api.bun.unit.test.ts` | 1 |
| `ui/src/components/dashboard/fixtures/runtime.bun.unit.test.ts` | 4 |
| `ui/src/components/dashboard/test-fixtures.bun.unit.test.ts` | 2 |
| `ui/src/components/dashboard/typography.bun.unit.test.ts` | 4 |

Totals reconcile to the leased baseline: `4` files / `11` named tests. No
retained Vitest exceptions. The leased `.test.ts` paths no longer exist in the
tree after rename.

### Lane exclusivity audit

Focused Bun invocation (exactly four files / eleven named cases):

```text
bun test `
  src/components/dashboard/fixtures/public-api.bun.unit.test.ts `
  src/components/dashboard/fixtures/runtime.bun.unit.test.ts `
  src/components/dashboard/test-fixtures.bun.unit.test.ts `
  src/components/dashboard/typography.bun.unit.test.ts
```

Result:

```text
bun test v1.3.12 (700fc117)
src\components\dashboard\test-fixtures.bun.unit.test.ts:
(pass) dashboard/test-fixtures > does not re-export the canonical twenty-node topology fixture
(pass) dashboard/test-fixtures > keeps twentyNodeDashboardSnapshot wired to the canonical topology fixture
src\components\dashboard\typography.bun.unit.test.ts:
(pass) dashboard typography contract > documents Material scale mappings for shared dashboard roles
(pass) dashboard typography contract > documents the code extension and label roles beside Material families
(pass) dashboard typography contract > retires the repeated dashboard-only size literals for the covered roles
(pass) dashboard typography contract > raises body and supporting roles above the prior shared dashboard baseline
src\components\dashboard\fixtures\public-api.bun.unit.test.ts:
(pass) dashboard fixture catalog > exports documented fixture catalog entries for direct Storybook and Vitest imports
src\components\dashboard\fixtures\runtime.bun.unit.test.ts:
(pass) dashboard runtime fixtures > applies active work overlays without mutating the base topology
(pass) dashboard runtime fixtures > builds retry, failure, and rejected snapshots with observable session outcomes
(pass) dashboard runtime fixtures > projects multimodal selected-work payload onto active work item refs
(pass) dashboard runtime fixtures > composes semantic overlays against one shared topology
 11 pass
 0 fail
 46 expect() calls
Ran 11 tests across 4 files. [87.00ms]
```

Vitest `dashboard-unit` selection of the migrated paths (must be zero):

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/components/dashboard/fixtures/public-api.bun.unit.test.ts `
  src/components/dashboard/fixtures/runtime.bun.unit.test.ts `
  src/components/dashboard/test-fixtures.bun.unit.test.ts `
  src/components/dashboard/typography.bun.unit.test.ts
```

Result:

```text
No test files found, exiting with code 1
filter: ...public-api.bun.unit.test.ts, ...runtime.bun.unit.test.ts, ...test-fixtures.bun.unit.test.ts, ...typography.bun.unit.test.ts
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename. Conclusion: the leased suite
executes exactly once under Bun and zero times under Vitest. Aggregate focused
Bun cohort run and affected Vitest selection checks pass; `check:test-lanes`
and frontend typecheck also pass on this head.

### Focused after wall-clock (comparable warm-deps)

Command (same four migrated paths as above). Wrapper wall-clock uses
`[System.Diagnostics.Stopwatch]` around the command, matching the baseline.

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 0 (first focused) | 131ms | 88ms | 4 files / 11 tests passed |
| 1 | 127ms | 88ms | 4 files / 11 tests passed |
| 2 | 129ms | 92ms | 4 files / 11 tests passed |
| 3 | 124ms | 87ms | 4 files / 11 tests passed |

Median of the three warm comparable samples (runs 1–3): wrapper wall
`127ms`, Bun reported `88ms`. All runs reported exit code `0`.

### Before / after comparison

| Metric | Vitest baseline | Bun after |
| --- | ---: | ---: |
| Focused warm median wrapper wall | 1128 ms | 127 ms |
| Runner-reported warm median | 389 ms (Vitest) | 88 ms (Bun) |
| Files | 4 | 4 |
| Named tests | 11 | 11 |
| Expect calls (Bun after) | — | 46 |
| Retained on Vitest | — | 0 files / 0 tests |

These are matched four-file focused observations only; they are not a
repository-wide speedup claim.

### Changed-line budget

Against `origin/main` at measurement time the cohort patch is **5 files /
101 insertions / 4 deletions** (baseline evidence doc + four leased
rename/import migrations). Well within the ~1,000 changed-line budget; no
unsafe-coupling split required.
