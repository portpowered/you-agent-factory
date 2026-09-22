package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	restoredCompletedAutomationStatus   = "COMPLETED"
	restoredLegacyAutomationDisposition = "completed_automation_without_recoverable_place"
)

type restoredWorkRecovery struct {
	excludedWorkIDs  map[string]struct{}
	toleratedWorkIDs map[string]struct{}
	legacyWorkIDs    map[string]struct{}
}

func restoreRestoredWorkMarking(
	cfg *runtimeConfig,
	marking *petri.Marking,
	constructionNow time.Time,
	resourcePlaceIDs, recordedDispatchWorkIDs map[string]struct{},
) (map[string]struct{}, error) {
	restoredForMarking := restoredWorldStateForMarking(cfg)
	restoredItems := restoredWorkItems(restoredForMarking)
	if !cfg.skipRestoredDispatchReconciliation {
		if err := materializeRestoredDispatchInputPlaces(restoredForMarking, cfg.net, restoredItems); err != nil {
			return nil, err
		}
	}
	recovery := classifyRestoredWorkRecovery(restoredForMarking, cfg.net, restoredItems)
	excludedWorkIDs := cloneRestoredWorkIDSet(recovery.excludedWorkIDs)
	if cfg.skipRestoredDispatchReconciliation {
		if excludedWorkIDs == nil {
			excludedWorkIDs = make(map[string]struct{}, len(recordedDispatchWorkIDs))
		}
		if recovery.toleratedWorkIDs == nil {
			recovery.toleratedWorkIDs = make(map[string]struct{}, len(recordedDispatchWorkIDs))
		}
		for workID := range recordedDispatchWorkIDs {
			excludedWorkIDs[workID] = struct{}{}
			recovery.toleratedWorkIDs[workID] = struct{}{}
		}
	}
	seededWorkIDs, err := seedRestoredWork(
		marking,
		cfg.net,
		restoredForMarking,
		constructionNow,
		resourcePlaceIDs,
		excludedWorkIDs,
		recovery.toleratedWorkIDs,
	)
	if err != nil {
		return nil, err
	}
	logRestoredWorkRecovery(cfg, recovery)
	return seededWorkIDs, nil
}

func classifyRestoredWorkRecovery(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
) restoredWorkRecovery {
	recovery := restoredWorkRecovery{}
	if restored == nil {
		return recovery
	}

	for workID, terminal := range restored.TerminalWorkByID {
		if !isRestoredCompletedAutomation(restored, net, items, workID, terminal.Status) {
			continue
		}
		// A completed cron Work may still be represented by the internal
		// pending-time place when its canonical occupancy was reconstructed
		// from completion facts. It is a historical scheduling marker, not a
		// new live trigger, so never seed it into the successor scheduler.
		recovery.excludedWorkIDs = addRestoredRecoveryWorkID(recovery.excludedWorkIDs, workID)
		if restoredWorkHasRecordedOccupancy(restored, workID) {
			continue
		}
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
	}

	for workID, active := range restored.ActiveWorkItemsByID {
		item := active
		if indexed, exists := items[workID]; exists {
			item = indexed
		}
		// System time Work has stricter historical recovery rules below. Keep
		// malformed or ambiguous automation history fail-closed rather than
		// treating its latest completion as ordinary consumed Work.
		if !interfaces.IsSystemTimeWorkType(item.WorkTypeID) && restoredWorkWasConsumed(restored, workID) {
			recovery.excludedWorkIDs = addRestoredRecoveryWorkID(recovery.excludedWorkIDs, workID)
			recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
			continue
		}
		if !isRestoredLegacyAutomation(restored, net, items, workID, active) {
			continue
		}
		recovery.excludedWorkIDs = addRestoredRecoveryWorkID(recovery.excludedWorkIDs, workID)
		recovery.toleratedWorkIDs = addRestoredRecoveryWorkID(recovery.toleratedWorkIDs, workID)
		recovery.legacyWorkIDs = addRestoredRecoveryWorkID(recovery.legacyWorkIDs, workID)
	}
	return recovery
}

