package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
		r.stateValue.PayloadLineage.RecordConsumedInputSnapshot(dispatchID, item)
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
	outputWorkItems, workIDs, traceIDs := r.applyDispatchOutputWork(event.Context.Tick, dispatch, payload, workIDs, traceIDs)
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
	observedTick int,
	dispatch interfaces.FactoryWorldDispatch,
	payload factoryapi.DispatchResponseEventPayload,
	workIDs []string,
	traceIDs []string,
) ([]interfaces.FactoryWorkItem, []string, []string) {
	outputWorkItems := make([]interfaces.FactoryWorkItem, 0, len(sliceValue(payload.OutputWork)))
	inputWorkItems := dispatchInputWorkItems(dispatch)
	for index, work := range sliceValue(payload.OutputWork) {
		item := r.dispatchOutputWorkItem(dispatch, payload, work)
		if item.ID == "" {
			continue
		}
		r.stateValue.PayloadLineage.RecordDispatchOutputSnapshot(
			observedTick,
			dispatch.DispatchID,
			inputWorkItems,
			item,
			index,
		)
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
		ConsumedInputs:  interfaces.CloneWorkstationInputs(dispatch.Inputs),
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
		ConsumedInputs:           interfaces.CloneWorkstationInputs(completion.ConsumedInputs),
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
