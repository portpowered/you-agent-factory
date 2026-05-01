package api

import (
	"sort"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuildFactoryWorldWorkstationRequestProjectionSlice keeps the additive
// workstation-request contract at the API boundary while deriving it from the
// canonical selected-tick FactoryWorldState model.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	dispatchViewsByID := buildFactoryWorldWorkstationDispatchViewsByID(state)
	if len(dispatchViewsByID) == 0 {
		return factoryapi.FactoryWorldWorkstationRequestProjectionSlice{}
	}
	return factoryapi.FactoryWorldWorkstationRequestProjectionSlice{
		WorkstationRequestsByDispatchId: &dispatchViewsByID,
	}
}

func buildFactoryWorldWorkstationDispatchViewsByID(
	state interfaces.FactoryWorldState,
) map[string]factoryapi.FactoryWorldWorkstationRequestView {
	dispatchIDs := make(map[string]struct{})
	completedByID := make(map[string]interfaces.FactoryWorldDispatchCompletion)
	for dispatchID, dispatch := range state.ActiveDispatches {
		if dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			dispatchIDs[dispatchID] = struct{}{}
		}
	}
	for _, completion := range state.CompletedDispatches {
		if !dispatchHasCustomerWork(completion.WorkItemIDs, state.WorkItemsByID) {
			continue
		}
		dispatchIDs[completion.DispatchID] = struct{}{}
		completedByID[completion.DispatchID] = completion
	}
	if len(dispatchIDs) == 0 {
		return nil
	}

	dispatchViewsByID := make(map[string]factoryapi.FactoryWorldWorkstationRequestView, len(dispatchIDs))
	for _, dispatchID := range sortedMapKeys(dispatchIDs) {
		latestAttempt := latestWorkstationInferenceAttempt(state.InferenceAttemptsByDispatchID[dispatchID])
		latestScriptResponse := latestWorkstationScriptResponse(state.ScriptResponsesByDispatchID[dispatchID])
		latestScriptRequest := workstationScriptRequestForProjection(latestScriptResponse, state.ScriptRequestsByDispatchID[dispatchID])
		if dispatch, ok := state.ActiveDispatches[dispatchID]; ok && dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			dispatchViewsByID[dispatchID] = workstationDispatchViewFromActiveDispatch(
				dispatch,
				state,
				latestAttempt,
				latestScriptRequest,
				latestScriptResponse,
			)
		}
		if completion, ok := completedByID[dispatchID]; ok {
			dispatchViewsByID[dispatchID] = workstationDispatchViewFromCompletion(
				completion,
				state,
				latestAttempt,
				latestScriptRequest,
				latestScriptResponse,
			)
		}
		view, ok := dispatchViewsByID[dispatchID]
		if !ok {
			continue
		}
		view.Counts = buildFactoryWorldWorkstationRequestCounts(
			state.InferenceAttemptsByDispatchID[dispatchID],
			state.ScriptRequestsByDispatchID[dispatchID],
			state.ScriptResponsesByDispatchID[dispatchID],
		)
		dispatchViewsByID[dispatchID] = view
	}
	if len(dispatchViewsByID) == 0 {
		return nil
	}
	return dispatchViewsByID
}

func workstationDispatchViewFromActiveDispatch(
	dispatch interfaces.FactoryWorldDispatch,
	state interfaces.FactoryWorldState,
	latestAttempt *interfaces.FactoryWorldInferenceAttempt,
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) factoryapi.FactoryWorldWorkstationRequestView {
	inputWorkItems := generatedWorkItemRefs(workItemRefsForInputs(dispatch.Inputs))
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID))
	}
	return factoryapi.FactoryWorldWorkstationRequestView{
		DispatchId:      dispatch.DispatchID,
		TransitionId:    dispatch.TransitionID,
		WorkstationName: workstationRequestStringPtr(workstationNameOrID(dispatch.Workstation.Name, dispatch.TransitionID)),
		Request: workstationDispatchRequestView(
			dispatch.StartedAt,
			inputWorkItems,
			dispatch.CurrentChainingTraceID,
			dispatch.PreviousChainingTraceIDs,
			sortedStrings(dispatch.TraceIDs),
			generatedTokenViewsFromInputs(dispatch.Inputs),
			latestAttempt,
			latestScriptRequest,
			inferenceAttemptProviderSession(latestAttempt),
			dispatch.Provider,
			dispatch.Model,
			inferenceAttemptDiagnostics(latestAttempt),
		),
		Response: workstationRequestResponseViewFromActiveDispatch(latestScriptResponse),
	}
}

