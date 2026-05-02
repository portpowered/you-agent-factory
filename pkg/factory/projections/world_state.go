package projections

import (
	"fmt"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	tokenKindResource = "resource"
	tokenKindWork     = "work"
)

// ReconstructFactoryWorldState applies canonical factory events in tick order
// and returns the reconstructed world state at selectedTick. Events after the
// selected tick are ignored.
func ReconstructFactoryWorldState(events []factoryapi.FactoryEvent, selectedTick int) (interfaces.FactoryWorldState, error) {
	reducer := newFactoryWorldReducer(selectedTick)
	ordered := append([]factoryapi.FactoryEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		if left.Context.Tick != right.Context.Tick {
			return left.Context.Tick < right.Context.Tick
		}
		if left.Context.Sequence != right.Context.Sequence {
			return left.Context.Sequence < right.Context.Sequence
		}
		if !left.Context.EventTime.Equal(right.Context.EventTime) {
			return left.Context.EventTime.Before(right.Context.EventTime)
		}
		return left.Id < right.Id
	})

	for _, event := range ordered {
		if event.Context.Tick > selectedTick {
			continue
		}
		if err := reducer.apply(event); err != nil {
			return interfaces.FactoryWorldState{}, err
		}
	}

	return reducer.state(), nil
}

type factoryWorldReducer struct {
	stateValue   interfaces.FactoryWorldState
	placeTokens  map[string]map[string]struct{}
	tokenPlaces  map[string]string
	tokenWorkIDs map[string]string
	tokenKinds   map[string]string
	placeCats    map[string]string
	workPlaces   map[string]string
}

func newFactoryWorldReducer(selectedTick int) *factoryWorldReducer {
	return &factoryWorldReducer{
		stateValue: interfaces.FactoryWorldState{
			Tick:                          selectedTick,
			WorkRequestsByID:              make(map[string]interfaces.WorkRequestPayload),
			RelationsByWorkID:             make(map[string][]interfaces.FactoryRelation),
			WorkItemsByID:                 make(map[string]interfaces.FactoryWorkItem),
			ActiveWorkItemsByID:           make(map[string]interfaces.FactoryWorkItem),
			TerminalWorkByID:              make(map[string]interfaces.FactoryTerminalWork),
			FailedWorkItemsByID:           make(map[string]interfaces.FactoryWorkItem),
			FailureDetailsByWorkID:        make(map[string]interfaces.FactoryWorldFailureDetail),
			InferenceAttemptsByDispatchID: make(map[string]map[string]interfaces.FactoryWorldInferenceAttempt),
			ScriptRequestsByDispatchID:    make(map[string]map[string]interfaces.FactoryWorldScriptRequest),
			ScriptResponsesByDispatchID:   make(map[string]map[string]interfaces.FactoryWorldScriptResponse),
			PlaceOccupancyByID:            make(map[string]interfaces.FactoryPlaceOccupancy),
			ActiveDispatches:              make(map[string]interfaces.FactoryWorldDispatch),
			TracesByID:                    make(map[string]interfaces.FactoryWorldTrace),
		},
		placeTokens:  make(map[string]map[string]struct{}),
		tokenPlaces:  make(map[string]string),
		tokenWorkIDs: make(map[string]string),
		tokenKinds:   make(map[string]string),
		placeCats:    make(map[string]string),
		workPlaces:   make(map[string]string),
	}
}

func (r *factoryWorldReducer) apply(event factoryapi.FactoryEvent) error {
	r.stateValue.EventTime = event.Context.EventTime
	switch event.Type {
	case factoryapi.FactoryEventTypeRunRequest:
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			return err
		}
		if !r.hasTopology() {
			r.applyInitialStructure(initialStructureFromGenerated(factoryapi.InitialStructureRequestEventPayload{
				Factory: payload.Factory,
			}))
		}
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyInitialStructure(initialStructureFromGenerated(payload))
	case factoryapi.FactoryEventTypeWorkRequest:
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyWorkRequest(event.Context, payload)
	case factoryapi.FactoryEventTypeRelationshipChangeRequest:
		payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyRelationshipChange(event.Context, payload)
	case factoryapi.FactoryEventTypeDispatchRequest:
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyDispatchCreated(event, payload)
	case factoryapi.FactoryEventTypeInferenceRequest:
		payload, err := event.Payload.AsInferenceRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyInferenceRequest(event, payload)
	case factoryapi.FactoryEventTypeInferenceResponse:
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			return err
		}
		r.applyInferenceResponse(event, payload)
	case factoryapi.FactoryEventTypeScriptRequest:
		payload, err := event.Payload.AsScriptRequestEventPayload()
		if err != nil {
			return err
		}
		r.applyScriptRequest(event, payload)
	case factoryapi.FactoryEventTypeScriptResponse:
		payload, err := event.Payload.AsScriptResponseEventPayload()
		if err != nil {
			return err
		}
		r.applyScriptResponse(event, payload)
	case factoryapi.FactoryEventTypeDispatchResponse:
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			return err
		}
		r.applyDispatchCompleted(event, payload)
	case factoryapi.FactoryEventTypeFactoryStateResponse:
		payload, err := event.Payload.AsFactoryStateResponseEventPayload()
		if err != nil {
			return err
		}
		r.applyFactoryStateChange(payload)
	case factoryapi.FactoryEventTypeRunResponse:
		return nil
	}
	return nil
}

func (r *factoryWorldReducer) hasTopology() bool {
	return len(r.stateValue.Topology.Places) > 0 ||
		len(r.stateValue.Topology.Resources) > 0 ||
		len(r.stateValue.Topology.WorkTypes) > 0 ||
		len(r.stateValue.Topology.Workstations) > 0
}

func (r *factoryWorldReducer) applyInitialStructure(payload interfaces.InitialStructurePayload) {
	r.stateValue.Topology = payload
	for _, place := range payload.Places {
		r.placeCats[place.ID] = place.Category
	}
	for _, resource := range payload.Resources {
		r.seedResourceTokens(resource)
	}
}

func (r *factoryWorldReducer) applyWorkRequest(context factoryapi.FactoryEventContext, payload factoryapi.WorkRequestEventPayload) {
	requestID := stringValue(context.RequestId)
	if requestID == "" {
		requestID = firstRequestID(payload.Works)
	}
	if requestID == "" {
		return
	}
	traceID := firstString(context.TraceIds)
	workItems := factoryWorkItemsFromGenerated(payload.Works)
	for i := range workItems {
		if workItems[i].TraceID == "" {
			workItems[i].TraceID = traceID
		}
		if workItems[i].PlaceID == "" {
			workItems[i].PlaceID = r.initialPlaceForWorkType(workItems[i].WorkTypeID)
		}
	}
	r.stateValue.WorkRequestsByID[requestID] = interfaces.WorkRequestPayload{
		RequestID:     requestID,
		Type:          interfaces.WorkRequestType(payload.Type),
		TraceID:       traceID,
		Source:        stringValue(payload.Source),
		ParentLineage: cloneStringSlice(sliceValue(payload.ParentLineage)),
		WorkItems:     cloneWorkItems(workItems),
	}
	for _, item := range workItems {
		r.stateValue.WorkItemsByID[item.ID] = item
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
		r.addWorkToken(item.ID, item.PlaceID, item)
		r.addTraceWork(item.TraceID, item.ID)
	}
	for _, relation := range r.factoryRelationsFromGenerated(payload.Relations, context) {
		r.addRelation(relation)
	}
}

