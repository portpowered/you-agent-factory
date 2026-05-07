# Simple Dashboard Render DTO Data Model

This artifact inventories the exact simple-dashboard formatter inputs on the
`pkg/service` to `pkg/cli/dashboard` seam that now uses a dedicated render DTO
instead of transporting `interfaces.FactoryWorldView` into the formatter.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-simple-dashboard-render-dto-split`)
- Owner: Codex branch `ralph/agent-factory-simple-dashboard-render-dto-split`
- Packages or subsystems: `pkg/service`, `pkg/cli/dashboard`, `pkg/interfaces`, `pkg/factory/projections`
- Canonical process doc to update before completion: `docs/processes/agent-factory-development.md`

## Seam Summary

1. `FactoryService.buildSimpleDashboardRenderInput(...)` reads the engine
   snapshot, reconstructs canonical event-first state through
   `simpleDashboardRenderData(...)`, and returns `SimpleDashboardRenderInput`.
2. `FactoryService.renderDashboard(...)` passes that input to the configured
   renderer.
3. `dashboard.BuildSimpleDashboardRenderData(...)` decomposes the selected-tick
   `Runtime` and `Topology` projections into dashboard-local active-work,
   queue, activity, dispatch-history, and session-summary view structs before
   rendering.

`EngineState` and `Now` are the remaining narrow boundary parameters.
`WorldView` was the only broad aggregate transport on this seam.

## Boundary Inventory

| Boundary field | Current owner | Classification | Why |
| --- | --- | --- | --- |
| `SimpleDashboardRenderInput.EngineState` | `pkg/service` | `canonical_passthrough` | The formatter still renders header state, runtime status, uptime, tick, and uses `es.Topology` as the nil fallback. |
| `SimpleDashboardRenderInput.Now` | `pkg/service` | `canonical_passthrough` | Active-row elapsed time and session start time both depend on the explicit render clock. |
| `SimpleDashboardRenderInput.RenderData` | `pkg/service` | `render_dto_field` | The service now transports only the formatter-owned view data needed by `FormatSimpleDashboardWithRenderData(...)`. |
| `BuildSimpleDashboardRenderData(... runtime interfaces.FactoryWorldRuntimeView, topology interfaces.FactoryWorldTopologyView ...)` | `pkg/cli/dashboard` | `render_dto_field` | The shared dashboard builder now consumes only the runtime and topology projections the formatter actually uses instead of accepting the broad aggregate. |

## Formatter-Required Fields

| Source field | Helper path | Classification | Rendered use |
| --- | --- | --- | --- |
| `FactoryWorldRuntimeView.InFlightDispatchCount` | `dashboardActiveViewFromWorldView(...)` | `render_dto_field` | Active workstation count, including the fallback path where the count is higher than the enumerated active rows. |
| `FactoryWorldRuntimeView.ActiveExecutionsByDispatchID` | `dashboardActiveViewFromWorldView(...)` | `render_dto_field` | Active rows keyed by dispatch for workstation, start time, work type, and work label rendering. |
| `FactoryWorldActiveExecution.TransitionID` | `dashboardActiveViewFromWorldView(...)` | `render_dto_field` | Active-row sort key and workstation fallback label. |
| `FactoryWorldActiveExecution.WorkstationName` | `dashboardActiveViewFromWorldView(...)` | `render_dto_field` | Primary active-row workstation label. |
| `FactoryWorldActiveExecution.StartedAt` | `dashboardActiveViewFromWorldView(...)` | `render_dto_field` | Active-row start time and elapsed duration. |
| `FactoryWorldActiveExecution.WorkTypeIDs` | `activeWorkTypesFromWorldExecution(...)` | `render_dto_field` | Existing active-row work-type labels. |
| `FactoryWorldActiveExecution.WorkItems` | `activeWorkTypesFromWorldExecution(...)`, `activeWorkLabelsFromWorldItems(...)` | `render_dto_field` | Active-row work-type backfill and work labels. |
| `FactoryWorldWorkItemRef.WorkTypeID` | `activeWorkTypesFromWorldExecution(...)` | `render_dto_field` | Backfills work-type labels when the execution-level slice is incomplete. |
| `FactoryWorldWorkItemRef.DisplayName` | `worldWorkItemLabel(...)` | `render_dto_field` | Preferred customer-facing work label everywhere the formatter renders work items. |
| `FactoryWorldWorkItemRef.WorkID` | `worldWorkItemLabel(...)` | `render_dto_field` | Fallback work label when `DisplayName` is empty. |
| `FactoryWorldRuntimeView.PlaceTokenCounts` | `dashboardQueueCountViewsFromWorldView(...)` | `render_dto_field` | Queue rows, token counts, and sorted place enumeration. |
| `FactoryWorldRuntimeView.CurrentWorkItemsByPlaceID` | `workItemsForQueuePlace(...)` | `render_dto_field` | Preferred queue work labels for the live current-work path. |
| `FactoryWorldRuntimeView.PlaceOccupancyWorkItemsByPlaceID` | `workItemsForQueuePlace(...)`, `worldViewWorkItemsForPlaceCategory(...)` | `render_dto_field` | Queue fallback labels plus completed/failed session summaries grouped by place category. |
| `FactoryWorldRuntimeView.WorkstationActivityByNodeID` | `dashboardWorkstationActivityViewsFromWorldView(...)` | `render_dto_field` | Workstation activity rows keyed by node id. |
| `FactoryWorldActivity.ActiveDispatchIDs` | `dashboardWorkstationActivityViewsFromWorldView(...)` | `render_dto_field` | Dispatch ids rendered per active workstation. |
| `FactoryWorldActivity.ActiveWorkItems` | `dashboardWorkstationActivityViewsFromWorldView(...)` | `render_dto_field` | Active work labels rendered per workstation. |
| `FactoryWorldActivity.TraceIDs` | `dashboardWorkstationActivityViewsFromWorldView(...)` | `render_dto_field` | Trace ids rendered per workstation. |
| `FactoryWorldTopologyView.WorkstationNodesByID` | `dashboardWorkstationActivityViewsFromWorldView(...)`, `worldViewPlaceCategories(...)` | `render_dto_field` | Node-id to workstation-name lookup plus place-category lookup for completed/failed session summaries. |
| `FactoryWorldWorkstationNode.WorkstationName` | `dashboardWorkstationActivityViewsFromWorldView(...)` | `render_dto_field` | Human-readable workstation label for activity rows. |
| `FactoryWorldWorkstationNode.InputPlaces` | `worldViewPlaceCategories(...)` | `render_dto_field` | Source of place-category metadata. |
| `FactoryWorldWorkstationNode.OutputPlaces` | `worldViewPlaceCategories(...)` | `render_dto_field` | Source of place-category metadata. |
| `FactoryWorldPlaceRef.PlaceID` | `worldViewPlaceCategories(...)` | `render_dto_field` | Maps occupancy entries back to a place category. |
| `FactoryWorldPlaceRef.StateCategory` | `worldViewPlaceCategories(...)` | `render_dto_field` | Distinguishes `TERMINAL` and `FAILED` occupancy buckets. |
| `FactoryWorldSessionRuntime.HasData` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Gates whether the session metrics section renders at all. |
| `FactoryWorldSessionRuntime.DispatchedCount` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Session metrics summary row. |
| `FactoryWorldSessionRuntime.CompletedCount` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Session metrics summary row. |
| `FactoryWorldSessionRuntime.FailedCount` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Session metrics summary row. |
| `FactoryWorldSessionRuntime.DispatchedByWorkType` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Work-type breakdown for dispatched count. |
| `FactoryWorldSessionRuntime.CompletedByWorkType` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Work-type breakdown for completed count. |
| `FactoryWorldSessionRuntime.FailedByWorkType` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Work-type breakdown for failed count. |
| `FactoryWorldSessionRuntime.ProviderSessions` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Provider-session rows rendered in the session metrics section. |
| `FactoryWorldProviderSessionRecord.DispatchID` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Provider-session correlation label. |
| `FactoryWorldProviderSessionRecord.TransitionID` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Provider-session fallback workstation label. |
| `FactoryWorldProviderSessionRecord.WorkstationName` | `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Provider-session primary workstation label. |
| `FactoryWorldProviderSessionRecord.ConsumedInputs` | `worldProviderSessionWorkItems(...)` | `render_dto_field` | Preferred work-item labels for provider-session rows. |
| `FactoryWorldProviderSessionRecord.WorkItemIDs` | `worldProviderSessionWorkItems(...)` | `render_dto_field` | Fallback work labels when consumed inputs do not carry work items. |
| `FactoryWorldProviderSessionRecord.ProviderSession` | `cloneProviderSessionMetadata(...)`, `formatProviderSession(...)` | `render_dto_field` | Provider, session kind, and session id rendering. |
| `ProviderSessionMetadata.ID` | `cloneProviderSessionMetadata(...)`, `formatProviderSession(...)` | `render_dto_field` | Required provider-session render token. |
| `ProviderSessionMetadata.Provider` | `formatProviderSession(...)` | `render_dto_field` | Provider-session label. |
| `ProviderSessionMetadata.Kind` | `formatProviderSession(...)` | `render_dto_field` | Provider-session label. |
| `FactoryWorldSessionRuntime.DispatchHistory` | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardSessionViewFromWorldView(...)` | `render_dto_field` | Completed workstation rows plus completed/failed work fallback logic and failed-work detail lookup. |
| `FactoryWorldDispatchCompletion.DispatchID` | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | `render_dto_field` | Dispatch-history correlation label and failed-work detail row. |
| `FactoryWorldDispatchCompletion.TransitionID` | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | `render_dto_field` | Dispatch-history fallback workstation label and system-time compatibility mapping. |
| `FactoryWorldDispatchCompletion.Workstation.Name` | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | `render_dto_field` | Dispatch-history and failed-work workstation label. |
| `FactoryWorldDispatchCompletion.StartedAt` | `dashboardDispatchHistoryFromWorldView(...)` | `render_dto_field` | Dispatch-history start time. |
| `FactoryWorldDispatchCompletion.CompletedAt` | `dashboardDispatchHistoryFromWorldView(...)` | `render_dto_field` | Dispatch-history end time. |
| `FactoryWorldDispatchCompletion.DurationMillis` | `dashboardDispatchHistoryFromWorldView(...)` | `render_dto_field` | Dispatch-history duration. |
| `FactoryWorldDispatchCompletion.Result.Outcome` | `dashboardDispatchHistoryFromWorldView(...)`, fallback helpers | `render_dto_field` | Dispatch status plus completed/failed fallback classification. |
| `FactoryWorldDispatchCompletion.Result.FailureReason` | `worldDispatchReason(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | `render_dto_field` | Rendered failure reason text. |
| `FactoryWorldDispatchCompletion.Result.FailureMessage` | `worldDispatchReason(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | `render_dto_field` | Rendered failure detail text. |
| `FactoryWorldDispatchCompletion.Result.Feedback` | `worldDispatchReason(...)` | `render_dto_field` | Fallback dispatch reason when explicit failure reason is absent. |
| `FactoryWorldDispatchCompletion.InputWorkItems` | `worldDispatchInputLabels(...)`, failed-work fallback | `render_dto_field` | Preferred dispatch input labels and failed-work fallback coverage. |
| `FactoryWorldDispatchCompletion.OutputWorkItems` | `worldDispatchOutputLabels(...)`, completed/failed fallback | `render_dto_field` | Preferred dispatch output labels and terminal fallback coverage. |
| `FactoryWorldDispatchCompletion.ConsumedInputs` | `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)` | `render_dto_field` | Input/output label fallback when explicit world work items are absent. |
| `FactoryWorldDispatchCompletion.WorkItemIDs` | `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)`, `worldFailedWorkIDsForDispatch(...)` | `render_dto_field` | Last-resort input/output labels and failed-work lookup keys. |
| `FactoryWorldDispatchCompletion.TerminalWork` | completed/failed fallback helpers | `render_dto_field` | Preferred terminal work-item identity for completed and failed summaries. |

## Aggregate Siblings Not Needed By The Simple Dashboard

These fields still belong to `FactoryWorldView` for other consumers, but they
are `dead_aggregate_only` for the simple-dashboard seam because this formatter
never reads them directly or through local helpers:

| Source field | Classification | Why it is out of scope for the dedicated render DTO |
| --- | --- | --- |
| `FactoryWorldTopologyView.SubmitWorkTypes` | `dead_aggregate_only` | Submit-work affordances are not rendered by the simple terminal dashboard. |
| `FactoryWorldTopologyView.WorkstationNodeIDs` | `dead_aggregate_only` | Activity rows sort by the runtime activity map keys instead. |
| `FactoryWorldTopologyView.Edges` | `dead_aggregate_only` | Graph-edge rendering belongs to the richer UI/dashboard path, not the terminal formatter. |
| `FactoryWorldWorkstationNode.{NodeID,TransitionID,WorkerType,WorkstationKind,InputPlaceIDs,OutputPlaceIDs,InputWorkTypeIDs,OutputWorkTypeIDs}` | `dead_aggregate_only` | The formatter only needs `WorkstationName` plus place-category metadata from `InputPlaces` and `OutputPlaces`. |
| `FactoryWorldPlaceRef.{TypeID,StateValue,Kind}` | `dead_aggregate_only` | Queue place labels come from `state.SplitPlaceID(placeID)` instead of world-view place metadata. |
| `FactoryWorldRuntimeView.ActiveDispatchIDs` | `dead_aggregate_only` | Active rows are built from `ActiveExecutionsByDispatchID`. |
| `FactoryWorldRuntimeView.ActiveWorkstationNodeIDs` | `dead_aggregate_only` | Workstation activity is built from `WorkstationActivityByNodeID`. |
| `FactoryWorldRuntimeView.InferenceAttemptsByDispatchID` | `dead_aggregate_only` | The simple dashboard renders provider-session summaries, not inference-attempt detail. |
| `FactoryWorldRuntimeView.ActiveThrottlePauses` | `dead_aggregate_only` | Throttle-pause state is not shown in the simple terminal dashboard. |
| `FactoryWorldActiveExecution.{DispatchID,WorkstationNodeID,CurrentChainingTraceID,PreviousChainingTraceIDs,TraceIDs,ConsumedTokens,OutputMutations}` | `dead_aggregate_only` | The formatter does not render active dispatch ids, node ids, trace lineage, token views, or mutation views in the active section. |
| `FactoryWorldWorkItemRef.{CurrentChainingTraceID,PreviousChainingTraceIDs,TraceID}` | `dead_aggregate_only` | The formatter renders only work labels and work-type labels from work-item refs. |
| `FactoryWorldProviderSessionRecord.{Outcome,Diagnostics,CurrentChainingTraceID,PreviousChainingTraceIDs,TraceIDs,FailureReason,FailureMessage}` | `dead_aggregate_only` | Provider-session rows show work labels plus safe provider session metadata only. |
| `FactoryWorldDispatchCompletion.{StartedTick,CompletedTick,CurrentChainingTraceID,PreviousChainingTraceIDs,TraceIDs,ProviderSession,Diagnostics}` | `dead_aggregate_only` | The formatter renders customer-visible history and failure text without dispatch-tick, trace, or diagnostics payload detail. |

## DTO Shape Implications

- Keep `EngineState` and `Now` as explicit boundary parameters or move them
  into a narrow render-input wrapper without reconstructing a new aggregate.
- Replace the `WorldView` transport with a dedicated simple-dashboard render
  DTO that carries:
  - active execution rows
  - queue token counts plus queue work labels
  - workstation activity rows plus workstation-name lookup
  - dispatch history rows
  - session counters, provider-session rows, and failed/completed work summary
    inputs
  - place-category lookup data needed to derive completed and failed work from
    occupancy
- Keep any remaining grouping, sorting, system-time compatibility mapping, and
  text rendering local to `pkg/cli/dashboard`.
- Keep any selected-tick `FactoryWorldView` reconstruction or ownership behind
  the service boundary; the dashboard helper should accept only the narrower
  projection pieces it renders.

## Verification

- `go test ./pkg/cli/dashboard ./pkg/service -count=1`