func restoredWorkWasConsumed(restored *interfaces.FactoryWorldState, workID string) bool {
	if restored == nil || workID == "" || restoredWorkHasRecordedOccupancy(restored, workID) {
		return false
	}
	if restoredWorkInActiveDispatches(restored.ActiveDispatches, workID) ||
		restoredWorkInFailedDispatches(restored.FailedDispatches, workID) {
		return false
	}
	completion, found := latestRestoredCompletionForWork(restored.CompletedDispatches, workID)
	return found && restoredCompletionConsumesWork(completion, workID)
}

func restoredWorkInActiveDispatches(dispatches map[string]interfaces.FactoryWorldDispatch, workID string) bool {
	for _, dispatch := range dispatches {
		if restoredActiveDispatchContainsWork(dispatch, workID) {
			return true
		}
	}
	return false
}

func restoredWorkInFailedDispatches(completions []interfaces.FactoryWorldDispatchCompletion, workID string) bool {
	for _, completion := range completions {
		if restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			return true
		}
	}
	return false
}

func latestRestoredCompletionForWork(
	completions []interfaces.FactoryWorldDispatchCompletion,
	workID string,
) (interfaces.FactoryWorldDispatchCompletion, bool) {
	for index := len(completions) - 1; index >= 0; index-- {
		completion := completions[index]
		if restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			return completion, true
		}
	}
	return interfaces.FactoryWorldDispatchCompletion{}, false
}

func restoredCompletionConsumesWork(completion interfaces.FactoryWorldDispatchCompletion, workID string) bool {
	if strings.TrimSpace(completion.DispatchID) == "" || strings.TrimSpace(completion.TransitionID) == "" {
		return false
	}
	if !restoredCompletionHasConsumableOutcome(completion.Result.Outcome) {
		return false
	}
	if restoredOutputContainsWork(completion.OutputWorkItems, workID) {
		return false
	}
	return completion.TerminalWork == nil || completion.TerminalWork.WorkItem.ID != workID
}

func restoredCompletionHasConsumableOutcome(outcome string) bool {
	switch outcome {
	case string(workerexecution.OutcomeAccepted),
		string(workerexecution.OutcomeRejected),
		string(workerexecution.OutcomeContinue):
		return true
	default:
		return false
	}
}

func restoredOutputContainsWork(items []work.FactoryWorkItem, workID string) bool {
	for _, item := range items {
		if item.ID == workID {
			return true
		}
	}
	return false
}

func isRestoredCompletedAutomation(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	workID, status string,
) bool {
	item, exists := items[workID]
	return status == restoredCompletedAutomationStatus && workID != "" && exists &&
		isCronAutomationWork(item) && restoredCronWorkShapeIsCompatible(net, item) &&
		restoredWorkHasConclusiveCompletion(restored, workID)
}

func isRestoredLegacyAutomation(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	workID string,
	active work.FactoryWorkItem,
) bool {
	if workID == "" || restoredWorkHasRecordedOccupancy(restored, workID) {
		return false
	}
	if _, terminal := restored.TerminalWorkByID[workID]; terminal {
		return false
	}
	if _, failed := restored.FailedWorkItemsByID[workID]; failed {
		return false
	}
	item := active
	if indexed, exists := items[workID]; exists {
		item = indexed
	}
	return isCronAutomationWork(item) && restoredCronWorkShapeIsCompatible(net, item) &&
		restoredWorkHasConclusiveCompletion(restored, workID)
}

func restoredWorkHasRecordedOccupancy(restored *interfaces.FactoryWorldState, workID string) bool {
	if restored == nil || workID == "" {
		return false
	}
	for _, occupancy := range restored.PlaceOccupancyByID {
		for _, occupiedWorkID := range occupancy.WorkItemIDs {
			if occupiedWorkID == workID {
				return true
			}
		}
	}
	return false
}

func addRestoredRecoveryWorkID(ids map[string]struct{}, workID string) map[string]struct{} {
	if ids == nil {
		ids = make(map[string]struct{})
	}
	ids[workID] = struct{}{}
	return ids
}

func cloneRestoredWorkIDSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]struct{}, len(source))
	for workID := range source {
		clone[workID] = struct{}{}
	}
	return clone
}

