package recordings

import (
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkstationFactoryWorldWorkstationRequestProjectionSlice is the canonical,
// transport-independent workstation request read model for one selected
// Factory tick.
type WorkstationFactoryWorldWorkstationRequestProjectionSlice struct {
	WorkstationRequestsByDispatchId *map[string]WorkstationFactoryWorldWorkstationRequestView `json:"workstationRequestsByDispatchId,omitempty"`
}

type WorkstationFactoryWorldWorkstationRequestView struct {
	Counts          WorkstationFactoryWorldWorkstationRequestCountView     `json:"counts"`
	DispatchId      string                                                 `json:"dispatchId"`
	Request         WorkstationFactoryWorldWorkstationRequestRequestView   `json:"request"`
	Response        *WorkstationFactoryWorldWorkstationRequestResponseView `json:"response,omitempty"`
	TransitionId    string                                                 `json:"transitionId"`
	WorkstationName *string                                                `json:"workstationName,omitempty"`
}

type WorkstationFactoryWorldWorkstationRequestCountView struct {
	DispatchedCount int `json:"dispatchedCount"`
	ErroredCount    int `json:"erroredCount"`
	RespondedCount  int `json:"respondedCount"`
}

type WorkstationFactoryWorldWorkstationRequestRequestView struct {
	ConsumedTokens           *[]WorkstationFactoryWorldTokenView        `json:"consumedTokens,omitempty"`
	CurrentChainingTraceId   *string                                    `json:"currentChainingTraceId,omitempty"`
	InputWorkItems           *[]WorkstationFactoryWorldWorkItemRef      `json:"inputWorkItems,omitempty"`
	InputWorkTypeIds         *[]string                                  `json:"inputWorkTypeIds,omitempty"`
	PreviousChainingTraceIds *[]string                                  `json:"previousChainingTraceIds,omitempty"`
	Runner                   *WorkstationFactoryWorldSelectedRunnerView `json:"runner,omitempty"`
	ScriptRequest            *WorkstationFactoryWorldScriptRequestView  `json:"scriptRequest,omitempty"`
	StartedAt                *time.Time                                 `json:"startedAt,omitempty"`
	TraceIds                 *[]string                                  `json:"traceIds,omitempty"`
}

type WorkstationFactoryWorldWorkstationRequestResponseView struct {
	AgentRunInspection          *workerexecution.SafeAgentRunDiagnostic    `json:"agentRunInspection,omitempty"`
	DurationMillis              *int64                                     `json:"durationMillis,omitempty"`
	EndTime                     *time.Time                                 `json:"endTime,omitempty"`
	FailureDetail               *workerexecution.FailureDetail             `json:"failureDetail,omitempty"`
	Feedback                    *string                                    `json:"feedback,omitempty"`
	Outcome                     *string                                    `json:"outcome,omitempty"`
	OutputMutations             *[]WorkstationFactoryWorldMutationView     `json:"outputMutations,omitempty"`
	OutputWorkItems             *[]WorkstationFactoryWorldWorkItemRef      `json:"outputWorkItems,omitempty"`
	Runner                      *WorkstationFactoryWorldSelectedRunnerView `json:"runner,omitempty"`
	ScriptResponse              *WorkstationFactoryWorldScriptResponseView `json:"scriptResponse,omitempty"`
	SelectedClassificationLabel *string                                    `json:"selectedClassificationLabel,omitempty"`
}

type WorkstationFactoryWorldSelectedRunnerView struct {
	Capabilities    *WorkstationFactoryWorldRunnerCapabilitiesView `json:"capabilities,omitempty"`
	DisplayName     *string                                        `json:"displayName,omitempty"`
	RunnerId        *WorkstationRunnerID                           `json:"runnerId,omitempty"`
	SelectionSource *WorkstationRunnerSelectionSource              `json:"selectionSource,omitempty"`
}

type WorkstationRunnerID string
type WorkstationRunnerSelectionSource string
type WorkstationFactoryWorldRunnerBaselineCapability string
type WorkstationFactoryWorldRunnerOptionalCapability string
type WorkstationFactoryWorldRunnerOptionalCapabilityStatus string

type WorkstationFactoryWorldRunnerCapabilitiesView struct {
	BaselineCapabilities []WorkstationFactoryWorldRunnerBaselineCapability            `json:"baselineCapabilities"`
	OptionalCapabilities []WorkstationFactoryWorldRunnerOptionalCapabilitySupportView `json:"optionalCapabilities"`
}

type WorkstationFactoryWorldRunnerOptionalCapabilitySupportView struct {
	Capability WorkstationFactoryWorldRunnerOptionalCapability       `json:"capability"`
	Detail     *string                                               `json:"detail,omitempty"`
	Status     WorkstationFactoryWorldRunnerOptionalCapabilityStatus `json:"status"`
}

type WorkstationFactoryWorldWorkItemRef struct {
	ChainingTraceDepth       *int                                                 `json:"chainingTraceDepth,omitempty"`
	Content                  *[]work.WorkContentPart                              `json:"content,omitempty"`
	CurrentChainingTraceId   *string                                              `json:"currentChainingTraceId,omitempty"`
	DisplayName              *string                                              `json:"displayName,omitempty"`
	LineageContinuity        *WorkstationFactoryWorldWorkItemRefLineageContinuity `json:"lineageContinuity,omitempty"`
	LineageLogicalWorkId     *string                                              `json:"lineageLogicalWorkId,omitempty"`
	LineageParentWorkIds     *[]string                                            `json:"lineageParentWorkIds,omitempty"`
	LineageSourceKind        *WorkstationFactoryWorldWorkItemRefLineageSourceKind `json:"lineageSourceKind,omitempty"`
	PayloadStatus            *WorkstationFactoryWorldWorkItemRefPayloadStatus     `json:"payloadStatus,omitempty"`
	PayloadUnavailableReason *string                                              `json:"payloadUnavailableReason,omitempty"`
	PreviousChainingTraceIds *[]string                                            `json:"previousChainingTraceIds,omitempty"`
	State                    *string                                              `json:"state,omitempty"`
	TraceId                  *string                                              `json:"traceId,omitempty"`
	WorkId                   string                                               `json:"workId"`
	WorkTypeId               *string                                              `json:"workTypeId,omitempty"`
}

type WorkstationFactoryWorldWorkItemRefLineageContinuity string
type WorkstationFactoryWorldWorkItemRefLineageSourceKind string
type WorkstationFactoryWorldWorkItemRefPayloadStatus string

type WorkstationFactoryWorldTokenView struct {
	ChainingTraceDepth       *int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceId   *string               `json:"currentChainingTraceId,omitempty"`
	Name                     *string               `json:"name,omitempty"`
	PlaceId                  string                `json:"placeId"`
	PreviousChainingTraceIds *[]string             `json:"previousChainingTraceIds,omitempty"`
	Tags                     *WorkstationStringMap `json:"tags,omitempty"`
	TokenId                  string                `json:"tokenId"`
	TraceId                  *string               `json:"traceId,omitempty"`
	WorkId                   *string               `json:"workId,omitempty"`
	WorkTypeId               *string               `json:"workTypeId,omitempty"`
}

type WorkstationStringMap map[string]string

type WorkstationFactoryWorldMutationView struct {
	FromPlace *string                           `json:"fromPlace,omitempty"`
	Reason    *string                           `json:"reason,omitempty"`
	ToPlace   *string                           `json:"toPlace,omitempty"`
	Token     *WorkstationFactoryWorldTokenView `json:"token,omitempty"`
	TokenId   string                            `json:"tokenId"`
	Type      string                            `json:"type"`
}

type WorkstationFactoryWorldScriptRequestView struct {
	Args            *[]string `json:"args,omitempty"`
	Attempt         *int      `json:"attempt,omitempty"`
	Command         *string   `json:"command,omitempty"`
	ScriptRequestId *string   `json:"scriptRequestId,omitempty"`
}

type WorkstationFactoryWorldScriptResponseView struct {
	Attempt         *int    `json:"attempt,omitempty"`
	DurationMillis  *int64  `json:"durationMillis,omitempty"`
	ExitCode        *int    `json:"exitCode,omitempty"`
	FailureType     *string `json:"failureType,omitempty"`
	Outcome         *string `json:"outcome,omitempty"`
	ScriptRequestId *string `json:"scriptRequestId,omitempty"`
	Stderr          *string `json:"stderr,omitempty"`
	Stdout          *string `json:"stdout,omitempty"`
}

// BuildFactoryWorldWorkstationRequestProjectionSlice keeps the additive
// workstation-request contract at the API boundary while deriving it from the
// canonical selected-tick FactoryWorldState model.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	dispatchViewsByID := buildFactoryWorldWorkstationDispatchViewsByID(state)
	if len(dispatchViewsByID) == 0 {
		return WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
	}
	return WorkstationFactoryWorldWorkstationRequestProjectionSlice{
		WorkstationRequestsByDispatchId: &dispatchViewsByID,
	}
}

