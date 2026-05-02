package dashboardrender

import (
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// SimpleDashboardRenderData carries only the event-first data the simple
// dashboard formatter needs across the service-to-CLI seam.
type SimpleDashboardRenderData struct {
	InFlightDispatchCount            int
	ActiveExecutionsByDispatchID     map[string]SimpleDashboardActiveExecution
	ActiveThrottlePauses             []interfaces.FactoryWorldThrottlePause
	PlaceTokenCounts                 map[string]int
	CurrentWorkItemsByPlaceID        map[string][]interfaces.FactoryWorldWorkItemRef
	PlaceOccupancyWorkItemsByPlaceID map[string][]interfaces.FactoryWorldWorkItemRef
	WorkstationActivityByNodeID      map[string]SimpleDashboardWorkstationActivity
	PlaceCategoriesByID              map[string]string
	Session                          SimpleDashboardSessionData
}

// SimpleDashboardActiveExecution carries the active execution fields rendered
// in the dashboard's active workstation section.
type SimpleDashboardActiveExecution struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	StartedAt       time.Time
	WorkTypeIDs     []string
	WorkItems       []interfaces.FactoryWorldWorkItemRef
}

// SimpleDashboardWorkstationActivity carries workstation activity rows plus the
// explicit workstation name lookup needed by the formatter.
type SimpleDashboardWorkstationActivity struct {
	NodeID            string
	WorkstationName   string
	ActiveDispatchIDs []string
	ActiveWorkItems   []interfaces.FactoryWorldWorkItemRef
	TraceIDs          []string
}

// SimpleDashboardSessionData carries session metrics together with the history
// and provider-session records needed for dashboard-local summaries.
type SimpleDashboardSessionData struct {
	HasData              bool
	DispatchedCount      int
	CompletedCount       int
	FailedCount          int
	DispatchedByWorkType map[string]int
	CompletedByWorkType  map[string]int
	FailedByWorkType     map[string]int
	DispatchHistory      []interfaces.FactoryWorldDispatchCompletion
	ProviderSessions     []interfaces.FactoryWorldProviderSessionRecord
}

// SimpleDashboardRenderDataFromWorldState builds the dedicated render DTO
// directly from canonical selected-tick state plus authored topology.
func SimpleDashboardRenderDataFromWorldState(worldState interfaces.FactoryWorldState) SimpleDashboardRenderData {
	activeDispatchIDs := customerActiveDispatchIDs(worldState)
	activeExecutions := make(map[string]SimpleDashboardActiveExecution, len(activeDispatchIDs))
	for _, dispatchID := range activeDispatchIDs {
		dispatch := worldState.ActiveDispatches[dispatchID]
		workItems := workItemRefsForIDs(dispatch.WorkItemIDs, worldState.WorkItemsByID)
		activeExecutions[dispatchID] = SimpleDashboardActiveExecution{
			DispatchID:      dispatchID,
			TransitionID:    dispatch.TransitionID,
			WorkstationName: dispatch.Workstation.Name,
			StartedAt:       dispatch.StartedAt,
			WorkTypeIDs:     workTypeIDsForWorkRefs(workItems),
			WorkItems:       workItems,
		}
	}

	completedHistory := buildDispatchHistory(worldState)
	providerSessions := buildProviderSessions(worldState)

	return SimpleDashboardRenderData{
		InFlightDispatchCount:            len(activeDispatchIDs),
		ActiveExecutionsByDispatchID:     activeExecutions,
		PlaceTokenCounts:                 buildPlaceTokenCounts(worldState.PlaceOccupancyByID),
		CurrentWorkItemsByPlaceID:        buildCurrentWorkItemsByPlaceID(worldState),
		PlaceOccupancyWorkItemsByPlaceID: buildPlaceOccupancyWorkItemsByPlaceID(worldState),
		WorkstationActivityByNodeID:      buildWorkstationActivityByNodeID(worldState, activeDispatchIDs),
		PlaceCategoriesByID:              placeCategoriesFromTopology(worldState.Topology),
		Session: SimpleDashboardSessionData{
			HasData:              len(activeDispatchIDs) > 0 || len(completedHistory) > 0 || hasCustomerWorkItems(worldState.WorkItemsByID),
			DispatchedCount:      len(activeDispatchIDs) + countCustomerCompletedDispatches(worldState),
			CompletedCount:       countCompletedDispatches(worldState),
			FailedCount:          countFailedDispatches(worldState),
			DispatchedByWorkType: countDispatchedByWorkType(worldState),
			CompletedByWorkType:  countTerminalByWorkType(worldState.TerminalWorkByID),
			FailedByWorkType:     countFailedByWorkType(worldState.FailedWorkItemsByID),
			DispatchHistory:      completedHistory,
			ProviderSessions:     providerSessions,
		},
	}
}

