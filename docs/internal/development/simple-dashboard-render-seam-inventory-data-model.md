# Simple Dashboard Render Seam Inventory

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-simple-dashboard-render-seam-closeout`)
- Owner: Codex branch `ralph/agent-factory-simple-dashboard-render-seam-closeout`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `pkg/service`, `pkg/cli/run`, `pkg/cli/dashboard`, `pkg/interfaces`, `pkg/factory/projections`
- Canonical process document to update before completion: `docs/processes/agent-factory-development.md`

## Render Path

1. `pkg/service/factory.go:buildSimpleDashboardRenderInput(...)` builds `SimpleDashboardRenderInput`.
2. `pkg/cli/run/run.go:renderSimpleDashboard(...)` forwards that input to the CLI formatter.
3. `pkg/cli/dashboard/dashboard.go:FormatSimpleDashboardWithRenderData(...)` reduces the dedicated render DTO into five dashboard-local view families:
   active executions, queue counts, workstation activity, dispatch history, and session summaries.

This keeps `FormatSimpleDashboardWithRenderData(...)` and its helper family as the source of truth for DTO sizing on this cleanup branch, while `dashboardrender.SimpleDashboardRenderDataFromWorldState(...)` owns the service-to-CLI boundary mapping.

## Classification Rules

| Classification | Meaning |
| --- | --- |
| `render_dto_field` | Data the replacement simple-dashboard DTO must carry across the service-to-CLI seam. |
| `service_owned_context` | Context still needed at the seam, but not part of the world-view-derived replacement DTO. |
| `dead_aggregate_only` | `FactoryWorldView` or `FactoryWorldRuntimeView` fields that cross this seam today but are not read by the simple dashboard formatter. |

## Service-Owned Context

| Input | Classification | Current owner | Why it stays outside the replacement world-view DTO |
| --- | --- | --- | --- |
| `SimpleDashboardRenderInput.EngineState` | `service_owned_context` | `pkg/service` | The formatter still uses the live engine snapshot for the snapshot-owned dashboard sections. |
| `SimpleDashboardRenderInput.Now` | `service_owned_context` | `pkg/service` | Rendering timestamps remain service-owned clock context, not selected-tick projection data. |

## Render DTO Inventory

| Dashboard concern | Exact formatter inputs | Classification | Evidence |
| --- | --- | --- | --- |
| Active execution rows | `Runtime.InFlightDispatchCount`; `Runtime.ActiveExecutionsByDispatchID[*].{DispatchID, TransitionID, WorkstationName, StartedAt, WorkItems, ConsumedInputs}` | `render_dto_field` | `dashboardActiveViewFromRenderData(...)`, `activeWorkTypesFromWorldExecution(...)`, `activeWorkLabelsFromWorldItems(...)` |
| Queue count rows | `Runtime.PlaceTokenCounts[placeID]`; `placeID` split into work-type and state; queue label fallback from `Runtime.CurrentWorkItemsByPlaceID[placeID]` to `Runtime.PlaceOccupancyWorkItemsByPlaceID[placeID]` | `render_dto_field` | `dashboardQueueCountViewsFromRenderData(...)`, `workItemsForQueuePlace(...)` |
| Workstation activity rows | `Runtime.WorkstationActivityByNodeID[nodeID].{ActiveDispatchIDs, ActiveWorkItems, TraceIDs}` plus workstation-name lookup data | `render_dto_field` | `dashboardWorkstationActivityViewsFromRenderData(...)` |
| Dispatch history rows | `Runtime.Session.DispatchHistory[*].{DispatchID, TransitionID, Workstation.Name, Result.Outcome, StartedAt, CompletedAt, DurationMillis, ConsumedInputs, InputWorkItems, OutputWorkItems, TerminalWork}` | `render_dto_field` | `dashboardDispatchHistoryFromRenderData(...)`, `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)`, `worldDispatchReason(...)` |
| Session counters | `Runtime.Session.{HasData, DispatchedCount, CompletedCount, FailedCount, DispatchedByWorkType, CompletedByWorkType, FailedByWorkType}` | `render_dto_field` | `dashboardSessionViewFromRenderData(...)` |
| Provider sessions | `Runtime.Session.ProviderSessions[*].{DispatchID, TransitionID, WorkstationName, ProviderSession, ConsumedInputs, WorkItemIDs}` | `render_dto_field` | `dashboardSessionViewFromRenderData(...)`, `worldProviderSessionWorkItems(...)` |
| Completed-work summary | Place-category path plus `Runtime.PlaceOccupancyWorkItemsByPlaceID`; fallback path: `Runtime.Session.DispatchHistory[*].{Result.Outcome, TerminalWork, OutputWorkItems}` | `render_dto_field` | `worldViewPlaceCategories(...)`, `worldViewWorkItemsForPlaceCategory(...)`, `worldViewFallbackCompletedWorkItems(...)` |
| Failed-work summary and details | Place-category path plus `Runtime.PlaceOccupancyWorkItemsByPlaceID`; fallback path: `Runtime.Session.DispatchHistory[*].{DispatchID, TransitionID, Workstation.Name, Result.Outcome, Result.FailureReason, Result.FailureMessage, TerminalWork, InputWorkItems, OutputWorkItems}` | `render_dto_field` | `dashboardSessionViewFromRenderData(...)`, `worldViewFallbackFailedWorkItems(...)`, `dashboardFailedWorkDetailsFromRenderData(...)` |

## Aggregate Fields That Are Dead For This Seam

These `FactoryWorldView` members belonged to the retired seam and are not required by the dedicated render DTO.

| Input | Classification | Why it is dead for the simple dashboard seam |
| --- | --- | --- |
| `Runtime.ActiveDispatchIDs` | `dead_aggregate_only` | The formatter derives active rows from `ActiveExecutionsByDispatchID` and `InFlightDispatchCount`. |
| `Runtime.ActiveWorkstationNodeIDs` | `dead_aggregate_only` | Workstation rows iterate `WorkstationActivityByNodeID` directly. |
| `Runtime.InferenceAttemptsByDispatchID` | `dead_aggregate_only` | No simple-dashboard formatter helper reads inference-attempt history. |
| `Runtime.ActiveThrottlePauses` | `dead_aggregate_only` | No formatter section renders throttle pause information. |
| `Topology.Edges`, `Topology.WorkstationNodes`, and other graph-only topology collections not used for workstation-name or place-category lookup | `dead_aggregate_only` | The formatter only needs `WorkstationNodesByID` for workstation names and place-category lookup. |

## DTO Implications For Follow-On Stories

- The replacement render DTO needs one dashboard-owned shape for the five derived view families, not another aggregate alias for `FactoryWorldView`.
- Queue rows must preserve the current label fallback order: `CurrentWorkItemsByPlaceID` first, then `PlaceOccupancyWorkItemsByPlaceID`.
- Session summaries must preserve both terminal/failed place-category lookup and dispatch-history fallback, otherwise empty selected-tick place occupancy would silently drop supported rows.
- `EngineState` and `Now` remain explicit seam context until the snapshot-owned formatter sections are reduced separately.
