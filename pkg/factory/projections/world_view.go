package projections

import (
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuildFactoryWorldView projects generic reconstructed world state into a thin
// selected-tick adapter. Presentation-only compatibility shaping belongs at
// API, CLI, and UI boundaries.
func BuildFactoryWorldView(state interfaces.FactoryWorldState) interfaces.FactoryWorldView {
	simpleDashboardProjection := BuildSimpleDashboardProjection(state)
	return interfaces.FactoryWorldView{
		Factory:  factoryWorldViewFactory(state.Factory),
		Topology: buildFactoryWorldTopologyView(state.Topology),
		Runtime:  buildFactoryWorldRuntimeView(state, simpleDashboardProjection.Runtime),
	}
}

func factoryWorldViewFactory(factory *factoryapi.Factory) *factoryapi.Factory {
	if factory == nil {
		return nil
	}
	clone, err := cloneGeneratedFactory(*factory)
	if err != nil {
		return nil
	}
	return &clone
}

func customerActiveDispatchIDs(state interfaces.FactoryWorldState) []string {
	activeIDs := make([]string, 0, len(state.ActiveDispatches))
	for dispatchID, dispatch := range state.ActiveDispatches {
		if dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			activeIDs = append(activeIDs, dispatchID)
		}
	}
	sort.Strings(activeIDs)
	return activeIDs
}

func dispatchHasCustomerWork(ids []string, items map[string]interfaces.FactoryWorkItem) bool {
	return len(workItemRefsForIDs(interfaces.WorkPayloadLineageProjection{}, ids, items)) > 0
}

func hasCustomerWorkItems(items map[string]interfaces.FactoryWorkItem) bool {
	for _, item := range items {
		if !interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			return true
		}
	}
	return false
}

func countTerminalByWorkType(terminal map[string]interfaces.FactoryTerminalWork) map[string]int {
	counts := make(map[string]int)
	for _, work := range terminal {
		if work.Status == "FAILED" {
			continue
		}
		if interfaces.IsSystemTimeWorkType(work.WorkItem.WorkTypeID) {
			continue
		}
		counts[work.WorkItem.WorkTypeID]++
	}
	return nilIfEmpty(counts)
}

func countFailedByWorkType(failed map[string]interfaces.FactoryWorkItem) map[string]int {
	counts := make(map[string]int)
	for _, work := range failed {
		if interfaces.IsSystemTimeWorkType(work.WorkTypeID) {
			continue
		}
		counts[work.WorkTypeID]++
	}
	return nilIfEmpty(counts)
}

func workRefsForActiveIDs(
	lineage interfaces.WorkPayloadLineageProjection,
	ids []string,
	items map[string]interfaces.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := workItemRefsForIDs(lineage, ids, items)
	if refs == nil {
		return []interfaces.FactoryWorldWorkItemRef{}
	}
	return refs
}

func mergeWorkRefs(existing []interfaces.FactoryWorldWorkItemRef, additional []interfaces.FactoryWorldWorkItemRef) []interfaces.FactoryWorldWorkItemRef {
	byID := make(map[string]interfaces.FactoryWorldWorkItemRef, len(existing)+len(additional))
	for _, ref := range existing {
		byID[ref.WorkID] = ref
	}
	for _, ref := range additional {
		byID[ref.WorkID] = ref
	}
	ids := sortedMapKeys(byID)
	merged := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range ids {
		merged = append(merged, byID[id])
	}
	return merged
}

func workTypeIDsForWorkRefs(refs []interfaces.FactoryWorldWorkItemRef) []string {
	var ids []string
	for _, ref := range refs {
		ids = appendUnique(ids, ref.WorkTypeID)
	}
	return sortedStrings(ids)
}

func filterCustomerPlaceIDs(placeIDs []string) []string {
	filtered := make([]string, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		if interfaces.IsSystemTimePlace(placeID) {
			continue
		}
		filtered = append(filtered, placeID)
	}
	return filtered
}

func isSystemTimeWorkstation(workstationID string) bool {
	return workstationID == interfaces.SystemTimeExpiryTransitionID
}