func workstationRequestResponseViewFromActiveDispatch(
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) *factoryapi.FactoryWorldWorkstationRequestResponseView {
	if latestScriptResponse == nil {
		return nil
	}
	return &factoryapi.FactoryWorldWorkstationRequestResponseView{
		ScriptResponse: generatedFactoryWorldScriptResponse(latestScriptResponse),
	}
}

func workstationDispatchViewFromCompletion(
	completion interfaces.FactoryWorldDispatchCompletion,
	state interfaces.FactoryWorldState,
	latestAttempt *interfaces.FactoryWorldInferenceAttempt,
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) factoryapi.FactoryWorldWorkstationRequestView {
	inputWorkItems := generatedWorkItemRefs(workItemRefsForItems(completion.InputWorkItems))
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForInputs(completion.ConsumedInputs))
	}
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForIDs(completion.WorkItemIDs, state.WorkItemsByID))
	}
	outputWorkItems := generatedWorkItemRefs(workItemRefsForItems(completion.OutputWorkItems))
	if len(outputWorkItems) == 0 && completion.TerminalWork != nil && !interfaces.IsSystemTimeWorkType(completion.TerminalWork.WorkItem.WorkTypeID) {
		outputWorkItems = generatedWorkItemRefs([]interfaces.FactoryWorldWorkItemRef{
			workItemRef(completion.TerminalWork.WorkItem),
		})
	}
	return factoryapi.FactoryWorldWorkstationRequestView{
		DispatchId:      completion.DispatchID,
		TransitionId:    completion.TransitionID,
		WorkstationName: workstationRequestStringPtr(workstationNameOrID(completion.Workstation.Name, completion.TransitionID)),
		Request: workstationDispatchRequestView(
			completion.StartedAt,
			inputWorkItems,
			completion.CurrentChainingTraceID,
			completion.PreviousChainingTraceIDs,
			sortedStrings(completion.TraceIDs),
			generatedTokenViewsFromInputs(completion.ConsumedInputs),
			latestAttempt,
			latestScriptRequest,
			inferenceAttemptProviderSessionOrFallback(latestAttempt, completion.ProviderSession),
			"",
			"",
			inferenceAttemptDiagnosticsOrFallback(latestAttempt, completion.Diagnostics),
		),
		Response: &factoryapi.FactoryWorldWorkstationRequestResponseView{
			Outcome:          workstationRequestStringPtr(completion.Result.Outcome),
			Feedback:         workstationRequestStringPtr(completion.Result.Feedback),
			FailureReason:    workstationRequestStringPtr(completion.Result.FailureReason),
			FailureMessage:   workstationRequestStringPtr(completion.Result.FailureMessage),
			ResponseText:     workstationRequestStringPtr(workstationResponseText(latestAttempt, latestScriptResponse, completion.Result.Output)),
			ErrorClass:       workstationRequestStringPtr(workstationErrorClass(latestAttempt)),
			ProviderSession:  interfaces.GeneratedProviderSessionMetadata(inferenceAttemptProviderSessionOrFallback(latestAttempt, completion.ProviderSession)),
			Diagnostics:      generatedFactoryWorldWorkDiagnostics(inferenceAttemptDiagnosticsOrFallback(latestAttempt, completion.Diagnostics)),
			ResponseMetadata: workstationRequestStringMapPtr(workstationResponseMetadata(inferenceAttemptDiagnosticsOrFallback(latestAttempt, completion.Diagnostics))),
			ScriptResponse:   generatedFactoryWorldScriptResponse(latestScriptResponse),
			EndTime:          timePtr(completion.CompletedAt),
			DurationMillis:   int64Ptr(completion.DurationMillis),
			OutputWorkItems:  workItemRefSlicePtr(outputWorkItems),
			OutputMutations:  mutationViewsPtrForCompletion(completion),
		},
	}
}