func (r *factoryWorldReducer) applyRelationshipChange(context factoryapi.FactoryEventContext, payload factoryapi.RelationshipChangeRequestEventPayload) {
	r.addRelation(r.factoryRelationFromGenerated(payload.Relation, context))
}

func (r *factoryWorldReducer) applyDispatchCreated(event factoryapi.FactoryEvent, payload factoryapi.DispatchRequestEventPayload) {
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return
	}
	inputWorkIDs := dispatchInputWorkIDs(payload, event.Context.WorkIds)
	workIDs := make([]string, 0, len(inputWorkIDs))
	traceIDs := make([]string, 0, len(inputWorkIDs))
	inputWorkItems := make([]interfaces.FactoryWorkItem, 0, len(inputWorkIDs))
	inputs := make([]interfaces.WorkstationInput, 0, len(inputWorkIDs))
	for _, workID := range inputWorkIDs {
		if workID == "" {
			continue
		}
		item, ok := r.stateValue.WorkItemsByID[workID]
		if !ok {
			item = interfaces.FactoryWorkItem{ID: workID}
		}
		if item.TraceID == "" {
			item.TraceID = firstString(event.Context.TraceIds)
		}
		placeID := r.workPlaces[item.ID]
		if placeID == "" {
			placeID = item.PlaceID
		}
		if placeID == "" {
			placeID = r.initialPlaceForWorkType(item.WorkTypeID)
		}
		item.PlaceID = placeID
		r.removeToken(item.ID)
		r.stateValue.WorkItemsByID[item.ID] = item
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
		workIDs = appendUnique(workIDs, item.ID)
		traceIDs = appendUnique(traceIDs, item.TraceID)
		r.addTraceWork(item.TraceID, item.ID)
		inputWorkItems = append(inputWorkItems, item)
		inputs = append(inputs, interfaces.WorkstationInput{
			TokenID:  item.ID,
			PlaceID:  placeID,
			WorkItem: &item,
		})
	}

	worker := r.workerForTransition(payload.TransitionId)
	dispatch := interfaces.FactoryWorldDispatch{
		DispatchID:   dispatchID,
		TransitionID: payload.TransitionId,
		Workstation:  r.workstationRefForTransition(payload.TransitionId),
		Provider:     worker.Provider,
		Model:        worker.Model,
		StartedTick:  event.Context.Tick,
		StartedAt:    event.Context.EventTime,
		Inputs:       inputs,
		WorkItemIDs:  sortedStrings(workIDs),
		CurrentChainingTraceID: dispatchCurrentChainingTraceID(
			event.Context.CurrentChainingTraceId,
			payload.CurrentChainingTraceId,
			inputWorkItems,
		),
		PreviousChainingTraceIDs: dispatchPreviousChainingTraceIDs(
			event.Context.PreviousChainingTraceIds,
			payload.PreviousChainingTraceIds,
			inputWorkItems,
		),
		TraceIDs: interfaces.CanonicalChainingTraceIDs(traceIDs),
	}
	dispatch.Resources = r.consumeResourceUnits(payload.Resources)
	r.stateValue.ActiveDispatches[dispatchID] = dispatch
	for _, traceID := range dispatch.TraceIDs {
		r.addTraceDispatch(traceID, dispatchID)
	}
}

func dispatchInputWorkIDs(payload factoryapi.DispatchRequestEventPayload, contextWorkIDs *[]string) []string {
	ordered := make([]string, 0, len(payload.Inputs)+len(sliceValue(contextWorkIDs)))
	for _, ref := range payload.Inputs {
		ordered = appendUnique(ordered, ref.WorkId)
	}
	for _, workID := range sliceValue(contextWorkIDs) {
		ordered = appendUnique(ordered, workID)
	}
	return ordered
}

func (r *factoryWorldReducer) applyInferenceRequest(event factoryapi.FactoryEvent, payload factoryapi.InferenceRequestEventPayload) {
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" || payload.InferenceRequestId == "" {
		return
	}
	attempts := r.inferenceAttemptsForDispatch(dispatchID)
	current := attempts[payload.InferenceRequestId]
	current.DispatchID = dispatchID
	current.TransitionID = firstNonEmpty(current.TransitionID, r.transitionIDForDispatch(dispatchID))
	current.InferenceRequestID = payload.InferenceRequestId
	current.Attempt = payload.Attempt
	current.WorkingDirectory = payload.WorkingDirectory
	current.Worktree = payload.Worktree
	current.Prompt = payload.Prompt
	current.RequestTime = event.Context.EventTime
	attempts[payload.InferenceRequestId] = current
}

func (r *factoryWorldReducer) applyInferenceResponse(event factoryapi.FactoryEvent, payload factoryapi.InferenceResponseEventPayload) {
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" || payload.InferenceRequestId == "" {
		return
	}
	attempts := r.inferenceAttemptsForDispatch(dispatchID)
	current := attempts[payload.InferenceRequestId]
	current.DispatchID = dispatchID
	current.TransitionID = firstNonEmpty(current.TransitionID, r.transitionIDForDispatch(dispatchID))
	current.InferenceRequestID = payload.InferenceRequestId
	current.Attempt = payload.Attempt
	current.Outcome = string(payload.Outcome)
	current.Response = stringValue(payload.Response)
	current.DurationMillis = payload.DurationMillis
	current.ExitCode = intPtrValue(payload.ExitCode)
	current.ErrorClass = stringValue(payload.ErrorClass)
	current.ProviderSession = interfaces.ProviderSessionMetadataFromGenerated(payload.ProviderSession)
	current.Diagnostics = interfaces.SafeWorkDiagnosticsFromGenerated(payload.Diagnostics)
	current.ResponseTime = event.Context.EventTime
	attempts[payload.InferenceRequestId] = current
}

func (r *factoryWorldReducer) applyScriptRequest(event factoryapi.FactoryEvent, payload factoryapi.ScriptRequestEventPayload) {
	if payload.DispatchId == "" || payload.ScriptRequestId == "" {
		return
	}
	requests := r.scriptRequestsForDispatch(payload.DispatchId)
	current := requests[payload.ScriptRequestId]
	current.DispatchID = payload.DispatchId
	current.TransitionID = payload.TransitionId
	current.ScriptRequestID = payload.ScriptRequestId
	current.Attempt = payload.Attempt
	current.Command = payload.Command
	current.Args = cloneStringSlice(payload.Args)
	current.RequestTime = event.Context.EventTime
	requests[payload.ScriptRequestId] = current
}

