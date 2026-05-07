# Simple Dashboard World-View Seam Inventory

This document inventories the `pkg/service/factory.go -> pkg/factory/projections/world_view.go -> pkg/cli/dashboard/dashboard.go` path used by the simple dashboard render flow. It is the durable field-by-field input for retiring the broad `interfaces.FactoryWorldView` aggregate from the service-to-CLI seam.

## Change

- PRD, design, or issue: `prd.json` (`US-001`, branch `ralph/agent-factory-dashboard-world-view-aggregate-retirement`)
- Owner: Codex branch `ralph/agent-factory-dashboard-world-view-aggregate-retirement`
- Reviewers: Agent Factory maintainers
- Packages or subsystems: `libraries/agent-factory/pkg/service`, `libraries/agent-factory/pkg/factory/projections`, `libraries/agent-factory/pkg/cli/dashboard`, `libraries/agent-factory/pkg/interfaces`
- Canonical process document to update before completion: `docs/processes/agent-factory-development.md`

## Seam Summary

1. `FactoryService.buildSimpleDashboardRenderInput(...)` reads the live engine snapshot for runtime header data, then reconstructs selected-tick event history through `factoryWorldView(...)`.
2. `FactoryService.factoryWorldView(...)` calls `projections.ReconstructFactoryWorldState(...)` and `projections.BuildFactoryWorldView(...)`.
3. Before the boundary-owned seam landed, `dashboard.FormatSimpleDashboardWithWorldView(...)` immediately decomposed `interfaces.FactoryWorldView` into dashboard-local read models for active rows, queue counts, workstation activity, completed history, provider sessions, failed-work details, and session metrics. The live formatter now consumes `dashboard.SimpleDashboardWorldView` directly.

The inventory below classifies each relevant aggregate field as:

- `canonical_passthrough`: dashboard consumes canonical event-derived data that already has a stable owner outside the aggregate shell.
- `dashboard_boundary`: the field exists to make the CLI boundary ergonomic, but it is not canonical runtime state.
- `dead_aggregate_only`: the simple dashboard seam does not read the field today.

## Top-Level Aggregate Inventory

| Aggregate field | Classification | Simple dashboard usage | Evidence |
| --- | --- | --- | --- |
| `FactoryWorldView.Topology` | `dashboard_boundary` | Used only to look up workstation names and place categories while deriving workstation-activity labels plus completed/failed work summaries. | `dashboardWorkstationActivityViewsFromWorldView(...)`, `dashboardSessionViewFromWorldView(...)`, `worldViewPlaceCategories(...)` |
| `FactoryWorldView.Runtime` | `canonical_passthrough` | All runtime-facing dashboard sections are derived from the event-first runtime payload. | `dashboardActiveViewFromWorldView(...)`, `dashboardQueueCountViewsFromWorldView(...)`, `dashboardDispatchHistoryFromWorldView(...)`, `dashboardSessionViewFromWorldView(...)` |

## `FactoryWorldTopologyView` Inventory

| Field | Classification | Read by simple dashboard? | Downstream use |
| --- | --- | --- | --- |
| `SubmitWorkTypes` | `dead_aggregate_only` | no | Not referenced by the CLI formatter. |
| `WorkstationNodeIDs` | `dead_aggregate_only` | no | The formatter iterates `Runtime.WorkstationActivityByNodeID` keys instead. |
| `WorkstationNodesByID` | `dashboard_boundary` | yes | `dashboardWorkstationActivityViewsFromWorldView(...)` reads `WorkstationName`; `worldViewPlaceCategories(...)` reads `InputPlaces` and `OutputPlaces`. |
| `Edges` | `dead_aggregate_only` | no | Graph-edge data is not used by the simple dashboard formatter. |

## `FactoryWorldRuntimeView` Inventory

| Field | Classification | Read by simple dashboard? | Downstream use |
| --- | --- | --- | --- |
| `InFlightDispatchCount` | `canonical_passthrough` | yes | Active workstation count fallback in `dashboardActiveViewFromWorldView(...)`. |
| `ActiveDispatchIDs` | `dead_aggregate_only` | no | The formatter derives active rows from `ActiveExecutionsByDispatchID`. |
| `ActiveExecutionsByDispatchID` | `canonical_passthrough` | yes | Active workstation rows. |
| `ActiveWorkstationNodeIDs` | `dead_aggregate_only` | no | Not read by the formatter. |
| `InferenceAttemptsByDispatchID` | `dead_aggregate_only` | no | Not read by the formatter. |
| `WorkstationActivityByNodeID` | `canonical_passthrough` | yes | Workstation activity table. |
| `PlaceTokenCounts` | `canonical_passthrough` | yes | Queue-count rows. |
| `CurrentWorkItemsByPlaceID` | `canonical_passthrough` | yes | Primary queue-count work labels. |
| `PlaceOccupancyWorkItemsByPlaceID` | `canonical_passthrough` | yes | Queue fallback labels plus completed/failed work summaries. |
| `ActiveThrottlePauses` | `dead_aggregate_only` | no | Not read by the formatter. |
| `Session` | `canonical_passthrough` | yes | Completed history, provider sessions, failed-work details, and session metrics. |

