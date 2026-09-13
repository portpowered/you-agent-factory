package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type restoredDispatchPlaceFailure string

const (
	restoredDispatchPlaceMissing   restoredDispatchPlaceFailure = "missing"
	restoredDispatchPlaceAmbiguous restoredDispatchPlaceFailure = "ambiguous"
)

// restoredDispatchPlaceError reports why a legacy Work-bearing dispatch input
// could not be assigned one exact logical place during detached restoration.
// It is intentionally private: the public event and Runtime contracts remain
// unchanged while callers inside this package can distinguish fail-closed
// recovery from ordinary validation failures.
type restoredDispatchPlaceError struct {
	Reason       restoredDispatchPlaceFailure
	DispatchID   string
	WorkID       string
	TransitionID string
	Candidates   []string
}

func (e *restoredDispatchPlaceError) Error() string {
	if e == nil {
		return "restore Work board: unable to resolve a legacy dispatch input place"
	}
	candidates := "none"
	if len(e.Candidates) > 0 {
		candidates = fmt.Sprintf("%q", e.Candidates)
	}
	return fmt.Sprintf(
		"restore Work board: active dispatch %q Work %q transition %q has %s canonical logical place candidates: %s",
		e.DispatchID,
		e.WorkID,
		e.TransitionID,
		e.Reason,
		candidates,
	)
}

// restoredWorldStateForMarking detaches replayed dispatch claims from the
// initial board. Deterministic replay re-materializes those claims from the
// recorded event stream; carrying an active projection into the seed would
// duplicate the dispatch and can conflict with a later recorded occupancy
// when a legacy artifact spans a logical-clock restart. The original state is
// retained on runtimeConfig for replay identity resolution and event history.
func restoredWorldStateForMarking(cfg *runtimeConfig) *interfaces.FactoryWorldState {
	if cfg == nil || cfg.restoredWorldState == nil || !cfg.skipRestoredDispatchReconciliation {
		if cfg == nil {
			return nil
		}
		return cfg.restoredWorldState
	}
	restored := *cfg.restoredWorldState
	restored.ActiveDispatches = nil
	return &restored
}

// materializeRestoredDispatchInputPlaces resolves only missing Work input
// places. Candidate calculation is completed for every active dispatch before
// any detached projection is changed, so a zero or ambiguous candidate cannot
// expose a partially restored board.
func materializeRestoredDispatchInputPlaces(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	items map[string]work.FactoryWorkItem,
) error {
	if restored == nil || net == nil {
		return nil
	}

	resolutions := make(map[string]map[int]string)
	for _, dispatchID := range sortedRestoredDispatchIDs(restored.ActiveDispatches) {
		dispatch := restored.ActiveDispatches[dispatchID]
		for inputIndex, input := range dispatch.Inputs {
			if input.PlaceID != "" {
				continue
			}
			workID, ok := restoredDispatchWorkID(input)
			if !ok || workID == "" {
				continue
			}
			candidates := restoredDispatchPlaceCandidates(restored, net, dispatch, workID, items)
			if len(candidates) != 1 {
				reason := restoredDispatchPlaceMissing
				if len(candidates) > 1 {
					reason = restoredDispatchPlaceAmbiguous
				}
				return &restoredDispatchPlaceError{
					Reason:       reason,
					DispatchID:   dispatchID,
					WorkID:       workID,
					TransitionID: dispatch.TransitionID,
					Candidates:   append([]string(nil), candidates...),
				}
			}
			if resolutions[dispatchID] == nil {
				resolutions[dispatchID] = make(map[int]string)
			}
			resolutions[dispatchID][inputIndex] = candidates[0]
		}
	}

	for dispatchID, inputPlaces := range resolutions {
		dispatch := restored.ActiveDispatches[dispatchID]
		for inputIndex, placeID := range inputPlaces {
			dispatch.Inputs[inputIndex].PlaceID = placeID
		}
		restored.ActiveDispatches[dispatchID] = dispatch
	}
	return nil
}