func buildFactoryWorldWorkstationDispatchViewsByID(
	state interfaces.FactoryWorldState,
) map[string]WorkstationFactoryWorldWorkstationRequestView {
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

	dispatchViewsByID := make(map[string]WorkstationFactoryWorldWorkstationRequestView, len(dispatchIDs))
	for _, dispatchID := range sortedMapKeys(dispatchIDs) {
		latestScriptResponse := latestWorkstationScriptResponse(state.ScriptResponsesByDispatchID[dispatchID])
		latestScriptRequest := workstationScriptRequestForProjection(latestScriptResponse, state.ScriptRequestsByDispatchID[dispatchID])
		if dispatch, ok := state.ActiveDispatches[dispatchID]; ok && dispatchHasCustomerWork(dispatch.WorkItemIDs, state.WorkItemsByID) {
			dispatchViewsByID[dispatchID] = workstationDispatchViewFromActiveDispatch(
				dispatch,
				state,
				latestScriptRequest,
				latestScriptResponse,
			)
		}
		if completion, ok := completedByID[dispatchID]; ok {
			dispatchViewsByID[dispatchID] = workstationDispatchViewFromCompletion(
				completion,
				state,
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
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) WorkstationFactoryWorldWorkstationRequestView {
	inputWorkItems := generatedWorkItemRefs(consumedInputWorkItemRefsForActiveDispatch(dispatch, state))
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForIDs(dispatch.WorkItemIDs, state.WorkItemsByID))
	}
	return WorkstationFactoryWorldWorkstationRequestView{
		DispatchId:      dispatch.DispatchID,
		TransitionId:    dispatch.TransitionID,
		WorkstationName: workstationRequestStringPtr(workstationNameOrID(dispatch.Workstation.Name, dispatch.TransitionID)),
		Request: workstationDispatchRequestView(
			dispatch.RunnerID,
			dispatch.RunnerSelectionSource,
			dispatch.StartedAt,
			inputWorkItems,
			dispatch.CurrentChainingTraceID,
			dispatch.PreviousChainingTraceIDs,
			sortedStrings(dispatch.TraceIDs),
			generatedTokenViewsFromInputs(dispatch.Inputs),
			latestScriptRequest,
		),
		Response: workstationRequestResponseViewFromActiveDispatch(dispatch, latestScriptResponse),
	}
}

func workstationRequestResponseViewFromActiveDispatch(
	dispatch interfaces.FactoryWorldDispatch,
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) *WorkstationFactoryWorldWorkstationRequestResponseView {
	if latestScriptResponse == nil {
		return nil
	}
	return &WorkstationFactoryWorldWorkstationRequestResponseView{
		Runner:         generatedFactoryWorldSelectedRunnerView(dispatch.RunnerID, dispatch.RunnerSelectionSource),
		ScriptResponse: generatedFactoryWorldScriptResponse(latestScriptResponse),
	}
}

func workstationDispatchViewFromCompletion(
	completion interfaces.FactoryWorldDispatchCompletion,
	state interfaces.FactoryWorldState,
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
	latestScriptResponse *interfaces.FactoryWorldScriptResponse,
) WorkstationFactoryWorldWorkstationRequestView {
	inputWorkItems := generatedWorkItemRefs(consumedInputWorkItemRefsForCompletion(completion, state))
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForInputs(completion.ConsumedInputs))
	}
	if len(inputWorkItems) == 0 {
		inputWorkItems = generatedWorkItemRefs(workItemRefsForIDs(completion.WorkItemIDs, state.WorkItemsByID))
	}
	outputWorkItems := generatedWorkItemRefs(outputWorkItemRefsForCompletion(completion, state))
	if len(outputWorkItems) == 0 && completion.TerminalWork != nil && !interfaces.IsSystemTimeWorkType(completion.TerminalWork.WorkItem.WorkTypeID) {
		outputWorkItems = generatedWorkItemRefs([]interfaces.FactoryWorldWorkItemRef{
			workItemRef(completion.TerminalWork.WorkItem),
		})
	}
	return WorkstationFactoryWorldWorkstationRequestView{
		DispatchId:      completion.DispatchID,
		TransitionId:    completion.TransitionID,
		WorkstationName: workstationRequestStringPtr(workstationNameOrID(completion.Workstation.Name, completion.TransitionID)),
		Request: workstationDispatchRequestView(
			completion.RunnerID,
			completion.RunnerSelectionSource,
			completion.StartedAt,
			inputWorkItems,
			completion.CurrentChainingTraceID,
			completion.PreviousChainingTraceIDs,
			sortedStrings(completion.TraceIDs),
			generatedTokenViewsFromInputs(completion.ConsumedInputs),
			latestScriptRequest,
		),
		Response: &WorkstationFactoryWorldWorkstationRequestResponseView{
			Runner:                      generatedFactoryWorldSelectedRunnerView(completion.RunnerID, completion.RunnerSelectionSource),
			Outcome:                     workstationRequestStringPtr(completion.Result.Outcome),
			Feedback:                    workstationRequestStringPtr(completion.Result.Feedback),
			SelectedClassificationLabel: workstationRequestStringPtr(completion.Result.SelectedClassificationLabel),
			FailureDetail:               workstationFailureDetailFromCanonical(completion.Result.FailureDetail),
			ScriptResponse:              generatedFactoryWorldScriptResponse(latestScriptResponse),
			AgentRunInspection:          generatedFactoryWorldAgentRunInspection(completion.Diagnostics),
			EndTime:                     timePtr(completion.CompletedAt),
			DurationMillis:              int64Ptr(completion.DurationMillis),
			OutputWorkItems:             workItemRefSlicePtr(outputWorkItems),
			OutputMutations:             mutationViewsPtrForCompletion(completion),
		},
	}
}

