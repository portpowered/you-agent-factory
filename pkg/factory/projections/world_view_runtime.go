package projections

import (
	"sort"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func buildFactoryWorldRuntimeView(
	state interfaces.FactoryWorldState,
	simpleDashboardRuntime SimpleDashboardRuntimeProjection,
) interfaces.FactoryWorldRuntimeView {
	activeIDs := customerActiveDispatchIDs(state)

	return interfaces.FactoryWorldRuntimeView{
		InFlightDispatchCount:            simpleDashboardRuntime.InFlightDispatchCount,
		ActiveDispatchIDs:                activeIDs,
		ActiveExecutionsByDispatchID:     simpleDashboardRuntime.ActiveExecutionsByDispatchID,
		ActiveWorkstationNodeIDs:         buildFactoryWorldActiveNodeIDs(state.ActiveDispatches),
		InferenceAttemptsByDispatchID:    interfaces.CloneFactoryWorldInferenceAttemptsByDispatchID(state.InferenceAttemptsByDispatchID),
		WorkstationActivityByNodeID:      simpleDashboardRuntime.WorkstationActivityByNodeID,
		PlaceTokenCounts:                 simpleDashboardRuntime.PlaceTokenCounts,
		CurrentWorkItemsByPlaceID:        simpleDashboardRuntime.CurrentWorkItemsByPlaceID,
		PlaceOccupancyWorkItemsByPlaceID: simpleDashboardRuntime.PlaceOccupancyWorkItemsByPlaceID,
		Session:                          simpleDashboardRuntime.Session,
	}
}

func buildFactoryWorldActiveExecutions(state interfaces.FactoryWorldState, activeIDs []string) map[string]interfaces.FactoryWorldActiveExecution {
	if len(activeIDs) == 0 {
		return nil
	}
	executions := make(map[string]interfaces.FactoryWorldActiveExecution, len(activeIDs))
	for _, dispatchID := range activeIDs {
		dispatch := state.ActiveDispatches[dispatchID]
		workItems := workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID)
		executions[dispatchID] = interfaces.FactoryWorldActiveExecution{
			DispatchID:               dispatchID,
			WorkstationNodeID:        dispatch.TransitionID,
			TransitionID:             dispatch.TransitionID,
			WorkstationName:          dispatch.Workstation.Name,
			StartedAt:                dispatch.StartedAt,
			WorkTypeIDs:              workTypeIDsForWorkRefs(workItems),
			WorkItems:                workItems,
			CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
			PreviousChainingTraceIDs: cloneStringSlice(dispatch.PreviousChainingTraceIDs),
			TraceIDs:                 interfaces.CanonicalChainingTraceIDs(dispatch.TraceIDs),
			ConsumedInputs:           interfaces.CloneWorkstationInputs(dispatch.Inputs),
		}
	}
	return executions
}

func buildFactoryWorldActivity(state interfaces.FactoryWorldState, activeIDs []string) map[string]interfaces.FactoryWorldActivity {
	if len(activeIDs) == 0 {
		return nil
	}
	activity := make(map[string]interfaces.FactoryWorldActivity)
	for _, dispatchID := range activeIDs {
		dispatch := state.ActiveDispatches[dispatchID]
		transitionID := dispatch.TransitionID
		current := activity[transitionID]
		current.WorkstationNodeID = transitionID
		current.ActiveDispatchIDs = append(current.ActiveDispatchIDs, dispatchID)
		current.ActiveWorkItems = mergeWorkRefs(
			current.ActiveWorkItems,
			workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID),
		)
		current.TraceIDs = interfaces.CanonicalChainingTraceIDs(append(current.TraceIDs, dispatch.TraceIDs...))
		activity[transitionID] = current
	}
	return activity
}

func buildFactoryWorldActiveNodeIDs(dispatches map[string]interfaces.FactoryWorldDispatch) []string {
	var ids []string
	for _, dispatch := range dispatches {
		ids = appendUnique(ids, dispatch.TransitionID)
	}
	return sortedStrings(ids)
}

