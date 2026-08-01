# Dashboard-unit baseline

Status: `LTV-UI-TEST-01-unit-lane-throughput-001` complete

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

## Verification

- `bun run typecheck` passed.
- `bun run check:test-lanes` passed.
- Focused `scripts/check-ui-test-lanes.test.mjs` passed (20 tests).
- `bun run lint` passed; it reported existing legacy allowlist information.
- `bun run check` remains red before the lane checks because Biome reports 275
  pre-existing formatting/import diagnostics in unrelated integration and
  generated package files. No baseline-change file is implicated, and those
  unrelated files were not modified for this story.