func restoredRetryExhaustedPlacements(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
) map[string]string {
	if restored == nil || net == nil {
		return nil
	}
	recordedWorkstations := restoredFactoryWorkstations(restored.Factory)
	if len(recordedWorkstations) == 0 {
		return nil
	}
	activeWorkIDs := restoredActiveWorkIDs(restored)
	strikes := make(map[string]map[string]int)
	exhausted := make(map[string]string)
	latestMoves := latestRestoredWorkStateChanges(restored.WorkStateChangesByWorkID)
	for _, completion := range restored.CompletedDispatches {
		recordRestoredRetryCompletion(restored, net, items, recordedWorkstations, activeWorkIDs, strikes, exhausted, latestMoves, completion)
	}
	if len(exhausted) == 0 {
		return nil
	}
	return exhausted
}

func restoredActiveWorkIDs(restored *interfaces.FactoryWorldState) map[string]struct{} {
	workIDs := make(map[string]struct{})
	if restored == nil {
		return workIDs
	}
	for _, dispatch := range restored.ActiveDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
		for _, input := range dispatch.Inputs {
			if workID, ok := restoredDispatchWorkID(input); ok {
				addHistoricalWorkID(workIDs, workID)
			}
		}
	}
	return workIDs
}

func recordRestoredRetryCompletion(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
	recordedWorkstations map[string]*interfaces.FactoryWorkstationConfig,
	activeWorkIDs map[string]struct{},
	strikes map[string]map[string]int,
	exhausted map[string]string,
	latestMoves map[string]interfaces.FactoryWorldWorkStateChangeRecord,
	completion interfaces.FactoryWorldDispatchCompletion,
) {
	workID, ok := restoredRetryCompletionWorkID(completion, latestMoves)
	if !ok || restoredWorkIDIsActive(activeWorkIDs, workID) {
		return
	}
	maxRetries, ok := restoredRetryLimit(net, recordedWorkstations, completion)
	if !ok {
		return
	}
	if strikes[workID] == nil {
		strikes[workID] = make(map[string]int)
	}
	if completion.Result.Outcome != string(workerexecution.OutcomeFailed) {
		strikes[workID][completion.TransitionID] = 0
		delete(exhausted, workID)
		return
	}
	strikes[workID][completion.TransitionID]++
	if !restoredRetryIsExhausted(restored, completion, workID, strikes[workID][completion.TransitionID], maxRetries) {
		return
	}
	if failedPlaceID := restoredFailedPlaceID(net, items[workID].WorkTypeID); failedPlaceID != "" {
		exhausted[workID] = failedPlaceID
	}
}

func restoredRetryCompletionWorkID(
	completion interfaces.FactoryWorldDispatchCompletion,
	latestMoves map[string]interfaces.FactoryWorldWorkStateChangeRecord,
) (string, bool) {
	workID := firstRestoredCompletionWorkID(completion)
	return workID, workID != "" && completionFollowsRestoredMove(completion, latestMoves[workID])
}

func restoredWorkIDIsActive(activeWorkIDs map[string]struct{}, workID string) bool {
	_, active := activeWorkIDs[workID]
	return active
}

func restoredRetryLimit(
	net *state.Net,
	recordedWorkstations map[string]*interfaces.FactoryWorkstationConfig,
	completion interfaces.FactoryWorldDispatchCompletion,
) (int, bool) {
	transition := net.Transitions[completion.TransitionID]
	if transition == nil {
		return 0, false
	}
	maxRetries := subsystems.EffectiveWorkstationMaxRetries(recordedWorkstations[transition.Name])
	return maxRetries, maxRetries > 0
}

func restoredRetryIsExhausted(
	restored *interfaces.FactoryWorldState,
	completion interfaces.FactoryWorldDispatchCompletion,
	workID string,
	strikes, maxRetries int,
) bool {
	if strikes < maxRetries || !completionRoutesRestoredWork(completion, workID) {
		return false
	}
	return restored.FailureDetailsByWorkID[workID].DispatchID == completion.DispatchID
}

func restoredFactoryWorkstations(snapshot *interfaces.FactorySnapshot) map[string]*interfaces.FactoryWorkstationConfig {
	if snapshot == nil {
		return nil
	}
	var recorded struct {
		Workstations []interfaces.FactoryWorkstationConfig `json:"workstations"`
	}
	if err := snapshot.Decode(&recorded); err != nil {
		return nil
	}
	workstations := make(map[string]*interfaces.FactoryWorkstationConfig, len(recorded.Workstations))
	for index := range recorded.Workstations {
		workstation := &recorded.Workstations[index]
		if name := strings.TrimSpace(workstation.Name); name != "" {
			workstations[name] = workstation
		}
	}
	return workstations
}

