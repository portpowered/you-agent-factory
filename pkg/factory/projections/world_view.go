package projections

import (
	"sort"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuildFactoryWorldView projects generic reconstructed world state into a thin
// selected-tick adapter. Presentation-only compatibility shaping belongs at
// API, CLI, and UI boundaries.
func BuildFactoryWorldView(state interfaces.FactoryWorldState) interfaces.FactoryWorldView {
	simpleDashboardProjection := BuildSimpleDashboardProjection(state)
	return interfaces.FactoryWorldView{
		Topology: buildFactoryWorldTopologyView(state.Topology),
		Runtime:  buildFactoryWorldRuntimeView(state, simpleDashboardProjection.Runtime),
	}
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
	return len(workItemRefsForIDs(ids, items)) > 0
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

func workRefsForActiveIDs(ids []string, items map[string]interfaces.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
	refs := workItemRefsForIDs(ids, items)
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
