# Bun Unit Cohort Baseline: features-current-factory-definition-01

This record freezes the Vitest `dashboard-unit` baseline for the leased
`current-factory-definition` lib cohort before Bun migration. Later stories
compare Bun focused timings against this matched workload.

## Workload

Leased files (7):

| File | Source `it` / `it.each` blocks | Vitest expanded tests |
| --- | ---: | ---: |
| `ui/src/features/current-factory-definition/lib/runner-selection.test.ts` | 1 | 1 |
| `ui/src/features/current-factory-definition/lib/work-type-default-handling.test.ts` | 3 | 3 |
| `ui/src/features/current-factory-definition/lib/worker-workstation-taxonomy.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/doc-editable-values.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/workstation-guards.test.ts` | 7 | 7 |
| `ui/src/features/current-factory-definition/lib/go-duration.test.ts` | 4 | 4 |
| `ui/src/features/current-factory-definition/lib/editable-workstation-cron-validation.test.ts` | 8 | 15 |
| **Total** | **39** | **46** |

The PRD ownership inventory counts 39 source-level named cases. Vitest expands
the cron descriptor `it.each` block (`@annually` … `@yearly`) into seven
distinct tests, so the executable baseline is **7 files / 46 passing tests**.
Migration must preserve all 46 expanded cases (every named assertion below).

## Named cases that must remain

### `runner-selection.test.ts` (1)

- `resolveRunnerSelection > prefers workstation overrides, then factory, then legacy modelProvider, then default`

### `work-type-default-handling.test.ts` (3)

- `workTypeHasDefaultHandling > returns true when the work type includes DEFAULT handling`
- `workTypeHasDefaultHandling > returns false when the work type is not default`
- `workTypeHasDefaultHandling > returns false when the factory or work type is missing`

### `worker-workstation-taxonomy.test.ts` (8)

- `worker-workstation taxonomy helpers > prefers new taxonomy defaults for factory creation`
- `worker-workstation taxonomy helpers > accepts legacy worker aliases for model-provider and poller behavior`
- `worker-workstation taxonomy helpers > accepts legacy workstation aliases for inference and agent runs`
- `worker-workstation taxonomy helpers > keeps legacy worker types selectable while preferring new taxonomy options`
- `worker-workstation taxonomy helpers > keeps legacy workstation types selectable while preferring new conversion options`
- `worker-workstation taxonomy helpers > returns classifier-only conversion options for classifier workstations`
- `worker-workstation taxonomy helpers > returns preferred conversion options for unsupported workstation types`
- `worker-workstation taxonomy helpers > returns preferred worker options for unsupported worker types`

### `doc-editable-values.test.ts` (8)

- `doc-editable-values > resolves editable values from bundled DOC entries`
- `doc-editable-values > preserves the original extension when the rename omits one`
- `doc-editable-values > keeps an explicitly changed extension`
- `doc-editable-values > applies draft edits to the selected bundled doc only`
- `doc-editable-values > builds canonical target paths from file names`
- `doc-editable-values > handles non-doc paths and extension edge cases`
- `doc-editable-values > lists doc target paths and rejects invalid apply targets`
- `doc-editable-values nested docs > resolves editable values for nested bundled DOC entries`

### `workstation-guards.test.ts` (7)

- `workstation-guards formatting and defaults > formats VISIT_COUNT and MATCHES_FIELDS summaries`
- `workstation-guards formatting and defaults > creates default guards with factory workstation names`
- `workstation-guards formatting and defaults > normalizes input guards to at most one entry`
- `workstation-guards formatting and defaults > formats input guard summaries and resolves peer work types`
- `workstation-guards formatting and defaults > creates default per-input guards and clears slot guards`
- `workstation-guards draft equality and rename > compares guard and full draft equality`
- `workstation-guards draft equality and rename > rewrites VISIT_COUNT workstation references on workstation and input guards`

### `go-duration.test.ts` (4)

- `parseGoDurationNanoseconds > parses common Go duration strings`
- `parseGoDurationNanoseconds > rejects invalid duration strings`
- `parseGoDurationNanoseconds > parses fractional and microsecond duration units`
- `parseGoDurationNanoseconds > rejects non-finite parsed amounts`

### `editable-workstation-cron-validation.test.ts` (8 source / 15 expanded)

- `validateEditableWorkstationCronDraft > requires a cron draft object`
- `validateEditableWorkstationCronDraft > requires a non-empty schedule`
- `validateEditableWorkstationCronDraft > rejects invalid five-field cron schedules with backend-aligned errors`
- `validateEditableWorkstationCronDraft > rejects invalid jitter and expiry window durations`
- `validateEditableWorkstationCronDraft > accepts valid cron schedules, jitter, and expiry windows`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @annually`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @daily`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @hourly`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @midnight`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @monthly`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @weekly`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts cron descriptor macro @yearly`
- `validateEditableWorkstationCronDraft descriptor schedules > uses a generic parse failure message when cron-parser throws a non-Error`
- `validateEditableWorkstationCronDraft descriptor schedules > accepts @every schedules with positive Go durations`
- `validateEditableWorkstationCronDraft descriptor schedules > rejects invalid @every durations and unrecognized descriptors`

## Runtime and host

- Recorded: 2026-08-01T18:43:03Z (Vitest start) / wall-clock wrapper measured immediately after.
- Repository revision: `375508098` (branch after merging `origin/main`, including merged Bun unit foundation).
- Bun: `1.3.12`.
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-factory-definition-01`.

## Matched Vitest baseline command and raw results

Command:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 \
  src/features/current-factory-definition/lib/doc-editable-values.test.ts \
  src/features/current-factory-definition/lib/editable-workstation-cron-validation.test.ts \
  src/features/current-factory-definition/lib/go-duration.test.ts \
  src/features/current-factory-definition/lib/runner-selection.test.ts \
  src/features/current-factory-definition/lib/work-type-default-handling.test.ts \
  src/features/current-factory-definition/lib/worker-workstation-taxonomy.test.ts \
  src/features/current-factory-definition/lib/workstation-guards.test.ts
```

Raw result:

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-features-current-factory-definition-01/ui
Test Files  7 passed (7)
Tests  46 passed (46)
Start at  11:43:03
Duration  627ms (transform 380ms, setup 0ms, import 524ms, tests 51ms, environment 1ms)
[timing] elapsed_ms=1861
[timing] exit_code=0
```

## Baseline summary

- Files: 7
- Passing Vitest tests (expanded): 46
- Source-level named cases (PRD inventory): 39
- Vitest reported duration: 627ms
- Wrapper wall-clock elapsed: 1861ms
- Exit code: 0
