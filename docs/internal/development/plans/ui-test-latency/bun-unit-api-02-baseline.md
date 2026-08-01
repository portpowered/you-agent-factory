# Bun Unit Cohort Baseline: api-02

This record freezes the Vitest `dashboard-unit` before-state for the leased
three-file API session-routing, session-scope, and prompt-template cohort. It is
measurement evidence for the migration contract; it does not invent product
behavior.

## Workload

Exact leased paths (3 files, 20 named cases):

| File | Named cases |
| --- | ---: |
| `ui/src/api/session-factory/prompt-template/api.test.ts` | 7 |
| `ui/src/api/session-routing.test.ts` | 6 |
| `ui/src/api/session-scope.test.ts` | 7 |

Totals: `7 + 6 + 7 = 20`.

### Named-case inventory

`prompt-template/api.test.ts` (7):

1. loads the current-factory workstation prompt-template contract
2. posts prompt validation against the current-factory workstation contract
3. uses the selected session for non-default prompt-template requests
4. surfaces typed current-factory prompt-template API errors
5. surfaces NETWORK_ERROR when prompt-template validation fetch rejects
6. surfaces a network error when no fetch implementation is available
7. surfaces invalid JSON and fallback API codes from prompt-template responses

`session-routing.test.ts` (6):

1. treats null, undefined, and empty sessions as the default session
2. always returns an explicit default-session scoped path
3. preserves non-default session identifiers in the scoped path
4. exposes the canonical current-factory session route directly
5. builds canonical current-factory workstation subroutes under the session path
6. exposes explicit work and events session routes

`session-scope.test.ts` (7):

1. maps null, empty, and ~default selection to the default session scope
2. keeps resolved default semantics while routing every resource by UUID
3. preserves non-default session identifiers and URL-encodes path segments
4. reflects pause state for the active non-default session
5. does not mark the default session paused when only other sessions are paused
6. does not mark the default session paused when the default id is listed as paused
7. does not mark a non-default session paused when it is absent from the paused list

## Runtime and host

- Recorded: 2026-08-01 UTC
- Repository revision: `42f7272bf` (`BUN-UNIT-api-02` after merging current `main`, including `BUN-UNIT-00-lane-foundation`)
- Bun: `1.3.12`
- Vitest: `4.1.3`
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-api-02`

## Matched Vitest command

From `ui/`:

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/api/session-factory/prompt-template/api.test.ts `
  src/api/session-routing.test.ts `
  src/api/session-scope.test.ts
```

Wrapper wall-clock is measured around that command with
`[System.Diagnostics.Stopwatch]`.

## Comparable baseline runs

| Run | Wrapper wall | Vitest wall | Transform | Import | Tests | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 (first focused) | 1249ms | 259ms | 142ms | 188ms | 30ms | 3 files / 20 tests passed |
| 2 | 927ms | 278ms | 150ms | 197ms | 30ms | 3 files / 20 tests passed |
| 3 | 851ms | 256ms | 148ms | 197ms | 32ms | 3 files / 20 tests passed |
| 4 | 812ms | 240ms | 140ms | 185ms | 29ms | 3 files / 20 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`851ms`, Vitest wall `256ms`. All runs reported exit code `0`.

Representative raw reporter output (run 3):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-api-02/ui
Test Files  3 passed (3)
Tests  20 passed (20)
Start at  12:19:09
Duration  256ms (transform 148ms, setup 0ms, import 197ms, tests 32ms, environment 0ms)
[timing] elapsed_ms=851
[timing] exit_code=0
```

## After migration — lane audit and latency report

Recorded for story `BUN-UNIT-api-02-005` on the same host class as the
Vitest baseline (Windows 11 Home `10.0.26200`, i7-13700K, 24 logical
processors; Bun `1.3.12`). Repository revision for these measurements:
`6508a79c2` (after the three leased migrations; before this report commit).

### Migrated / retained counts

| Lane ownership | Files | Named tests |
| --- | ---: | ---: |
| Migrated to Bun (exclusive) | 3 | 20 |
| Retained on Vitest | 0 | 0 |

Migrated files (exactly once under Bun):

| Migrated path | Named cases |
| --- | ---: |
| `ui/src/api/session-factory/prompt-template/api.bun.unit.test.ts` | 7 |
| `ui/src/api/session-routing.bun.unit.test.ts` | 6 |
| `ui/src/api/session-scope.bun.unit.test.ts` | 7 |

Totals reconcile to the leased baseline: `3` files / `20` named tests. No
retained Vitest exceptions. The leased `.test.ts` paths no longer exist in the
tree after rename.

### Lane exclusivity audit

Focused Bun invocation (exactly three files / twenty named cases):

```text
bun test `
  src/api/session-factory/prompt-template/api.bun.unit.test.ts `
  src/api/session-routing.bun.unit.test.ts `
  src/api/session-scope.bun.unit.test.ts
```