func buildFactoryWorldPlaceTokenCounts(occupancy map[string]interfaces.FactoryPlaceOccupancy) map[string]int {
	if len(occupancy) == 0 {
		return nil
	}
	counts := make(map[string]int, len(occupancy))
	for placeID, entry := range occupancy {
		if interfaces.IsSystemTimePlace(placeID) {
			continue
		}
		if entry.TokenCount > 0 {
			counts[placeID] = entry.TokenCount
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func buildFactoryWorldCurrentWorkItemsByPlaceID(state interfaces.FactoryWorldState) map[string][]interfaces.FactoryWorldWorkItemRef {
	workTypeIDs := make(map[string]struct{}, len(state.Topology.WorkTypes))
	for _, workType := range state.Topology.WorkTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) {
			continue
		}
		workTypeIDs[workType.ID] = struct{}{}
	}
	refsByPlace := make(map[string][]interfaces.FactoryWorldWorkItemRef)
	for _, place := range state.Topology.Places {
		if _, ok := workTypeIDs[place.TypeID]; !ok {
			continue
		}
		if place.Category == "TERMINAL" || place.Category == "FAILED" {
			continue
		}
		refsByPlace[place.ID] = []interfaces.FactoryWorldWorkItemRef{}
	}
	for placeID, entry := range state.PlaceOccupancyByID {
		if _, ok := refsByPlace[placeID]; !ok {
			continue
		}
		refsByPlace[placeID] = workRefsForActiveIDs(entry.WorkItemIDs, state.ActiveWorkItemsByID)
	}
	if len(refsByPlace) == 0 {
		return nil
	}
	return refsByPlace
}

func buildFactoryWorldPlaceOccupancyWorkItemsByPlaceID(state interfaces.FactoryWorldState) map[string][]interfaces.FactoryWorldWorkItemRef {
	workTypeIDs := make(map[string]struct{}, len(state.Topology.WorkTypes))
	for _, workType := range state.Topology.WorkTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) {
			continue
		}
		workTypeIDs[workType.ID] = struct{}{}
	}
	workPlaceIDs := make(map[string]struct{}, len(state.Topology.Places))
	for _, place := range state.Topology.Places {
		if interfaces.IsSystemTimePlace(place.ID) {
			continue
		}
		if _, ok := workTypeIDs[place.TypeID]; ok {
			workPlaceIDs[place.ID] = struct{}{}
		}
	}

	refsByPlace := make(map[string][]interfaces.FactoryWorldWorkItemRef)
	for placeID, entry := range state.PlaceOccupancyByID {
		if _, ok := workPlaceIDs[placeID]; !ok {
			continue
		}
		refs := workItemRefsForIDs(entry.WorkItemIDs, state.WorkItemsByID)
		if len(refs) == 0 {
			continue
		}
		refsByPlace[placeID] = refs
	}
	if len(refsByPlace) == 0 {
		return nil
	}
	return refsByPlace
}

func buildFactoryWorldDispatchHistory(state interfaces.FactoryWorldState) []interfaces.FactoryWorldDispatchCompletion {
	if len(state.CompletedDispatches) == 0 {
		return nil
	}
	completions := make([]interfaces.FactoryWorldDispatchCompletion, 0, len(state.CompletedDispatches))
	for _, dispatch := range state.CompletedDispatches {
		if !dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) && dispatch.TransitionID != interfaces.SystemTimeExpiryTransitionID {
			continue
		}
		completions = append(completions, interfaces.CloneFactoryWorldDispatchCompletion(dispatch))
	}
	sort.Slice(completions, func(i, j int) bool {
		if !completions[i].CompletedAt.Equal(completions[j].CompletedAt) {
			return completions[i].CompletedAt.Before(completions[j].CompletedAt)
		}
		if completions[i].TransitionID != completions[j].TransitionID {
			return completions[i].TransitionID < completions[j].TransitionID
		}
		return completions[i].DispatchID < completions[j].DispatchID
	})
	return completions
}

func buildFactoryWorldProviderSessions(state interfaces.FactoryWorldState) []interfaces.FactoryWorldProviderSessionRecord {
	if len(state.ProviderSessions) == 0 {
		return nil
	}
	sessions := make([]interfaces.FactoryWorldProviderSessionRecord, 0, len(state.ProviderSessions))
	for _, session := range state.ProviderSessions {
		if !dispatchHasCustomerWork(session.WorkItemIDs, state.WorkItemsByID) {
			continue
		}
		sessions = append(sessions, interfaces.CloneFactoryWorldProviderSessionRecord(session))
	}
	return sessions
}

func countCustomerCompletedDispatches(state interfaces.FactoryWorldState) int {
	count := 0
	for _, dispatch := range state.CompletedDispatches {
		if dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			count++
		}
	}
	return count
}

func countCompletedDispatches(state interfaces.FactoryWorldState) int {
	count := 0
	for _, dispatch := range state.CompletedDispatches {
		if !dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			continue
		}
		if dispatch.Result.Outcome == "ACCEPTED" || (dispatch.TerminalWork != nil && dispatch.TerminalWork.Status != "FAILED") {
			count++
		}
	}
	return count
}

func countFailedWorkItems(failed map[string]interfaces.FactoryWorkItem) int {
	count := 0
	for _, work := range failed {
		if interfaces.IsSystemTimeWorkType(work.WorkTypeID) {
			continue
		}
		count++
	}
	return count
}

func countDispatchedByWorkType(state interfaces.FactoryWorldState) map[string]int {
	counts := make(map[string]int)
	for _, dispatch := range state.ActiveDispatches {
		for _, ref := range workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID) {
			counts[ref.WorkTypeID]++
		}
	}
	for _, dispatch := range state.CompletedDispatches {
		for _, ref := range workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID) {
			counts[ref.WorkTypeID]++
		}
	}
	return nilIfEmpty(counts)
}
