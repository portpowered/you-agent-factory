package projections

import (
	"sort"

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
	if !factoryContainsSystemTimeGraph(clone) {
		return &clone
	}
	clone = omitSystemTimeGraphEntities(clone)
	metadata := map[string]string{}
	if clone.Metadata != nil {
		for key, value := range map[string]string(*clone.Metadata) {
			metadata[key] = value
		}
	}
	metadata["internalGraphEntitiesOmitted"] = "true"
	metadata["internalGraphEntityPolicy"] = "dashboard_view_omits_internal_system_time"
	generatedMetadata := factoryapi.StringMap(metadata)
	clone.Metadata = &generatedMetadata
	return &clone
}

func factoryContainsSystemTimeGraph(factory factoryapi.Factory) bool {
	for _, workType := range sliceValue(factory.WorkTypes) {
		if interfaces.IsSystemTimeWorkType(workType.Name) {
			return true
		}
	}
	for _, workstation := range sliceValue(factory.Workstations) {
		if workstationID(workstation) == interfaces.SystemTimeExpiryTransitionID {
			return true
		}
		for _, input := range workstation.Inputs {
			if interfaces.IsSystemTimeWorkType(input.WorkType) {
				return true
			}
		}
	}
	return false
}

func omitSystemTimeGraphEntities(factory factoryapi.Factory) factoryapi.Factory {
	if factory.WorkTypes != nil {
		workTypes := make([]factoryapi.WorkType, 0, len(*factory.WorkTypes))
		for _, workType := range *factory.WorkTypes {
			if interfaces.IsSystemTimeWorkType(workType.Name) {
				continue
			}
			workTypes = append(workTypes, workType)
		}
		factory.WorkTypes = &workTypes
	}
	if factory.Workstations != nil {
		workstations := make([]factoryapi.Workstation, 0, len(*factory.Workstations))
		for _, workstation := range *factory.Workstations {
			if workstationID(workstation) == interfaces.SystemTimeExpiryTransitionID {
				continue
			}
			workstation.Inputs = filterSystemTimeIOs(workstation.Inputs)
			workstation.Outputs = filterSystemTimeIOsPtr(workstation.Outputs)
			workstation.OnContinue = filterSystemTimeIOsPtr(workstation.OnContinue)
			workstation.OnRejection = filterSystemTimeIOsPtr(workstation.OnRejection)
			workstation.OnFailure = filterSystemTimeIOsPtr(workstation.OnFailure)
			workstations = append(workstations, workstation)
		}
		factory.Workstations = &workstations
	}
	return factory
}

func workstationID(workstation factoryapi.Workstation) string {
	if workstation.Id != nil && *workstation.Id != "" {
		return *workstation.Id
	}
	return workstation.Name
}

func filterSystemTimeIOsPtr(values *[]factoryapi.WorkstationIO) *[]factoryapi.WorkstationIO {
	if values == nil {
		return nil
	}
	filtered := filterSystemTimeIOs(*values)
	return &filtered
}

func filterSystemTimeIOs(values []factoryapi.WorkstationIO) []factoryapi.WorkstationIO {
	filtered := make([]factoryapi.WorkstationIO, 0, len(values))
	for _, value := range values {
		if interfaces.IsSystemTimeWorkType(value.WorkType) {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
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