func workstationFailureDetail(reason, message string) *workerexecution.FailureDetail {
	reason = strings.TrimSpace(reason)
	message = strings.TrimSpace(message)
	if reason == "" || message == "" {
		return nil
	}
	return &workerexecution.FailureDetail{
		Reason:  workerexecution.WorkFailureType(reason),
		Message: message,
	}
}

func workstationFailureDetailFromCanonical(detail *workerexecution.FailureDetail) *workerexecution.FailureDetail {
	if detail == nil {
		return nil
	}
	return workstationFailureDetail(string(detail.Reason), detail.Message)
}

func workstationDispatchRequestView(
	runnerID string,
	runnerSource workerexecution.RunnerSelectionSource,
	startedAt time.Time,
	inputWorkItems []WorkstationFactoryWorldWorkItemRef,
	currentChainingTraceID string,
	previousChainingTraceIDs []string,
	traceIDs []string,
	consumedTokens []WorkstationFactoryWorldTokenView,
	latestScriptRequest *interfaces.FactoryWorldScriptRequest,
) WorkstationFactoryWorldWorkstationRequestRequestView {
	return WorkstationFactoryWorldWorkstationRequestRequestView{
		Runner:                   generatedFactoryWorldSelectedRunnerView(runnerID, runnerSource),
		StartedAt:                timePtr(startedAt),
		InputWorkItems:           workItemRefSlicePtr(inputWorkItems),
		InputWorkTypeIds:         stringSlicePtr(workTypeIDsForWorkRefs(inputWorkItems)),
		CurrentChainingTraceId:   workstationRequestStringPtr(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(sortedStrings(previousChainingTraceIDs)),
		TraceIds:                 stringSlicePtr(traceIDs),
		ConsumedTokens:           tokenViewSlicePtr(consumedTokens),
		ScriptRequest:            generatedFactoryWorldScriptRequest(latestScriptRequest),
	}
}

func generatedFactoryWorldSelectedRunnerView(runnerID string, runnerSource workerexecution.RunnerSelectionSource) *WorkstationFactoryWorldSelectedRunnerView {
	runnerID = workerexecution.NormalizeRunnerID(runnerID)
	if runnerID == "" && runnerSource == "" {
		return nil
	}
	view := &WorkstationFactoryWorldSelectedRunnerView{
		RunnerId:        runnerIDPtr(runnerID),
		SelectionSource: runnerSelectionSourcePtr(string(runnerSource)),
	}
	if metadata, ok := workerexecution.BuiltInRunnerMetadata(runnerID); ok {
		view.DisplayName = workstationRequestStringPtr(metadata.DisplayName)
		view.Capabilities = generatedFactoryWorldRunnerCapabilitiesView(metadata.Capabilities)
	}
	return view
}

func runnerIDPtr(value string) *WorkstationRunnerID {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := WorkstationRunnerID(interfaces.PermissivePublicFactoryRunnerID(workerexecution.NormalizeRunnerID(value)))
	return &converted
}

func runnerSelectionSourcePtr(value string) *WorkstationRunnerSelectionSource {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := WorkstationRunnerSelectionSource(interfaces.PermissivePublicFactoryRunnerSelectionSource(value))
	return &converted
}

func generatedFactoryWorldRunnerCapabilitiesView(
	capabilities workerexecution.RunnerCapabilities,
) *WorkstationFactoryWorldRunnerCapabilitiesView {
	baseline := make([]WorkstationFactoryWorldRunnerBaselineCapability, 0, len(capabilities.Baseline))
	for _, capability := range capabilities.Baseline {
		baseline = append(baseline, WorkstationFactoryWorldRunnerBaselineCapability(capability))
	}

	optional := make([]WorkstationFactoryWorldRunnerOptionalCapabilitySupportView, 0, len(capabilities.Optional))
	for _, support := range capabilities.Optional {
		optional = append(optional, WorkstationFactoryWorldRunnerOptionalCapabilitySupportView{
			Capability: WorkstationFactoryWorldRunnerOptionalCapability(support.Capability),
			Status:     WorkstationFactoryWorldRunnerOptionalCapabilityStatus(support.Status),
			Detail:     workstationRequestStringPtr(support.Detail),
		})
	}

	return &WorkstationFactoryWorldRunnerCapabilitiesView{
		BaselineCapabilities: baseline,
		OptionalCapabilities: optional,
	}
}

func buildFactoryWorldWorkstationRequestCounts(
	attempts map[string]interfaces.FactoryWorldInferenceAttempt,
	scriptRequests map[string]interfaces.FactoryWorldScriptRequest,
	scriptResponses map[string]interfaces.FactoryWorldScriptResponse,
) WorkstationFactoryWorldWorkstationRequestCountView {
	counts := WorkstationFactoryWorldWorkstationRequestCountView{}
	for _, requestID := range sortedMapKeys(attempts) {
		attempt := attempts[requestID]
		if attempt.InferenceRequestID != "" {
			counts.DispatchedCount++
		}
		if attempt.ResponseTime.IsZero() {
			continue
		}
		if attempt.FailureDetail != nil || attempt.Outcome == "FAILED" {
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
	case "FAILED_EXIT_CODE",
		"PROCESS_ERROR",
		"TIMED_OUT":
		return true
	default:
		return false
	}
}

func dispatchHasCustomerWork(ids []string, items map[string]work.FactoryWorkItem) bool {
	return len(workItemRefsForIDs(ids, items)) > 0
}

func generatedWorkItemRefs(refs []interfaces.FactoryWorldWorkItemRef) []WorkstationFactoryWorldWorkItemRef {
	out := make([]WorkstationFactoryWorldWorkItemRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, WorkstationFactoryWorldWorkItemRef{
			WorkId:                   ref.WorkID,
			WorkTypeId:               workstationRequestStringPtr(ref.WorkTypeID),
			State:                    workstationRequestStringPtr(ref.State),
			DisplayName:              workstationRequestStringPtr(ref.DisplayName),
			ChainingTraceDepth:       intPtr(ref.ChainingTraceDepth),
			CurrentChainingTraceId:   workstationRequestStringPtr(ref.CurrentChainingTraceID),
			PreviousChainingTraceIds: stringSlicePtr(sortedStrings(ref.PreviousChainingTraceIDs)),
			TraceId:                  workstationRequestStringPtr(ref.TraceID),
			Content:                  workstationWorkContentPtr(ref.Content),
			PayloadStatus:            generatedWorkItemPayloadStatusPtr(ref.PayloadStatus),
			PayloadUnavailableReason: workstationRequestStringPtr(ref.PayloadUnavailableReason),
			LineageLogicalWorkId:     workstationRequestStringPtr(ref.LineageLogicalWorkID),
			LineageSourceKind:        generatedWorkItemLineageSourceKindPtr(ref.LineageSourceKind),
			LineageContinuity:        generatedWorkItemLineageContinuityPtr(ref.LineageContinuity),
			LineageParentWorkIds:     stringSlicePtr(sortedStrings(ref.LineageParentWorkIDs)),
		})
	}
	return out
}

func generatedWorkItemPayloadStatusPtr(value string) *WorkstationFactoryWorldWorkItemRefPayloadStatus {
	if value == "" {
		return nil
	}
	status := WorkstationFactoryWorldWorkItemRefPayloadStatus(value)
	return &status
}

func generatedWorkItemLineageSourceKindPtr(value string) *WorkstationFactoryWorldWorkItemRefLineageSourceKind {
	if value == "" {
		return nil
	}
	sourceKind := WorkstationFactoryWorldWorkItemRefLineageSourceKind(value)
	return &sourceKind
}

func generatedWorkItemLineageContinuityPtr(value string) *WorkstationFactoryWorldWorkItemRefLineageContinuity {
	if value == "" {
		return nil
	}
	continuity := WorkstationFactoryWorldWorkItemRefLineageContinuity(value)
	return &continuity
}

func consumedInputWorkItemRefsForActiveDispatch(
	dispatch interfaces.FactoryWorldDispatch,
	state interfaces.FactoryWorldState,
) []interfaces.FactoryWorldWorkItemRef {
	return consumedInputWorkItemRefs(
		dispatch.DispatchID,
		dispatch.Inputs,
		nil,
		dispatch.WorkItemIDs,
		state,
	)
}

func consumedInputWorkItemRefsForCompletion(
	completion interfaces.FactoryWorldDispatchCompletion,
	state interfaces.FactoryWorldState,
) []interfaces.FactoryWorldWorkItemRef {
	return consumedInputWorkItemRefs(
		completion.DispatchID,
		completion.ConsumedInputs,
		completion.InputWorkItems,
		completion.WorkItemIDs,
		state,
	)
}

func consumedInputWorkItemRefs(
	dispatchID string,
	inputs []interfaces.WorkstationInput,
	fallbackItems []work.FactoryWorkItem,
	fallbackIDs []string,
	state interfaces.FactoryWorldState,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(inputs)+len(fallbackItems)+len(fallbackIDs))
	seen := make(map[string]struct{}, len(inputs)+len(fallbackItems)+len(fallbackIDs))

	appendConsumedRef := func(workID string, fallback *work.FactoryWorkItem) {
		if workID == "" {
			return
		}
		if _, exists := seen[workID]; exists {
			return
		}
		resolution := state.PayloadLineage.ResolveConsumedInputSnapshot(dispatchID, workID)
		refs = append(refs, consumedInputWorkItemRef(workID, fallback, resolution))
		seen[workID] = struct{}{}
	}

	for _, input := range inputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" || interfaces.IsSystemTimeWorkType(input.WorkItem.WorkTypeID) {
			continue
		}
		workItem := *input.WorkItem
		appendConsumedRef(workItem.ID, &workItem)
	}
	for _, item := range fallbackItems {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		itemCopy := item
		appendConsumedRef(item.ID, &itemCopy)
	}
	for _, workID := range sortedStrings(fallbackIDs) {
		item, ok := state.WorkItemsByID[workID]
		if !ok || item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		itemCopy := item
		appendConsumedRef(workID, &itemCopy)
	}
	return refs
}