func latestRestoredWorkStateChanges(
	changes map[string][]interfaces.FactoryWorldWorkStateChangeRecord,
) map[string]interfaces.FactoryWorldWorkStateChangeRecord {
	latest := make(map[string]interfaces.FactoryWorldWorkStateChangeRecord, len(changes))
	for workID, records := range changes {
		for _, record := range records {
			current, exists := latest[workID]
			if !exists || record.Sequence > current.Sequence ||
				(record.Sequence == current.Sequence && record.EventTime.After(current.EventTime)) {
				latest[workID] = record
			}
		}
	}
	return latest
}

func completionFollowsRestoredMove(
	completion interfaces.FactoryWorldDispatchCompletion,
	move interfaces.FactoryWorldWorkStateChangeRecord,
) bool {
	if move.WorkID == "" {
		return true
	}
	if !completion.CompletedAt.IsZero() && !move.EventTime.IsZero() {
		return completion.CompletedAt.After(move.EventTime)
	}
	return completion.CompletedTick > move.Tick
}

func firstRestoredCompletionWorkID(completion interfaces.FactoryWorldDispatchCompletion) string {
	for _, workID := range completion.WorkItemIDs {
		if workID = strings.TrimSpace(workID); workID != "" {
			return workID
		}
	}
	return ""
}

func completionRoutesRestoredWork(completion interfaces.FactoryWorldDispatchCompletion, workID string) bool {
	for _, item := range completion.OutputWorkItems {
		if item.ID == workID {
			return true
		}
	}
	return false
}

func restoredFailedPlaceID(net *state.Net, workTypeID string) string {
	workType := net.WorkTypes[workTypeID]
	if workType == nil {
		return ""
	}
	for _, definition := range workType.States {
		if definition.Category == state.StateCategoryFailed {
			return state.PlaceID(workTypeID, definition.Value)
		}
	}
	return ""
}

func applyRestoredPlacementStates(
	items map[string]work.FactoryWorkItem,
	net *state.Net,
	placements map[string]string,
) {
	for workID, placeID := range placements {
		item, exists := items[workID]
		place := net.Places[placeID]
		if !exists || place == nil {
			continue
		}
		item.State = place.State
		items[workID] = item
	}
}

func cloneRestoredPlacements(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for workID, placeID := range source {
		clone[workID] = placeID
	}
	return clone
}