func restoredDispatchPlaceCandidates(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	dispatch interfaces.FactoryWorldDispatch,
	workID string,
	items map[string]work.FactoryWorkItem,
) []string {
	item, candidates := restoredDispatchTransitionCandidates(net, dispatch, workID, items)
	if len(candidates) == 0 {
		return nil
	}
	candidates = constrainRestoredDispatchByTopology(restored, net, dispatch, item, candidates)
	if len(candidates) == 0 {
		return nil
	}
	candidates = constrainRestoredDispatchByLatestMove(restored, net, item, workID, candidates)
	if len(candidates) == 0 {
		return nil
	}
	return constrainRestoredDispatchByRecordedInputs(restored, net, dispatch, item, workID, candidates)
}

func restoredDispatchTransitionCandidates(
	net *state.Net,
	dispatch interfaces.FactoryWorldDispatch,
	workID string,
	items map[string]work.FactoryWorkItem,
) (work.FactoryWorkItem, []string) {
	item, ok := items[workID]
	if !ok || item.ID != workID || strings.TrimSpace(item.WorkTypeID) == "" || strings.TrimSpace(item.State) == "" ||
		strings.TrimSpace(dispatch.TransitionID) == "" {
		return work.FactoryWorkItem{}, nil
	}
	transition, ok := net.Transitions[dispatch.TransitionID]
	if !ok || transition == nil {
		return work.FactoryWorkItem{}, nil
	}
	canonicalPlaces := restoredCanonicalWorkStatePlaces(net, item)
	loadedArcPlaces := restoredWorkTypeDispatchPlaces(net, item, transitionInputPlaceIDs(transition.InputArcs))
	if len(loadedArcPlaces) == 0 {
		return work.FactoryWorkItem{}, nil
	}
	// A legacy active dispatch can survive a later canonical Work mutation in
	// the detached history. In that case the current Work state no longer
	// names the consumed input place, but the loaded transition arc still does.
	// Keep the canonical state as the primary constraint whenever it agrees;
	// use the authored arc as the bounded historical fallback when it does not.
	candidates := append([]string(nil), loadedArcPlaces...)
	if len(canonicalPlaces) > 0 {
		if canonicalCandidates := intersectRestoredDispatchPlaceIDs(canonicalPlaces, loadedArcPlaces); len(canonicalCandidates) > 0 {
			candidates = canonicalCandidates
		}
	}
	return item, candidates
}

func constrainRestoredDispatchByTopology(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	dispatch interfaces.FactoryWorldDispatch,
	item work.FactoryWorkItem,
	candidates []string,
) []string {
	if len(restored.Topology.Workstations) == 0 {
		return candidates
	}
	recordedPlaces, found := restoredRecordedWorkstationPlaces(restored.Topology, dispatch.TransitionID)
	recordedCandidates := restoredWorkTypeDispatchPlaces(net, item, recordedPlaces)
	if !found || len(recordedCandidates) == 0 {
		return candidates
	}
	return intersectRestoredDispatchPlaceIDs(candidates, recordedCandidates)
}

func constrainRestoredDispatchByLatestMove(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	item work.FactoryWorkItem,
	workID string,
	candidates []string,
) []string {
	placeID, found := latestRestoredNonEmptyWorkStateChangePlace(restored, workID)
	if !found {
		return candidates
	}
	return intersectRestoredDispatchPlaceIDs(
		candidates,
		restoredWorkTypeDispatchPlaces(net, item, []string{placeID}),
	)
}

func constrainRestoredDispatchByRecordedInputs(
	restored *interfaces.FactoryWorldState,
	net *state.Net,
	dispatch interfaces.FactoryWorldDispatch,
	item work.FactoryWorkItem,
	workID string,
	candidates []string,
) []string {
	recordedPlaces, found := restoredRecordedExactWorkstationInputPlaces(restored, dispatch.TransitionID, workID)
	if !found {
		return candidates
	}
	return intersectRestoredDispatchPlaceIDs(candidates, restoredWorkTypeDispatchPlaces(net, item, recordedPlaces))
}

