package projections

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *factoryWorldReducer) applyDispatchCreated(event interfaces.FactoryEvent, payload interfaces.DispatchRequestEventPayload) {
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return
	}
	inputWorkIDs := dispatchInputWorkIDs(payload, event.Context.WorkIDs)
	workIDs := make([]string, 0, len(inputWorkIDs))
	traceIDs := make([]string, 0, len(inputWorkIDs))
	inputWorkItems := make([]workdomain.FactoryWorkItem, 0, len(inputWorkIDs))
	inputs := make([]interfaces.WorkstationInput, 0, len(inputWorkIDs))
	for _, workID := range inputWorkIDs {
		if workID == "" {
			continue
		}
		item, ok := r.stateValue.WorkItemsByID[workID]
		if !ok {
			item = workdomain.FactoryWorkItem{ID: workID}
		}
		if item.TraceID == "" {
			item.TraceID = firstString(event.Context.TraceIDs)
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

	worker := r.workerForTransition(payload.TransitionID)
	dispatch := interfaces.FactoryWorldDispatch{
		DispatchID:   dispatchID,
		TransitionID: payload.TransitionID,
		Workstation:  r.workstationRefForTransition(payload.TransitionID),
		Provider:     worker.Provider,
		Model:        worker.Model,
		StartedTick:  event.Context.Tick,
		StartedAt:    event.Context.EventTime,
		Inputs:       inputs,
		WorkItemIDs:  sortedStrings(workIDs),
		CurrentChainingTraceID: dispatchCurrentChainingTraceID(
			event.Context.CurrentChainingTraceID,
			payload.CurrentChainingTraceID,
			inputWorkItems,
		),
		PreviousChainingTraceIDs: dispatchPreviousChainingTraceIDs(
			event.Context.PreviousChainingTraceIDs,
			payload.PreviousChainingTraceIDs,
			inputWorkItems,
		),
		TraceIDs: work.CanonicalChainingTraceIDs(traceIDs),
	}
	dispatch.Resources = r.consumeResourceUnits(payload.Resources)
	r.stateValue.ActiveDispatches[dispatchID] = dispatch
	for _, traceID := range dispatch.TraceIDs {
		r.addTraceDispatch(traceID, dispatchID)
	}
}

func dispatchInputWorkIDs(payload interfaces.DispatchRequestEventPayload, contextWorkIDs *[]string) []string {
	ordered := make([]string, 0, len(payload.Inputs)+len(sliceValue(contextWorkIDs)))
	for _, ref := range payload.Inputs {
		ordered = appendUnique(ordered, ref.WorkID)
	}
	for _, workID := range sliceValue(contextWorkIDs) {
		ordered = appendUnique(ordered, workID)
	}
	return ordered
}

func (r *factoryWorldReducer) applyWorkerExecutionEvent(event interfaces.FactoryEvent) error {
	switch event.Type {
	case interfaces.FactoryEventTypeInferenceRequest:
		var payload workerexecution.InferenceRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		r.applyInferenceRequest(event, payload)
	case interfaces.FactoryEventTypeInferenceResponse:
		var payload workerexecution.InferenceResponseEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return r.applyInferenceResponse(event, payload)
	case interfaces.FactoryEventTypeScriptRequest:
		var payload workerexecution.ScriptRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		r.applyScriptRequest(event, payload)
	case interfaces.FactoryEventTypeScriptResponse:
		var payload workerexecution.ScriptResponseEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		r.applyScriptResponse(event, payload)
	case interfaces.FactoryEventTypeAgentRunResponse:
		var payload workerexecution.AgentRunResponseEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return err
		}
		return r.applyAgentRunResponse(event, payload)
	}
	return nil
}

func (r *factoryWorldReducer) applyInferenceRequest(event interfaces.FactoryEvent, payload workerexecution.InferenceRequestEventPayload) {
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" || payload.InferenceRequestID == "" {
		return
	}
	attempts := r.inferenceAttemptsForDispatch(dispatchID)
	current := attempts[payload.InferenceRequestID]
	current.DispatchID = dispatchID
	current.TransitionID = firstNonEmpty(current.TransitionID, r.transitionIDForDispatch(dispatchID))
	current.InferenceRequestID = payload.InferenceRequestID
	current.Attempt = payload.Attempt
	current.WorkingDirectory = payload.WorkingDirectory
	current.Worktree = payload.Worktree
	current.Prompt = payload.Prompt
	current.RequestTime = event.Context.EventTime
	attempts[payload.InferenceRequestID] = current
}

