package projections

import "github.com/portpowered/agent-factory/pkg/interfaces"

// SimpleDashboardProjection contains the selected-tick data shared by the
// broad world-view projection and the simple dashboard boundary seam.
type SimpleDashboardProjection struct {
	Runtime              SimpleDashboardRuntimeProjection
	WorkstationNodesByID map[string]SimpleDashboardWorkstationNodeProjection
}

// SimpleDashboardRuntimeProjection contains the runtime-selected data used by
// the simple dashboard seam.
type SimpleDashboardRuntimeProjection struct {
	InFlightDispatchCount            int
	ActiveExecutionsByDispatchID     map[string]interfaces.FactoryWorldActiveExecution
	WorkstationActivityByNodeID      map[string]interfaces.FactoryWorldActivity
	PlaceTokenCounts                 map[string]int
	CurrentWorkItemsByPlaceID        map[string][]interfaces.FactoryWorldWorkItemRef
	PlaceOccupancyWorkItemsByPlaceID map[string][]interfaces.FactoryWorldWorkItemRef
	Session                          interfaces.FactoryWorldSessionRuntime
}

// SimpleDashboardWorkstationNodeProjection keeps only the workstation metadata
// the simple dashboard formatter consumes.
type SimpleDashboardWorkstationNodeProjection struct {
	WorkstationName string
	InputPlaces     []interfaces.FactoryWorldPlaceRef
	OutputPlaces    []interfaces.FactoryWorldPlaceRef
}

// BuildSimpleDashboardProjection derives the selected-tick data the simple
// dashboard seam uses without rebuilding the broad aggregate shell.
func BuildSimpleDashboardProjection(state interfaces.FactoryWorldState) SimpleDashboardProjection {
	activeIDs := customerActiveDispatchIDs(state)
	completedHistory := buildFactoryWorldDispatchHistory(state)

	return SimpleDashboardProjection{
		Runtime: SimpleDashboardRuntimeProjection{
			InFlightDispatchCount:            len(activeIDs),
			ActiveExecutionsByDispatchID:     buildFactoryWorldActiveExecutions(state, activeIDs),
			WorkstationActivityByNodeID:      buildFactoryWorldActivity(state, activeIDs),
			PlaceTokenCounts:                 buildFactoryWorldPlaceTokenCounts(state.PlaceOccupancyByID),
			CurrentWorkItemsByPlaceID:        buildFactoryWorldCurrentWorkItemsByPlaceID(state),
			PlaceOccupancyWorkItemsByPlaceID: buildFactoryWorldPlaceOccupancyWorkItemsByPlaceID(state),
			Session: interfaces.FactoryWorldSessionRuntime{
				HasData:              len(activeIDs) > 0 || len(completedHistory) > 0 || hasCustomerWorkItems(state.WorkItemsByID),
				DispatchedCount:      len(activeIDs) + countCustomerCompletedDispatches(state),
				CompletedCount:       countCompletedDispatches(state),
				FailedCount:          countFailedDispatches(state),
				DispatchHistory:      completedHistory,
				ProviderSessions:     buildFactoryWorldProviderSessions(state),
				DispatchedByWorkType: countDispatchedByWorkType(state),
				CompletedByWorkType:  countTerminalByWorkType(state.TerminalWorkByID),
				FailedByWorkType:     countFailedByWorkType(state.FailedWorkItemsByID),
			},
		},
		WorkstationNodesByID: buildSimpleDashboardWorkstationNodes(state.Topology),
	}
}

func buildSimpleDashboardWorkstationNodes(
	topology interfaces.InitialStructurePayload,
) map[string]SimpleDashboardWorkstationNodeProjection {
	if len(topology.Workstations) == 0 {
		return nil
	}

	placesByID := make(map[string]interfaces.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		placesByID[place.ID] = place
	}
	workTypeIDs := make(map[string]struct{}, len(topology.WorkTypes))
	for _, workType := range topology.WorkTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) {
			continue
		}
		workTypeIDs[workType.ID] = struct{}{}
	}
	resourceIDs := make(map[string]struct{}, len(topology.Resources))
	for _, resource := range topology.Resources {
		resourceIDs[resource.ID] = struct{}{}
	}

	nodes := make(map[string]SimpleDashboardWorkstationNodeProjection, len(topology.Workstations))
	for _, workstation := range topology.Workstations {
		if workstation.ID == interfaces.SystemTimeExpiryTransitionID {
			continue
		}
		nodes[workstation.ID] = SimpleDashboardWorkstationNodeProjection{
			WorkstationName: workstation.Name,
			InputPlaces:     buildFactoryWorldPlaceRefs(workstation.InputPlaceIDs, placesByID, workTypeIDs, resourceIDs),
			OutputPlaces: buildFactoryWorldPlaceRefs(
				appendCustomerOutputPlaceIDs(workstation),
				placesByID,
				workTypeIDs,
				resourceIDs,
			),
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	return nodes
}

func appendCustomerOutputPlaceIDs(workstation interfaces.FactoryWorkstation) []string {
	outputPlaceIDs := append([]string(nil), workstation.OutputPlaceIDs...)
	outputPlaceIDs = append(outputPlaceIDs, workstation.ContinuePlaceIDs...)
	outputPlaceIDs = append(outputPlaceIDs, workstation.RejectionPlaceIDs...)
	outputPlaceIDs = append(outputPlaceIDs, workstation.FailurePlaceIDs...)
	return outputPlaceIDs
}
