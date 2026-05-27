# UI Coverage Speed Closeout

This closeout records the CI timing evidence for the first UI coverage speedup rollout on branch `ralph/ui-coverage-speed`.

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