func (r *factoryWorldReducer) applyInferenceResponse(event interfaces.FactoryEvent, payload workerexecution.InferenceResponseEventPayload) error {
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" || payload.InferenceRequestID == "" {
		return nil
	}
	attempts := r.inferenceAttemptsForDispatch(dispatchID)
	current := attempts[payload.InferenceRequestID]
	current.DispatchID = dispatchID
	current.TransitionID = firstNonEmpty(current.TransitionID, r.transitionIDForDispatch(dispatchID))
	current.InferenceRequestID = payload.InferenceRequestID
	current.Attempt = payload.Attempt
	current.Outcome = string(payload.Outcome)
	current.Response = stringValue(payload.Response)
	current.DurationMillis = payload.DurationMillis
	current.ExitCode = intPtrValue(payload.ExitCode)
	if payload.FailureDetail != nil {
		current.FailureDetail = &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureType(payload.FailureDetail.Reason),
			Message: payload.FailureDetail.Message,
		}
	}
	current.ProviderSession = workerexecution.CloneProviderSessionMetadata(payload.ProviderSession)
	diagnostics, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(payload.Diagnostics)
	if err != nil {
		return err
	}
	current.Diagnostics = diagnostics
	current.ResponseTime = event.Context.EventTime
	attempts[payload.InferenceRequestID] = current
	return nil
}

func (r *factoryWorldReducer) applyScriptRequest(event interfaces.FactoryEvent, payload workerexecution.ScriptRequestEventPayload) {
	if payload.DispatchID == "" || payload.ScriptRequestID == "" {
		return
	}
	requests := r.scriptRequestsForDispatch(payload.DispatchID)
	current := requests[payload.ScriptRequestID]
	current.DispatchID = payload.DispatchID
	current.TransitionID = payload.TransitionID
	current.ScriptRequestID = payload.ScriptRequestID
	current.Attempt = payload.Attempt
	current.Command = payload.Command
	current.Args = cloneStringSlice(payload.Args)
	current.RequestTime = event.Context.EventTime
	requests[payload.ScriptRequestID] = current
}

func (r *factoryWorldReducer) applyScriptResponse(event interfaces.FactoryEvent, payload workerexecution.ScriptResponseEventPayload) {
	if payload.DispatchID == "" || payload.ScriptRequestID == "" {
		return
	}
	responses := r.scriptResponsesForDispatch(payload.DispatchID)
	current := responses[payload.ScriptRequestID]
	current.DispatchID = payload.DispatchID
	current.TransitionID = payload.TransitionID
	current.ScriptRequestID = payload.ScriptRequestID
	current.Attempt = payload.Attempt
	current.Outcome = string(payload.Outcome)
	current.Stdout = payload.Stdout
	current.Stderr = payload.Stderr
	current.DurationMillis = payload.DurationMillis
	current.ExitCode = intPtrValue(payload.ExitCode)
	current.FailureType = enumStringValue(payload.FailureType)
	current.ResponseTime = event.Context.EventTime
	responses[payload.ScriptRequestID] = current
}

func (r *factoryWorldReducer) applyAgentRunResponse(event interfaces.FactoryEvent, payload workerexecution.AgentRunResponseEventPayload) error {
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" || payload.AgentRunID == "" {
		return nil
	}
	diagnostics, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(payload.Diagnostics)
	if err != nil {
		return err
	}
	responses := r.agentRunResponsesForDispatch(dispatchID)
	responses[payload.AgentRunID] = interfaces.FactoryWorldAgentRunResponse{
		DispatchID:     dispatchID,
		AgentRunID:     payload.AgentRunID,
		Outcome:        string(payload.Outcome),
		DurationMillis: payload.DurationMillis,
		Diagnostics:    diagnostics,
		ResponseTime:   event.Context.EventTime,
	}
	return nil
}

