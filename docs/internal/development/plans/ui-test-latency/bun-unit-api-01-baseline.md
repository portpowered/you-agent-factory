# Bun Unit Cohort Baseline: api-01

This record freezes the Vitest `dashboard-unit` before-state for the leased
five-file API adapter cohort. It is measurement evidence for the migration
contract; it does not invent product behavior.

## Workload

Exact leased paths (5 files, 17 named cases):

| File | Named cases |
| --- | ---: |
| `ui/src/api/factory-definition/preserve-bundled-files.test.ts` | 3 |
| `ui/src/api/factory-definition/preserve-layout.test.ts` | 2 |
| `ui/src/api/factory-sessions/lifecycle-controls.test.ts` | 5 |
| `ui/src/api/generated/openapi.session-factory.test.ts` | 4 |
| `ui/src/api/session-factory/import-activation.discover.test.ts` | 3 |

Totals: `3 + 2 + 5 + 4 + 3 = 17`.

### Named-case inventory

`preserve-bundled-files.test.ts` (3):

1. preserves bundled files from the saved document when the incoming factory omits them
2. keeps explicit incoming bundled files when the event payload includes them
3. keeps explicit empty bundled files when the incoming factory clears docs

`preserve-layout.test.ts` (2):

1. preserves layout from the existing factory when the incoming factory omits it
2. keeps explicit incoming layout when it is present

`lifecycle-controls.test.ts` (5):

1. posts pause controls to the shared session lifecycle endpoint
2. posts approval through the typed durable approval endpoint
3. posts retry-dispatch requests with the selected dispatch payload
4. posts interrupt-dispatch requests with the selected dispatch payload
5. returns typed conflict lifecycle outcomes instead of surfacing them as transport errors

`openapi.session-factory.test.ts` (4):

1. round-trips the typed provider catalog contract without changing meaning
2. exposes concrete text and image empty-state variants
3. supports a typed response-event stream consumer
4. exposes typed session factory save payloads and machine-readable error codes

`import-activation.discover.test.ts` (3):

1. returns an empty list when the session has no folder path
2. opens the session folder and returns sorted named factory targets
3. discovers names for the default session when sessionID is omitted

## Runtime and host

- Recorded: 2026-08-01 UTC
- Repository revision: `ab3f6226d` (`BUN-UNIT-api-01` after merging current `main`, including `BUN-UNIT-00-lane-foundation`)
- Bun: `1.3.12`
- Vitest: `4.1.3`
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you\.claude\worktrees\BUN-UNIT-api-01`

## Matched Vitest command

From `ui/`:

```text
.\node_modules\.bin\vitest.exe run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 `
  src/api/factory-definition/preserve-bundled-files.test.ts `
  src/api/factory-definition/preserve-layout.test.ts `
  src/api/factory-sessions/lifecycle-controls.test.ts `
  src/api/generated/openapi.session-factory.test.ts `
  src/api/session-factory/import-activation.discover.test.ts
```

Wrapper wall-clock is measured around that command with
`[System.Diagnostics.Stopwatch]`.

## Comparable baseline runs

| Run | Wrapper wall | Vitest wall | Transform | Import | Tests | Result |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 (first focused) | 1353ms | 426ms | 428ms | 556ms | 50ms | 5 files / 17 tests passed |
| 2 | 1155ms | 451ms | 460ms | 593ms | 60ms | 5 files / 17 tests passed |
| 3 | 1096ms | 448ms | 466ms | 597ms | 53ms | 5 files / 17 tests passed |
| 4 | 1099ms | 471ms | 276ms | 402ms | 49ms | 5 files / 17 tests passed |

Median of the three warm comparable samples (runs 2–4): wrapper wall
`1099ms`, Vitest wall `451ms`. All runs reported exit code `0`.

Representative raw reporter output (run 3):

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-api-01/ui
Test Files  5 passed (5)
Tests  17 passed (17)
Start at  12:03:46
Duration  448ms (transform 466ms, setup 0ms, import 597ms, tests 53ms, environment 0ms)
[timing] elapsed_ms=1096
[timing] exit_code=0
```

## After-state placeholder

Focused Bun after-state wall-clock for the same five files will be recorded in
story `BUN-UNIT-api-01-006` under conditions comparable to this baseline.
