# Simple Dashboard World-View Field Inventory

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-simple-dashboard-world-view-localization`)
- Owner: Codex branch `ralph/agent-factory-simple-dashboard-world-view-localization`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `pkg/service`, `pkg/factory/projections`, `pkg/cli/dashboard`, `pkg/interfaces`
- Canonical process document to update before completion: `docs/processes/agent-factory-development.md`

## Scope

This inventory is limited to the simple-dashboard render path:

1. `pkg/service/factory.go` `buildSimpleDashboardRenderInput(...)`
2. `pkg/service/factory.go` `factoryWorldView(...)`
3. `pkg/factory/projections/world_view.go` `BuildFactoryWorldView(...)`
4. `pkg/cli/dashboard/dashboard.go` `FormatSimpleDashboardWithWorldView(...)` and its helper mappers

It does not attempt to retire the full `FactoryWorldView` family. It records only the fields the simple dashboard still consumes from the broad aggregate seam.

## Classification Rules

- `canonical_passthrough`: canonical selected-tick data already owned by `FactoryWorldState` or its canonical member types and merely transported through the simple dashboard seam today
- `dashboard_boundary`: display-only grouping or fallback logic that should remain local to the simple dashboard boundary rather than expand shared contracts
- `dead_aggregate_only`: fields present on `FactoryWorldView` or its nested shared shells that the simple dashboard path does not read

## Seam Summary

`buildSimpleDashboardRenderInput(...)` captures `EngineState`, `Topology`, `WorldView`, and `Now`, but the simple dashboard only reads a small subset of `WorldView`. `FormatSimpleDashboardWithWorldView(...)` immediately decomposes that aggregate into five local view slices:

1. active rows
2. queue counts
3. workstation activity
4. dispatch history
5. session metrics

That means the remaining broad seam is transport, not behavior. The formatter does not need an open-ended aggregate once those five slices or their minimal source fields are made explicit.

## Exact Field Inventory

| Field path | Classification | Used by | Customer-visible purpose | Notes |
| --- | --- | --- | --- | --- |
| `FactoryWorldView.Runtime.InFlightDispatchCount` | canonical_passthrough | `dashboardActiveViewFromWorldView(...)` | active rows | used as the displayed active count, with a local fallback to `len(ActiveExecutionsByDispatchID)` when the count is smaller |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].DispatchID` | canonical_passthrough | `dashboardActiveViewFromWorldView(...)` | active rows | dispatch identity for sorting and rendering |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].TransitionID` | canonical_passthrough | `dashboardActiveViewFromWorldView(...)` | active rows | workstation fallback label and sort key |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].WorkstationName` | canonical_passthrough | `dashboardActiveViewFromWorldView(...)` | active rows | preferred workstation label |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].StartedAt` | canonical_passthrough | `dashboardActiveViewFromWorldView(...)` | active rows | start time and elapsed duration |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].WorkTypeIDs` | canonical_passthrough | `activeWorkTypesFromWorldExecution(...)` | active rows | direct work-type list |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].WorkTypeID` | canonical_passthrough | `activeWorkTypesFromWorldExecution(...)` | active rows | fills missing work types not already present on `WorkTypeIDs` |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].DisplayName` | canonical_passthrough | `activeWorkLabelsFromWorldItems(...)` | active rows | preferred work label |
| `FactoryWorldView.Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].WorkID` | canonical_passthrough | `activeWorkLabelsFromWorldItems(...)` | active rows | fallback work label |
| `FactoryWorldView.Runtime.PlaceTokenCounts[*]` | canonical_passthrough | `dashboardQueueCountViewsFromWorldView(...)` | queue counts | token count per place; zero-count places are dropped locally |
| `FactoryWorldView.Runtime.CurrentWorkItemsByPlaceID[*][*].DisplayName` | canonical_passthrough | `workItemsForQueuePlace(...)`, `worldWorkItemLabels(...)` | queue counts | preferred queue work label |
| `FactoryWorldView.Runtime.CurrentWorkItemsByPlaceID[*][*].WorkID` | canonical_passthrough | `workItemsForQueuePlace(...)`, `worldWorkItemLabels(...)` | queue counts | fallback queue work label |
| `FactoryWorldView.Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].DisplayName` | canonical_passthrough | `workItemsForQueuePlace(...)`, `worldWorkItemLabels(...)` | queue counts, session metrics | fallback queue/session work label source when current-work map is empty |
| `FactoryWorldView.Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].WorkID` | canonical_passthrough | `workItemsForQueuePlace(...)`, `worldWorkItemLabels(...)` | queue counts, session metrics | fallback queue/session work label source |
| `FactoryWorldView.Runtime.WorkstationActivityByNodeID[*].ActiveDispatchIDs` | canonical_passthrough | `dashboardWorkstationActivityViewsFromWorldView(...)` | workstation activity | displayed as sorted unique dispatch IDs |
| `FactoryWorldView.Runtime.WorkstationActivityByNodeID[*].ActiveWorkItems[*].DisplayName` | canonical_passthrough | `dashboardWorkstationActivityViewsFromWorldView(...)` | workstation activity | preferred active work label |
| `FactoryWorldView.Runtime.WorkstationActivityByNodeID[*].ActiveWorkItems[*].WorkID` | canonical_passthrough | `dashboardWorkstationActivityViewsFromWorldView(...)` | workstation activity | fallback active work label |
| `FactoryWorldView.Runtime.WorkstationActivityByNodeID[*].TraceIDs` | canonical_passthrough | `dashboardWorkstationActivityViewsFromWorldView(...)` | workstation activity | displayed trace list |
| `FactoryWorldView.Topology.WorkstationNodesByID[*].WorkstationName` | canonical_passthrough | `dashboardWorkstationActivityViewsFromWorldView(...)` | workstation activity | node-to-name lookup for activity rows |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].DispatchID` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | dispatch history, failed-work details | dispatch identity |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].TransitionID` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | dispatch history, failed-work details | mapped locally through `dashboardWorldViewTransitionID(...)` for `__system_time` compatibility |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].Workstation.Name` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | dispatch history, failed-work details | preferred workstation label |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].StartedAt` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)` | dispatch history | completed row start time |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].CompletedAt` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)` | dispatch history | completed row end time |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].DurationMillis` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)` | dispatch history | completed row duration |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].Result.Outcome` | canonical_passthrough | `dashboardDispatchHistoryFromWorldView(...)`, completed/failed fallback helpers | dispatch history, completed history, failed work | success/rejected/failed status and fallback filtering |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].Result.FailureReason` | canonical_passthrough | `worldDispatchReason(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | dispatch history, failed-work details | customer-visible reason text |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].Result.Feedback` | canonical_passthrough | `worldDispatchReason(...)` | dispatch history | fallback reason text when `FailureReason` is empty |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].Result.FailureMessage` | canonical_passthrough | `worldDispatchReason(...)`, `dashboardFailedWorkDetailsFromWorldView(...)` | dispatch history, failed-work details | customer-visible failure message |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].InputWorkItems[*].ID` | canonical_passthrough | `worldDispatchInputLabels(...)`, failed-work fallback helpers | dispatch history, failed work | fallback input labels and failed-work identity |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].InputWorkItems[*].DisplayName` | canonical_passthrough | `worldDispatchInputLabels(...)`, failed-work fallback helpers | dispatch history, failed work | preferred input label |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].InputWorkItems[*].WorkTypeID` | canonical_passthrough | failed-work fallback helpers via `workRefForDashboardItem(...)` | failed-work details | preserved in fallback work refs |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].OutputWorkItems[*].ID` | canonical_passthrough | `worldDispatchOutputLabels(...)`, completed/failed fallback helpers | dispatch history, completed history, failed work | fallback output labels and work identity |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].OutputWorkItems[*].DisplayName` | canonical_passthrough | `worldDispatchOutputLabels(...)`, completed/failed fallback helpers | dispatch history, completed history, failed work | preferred output label |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].OutputWorkItems[*].WorkTypeID` | canonical_passthrough | completed/failed fallback helpers via `workRefForDashboardItem(...)` | completed history, failed work | preserved in fallback work refs |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].ConsumedInputs[*].WorkItem.ID` | canonical_passthrough | `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)`, provider-session fallback | dispatch history, provider sessions | fallback work identity when work-item structs are all the formatter has |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].ConsumedInputs[*].WorkItem.DisplayName` | canonical_passthrough | `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)`, provider-session fallback | dispatch history, provider sessions | preferred fallback label |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].ConsumedInputs[*].WorkItem.WorkTypeID` | canonical_passthrough | provider-session fallback via `workRefForDashboardItem(...)` | provider sessions | preserved in fallback work refs |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].WorkItemIDs` | canonical_passthrough | `worldDispatchInputLabels(...)`, `worldDispatchOutputLabels(...)`, `worldFailedWorkIDsForDispatch(...)` | dispatch history, failed-work details | last-resort ID fallback when richer work items are absent |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].TerminalWork.WorkItem.ID` | canonical_passthrough | completed/failed fallback helpers, `worldFailedWorkIDsForDispatch(...)` | completed history, failed work | terminal work identity |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].TerminalWork.WorkItem.DisplayName` | canonical_passthrough | completed/failed fallback helpers | completed history, failed work | preferred terminal work label |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].TerminalWork.WorkItem.WorkTypeID` | canonical_passthrough | completed/failed fallback helpers | completed history, failed work | preserved in fallback work refs |
| `FactoryWorldView.Runtime.Session.DispatchHistory[*].TerminalWork.Status` | canonical_passthrough | `worldViewFallbackCompletedWorkItems(...)` | completed history | excludes failed terminal work from completed fallback labels |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].DispatchID` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | provider sessions | provider-session row identity |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].TransitionID` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | provider sessions | workstation fallback label |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].WorkstationName` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | provider sessions | preferred workstation label |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].ProviderSession` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)`, `formatProviderSession(...)` | provider sessions | cloned locally and rendered from safe metadata (`ID`, `Provider`, `Kind`) |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].ConsumedInputs[*].WorkItem.ID` | canonical_passthrough | `worldProviderSessionWorkItems(...)` | provider sessions | preferred work-item identity for session rows |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].ConsumedInputs[*].WorkItem.DisplayName` | canonical_passthrough | `worldProviderSessionWorkItems(...)` | provider sessions | preferred work-item label for session rows |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].ConsumedInputs[*].WorkItem.WorkTypeID` | canonical_passthrough | `worldProviderSessionWorkItems(...)` | provider sessions | preserved in rendered work refs |
| `FactoryWorldView.Runtime.Session.ProviderSessions[*].WorkItemIDs` | canonical_passthrough | `worldProviderSessionWorkItems(...)` | provider sessions | fallback work identity when consumed input work items are absent |
| `FactoryWorldView.Topology.WorkstationNodesByID[*].InputPlaces[*].PlaceID` | canonical_passthrough | `worldViewPlaceCategories(...)` | session metrics | place-to-category map for terminal/failed work discovery |
| `FactoryWorldView.Topology.WorkstationNodesByID[*].InputPlaces[*].StateCategory` | canonical_passthrough | `worldViewPlaceCategories(...)` | session metrics | classifies places as `TERMINAL` or `FAILED` |
| `FactoryWorldView.Topology.WorkstationNodesByID[*].OutputPlaces[*].PlaceID` | canonical_passthrough | `worldViewPlaceCategories(...)` | session metrics | place-to-category map for terminal/failed work discovery |
| `FactoryWorldView.Topology.WorkstationNodesByID[*].OutputPlaces[*].StateCategory` | canonical_passthrough | `worldViewPlaceCategories(...)` | session metrics | classifies places as `TERMINAL` or `FAILED` |
| `FactoryWorldView.Runtime.Session.HasData` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | gates the entire session section |
| `FactoryWorldView.Runtime.Session.DispatchedCount` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | total dispatched count |
| `FactoryWorldView.Runtime.Session.CompletedCount` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | total completed count |
| `FactoryWorldView.Runtime.Session.FailedCount` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | total failed count |
| `FactoryWorldView.Runtime.Session.DispatchedByWorkType` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | per-work-type dispatched counts |
| `FactoryWorldView.Runtime.Session.CompletedByWorkType` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | per-work-type completed counts |
| `FactoryWorldView.Runtime.Session.FailedByWorkType` | canonical_passthrough | `dashboardSessionViewFromWorldView(...)` | session metrics | per-work-type failed counts |

## Dashboard-Boundary Shaping Kept Local

These behaviors are required for the supported simple dashboard, but they should stay local to the service or CLI boundary instead of broadening `FactoryWorldView`:

| Local shaping | Classification | Why it stays local |
| --- | --- | --- |
| `dashboardWorldViewTransitionID(...)` remaps `__system_time` to the dashboard compatibility label | dashboard_boundary | presentation-only compatibility |
| `dashboardWorldViewWorkstationName(...)` and `displayDispatchWorkstationName(...)` choose workstation display fallbacks | dashboard_boundary | display-only workstation naming |
| `activeWorkTypesFromWorldExecution(...)` merges explicit `WorkTypeIDs` with `WorkItems[*].WorkTypeID` | dashboard_boundary | formatter-only convenience |
| `worldWorkItemLabel(...)` prefers `DisplayName` and falls back to `WorkID` | dashboard_boundary | display-only label policy |
| `workItemsForQueuePlace(...)` prefers `CurrentWorkItemsByPlaceID` and falls back to `PlaceOccupancyWorkItemsByPlaceID` | dashboard_boundary | queue-label convenience |
| `worldViewPlaceCategories(...)` derives a `placeID -> stateCategory` lookup from topology | dashboard_boundary | local lookup table for session summaries |
| `worldViewWorkItemsForPlaceCategory(...)` collects unique work items for `TERMINAL` and `FAILED` categories | dashboard_boundary | display-only aggregation |
| `worldViewFallbackCompletedWorkItems(...)` and `worldViewFallbackFailedWorkItems(...)` recover session labels from dispatch history when place occupancy is absent | dashboard_boundary | compatibility fallback, not canonical shared state |
| `dashboardFailedWorkDetailsFromWorldView(...)` joins failed work items back to failed dispatch completions | dashboard_boundary | display-only detail enrichment |

## Dead Aggregate-Only Surface For This Path

The simple dashboard does not read these shared aggregate fields today:

- `FactoryWorldTopologyView.SubmitWorkTypes`
- `FactoryWorldTopologyView.WorkstationNodeIDs`
- `FactoryWorldTopologyView.Edges`
- `FactoryWorldWorkstationNode.NodeID`
- `FactoryWorldWorkstationNode.TransitionID`
- `FactoryWorldWorkstationNode.WorkerType`
- `FactoryWorldWorkstationNode.WorkstationKind`
- `FactoryWorldWorkstationNode.InputPlaceIDs`
- `FactoryWorldWorkstationNode.OutputPlaceIDs`
- `FactoryWorldWorkstationNode.InputWorkTypeIDs`
- `FactoryWorldWorkstationNode.OutputWorkTypeIDs`
- `FactoryWorldRuntimeView.ActiveDispatchIDs`
- `FactoryWorldRuntimeView.ActiveWorkstationNodeIDs`
- `FactoryWorldRuntimeView.InferenceAttemptsByDispatchID`
- `FactoryWorldRuntimeView.ActiveThrottlePauses`
- `FactoryWorldActiveExecution.WorkstationNodeID`
- `FactoryWorldActiveExecution.CurrentChainingTraceID`
- `FactoryWorldActiveExecution.PreviousChainingTraceIDs`
- `FactoryWorldActiveExecution.TraceIDs`
- `FactoryWorldActiveExecution.ConsumedTokens`
- `FactoryWorldActiveExecution.OutputMutations`

Those fields may still be required by other API, CLI, or UI consumers, but they are dead for the simple dashboard seam and should not be carried into the localized dashboard-owned input.

## Minimal Replacement Shape Implied By This Inventory

The next story does not need another aggregate. The simple dashboard needs only:

1. active execution rows and active count
2. queue-place token counts plus resolved work labels
3. workstation activity rows with workstation names
4. canonical dispatch-completion history for completed rows and failed-work joins
5. provider-session rows and session counters
6. topology-derived place-category lookup or pre-resolved completed/failed work lists

That is the narrow seam to localize in `US-002` and `US-003`.

## Evidence

- `libraries/agent-factory/pkg/service/factory.go`
  - `buildSimpleDashboardRenderInput(...)`
  - `factoryWorldView(...)`
- `libraries/agent-factory/pkg/factory/projections/world_view.go`
  - `BuildFactoryWorldView(...)`
- `libraries/agent-factory/pkg/cli/dashboard/dashboard.go`
  - `FormatSimpleDashboardWithWorldView(...)`
  - `dashboardActiveViewFromWorldView(...)`
  - `dashboardQueueCountViewsFromWorldView(...)`
  - `dashboardWorkstationActivityViewsFromWorldView(...)`
  - `dashboardDispatchHistoryFromWorldView(...)`
  - `dashboardSessionViewFromWorldView(...)`
