# Bun Unit Cohort Timing: features-current-factory-definition-01

This record captures the lane exclusivity audit after migrating the leased
`current-factory-definition` lib cohort and the matched focused before/after
wall-clock comparison. The Vitest baseline inventory lives in
`bun-unit-features-current-factory-definition-01-baseline.md`. The measurements
are local diagnostic evidence for the migration contract; they must not be
converted into a repository-wide speedup claim.

## Workload

- Cohort work item: `BUN-UNIT-features-current-factory-definition-01`
- Leased files: 7
- Source-level named cases (PRD inventory): 39
- Expanded executable tests (cron `it.each`): 46
- Retained Vitest exceptions: 0

Migrated Bun-owned paths:

| Migrated path | Source cases | Expanded tests |
| --- | ---: | ---: |
| `ui/src/features/current-factory-definition/lib/runner-selection.bun.unit.test.ts` | 1 | 1 |
| `ui/src/features/current-factory-definition/lib/work-type-default-handling.bun.unit.test.ts` | 3 | 3 |
| `ui/src/features/current-factory-definition/lib/worker-workstation-taxonomy.bun.unit.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/doc-editable-values.bun.unit.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/workstation-guards.bun.unit.test.ts` | 7 | 7 |
| `ui/src/features/current-factory-definition/lib/go-duration.bun.unit.test.ts` | 4 | 4 |
| `ui/src/features/current-factory-definition/lib/editable-workstation-cron-validation.bun.unit.test.ts` | 8 | 15 |
| **Total** | **39** | **46** |

Totals reconcile to the leased baseline: `7` files / `39` source-level named
cases / `46` expanded executable tests. No retained Vitest exceptions. The
leased `.test.ts` paths no longer exist after rename. Named-case inventory is
unchanged from the baseline document.

## Runtime and host

- Recorded: 2026-08-01 UTC (story `005` lane audit + focused after).
- Repository revision for the Vitest baseline: `375508098`.
- Repository revision for lane audit + focused after: `03e254bc2` (plus this
  timing document).
- Bun: `1.3.12`.
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen Intel
  Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-factory-definition-01`.

## Matched commands and raw results

### Vitest baseline (before)

Command and raw result are frozen in
`bun-unit-features-current-factory-definition-01-baseline.md`:

| Field | Value |
| --- | --- |
| Files | 7 |
| Passing tests (expanded) | 46 |
| Vitest reported duration | 627ms |
| Wrapper wall-clock `elapsed_ms` | 1861 |
| Exit code | 0 |

### Bun after (full cohort)

Focused Bun command (exactly the seven migrated paths):

```text
bun test \
  src/features/current-factory-definition/lib/runner-selection.bun.unit.test.ts \
  src/features/current-factory-definition/lib/work-type-default-handling.bun.unit.test.ts \
  src/features/current-factory-definition/lib/worker-workstation-taxonomy.bun.unit.test.ts \
  src/features/current-factory-definition/lib/doc-editable-values.bun.unit.test.ts \
  src/features/current-factory-definition/lib/workstation-guards.bun.unit.test.ts \
  src/features/current-factory-definition/lib/go-duration.bun.unit.test.ts \
  src/features/current-factory-definition/lib/editable-workstation-cron-validation.bun.unit.test.ts
```

Wrapper wall-clock uses `[System.Diagnostics.Stopwatch]` around that command,
matching the baseline measurement style.

Comparable warm-deps runs (same host class as the Vitest baseline):

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 0 (first focused) | 269ms | 147ms | 7 files / 46 tests passed |
| 1 | 195ms | 155ms | 7 files / 46 tests passed |
| 2 | 155ms | 116ms | 7 files / 46 tests passed |
| 3 | 161ms | 121ms | 7 files / 46 tests passed |

Median of the three warm comparable samples (runs 1–3): wrapper wall `161ms`,
Bun reported `121ms`. All runs reported exit code `0`.

Representative raw reporter output (run 2):

```text
bun test v1.3.12 (700fc117)
...
 46 pass
 0 fail
 101 expect() calls
Ran 46 tests across 7 files. [116.00ms]
[timing] elapsed_ms=155
[timing] exit_code=0
```

## Lane exclusivity audit

| Leased logical file | Lane ownership | Bun execution | Vitest `dashboard-unit` |
| --- | --- | --- | --- |
| `runner-selection` | Bun unit | included in 46/46 cohort pass | No test files found (`.bun.unit.test.ts` excluded; old `.test.ts` absent) |
| `work-type-default-handling` | Bun unit | included in 46/46 cohort pass | No test files found |
| `worker-workstation-taxonomy` | Bun unit | included in 46/46 cohort pass | No test files found |
| `doc-editable-values` | Bun unit | included in 46/46 cohort pass | No test files found |
| `workstation-guards` | Bun unit | included in 46/46 cohort pass | No test files found |
| `go-duration` | Bun unit | included in 46/46 cohort pass | No test files found |
| `editable-workstation-cron-validation` | Bun unit | included in 46/46 cohort pass (15 expanded) | No test files found |

Vitest `dashboard-unit` selection of the migrated `.bun.unit.test.ts` paths:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 \
  <seven migrated .bun.unit.test.ts paths>
```

Result:

```text
No test files found, exiting with code 1
filter: ...runner-selection.bun.unit.test.ts, ... (seven paths)
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename.

Audit conclusions:

- All seven leased files execute exactly once under Bun and are not selected by
  Vitest `dashboard-unit`.
- Migrated plus retained totals reconcile to `7` files / `39` source-level cases /
  `46` expanded tests with zero Vitest retention.
- No double-lane ownership for any leased case set.
- `bun run check:test-lanes` passed on this head; frontend typecheck passed.

## Focused before/after wall-clock comparison

Comparable host and runner versions. Before = single Vitest command over the
seven leased `.test.ts` files. After = single Bun command over the seven
renamed `.bun.unit.test.ts` files (same 46 expanded cases).

| Field | Vitest baseline | Bun after (warm median) |
| --- | --- | --- |
| Migrated files | 0 | 7 |
| Retained Vitest files | 7 | 0 |
| Passing tests (expanded) | 46 | 46 |
| Source-level named cases | 39 | 39 |
| Wall-clock `elapsed_ms` | 1861 | 161 |
| Runner duration summary | Vitest 627ms | Bun 121ms |
| Patch size vs `375508098` | n/a | 8 files before this timing doc (`+150` / `−11`); leased renames + baseline note only; well under ~1,000 changed lines |

Observation for this cohort only: focused wall-clock improved from `1861ms` to
about `161ms` under comparable local warm-deps conditions while preserving all
46 expanded cases and assertion strength. No behavior tests were converted into
source-shape/inventory assertions; spy cleanup uses Bun-native `spyOn` +
`mock.restore()` only where Vitest `vi.*` was previously required.