func (r *factoryWorldReducer) applyScriptResponse(event factoryapi.FactoryEvent, payload factoryapi.ScriptResponseEventPayload) {
	if payload.DispatchId == "" || payload.ScriptRequestId == "" {
		return
	}
	responses := r.scriptResponsesForDispatch(payload.DispatchId)
	current := responses[payload.ScriptRequestId]
	current.DispatchID = payload.DispatchId
	current.TransitionID = payload.TransitionId
	current.ScriptRequestID = payload.ScriptRequestId
	current.Attempt = payload.Attempt
	current.Outcome = string(payload.Outcome)
	current.Stdout = payload.Stdout
	current.Stderr = payload.Stderr
	current.DurationMillis = payload.DurationMillis
	current.ExitCode = intPtrValue(payload.ExitCode)
	current.FailureType = enumStringValue(payload.FailureType)
	current.ResponseTime = event.Context.EventTime
	responses[payload.ScriptRequestId] = current
}

func (r *factoryWorldReducer) inferenceAttemptsForDispatch(dispatchID string) map[string]interfaces.FactoryWorldInferenceAttempt {
	attempts := r.stateValue.InferenceAttemptsByDispatchID[dispatchID]
	if attempts == nil {
		attempts = make(map[string]interfaces.FactoryWorldInferenceAttempt)
		r.stateValue.InferenceAttemptsByDispatchID[dispatchID] = attempts
	}
	return attempts
}

func (r *factoryWorldReducer) scriptRequestsForDispatch(dispatchID string) map[string]interfaces.FactoryWorldScriptRequest {
	requests := r.stateValue.ScriptRequestsByDispatchID[dispatchID]
	if requests == nil {
		requests = make(map[string]interfaces.FactoryWorldScriptRequest)
		r.stateValue.ScriptRequestsByDispatchID[dispatchID] = requests
	}
	return requests
}

func (r *factoryWorldReducer) scriptResponsesForDispatch(dispatchID string) map[string]interfaces.FactoryWorldScriptResponse {
	responses := r.stateValue.ScriptResponsesByDispatchID[dispatchID]
	if responses == nil {
		responses = make(map[string]interfaces.FactoryWorldScriptResponse)
		r.stateValue.ScriptResponsesByDispatchID[dispatchID] = responses
	}
	return responses
}

func (r *factoryWorldReducer) applyDispatchCompleted(event factoryapi.FactoryEvent, payload factoryapi.DispatchResponseEventPayload) {
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return
	}
	dispatch := r.stateValue.ActiveDispatches[dispatchID]
	delete(r.stateValue.ActiveDispatches, dispatchID)

	workIDs := append([]string(nil), dispatch.WorkItemIDs...)
	traceIDs := dispatchCompletionTraceIDs(dispatch, event.Context.TraceIds)
	outputWorkItems, workIDs, traceIDs := r.applyDispatchOutputWork(dispatch, payload, workIDs, traceIDs)
	r.releaseResourceUnits(dispatch.Resources, payload.OutputResources)
	completion := r.dispatchCompletionFromResponse(event, payload, dispatchID, dispatch, workIDs, traceIDs, outputWorkItems)
	r.recordDispatchCompletionState(dispatchID, dispatch, payload, completion)
}

func dispatchCompletionTraceIDs(dispatch interfaces.FactoryWorldDispatch, eventTraceIDs *[]string) []string {
	traceIDs := append([]string(nil), dispatch.TraceIDs...)
	for _, traceID := range sliceValue(eventTraceIDs) {
		traceIDs = appendUnique(traceIDs, traceID)
	}
	return traceIDs
}

func (r *factoryWorldReducer) applyDispatchOutputWork(
	dispatch interfaces.FactoryWorldDispatch,
	payload factoryapi.DispatchResponseEventPayload,
	workIDs []string,
	traceIDs []string,
) ([]interfaces.FactoryWorkItem, []string, []string) {
	outputWorkItems := make([]interfaces.FactoryWorkItem, 0, len(sliceValue(payload.OutputWork)))
	for _, work := range sliceValue(payload.OutputWork) {
		item := r.dispatchOutputWorkItem(dispatch, payload, work)
		if item.ID == "" {
			continue
		}
		r.stateValue.WorkItemsByID[item.ID] = item
		workIDs = appendUnique(workIDs, item.ID)
		traceIDs = appendUnique(traceIDs, item.TraceID)
		r.addTraceWork(item.TraceID, item.ID)
		r.addWorkToken(item.ID, item.PlaceID, item)
		outputWorkItems = append(outputWorkItems, item)
	}
	return outputWorkItems, workIDs, traceIDs
}

func (r *factoryWorldReducer) dispatchOutputWorkItem(
	dispatch interfaces.FactoryWorldDispatch,
	payload factoryapi.DispatchResponseEventPayload,
	work factoryapi.Work,
) interfaces.FactoryWorkItem {
	item := factoryWorkItemFromGenerated(work)
	if item.ID == "" {
		return interfaces.FactoryWorkItem{}
	}
	explicitPlaceID := item.PlaceID
	previousPlaceID := item.PlaceID
	if existing, ok := r.stateValue.WorkItemsByID[item.ID]; ok {
		previousPlaceID = existing.PlaceID
		item = mergeFactoryWorkItem(existing, item)
	}
	if explicitPlaceID == "" {
		if derivedPlaceID := r.outputPlaceForWork(dispatch.Workstation.ID, payload.Outcome, item.WorkTypeID); derivedPlaceID != "" {
			item.PlaceID = derivedPlaceID
		} else if payload.Outcome == factoryapi.WorkOutcomeContinue || payload.Outcome == factoryapi.WorkOutcomeRejected {
			item.PlaceID = previousPlaceID
		}
	}
	return item
}