func consumedInputWorkItemRef(
	workID string,
	fallback *work.FactoryWorkItem,
	resolution work.WorkPayloadResolution,
) interfaces.FactoryWorldWorkItemRef {
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

func outputWorkItemRefsForCompletion(
	completion interfaces.FactoryWorldDispatchCompletion,
	state interfaces.FactoryWorldState,
) []interfaces.FactoryWorldWorkItemRef {
	refs := make([]interfaces.FactoryWorldWorkItemRef, 0, len(completion.OutputWorkItems))
	seen := make(map[string]struct{}, len(completion.OutputWorkItems))
	for _, item := range completion.OutputWorkItems {
		if item.ID == "" || interfaces.IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		resolution := state.PayloadLineage.ResolveOutputWorkSnapshot(completion.DispatchID, item.ID)
		if resolution.Status == work.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
			refs = append(refs, lineageResolvedWorkItemRef(resolution.Snapshot, string(resolution.Status)))
		} else {
			ref := workItemRef(item)
			ref.PayloadStatus = string(resolution.Status)
			ref.PayloadUnavailableReason = resolution.Reason
			refs = append(refs, ref)
		}
		seen[item.ID] = struct{}{}
	}
	if len(refs) > 0 {
		return refs
	}
	return workItemRefsForItems(completion.OutputWorkItems)
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

func workItemRefsForIDs(
	ids []string,
	items map[string]work.FactoryWorkItem,
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

func workItemRefsForItems(items []work.FactoryWorkItem) []interfaces.FactoryWorldWorkItemRef {
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

func workTypeIDsForWorkRefs(refs []WorkstationFactoryWorldWorkItemRef) []string {
	var ids []string
	for _, ref := range refs {
		if ref.WorkTypeId == nil {
			continue
		}
		ids = appendUnique(ids, *ref.WorkTypeId)
	}
	return sortedStrings(ids)
}

func generatedTokenViewsFromInputs(inputs []interfaces.WorkstationInput) []WorkstationFactoryWorldTokenView {
	out := make([]WorkstationFactoryWorldTokenView, 0, len(inputs))
	for _, input := range inputs {
		view := WorkstationFactoryWorldTokenView{
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
			view.ChainingTraceDepth = intPtr(input.WorkItem.ChainingTraceDepth)
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
) []WorkstationFactoryWorldMutationView {
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
	views := make([]WorkstationFactoryWorldMutationView, 0, len(dispatch.OutputWorkItems))
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
		views = append(views, WorkstationFactoryWorldMutationView{
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
) *[]WorkstationFactoryWorldMutationView {
	views := mutationViewsForCompletion(dispatch)
	if len(views) == 0 {
		return nil
	}
	return &views
}

func generatedTokenViewForWorkItem(tokenID string, item work.FactoryWorkItem) *WorkstationFactoryWorldTokenView {
	if tokenID == "" {
		tokenID = item.ID
	}
	currentChainingTraceID := item.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = item.TraceID
	}
	return &WorkstationFactoryWorldTokenView{
		TokenId:                  tokenID,
		PlaceId:                  item.PlaceID,
		Name:                     workstationRequestStringPtr(item.DisplayName),
		WorkId:                   workstationRequestStringPtr(item.ID),
		WorkTypeId:               workstationRequestStringPtr(item.WorkTypeID),
		ChainingTraceDepth:       intPtr(item.ChainingTraceDepth),
		CurrentChainingTraceId:   workstationRequestStringPtr(currentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePtr(sortedStrings(item.PreviousChainingTraceIDs)),
		TraceId:                  workstationRequestStringPtr(item.TraceID),
		Tags:                     workstationRequestStringMapPtr(cloneStringMap(item.Tags)),
	}
}

func mutationTypeForOutput(input interfaces.WorkstationInput, item work.FactoryWorkItem) string {
	if input.WorkItem != nil && input.WorkItem.ID == item.ID {
		return string(interfaces.MutationMove)
	}
	return string(interfaces.MutationCreate)
}

func mutationTokenID(input interfaces.WorkstationInput, item work.FactoryWorkItem) string {
	if input.TokenID != "" && input.WorkItem != nil && input.WorkItem.ID == item.ID {
		return input.TokenID
	}
	return item.ID
}

func generatedFactoryWorldScriptRequest(
	request *interfaces.FactoryWorldScriptRequest,
) *WorkstationFactoryWorldScriptRequestView {
	if request == nil {
		return nil
	}
	return &WorkstationFactoryWorldScriptRequestView{
		Args:            stringSlicePtr(cloneStringSlice(request.Args)),
		Attempt:         intPtr(request.Attempt),
		Command:         workstationRequestStringPtr(request.Command),
		ScriptRequestId: workstationRequestStringPtr(request.ScriptRequestID),
	}
}

func generatedFactoryWorldScriptResponse(
	response *interfaces.FactoryWorldScriptResponse,
) *WorkstationFactoryWorldScriptResponseView {
	if response == nil {
		return nil
	}
	return &WorkstationFactoryWorldScriptResponseView{
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

func generatedFactoryWorldAgentRunInspection(
	diagnostics *workerexecution.SafeWorkDiagnostics,
) *workerexecution.SafeAgentRunDiagnostic {
	if diagnostics == nil || diagnostics.AgentRun == nil {
		return nil
	}
	cloned := *diagnostics.AgentRun
	cloned.ToolDiagnostics = append(
		[]workerexecution.AgentRunToolDiagnostic(nil),
		diagnostics.AgentRun.ToolDiagnostics...,
	)
	cloned.Transcript = append(
		[]workerexecution.AgentRunTranscriptEntry(nil),
		diagnostics.AgentRun.Transcript...,
	)
	return &cloned
}

func stringSlicePtr(values []string) *[]string {
	cloned := cloneStringSlice(values)
	if len(cloned) == 0 {
		return nil
	}
	return &cloned
}

func workstationRequestStringMapPtr(values map[string]string) *WorkstationStringMap {
	cloned := cloneStringMap(values)
	if len(cloned) == 0 {
		return nil
	}
	converted := WorkstationStringMap(cloned)
	return &converted
}

func workItemRefSlicePtr(values []WorkstationFactoryWorldWorkItemRef) *[]WorkstationFactoryWorldWorkItemRef {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func tokenViewSlicePtr(values []WorkstationFactoryWorldTokenView) *[]WorkstationFactoryWorldTokenView {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
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
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func workstationWorkContentPtr(parts []work.WorkContentPart) *[]work.WorkContentPart {
	cloned := work.CloneWorkContentParts(parts)
	if len(cloned) == 0 {
		return nil
	}
	return &cloned
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