func buildWorkstationActivityByNodeID(
	worldState interfaces.FactoryWorldState,
	activeDispatchIDs []string,
) map[string]SimpleDashboardWorkstationActivity {
	if len(activeDispatchIDs) == 0 {
		return nil
	}
	activity := make(map[string]SimpleDashboardWorkstationActivity)
	workstationNames := workstationNamesByID(worldState.Topology)
	for _, dispatchID := range activeDispatchIDs {
		dispatch := worldState.ActiveDispatches[dispatchID]
		current := activity[dispatch.TransitionID]
		current.NodeID = dispatch.TransitionID
		current.WorkstationName = workstationNames[dispatch.TransitionID]
		if current.WorkstationName == "" {
			current.WorkstationName = dispatch.Workstation.Name
		}
		current.ActiveDispatchIDs = append(current.ActiveDispatchIDs, dispatchID)
		current.ActiveWorkItems = mergeWorkRefs(current.ActiveWorkItems, workItemRefsForIDs(dispatch.WorkItemIDs, worldState.WorkItemsByID))
		current.TraceIDs = canonicalChainingTraceIDs(append(current.TraceIDs, dispatch.TraceIDs...))
		activity[dispatch.TransitionID] = current
	}
	return activity
}

func workstationNamesByID(topology interfaces.InitialStructurePayload) map[string]string {
	if len(topology.Workstations) == 0 {
		return nil
	}
	names := make(map[string]string, len(topology.Workstations))
	for _, workstation := range topology.Workstations {
		if workstation.ID == "" || workstation.ID == interfaces.SystemTimeExpiryTransitionID {
			continue
		}
		names[workstation.ID] = workstation.Name
	}
	return names
}

func placeCategoriesFromTopology(topology interfaces.InitialStructurePayload) map[string]string {
	if len(topology.Workstations) == 0 {
		return nil
	}
	categories := make(map[string]string)
	placesByID := make(map[string]interfaces.FactoryPlace, len(topology.Places))
	for _, place := range topology.Places {
		placesByID[place.ID] = place
	}
	for _, workstation := range topology.Workstations {
		for _, placeID := range workstation.InputPlaceIDs {
			place, ok := placesByID[placeID]
			if !ok || place.ID == "" || place.Category == "" {
				continue
			}
			categories[place.ID] = place.Category
		}
		for _, placeID := range appendWorkstationOutputPlaceIDs(workstation) {
			place, ok := placesByID[placeID]
			if !ok || place.ID == "" || place.Category == "" {
				continue
			}
			categories[place.ID] = place.Category
		}
	}
	if len(categories) == 0 {
		return nil
	}
	return categories
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

func buildPlaceTokenCounts(occupancy map[string]interfaces.FactoryPlaceOccupancy) map[string]int {
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
	return nilIfEmpty(counts)
}

func buildCurrentWorkItemsByPlaceID(state interfaces.FactoryWorldState) map[string][]interfaces.FactoryWorldWorkItemRef {
	workTypeIDs := customerWorkTypeIDs(state.Topology.WorkTypes)
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

func buildPlaceOccupancyWorkItemsByPlaceID(state interfaces.FactoryWorldState) map[string][]interfaces.FactoryWorldWorkItemRef {
	workTypeIDs := customerWorkTypeIDs(state.Topology.WorkTypes)
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

func customerWorkTypeIDs(workTypes []interfaces.FactoryWorkType) map[string]struct{} {
	ids := make(map[string]struct{}, len(workTypes))
	for _, workType := range workTypes {
		if interfaces.IsSystemTimeWorkType(workType.ID) {
			continue
		}
		ids[workType.ID] = struct{}{}
	}
	return ids
}

func buildDispatchHistory(state interfaces.FactoryWorldState) []interfaces.FactoryWorldDispatchCompletion {
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

func buildProviderSessions(state interfaces.FactoryWorldState) []interfaces.FactoryWorldProviderSessionRecord {
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

func countFailedDispatches(state interfaces.FactoryWorldState) int {
	count := 0
	for _, dispatch := range state.FailedDispatches {
		if dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			count++
		}
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

func countTerminalByWorkType(terminal map[string]interfaces.FactoryTerminalWork) map[string]int {
	counts := make(map[string]int)
	for _, work := range terminal {
		if work.Status == "FAILED" || interfaces.IsSystemTimeWorkType(work.WorkItem.WorkTypeID) {
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

func workItemRefsForIDs(ids []string, items map[string]interfaces.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(ids))
	for _, id := range sortedStrings(ids) {
		item, ok := items[id]
		if !ok || item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		refs = append(refs, workItemRef(item))
	}
	return refs
}

func workRefsForActiveIDs(ids []string, items map[string]interfaces.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
	refs := workItemRefsForIDs(ids, items)
	if refs == nil {
		return []interfaces.FactoryWorldWorkItemRef{}
	}
	return refs
}

func workItemRef(item interfaces.FactoryWorkItem) interfaces.FactoryWorldWorkItemRef {
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return interfaces.FactoryWorldWorkItemRef{
		WorkID:                   item.ID,
		WorkTypeID:               item.WorkTypeID,
		DisplayName:              item.DisplayName,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
	}
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

func canonicalChainingTraceIDs(traceIDs []string) []string {
	return sortedStrings(uniqueNonEmpty(traceIDs))
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func appendWorkstationOutputPlaceIDs(workstation interfaces.FactoryWorkstation) []string {
	outputs := append([]string(nil), workstation.OutputPlaceIDs...)
	outputs = append(outputs, workstation.ContinuePlaceIDs...)
	outputs = append(outputs, workstation.RejectionPlaceIDs...)
	outputs = append(outputs, workstation.FailurePlaceIDs...)
	return outputs
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	sort.Strings(cloned)
	return cloned
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

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func nilIfEmpty(values map[string]int) map[string]int {
	delete(values, "")
	if len(values) == 0 {
		return nil
	}
	return values
}