func (r *factoryWorldReducer) dispatchCompletionFromResponse(
	event factoryapi.FactoryEvent,
	payload factoryapi.DispatchResponseEventPayload,
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	workIDs []string,
	traceIDs []string,
	outputWorkItems []interfaces.FactoryWorkItem,
) interfaces.FactoryWorldDispatchCompletion {
	inputWorkItems := dispatchInputWorkItems(dispatch)
	latestAttempt := r.latestInferenceAttemptForDispatch(dispatchID)
	return interfaces.FactoryWorldDispatchCompletion{
		DispatchID:      dispatchID,
		TransitionID:    payload.TransitionId,
		Workstation:     dispatch.Workstation,
		StartedTick:     dispatch.StartedTick,
		CompletedTick:   event.Context.Tick,
		StartedAt:       dispatch.StartedAt,
		CompletedAt:     event.Context.EventTime,
		DurationMillis:  int64Value(payload.DurationMillis),
		Result:          workstationResultFromGenerated(payload),
		WorkItemIDs:     sortedStrings(workIDs),
		ConsumedInputs:  cloneWorkstationInputs(dispatch.Inputs),
		InputWorkItems:  sortedWorkItems(inputWorkItems),
		OutputWorkItems: sortedWorkItems(outputWorkItems),
		CurrentChainingTraceID: completedDispatchCurrentChainingTraceID(
			event.Context.CurrentChainingTraceId,
			payload.CurrentChainingTraceId,
			dispatch,
			inputWorkItems,
		),
		PreviousChainingTraceIDs: completedDispatchPreviousChainingTraceIDs(
			event.Context.PreviousChainingTraceIds,
			payload.PreviousChainingTraceIds,
			dispatch,
			inputWorkItems,
		),
		TraceIDs:        interfaces.CanonicalChainingTraceIDs(traceIDs),
		ProviderSession: latestInferenceProviderSession(latestAttempt),
		Diagnostics:     latestInferenceDiagnostics(latestAttempt),
		TerminalWork:    r.terminalWorkForCompletion(payload.Outcome, workIDs),
	}
}

func (r *factoryWorldReducer) recordDispatchCompletionState(
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	payload factoryapi.DispatchResponseEventPayload,
	completion interfaces.FactoryWorldDispatchCompletion,
) {
	r.stateValue.CompletedDispatches = append(r.stateValue.CompletedDispatches, completion)
	if payload.Outcome == factoryapi.WorkOutcomeFailed {
		r.stateValue.FailedDispatches = append(r.stateValue.FailedDispatches, completion)
		r.recordFailedCompletion(completion)
	}
	for _, traceID := range completion.TraceIDs {
		r.addTraceDispatch(traceID, dispatchID)
	}
	r.appendProviderSessionRecord(dispatch, payload, completion)
}

func (r *factoryWorldReducer) appendProviderSessionRecord(
	dispatch interfaces.FactoryWorldDispatch,
	payload factoryapi.DispatchResponseEventPayload,
	completion interfaces.FactoryWorldDispatchCompletion,
) {
	if completion.ProviderSession == nil || completion.ProviderSession.ID == "" {
		return
	}
	r.stateValue.ProviderSessions = append(r.stateValue.ProviderSessions, interfaces.FactoryWorldProviderSessionRecord{
		DispatchID:               completion.DispatchID,
		TransitionID:             payload.TransitionId,
		WorkstationName:          dispatch.Workstation.Name,
		Outcome:                  string(payload.Outcome),
		ProviderSession:          *interfaces.CloneProviderSessionMetadata(completion.ProviderSession),
		WorkItemIDs:              completion.WorkItemIDs,
		ConsumedInputs:           cloneWorkstationInputs(completion.ConsumedInputs),
		CurrentChainingTraceID:   completion.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(completion.PreviousChainingTraceIDs),
		TraceIDs:                 cloneStringSlice(completion.TraceIDs),
		Diagnostics:              interfaces.CloneSafeWorkDiagnostics(completion.Diagnostics),
		FailureReason:            completion.Result.FailureReason,
		FailureMessage:           completion.Result.FailureMessage,
	})
}

func dispatchCurrentChainingTraceID(
	contextCurrent *string,
	payloadCurrent *string,
	inputWorkItems []interfaces.FactoryWorkItem,
) string {
	if current := stringValue(contextCurrent); current != "" {
		return current
	}
	if current := stringValue(payloadCurrent); current != "" {
		return current
	}
	return interfaces.CurrentChainingTraceIDFromWorkItems(inputWorkItems)
}

func dispatchPreviousChainingTraceIDs(
	contextPrevious *[]string,
	payloadPrevious *[]string,
	inputWorkItems []interfaces.FactoryWorkItem,
) []string {
	if previous := cloneStringSlice(sliceValue(contextPrevious)); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	if previous := cloneStringSlice(sliceValue(payloadPrevious)); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	return interfaces.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)
}

func completedDispatchCurrentChainingTraceID(
	contextCurrent *string,
	payloadCurrent *string,
	dispatch interfaces.FactoryWorldDispatch,
	inputWorkItems []interfaces.FactoryWorkItem,
) string {
	if current := stringValue(contextCurrent); current != "" {
		return current
	}
	if current := stringValue(payloadCurrent); current != "" {
		return current
	}
	if dispatch.CurrentChainingTraceID != "" {
		return dispatch.CurrentChainingTraceID
	}
	return interfaces.CurrentChainingTraceIDFromWorkItems(inputWorkItems)
}

func completedDispatchPreviousChainingTraceIDs(
	contextPrevious *[]string,
	payloadPrevious *[]string,
	dispatch interfaces.FactoryWorldDispatch,
	inputWorkItems []interfaces.FactoryWorkItem,
) []string {
	if previous := cloneStringSlice(sliceValue(contextPrevious)); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	if previous := cloneStringSlice(sliceValue(payloadPrevious)); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	if len(dispatch.PreviousChainingTraceIDs) > 0 {
		return cloneStringSlice(dispatch.PreviousChainingTraceIDs)
	}
	return interfaces.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)
}

func (r *factoryWorldReducer) recordFailedCompletion(completion interfaces.FactoryWorldDispatchCompletion) {
	if completion.TerminalWork != nil {
		r.recordFailedWorkDetail(completion, completion.TerminalWork.WorkItem)
		return
	}
	for _, workID := range completion.WorkItemIDs {
		if item, ok := r.stateValue.WorkItemsByID[workID]; ok {
			r.recordFailedWorkDetail(completion, item)
		}
	}
}

func dispatchInputWorkItems(
	dispatch interfaces.FactoryWorldDispatch,
) []interfaces.FactoryWorkItem {
	items := make([]interfaces.FactoryWorkItem, 0, len(dispatch.Inputs))
	for _, input := range dispatch.Inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" {
			continue
		}
		items = append(items, *input.WorkItem)
	}
	return items
}

func (r *factoryWorldReducer) latestInferenceAttemptForDispatch(dispatchID string) *interfaces.FactoryWorldInferenceAttempt {
	attempts := r.stateValue.InferenceAttemptsByDispatchID[dispatchID]
	if len(attempts) == 0 {
		return nil
	}
	var latest *interfaces.FactoryWorldInferenceAttempt
	for _, requestID := range sortedMapKeys(attempts) {
		attempt := attempts[requestID]
		if latest == nil ||
			attempt.Attempt > latest.Attempt ||
			(attempt.Attempt == latest.Attempt && attempt.ResponseTime.After(latest.ResponseTime)) ||
			(attempt.Attempt == latest.Attempt && attempt.ResponseTime.Equal(latest.ResponseTime) && attempt.InferenceRequestID > latest.InferenceRequestID) {
			attemptCopy := attempt
			latest = &attemptCopy
		}
	}
	return latest
}

