# UI Coverage Speed Closeout

This closeout records the CI timing evidence for the first UI coverage speedup rollout on branch `ralph/ui-coverage-speed`.

## Evidence Sources

All timestamps are UTC.

| Evidence | Run | Head | UI Coverage job |
| --- | --- | --- | --- |
| Baseline before timing and worker changes | [`26525489072`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072) | `a2213c98f4ac3b8bebcc3d8c0bacc5b9ac670fe3` | [`78128635406`](https://github.com/portpowered/you-agent-factory/actions/runs/26525489072/job/78128635406) |
| Post-change PR run | [`26527975889`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889) | `2e13a5c30265790ac070cde5fdb2c47eb8a1be36` | [`78137472057`](https://github.com/portpowered/you-agent-factory/actions/runs/26527975889/job/78137472057) |

## Timing Summary

| Phase (`[ui-coverage]` elapsed label) | Baseline CI (Vitest) | Post speedup CI (Vitest) | Bun migration (2026-05-30) |
| --- | ---: | ---: | ---: |
| Main covered pass (`Main covered Vitest pass`) | 630.57s | 330.89s | Bun `run-bun-coverage-main.mjs` batches |
| Isolated React Flow covered pass | 123.21s | 106.69s | `bun test` single-worker on card spec |
| Blob report merge pass | about 1.0s from log boundaries | 1.77s | `merge-bun-coverage-thresholds.mjs` lcov merge |
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

The package-owned `test:coverage` flow should keep its phase definitions in `ui/scripts/ui-coverage-runner.mjs`. The main covered pass (logged as `Main covered Vitest pass` for stable `[ui-coverage]` labels) is the phase that is safe to parallelize in this rollout and defaults to two workers through the runner-owned `UI_COVERAGE_MAIN_MAX_WORKERS` seam on Bun. Use that environment variable for local or CI comparison runs before changing the default.

The isolated `src/features/workflow-activity/components/react-flow-current-activity-card.test.tsx` pass remains intentionally separate and conservative with `--maxWorkers=1`. Keep that boundary unless future comparable CI evidence shows that merging or further parallelizing the React Flow-heavy pass improves runtime without adding flake.

The blob report merge pass (logged as `Blob report merge pass`) remains the owner of coverage threshold enforcement through `ui/scripts/bun-coverage-config.mjs` and `ui/scripts/merge-bun-coverage-thresholds.mjs`. The main covered pass should continue to exclude browser-backed `integration/*.integration.test.mjs`, the standalone script-style `scripts/dashboard-shell-storybook-responsive.test.mjs` (Vitest-only `Standalone script-style test` phase), and the isolated React Flow card test so each surface stays in its intended lane.

Replay coverage is part of the preserved verification contract. Changes to UI coverage orchestration should keep the trailing replay coverage check visible through `make test-ui-coverage` and preserve the stable `[ui-coverage]` elapsed labels for the main covered pass, isolated React Flow covered pass, blob report merge pass, standalone script-style test, and replay coverage check.

## Bun coverage threshold baseline (2026-05-30)

After migrating merge/threshold enforcement to Bun lcov merge (`ui/scripts/merge-bun-coverage-thresholds.mjs`), the first green local run on branch `ralph/ui-bun-test-migration` measured **87.80%** statements/lines on the Vitest-equivalent exclude set. Bun lcov output in this rollout does not emit branch/function summary records (`BRF`/`BRH`/`FNF`/`FNH`), so branch/function thresholds remain configured but are skipped until Bun exposes comparable metrics.

Until a follow-up closes the engine gap, `ui/scripts/bun-coverage-config.mjs` temporarily enforces **87.5%** statements/lines (statements and lines only) instead of the historical Vitest v8 **93.1%** floors from the pre-migration `ui/vite.config.ts` `test.coverage` block. Record any future threshold ratchet in this closeout after comparable Bun CI evidence.