## Nested Field Inventory Used By The Formatter

### Active Workstations

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.ActiveExecutionsByDispatchID[*].TransitionID` | `canonical_passthrough` | Active workstation row identity and fallback workstation label. |
| `Runtime.ActiveExecutionsByDispatchID[*].WorkstationName` | `canonical_passthrough` | Active workstation row label. |
| `Runtime.ActiveExecutionsByDispatchID[*].StartedAt` | `canonical_passthrough` | Active workstation start time and elapsed duration. |
| `Runtime.ActiveExecutionsByDispatchID[*].WorkTypeIDs` | `canonical_passthrough` | Active workstation work-type labels. |
| `Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].WorkTypeID` | `canonical_passthrough` | Fallback work-type labels when `WorkTypeIDs` is incomplete. |
| `Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].DisplayName` | `canonical_passthrough` | Preferred active work label. |
| `Runtime.ActiveExecutionsByDispatchID[*].WorkItems[*].WorkID` | `canonical_passthrough` | Active work fallback label. |

### Queue Counts

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.PlaceTokenCounts[*]` | `canonical_passthrough` | Queue token count and place iteration. |
| `Runtime.CurrentWorkItemsByPlaceID[*][*].DisplayName` | `canonical_passthrough` | Preferred queue work label. |
| `Runtime.CurrentWorkItemsByPlaceID[*][*].WorkID` | `canonical_passthrough` | Queue work fallback label. |
| `Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].DisplayName` | `canonical_passthrough` | Queue fallback label when current-work slice is empty. |
| `Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].WorkID` | `canonical_passthrough` | Queue fallback label when current-work slice is empty. |

### Workstation Activity

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.WorkstationActivityByNodeID[*].ActiveDispatchIDs` | `canonical_passthrough` | Dispatch list per workstation. |
| `Runtime.WorkstationActivityByNodeID[*].ActiveWorkItems[*].DisplayName` | `canonical_passthrough` | Preferred active-work label. |
| `Runtime.WorkstationActivityByNodeID[*].ActiveWorkItems[*].WorkID` | `canonical_passthrough` | Active-work fallback label. |
| `Runtime.WorkstationActivityByNodeID[*].TraceIDs` | `canonical_passthrough` | Trace list per workstation. |
| `Topology.WorkstationNodesByID[*].WorkstationName` | `dashboard_boundary` | Workstation label for the activity table. |

### Completed History

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.Session.DispatchHistory[*].DispatchID` | `canonical_passthrough` | Completed row identity. |
| `Runtime.Session.DispatchHistory[*].TransitionID` | `canonical_passthrough` | Workstation fallback label. |
| `Runtime.Session.DispatchHistory[*].Workstation.Name` | `canonical_passthrough` | Preferred workstation label. |
| `Runtime.Session.DispatchHistory[*].Result.Outcome` | `canonical_passthrough` | Success/rejected/failed status column. |
| `Runtime.Session.DispatchHistory[*].StartedAt` | `canonical_passthrough` | Started column. |
| `Runtime.Session.DispatchHistory[*].CompletedAt` | `canonical_passthrough` | Ended column. |
| `Runtime.Session.DispatchHistory[*].DurationMillis` | `canonical_passthrough` | Duration column. |
| `Runtime.Session.DispatchHistory[*].InputWorkItems[*].DisplayName` | `canonical_passthrough` | Preferred input label. |
| `Runtime.Session.DispatchHistory[*].InputWorkItems[*].ID` | `canonical_passthrough` | Input fallback label. |
| `Runtime.Session.DispatchHistory[*].ConsumedInputs[*].WorkItem.DisplayName` | `canonical_passthrough` | Input compatibility fallback label. |
| `Runtime.Session.DispatchHistory[*].ConsumedInputs[*].WorkItem.ID` | `canonical_passthrough` | Input compatibility fallback label. |
| `Runtime.Session.DispatchHistory[*].WorkItemIDs` | `canonical_passthrough` | Final input/output fallback labels. |
| `Runtime.Session.DispatchHistory[*].OutputWorkItems[*].DisplayName` | `canonical_passthrough` | Preferred output label. |
| `Runtime.Session.DispatchHistory[*].OutputWorkItems[*].ID` | `canonical_passthrough` | Output fallback label. |
| `Runtime.Session.DispatchHistory[*].TerminalWork.WorkItem.DisplayName` | `canonical_passthrough` | Preferred terminal output label. |
| `Runtime.Session.DispatchHistory[*].TerminalWork.WorkItem.ID` | `canonical_passthrough` | Terminal output fallback label. |
| `Runtime.Session.DispatchHistory[*].Result.FailureReason` | `canonical_passthrough` | Failure reason column for failed-work details. |
| `Runtime.Session.DispatchHistory[*].Result.FailureMessage` | `canonical_passthrough` | Failure message column for failed-work details. |