func latestInferenceProviderSession(attempt *interfaces.FactoryWorldInferenceAttempt) *interfaces.ProviderSessionMetadata {
	if attempt == nil {
		return nil
	}
	return interfaces.CloneProviderSessionMetadata(attempt.ProviderSession)
}

func latestInferenceDiagnostics(attempt *interfaces.FactoryWorldInferenceAttempt) *interfaces.SafeWorkDiagnostics {
	if attempt == nil {
		return nil
	}
	return interfaces.CloneSafeWorkDiagnostics(attempt.Diagnostics)
}

func (r *factoryWorldReducer) recordFailedWorkDetail(completion interfaces.FactoryWorldDispatchCompletion, item interfaces.FactoryWorkItem) {
	if item.ID == "" {
		return
	}
	r.stateValue.WorkItemsByID[item.ID] = item
	r.stateValue.FailedWorkItemsByID[item.ID] = item
	delete(r.stateValue.ActiveWorkItemsByID, item.ID)
	r.addTraceFailed(item.TraceID, item.ID)
	r.stateValue.FailureDetailsByWorkID[item.ID] = interfaces.FactoryWorldFailureDetail{
		DispatchID:      completion.DispatchID,
		TransitionID:    completion.TransitionID,
		WorkstationName: completion.Workstation.Name,
		WorkItem:        item,
		FailureReason:   completion.Result.FailureReason,
		FailureMessage:  completion.Result.FailureMessage,
	}
}

func (r *factoryWorldReducer) applyFactoryStateChange(payload factoryapi.FactoryStateResponseEventPayload) {
	r.stateValue.FactoryStatePrevious = factoryStateString(payload.PreviousState)
	r.stateValue.FactoryState = string(payload.State)
	r.stateValue.FactoryStateReason = stringValue(payload.Reason)
}

func (r *factoryWorldReducer) state() interfaces.FactoryWorldState {
	r.rebuildOccupancy()
	r.sortTraceSlices()
	return r.stateValue
}

func (r *factoryWorldReducer) addWorkToken(tokenID string, placeID string, item interfaces.FactoryWorkItem) {
	if tokenID == "" || placeID == "" {
		return
	}
	r.addToken(tokenID, placeID, tokenKindWork)
	r.tokenWorkIDs[tokenID] = item.ID
	r.workPlaces[item.ID] = placeID
	if r.isTerminalPlace(placeID) {
		r.stateValue.TerminalWorkByID[item.ID] = interfaces.FactoryTerminalWork{WorkItem: item, Status: r.placeCats[placeID]}
		delete(r.stateValue.ActiveWorkItemsByID, item.ID)
		r.addTraceTerminal(item.TraceID, item.ID)
	} else if r.isFailedPlace(placeID) {
		r.stateValue.FailedWorkItemsByID[item.ID] = item
		delete(r.stateValue.ActiveWorkItemsByID, item.ID)
		r.addTraceFailed(item.TraceID, item.ID)
	} else {
		r.stateValue.ActiveWorkItemsByID[item.ID] = item
	}
}

func (r *factoryWorldReducer) addToken(tokenID string, placeID string, kind string) {
	r.removeToken(tokenID)
	if r.placeTokens[placeID] == nil {
		r.placeTokens[placeID] = make(map[string]struct{})
	}
	r.placeTokens[placeID][tokenID] = struct{}{}
	r.tokenPlaces[tokenID] = placeID
	r.tokenKinds[tokenID] = kind
}

func (r *factoryWorldReducer) removeToken(tokenID string) {
	if tokenID == "" {
		return
	}
	placeID := r.tokenPlaces[tokenID]
	r.removeTokenFromPlaceIndex(placeID, tokenID)
	delete(r.tokenPlaces, tokenID)
	delete(r.tokenKinds, tokenID)
	delete(r.tokenWorkIDs, tokenID)
}

func (r *factoryWorldReducer) removeTokenFromPlaceIndex(placeID string, tokenID string) {
	if placeID == "" {
		return
	}
	delete(r.placeTokens[placeID], tokenID)
	if len(r.placeTokens[placeID]) == 0 {
		delete(r.placeTokens, placeID)
	}
}

func (r *factoryWorldReducer) seedResourceTokens(resource interfaces.FactoryResource) {
	if resource.ID == "" || resource.Capacity <= 0 {
		return
	}
	placeID := resourceAvailablePlaceID(resource.ID)
	for i := range resource.Capacity {
		r.addToken(resourceTokenID(resource.ID, i), placeID, tokenKindResource)
	}
}

func (r *factoryWorldReducer) consumeResourceUnits(resources *[]factoryapi.Resource) []interfaces.FactoryResourceUnit {
	generated := resourceUnitsFromGenerated(resources)
	if len(generated) == 0 {
		return nil
	}
	consumed := make([]interfaces.FactoryResourceUnit, 0, len(generated))
	for _, resource := range generated {
		tokenID := r.firstAvailableResourceTokenID(resource.ResourceID)
		unit := interfaces.FactoryResourceUnit{
			ResourceID: resource.ResourceID,
			TokenID:    tokenID,
			PlaceID:    resourceAvailablePlaceID(resource.ResourceID),
		}
		if tokenID != "" {
			r.removeToken(tokenID)
		}
		consumed = append(consumed, unit)
	}
	return consumed
}

func (r *factoryWorldReducer) releaseResourceUnits(consumed []interfaces.FactoryResourceUnit, resources *[]factoryapi.Resource) {
	released := make([]bool, len(consumed))
	for _, resource := range resourceUnitsFromGenerated(resources) {
		index := firstConsumedResourceIndex(consumed, released, resource.ResourceID)
		if index < 0 {
			continue
		}
		released[index] = true
		unit := consumed[index]
		if unit.TokenID == "" {
			continue
		}
		placeID := unit.PlaceID
		if placeID == "" {
			placeID = resourceAvailablePlaceID(unit.ResourceID)
		}
		r.addToken(unit.TokenID, placeID, tokenKindResource)
	}
}

func (r *factoryWorldReducer) firstAvailableResourceTokenID(resourceID string) string {
	if resourceID == "" {
		return ""
	}
	tokenIDs := make([]string, 0, len(r.placeTokens[resourceAvailablePlaceID(resourceID)]))
	for tokenID := range r.placeTokens[resourceAvailablePlaceID(resourceID)] {
		if r.tokenKinds[tokenID] == tokenKindResource {
			tokenIDs = append(tokenIDs, tokenID)
		}
	}
	tokenIDs = sortedStrings(tokenIDs)
	if len(tokenIDs) == 0 {
		return ""
	}
	return tokenIDs[0]
}