func (r *factoryWorldReducer) agentRunResponsesForDispatch(dispatchID string) map[string]interfaces.FactoryWorldAgentRunResponse {
	responses := r.stateValue.AgentRunResponsesByDispatchID[dispatchID]
	if responses == nil {
		responses = make(map[string]interfaces.FactoryWorldAgentRunResponse)
		r.stateValue.AgentRunResponsesByDispatchID[dispatchID] = responses
	}
	return responses
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

func (r *factoryWorldReducer) applyDispatchCompleted(event interfaces.FactoryEvent, payload workerexecution.DispatchResponseEventPayload) {
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return
	}
	dispatch := r.stateValue.ActiveDispatches[dispatchID]
	delete(r.stateValue.ActiveDispatches, dispatchID)

	workIDs := append([]string(nil), dispatch.WorkItemIDs...)
	traceIDs := dispatchCompletionTraceIDs(dispatch, event.Context.TraceIDs)
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
	payload workerexecution.DispatchResponseEventPayload,
	workIDs []string,
	traceIDs []string,
) ([]workdomain.FactoryWorkItem, []string, []string) {
	outputWorkItems := make([]workdomain.FactoryWorkItem, 0, len(sliceValue(payload.OutputWork)))
	inputWorkItems := dispatchInputWorkItems(dispatch)
	for index, eventWork := range sliceValue(payload.OutputWork) {
		item := r.dispatchOutputWorkItem(dispatch, payload, factoryWorkItemFromEventWork(eventWork))
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
	payload workerexecution.DispatchResponseEventPayload,
	item workdomain.FactoryWorkItem,
) workdomain.FactoryWorkItem {
	if item.ID == "" {
		return workdomain.FactoryWorkItem{}
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
		} else if payload.Outcome == workerexecution.OutcomeContinue || payload.Outcome == workerexecution.OutcomeRejected {
			item.PlaceID = previousPlaceID
		} else if item.State != "" {
			item.PlaceID = r.placeForWorkTypeState(item.WorkTypeID, item.State)
		}
	}
	return item
}

func (r *factoryWorldReducer) dispatchCompletionFromResponse(
	event interfaces.FactoryEvent,
	payload workerexecution.DispatchResponseEventPayload,
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	workIDs []string,
	traceIDs []string,
	outputWorkItems []workdomain.FactoryWorkItem,
) interfaces.FactoryWorldDispatchCompletion {
	inputWorkItems := dispatchInputWorkItems(dispatch)
	latestAttempt := r.latestInferenceAttemptForDispatch(dispatchID)
	return interfaces.FactoryWorldDispatchCompletion{
		DispatchID:     dispatchID,
		TransitionID:   payload.TransitionID,
		Workstation:    dispatch.Workstation,
		StartedTick:    dispatch.StartedTick,
		CompletedTick:  event.Context.Tick,
		StartedAt:      dispatch.StartedAt,
		CompletedAt:    event.Context.EventTime,
		DurationMillis: int64Value(payload.DurationMillis),
		Result: interfaces.WorkstationResult{
			Outcome:                     string(payload.Outcome),
			Output:                      stringValue(payload.Output),
			Error:                       stringValue(payload.Error),
			Feedback:                    stringValue(payload.Feedback),
			SelectedClassificationLabel: stringValue(payload.SelectedClassificationLabel),
			FailureDetail:               workerexecution.CloneFailureDetail(payload.FailureDetail),
			FailureMetadata:             workerexecution.CloneWorkFailureMetadata(payload.ProviderFailure),
		},
		WorkItemIDs:     sortedStrings(workIDs),
		ConsumedInputs:  interfaces.CloneWorkstationInputs(dispatch.Inputs),
		InputWorkItems:  sortedWorkItems(inputWorkItems),
		OutputWorkItems: sortedWorkItems(outputWorkItems),
		CurrentChainingTraceID: completedDispatchCurrentChainingTraceID(
			event.Context.CurrentChainingTraceID,
			payload.CurrentChainingTraceID,
			dispatch,
			inputWorkItems,
		),
		PreviousChainingTraceIDs: completedDispatchPreviousChainingTraceIDs(
			event.Context.PreviousChainingTraceIDs,
			payload.PreviousChainingTraceIDs,
			dispatch,
			inputWorkItems,
		),
		TraceIDs:        work.CanonicalChainingTraceIDs(traceIDs),
		ProviderSession: latestInferenceProviderSession(latestAttempt),
		Diagnostics:     dispatchCompletionDiagnostics(r.latestAgentRunResponseForDispatch(dispatchID), latestAttempt),
		TerminalWork:    r.terminalWorkForCompletion(payload.Outcome, workIDs),
	}
}