func workstationDispatchRequestView(
	startedAt time.Time,
	inputWorkItems []factoryapi.FactoryWorldWorkItemRef,
	currentChainingTraceID string,
	previousChainingTraceIDs []string,
	traceIDs []string,
	consumedTokens []factoryapi.FactoryWorldTokenView,
	latestAttempt *interfaces.FactoryWorldInferenceAttempt,
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
	providerSession *interfaces.ProviderSessionMetadata,
	fallbackProvider string,
	fallbackModel string,
	diagnostics *interfaces.SafeWorkDiagnostics,
) factoryapi.FactoryWorldWorkstationRequestRequestView {
	provider, model := workstationRequestProviderModel(diagnostics, providerSession, fallbackProvider, fallbackModel)
	return factoryapi.FactoryWorldWorkstationRequestRequestView{
		StartedAt:                timePtr(startedAt),
		RequestTime:              timePtr(workstationRequestTime(latestAttempt)),
		InputWorkItems:           workItemRefSlicePtr(inputWorkItems),
		InputWorkTypeIds:         stringSlicePtr(workTypeIDsForWorkRefs(inputWorkItems)),
		CurrentChainingTraceId:   workstationRequestStringPtr(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(sortedStrings(previousChainingTraceIDs)),
		TraceIds:                 stringSlicePtr(traceIDs),
		ConsumedTokens:           tokenViewSlicePtr(consumedTokens),
		Prompt:                   workstationRequestStringPtr(workstationRequestPrompt(latestAttempt)),
		WorkingDirectory:         workstationRequestStringPtr(workstationWorkingDirectory(latestAttempt, diagnostics)),
		Worktree:                 workstationRequestStringPtr(workstationWorktree(latestAttempt, diagnostics)),
		Provider:                 workstationRequestStringPtr(provider),
		Model:                    workstationRequestStringPtr(model),
		ScriptRequest:            generatedFactoryWorldScriptRequest(latestScriptRequest),
		RequestMetadata:          workstationRequestStringMapPtr(workstationRequestMetadata(diagnostics)),
	}
}

func latestWorkstationInferenceAttempt(
	attempts map[string]interfaces.FactoryWorldInferenceAttempt,
) *interfaces.FactoryWorldInferenceAttempt {
	if len(attempts) == 0 {
		return nil
	}
	var latest *interfaces.FactoryWorldInferenceAttempt
	for _, requestID := range sortedMapKeys(attempts) {
		attempt := attempts[requestID]
		if latest == nil ||
			attempt.Attempt > latest.Attempt ||
			(attempt.Attempt == latest.Attempt && attempt.RequestTime.After(latest.RequestTime)) ||
			(attempt.Attempt == latest.Attempt && attempt.RequestTime.Equal(latest.RequestTime) && attempt.InferenceRequestID > latest.InferenceRequestID) {
			attemptCopy := attempt
			latest = &attemptCopy
		}
	}
	return latest
}

func buildFactoryWorldWorkstationRequestCounts(
	attempts map[string]interfaces.FactoryWorldInferenceAttempt,
	scriptRequests map[string]interfaces.FactoryWorldScriptRequest,
	scriptResponses map[string]interfaces.FactoryWorldScriptResponse,
) factoryapi.FactoryWorldWorkstationRequestCountView {
	counts := factoryapi.FactoryWorldWorkstationRequestCountView{}
	for _, requestID := range sortedMapKeys(attempts) {
		attempt := attempts[requestID]
		if attempt.InferenceRequestID != "" {
			counts.DispatchedCount++
		}
		if attempt.ResponseTime.IsZero() {
			continue
		}
		if attempt.ErrorClass != "" || attempt.Outcome == "FAILED" {
			counts.ErroredCount++
			continue
		}
		counts.RespondedCount++
	}
	for _, requestID := range sortedMapKeys(scriptRequests) {
		if scriptRequests[requestID].ScriptRequestID != "" {
			counts.DispatchedCount++
		}
	}
	for _, requestID := range sortedMapKeys(scriptResponses) {
		response := scriptResponses[requestID]
		if response.ResponseTime.IsZero() {
			continue
		}
		if scriptResponseErrored(response) {
			counts.ErroredCount++
			continue
		}
		counts.RespondedCount++
	}
	return counts
}

func workstationRequestPrompt(attempt *interfaces.FactoryWorldInferenceAttempt) string {
	if attempt == nil {
		return ""
	}
	return attempt.Prompt
}

func inferenceAttemptProviderSession(attempt *interfaces.FactoryWorldInferenceAttempt) *interfaces.ProviderSessionMetadata {
	if attempt == nil {
		return nil
	}
	return attempt.ProviderSession
}

func inferenceAttemptProviderSessionOrFallback(
	attempt *interfaces.FactoryWorldInferenceAttempt,
	fallback *interfaces.ProviderSessionMetadata,
) *interfaces.ProviderSessionMetadata {
	if session := inferenceAttemptProviderSession(attempt); session != nil {
		return session
	}
	return fallback
}

func inferenceAttemptDiagnostics(attempt *interfaces.FactoryWorldInferenceAttempt) *interfaces.SafeWorkDiagnostics {
	if attempt == nil {
		return nil
	}
	return attempt.Diagnostics
}

func inferenceAttemptDiagnosticsOrFallback(
	attempt *interfaces.FactoryWorldInferenceAttempt,
	fallback *interfaces.SafeWorkDiagnostics,
) *interfaces.SafeWorkDiagnostics {
	if diagnostics := inferenceAttemptDiagnostics(attempt); diagnostics != nil {
		return diagnostics
	}
	return fallback
}

func workstationRequestTime(attempt *interfaces.FactoryWorldInferenceAttempt) time.Time {
	if attempt == nil {
		return time.Time{}
	}
	return attempt.RequestTime
}

func workstationWorkingDirectory(attempt *interfaces.FactoryWorldInferenceAttempt, diagnostics *interfaces.SafeWorkDiagnostics) string {
	if attempt != nil && attempt.WorkingDirectory != "" {
		return attempt.WorkingDirectory
	}
	return workstationProviderRequestMetadataValue(diagnostics, "working_directory")
}

func workstationWorktree(attempt *interfaces.FactoryWorldInferenceAttempt, diagnostics *interfaces.SafeWorkDiagnostics) string {
	if attempt != nil && attempt.Worktree != "" {
		return attempt.Worktree
	}
	return workstationProviderRequestMetadataValue(diagnostics, "worktree")
}

func workstationRequestProviderModel(
	diagnostics *interfaces.SafeWorkDiagnostics,
	providerSession *interfaces.ProviderSessionMetadata,
	fallbackProvider string,
	fallbackModel string,
) (string, string) {
	if diagnostics != nil && diagnostics.Provider != nil {
		return firstNonEmpty(diagnostics.Provider.Provider, providerSessionProvider(providerSession), fallbackProvider),
			firstNonEmpty(diagnostics.Provider.Model, fallbackModel)
	}
	return firstNonEmpty(providerSessionProvider(providerSession), fallbackProvider), fallbackModel
}

func workstationProviderRequestMetadataValue(diagnostics *interfaces.SafeWorkDiagnostics, key string) string {
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.RequestMetadata == nil {
		return ""
	}
	return diagnostics.Provider.RequestMetadata[key]
}

func workstationRequestMetadata(diagnostics *interfaces.SafeWorkDiagnostics) map[string]string {
	if diagnostics == nil || diagnostics.Provider == nil {
		return nil
	}
	return cloneStringMap(diagnostics.Provider.RequestMetadata)
}

func workstationResponseMetadata(diagnostics *interfaces.SafeWorkDiagnostics) map[string]string {
	if diagnostics == nil || diagnostics.Provider == nil {
		return nil
	}
	return cloneStringMap(diagnostics.Provider.ResponseMetadata)
}

func workstationResponseText(
	attempt *interfaces.FactoryWorldInferenceAttempt,
	scriptResponse *interfaces.FactoryWorldScriptResponse,
	fallback string,
) string {
	if attempt != nil && attempt.Response != "" {
		return attempt.Response
	}
	if scriptResponse != nil {
		return ""
	}
	return fallback
}

func workstationErrorClass(attempt *interfaces.FactoryWorldInferenceAttempt) string {
	if attempt == nil {
		return ""
	}
	return attempt.ErrorClass
}

func latestWorkstationScriptRequest(
	requests map[string]interfaces.FactoryWorldScriptRequest,
) *interfaces.FactoryWorldScriptRequest {
	if len(requests) == 0 {
		return nil
	}
	var latest *interfaces.FactoryWorldScriptRequest
	for _, requestID := range sortedMapKeys(requests) {
		request := requests[requestID]
		if latest == nil ||
			request.Attempt > latest.Attempt ||
			(request.Attempt == latest.Attempt && request.RequestTime.After(latest.RequestTime)) ||
			(request.Attempt == latest.Attempt && request.RequestTime.Equal(latest.RequestTime) && request.ScriptRequestID > latest.ScriptRequestID) {
			requestCopy := request
			requestCopy.Args = cloneStringSlice(request.Args)
			latest = &requestCopy
		}
	}
	return latest
}

func latestWorkstationScriptResponse(
	responses map[string]interfaces.FactoryWorldScriptResponse,
) *interfaces.FactoryWorldScriptResponse {
	if len(responses) == 0 {
		return nil
	}
	var latest *interfaces.FactoryWorldScriptResponse
	for _, requestID := range sortedMapKeys(responses) {
		response := responses[requestID]
		if latest == nil ||
			response.Attempt > latest.Attempt ||
			(response.Attempt == latest.Attempt && response.ResponseTime.After(latest.ResponseTime)) ||
			(response.Attempt == latest.Attempt && response.ResponseTime.Equal(latest.ResponseTime) && response.ScriptRequestID > latest.ScriptRequestID) {
			responseCopy := response
			responseCopy.ExitCode = cloneIntPtr(response.ExitCode)
			latest = &responseCopy
		}
	}
	return latest
}

func workstationScriptRequestForProjection(
	response *interfaces.FactoryWorldScriptResponse,
	requests map[string]interfaces.FactoryWorldScriptRequest,
) *interfaces.FactoryWorldScriptRequest {
	if response != nil {
		if request, ok := requests[response.ScriptRequestID]; ok {
			requestCopy := request
			requestCopy.Args = cloneStringSlice(request.Args)
			return &requestCopy
		}
	}
	return latestWorkstationScriptRequest(requests)
}

func scriptResponseErrored(response interfaces.FactoryWorldScriptResponse) bool {
	if response.FailureType != "" {
		return true
	}
	switch response.Outcome {
	case string(factoryapi.ScriptExecutionOutcomeFailedExitCode),
		string(factoryapi.ScriptExecutionOutcomeProcessError),
		string(factoryapi.ScriptExecutionOutcomeTimedOut):
		return true
	default:
		return false
	}
}

func dispatchHasCustomerWork(ids []string, items map[string]interfaces.FactoryWorkItem) bool {
	return len(workItemRefsForIDs(ids, items)) > 0
}

func generatedWorkItemRefs(refs []interfaces.FactoryWorldWorkItemRef) []factoryapi.FactoryWorldWorkItemRef {
	out := make([]factoryapi.FactoryWorldWorkItemRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, factoryapi.FactoryWorldWorkItemRef{
			WorkId:                   ref.WorkID,
			WorkTypeId:               workstationRequestStringPtr(ref.WorkTypeID),
			DisplayName:              workstationRequestStringPtr(ref.DisplayName),
			CurrentChainingTraceId:   workstationRequestStringPtr(ref.CurrentChainingTraceID),
			PreviousChainingTraceIds: stringSlicePtr(sortedStrings(ref.PreviousChainingTraceIDs)),
			TraceId:                  workstationRequestStringPtr(ref.TraceID),
		})
	}
	return out
}

