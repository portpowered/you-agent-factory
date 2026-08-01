# Bun Unit Cohort Baseline: features-current-selection-08

This record freezes the Vitest `dashboard-unit` before-state for the leased
four-file workstation-selection cohort. It is measurement evidence for the
migration contract; it does not invent product behavior.

## Workload

Exact leased paths (4 files, 34 named cases):

| File | Named cases |
| --- | ---: |
| `ui/src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.test.ts` | 18 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.test.ts` | 6 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.test.ts` | 6 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.test.ts` | 4 |

Totals: `18 + 6 + 6 + 4 = 34`.

### Named-case inventory

`workstation-summary-field-values.test.ts` (18):

1. returns false for ready LOGICAL_MOVE editable configuration
2. returns true for ready model workstation editable configuration
3. returns false when topology kind is LOGICAL_MOVE before editable configuration is ready
4. localizes the authoritative workstation type when editable configuration is ready
5. returns loading and unavailable copy for non-ready editable configuration states
6. localizes LOGICAL_MOVE workstation type when editable configuration is ready
7. localizes draft scheduling kind when editable configuration is ready
8. ignores stale topology kind when editable configuration is ready
9. returns loading and unavailable copy for non-ready editable configuration states
10. localizes uppercase topology kinds when only topology is available
11. normalizes legacy lowercase topology kinds when only topology is available
12. localizes unknown topology kinds with explicit fallback text
13. returns null for LOGICAL_MOVE workstations
14. derives poller summary presentation from editable workstation behavior
15. returns null for LOGICAL_MOVE workstations
16. shows localized runner display metadata with RunnerSelectionSource labels
17. uses legacy_provider when inheriting from worker modelProvider
18. returns unavailable copy for invalid runner ids

`editable-workstation-configuration-mutators.test.ts` (6):

1. updates cron draft fields through cron mutators
2. marks changes saved from the current draft
3. resets the draft to the latest factory definition
4. updates prompt, runner, worker, and behavior on the session draft
5. updates the workstation name on the session draft
6. no-ops mutators when session state is null

`editable-workstation-cron-draft-mutators.test.ts` (6):

1. initializes cron when switching to CRON without an existing draft cron
2. uses an empty cron draft when switching to CRON and the factory has no cron
3. preserves an in-progress cron draft when switching to CRON
4. clears cron when switching away from CRON
5. returns the draft unchanged for non-CRON behavior
6. creates and patches cron fields for CRON behavior

`editable-workstation-overwrite-fields.test.ts` (4):

1. flags model-invoke workstation type, operation, and binding drift
2. ignores model-invoke fields that already match the latest definition
3. flags behavior and runner drift for prompt-oriented workstations
4. formats model-invoke overwrite labels for the warning banner

## Runtime and host

- Recorded: 2026-08-01 UTC
- Repository revision: `ef0bfc581` (`BUN-UNIT-features-current-selection-08` after merging current `main`, including `BUN-UNIT-00-lane-foundation`)
- Bun: `1.3.12`
- Vitest: `4.1.3`
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-selection-08`

## Matched Vitest command

From `ui/`:

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.test.ts
```

Wrapper wall-clock is measured around that command with
`[System.Diagnostics.Stopwatch]`.

## Comparable baseline runs

| Run | Wrapper wall | Vitest wall | Transform | Import | Tests | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 (first focused) | 1600ms | 643ms | 778ms | 1.15s | 19ms | 4 files / 34 tests passed |
| 2 | 1687ms | 853ms | 782ms | 1.14s | 19ms | 4 files / 34 tests passed |
| 3 | 1358ms | 698ms | 815ms | 1.19s | 25ms | 4 files / 34 tests passed |
| 4 | 1312ms | 594ms | 730ms | 1.05s | 16ms | 4 files / 34 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`1358ms`, Vitest wall `698ms`. All runs reported exit code `0`.

Representative raw reporter output (run 3):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-current-selection-08/ui
Test Files  4 passed (4)
Tests  34 passed (34)
Start at  11:53:15
Duration  698ms (transform 815ms, setup 0ms, import 1.19s, tests 25ms, environment 0ms)
[timing] elapsed_ms=1358
[timing] exit_code=0
```

## After-state placeholder

Focused Bun after-state wall-clock for the same four files will be recorded in
story `BUN-UNIT-features-current-selection-08-006` under conditions comparable
to this baseline.
