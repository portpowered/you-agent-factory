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

## After-state: lane audit and focused Bun wall-clock

Recorded for story `BUN-UNIT-features-current-selection-08-006` on the same
host class as the Vitest baseline (Windows 11 Home `10.0.26200`, i7-13700K, 24
logical processors; Bun `1.3.12`). Repository revision for these measurements:
`6f24e3e23` (after the four leased migrations; before this report commit).

### Migrated / retained counts

| Lane ownership | Files | Named tests |
| --- | ---: | ---: |
| Migrated to Bun (exclusive) | 4 | 34 |
| Retained on Vitest | 0 | 0 |

Migrated files (exactly once under Bun):

| Migrated path | Named cases |
| --- | ---: |
| `ui/src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.bun.unit.test.ts` | 18 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.bun.unit.test.ts` | 6 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.bun.unit.test.ts` | 6 |
| `ui/src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.bun.unit.test.ts` | 4 |

Totals reconcile to the leased baseline: `4` files / `34` named tests. No
retained Vitest exceptions. The leased `.test.ts` paths no longer exist in the
tree after rename.

### Lane exclusivity audit

Focused Bun invocation (exactly four files / thirty-four named cases):

```text
bun test `
  src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.bun.unit.test.ts
```

Result:

```text
bun test v1.3.12 (700fc117)
src\features\current-selection\workstation-selection\editing\editable-workstation-configuration-mutators.bun.unit.test.ts:
(pass) buildEditableWorkstationConfigurationMutators cron fields > updates cron draft fields through cron mutators
(pass) buildEditableWorkstationConfigurationMutators session sync > marks changes saved from the current draft
(pass) buildEditableWorkstationConfigurationMutators session sync > resets the draft to the latest factory definition
(pass) buildEditableWorkstationConfigurationMutators other draft fields > updates prompt, runner, worker, and behavior on the session draft
(pass) buildEditableWorkstationConfigurationMutators other draft fields > updates the workstation name on the session draft
(pass) buildEditableWorkstationConfigurationMutators other draft fields > no-ops mutators when session state is null
src\features\current-selection\workstation-selection\editing\editable-workstation-cron-draft-mutators.bun.unit.test.ts:
(pass) resolveDraftForBehaviorChange > initializes cron when switching to CRON without an existing draft cron
(pass) resolveDraftForBehaviorChange > uses an empty cron draft when switching to CRON and the factory has no cron
(pass) resolveDraftForBehaviorChange > preserves an in-progress cron draft when switching to CRON
(pass) resolveDraftForBehaviorChange > clears cron when switching away from CRON
(pass) updateEditableWorkstationCronDraft > returns the draft unchanged for non-CRON behavior
(pass) updateEditableWorkstationCronDraft > creates and patches cron fields for CRON behavior
src\features\current-selection\workstation-selection\editing\editable-workstation-overwrite-fields.bun.unit.test.ts:
(pass) editable workstation overwrite fields > flags model-invoke workstation type, operation, and binding drift
(pass) editable workstation overwrite fields > ignores model-invoke fields that already match the latest definition
(pass) editable workstation overwrite fields > flags behavior and runner drift for prompt-oriented workstations
(pass) editable workstation overwrite fields > formats model-invoke overwrite labels for the warning banner
src\features\current-selection\workstation-selection\components\fields\workstation-summary-field-values.bun.unit.test.ts:
(pass) resolveWorkstationSummaryRequiresWorkerAssignment > returns false for ready LOGICAL_MOVE editable configuration
(pass) resolveWorkstationSummaryRequiresWorkerAssignment > returns true for ready model workstation editable configuration
(pass) resolveWorkstationSummaryRequiresWorkerAssignment > returns false when topology kind is LOGICAL_MOVE before editable configuration is ready
(pass) resolveWorkstationSummaryTypeValue > localizes the authoritative workstation type when editable configuration is ready
(pass) resolveWorkstationSummaryTypeValue > returns loading and unavailable copy for non-ready editable configuration states
(pass) resolveWorkstationSummaryTypeValue > localizes LOGICAL_MOVE workstation type when editable configuration is ready
(pass) resolveWorkstationSummaryKindValue > localizes draft scheduling kind when editable configuration is ready
(pass) resolveWorkstationSummaryKindValue > ignores stale topology kind when editable configuration is ready
(pass) resolveWorkstationSummaryKindValue > returns loading and unavailable copy for non-ready editable configuration states
(pass) resolveWorkstationSummaryKindValue > localizes uppercase topology kinds when only topology is available
(pass) resolveWorkstationSummaryKindValue > normalizes legacy lowercase topology kinds when only topology is available
(pass) resolveWorkstationSummaryKindValue > localizes unknown topology kinds with explicit fallback text
(pass) resolveWorkstationSummaryKindValue > returns null for LOGICAL_MOVE workstations
(pass) resolveWorkstationSummaryPresentation > derives poller summary presentation from editable workstation behavior
(pass) resolveWorkstationSummaryRunnerValue > returns null for LOGICAL_MOVE workstations
(pass) resolveWorkstationSummaryRunnerValue > shows localized runner display metadata with RunnerSelectionSource labels
(pass) resolveWorkstationSummaryRunnerValue > uses legacy_provider when inheriting from worker modelProvider
(pass) resolveWorkstationSummaryRunnerValue > returns unavailable copy for invalid runner ids
 34 pass
 0 fail
 46 expect() calls
Ran 34 tests across 4 files. [135.00ms]
```

Vitest `dashboard-unit` selection of the migrated paths (must be zero):

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.bun.unit.test.ts `
  src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.bun.unit.test.ts
```

Result:

```text
No test files found, exiting with code 1
filter: ...workstation-summary-field-values.bun.unit.test.ts, ...editable-workstation-configuration-mutators.bun.unit.test.ts, ...editable-workstation-cron-draft-mutators.bun.unit.test.ts, ...editable-workstation-overwrite-fields.bun.unit.test.ts
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename. Conclusion: the leased suite
executes exactly once under Bun and zero times under Vitest. Aggregate focused
Bun cohort run and affected Vitest selection checks pass; `check-ui-test-lanes.mjs`
and frontend typecheck also pass on this head.

### Focused after wall-clock (comparable warm-deps)

Command (same four migrated paths as above). Wrapper wall-clock uses
`[System.Diagnostics.Stopwatch]` around the command, matching the baseline.

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 1 (first focused) | 189ms | 142ms | 4 files / 34 tests passed |
| 2 | 174ms | 134ms | 4 files / 34 tests passed |
| 3 | 172ms | 135ms | 4 files / 34 tests passed |
| 4 | 167ms | 130ms | 4 files / 34 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`172ms`, Bun reported `134ms`. All runs reported exit code `0`.

### Before / after comparison

| Metric | Vitest baseline | Bun after |
| --- | ---: | ---: |
| Focused warm median wrapper wall | 1358 ms | 172 ms |
| Runner-reported warm median | 698 ms (Vitest) | 134 ms (Bun) |
| Files | 4 | 4 |
| Named tests | 34 | 34 |
| Expect calls (Bun after) | — | 46 |
| Retained on Vitest | — | 0 files / 0 tests |

These are matched four-file focused observations only; they are not a
repository-wide speedup claim.

### Changed-line budget

Against the merge-base with `origin/main`, the rename-aware cohort patch is
**5 files / ~127 insertions / ~6 deletions** (baseline evidence + after-state
report + four leased rename/import migrations). Well within the ~1,000
changed-line budget; no unsafe-coupling split required. No production-code
edits.