func firstConsumedResourceIndex(resources []interfaces.FactoryResourceUnit, released []bool, resourceID string) int {
	for i, resource := range resources {
		if released[i] || resource.ResourceID != resourceID {
			continue
		}
		return i
	}
	return -1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type worldStateWorkerMetadata struct {
	Provider string
	Model    string
}

func resourceAvailablePlaceID(resourceID string) string {
	return generatedPlaceID(resourceID, interfaces.ResourceStateAvailable)
}

func resourceTokenID(resourceID string, index int) string {
	return fmt.Sprintf("%s:resource:%d", resourceID, index)
}

func (r *factoryWorldReducer) initialPlaceForWorkType(workTypeID string) string {
	fallback := ""
	for _, place := range r.stateValue.Topology.Places {
		if !placeMatchesWorkType(place, workTypeID) {
			continue
		}
		if fallback == "" {
			fallback = place.ID
		}
		if place.Category == "INITIAL" {
			return place.ID
		}
	}
	return fallback
}

func (r *factoryWorldReducer) outputPlaceForWork(workstationID string, outcome factoryapi.WorkOutcome, workTypeID string) string {
	workstation, ok := r.topologyWorkstation(workstationID)
	if !ok {
		return ""
	}
	routes := workstation.OutputPlaceIDs
	switch outcome {
	case factoryapi.WorkOutcomeContinue:
		if len(workstation.ContinuePlaceIDs) == 0 {
			return ""
		}
		routes = workstation.ContinuePlaceIDs
	case factoryapi.WorkOutcomeRejected:
		if len(workstation.RejectionPlaceIDs) == 0 {
			return ""
		}
		routes = workstation.RejectionPlaceIDs
	case factoryapi.WorkOutcomeFailed:
		if len(workstation.FailurePlaceIDs) > 0 {
			routes = workstation.FailurePlaceIDs
		}
	}
	for _, placeID := range routes {
		if place, ok := r.topologyPlace(placeID); ok && placeMatchesWorkType(place, workTypeID) {
			return place.ID
		}
		if placeIDMatchesWorkType(placeID, workTypeID) {
			return placeID
		}
	}
	if outcome == factoryapi.WorkOutcomeFailed {
		for _, place := range r.stateValue.Topology.Places {
			if placeMatchesWorkType(place, workTypeID) && place.Category == "FAILED" {
				return place.ID
			}
		}
	}
	return ""
}

func placeMatchesWorkType(place interfaces.FactoryPlace, workTypeID string) bool {
	if place.TypeID == workTypeID {
		return true
	}
	return placeIDMatchesWorkType(place.ID, workTypeID)
}

func placeIDMatchesWorkType(placeID string, workTypeID string) bool {
	if placeID == "" || workTypeID == "" {
		return false
	}
	prefix, _, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID == workTypeID
	}
	return prefix == workTypeID
}

func (r *factoryWorldReducer) terminalWorkForCompletion(outcome factoryapi.WorkOutcome, workIDs []string) *interfaces.FactoryTerminalWork {
	for _, workID := range sortedStrings(workIDs) {
		item, ok := r.stateValue.WorkItemsByID[workID]
		if !ok || item.PlaceID == "" {
			continue
		}
		category := r.placeCats[item.PlaceID]
		if category == "TERMINAL" || category == "FAILED" || outcome == factoryapi.WorkOutcomeFailed {
			return &interfaces.FactoryTerminalWork{WorkItem: item, Status: category}
		}
	}
	return nil
}

func (r *factoryWorldReducer) rebuildOccupancy() {
	occupancy := make(map[string]interfaces.FactoryPlaceOccupancy, len(r.placeTokens))
	for placeID, tokens := range r.placeTokens {
		entry := interfaces.FactoryPlaceOccupancy{PlaceID: placeID}
		for tokenID := range tokens {
			switch r.tokenKinds[tokenID] {
			case tokenKindResource:
				entry.ResourceTokenIDs = append(entry.ResourceTokenIDs, tokenID)
			default:
				if workID := r.tokenWorkIDs[tokenID]; workID != "" {
					entry.WorkItemIDs = append(entry.WorkItemIDs, workID)
				}
			}
		}
		entry.WorkItemIDs = sortedStrings(entry.WorkItemIDs)
		entry.ResourceTokenIDs = sortedStrings(entry.ResourceTokenIDs)
		entry.TokenCount = len(entry.WorkItemIDs) + len(entry.ResourceTokenIDs)
		occupancy[placeID] = entry
	}
	r.stateValue.PlaceOccupancyByID = occupancy
}

func (r *factoryWorldReducer) isTerminalPlace(placeID string) bool {
	return r.placeCats[placeID] == "TERMINAL"
}

func (r *factoryWorldReducer) isFailedPlace(placeID string) bool {
	return r.placeCats[placeID] == "FAILED"
}