func workItemRefsForIDs(
	ids []string,
	items map[string]interfaces.FactoryWorkItem,
) []interfaces.FactoryWorldWorkItemRef {
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

func workItemRefsForItems(items []interfaces.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		refs = append(refs, workItemRef(item))
		seen[item.ID] = struct{}{}
	}
	return refs
}

func workItemRefsForInputs(inputs []interfaces.WorkstationInput) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		if _, exists := seen[input.WorkItem.ID]; exists {
			continue
		}
		refs = append(refs, workItemRef(*input.WorkItem))
		seen[input.WorkItem.ID] = struct{}{}
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

func workTypeIDsForWorkRefs(refs []factoryapi.FactoryWorldWorkItemRef) []string {
	var ids []string
	for _, ref := range refs {
		if ref.WorkTypeId == nil {
			continue
		}
		ids = appendUnique(ids, *ref.WorkTypeId)
	}
	return sortedStrings(ids)
}

func generatedTokenViewsFromInputs(inputs []interfaces.WorkstationInput) []factoryapi.FactoryWorldTokenView {
	out := make([]factoryapi.FactoryWorldTokenView, 0, len(inputs))
	for _, input := range inputs {
		view := factoryapi.FactoryWorldTokenView{
			TokenId: input.TokenID,
			PlaceId: input.PlaceID,
		}
		if input.WorkItem != nil {
			currentChainingTraceID := input.WorkItem.CurrentChainingTraceID
			if currentChainingTraceID == "" {
				currentChainingTraceID = input.WorkItem.TraceID
			}
			view.Name = workstationRequestStringPtr(input.WorkItem.DisplayName)
			view.WorkId = workstationRequestStringPtr(input.WorkItem.ID)
			view.WorkTypeId = workstationRequestStringPtr(input.WorkItem.WorkTypeID)
			view.CurrentChainingTraceId = workstationRequestStringPtr(currentChainingTraceID)
			view.PreviousChainingTraceIds = stringSlicePtr(sortedStrings(input.WorkItem.PreviousChainingTraceIDs))
			view.TraceId = workstationRequestStringPtr(input.WorkItem.TraceID)
			view.Tags = workstationRequestStringMapPtr(cloneStringMap(input.WorkItem.Tags))
		}
		out = append(out, view)
	}
	return out
}