func (r *factoryWorldReducer) recordDispatchCompletionState(
	dispatchID string,
	dispatch interfaces.FactoryWorldDispatch,
	payload workerexecution.DispatchResponseEventPayload,
	completion interfaces.FactoryWorldDispatchCompletion,
) {
	r.stateValue.CompletedDispatches = append(r.stateValue.CompletedDispatches, completion)
	if payload.Outcome == workerexecution.OutcomeFailed {
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
	payload workerexecution.DispatchResponseEventPayload,
	completion interfaces.FactoryWorldDispatchCompletion,
) {
	if completion.ProviderSession == nil || completion.ProviderSession.ID == "" {
		return
	}
	r.stateValue.ProviderSessions = append(r.stateValue.ProviderSessions, interfaces.FactoryWorldProviderSessionRecord{
		DispatchID:               completion.DispatchID,
		TransitionID:             payload.TransitionID,
		WorkstationName:          dispatch.Workstation.Name,
		Outcome:                  string(payload.Outcome),
		ProviderSession:          *workerexecution.CloneProviderSessionMetadata(completion.ProviderSession),
		WorkItemIDs:              completion.WorkItemIDs,
		ConsumedInputs:           interfaces.CloneWorkstationInputs(completion.ConsumedInputs),
		CurrentChainingTraceID:   completion.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(completion.PreviousChainingTraceIDs),
		TraceIDs:                 cloneStringSlice(completion.TraceIDs),
		Diagnostics:              workerdiagnostics.CloneSafeWorkDiagnostics(completion.Diagnostics),
		FailureDetail:            workerexecution.CloneFailureDetail(completion.Result.FailureDetail),
	})
}

func dispatchCurrentChainingTraceID(
	contextCurrent *string,
	payloadCurrent *string,
	inputWorkItems []workdomain.FactoryWorkItem,
) string {
	if current := stringValue(contextCurrent); current != "" {
		return current
	}
	if current := stringValue(payloadCurrent); current != "" {
		return current
	}
	return work.CurrentChainingTraceIDFromWorkItems(inputWorkItems)
}

func dispatchPreviousChainingTraceIDs(
	contextPrevious *[]string,
	payloadPrevious *[]string,
	inputWorkItems []workdomain.FactoryWorkItem,
) []string {
	if previous := cloneStringSlice(sliceValue(contextPrevious)); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	if previous := cloneStringSlice(sliceValue(payloadPrevious)); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	return work.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)
}

func completedDispatchCurrentChainingTraceID(
	contextCurrent *string,
	payloadCurrent *string,
	dispatch interfaces.FactoryWorldDispatch,
	inputWorkItems []workdomain.FactoryWorkItem,
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
	return work.CurrentChainingTraceIDFromWorkItems(inputWorkItems)
}

