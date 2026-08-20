# Bun Unit Cohort Timing: features-current-factory-definition-02

This record captures the lane exclusivity audit after migrating the leased
`current-factory-definition` workstation-helper cohort and the matched focused
before/after wall-clock comparison. The Vitest baseline inventory lives in
`bun-unit-features-current-factory-definition-02-baseline.md`. The measurements
are local diagnostic evidence for the migration contract; they must not be
converted into a repository-wide speedup claim.

## Workload

- Cohort work item: `BUN-UNIT-features-current-factory-definition-02`
- Leased files: 5
- Source-level named cases (PRD inventory): 41
- Expanded executable tests: 41 (no `it.each` expansions)
- Retained Vitest exceptions: 0

Migrated Bun-owned paths:

| Migrated path | Source cases | Expanded tests |
| --- | ---: | ---: |
| `ui/src/features/current-factory-definition/lib/workstation-progress-outcome-routes.bun.unit.test.ts` | 20 | 20 |
| `ui/src/features/current-factory-definition/lib/workstation-worker-assignment.bun.unit.test.ts` | 4 | 4 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-editable-resolution.bun.unit.test.ts` | 5 | 5 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-model-invoke.bun.unit.test.ts` | 8 | 8 |
| `ui/src/features/current-factory-definition/lib/workstation/workstation-type.bun.unit.test.ts` | 4 | 4 |
| **Total** | **41** | **41** |

Totals reconcile to the leased baseline: `5` files / `41` source-level named
cases / `41` expanded executable tests. No retained Vitest exceptions. The
leased `.test.ts` paths no longer exist after rename. Named-case inventory is
unchanged from the baseline document.

## Runtime and host

- Recorded: 2026-08-01T23:09:43Z (story `007` lane audit + focused after).
- Repository revision for the Vitest baseline: `ccce5c32e`.
- Repository revision for lane audit + focused after: `3985799a7` (plus this
  timing document).
- Bun: `1.3.12`.
- Vitest: `4.1.3`.
- Node: `v24.5.0`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen Intel
  Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-features-current-factory-definition-02`.

## Matched commands and raw results

### Vitest baseline (before)

Command and raw result are frozen in
`bun-unit-features-current-factory-definition-02-baseline.md`:

| Field | Value |
| --- | --- |
| Files | 5 |
| Passing tests (expanded) | 41 |
| Vitest reported duration | 715ms |
| Wrapper wall-clock `elapsed_ms` | 2417 |
| Exit code | 0 |

### Bun after (full cohort)

Focused Bun command (exactly the five migrated paths, from `ui/`):

```text
bun test \
  src/features/current-factory-definition/lib/workstation-progress-outcome-routes.bun.unit.test.ts \
  src/features/current-factory-definition/lib/workstation-worker-assignment.bun.unit.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-editable-resolution.bun.unit.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-model-invoke.bun.unit.test.ts \
  src/features/current-factory-definition/lib/workstation/workstation-type.bun.unit.test.ts
```

Wrapper wall-clock uses `[System.Diagnostics.Stopwatch]` around that command
with stdout/stderr redirected to a temp file (matching the baseline measurement
style of capturing process wall time without PowerShell stream-tee overhead).

Comparable warm-deps runs (same host class as the Vitest baseline):

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 0 (first focused, console) | 268ms | 184ms | 5 files / 41 tests passed |
| 1 (warm, file redirect) | 107ms | 71ms | 5 files / 41 tests passed |
| 2 (warm, file redirect) | 114ms | 77ms | 5 files / 41 tests passed |
| 3 (warm, file redirect) | 116ms | 79ms | 5 files / 41 tests passed |

Median of the three warm comparable samples (runs 1–3): wrapper wall `114ms`,
Bun reported `77ms`. All runs reported exit code `0`.

Representative raw reporter output (run 3):

```text
bun test v1.3.12 (700fc117)
...
 41 pass
 0 fail
 76 expect() calls
Ran 41 tests across 5 files. [79.00ms]
[timing] elapsed_ms=116
[timing] exit_code=0
```

## Lane exclusivity audit

| Leased logical file | Lane ownership | Bun execution | Vitest `dashboard-unit` |
| --- | --- | --- | --- |
| `workstation-progress-outcome-routes` | Bun unit | included in 41/41 cohort pass | No test files found (`.bun.unit.test.ts` excluded; old `.test.ts` absent) |
| `workstation-worker-assignment` | Bun unit | included in 41/41 cohort pass | No test files found |
| `workstation-editable-resolution` | Bun unit | included in 41/41 cohort pass | No test files found |
| `workstation-model-invoke` | Bun unit | included in 41/41 cohort pass | No test files found |
| `workstation-type` | Bun unit | included in 41/41 cohort pass | No test files found |

Vitest `dashboard-unit` selection of the migrated `.bun.unit.test.ts` paths:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 \
  <five migrated .bun.unit.test.ts paths>
```

Result:

```text
No test files found, exiting with code 1
filter: ...workstation-progress-outcome-routes.bun.unit.test.ts, ... (five paths)
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename.

Audit conclusions:

- All five leased files execute exactly once under Bun and are not selected by
  Vitest `dashboard-unit`.
- Migrated plus retained totals reconcile to `5` files / `41` source-level cases /
  `41` expanded tests with zero Vitest retention.
- No double-lane ownership for any leased case set.
- `bun run check:test-lanes` passed on this head; frontend typecheck and lint
  checks for the touched surface passed.

## Focused before/after wall-clock comparison

Comparable host and runner versions. Before = single Vitest command over the
five leased `.test.ts` files. After = single Bun command over the five renamed
`.bun.unit.test.ts` files (same 41 expanded cases).

| Field | Vitest baseline | Bun after (warm median) |
| --- | --- | --- |
| Migrated files | 0 | 5 |
| Retained Vitest files | 5 | 0 |
| Passing tests (expanded) | 41 | 41 |
| Source-level named cases | 41 | 41 |
| Wall-clock `elapsed_ms` | 2417 | 114 |
| Runner duration summary | Vitest 715ms | Bun 77ms |
| Patch size vs `ccce5c32e` | n/a | 6 files before this timing doc (`+129` / `−5`); leased renames + baseline note only; well under ~1,000 changed lines |

Observation for this cohort only: focused wall-clock improved from `2417ms` to
about `114ms` under comparable local warm-deps conditions while preserving all
41 expanded cases and assertion strength. No behavior tests were converted into
source-shape/inventory assertions; every leased file migrated by rename +
`bun:test` import swap with no Vitest retention exception.