### Provider Sessions

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.Session.ProviderSessions[*].DispatchID` | `canonical_passthrough` | Provider-session row identity. |
| `Runtime.Session.ProviderSessions[*].TransitionID` | `canonical_passthrough` | Workstation fallback label. |
| `Runtime.Session.ProviderSessions[*].WorkstationName` | `canonical_passthrough` | Preferred provider-session row label. |
| `Runtime.Session.ProviderSessions[*].ConsumedInputs[*].WorkItem.DisplayName` | `canonical_passthrough` | Preferred provider-session work label. |
| `Runtime.Session.ProviderSessions[*].ConsumedInputs[*].WorkItem.ID` | `canonical_passthrough` | Provider-session work fallback label. |
| `Runtime.Session.ProviderSessions[*].WorkItemIDs` | `canonical_passthrough` | Provider-session fallback label when consumed inputs are absent. |
| `Runtime.Session.ProviderSessions[*].ProviderSession` | `canonical_passthrough` | Rendered provider metadata (`provider`, `session_id`, `model`, token counts). |

### Failed-Work Details And Session Metrics

| Source field | Classification | Use |
| --- | --- | --- |
| `Runtime.Session.HasData` | `canonical_passthrough` | Enables the session-metrics section. |
| `Runtime.Session.DispatchedCount` | `canonical_passthrough` | Session metrics total. |
| `Runtime.Session.CompletedCount` | `canonical_passthrough` | Session metrics total. |
| `Runtime.Session.FailedCount` | `canonical_passthrough` | Session metrics total. |
| `Runtime.Session.DispatchedByWorkType` | `canonical_passthrough` | Session metrics grouped counts. |
| `Runtime.Session.CompletedByWorkType` | `canonical_passthrough` | Session metrics grouped counts. |
| `Runtime.Session.FailedByWorkType` | `canonical_passthrough` | Session metrics grouped counts. |
| `Topology.WorkstationNodesByID[*].InputPlaces[*].PlaceID` | `dashboard_boundary` | Builds the place-category map. |
| `Topology.WorkstationNodesByID[*].InputPlaces[*].StateCategory` | `dashboard_boundary` | Detects terminal versus failed places. |
| `Topology.WorkstationNodesByID[*].OutputPlaces[*].PlaceID` | `dashboard_boundary` | Builds the place-category map. |
| `Topology.WorkstationNodesByID[*].OutputPlaces[*].StateCategory` | `dashboard_boundary` | Detects terminal versus failed places. |
| `Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].DisplayName` | `canonical_passthrough` | Preferred completed/failed work label. |
| `Runtime.PlaceOccupancyWorkItemsByPlaceID[*][*].WorkID` | `canonical_passthrough` | Completed/failed work fallback label. |

## Aggregate Fields The Simple Dashboard Does Not Need

These fields are currently present on the aggregate seam but unused by the formatter:

- `FactoryWorldTopologyView.SubmitWorkTypes`
- `FactoryWorldTopologyView.WorkstationNodeIDs`
- `FactoryWorldTopologyView.Edges`
- `FactoryWorldRuntimeView.ActiveDispatchIDs`
- `FactoryWorldRuntimeView.ActiveWorkstationNodeIDs`
- `FactoryWorldRuntimeView.InferenceAttemptsByDispatchID`
- `FactoryWorldRuntimeView.ActiveThrottlePauses`

These are the broad aggregate fields most likely to become deletion candidates or to move behind narrower boundary-specific adapters in later stories.

## Story-Relevant Conclusions

- Active workstations depend on canonical active execution records plus the `InFlightDispatchCount` counter.
- Queue counts depend on `PlaceTokenCounts` with `CurrentWorkItemsByPlaceID` and `PlaceOccupancyWorkItemsByPlaceID` as the label sources.
- Workstation activity depends on canonical activity records plus a topology name lookup from `WorkstationNodesByID`.
- Completed history, provider sessions, failed-work details, and session metrics all come from `Runtime.Session` plus the topology-derived terminal/failed place-category lookup.
- The simple dashboard does not need graph edges, submit-work topology, inference attempts, throttle pauses, or active-node ID lists from the shared aggregate shell.