func mutationViewsForCompletion(
	dispatch interfaces.FactoryWorldDispatchCompletion,
) []factoryapi.FactoryWorldMutationView {
	if len(dispatch.OutputWorkItems) == 0 {
		return nil
	}
	inputsByWorkID := make(map[string]interfaces.WorkstationInput, len(dispatch.ConsumedInputs))
	for _, input := range dispatch.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" {
			continue
		}
		inputsByWorkID[input.WorkItem.ID] = input
	}
	views := make([]factoryapi.FactoryWorldMutationView, 0, len(dispatch.OutputWorkItems))
	seen := make(map[string]struct{}, len(dispatch.OutputWorkItems))
	for _, item := range dispatch.OutputWorkItems {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		input := inputsByWorkID[item.ID]
		views = append(views, factoryapi.FactoryWorldMutationView{
			Type:      mutationTypeForOutput(input, item),
			TokenId:   mutationTokenID(input, item),
			FromPlace: workstationRequestStringPtr(input.PlaceID),
			ToPlace:   workstationRequestStringPtr(item.PlaceID),
			Token:     generatedTokenViewForWorkItem(mutationTokenID(input, item), item),
		})
	}
	if len(views) == 0 {
		return nil
	}
	return views
}

func mutationViewsPtrForCompletion(
	dispatch interfaces.FactoryWorldDispatchCompletion,
) *[]factoryapi.FactoryWorldMutationView {
	views := mutationViewsForCompletion(dispatch)
	if len(views) == 0 {
		return nil
	}
	return &views
}

