# Simple Dashboard Topology Transport Inventory

This artifact records the inventory that drove retirement of
`service.SimpleDashboardRenderInput.Topology` on the simple-dashboard render
path. It remains as a durable record of the removed seam and the supported
formatter behavior that now relies only on `EngineState.Topology`.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-simple-dashboard-topology-transport-retirement`)
- Owner: Codex branch `ralph/agent-factory-simple-dashboard-topology-transport-retirement`
- Packages or subsystems: `pkg/service`, `pkg/cli/run`, `pkg/cli/dashboard`
- Canonical process doc to update before completion: `docs/processes/agent-factory-development.md`

## Retirement Summary

| Location | Prior dependency | Current state | What it proves |
| --- | --- | --- | --- |
| `pkg/service/factory.go:SimpleDashboardRenderInput` | field definition | removed | The service seam no longer advertises a second topology source. |
| `pkg/service/factory.go:buildSimpleDashboardRenderInput(...)` | write | removed | The service now transports only `EngineState`, render data, and `Now`. |
| `pkg/cli/run/run.go:renderSimpleDashboard(...)` | read | narrowed | The CLI pass-through now calls `FormatSimpleDashboardWithRenderData(...)` without an explicit topology argument. |
| `pkg/cli/dashboard/dashboard.go:FormatSimpleDashboardWithRenderData(...)` | compatibility arg | narrowed | The supported formatter seam now takes only `EngineState`, render data, and `Now`, sourcing topology from `EngineState.Topology`. |
| `pkg/service/factory_test.go:TestFactoryService_BuildSimpleDashboardRenderInputProjectsSelectedTickFromEvents` | assert | updated | The focused service test now proves selected-tick render data without asserting a redundant topology copy. |
| `pkg/cli/dashboard/dashboard_test.go` focused formatter tests | fixture input | updated | Focused CLI tests now populate `EngineState.Topology` and exercise the supported path directly. |

## Supported Production Path

1. `FactoryService.buildSimpleDashboardRenderInput(...)` reads the engine snapshot
   and returns `EngineState: *es`, event-derived render data, and `Now`.
2. `pkg/cli/run/renderSimpleDashboard(...)` forwards those values into
   `dashboard.FormatSimpleDashboardWithRenderData(...)`.
3. `pkg/cli/dashboard/formatSimpleDashboard(...)` uses `EngineState.Topology` for
   the remaining topology-backed queue, workstation-label, and session lookups.

That means the removed `SimpleDashboardRenderInput.Topology` transport was never
an independent production data source. It was a duplicate copy of the same
topology already present on `EngineState`.

## Formatter Sections That Still Need Topology

The simple dashboard still has supported topology-dependent formatting, but the
dependency is already satisfiable from `EngineState.Topology`.

| Formatter concern | Topology use | Current proof |
| --- | --- | --- |
| Queue counts | `state.SplitPlaceID(placeID)` and work-type display names | `pkg/cli/dashboard/dashboard.go:displayQueuePlace(...)` renders names from topology-backed work-type lookup. |
| Completed and failed workstation labels | transition-to-workstation compatibility mapping, including `time:expire` | `TestFormatSimpleDashboardWithRenderData_MapsSystemTimeCompatibilityAtCliBoundary` passes when the formatter resolves system-time workstation labels. |
| Session summaries | terminal and failed state-category lookup | Existing formatter path still derives terminal and failed sections through topology-backed place-category lookup and render DTO fallbacks. |

The supported behavior is therefore "formatter needs topology", not "formatter
needs a second explicit topology transport field".

## Focused Test Inventory

| Test | Current dependency | Retirement impact |
| --- | --- | --- |
| `TestFactoryService_SimpleDashboardRenderInputUsesRenderData` | Confirms `completed.EngineState.Topology` remains present, not `input.Topology` | Already aligned with the supported path. |
| `TestFactoryService_BuildSimpleDashboardRenderInputProjectsSelectedTickFromEvents` | Proves selected-tick render data while asserting `input.EngineState.Topology == topology` | Aligned with the supported path. |
| `TestFormatSimpleDashboardWithRenderData_RendersSessionMetricsAndActiveRows` | Populates `EngineState.Topology` and calls the narrower formatter seam directly | Proves queue, activity, and session output from the supported path. |
| `TestFormatSimpleDashboardWithRenderData_RendersTerminalProviderAndDispatchDetails` | Populates `EngineState.Topology` and calls the narrower formatter seam directly | Proves terminal, failed, and provider-session output from the supported path. |
| `TestFormatSimpleDashboardWithRenderData_MapsSystemTimeCompatibilityAtCliBoundary` | Uses `EngineState.Topology` only | Proves the compatibility workstation-name mapping still works on the supported path. |

## Closeout Conditions

- Keep `formatSimpleDashboard(...)` support for topology-backed queue and
  session sections sourced from `EngineState.Topology`.
- Keep the supported render seam narrowed to `EngineState`, render data, and
  `Now`; do not reintroduce another explicit topology transport by a different
  name.
- Keep focused CLI, service, and selected-tick API regressions on the supported
  path so future cleanups cannot revive the removed compatibility seam.

## Verification

- `cd libraries/agent-factory && go test ./pkg/cli/dashboard ./pkg/service ./pkg/api -count=1`
