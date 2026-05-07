# Cleanup Analyzer Report: Retire Raw Dashboard Session Read Models

Date: 2026-04-19

## Scope

Agent Factory cleanup evidence for retiring the raw simple-dashboard session read model. The simple CLI dashboard session sections now use canonical factory events, selected-tick `FactoryWorldState` reconstruction, and `FactoryWorldView.Runtime` instead of rebuilding session rows from raw `EngineStateSnapshot` helpers.

Historical cleanup reports under `libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are excluded from active-symbol conclusions because they intentionally preserve earlier analyzer output. The factory input idea at `libraries/agent-factory/factory/inputs/idea/default/retire-raw-dashboard-session-read-models.md` is also historical planning input, not active production code.

## Analyzer Commands

Historical before inventory:

```bash
git grep -n -E "RawSessionSummary|BuildRawSessionSummary|ProviderSessionAttempt|TraceTokenView|TraceMutationView|DispatchLineage|\<WorkItemRef\>" ace0f0269^ -- libraries/agent-factory/pkg
```

Deleted raw helper file inventory:

```bash
git show --stat --oneline ace0f0269
git show --stat --oneline 3cc16c294
```

Current active production inventory:

```bash
rg -n "RawSessionSummary|BuildRawSessionSummary|ProviderSessionAttempt|TraceTokenView|TraceMutationView|DispatchLineage|\bWorkItemRef\b" libraries/agent-factory/pkg -g "*.go"
```

Current compatibility-test inventory:

```bash
rg -n "RawSessionSummary|BuildRawSessionSummary|ProviderSessionAttempt|TraceTokenView|TraceMutationView|DispatchLineage|\bWorkItemRef\b" libraries/agent-factory/tests/functional_test -g "*.go"
```

Current repository inventory excluding historical cleanup reports:

```bash
rg -n "RawSessionSummary|BuildRawSessionSummary|ProviderSessionAttempt|TraceTokenView|TraceMutationView|DispatchLineage|\bWorkItemRef\b" libraries/agent-factory -g "!docs/development/cleanup-analyzer-reports/**"
```

`WorkItemRef` is matched with word boundaries so the accepted replacement type `FactoryWorldWorkItemRef` does not produce false raw-symbol matches.

## Before Inventory

The historical before inventory from `ace0f0269^` returned 96 matches across 11 active `pkg` files:

- `libraries/agent-factory/pkg/cli/dashboard/dashboard.go`
- `libraries/agent-factory/pkg/cli/dashboard/dashboard_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/dispatch_lineage.go`
- `libraries/agent-factory/pkg/cli/dashboard/dispatch_lineage_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/session_summary.go`
- `libraries/agent-factory/pkg/cli/dashboard/session_summary_test.go`
- `libraries/agent-factory/pkg/cli/dashboard/trace_view.go`
- `libraries/agent-factory/pkg/factory/projections/world_state.go`
- `libraries/agent-factory/pkg/factory/projections/world_view_test.go`
- `libraries/agent-factory/pkg/interfaces/dashboard_read_models.go`
- `libraries/agent-factory/pkg/interfaces/factory_world_state.go`

The before inventory contained the retired raw dashboard session model names:

- `RawSessionSummary`
- `BuildRawSessionSummary`
- `ProviderSessionAttempt`
- `TraceTokenView`
- `TraceMutationView`
- `DispatchLineage`
- raw `WorkItemRef`

The raw helper files present before cleanup were:

- `libraries/agent-factory/pkg/interfaces/dashboard_read_models.go`
- `libraries/agent-factory/pkg/cli/dashboard/session_summary.go`
- `libraries/agent-factory/pkg/cli/dashboard/trace_view.go`
- `libraries/agent-factory/pkg/cli/dashboard/dispatch_lineage.go`

## Cleanup Evidence

Commit `ace0f0269` deleted the raw session summary reducers and overlapping DTOs:

- `pkg/interfaces/dashboard_read_models.go`
- `pkg/cli/dashboard/session_summary.go`
- `pkg/cli/dashboard/session_summary_test.go`
- `pkg/cli/dashboard/trace_view.go`
- `pkg/cli/dashboard/dispatch_lineage.go`
- `pkg/cli/dashboard/dispatch_lineage_test.go`
- `tests/functional_test/dashboard_mixed_snapshot_test.go`

Commit `3cc16c294` recorded the broader event-first dashboard cleanup and added the prior cleanup report. The current branch adds focused CLI and service assertions that keep session rendering on `FactoryWorldView.Runtime` while preserving the aggregate `EngineStateSnapshot` shell for non-session dashboard status.

The review follow-up commit for this report adds explicit simple CLI rendering for `FactoryWorldView.Runtime.PlaceTokenCounts`, `FactoryWorldView.Runtime.CurrentWorkItemsByPlaceID`, `FactoryWorldView.Runtime.PlaceOccupancyWorkItemsByPlaceID`, and `FactoryWorldView.Runtime.WorkstationActivityByNodeID`. The simple CLI dashboard now exposes event-derived queue count and workstation activity sections instead of merely carrying those fields through the world-view contract.

## After Inventory

The current active production inventory returned no matches:

```bash
rg -n "RawSessionSummary|BuildRawSessionSummary|ProviderSessionAttempt|TraceTokenView|TraceMutationView|DispatchLineage|\bWorkItemRef\b" libraries/agent-factory/pkg -g "*.go"
```

Result: no active production matches.

The current compatibility-test inventory returned 11 matches in one file:

- `libraries/agent-factory/tests/functional_test/compatibility_read_models_test.go`

Those matches are local JSON mirror structs for older dashboard response compatibility assertions. They do not import or call `pkg/cli/dashboard` raw helper code, and they are outside the production `pkg` surface.

The current repository inventory excluding historical cleanup reports returned additional non-production or replacement-surface matches:

- `libraries/agent-factory/factory/inputs/idea/default/retire-raw-dashboard-session-read-models.md` records the original cleanup idea.
- `libraries/agent-factory/ui/src/**` uses `DashboardProviderSessionAttempt` and `DashboardWorkItemRef` TypeScript names for the browser dashboard API shape, not the removed Go raw dashboard read-model package.
- `libraries/agent-factory/pkg/**` uses `FactoryWorldWorkItemRef`, the event-derived world-view replacement. The bounded `\bWorkItemRef\b` production search above confirms the raw `WorkItemRef` name is not present as an active Go symbol.
- `libraries/agent-factory/tests/functional_test/compatibility_read_models_test.go` retains local compatibility mirror names as described above.

## EngineStateSnapshot Boundary

Allowed `EngineStateSnapshot` usage remains in these areas:

- Scheduler, dispatcher, transitioner, history, circuit-breaker, termination, and engine runtime packages under `pkg/factory`.
- API status handlers in `pkg/api`, including `statusFromEngineStateSnapshot`.
- Service aggregate status and lifecycle wiring in `pkg/service`.
- Runtime-internal, scheduler, service, API, and harness tests that inspect engine internals or wait for runtime state.

Dashboard presentation usage is limited to shell data. `pkg/cli/dashboard.FormatSimpleDashboard` renders only aggregate shell fields. `FormatSimpleDashboardWithWorldView` renders active executions, terminal history, failed details, provider sessions, token views, queue counts, and workstation activity from `FactoryWorldView.Runtime`. `pkg/service.factoryWorldView` obtains canonical factory events, calls `projections.ReconstructFactoryWorldState(...)` for the selected tick, and then calls `projections.BuildFactoryWorldView(...)` before rendering session sections.

## Validation Commands

```bash
cd libraries/agent-factory
go test ./pkg/cli/dashboard ./pkg/factory/projections ./pkg/service -count=1
make lint
```

Results on 2026-04-19:

- `go test ./pkg/cli/dashboard ./pkg/factory/projections ./pkg/service -count=1` passed.
- `make lint` passed with the deadcode baseline matching.

Additional review follow-up checks on 2026-04-19:

- `go test ./pkg/cli/dashboard -run TestFormatSimpleDashboardWithWorldView_RendersSessionMetricsAndActiveRowsFromWorldView -count=1` passed.
- `go test ./pkg/service -run TestFactoryService_SimpleDashboardRenderInputUsesFactoryWorldView -count=1` passed.
