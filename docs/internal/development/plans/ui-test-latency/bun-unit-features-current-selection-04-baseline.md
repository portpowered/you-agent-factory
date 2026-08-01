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

## After-migration placeholder

Bun after timings, migrated/retained counts, and lane-exclusivity proof belong
in story `BUN-UNIT-features-current-selection-04-003` once migration completes.
They must use the same host class, worker/retry settings for any remaining
Vitest retained files, and a focused Bun invocation of the migrated
`.bun.unit.test.ts` file under comparable warm-deps conditions.