func transitionInputPlaceIDs(arcs []petri.Arc) []string {
	placeIDs := make([]string, 0, len(arcs))
	for _, arc := range arcs {
		if placeID := strings.TrimSpace(arc.PlaceID); placeID != "" {
			placeIDs = append(placeIDs, placeID)
		}
	}
	return placeIDs
}

func restoredRecordedWorkstationPlaces(
	topology interfaces.InitialStructurePayload,
	transitionID string,
) ([]string, bool) {
	var placeIDs []string
	found := false
	for _, workstation := range topology.Workstations {
		if workstation.ID != transitionID {
			continue
		}
		found = true
		placeIDs = append(placeIDs, workstation.InputPlaceIDs...)
	}
	return placeIDs, found
}

func restoredRecordedExactWorkstationInputPlaces(
	restored *interfaces.FactoryWorldState,
	transitionID string,
	workID string,
) ([]string, bool) {
	placeIDs := make([]string, 0)
	appendInputs := func(inputs []interfaces.WorkstationInput) {
		for _, input := range inputs {
			inputWorkID, ok := restoredDispatchWorkID(input)
			if !ok || inputWorkID != workID || strings.TrimSpace(input.PlaceID) == "" {
				continue
			}
			placeIDs = append(placeIDs, input.PlaceID)
		}
	}
	for _, dispatch := range restored.ActiveDispatches {
		if dispatch.TransitionID == transitionID {
			appendInputs(dispatch.Inputs)
		}
	}
	for _, completion := range restored.CompletedDispatches {
		if completion.TransitionID == transitionID {
			appendInputs(completion.ConsumedInputs)
		}
	}
	for _, completion := range restored.FailedDispatches {
		if completion.TransitionID == transitionID {
			appendInputs(completion.ConsumedInputs)
		}
	}
	for _, session := range restored.ProviderSessions {
		if session.TransitionID == transitionID {
			appendInputs(session.ConsumedInputs)
		}
	}
	return placeIDs, len(placeIDs) > 0
}

func latestRestoredNonEmptyWorkStateChangePlace(
	restored *interfaces.FactoryWorldState,
	workID string,
) (string, bool) {
	if restored == nil {
		return "", false
	}
	records := restored.WorkStateChangesByWorkID[workID]
	latestIndex := -1
	var latest interfaces.FactoryWorldWorkStateChangeRecord
	for index, record := range records {
		placeID := strings.TrimSpace(record.ToPlaceID)
		if placeID == "" {
			continue
		}
		if latestIndex < 0 || restoredWorkStateChangeIsLater(record, latest, index, latestIndex) {
			latest = record
			latestIndex = index
		}
	}
	if latestIndex < 0 {
		return "", false
	}
	return strings.TrimSpace(latest.ToPlaceID), true
}

func restoredWorkStateChangeIsLater(
	candidate interfaces.FactoryWorldWorkStateChangeRecord,
	current interfaces.FactoryWorldWorkStateChangeRecord,
	candidateIndex int,
	currentIndex int,
) bool {
	if candidate.Sequence != current.Sequence {
		return candidate.Sequence > current.Sequence
	}
	if !candidate.EventTime.Equal(current.EventTime) {
		return candidate.EventTime.After(current.EventTime)
	}
	if candidate.Tick != current.Tick {
		return candidate.Tick > current.Tick
	}
	return candidateIndex > currentIndex
}

func restoredCanonicalWorkStatePlaces(net *state.Net, item work.FactoryWorkItem) []string {
	placeIDs := make([]string, 0)
	for placeID, place := range net.Places {
		if place == nil || place.TypeID != item.WorkTypeID || place.State != item.State {
			continue
		}
		placeIDs = append(placeIDs, placeID)
	}
	return sortedUniqueRestoredDispatchPlaceIDs(placeIDs)
}

func restoredWorkTypeDispatchPlaces(
	net *state.Net,
	item work.FactoryWorkItem,
	placeIDs []string,
) []string {
	unique := make(map[string]struct{}, len(placeIDs))
	for _, placeID := range placeIDs {
		placeID = strings.TrimSpace(placeID)
		place, ok := net.Places[placeID]
		if !ok || place == nil || place.TypeID != item.WorkTypeID {
			continue
		}
		unique[placeID] = struct{}{}
	}
	return sortedRestoredKeys(unique)
}

