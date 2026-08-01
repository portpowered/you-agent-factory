# Dashboard-unit baseline

Status: `LTV-UI-TEST-01-unit-lane-throughput-003` complete

Recorded: 2026-08-01T09:39:10Z

This is the reproducible baseline for the isolated Node-only `dashboard-unit`
project. It is a measurement artifact for the harness work; it does not change
production state, test assertions, lane classification, or the component,
browser, performance, or Storybook lanes.

## Reproduction conditions

- Revision: `08ae95c0e141a71a913c73fe64aab13476146ad4`
- Host: Windows 11 Home `10.0.26200` / build `26200`, 24 logical processors,
  63.72 GiB physical memory
- Runtime: Bun `1.3.12`, Node `22.12.0`, Vitest `4.1.3`
- Dependency state: installed from `ui/bun.lock`; local Vite/Vitest caches were
  left warm between the comparable samples
- Timing command, from `ui/`:

  ```text
  bun run test:unit -- --logHeapUsage --no-color
  ```

  The package script expands to the following runner policy, which is part of
  the evidence and must remain unchanged for matched comparisons:

  ```text
  vitest run --config vitest.lanes.config.ts --project=dashboard-unit --maxWorkers=4 --retry=0
  ```

- The selected project uses the Node environment, keeps Vitest's default
  per-file isolation, and does not run the component project.

The classified workload fingerprint was captured separately with:

```text
bunx --no-install vitest list --config vitest.lanes.config.ts --project=dashboard-unit --json=<temporary-output>
```

The JSON contained 3,161 test entries across 390 unique files. The normalized,
sorted repository-relative file paths have SHA-256:

```text
87768de6badf4fe4af908ec0e35f99a337dd57c45bae09c36067b27f6e6604d1
```

The path hash is a comparison guard, not a replacement for behavioral tests.
The three run logs each reported 390 passed files and 3,161 passed tests.

## Comparable baseline runs

Vitest phase values are aggregate worker time. The `import` label in Vitest
4.1.3's default reporter is the runner's aggregate `collectDuration`; it is
shown as `collection/import` below because the reporter does not emit a second
aggregate module-import value. `tests` is aggregate test-body time.

| Run | Wrapper wall | Vitest wall | Transform | Setup | Collection/import | Test bodies | Environment | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 95.89s | 93.42s | 13.51s | 185.03s | 48.42s | 12.21s | 57ms | 390 files / 3,161 tests passed |
| 2 | 100.21s | 98.45s | 16.65s | 162.69s | 72.25s | 14.04s | 58ms | 390 files / 3,161 tests passed |
| 3 | 83.96s | 82.63s | 30.48s | 130.89s | 71.47s | 11.25s | 47ms | 390 files / 3,161 tests passed |

Medians are `95.89s` wrapper wall and `93.42s` Vitest wall. Median phase values
are transform `16.65s`, setup `162.69s`, collection/import `71.47s`, test
bodies `12.21s`, and environment `57ms`. No run reported a failure, leak,
worker crash, out-of-memory condition, or file-lock symptom (`EACCES`, `EBUSY`,
or equivalent).

## Resource observation

Resource sampling was run as a separate probe and is not part of the timing
median because process inspection added material observer overhead. A fixed
runner-tree probe observed:

- 9 processes in the fixed tree at peak, including the Bun launcher, Vitest
  manager, Node/Bun workers, the console host, and the esbuild helper;
- at most 5 Bun/Node descendants at once, consistent with a manager plus the
  configured four worker slots;
- 672.2 MiB peak aggregate working set for that tree;
- no lock/contention error in the probe output.

An earlier all-process CIM sampler observed 10 tree processes, 6 Bun/Node
descendants, and 662.4 MiB, but inflated Vitest wall time from the normal
82.63–98.45s sample range to 113.85s. It is retained as a diagnostic
cross-check only, not as a performance sample. The worker count therefore
remains a measured, documented baseline variable rather than an optimization
claim.

## Bottleneck and next hypothesis

The dominant repeated cost is per-file Vitest preparation: the median aggregate
setup plus collection/import time is `234.16s` of worker time, compared with
`12.21s` of aggregate test-body time. Environment setup is negligible and the
three file/test counts are stable. This points to repeated harness transform,
setup, and module-collection work across isolated files rather than slow
behavioral assertions.

Story 002 should test one bounded hypothesis inside the leased dashboard-unit
runner/shared setup boundary: remove a proven redundant preparation path while
retaining Node-only isolation, the same project selection, four-worker policy,
and retries disabled. Worker-count changes are not justified by this baseline
alone. Any accepted change must repeat the same workload fingerprint and three
matched runs before it can claim a throughput improvement.

