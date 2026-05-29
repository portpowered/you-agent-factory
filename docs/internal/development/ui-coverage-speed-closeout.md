# UI Coverage Speed Closeout

This closeout records UI Coverage lane timing evidence for speedup rollouts and the `ralph/latency-analysis-ui-test-coverage` latency analysis initiative.

## Current baseline (latency analysis, 2026-05-30 UTC)

Use this section as the comparison anchor for follow-up stories in the latency-analysis initiative. All phase labels come from stable `[ui-coverage] … elapsed` output emitted by `make test-ui-coverage` (via `ui/scripts/ui-coverage-runner.mjs` and the Makefile replay step).

| Setting | Value |
| --- | --- |
| Canonical command | `make test-ui-coverage` |
| Main-pass workers | `UI_COVERAGE_MAIN_MAX_WORKERS` unset → runner default **2** (`ui/scripts/ui-coverage-runner.mjs`) |
| Browser integration | **Not included** — `make ui-integration-test` is a separate required lane (Playwright + preview build). Do not attribute its wall time to the UI Coverage figures below unless comparing full `make verify-tests` frontend cost. |

### CI reference (GitHub Actions `UI Coverage` job)

| Field | Value |
| --- | --- |
| Workflow run | [`26654493647`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647) |
| Head SHA | `4f63b7fe033f4998cb672adcec01be92c5090453` (merged `main`, 2026-05-29 UTC) |
| Job | [`78561728707`](https://github.com/portpowered/you-agent-factory/actions/runs/26654493647/job/78561728707) (`ubuntu-latest`, default two main-pass workers) |
| `Run dashboard coverage lane` step wall | **8m52s** (2026-05-29T18:24:05Z → 2026-05-29T18:32:57Z) |
| `UI Coverage` job wall | **9m17s** (2026-05-29T18:23:44Z → 2026-05-29T18:33:01Z, includes checkout/setup/install) |

| Phase (`[ui-coverage]` label) | Elapsed (CI) |
| --- | ---: |
| Main covered Vitest pass | 403.37s |
| Isolated React Flow covered pass | 125.50s |
| Blob report merge pass | 1.78s |
| Standalone script-style test | 1.31s |
| Replay coverage check | 0.00s |
| **Phase total (lane command only)** | **~531.96s (~8m52s)** |

Reproduce CI phase lines locally after a failing or successful lane:

```bash
make test-ui-coverage 2>&1 | rg '\[ui-coverage\].*elapsed'
```

### Local reference (maintainer workstation)

| Field | Value |
| --- | --- |
| Date (UTC) | 2026-05-29T18:21:06Z → 2026-05-29T18:36:39Z |
| Environment | macOS Darwin arm64, 10 logical CPUs, Bun `ui/` workspace (`bun install` then `make test-ui-coverage`) |
| Head | `4f63b7fe033f4998cb672adcec01be92c5090453` worktree on `ralph/latency-analysis-ui-test-coverage` |
| `/usr/bin/time -p` wall (`make test-ui-coverage`) | **932.92s (~15m33s)** |

| Phase (`[ui-coverage]` label) | Elapsed (local) |
| --- | ---: |
| Main covered Vitest pass | 846.11s |
| Isolated React Flow covered pass | 83.87s |
| Blob report merge pass | 1.50s |
| Standalone script-style test | 0.94s |
| Replay coverage check | 0.00s |
| **Phase total** | **~932.42s** |

Local runs are useful for iteration and slow-file tuning but are **not** interchangeable with CI job wall time; prefer the CI table above when estimating PR lane cost. The main covered pass dominates both environments; local App shell and jsdom suites ran slower than the comparable CI runner in this snapshot.

## Historical rollout (first speedup, `ralph/ui-coverage-speed`)

This section records the CI timing evidence for the first UI coverage speedup rollout on branch `ralph/ui-coverage-speed`.

## Evidence Sources

All timestamps are UTC.

| Evidence | Run | Head | UI Coverage job |
| --- | --- | --- | --- |
| Baseline before timing and worker changes | [`26525489072`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072) | `a2213c98f4ac3b8bebcc3d8c0bacc5b9ac670fe3` | [`78128635406`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072/job/78128635406) |
| Post-change PR run | [`26527975889`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889) | `2e13a5c30265790ac070cde5fdb2c47eb8a1be36` | [`78137472057`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889/job/78137472057) |

## Timing Summary

| Phase | Baseline CI | Post-change CI |
| --- | ---: | ---: |
| Main covered Vitest pass | 630.57s | 330.89s |
| Isolated React Flow covered pass | 123.21s | 106.69s |
| Blob report merge pass | about 1.0s from log boundaries | 1.77s |
| Standalone script-style test | 0.83s | 1.18s |
| Replay coverage check | about 0.01s from log boundaries | 0.00s |
| `Run dashboard coverage lane` step wall time | 12m40s | 7m21s |
| `UI Coverage` job wall time | 13m02s | 7m48s |

The post-change run improved the UI Coverage job by 5m14s, from 13m02s to 7m48s. The largest movement was the main covered Vitest pass, which dropped from 630.57s to 330.89s after the main coverage corpus moved from one worker to the runner-owned default of two workers.

## Notes

The baseline run predates the stable `[ui-coverage] ... elapsed` output, so baseline phase values were mapped to the same rollout phase names from the existing CI log: Vitest's per-run `Duration` lines for the main and isolated covered passes, the timestamp boundary around `vitest --mergeReports` for the blob merge pass, the standalone script test's `Duration` line, and the timestamp boundary around `bun scripts/write-replay-coverage-report.ts --check` for replay coverage. The post-change run uses the stable phase labels emitted by `make test-ui-coverage`.

The post-change CI run passed Typecheck, Build/Lint/API, UI Coverage, UI Browser Integration, and Backend Verification. Because the measured UI Coverage lane improved materially and passed, the two-worker main pass remains the default for this rollout.

## Guardrails

Keep `make test-ui-coverage` as the canonical root command for local and CI reruns. That target delegates the covered UI phases to `ui/package.json`'s `test:coverage` script, then runs the replay coverage check; do not replace it with a workflow-only shell sequence.

The package-owned `test:coverage` flow should keep its phase definitions in `ui/scripts/ui-coverage-runner.mjs`. The main covered Vitest pass is the phase that is safe to parallelize in this rollout and defaults to two workers through the runner-owned `UI_COVERAGE_MAIN_MAX_WORKERS` seam. Use that environment variable for local or CI comparison runs before changing the default.

The isolated `src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` pass remains intentionally separate and conservative with `--maxWorkers=1`. Keep that boundary unless future comparable CI evidence shows that merging or further parallelizing the React Flow-heavy pass improves runtime without adding flake.

The blob report merge remains the owner of coverage threshold enforcement through `ui/vite.config.ts`. The main covered pass should continue to exclude browser-backed `integration/*.integration.test.mjs`, the standalone script-style `scripts/dashboard-shell-storybook-responsive.test.mjs`, and the isolated React Flow card test so each surface stays in its intended lane.

Replay coverage is part of the preserved verification contract. Changes to UI coverage orchestration should keep the trailing replay coverage check visible through `make test-ui-coverage` and preserve the stable `[ui-coverage]` elapsed labels for the main covered pass, isolated React Flow covered pass, blob report merge pass, standalone script-style test, and replay coverage check.