func sortedUniqueRestoredDispatchPlaceIDs(placeIDs []string) []string {
	unique := make(map[string]struct{}, len(placeIDs))
	for _, placeID := range placeIDs {
		if placeID = strings.TrimSpace(placeID); placeID != "" {
			unique[placeID] = struct{}{}
		}
	}
	return sortedRestoredKeys(unique)
}

func intersectRestoredDispatchPlaceIDs(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, placeID := range right {
		rightSet[placeID] = struct{}{}
	}
	intersection := make(map[string]struct{})
	for _, placeID := range left {
		if _, ok := rightSet[placeID]; ok {
			intersection[placeID] = struct{}{}
		}
	}
	return sortedRestoredKeys(intersection)
}

// restoredHistoricalWorkIDs returns every Work identity known to durable
// replay, including Work no longer present in the restored marking.
func restoredHistoricalWorkIDs(cfg *runtimeConfig) map[string]struct{} {
	workIDs := make(map[string]struct{})
	if cfg == nil {
		return workIDs
	}
	addHistoricalWorldWorkIDs(workIDs, cfg.restoredWorldState)
	for _, event := range cfg.restoredEventPrefix {
		addHistoricalEventWorkIDs(workIDs, event)
	}
	return workIDs
}

// restoredHistoricalAdmissionWorks returns the durable identities that replay
// may use as relation targets while rebuilding recorded dispatches. These
// identities are deliberately not added to ordinary live admission: a Work
// that has left the current board must not become a new live dependency target
// merely because it appears in historical state.
func restoredHistoricalAdmissionWorks(cfg *runtimeConfig) []work.ExistingWork {
	if cfg == nil || !cfg.skipRestoredDispatchReconciliation {
		return nil
	}

	items := restoredWorkItems(cfg.restoredWorldState)
	workIDs := restoredHistoricalWorkIDs(cfg)
	works := make([]work.ExistingWork, 0, len(workIDs))
	for workID := range workIDs {
		candidate := work.ExistingWork{WorkID: workID}
		if item, ok := items[workID]; ok {
			candidate.Name = item.DisplayName
			candidate.WorkTypeID = item.WorkTypeID
		}
		works = append(works, candidate)
	}
	sort.Slice(works, func(i, j int) bool { return works[i].WorkID < works[j].WorkID })
	return works
}