func sortedMapKeys[T any](values map[string]T) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nilIfEmpty(values map[string]int) map[string]int {
	delete(values, "")
	if len(values) == 0 {
		return nil
	}
	return values
}

// ProjectActiveThrottlePauses converts dispatcher-owned runtime pause windows
// into the world-view/dashboard pause shape using authored topology metadata.
func ProjectActiveThrottlePauses(
	topology interfaces.InitialStructurePayload,
	pauses []interfaces.ActiveThrottlePause,
) []interfaces.FactoryWorldThrottlePause {
	if len(pauses) == 0 {
		return nil
	}

	workersByID := make(map[string]interfaces.FactoryWorker, len(topology.Workers))
	for _, worker := range topology.Workers {
		if worker.ID == "" {
			continue
		}
		workersByID[worker.ID] = worker
	}

	placesByID := make(map[string]interfaces.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		if place.ID == "" {
			continue
		}
		placesByID[place.ID] = place
	}

	projected := make([]interfaces.FactoryWorldThrottlePause, 0, len(pauses))
	for _, pause := range pauses {
		projected = append(projected, interfaces.FactoryWorldThrottlePause{
			LaneID:                   pause.LaneID,
			Provider:                 pause.Provider,
			Model:                    pause.Model,
			PausedAt:                 pause.PausedAt,
			PausedUntil:              pause.PausedUntil,
			RecoverAt:                pause.PausedUntil,
			AffectedTransitionIDs:    affectedTransitionIDsForPause(topology.Workstations, workersByID, pause),
			AffectedWorkstationNames: affectedWorkstationNamesForPause(topology.Workstations, workersByID, pause),
			AffectedWorkerTypes:      affectedWorkerTypesForPause(topology.Workstations, workersByID, pause),
			AffectedWorkTypeIDs:      affectedWorkTypeIDsForPause(topology.Workstations, workersByID, placesByID, pause),
		})
	}

	return projected
}

func BuildFactoryWorldViewWithActiveThrottlePauses(
	state interfaces.FactoryWorldState,
	pauses []interfaces.ActiveThrottlePause,
) interfaces.FactoryWorldView {
	view := BuildFactoryWorldView(state)
	view.Runtime.ActiveThrottlePauses = ProjectActiveThrottlePauses(state.Topology, pauses)
	return view
}

func affectedTransitionIDsForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var ids []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		ids = appendUnique(ids, workstation.ID)
	}
	return sortedStrings(ids)
}

func affectedWorkstationNamesForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var names []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		names = appendUnique(names, workstation.Name)
	}
	return sortedStrings(names)
}

func affectedWorkerTypesForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) []string {
	var workerTypes []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		workerTypes = appendUnique(workerTypes, workstation.WorkerID)
	}
	return sortedStrings(workerTypes)
}

func affectedWorkTypeIDsForPause(
	workstations []interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	placesByID map[string]interfaces.FactoryPlace,
	pause interfaces.ActiveThrottlePause,
) []string {
	var workTypeIDs []string
	for _, workstation := range workstations {
		if !workstationMatchesPause(workstation, workersByID, pause) {
			continue
		}
		for _, placeID := range workstation.InputPlaceIDs {
			place, ok := placesByID[placeID]
			if !ok || place.TypeID == "" || interfaces.IsSystemTimeWorkType(place.TypeID) {
				continue
			}
			workTypeIDs = appendUnique(workTypeIDs, place.TypeID)
		}
	}
	return sortedStrings(workTypeIDs)
}

func workstationMatchesPause(
	workstation interfaces.FactoryWorkstation,
	workersByID map[string]interfaces.FactoryWorker,
	pause interfaces.ActiveThrottlePause,
) bool {
	if workstation.WorkerID == "" {
		return false
	}
	worker, ok := workersByID[workstation.WorkerID]
	if !ok {
		return false
	}
	provider := firstNonEmpty(worker.ModelProvider, worker.Provider)
	return strings.EqualFold(provider, pause.Provider) && worker.Model == pause.Model
}
