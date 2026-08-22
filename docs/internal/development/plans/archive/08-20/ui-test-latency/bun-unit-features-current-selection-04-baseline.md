# Bun Unit Cohort Baseline: features-current-selection-04

Vitest baseline for the leased current-selection request-helpers unit cohort
before Bun migration. Recorded so later stories can prove named-case parity and
comparable focused wall-clock change. No test files were changed for this
measurement.

## Workload

Leased file (1):

- `ui/src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.test.ts`

Passing tests: **5** across **1** file.

### Named case inventory (parity baseline)

1. `useCurrentSelection.request-helpers > resolves projected requests from explicit maps or runtime snapshots`
2. `useCurrentSelection.request-helpers > filters provider-session attempts and selects the latest session per dispatch in request order`
3. `useCurrentSelection.request-helpers > sorts and selects workstation requests by started time and related work items`
4. `useCurrentSelection.request-helpers > derives selected-work dispatch attempts from requests and merges them with provider attempts`
5. `useCurrentSelection.request-helpers > converts runtime requests to projected requests and exposes request-owned work items`

## Runtime and host

- Recorded: 2026-08-01 UTC.
- Repository revision: `7c573b37e`.
- Bun: `1.3.12` (used only to invoke `bunx vitest` for this baseline).
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-selection-04`.

## Matched Vitest command and raw result

Comparable conditions: Vitest `dashboard-unit` project, `--maxWorkers=4`,
`--retry=0`, focused file list only (no coverage, no file parallelism changes).
Wall-clock is process elapsed time around the command (same pattern as
`bun-unit-seed-timing.md`).

Command:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.test.ts
```

Raw result (primary timing sample, first focused run after install):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-current-selection-04/ui
Test Files  1 passed (1)
Tests  5 passed (5)
Start at 12:29:12
Duration  267ms (transform 74ms, setup 0ms, import 94ms, tests 14ms, environment 0ms)
[timing] elapsed_ms=1380
[timing] exit_code=0
```

Warm re-runs under the same command (comparable warm-deps conditions):

| Run | Wrapper wall | Vitest Duration | Result |
| --- | ---: | ---: | --- |
| 1 | 981ms | 264ms | 1 file / 5 tests passed |
| 2 | 1052ms | 362ms | 1 file / 5 tests passed |
| 3 | 1256ms | 292ms | 1 file / 5 tests passed |

Median warm wrapper wall-clock: **1052 ms**.

Focused wall-clock baseline used for later before/after comparison:
**1052 ms** process elapsed (median of three warm samples). The first-run
sample (`1380 ms`) is retained above for cold-ish context only.

Verbose confirmation of the five named cases was re-run under the same command
with `--reporter=verbose`; all five cases listed above passed.

## After migration — lane audit and latency report

Recorded for story `BUN-UNIT-features-current-selection-04-003` on the same
host class as the Vitest baseline (Windows 11 Home `10.0.26200`, i7-13700K,
24 logical processors; Bun `1.3.12`). Repository revision for these
measurements: `8ff7e969d`.

### Migrated / retained counts

| Lane ownership | Files | Named tests |
| --- | ---: | ---: |
| Migrated to Bun (exclusive) | 1 | 5 |
| Retained on Vitest | 0 | 0 |

Migrated file:

- `ui/src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.bun.unit.test.ts`

No retained exceptions. The leased Vitest path
`useCurrentSelection.request-helpers.test.ts` no longer exists in the tree.

### Lane exclusivity audit

Focused Bun invocation (exactly one file / five named cases):

```text
bun test src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.bun.unit.test.ts
```

Result:

```text
bun test v1.3.12 (700fc117)
src\features\current-selection\hooks\helpers\useCurrentSelection.request-helpers.bun.unit.test.ts:
(pass) useCurrentSelection.request-helpers > resolves projected requests from explicit maps or runtime snapshots
(pass) useCurrentSelection.request-helpers > filters provider-session attempts and selects the latest session per dispatch in request order
(pass) useCurrentSelection.request-helpers > sorts and selects workstation requests by started time and related work items
(pass) useCurrentSelection.request-helpers > derives selected-work dispatch attempts from requests and merges them with provider attempts
(pass) useCurrentSelection.request-helpers > converts runtime requests to projected requests and exposes request-owned work items
 5 pass
 0 fail
 19 expect() calls
Ran 5 tests across 1 file. [58.00ms]
```

Vitest `dashboard-unit` selection of the migrated path (must be zero):

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.bun.unit.test.ts
```

Result:

```text
No test files found, exiting with code 1
filter: .../useCurrentSelection.request-helpers.bun.unit.test.ts
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` path is also zero (`No test files
found`); the file is gone after rename. Conclusion: the leased suite executes
exactly once under Bun and zero times under Vitest.

### Focused after wall-clock (comparable warm-deps)

Command:

```text
bun test src/features/current-selection/hooks/helpers/useCurrentSelection.request-helpers.bun.unit.test.ts
```

Warm re-runs after one discarded warm-up (same host / Bun version as baseline):

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 1 | 101ms | 66ms | 1 file / 5 tests passed |
| 2 | 100ms | 68ms | 1 file / 5 tests passed |
| 3 | 96ms | 62ms | 1 file / 5 tests passed |

Median warm wrapper wall-clock after migration: **100 ms**.

### Before / after comparison

| Metric | Vitest baseline | Bun after |
| --- | ---: | ---: |
| Focused warm median wrapper wall | 1052 ms | 100 ms |
| Files | 1 | 1 |
| Named tests | 5 | 5 |
| Expect calls (Bun after) | — | 19 |

These are matched one-file focused observations only; they are not a
repository-wide speedup claim.

### Changed-line budget

Against `origin/main` at measurement time the cohort patch is **2 files /
83 insertions / 1 deletion** (baseline note + rename/import of the leased
test). Well within the ~1,000 changed-line budget; no split required.
