# Cleanup Analyzer Report: Raw Dashboard Session Summary

Date: 2026-04-18

## Scope

Agent Factory cleanup pass for retiring the raw CLI dashboard session summary. Simple CLI dashboard session accounting now comes from generated `FactoryEvent` history, reconstructed `FactoryWorldState`, and `FactoryWorldView`.

## Analyzer Commands

Initial branch-base inventory:

```bash
git grep -n -E "BuildRawSessionSummary|RawSessionSummary|DeriveDispatchLineage|DispatchLineage|TraceTokenView|TraceMutationView|ProviderSessionAttempt" origin/main -- libraries/agent-factory/pkg libraries/agent-factory/tests/functional_test
```

Final active production inventory:

```bash
rg -n "BuildRawSessionSummary|RawSessionSummary|DeriveDispatchLineage|DispatchLineage|TraceTokenView|TraceMutationView|ProviderSessionAttempt" libraries/agent-factory/pkg -g "*.go"
```

Final retained compatibility-test inventory:

```bash
rg -n "BuildRawSessionSummary|RawSessionSummary|DeriveDispatchLineage|DispatchLineage|TraceTokenView|TraceMutationView|ProviderSessionAttempt" libraries/agent-factory/tests/functional_test -g "*.go"
```

Deleted-file inventory:

```bash
git diff --name-only --diff-filter=D origin/main...HEAD -- libraries/agent-factory/pkg/cli/dashboard libraries/agent-factory/pkg/interfaces libraries/agent-factory/tests/functional_test
```

## Initial Inventory

The branch-base inventory returned 94 matches across 17 files:

- `libraries/agent-factory/pkg/cli/dashboard/dashboard.go`
- `libraries/agent-factory/pkg/cli/dashboard/dashboard_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/dispatch_lineage.go`
- `libraries/agent-factory/pkg/cli/dashboard/dispatch_lineage_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/session_summary.go`
- `libraries/agent-factory/pkg/cli/dashboard/session_summary_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/trace_view.go`
- `libraries/agent-factory/pkg/factory/projections/world_state.go`
- `libraries/agent-factory/pkg/interfaces/dashboard_read_models.go`
- `libraries/agent-factory/pkg/interfaces/factory_world_state.go`
- `libraries/agent-factory/tests/functional_test/compatibility_read_models_test.go`
- `libraries/agent-factory/tests/functional_test/dashboard_inflight_test.go`
- `libraries/agent-factory/tests/functional_test/dashboard_mixed_snapshot_test.go`
- `libraries/agent-factory/tests/functional_test/dashboard_snapshot_test.go`
- `libraries/agent-factory/tests/functional_test/dispatch_timing_test.go`
- `libraries/agent-factory/tests/functional_test/resource_token_name_test.go`
- `libraries/agent-factory/tests/functional_test/runtime_state_test.go`

## Cleanup Applied

- Routed simple CLI dashboard rendering through `service.SimpleDashboardRenderInput`, preserving aggregate `EngineStateSnapshot` diagnostics while adding event-first `FactoryWorldView` session data.
- Moved active rows, completed/failed rows, dispatch history, failed work details, trace projections, and provider-session rows to `FactoryWorldView`-derived adapters.
- Removed raw reducer files: `pkg/cli/dashboard/session_summary.go`, `pkg/cli/dashboard/dispatch_lineage.go`, and `pkg/cli/dashboard/trace_view.go`.
- Removed raw reducer tests: `pkg/cli/dashboard/session_summary_test.go` and `pkg/cli/dashboard/dispatch_lineage_test.go`.
- Deleted overlapping dashboard DTO definitions in `pkg/interfaces/dashboard_read_models.go`.
- Removed the raw-summary functional smoke `tests/functional_test/dashboard_mixed_snapshot_test.go`.
- Migrated functional runtime assertions away from `pkg/cli/dashboard` helpers and onto generated events, `FactoryWorldView`, or package-local token identity helpers.
- Strengthened projection coverage for session counts, active execution labels, completed dispatch labels, failed work details, provider-session attempts, trace token views, mutation views, and hidden cron `__system_time` counts.

## Final Removed-Symbol Evidence

- `rg -n "BuildRawSessionSummary|RawSessionSummary|DeriveDispatchLineage|DispatchLineage|TraceTokenView|TraceMutationView|ProviderSessionAttempt" libraries/agent-factory/pkg -g "*.go"` returned no active production matches.
- `rg -n "BuildRawSessionSummary|RawSessionSummary|DeriveDispatchLineage|DispatchLineage|TraceTokenView|TraceMutationView|ProviderSessionAttempt" libraries/agent-factory/tests/functional_test -g "*.go"` returned 11 matches in one file: `tests/functional_test/compatibility_read_models_test.go`.
- The compatibility-test matches are local JSON mirror structs for older dashboard response shape assertions; they do not import or call CLI dashboard helpers.

## Retained Exceptions

- `tests/functional_test/compatibility_read_models_test.go` intentionally retains legacy JSON field names such as `ProviderSessionAttempt`, `TraceTokenView`, and `TraceMutationView` so compatibility response decoding stays explicit.
- Low-level `GetEngineStateSnapshot` diagnostics and `/status` token counts remain available. The cleanup only changes dashboard session accounting.
- Historical cleanup reports may mention retired symbol names as audit evidence.

## Smoke Coverage

- `TestFactoryService_SimpleDashboardRenderInputUsesFactoryWorldView` runs representative success and failure work through service mode, builds `FactoryWorldView` from generated events, renders the simple CLI dashboard, and verifies active rows, session metrics, completed rows, failed details, provider sessions, and dispatch history text.
- `TestDashboard_EngineStateSnapshot_EndToEnd` keeps functional coverage on `ServiceTestHarness.GetFactoryEvents`, selected-tick world-state reconstruction, and `FactoryWorldView` terminal session behavior.
- `TestFormatSimpleDashboardWithWorldView_RendersSessionMetricsAndActiveRowsFromWorldView` and `TestFormatSimpleDashboardWithWorldView_RendersTerminalProviderAndDispatchDetailsFromWorldView` verify the formatter ignores conflicting raw snapshot labels and uses only world-view data for session output.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/cli/dashboard ./pkg/factory/projections ./pkg/service ./tests/functional_test -count=1
make lint
cd ../..
make check-go-architecture-quality
```
