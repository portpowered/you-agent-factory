# Bun Unit Seed Timing

This record compares the first Bun-native dashboard unit seed with the same
one-test workload under the existing Vitest `dashboard-unit` project. The
measurements are local diagnostic evidence for the migration contract; one seed
does not establish a repository-wide performance improvement.

## Workload

- Test file: `ui/src/lib/cn.bun.unit.test.ts` after migration; the equivalent
  baseline file was `ui/src/lib/cn.test.ts`.
- Test name: `cn > joins only truthy class segments in order`.
- Assertions: one.
- Passing tests: one.
- Revision-equivalent source: the same `cn` assertion and production helper;
  only the test filename and test API import changed.

## Runtime and host

- Recorded: 2026-08-01 UTC.
- Repository revision for the baseline: `9fd98265c`.
- Bun measurement revision-equivalent worktree: `9fd98265c` plus only the
  filename and test API import migration; `ui/src/lib/cn.ts` was unchanged.
- Bun: `1.3.12`.
- Vitest: `4.1.3`.
- Host: Microsoft Windows 11 Home 64-bit, version `10.0.26200`; 13th Gen
  Intel Core i7-13700K with 24 logical processors; worktree at
  `C:\Users\andre\work\portos\infinite-you`.

## Matched commands and raw results

### Vitest baseline

Command:

```text
bunx --no-install vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0 src/lib/cn.test.ts
```

Raw result:

```text
RUN  v4.1.3 C:/Users/andre/work/portos/infinite-you/.claude/worktrees/BUN-UNIT-00-lane-foundation/ui
Test Files  1 passed (1)
Tests  1 passed (1)
Start at 03:05:11
Duration  3.63s (transform 641ms, setup 1.36s, import 385ms, tests 7ms, environment 0ms)
[timing] elapsed_ms=10254
[timing] exit_code=0
```

### Bun seed

Command:

```text
bun run test:unit:bun
```

The runner reports the discovered `.bun.unit.test.ts` file and expected seed
count before invoking Bun.

Raw result:

```text
[ui-unit:bun] discovered 1 file(s): src\lib\cn.bun.unit.test.ts
[ui-unit:bun] expected seed test count: 1; Bun will report the terminal file and test totals below
bun test v1.3.12 (700fc117)
[timing] elapsed_ms=327
[timing] exit_code=0
$ bun scripts/run-bun-unit-tests.ts
src\lib\cn.bun.unit.test.ts:
(pass) cn > joins only truthy class segments in order
1 pass
0 fail
1 expect() calls
Ran 1 test across 1 file. [63.00ms]
```

The two measurements must be read as matched one-test observations only. They
must not be converted into a repository-wide speedup claim.