func (r *factoryWorldReducer) addTraceWork(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.WorkItemIDs = appendUnique(trace.WorkItemIDs, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceDispatch(traceID string, dispatchID string) {
	if traceID == "" || dispatchID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.DispatchIDs = appendUnique(trace.DispatchIDs, dispatchID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceTerminal(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.TerminalWork = appendUnique(trace.TerminalWork, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addTraceFailed(traceID string, workID string) {
	if traceID == "" || workID == "" {
		return
	}
	trace := r.stateValue.TracesByID[traceID]
	trace.TraceID = traceID
	trace.FailedWorkIDs = appendUnique(trace.FailedWorkIDs, workID)
	r.stateValue.TracesByID[traceID] = trace
}

func (r *factoryWorldReducer) addRelation(relation interfaces.FactoryRelation) {
	if relation.SourceWorkID == "" || relation.TargetWorkID == "" {
		return
	}
	existing := r.stateValue.RelationsByWorkID[relation.SourceWorkID]
	for _, current := range existing {
		if current.Type == relation.Type &&
			current.TargetWorkID == relation.TargetWorkID &&
			current.RequiredState == relation.RequiredState &&
			current.RequestID == relation.RequestID {
			return
		}
	}
	r.stateValue.RelationsByWorkID[relation.SourceWorkID] = append(existing, relation)
}

func (r *factoryWorldReducer) sortTraceSlices() {
	for traceID, trace := range r.stateValue.TracesByID {
		trace.WorkItemIDs = sortedStrings(trace.WorkItemIDs)
		trace.DispatchIDs = sortedStrings(trace.DispatchIDs)
		trace.TerminalWork = sortedStrings(trace.TerminalWork)
		trace.FailedWorkIDs = sortedStrings(trace.FailedWorkIDs)
		r.stateValue.TracesByID[traceID] = trace
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func cloneWorkItems(input []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(input) == 0 {
		return nil
	}
	out := make([]interfaces.FactoryWorkItem, len(input))
	for i, item := range input {
		out[i] = item
		out[i].Tags = cloneStringMap(item.Tags)
	}
	return out
}

func cloneWorkstationInputs(input []interfaces.WorkstationInput) []interfaces.WorkstationInput {
	if len(input) == 0 {
		return nil
	}
	out := make([]interfaces.WorkstationInput, len(input))
	for i, value := range input {
		out[i] = value
		if value.WorkItem != nil {
			item := *value.WorkItem
			item.Tags = cloneStringMap(item.Tags)
			out[i].WorkItem = &item
		}
		if value.Resource != nil {
			resource := *value.Resource
			out[i].Resource = &resource
		}
	}
	return out
}

func sortedWorkItems(input []interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	if len(input) == 0 {
		return nil
	}
	out := cloneWorkItems(input)
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkTypeID != out[j].WorkTypeID {
			return out[i].WorkTypeID < out[j].WorkTypeID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
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

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	deduped := out[:0]
	var previous string
	for i, value := range out {
		if value == "" || (i > 0 && value == previous) {
			previous = value
			continue
		}
		deduped = append(deduped, value)
		previous = value
	}
	return deduped
}

// portos:func-length-exception owner=agent-factory reason=generated-initial-structure-adapter review=2026-07-18 removal=split-resource-worktype-worker-and-workstation-converters-before-next-projection-expansion
func initialStructureFromGenerated(payload factoryapi.InitialStructureRequestEventPayload) interfaces.InitialStructurePayload {
	factoryPayload := payload.Factory
	resources := make([]interfaces.FactoryResource, 0, len(sliceValue(factoryPayload.Resources)))
	places := make([]interfaces.FactoryPlace, 0)
	for _, resource := range sliceValue(factoryPayload.Resources) {
		resources = append(resources, interfaces.FactoryResource{
			ID:       resource.Name,
			Name:     resource.Name,
			Capacity: resource.Capacity,
		})
		places = append(places, interfaces.FactoryPlace{
			ID:       generatedPlaceID(resource.Name, "available"),
			TypeID:   resource.Name,
			State:    "available",
			Category: "PROCESSING",
		})
	}

	workTypes := make([]interfaces.FactoryWorkType, 0, len(sliceValue(factoryPayload.WorkTypes)))
	for _, workType := range sliceValue(factoryPayload.WorkTypes) {
		converted := interfaces.FactoryWorkType{
			ID:   workType.Name,
			Name: workType.Name,
		}
		for _, stateDef := range workType.States {
			category := string(stateDef.Type)
			converted.States = append(converted.States, interfaces.FactoryStateDefinition{
				Value:    stateDef.Name,
				Category: category,
			})
			places = append(places, interfaces.FactoryPlace{
				ID:       generatedPlaceID(workType.Name, stateDef.Name),
				TypeID:   workType.Name,
				State:    stateDef.Name,
				Category: category,
			})
		}
		workTypes = append(workTypes, converted)
	}

	workers := make([]interfaces.FactoryWorker, 0, len(sliceValue(factoryPayload.Workers)))
	for _, worker := range sliceValue(factoryPayload.Workers) {
		config := map[string]string{}
		if workerType := enumStringValue(worker.Type); workerType != "" {
			config["type"] = workerType
		}
		workers = append(workers, interfaces.FactoryWorker{
			ID:            worker.Name,
			Name:          worker.Name,
			Provider:      enumStringValue(worker.ExecutorProvider),
			ModelProvider: enumStringValue(worker.ModelProvider),
			Model:         stringValue(worker.Model),
			Config:        nilIfEmptyStringMap(config),
		})
	}

	workstations := make([]interfaces.FactoryWorkstation, 0, len(sliceValue(factoryPayload.Workstations)))
	for _, workstation := range sliceValue(factoryPayload.Workstations) {
		id := stringValue(workstation.Id)
		if id == "" {
			id = workstation.Name
		}
		config := map[string]string{}
		if runtimeType := enumStringValue(workstation.Type); runtimeType != "" {
			config["type"] = runtimeType
		}
		if workstation.Worker != "" {
			config["worker"] = workstation.Worker
			config["configured_worker"] = workstation.Worker
		}
		workstations = append(workstations, interfaces.FactoryWorkstation{
			ID:                id,
			Name:              workstation.Name,
			WorkerID:          workstation.Worker,
			Kind:              workstationKindString(workstation.Behavior),
			Config:            nilIfEmptyStringMap(config),
			InputPlaceIDs:     placeIDsFromGeneratedIOs(workstation.Inputs),
			OutputPlaceIDs:    placeIDsFromGeneratedIOs(workstation.Outputs),
			ContinuePlaceIDs:  placeIDsFromGeneratedIOPtr(workstation.OnContinue),
			RejectionPlaceIDs: placeIDsFromGeneratedIOPtr(workstation.OnRejection),
			FailurePlaceIDs:   placeIDsFromGeneratedIOPtr(workstation.OnFailure),
		})
	}

	return interfaces.InitialStructurePayload{
		Name:         string(factoryPayload.Name),
		Resources:    resources,
		Workers:      workers,
		WorkTypes:    workTypes,
		Workstations: workstations,
		Places:       places,
	}
}

func nilIfEmptyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func factoryWorkItemsFromGenerated(works *[]factoryapi.Work) []interfaces.FactoryWorkItem {
	if works == nil {
		return nil
	}
	out := make([]interfaces.FactoryWorkItem, 0, len(*works))
	for _, work := range *works {
		item := factoryWorkItemFromGenerated(work)
		if item.ID != "" {
			out = append(out, item)
		}
	}
	return out
}

func factoryWorkItemFromGenerated(work factoryapi.Work) interfaces.FactoryWorkItem {
	currentChainingTraceID := stringValue(work.CurrentChainingTraceId)
	traceID := stringValue(work.TraceId)
	if currentChainingTraceID == "" {
		currentChainingTraceID = traceID
	}
	return interfaces.FactoryWorkItem{
		ID:                       stringValue(work.WorkId),
		WorkTypeID:               stringValue(work.WorkTypeName),
		State:                    stringValue(work.State),
		DisplayName:              work.Name,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(sliceValue(work.PreviousChainingTraceIds)),
		TraceID:                  traceID,
		Tags:                     stringMapFromGenerated(work.Tags),
	}
}

func mergeFactoryWorkItem(existing interfaces.FactoryWorkItem, incoming interfaces.FactoryWorkItem) interfaces.FactoryWorkItem {
	if incoming.ID == "" {
		incoming.ID = existing.ID
	}
	if incoming.WorkTypeID == "" {
		incoming.WorkTypeID = existing.WorkTypeID
	}
	if incoming.State == "" {
		incoming.State = existing.State
	}
	if incoming.DisplayName == "" {
		incoming.DisplayName = existing.DisplayName
	}
	if incoming.TraceID == "" {
		incoming.TraceID = existing.TraceID
	}
	if incoming.CurrentChainingTraceID == "" {
		incoming.CurrentChainingTraceID = existing.CurrentChainingTraceID
	}
	if incoming.PreviousChainingTraceIDs == nil {
		incoming.PreviousChainingTraceIDs = append([]string(nil), existing.PreviousChainingTraceIDs...)
	}
	if incoming.ParentID == "" {
		incoming.ParentID = existing.ParentID
	}
	if incoming.PlaceID == "" {
		incoming.PlaceID = existing.PlaceID
	}
	if incoming.Tags == nil {
		incoming.Tags = cloneStringMap(existing.Tags)
	}
	return incoming
}

func (r *factoryWorldReducer) factoryRelationsFromGenerated(relations *[]factoryapi.Relation, context factoryapi.FactoryEventContext) []interfaces.FactoryRelation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.FactoryRelation, 0, len(*relations))
	for _, relation := range *relations {
		converted := r.factoryRelationFromGenerated(relation, context)
		if converted.TargetWorkID != "" {
			out = append(out, converted)
		}
	}
	return out
}

func (r *factoryWorldReducer) factoryRelationFromGenerated(relation factoryapi.Relation, context factoryapi.FactoryEventContext) interfaces.FactoryRelation {
	requestItems := r.requestWorkItems(stringValue(context.RequestId))
	targetWorkID := stringValue(relation.TargetWorkId)
	if targetWorkID == "" {
		targetWorkID = workIDForRequestName(requestItems, relation.TargetWorkName)
	}
	sourceWorkID := workIDForRequestName(requestItems, relation.SourceWorkName)
	if sourceWorkID == "" {
		sourceWorkID = sourceWorkIDFromContext(context, targetWorkID)
	}
	return interfaces.FactoryRelation{
		Type:           string(relation.Type),
		SourceWorkID:   sourceWorkID,
		SourceWorkName: relation.SourceWorkName,
		TargetWorkID:   targetWorkID,
		TargetWorkName: relation.TargetWorkName,
		RequiredState:  stringValue(relation.RequiredState),
		RequestID:      stringValue(context.RequestId),
		TraceID:        firstString(context.TraceIds),
	}
}

func (r *factoryWorldReducer) requestWorkItems(requestID string) []interfaces.FactoryWorkItem {
	if requestID == "" {
		return nil
	}
	return r.stateValue.WorkRequestsByID[requestID].WorkItems
}

func workIDForRequestName(items []interfaces.FactoryWorkItem, workName string) string {
	if workName == "" {
		return ""
	}
	for _, item := range items {
		if item.DisplayName == workName {
			return item.ID
		}
	}
	return ""
}

func sourceWorkIDFromContext(context factoryapi.FactoryEventContext, targetWorkID string) string {
	for _, workID := range sliceValue(context.WorkIds) {
		if workID != "" && workID != targetWorkID {
			return workID
		}
	}
	return ""
}

func (r *factoryWorldReducer) transitionIDForDispatch(dispatchID string) string {
	if dispatchID == "" {
		return ""
	}
	if dispatch, ok := r.stateValue.ActiveDispatches[dispatchID]; ok {
		return dispatch.TransitionID
	}
	for _, completion := range r.stateValue.CompletedDispatches {
		if completion.DispatchID == dispatchID {
			return completion.TransitionID
		}
	}
	for _, completion := range r.stateValue.FailedDispatches {
		if completion.DispatchID == dispatchID {
			return completion.TransitionID
		}
	}
	return ""
}

func (r *factoryWorldReducer) workstationRefForTransition(transitionID string) interfaces.FactoryWorkstationRef {
	workstation, ok := r.topologyWorkstation(transitionID)
	if !ok {
		return interfaces.FactoryWorkstationRef{ID: transitionID, Name: transitionID}
	}
	name := workstation.Name
	if name == "" {
		name = workstation.ID
	}
	return interfaces.FactoryWorkstationRef{ID: workstation.ID, Name: name}
}

func (r *factoryWorldReducer) workerForTransition(transitionID string) worldStateWorkerMetadata {
	workstation, ok := r.topologyWorkstation(transitionID)
	if !ok || workstation.WorkerID == "" {
		return worldStateWorkerMetadata{}
	}
	for _, worker := range r.stateValue.Topology.Workers {
		if worker.ID != workstation.WorkerID {
			continue
		}
		return worldStateWorkerMetadata{
			Provider: firstNonEmpty(worker.Provider, worker.ModelProvider),
			Model:    worker.Model,
		}
	}
	return worldStateWorkerMetadata{}
}

func (r *factoryWorldReducer) topologyWorkstation(transitionID string) (interfaces.FactoryWorkstation, bool) {
	for _, workstation := range r.stateValue.Topology.Workstations {
		if workstation.ID == transitionID || workstation.Name == transitionID {
			if workstation.ID == "" {
				workstation.ID = transitionID
			}
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstation{}, false
}

func (r *factoryWorldReducer) topologyPlace(placeID string) (interfaces.FactoryPlace, bool) {
	for _, place := range r.stateValue.Topology.Places {
		if place.ID == placeID {
			return place, true
		}
	}
	return interfaces.FactoryPlace{}, false
}

func resourceUnitsFromGenerated(resources *[]factoryapi.Resource) []interfaces.FactoryResourceUnit {
	if resources == nil {
		return nil
	}
	out := make([]interfaces.FactoryResourceUnit, 0, len(*resources))
	for _, resource := range *resources {
		if resource.Name == "" {
			continue
		}
		out = append(out, interfaces.FactoryResourceUnit{ResourceID: resource.Name})
	}
	return out
}

func workstationResultFromGenerated(payload factoryapi.DispatchResponseEventPayload) interfaces.WorkstationResult {
	return interfaces.WorkstationResult{
		Outcome:         string(payload.Outcome),
		Output:          stringValue(payload.Output),
		Error:           stringValue(payload.Error),
		Feedback:        stringValue(payload.Feedback),
		FailureReason:   stringValue(payload.FailureReason),
		FailureMessage:  stringValue(payload.FailureMessage),
		ProviderFailure: interfaces.ProviderFailureMetadataFromGenerated(payload.ProviderFailure),
	}
}

func placeIDsFromGeneratedIOs(values []factoryapi.WorkstationIO) []string {
	if len(values) == 0 {
		return nil
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, placeIDFromGeneratedIO(value))
	}
	return sortedStrings(ids)
}

func placeIDsFromGeneratedIOPtr(value *factoryapi.WorkstationIO) []string {
	if value == nil {
		return nil
	}
	return []string{placeIDFromGeneratedIO(*value)}
}

func placeIDFromGeneratedIO(value factoryapi.WorkstationIO) string {
	return generatedPlaceID(value.WorkType, value.State)
}

func generatedPlaceID(workTypeID string, stateValue string) string {
	if workTypeID == "" || stateValue == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", workTypeID, stateValue)
}

func firstRequestID(works *[]factoryapi.Work) string {
	for _, work := range sliceValue(works) {
		if requestID := stringValue(work.RequestId); requestID != "" {
			return requestID
		}
	}
	return ""
}

func firstString(values *[]string) string {
	for _, value := range sliceValue(values) {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringMapFromGenerated(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	return cloneStringMap(map[string]string(*values))
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func factoryStateString(value *factoryapi.FactoryState) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func workstationKindString(value *factoryapi.WorkstationKind) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func intPtrValue(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}