// restoredHistoricalRelations returns canonical relation identities retained
// by the Recordings projection. Replay uses these only to resolve a worker
// output's name-only relation against the request that originally admitted it.
func restoredHistoricalRelations(cfg *runtimeConfig) []work.FactoryRelation {
	if cfg == nil || !cfg.skipRestoredDispatchReconciliation || cfg.restoredWorldState == nil {
		return nil
	}

	byKey := make(map[string]work.FactoryRelation)
	for _, relations := range cfg.restoredWorldState.RelationsByWorkID {
		for _, relation := range relations {
			if relation.RequestID == "" || relation.TargetWorkID == "" {
				continue
			}
			key := strings.Join([]string{
				relation.RequestID,
				relation.Type,
				relation.SourceWorkID,
				relation.SourceWorkName,
				relation.TargetWorkID,
				relation.TargetWorkName,
				relation.RequiredState,
			}, "\x00")
			byKey[key] = relation
		}
	}
	relations := make([]work.FactoryRelation, 0, len(byKey))
	for _, relation := range byKey {
		relations = append(relations, relation)
	}
	sort.Slice(relations, func(i, j int) bool {
		left, right := relations[i], relations[j]
		for _, pair := range [][2]string{
			{left.RequestID, right.RequestID},
			{left.Type, right.Type},
			{left.SourceWorkID, right.SourceWorkID},
			{left.SourceWorkName, right.SourceWorkName},
			{left.TargetWorkID, right.TargetWorkID},
			{left.TargetWorkName, right.TargetWorkName},
			{left.RequiredState, right.RequiredState},
		} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	return relations
}

func addHistoricalWorldWorkIDs(destination map[string]struct{}, restored *interfaces.FactoryWorldState) {
	if restored == nil {
		return
	}
	for _, items := range []map[string]work.FactoryWorkItem{
		restored.WorkItemsByID, restored.ActiveWorkItemsByID, restored.FailedWorkItemsByID,
	} {
		for workID, item := range items {
			addHistoricalWorkID(destination, workID)
			addHistoricalWorkID(destination, item.ID)
		}
	}
	for workID, terminal := range restored.TerminalWorkByID {
		addHistoricalWorkID(destination, workID)
		addHistoricalWorkID(destination, terminal.WorkItem.ID)
	}
	for workID := range restored.FailureDetailsByWorkID {
		addHistoricalWorkID(destination, workID)
	}
	for workID := range restored.WorkStateChangesByWorkID {
		addHistoricalWorkID(destination, workID)
	}
	for workID, relations := range restored.RelationsByWorkID {
		addHistoricalWorkID(destination, workID)
		for _, relation := range relations {
			addHistoricalWorkID(destination, relation.TargetWorkID)
		}
	}
	for _, request := range restored.WorkRequestsByID {
		for _, item := range request.WorkItems {
			addHistoricalWorkID(destination, item.ID)
		}
	}
	for _, occupancy := range restored.PlaceOccupancyByID {
		for _, workID := range occupancy.WorkItemIDs {
			addHistoricalWorkID(destination, workID)
		}
	}
	for _, dispatch := range restored.ActiveDispatches {
		addRestoredDispatchIdentity(destination, dispatch.WorkItemIDs, dispatch.Inputs, nil, nil, nil)
	}
	completions := append(append([]interfaces.FactoryWorldDispatchCompletion(nil),
		restored.CompletedDispatches...), restored.FailedDispatches...)
	for _, completion := range completions {
		addRestoredDispatchIdentity(destination, completion.WorkItemIDs, completion.ConsumedInputs,
			completion.InputWorkItems, completion.OutputWorkItems, completion.TerminalWork)
	}
}

func addHistoricalEventWorkIDs(destination map[string]struct{}, event interfaces.FactoryEvent) {
	if event.Context.WorkIDs != nil {
		for _, workID := range *event.Context.WorkIDs {
			addHistoricalWorkID(destination, workID)
		}
	}
	switch event.Type {
	case interfaces.FactoryEventTypeWorkRequest:
		var payload work.WorkRequestEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
			for _, item := range payload.Works {
				addHistoricalWorkID(destination, item.WorkID)
			}
		}
	case interfaces.FactoryEventTypeDispatchResponse:
		var payload workerexecution.DispatchResponseEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil && payload.OutputWork != nil {
			for _, item := range *payload.OutputWork {
				addHistoricalWorkID(destination, item.WorkID)
			}
		}
	case interfaces.FactoryEventTypeWorkStateChange:
		var payload interfaces.WorkStateChangeEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
			addHistoricalWorkID(destination, payload.WorkID)
			if payload.TriggerWorkID != nil {
				addHistoricalWorkID(destination, *payload.TriggerWorkID)
			}
		}
	}
}

func addHistoricalWorkID(destination map[string]struct{}, workID string) {
	if workID = strings.TrimSpace(workID); workID != "" {
		destination[workID] = struct{}{}
	}
}

func addRestoredDispatchIdentity(
	destination map[string]struct{},
	workIDs []string,
	inputs []interfaces.WorkstationInput,
	inputWork []work.FactoryWorkItem,
	outputWork []work.FactoryWorkItem,
	terminal *interfaces.FactoryTerminalWork,
) {
	addRestoredDispatchWorkIDs(destination, workIDs)
	for _, input := range inputs {
		if input.WorkItem != nil {
			addHistoricalWorkID(destination, input.WorkItem.ID)
		}
	}
	for _, item := range append(append([]work.FactoryWorkItem(nil), inputWork...), outputWork...) {
		addHistoricalWorkID(destination, item.ID)
	}
	if terminal != nil {
		addHistoricalWorkID(destination, terminal.WorkItem.ID)
	}
}