## Story 002 implementation

Recorded: 2026-08-01T09:59:42Z

The redundant preparation path was the root Vitest setup inherited by the
Node-only project. The project already declared `setupFiles: []`, but Vitest's
`extends: true` merge preserves array values from the root config, so the
dashboard-unit workers still installed the shared Testing Library and guarded
console setup for every file. The leased runner now detects an explicit
`dashboard-unit` project invocation and removes that setup at the root-config
boundary. The same condition also omits the React SWC transform plugin and
React/Radix/charting dependency optimization and inlining hints that the
Node-only workload does not import.

The change does not alter the unit file globs, Node environment, per-file
isolation, four-worker policy, retries-disabled policy, aliases, or any other
test project. Component and combined Vitest invocations retain the shared
setup and browser-oriented preparation.

The first validation run after the change kept the baseline fingerprint at 390
files and 3,161 tests, all passing. It was directional evidence; the matched
three-run throughput proof is recorded below.

| Run | Wrapper wall | Vitest wall | Transform | Setup | Collection/import | Test bodies | Environment | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 70.34s | 67.09s | 29.11s | 0ms | 88.27s | 12.32s | 64ms | 390 files / 3,161 tests passed |

## Story 003 matched after-change runs

Recorded: 2026-08-01T10:13:24Z

The three after-change runs used the same command, host, warm-cache conditions,
project selection, four-worker policy, retries-disabled policy, and normal
output flags as the baseline samples:

```text
bun run test:unit -- --logHeapUsage --no-color
```

The current revision was `3f84770e3c0721facf7abee2ceb0c5169b5f6d4d`; the host
was Windows 11 Home `10.0.26200` / build `26200`, with 24 logical processors
and 63.72 GiB physical memory. Runtime versions were Bun `1.3.12`, Node
`22.12.0`, and Vitest `4.1.3`.

The workload remained 3,161 test entries across 390 unique files. The
normalized, sorted `ui/`-relative file paths retained the baseline SHA-256:

```text
87768de6badf4fe4af908ec0e35f99a337dd57c45bae09c36067b27f6e6604d1
```

| Run | Wrapper wall | Vitest wall | Transform | Setup | Collection/import | Test bodies | Environment | Result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 95.79s | 89.13s | 44.27s | 0ms | 115.90s | 18.62s | 73ms | 390 files / 3,161 tests passed |
| 2 | 69.23s | 66.54s | 24.95s | 0ms | 83.19s | 14.49s | 78ms | 390 files / 3,161 tests passed |
| 3 | 55.05s | 53.17s | 15.84s | 0ms | 65.48s | 13.65s | 67ms | 390 files / 3,161 tests passed |

The matched wrapper median was `69.23s`, down from the baseline median of
`95.89s` (`27.80%` faster). The Vitest wall median was `66.54s`, down from
`93.42s` (`28.77%` faster). Setup remained `0ms` in every optimized run;
aggregate collection/import and transform values varied with the host, while
the wall-time result remained above the required 20% improvement.

All three runs were terminal and passing with identical file/test counts. None
reported a file-lock symptom (`EACCES`, `EBUSY`, or equivalent), worker crash,
out-of-memory failure, or test failure. The runner retained the baseline
four-worker setting, and no concurrency increase or timeout change was used.
The separate baseline process-tree observation remains the resource evidence
for that unchanged worker policy because sampling during a timed run materially
inflated wall time.

## Story 003 verification

- `bun run typecheck` passed.
- `bun run lint` passed, including the UI lane and boundary checks.
- `bun run check:test-lanes` passed.
- Focused `bunx --no-install vitest run --config vite.config.ts vite.config.test.ts --no-color` passed (3 tests).
- The full `bun run test:unit -- --logHeapUsage --no-color` lane passed three
  times with the matched workload above.

The broader `bun run check` remains red on the same pre-existing Biome
diagnostics in unrelated integration and generated package files recorded for
Story 002; no such files changed in this story.

## Verification

- `bun run typecheck` passed.
- `bun run check:test-lanes` passed.
- Focused `scripts/check-ui-test-lanes.test.mjs` passed (20 tests).
- `bun run lint` passed; it reported existing legacy allowlist information.
- `bun run check` remains red before the lane checks because Biome reports 275
  pre-existing formatting/import diagnostics in unrelated integration and
  generated package files. No baseline-change file is implicated, and those
  unrelated files were not modified for this story.