func addRestoredOccupancyPlacements(
	placements map[string]string,
	occupancy map[string]interfaces.FactoryPlaceOccupancy,
	authoritativePlacements map[string]string,
) error {
	for _, placeID := range sortedRestoredKeys(occupancy) {
		workIDs := append([]string(nil), occupancy[placeID].WorkItemIDs...)
		sort.Strings(workIDs)
		for _, workID := range workIDs {
			if _, overridden := authoritativePlacements[workID]; overridden {
				continue
			}
			if err := addRestoredPlacement(placements, workID, placeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func addRestoredDispatchPlacements(
	placements map[string]string,
	dispatches map[string]interfaces.FactoryWorldDispatch,
) error {
	for _, dispatchID := range sortedRestoredDispatchIDs(dispatches) {
		for _, input := range dispatches[dispatchID].Inputs {
			workID, ok := restoredDispatchWorkID(input)
			if !ok || input.PlaceID == "" {
				continue
			}
			if err := addRestoredPlacement(placements, workID, input.PlaceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoredCompletedDispatchPlacements(
	completions []interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) (map[string]string, error) {
	latest := make(map[string]map[string]struct{})
	for _, completion := range completions {
		current := make(map[string]map[string]struct{})
		touched := make(map[string]struct{}, len(completion.WorkItemIDs))
		for _, workID := range completion.WorkItemIDs {
			if workID != "" {
				touched[workID] = struct{}{}
			}
		}
		for _, item := range completion.OutputWorkItems {
			if item.ID != "" {
				touched[item.ID] = struct{}{}
			}
			addRestoredItemPlacementCandidate(current, item, items)
		}
		if completion.TerminalWork != nil {
			if completion.TerminalWork.WorkItem.ID != "" {
				touched[completion.TerminalWork.WorkItem.ID] = struct{}{}
			}
			addRestoredItemPlacementCandidate(current, completion.TerminalWork.WorkItem, items)
		}
		for workID := range touched {
			latest[workID] = current[workID]
		}
	}
	for _, workID := range sortedRestoredKeys(latest) {
		placeIDs := sortedStringKeys(latest[workID])
		if len(placeIDs) > 1 {
			return nil, fmt.Errorf("restore Work board: Work %q has conflicting completed-dispatch places %q", workID, placeIDs)
		}
		if len(placeIDs) == 1 {
			latest[workID] = map[string]struct{}{placeIDs[0]: {}}
		}
	}
	placements := make(map[string]string, len(latest))
	for workID, placeIDs := range latest {
		for placeID := range placeIDs {
			placements[workID] = placeID
		}
	}
	return placements, nil
}

func addRestoredWorkStateChangePlacements(
	placements map[string]string,
	changes map[string][]interfaces.FactoryWorldWorkStateChangeRecord,
) error {
	for _, workID := range sortedRestoredKeys(changes) {
		if _, hasCurrentPlacement := placements[workID]; hasCurrentPlacement {
			continue
		}
		records := changes[workID]
		for index := len(records) - 1; index >= 0; index-- {
			placeID := strings.TrimSpace(records[index].ToPlaceID)
			if placeID == "" {
				continue
			}
			if err := addRestoredPlacement(placements, workID, placeID); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func addRestoredItemPlacementCandidate(
	candidates map[string]map[string]struct{},
	item work.FactoryWorkItem,
	items map[string]work.FactoryWorkItem,
) {
	if item.ID == "" {
		return
	}
	if indexed, ok := items[item.ID]; ok {
		if item.WorkTypeID == "" {
			item.WorkTypeID = indexed.WorkTypeID
		}
		if item.State == "" {
			item.State = indexed.State
		}
	}
	placeID := state.PlaceID(item.WorkTypeID, item.State)
	if placeID == "" {
		return
	}
	if candidates[item.ID] == nil {
		candidates[item.ID] = make(map[string]struct{})
	}
	candidates[item.ID][placeID] = struct{}{}
}

func addRestoredItemPlacements(placements map[string]string, items map[string]work.FactoryWorkItem) error {
	for _, workID := range sortedRestoredKeys(items) {
		item := items[workID]
		if err := addRestoredPlacement(placements, workID, state.PlaceID(item.WorkTypeID, item.State)); err != nil {
			return err
		}
	}
	return nil
}

func addRestoredPlacement(placements map[string]string, workID, placeID string) error {
	if workID == "" || placeID == "" {
		return nil
	}
	if existing, exists := placements[workID]; exists {
		if existing != placeID {
			return fmt.Errorf("restore Work board: Work %q has conflicting current places %q and %q", workID, existing, placeID)
		}
		return nil
	}
	placements[workID] = placeID
	return nil
}

// addRestoredMissingInitialWorkPlacements recovers only an active Work whose
// durable admission and recorded definition prove one exact initial place.
// The placement is added to the detached in-memory projection; no recording or
// canonical event is changed by this compatibility path.
func addRestoredMissingInitialWorkPlacements(
	placements map[string]string,
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	resourcePlaceIDs map[string]struct{},
	toleratedWorkIDs map[string]struct{},
) error {
	if restored == nil || restored.PlaceOccupancyByID == nil || net == nil {
		return nil
	}
	for _, workID := range sortedRestoredKeys(restored.ActiveWorkItemsByID) {
		if _, exists := placements[workID]; exists {
			continue
		}
		if _, tolerated := toleratedWorkIDs[workID]; tolerated {
			continue
		}
		placeID, ok := restoredMissingInitialWorkPlace(restored, net, resourcePlaceIDs, workID)
		if !ok {
			return fmt.Errorf(
				"restore Work board: active Work %q has no current place occupancy",
				workID,
			)
		}
		placements[workID] = placeID
	}
	return nil
}

func restoredMissingInitialWorkPlace(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	resourcePlaceIDs map[string]struct{},
	workID string,
) (string, bool) {
	if strings.TrimSpace(workID) == "" {
		return "", false
	}
	active, activeOK := restored.ActiveWorkItemsByID[workID]
	canonical, canonicalOK := restored.WorkItemsByID[workID]
	admission, admissionOK := restoredUniqueAdmissionForWork(restored, workID)
	if !activeOK || !canonicalOK || !admissionOK ||
		active.ID != workID || canonical.ID != workID || admission.ID != workID {
		return "", false
	}
	if admission.WorkTypeID == "" || active.WorkTypeID != admission.WorkTypeID || canonical.WorkTypeID != admission.WorkTypeID {
		return "", false
	}
	initialPlaceID, initialState, ok := restoredUniqueInitialPlace(restored, net, resourcePlaceIDs, admission.WorkTypeID)
	if !ok || !restoredWorkStatesMatchInitial(initialState, admission.State, active.State, canonical.State) {
		return "", false
	}
	if restoredMissingInitialWorkHasLaterFacts(restored, workID) {
		return "", false
	}
	return initialPlaceID, true
}

func restoredUniqueAdmissionForWork(
	restored *interfaces.FactoryWorldState,
	workID string,
) (work.FactoryWorkItem, bool) {
	var admission work.FactoryWorkItem
	count := 0
	for requestKey, request := range restored.WorkRequestsByID {
		for _, item := range request.WorkItems {
			if item.ID != workID {
				continue
			}
			count++
			if count == 1 {
				admission = item
			}
			requestID := strings.TrimSpace(request.RequestID)
			if requestID == "" {
				requestID = strings.TrimSpace(requestKey)
			}
			if requestID == "" {
				return work.FactoryWorkItem{}, false
			}
		}
	}
	return admission, count == 1
}

func restoredUniqueInitialPlace(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	resourcePlaceIDs map[string]struct{},
	workTypeID string,
) (string, string, bool) {
	initialStates := make([]string, 0, 1)
	workTypeCount := 0
	for _, workType := range restored.Topology.WorkTypes {
		if workType.ID != workTypeID {
			continue
		}
		workTypeCount++
		for _, stateDefinition := range workType.States {
			if stateDefinition.Category == string(state.StateCategoryInitial) {
				initialStates = append(initialStates, stateDefinition.Value)
			}
		}
	}
	if workTypeCount != 1 || len(initialStates) != 1 || strings.TrimSpace(initialStates[0]) == "" {
		return "", "", false
	}
	initialState := initialStates[0]
	expectedPlaceID := state.PlaceID(workTypeID, initialState)
	recordedPlaceIDs := make([]string, 0, 1)
	for _, place := range restored.Topology.Places {
		if place.TypeID != workTypeID || place.State != initialState ||
			place.Category != "" && place.Category != string(state.StateCategoryInitial) {
			continue
		}
		if _, isResource := resourcePlaceIDs[place.ID]; isResource {
			continue
		}
		recordedPlaceIDs = append(recordedPlaceIDs, place.ID)
	}
	if len(recordedPlaceIDs) != 1 || recordedPlaceIDs[0] != expectedPlaceID {
		return "", "", false
	}

	currentPlace, exists := net.Places[expectedPlaceID]
	if !exists || currentPlace == nil || currentPlace.TypeID != workTypeID || currentPlace.State != initialState {
		return "", "", false
	}
	currentPlaceCount := 0
	for placeID, place := range net.Places {
		if place == nil || place.TypeID != workTypeID || place.State != initialState {
			continue
		}
		if _, isResource := resourcePlaceIDs[placeID]; !isResource {
			currentPlaceCount++
		}
	}
	return expectedPlaceID, initialState, currentPlaceCount == 1
}

func restoredWorkStatesMatchInitial(initialState string, states ...string) bool {
	for _, currentState := range states {
		if currentState != "" && currentState != initialState {
			return false
		}
	}
	return true
}

func restoredMissingInitialWorkHasLaterFacts(restored *interfaces.FactoryWorldState, workID string) bool {
	if len(restored.WorkStateChangesByWorkID[workID]) > 0 ||
		restoredWorkInActiveDispatches(restored.ActiveDispatches, workID) ||
		restoredDispatchCompletionsContainWork(restored.CompletedDispatches, workID) ||
		restoredDispatchCompletionsContainWork(restored.FailedDispatches, workID) {
		return true
	}
	for indexedWorkID, terminal := range restored.TerminalWorkByID {
		if indexedWorkID == workID || terminal.WorkItem.ID == workID {
			return true
		}
	}
	for indexedWorkID, failedWork := range restored.FailedWorkItemsByID {
		if indexedWorkID == workID || failedWork.ID == workID {
			return true
		}
	}
	if _, failure := restored.FailureDetailsByWorkID[workID]; failure {
		return true
	}
	if restoredWorkHasRecordedOccupancy(restored, workID) {
		return true
	}
	for _, approval := range restored.PendingHumanApprovalsByID {
		if restoredDispatchContainsWork(approval.WorkItemIDs, workID) {
			return true
		}
		if approval.DispatchID == "" {
			continue
		}
		if dispatch, exists := restored.ActiveDispatches[approval.DispatchID]; exists && restoredActiveDispatchContainsWork(dispatch, workID) {
			return true
		}
	}
	return false
}

func restoredDispatchCompletionsContainWork(
	completions []interfaces.FactoryWorldDispatchCompletion,
	workID string,
) bool {
	for _, completion := range completions {
		if restoredDispatchContainsWork(completion.WorkItemIDs, workID) ||
			restoredDispatchInputsContainWork(completion.ConsumedInputs, workID) ||
			restoredWorkItemsContainWork(completion.InputWorkItems, workID) ||
			restoredWorkItemsContainWork(completion.OutputWorkItems, workID) ||
			completion.TerminalWork != nil && completion.TerminalWork.WorkItem.ID == workID {
			return true
		}
	}
	return false
}

func restoredDispatchInputsContainWork(inputs []interfaces.WorkstationInput, workID string) bool {
	for _, input := range inputs {
		if input.TokenID == workID || input.WorkItem != nil && input.WorkItem.ID == workID {
			return true
		}
	}
	return false
}

func restoredWorkItemsContainWork(items []work.FactoryWorkItem, workID string) bool {
	for _, item := range items {
		if item.ID == workID {
			return true
		}
	}
	return false
}

func sortedRestoredKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isCronAutomationWork(item work.FactoryWorkItem) bool {
	return interfaces.IsSystemTimeWorkType(item.WorkTypeID) &&
		item.Tags[interfaces.TimeWorkTagKeySource] == interfaces.TimeWorkSourceCron &&
		strings.TrimSpace(item.Tags[interfaces.TimeWorkTagKeyCronWorkstation]) != ""
}

func restoredCronWorkShapeIsCompatible(net *state.Net, item work.FactoryWorkItem) bool {
	if net == nil || item.State != interfaces.SystemTimePendingState {
		return false
	}
	place := net.Places[interfaces.SystemTimePendingPlaceID]
	return place != nil && place.TypeID == interfaces.SystemTimeWorkTypeID && place.State == interfaces.SystemTimePendingState
}

func restoredWorkHasConclusiveCompletion(restored *interfaces.FactoryWorldState, workID string) bool {
	if restored == nil || workID == "" {
		return false
	}
	completed := 0
	for _, completion := range restored.CompletedDispatches {
		if !restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			continue
		}
		if completion.DispatchID == "" || completion.Result.Outcome != string(workerexecution.OutcomeAccepted) {
			return false
		}
		completed++
	}
	if completed != 1 {
		return false
	}
	for _, completion := range restored.FailedDispatches {
		if restoredDispatchContainsWork(completion.WorkItemIDs, workID) {
			return false
		}
	}
	for _, dispatch := range restored.ActiveDispatches {
		if restoredActiveDispatchContainsWork(dispatch, workID) {
			return false
		}
	}
	return true
}

func restoredActiveDispatchContainsWork(dispatch interfaces.FactoryWorldDispatch, workID string) bool {
	if restoredDispatchContainsWork(dispatch.WorkItemIDs, workID) {
		return true
	}
	for _, input := range dispatch.Inputs {
		if inputWorkID, ok := restoredDispatchWorkID(input); ok && inputWorkID == workID {
			return true
		}
	}
	return false
}

func restoredDispatchContainsWork(workIDs []string, workID string) bool {
	for _, candidate := range workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func logRestoredWorkRecovery(cfg *runtimeConfig, recovery restoredWorkRecovery) {
	if cfg == nil || len(recovery.legacyWorkIDs) == 0 {
		return
	}
	workIDs := make([]string, 0, len(recovery.legacyWorkIDs))
	for workID := range recovery.legacyWorkIDs {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	logger := logging.EnsureLogger(cfg.logger)
	for _, workID := range workIDs {
		logger.Warn(
			"restore Factory Runtime Work board: skipping completed automation Work without recoverable place",
			"session_id", sessionIDFromFactoryConfig(cfg),
			"recording_id", strings.TrimSpace(cfg.recordingID),
			"work_id", workID,
			"disposition", restoredLegacyAutomationDisposition,
		)
	}
}

func validateRestoredDispatchWorkReferences(
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
) error {
	if restored == nil {
		return nil
	}
	activeDispatchIDs := sortedRestoredDispatchIDs(restored.ActiveDispatches)
	for _, dispatchID := range activeDispatchIDs {
		dispatch := restored.ActiveDispatches[dispatchID]
		if err := validateRestoredActiveDispatch(dispatchID, dispatch, items); err != nil {
			return err
		}
	}
	for index, completion := range restored.CompletedDispatches {
		if err := validateRestoredCompletedDispatch(index, completion, items); err != nil {
			return err
		}
	}
	for index, completion := range restored.FailedDispatches {
		if err := validateRestoredFailedDispatch(index, completion, items); err != nil {
			return err
		}
	}
	approvalIDs := sortedRestoredApprovalIDs(restored.PendingHumanApprovalsByID)
	for _, approvalID := range approvalIDs {
		approval := restored.PendingHumanApprovalsByID[approvalID]
		if err := validateRestoredWorkReferences("pending approval", approvalID, approval.WorkItemIDs, items); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredActiveDispatch(
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("active dispatch", dispatchID, dispatch.WorkItemIDs, items); err != nil {
		return err
	}
	for _, input := range dispatch.Inputs {
		workID, ok := restoredDispatchWorkID(input)
		if !ok || workID == "" {
			continue
		}
		if err := validateRestoredWorkReferences("active dispatch", dispatchID, []string{workID}, items); err != nil {
			return err
		}
	}
	return nil
}

func validateRestoredCompletedDispatch(
	index int,
	completion interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("completed dispatch", completion.DispatchID, completion.WorkItemIDs, items); err != nil {
		return err
	}
	if err := validateRestoredWorkItems("completed dispatch", completion.DispatchID, completion.InputWorkItems, items); err != nil {
		return err
	}
	if err := validateRestoredWorkItems("completed dispatch", completion.DispatchID, completion.OutputWorkItems, items); err != nil {
		return err
	}
	if completion.DispatchID == "" {
		return fmt.Errorf("restore Work board: completed dispatch at index %d has no dispatch identity", index)
	}
	return nil
}

func validateRestoredFailedDispatch(
	index int,
	completion interfaces.FactoryWorldDispatchCompletion,
	items map[string]work.FactoryWorkItem,
) error {
	if err := validateRestoredWorkReferences("failed dispatch", completion.DispatchID, completion.WorkItemIDs, items); err != nil {
		return err
	}
	if completion.DispatchID == "" {
		return fmt.Errorf("restore Work board: failed dispatch at index %d has no dispatch identity", index)
	}
	return nil
}

func validateRestoredWorkReferences(
	category string,
	identity string,
	workIDs []string,
	items map[string]work.FactoryWorkItem,
) error {
	for _, workID := range workIDs {
		if workID == "" {
			continue
		}
		if _, exists := items[workID]; !exists {
			return fmt.Errorf("restore Work board: %s %q references unknown Work %q", category, identity, workID)
		}
	}
	return nil
}

func validateRestoredWorkItems(
	category string,
	identity string,
	itemsToValidate []work.FactoryWorkItem,
	items map[string]work.FactoryWorkItem,
) error {
	for _, item := range itemsToValidate {
		if item.ID == "" {
			continue
		}
		if err := validateRestoredWorkReferences(category, identity, []string{item.ID}, items); err != nil {
			return err
		}
	}
	return nil
}

func sortedRestoredDispatchIDs(dispatches map[string]interfaces.FactoryWorldDispatch) []string {
	ids := make([]string, 0, len(dispatches))
	for dispatchID := range dispatches {
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	return ids
}

func sortedRestoredApprovalIDs(approvals map[string]interfaces.FactoryWorldHumanApproval) []string {
	ids := make([]string, 0, len(approvals))
	for approvalID := range approvals {
		ids = append(ids, approvalID)
	}
	sort.Strings(ids)
	return ids
}
