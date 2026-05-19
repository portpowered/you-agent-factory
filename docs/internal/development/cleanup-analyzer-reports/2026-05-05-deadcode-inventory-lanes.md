# Cleanup Analyzer Report: Deadcode Inventory Lanes

Date: 2026-05-05

## Scope

Inventory-only pass for `US-001`. This report classifies suspicious dead-code
surfaces into bounded cleanup lanes before any deletion work lands. The goal is
to separate real delete candidates from analyzer or graph false positives and
from compatibility seams that still back supported behavior.

## Evidence Commands

```powershell
Get-Content docs/development/deadcode-baseline.txt
rg -l "\bnewReplayContractHarness\b|\bloadBatchBoundarySummary\b|\bsubmissionsObservedAtTick\b" tests pkg ui cmd
rg -n "\bWithExecutionBaseDir\b|\bWriteSeedMarkdownFile\b|\bWriteSeedBatchFile\b" pkg tests
rg -n "\bLegacyFixtureDir\b" tests
rg -n "\bdistFSProvider\b|\bfallbackDistFS\b" ui/embed.go ui/embed_test.go
rg -n "\bdashboardCompatibilityTransitionID\b|\bdashboardCompatibilityWorkstationName\b" pkg/cli/dashboard
rg -n "\bLEGACY_SELECTION_WIDGET_IDS\b|\bLEGACY_WORK_OUTCOME_WIDGET_IDS\b|\bLegacyTimelineWorkCompat\b" ui/src
```

## Inventory

| Surface | Evidence | Classification | Reason |
| --- | --- | --- | --- |
| `tests/functional/replay_contracts/replay_harness_customization_test.go` | Helper-only file; no `Test*` entrypoints; `newReplayContractHarness` only matches in this file. | `delete` | Orphan helper file. Safe future cleanup lane is to remove the file after confirming no pending test resurrection depends on it. |
| `tests/functional/runtime_api/api_batch_submission_boundary_smoke_test.go` | Helper-only file; no `Test*` entrypoints; `loadBatchBoundarySummary` only matches in this file. | `delete` | Test-only dead lane. The file contains helper types and functions without live tests or external callers. |
| `tests/functional/replay_contracts/replay_adhoc_work_in_queue_scheduler_test.go` | Helper-only file; no `Test*` entrypoints; `submissionsObservedAtTick`, `maxRecordedDispatchesPerTick`, `firstTransitionTick`, and related helpers only match in this file. | `delete` | Another orphan test-helper cluster. Delete as one bounded file-level cleanup, not symbol-by-symbol. |
| `tests/functional/internal/support/fixtures.go:LegacyFixtureDir` | Referenced across workflow, smoke, runtime API, replay, and guard tests; implementation is only a path forwarder to `testutil.MustRepoPath(...)`. | `collapse to canonical owner` | Active behavior remains supported, but the `Legacy*` helper name is now misleading. Future cleanup should collapse callers onto a canonically named fixture-path helper rather than keep a permanent legacy owner. |
| `pkg/testutil/service_harness.go:WithExecutionBaseDir` | Flagged in `deadcode-baseline.txt`, but referenced by workflow and replay functional tests. | `retain` | Accepted baseline false positive today. The symbol still backs supported functional test setup. |
| `pkg/testutil/testutil.go:WriteSeedMarkdownFile` and `WriteSeedBatchFile` | Flagged in `deadcode-baseline.txt`, but referenced by workflow and guards-batch functional tests. | `retain` | Functional seed submission tests still exercise these helpers directly. Do not delete from baseline alone. |
| `ui/embed.go:fallbackDistFS` | Graph inbound callers are empty, but `distFSProvider = fallbackDistFS` and `DistFS()` dispatches through that provider; covered by `ui/embed_test.go`. | `retain` | Function-value assignment hides usage from simple call-graph reachability. This is a live fallback shell seam for supported backend-only builds. |
| `pkg/cli/dashboard/dashboard.go:dashboardCompatibilityTransitionID` and `dashboardCompatibilityWorkstationName` | Reachable from `FormatSimpleDashboardWithRenderData` through the CLI dashboard render path. | `retain` | Compatibility naming is temporary-looking, but the helpers still participate in supported dashboard rendering. |
| `ui/src/features/bento/useDashboardLayout.ts:LEGACY_SELECTION_WIDGET_IDS` and `LEGACY_WORK_OUTCOME_WIDGET_IDS` | Referenced by layout migration code that upgrades persisted dashboard widget IDs. | `retain` | These constants preserve supported local-storage migration behavior and are not dead. |
| `ui/src/features/work-outcome/useWorkOutcomeChart.ts:LegacyTimelineWorkCompat` | Used by runtime casts in the work-outcome chart. | `retain` | Still part of supported UI compatibility reads. Removal belongs in a later event-contract cleanup lane, not deadcode deletion. |

## Lane Summary

- `delete` lanes are currently strongest in orphaned helper-only `_test.go`
  files that no longer own any executable tests.
- `collapse to canonical owner` is the right treatment for active but
  misleading legacy wrappers such as `LegacyFixtureDir`.
- `retain` is required for the current deadcode-baseline hits that still back
  long-tag, functional, dashboard, or embed behavior.

## Notes

- `docs/development/deadcode-baseline.txt` is useful as an inventory seed, but
  it is not a direct delete list in this branch.
- Call-graph evidence alone is insufficient for function values such as
  `distFSProvider = fallbackDistFS`; verify variable assignment and test-backed
  entrypoints before deleting graph-zero symbols.