func completedDispatchPreviousChainingTraceIDs(
	contextPrevious *[]string,
	payloadPrevious *[]string,
	dispatch interfaces.FactoryWorldDispatch,
	inputWorkItems []workdomain.FactoryWorkItem,
) []string {
	if previous := cloneStringSlice(sliceValue(contextPrevious)); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	if previous := cloneStringSlice(sliceValue(payloadPrevious)); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	if len(dispatch.PreviousChainingTraceIDs) > 0 {
		return cloneStringSlice(dispatch.PreviousChainingTraceIDs)
	}
	return work.PreviousChainingTraceIDsFromWorkItems(inputWorkItems)
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
) []workdomain.FactoryWorkItem {
	items := make([]workdomain.FactoryWorkItem, 0, len(dispatch.Inputs))
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

func latestInferenceProviderSession(attempt *interfaces.FactoryWorldInferenceAttempt) *workerexecution.ProviderSessionMetadata {
	if attempt == nil {
		return nil
	}
	return workerexecution.CloneProviderSessionMetadata(attempt.ProviderSession)
}

func latestInferenceDiagnostics(attempt *interfaces.FactoryWorldInferenceAttempt) *workerdiagnostics.SafeWorkDiagnostics {
	if attempt == nil {
		return nil
	}
	return workerdiagnostics.CloneSafeWorkDiagnostics(attempt.Diagnostics)
}

func dispatchCompletionDiagnostics(
	agentRun *interfaces.FactoryWorldAgentRunResponse,
	attempt *interfaces.FactoryWorldInferenceAttempt,
) *workerdiagnostics.SafeWorkDiagnostics {
	if agentRun != nil && agentRun.Diagnostics != nil {
		return workerdiagnostics.CloneSafeWorkDiagnostics(agentRun.Diagnostics)
	}
	return latestInferenceDiagnostics(attempt)
}

func (r *factoryWorldReducer) latestAgentRunResponseForDispatch(dispatchID string) *interfaces.FactoryWorldAgentRunResponse {
	responses := r.stateValue.AgentRunResponsesByDispatchID[dispatchID]
	if len(responses) == 0 {
		return nil
	}
	var latest *interfaces.FactoryWorldAgentRunResponse
	for _, agentRunID := range sortedMapKeys(responses) {
		response := responses[agentRunID]
		if latest == nil ||
			response.ResponseTime.After(latest.ResponseTime) ||
			(response.ResponseTime.Equal(latest.ResponseTime) && response.AgentRunID > latest.AgentRunID) {
			responseCopy := response
			latest = &responseCopy
		}
	}
	return latest
}

func (r *factoryWorldReducer) recordWorkStateChange(event interfaces.FactoryEvent, payload interfaces.WorkStateChangeEventPayload) {
	workID := payload.WorkID
	if workID == "" {
		return
	}
	record := interfaces.FactoryWorldWorkStateChangeRecord{
		WorkID:       workID,
		WorkTypeName: payload.WorkTypeName,
		FromState:    payload.FromState,
		ToState:      payload.ToState,
		FromPlaceID:  payload.FromPlaceID,
		ToPlaceID:    payload.ToPlaceID,
		Source:       work.WorkStateChangeSource(payload.Source),
		RequestID:    stringValue(event.Context.RequestID),
		Tick:         event.Context.Tick,
		Sequence:     event.Context.Sequence,
		EventTime:    event.Context.EventTime,
	}
	r.stateValue.WorkStateChangesByWorkID[workID] = append(
		r.stateValue.WorkStateChangesByWorkID[workID],
		record,
	)
}

func (r *factoryWorldReducer) applyWorkStateChange(payload interfaces.WorkStateChangeEventPayload) {
	workID := payload.WorkID
	if workID == "" {
		return
	}

	item, ok := r.stateValue.WorkItemsByID[workID]
	if !ok {
		item = workdomain.FactoryWorkItem{ID: workID}
	}
	if payload.WorkTypeName != "" {
		item.WorkTypeID = firstNonEmpty(item.WorkTypeID, payload.WorkTypeName)
	}
	if payload.ToState != "" {
		item.State = payload.ToState
	}
	toPlaceID := payload.ToPlaceID
	if toPlaceID != "" {
		item.PlaceID = toPlaceID
	}

	fromPlaceID := payload.FromPlaceID
	if fromPlaceID != "" && fromPlaceID != toPlaceID {
		if r.isFailedPlace(fromPlaceID) {
			delete(r.stateValue.FailedWorkItemsByID, workID)
			r.removeTraceFailed(item.TraceID, workID)
		}
		if r.isTerminalPlace(fromPlaceID) {
			delete(r.stateValue.TerminalWorkByID, workID)
			r.removeTraceTerminal(item.TraceID, workID)
		}
	}

	r.stateValue.WorkItemsByID[workID] = item
	if toPlaceID != "" {
		r.addWorkToken(workID, toPlaceID, item)
	}
}

func (r *factoryWorldReducer) recordFailedWorkDetail(completion interfaces.FactoryWorldDispatchCompletion, item workdomain.FactoryWorkItem) {
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
		FailureDetail:   workerexecution.CloneFailureDetail(completion.Result.FailureDetail),
	}
}