func generatedTokenViewForWorkItem(tokenID string, item interfaces.FactoryWorkItem) *factoryapi.FactoryWorldTokenView {
	if tokenID == "" {
		tokenID = item.ID
	}
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return &factoryapi.FactoryWorldTokenView{
		TokenId:                  tokenID,
		PlaceId:                  item.PlaceID,
		Name:                     workstationRequestStringPtr(item.DisplayName),
		WorkId:                   workstationRequestStringPtr(item.ID),
		WorkTypeId:               workstationRequestStringPtr(item.WorkTypeID),
		CurrentChainingTraceId:   workstationRequestStringPtr(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(sortedStrings(item.PreviousChainingTraceIDs)),
		TraceId:                  workstationRequestStringPtr(item.TraceID),
		Tags:                     workstationRequestStringMapPtr(cloneStringMap(item.Tags)),
	}
}

func mutationTypeForOutput(input interfaces.WorkstationInput, item interfaces.FactoryWorkItem) string {
	if input.WorkItem != nil && input.WorkItem.ID == item.ID {
		return string(interfaces.MutationMove)
	}
	return string(interfaces.MutationCreate)
}

func mutationTokenID(input interfaces.WorkstationInput, item interfaces.FactoryWorkItem) string {
	if input.TokenID != "" && input.WorkItem != nil && input.WorkItem.ID == item.ID {
		return input.TokenID
	}
	return item.ID
}

func generatedFactoryWorldWorkDiagnostics(
	diagnostics *interfaces.SafeWorkDiagnostics,
) *factoryapi.FactoryWorldWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	return &factoryapi.FactoryWorldWorkDiagnostics{
		RenderedPrompt: generatedFactoryWorldRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       generatedFactoryWorldProviderDiagnostic(diagnostics.Provider),
	}
}

