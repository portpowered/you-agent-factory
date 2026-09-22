package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pointerStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

func restoredWorldStateForEvents(
	state *interfaces.FactoryWorldState,
	prefix []interfaces.FactoryEvent,
	events []interfaces.FactoryEvent,
) (*interfaces.FactoryWorldState, bool) {
	if state == nil || len(prefix) == 0 || len(events) < len(prefix) {
		return nil, false
	}
	for index := range prefix {
		if !sameFactoryEventIdentity(prefix[index], events[index]) {
			return nil, false
		}
	}
	for _, event := range events[len(prefix):] {
		if factoryEventRequiresWorkerSessionProjection(*state, event) {
			return nil, false
		}
	}
	return state, true
}

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

func restoredDispatchHistoryForRuntime(restored *interfaces.FactoryWorldState) []interfaces.CompletedDispatch {
	if restored == nil || len(restored.CompletedDispatches) == 0 {
		return nil
	}
	history := make([]interfaces.CompletedDispatch, 0, len(restored.CompletedDispatches))
	for _, completion := range restored.CompletedDispatches {
		result := completion.Result
		reason := result.Error
		if workerexecution.WorkOutcome(result.Outcome) == workerexecution.OutcomeContinue || workerexecution.WorkOutcome(result.Outcome) == workerexecution.OutcomeRejected {
			reason = result.Feedback
		}
		dispatch := interfaces.CompletedDispatch{
			DispatchID:                  completion.DispatchID,
			TransitionID:                completion.TransitionID,
			WorkstationName:             completion.Workstation.Name,
			ExpectedArtifactContext:     completion.ExpectedArtifactContext.Clone(),
			Outcome:                     workerexecution.WorkOutcome(result.Outcome),
			Cancellation:                result.Cancellation.Clone(),
			SelectedClassificationLabel: result.SelectedClassificationLabel,
			Reason:                      reason,
			ArtifactVerification:        result.ArtifactVerification.Clone(),
			FailureMetadata:             workerexecution.CloneWorkFailureMetadata(result.FailureMetadata),
			FailureDetail:               workerexecution.CloneFailureDetail(result.FailureDetail),
			ProviderSession:             completion.ProviderSession.Clone(),
			StartTime:                   completion.StartedAt,
			EndTime:                     completion.CompletedAt,
			Duration:                    time.Duration(completion.DurationMillis) * time.Millisecond,
			ConsumedTokens:              restoredCompletionInputTokens(completion),
			OutputMutations:             restoredCompletionOutputMutations(completion),
		}
		history = append(history, dispatch)
	}
	return history
}

func restoredCompletionInputTokens(completion interfaces.FactoryWorldDispatchCompletion) []workerexecution.Token {
	tokens := make([]workerexecution.Token, 0, len(completion.ConsumedInputs))
	seen := make(map[string]struct{}, len(completion.ConsumedInputs))
	for _, input := range completion.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" {
			continue
		}
		if _, ok := seen[input.WorkItem.ID]; ok {
			continue
		}
		seen[input.WorkItem.ID] = struct{}{}
		tokens = append(tokens, restoredWorkerToken(*input.WorkItem, input.TokenID))
	}
	if len(tokens) > 0 {
		return tokens
	}
	for _, item := range completion.InputWorkItems {
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		tokens = append(tokens, restoredWorkerToken(item, item.ID))
	}
	if len(tokens) > 0 {
		return tokens
	}
	for _, workID := range completion.WorkItemIDs {
		if workID == "" {
			continue
		}
		if _, ok := seen[workID]; ok {
			continue
		}
		seen[workID] = struct{}{}
		tokens = append(tokens, workerexecution.Token{ID: workID, Color: workerexecution.Color{WorkID: workID, DataType: workerexecution.DataTypeWork}})
	}
	return tokens
}

func restoredWorkerToken(item work.FactoryWorkItem, tokenID string) workerexecution.Token {
	if tokenID == "" {
		tokenID = item.ID
	}
	return workerexecution.Token{
		ID:    tokenID,
		State: item.State,
		Color: workerexecution.Color{
			Name:                     item.DisplayName,
			WorkID:                   item.ID,
			WorkTypeID:               item.WorkTypeID,
			DataType:                 workerexecution.DataTypeWork,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   item.CurrentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
			TraceID:                  item.TraceID,
			ParentID:                 item.ParentID,
			Tags:                     work.CloneTags(item.Tags),
			Content:                  work.CloneWorkContentParts(item.Content),
			StructuredResult:         item.StructuredResult,
			StructuredResultPresent:  item.StructuredResultPresent,
		},
	}
}

func restoredCompletionOutputMutations(completion interfaces.FactoryWorldDispatchCompletion) []interfaces.TokenMutationRecord {
	if len(completion.OutputWorkItems) == 0 {
		return nil
	}
	mutations := make([]interfaces.TokenMutationRecord, 0, len(completion.OutputWorkItems))
	for _, item := range completion.OutputWorkItems {
		if item.ID == "" {
			continue
		}
		token := restoredWorkerToken(item, item.ID)
		mutations = append(mutations, interfaces.TokenMutationRecord{
			DispatchID:   completion.DispatchID,
			TransitionID: completion.TransitionID,
			Outcome:      workerexecution.WorkOutcome(completion.Result.Outcome),
			Type:         interfaces.MutationCreate,
			TokenID:      item.ID,
			Token:        &token,
		})
	}
	return mutations
}

func workerRecordingSessionStartedAt(
	session recordings.WorkerSessionRecordingSnapshot,
) (*time.Time, error) {
	if len(session.Records) == 0 {
		return nil, nil
	}
	var draft workerexecution.Draft
	if err := json.Unmarshal(session.Records[0].Payload, &draft); err != nil {
		return nil, err
	}
	if draft.Kind != workerexecution.KindSession || draft.Phase != workerexecution.PhaseStarted {
		return nil, fmt.Errorf("first source record is %s/%s, want SESSION/STARTED", draft.Kind, draft.Phase)
	}
	var payload workerexecution.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.WorkerSessionID != session.WorkerSessionID {
		return nil, fmt.Errorf("opening identity %q does not match %q", payload.WorkerSessionID, session.WorkerSessionID)
	}
	if payload.StartedAt == nil {
		// Older sidecars did not retain source-native opening timestamps.
		return nil, nil
	}
	startedAt := payload.StartedAt.UTC()
	return &startedAt, nil
}

func validWorkerRecordingHealth(status recordings.WorkerRecordingStatus) bool {
	switch status {
	case recordings.WorkerRecordingStatusComplete,
		recordings.WorkerRecordingStatusDegraded,
		recordings.WorkerRecordingStatusIncomplete:
		return true
	}
	return false
}

func recordingHealthReason(status recordings.WorkerRecordingStatus, failure, interruption string) string {
	if status == recordings.WorkerRecordingStatusDegraded {
		return strings.TrimSpace(failure)
	}
	if status == recordings.WorkerRecordingStatusIncomplete {
		return strings.TrimSpace(interruption)
	}
	return ""
}