func (r *factoryWorldReducer) applyDispatchLifecycleEvent(event interfaces.FactoryEvent) (bool, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchQueued:
		return true, r.applyDispatchQueuedEvent(event)
	case interfaces.FactoryEventTypeDispatchInterrupted:
		return true, r.applyDispatchInterruptedEvent(event)
	case interfaces.FactoryEventTypeDispatchReconciled:
		return true, r.applyDispatchReconciledEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyDispatchQueuedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.DispatchQueuedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return nil
	}
	state := interfaces.FactorySessionDispatchState{
		ID:             dispatchID,
		DispatchKind:   string(payload.DispatchKind),
		Status:         string(interfaces.FactoryDispatchStatusQueued),
		Phase:          dispatchLifecyclePhase(event.Context),
		Label:          stringValue(payload.Label),
		RunnerID:       stringValue(payload.RunnerID),
		Model:          stringValue(payload.Model),
		Provider:       stringValue(payload.Provider),
		PromptDigest:   stringValue(payload.PromptDigest),
		SchemaDigest:   stringValue(payload.SchemaDigest),
		RelatedWorkIDs: cloneStringSlice(sliceValue(payload.InputWorkIDs)),
	}
	if payload.DispatchKind == interfaces.FactoryDispatchKindPetriTransition {
		state.Petri = &interfaces.FactorySessionDispatchPetriState{
			TransitionID: dispatchID,
		}
	} else {
		state.JavaScript = &interfaces.FactorySessionDispatchJavaScriptState{
			TaskKind:  javaScriptTaskKindFromDispatchKind(payload.DispatchKind),
			TaskLabel: stringValue(payload.Label),
		}
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) applyDispatchInterruptedEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.DispatchInterruptedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return nil
	}
	if r.interruptedDispatchIDs == nil {
		r.interruptedDispatchIDs = make(map[string]struct{})
	}
	r.interruptedDispatchIDs[dispatchID] = struct{}{}
	state := interfaces.FactorySessionDispatchState{
		ID:     dispatchID,
		Status: string(interfaces.FactoryDispatchStatusInterrupted),
		Phase:  dispatchLifecyclePhase(event.Context),
	}
	if payload.Reason != "" {
		state.FailureDetail = &interfaces.FactorySessionDispatchFailureDetail{
			Message: payload.Reason,
		}
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) applyDispatchReconciledEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.DispatchReconciledEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return err
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return nil
	}
	if r.dispatchInterrupted(dispatchID) && !payload.Replayed {
		return nil
	}
	state := interfaces.FactorySessionDispatchState{
		ID:          dispatchID,
		Status:      string(payload.ReconciledStatus),
		Phase:       dispatchLifecyclePhase(event.Context),
		ArtifactIDs: cloneStringSlice(sliceValue(payload.ArtifactIDs)),
	}
	if payload.Usage != nil {
		state.Usage = &interfaces.FactorySessionDispatchUsage{
			InputTokens:    int64Value(payload.Usage.InputTokens),
			OutputTokens:   int64Value(payload.Usage.OutputTokens),
			TotalTokens:    int64Value(payload.Usage.TotalTokens),
			CostUSD:        float64Value(payload.Usage.CostUSD),
			DurationMillis: int64Value(payload.Usage.DurationMillis),
			RetryCount:     int32Value(payload.Usage.RetryCount),
		}
	}
	if payload.FailureDetail != nil {
		state.FailureDetail = &interfaces.FactorySessionDispatchFailureDetail{
			Reason:  string(payload.FailureDetail.Reason),
			Message: payload.FailureDetail.Message,
		}
	}
	if payload.ResultArtifactRef != nil {
		state.ArtifactIDs = appendUnique(state.ArtifactIDs, payload.ResultArtifactRef.ID)
	}
	r.upsertJavaScriptDispatch(state)
	return nil
}

func (r *factoryWorldReducer) upsertJavaScriptDispatch(state interfaces.FactorySessionDispatchState) {
	if strings.TrimSpace(state.ID) == "" {
		return
	}
	runtime := r.ensureJavaScriptRuntime()
	for index, existing := range runtime.Dispatches {
		if existing.ID != state.ID {
			continue
		}
		runtime.Dispatches[index] = mergeJavaScriptDispatchState(existing, state)
		r.recountJavaScriptDispatchTotals()
		return
	}
	runtime.Dispatches = append(runtime.Dispatches, state)
	r.recountJavaScriptDispatchTotals()
}