func generatedFactoryWorldRenderedPromptDiagnostic(
	diagnostic *interfaces.SafeRenderedPromptDiagnostic,
) *factoryapi.FactoryWorldRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.FactoryWorldRenderedPromptDiagnostic{
		SystemPromptHash: workstationRequestStringPtr(diagnostic.SystemPromptHash),
		UserMessageHash:  workstationRequestStringPtr(diagnostic.UserMessageHash),
		Variables:        workstationRequestStringMapPtr(diagnostic.Variables),
	}
}

func generatedFactoryWorldProviderDiagnostic(
	diagnostic *interfaces.SafeProviderDiagnostic,
) *factoryapi.FactoryWorldProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &factoryapi.FactoryWorldProviderDiagnostic{
		Provider:         workstationRequestStringPtr(diagnostic.Provider),
		Model:            workstationRequestStringPtr(diagnostic.Model),
		RequestMetadata:  workstationRequestStringMapPtr(diagnostic.RequestMetadata),
		ResponseMetadata: workstationRequestStringMapPtr(diagnostic.ResponseMetadata),
	}
}

func generatedFactoryWorldScriptRequest(
	request *interfaces.FactoryWorldScriptRequest,
) *factoryapi.FactoryWorldScriptRequestView {
	if request == nil {
		return nil
	}
	return &factoryapi.FactoryWorldScriptRequestView{
		Args:            stringSlicePtr(cloneStringSlice(request.Args)),
		Attempt:         intPtr(request.Attempt),
		Command:         workstationRequestStringPtr(request.Command),
		ScriptRequestId: workstationRequestStringPtr(request.ScriptRequestID),
	}
}

func generatedFactoryWorldScriptResponse(
	response *interfaces.FactoryWorldScriptResponse,
) *factoryapi.FactoryWorldScriptResponseView {
	if response == nil {
		return nil
	}
	return &factoryapi.FactoryWorldScriptResponseView{
		Attempt:         intPtr(response.Attempt),
		DurationMillis:  int64Ptr(response.DurationMillis),
		ExitCode:        cloneIntPtr(response.ExitCode),
		FailureType:     workstationRequestStringPtr(response.FailureType),
		Outcome:         workstationRequestStringPtr(response.Outcome),
		ScriptRequestId: workstationRequestStringPtr(response.ScriptRequestID),
		Stderr:          workstationRequestStringPtr(response.Stderr),
		Stdout:          workstationRequestStringPtr(response.Stdout),
	}
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func workstationRequestStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(cloneStringMap(values))
	return &converted
}

func workItemRefSlicePtr(values []factoryapi.FactoryWorldWorkItemRef) *[]factoryapi.FactoryWorldWorkItemRef {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func tokenViewSlicePtr(values []factoryapi.FactoryWorldTokenView) *[]factoryapi.FactoryWorldTokenView {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func timePtr(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func int64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func intPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func workstationRequestStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func providerSessionProvider(session *interfaces.ProviderSessionMetadata) string {
	if session == nil {
		return ""
	}
	return session.Provider
}

func workstationNameOrID(name string, id string) string {
	if name != "" {
		return name
	}
	return id
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

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(unique))
	for value := range unique {
		sorted = append(sorted, value)
	}
	sort.Strings(sorted)
	return sorted
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	clone := make(map[string]string, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	clone := make([]string, len(input))
	copy(clone, input)
	return clone
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