Result:

```text
bun test v1.3.12 (700fc117)
src\api\session-routing.bun.unit.test.ts:
(pass) factorySessionScopedPath > treats null, undefined, and empty sessions as the default session
(pass) factorySessionScopedPath > always returns an explicit default-session scoped path
(pass) factorySessionScopedPath > preserves non-default session identifiers in the scoped path
(pass) factorySessionScopedPath > exposes the canonical current-factory session route directly
(pass) factorySessionScopedPath > builds canonical current-factory workstation subroutes under the session path
(pass) factorySessionScopedPath > exposes explicit work and events session routes
src\api\session-scope.bun.unit.test.ts:
(pass) buildSessionScope > maps null, empty, and ~default selection to the default session scope
(pass) buildSessionScope > keeps resolved default semantics while routing every resource by UUID
(pass) buildSessionScope > preserves non-default session identifiers and URL-encodes path segments
(pass) buildSessionScope > reflects pause state for the active non-default session
(pass) buildSessionScope > does not mark the default session paused when only other sessions are paused
(pass) buildSessionScope > does not mark the default session paused when the default id is listed as paused
(pass) buildSessionScope > does not mark a non-default session paused when it is absent from the paused list
src\api\session-factory\prompt-template\api.bun.unit.test.ts:
(pass) session-factory prompt-template API > loads the current-factory workstation prompt-template contract
(pass) session-factory prompt-template API > posts prompt validation against the current-factory workstation contract
(pass) session-factory prompt-template API > uses the selected session for non-default prompt-template requests
(pass) session-factory prompt-template API > surfaces typed current-factory prompt-template API errors
(pass) session-factory prompt-template API > surfaces NETWORK_ERROR when prompt-template validation fetch rejects
(pass) session-factory prompt-template API > surfaces a network error when no fetch implementation is available
(pass) session-factory prompt-template API > surfaces invalid JSON and fallback API codes from prompt-template responses
 20 pass
 0 fail
 41 expect() calls
Ran 20 tests across 3 files. [132.00ms]
```

Vitest `dashboard-unit` selection of the migrated paths (must be zero):

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/api/session-factory/prompt-template/api.bun.unit.test.ts `
  src/api/session-routing.bun.unit.test.ts `
  src/api/session-scope.bun.unit.test.ts
```

Result:

```text
No test files found, exiting with code 1
filter: ...api.bun.unit.test.ts, ...session-routing.bun.unit.test.ts, ...session-scope.bun.unit.test.ts
projects: dashboard-unit
exclude: ... src/**/*.bun.unit.test.ts ...
```

Vitest selection of the retired `.test.ts` paths is also zero (`No test files
found`); those files are gone after rename. Conclusion: the leased suite
executes exactly once under Bun and zero times under Vitest. Aggregate focused
Bun cohort run and affected Vitest selection checks pass; `check:test-lanes`
and frontend typecheck also pass on this head.

### Focused after wall-clock (comparable warm-deps)

Command (same three migrated paths as above). Wrapper wall-clock uses
`[System.Diagnostics.Stopwatch]` around the command, matching the baseline.

| Run | Wrapper wall | Bun reported | Result |
| --- | ---: | ---: | --- |
| 1 (first focused) | 197ms | 124ms | 3 files / 20 tests passed |
| 2 | 194ms | 134ms | 3 files / 20 tests passed |
| 3 | 233ms | 150ms | 3 files / 20 tests passed |
| 4 | 213ms | 120ms | 3 files / 20 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`213ms`, Bun reported `134ms`. All runs reported exit code `0`.

### Before / after comparison

| Metric | Vitest baseline | Bun after |
| --- | ---: | ---: |
| Focused warm median wrapper wall | 851 ms | 213 ms |
| Runner-reported warm median | 256 ms (Vitest) | 134 ms (Bun) |
| Files | 3 | 3 |
| Named tests | 20 | 20 |
| Expect calls (Bun after) | — | 41 |
| Retained on Vitest | — | 0 files / 0 tests |

These are matched three-file focused observations only; they are not a
repository-wide speedup claim.

### Changed-line budget

Against the merge-base with `origin/main`, the rename-aware cohort patch is
**4 files / ~235 insertions / ~6 deletions** (baseline evidence + after-state
report + three leased rename/import migrations). Well within the ~1,000
changed-line budget; no unsafe-coupling split required. No production-code
edits.