// pkgmaintcheck:ignore-cyclomatic-complexity dispatch replay merge keeps JavaScript dispatch field updates together for queue/interrupt/reconcile states.
func mergeJavaScriptDispatchState(
	existing interfaces.FactorySessionDispatchState,
	incoming interfaces.FactorySessionDispatchState,
) interfaces.FactorySessionDispatchState {
	merged := existing
	if incoming.DispatchKind != "" {
		merged.DispatchKind = incoming.DispatchKind
	}
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if incoming.Phase != "" {
		merged.Phase = incoming.Phase
	}
	if incoming.Label != "" {
		merged.Label = incoming.Label
	}
	if incoming.RunnerID != "" {
		merged.RunnerID = incoming.RunnerID
	}
	if incoming.Model != "" {
		merged.Model = incoming.Model
	}
	if incoming.Provider != "" {
		merged.Provider = incoming.Provider
	}
	if incoming.PromptDigest != "" {
		merged.PromptDigest = incoming.PromptDigest
	}
	if incoming.SchemaDigest != "" {
		merged.SchemaDigest = incoming.SchemaDigest
	}
	if len(incoming.RelatedWorkIDs) > 0 {
		merged.RelatedWorkIDs = cloneStringSlice(incoming.RelatedWorkIDs)
	}
	if len(incoming.ArtifactIDs) > 0 {
		merged.ArtifactIDs = cloneStringSlice(merged.ArtifactIDs)
		for _, artifactID := range incoming.ArtifactIDs {
			merged.ArtifactIDs = appendUnique(merged.ArtifactIDs, artifactID)
		}
	}
	if incoming.Usage != nil {
		merged.Usage = incoming.Usage
	}
	if incoming.FailureDetail != nil {
		merged.FailureDetail = incoming.FailureDetail
	}
	if incoming.Petri != nil {
		merged.Petri = incoming.Petri
	}
	if incoming.JavaScript != nil {
		merged.JavaScript = incoming.JavaScript
	}
	return merged
}

func (r *factoryWorldReducer) dispatchInterrupted(dispatchID string) bool {
	if r == nil || len(r.interruptedDispatchIDs) == 0 {
		return false
	}
	_, ok := r.interruptedDispatchIDs[strings.TrimSpace(dispatchID)]
	return ok
}

func (r *factoryWorldReducer) recountJavaScriptDispatchTotals() {
	runtime := r.ensureJavaScriptRuntime()
	var queued, running, completed int
	for _, dispatch := range runtime.Dispatches {
		switch interfaces.FactoryDispatchStatus(strings.TrimSpace(dispatch.Status)) {
		case interfaces.FactoryDispatchStatusQueued:
			queued++
		case interfaces.FactoryDispatchStatusRunning:
			running++
		case interfaces.FactoryDispatchStatusCompleted:
			completed++
		}
	}
	runtime.QueuedDispatches = queued
	runtime.RunningDispatches = running
	runtime.CompletedDispatches = completed
}

func dispatchLifecyclePhase(context interfaces.FactoryEventContext) string {
	if phase := stringValue(context.PhaseName); phase != "" {
		return phase
	}
	return stringValue(context.PhaseID)
}

func javaScriptTaskKindFromDispatchKind(kind interfaces.FactoryDispatchKind) string {
	switch kind {
	case interfaces.FactoryDispatchKindJavaScriptVerify:
		return "VERIFY"
	case interfaces.FactoryDispatchKindJavaScriptSynthesize:
		return "SYNTHESIZE"
	case interfaces.FactoryDispatchKindJavaScriptTool:
		return "TOOL"
	case interfaces.FactoryDispatchKindJavaScriptScript:
		return "SCRIPT"
	case interfaces.FactoryDispatchKindJavaScriptSystem:
		return "SYSTEM"
	default:
		return "AGENT"
	}
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func int32Value(value *int32) int {
	if value == nil {
		return 0
	}
	return int(*value)
}
