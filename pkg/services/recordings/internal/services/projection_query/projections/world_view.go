package projections

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

func factoryWorldViewFactory(factory *interfaces.FactorySnapshot) *interfaces.FactorySnapshot {
	return factory.Clone()
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

func dispatchHasCustomerWork(ids []string, items map[string]work.FactoryWorkItem) bool {
	return len(workItemRefsForIDs(work.WorkPayloadLineageProjection{}, ids, items)) > 0
}

func hasCustomerWorkItems(items map[string]work.FactoryWorkItem) bool {
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

func countFailedByWorkType(failed map[string]work.FactoryWorkItem) map[string]int {
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
	lineage work.WorkPayloadLineageProjection,
	ids []string,
	items map[string]work.FactoryWorkItem,
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

func workItemRefsForIDs(
	lineage work.WorkPayloadLineageProjection,
	ids []string,
	items map[string]work.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range sortedStrings(ids) {
		item, ok := items[id]
		if !ok || item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, item))
	}
	return refs
}

func workItemRefsForItems(
	lineage work.WorkPayloadLineageProjection,
	items []work.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, item))
		seen[item.ID] = struct{}{}
	}
	return refs
}

func workItemRefsForInputs(
	lineage work.WorkPayloadLineageProjection,
	inputs []interfaces.WorkstationInput,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, *input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
	}
	return refs
}

func providerSessionWorkItemRefs(
	lineage work.WorkPayloadLineageProjection,
	session interfaces.FactoryWorldProviderSessionRecord,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(session.ConsumedInputs)+len(session.WorkItemIDs))
	seen := make(map[string]struct{}, len(session.ConsumedInputs)+len(session.WorkItemIDs))
	for _, input := range session.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRefWithSelectedPayload(lineage, *input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
	}
	for _, workID := range sortedStrings(session.WorkItemIDs) {
		if workID == "" {
			continue
		}
		if _, exists := seen[workID]; exists {
			continue
		}
		refs = append(refs, selectedWorkItemRefForID(lineage, workID, nil))
		seen[workID] = struct{}{}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func workItemRef(item work.FactoryWorkItem) interfaces.FactoryWorldWorkItemRef {
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return interfaces.FactoryWorldWorkItemRef{
		WorkID:                   item.ID,
		WorkTypeID:               item.WorkTypeID,
		DisplayName:              item.DisplayName,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
	}
}

func workItemRefWithSelectedPayload(
	lineage work.WorkPayloadLineageProjection,
	item work.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	return selectedWorkItemRefForID(lineage, item.ID, &item)
}

func selectedWorkItemRefForID(
	lineage work.WorkPayloadLineageProjection,
	workID string,
	fallback *work.FactoryWorkItem,
) interfaces.FactoryWorldWorkItemRef {
	resolution := lineage.ResolveSelectedWorkSnapshot(workID)
	if resolution.Status == work.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
		return lineageResolvedWorkItemRef(resolution.Snapshot, string(resolution.Status))
	}

	item := work.FactoryWorkItem{ID: workID}
	if fallback != nil {
		item = *fallback
	}
	ref := workItemRef(item)
	if ref.WorkID == "" {
		ref.WorkID = workID
	}
	ref.State = item.State
	ref.PayloadStatus = string(resolution.Status)
	ref.PayloadUnavailableReason = resolution.Reason
	return ref
}

func lineageResolvedWorkItemRef(
	snapshot *work.WorkPayloadSnapshot,
	payloadStatus string,
) interfaces.FactoryWorldWorkItemRef {
	ref := workItemRef(snapshot.WorkItem)
	ref.State = snapshot.WorkItem.State
	ref.Content = work.CloneWorkContentParts(snapshot.WorkItem.Content)
	ref.PayloadStatus = payloadStatus
	ref.LineageLogicalWorkID = snapshot.LogicalWorkID
	ref.LineageSourceKind = string(snapshot.SourceKind)
	ref.LineageContinuity = string(snapshot.Continuity)
	ref.LineageParentWorkIDs = cloneStringSlice(snapshot.ParentWorkIDs)
	return ref
}
