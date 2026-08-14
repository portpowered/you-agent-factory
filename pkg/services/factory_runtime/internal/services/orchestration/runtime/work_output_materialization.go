package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

func workstationDispatchResultFromExecute(
	request workers.WorkstationDispatchRequest,
	result workers.ExecuteResult,
	executeErr error,
) (workers.WorkstationDispatchResult, error) {
	dispatch := request.Execution.Dispatch
	proposedOutput := result.Output.Clone()
	workResult := workers.WorkResult{
		DispatchID:                  dispatch.DispatchID,
		TransitionID:                dispatch.TransitionID,
		Outcome:                     workers.OutcomeAccepted,
		Output:                      primaryOutputText(result.Output.Primary),
		StructuredResult:            jsonvalue.Clone(result.StructuredResult),
		StructuredResultPresent:     jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent),
		ArtifactVerification:        result.ArtifactVerification.Clone(),
		Feedback:                    result.Output.Feedback,
		SelectedClassificationLabel: result.Output.Classification,
		Metrics: workers.WorkMetrics{
			Duration:   result.Metrics.Duration,
			Cost:       result.Metrics.Cost,
			RetryCount: result.Metrics.RetryCount,
		},
		ProviderSession: providerSessionFromContinuation(result.Continuation),
		Diagnostics:     result.Diagnostics.ToWorkDiagnostics(),
	}
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	switch result.Outcome {
	case workers.ExecutionOutcomeContinue:
		workResult.Outcome = workers.OutcomeContinue
	case workers.ExecutionOutcomeRejected:
		workResult.Outcome = workers.OutcomeRejected
	case workers.ExecutionOutcomeFailed:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	case workers.ExecutionOutcomeCanceled:
		workResult.Outcome = workers.OutcomeFailed
		terminal = workers.WorkstationDispatchTerminalOutcomeCanceled
	default:
		if result.Outcome != workers.ExecutionOutcomeAccepted {
			workResult.Outcome = workers.OutcomeFailed
			terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		}
	}
	if result.Failure != nil {
		workResult.Error = strings.TrimSpace(result.Failure.Message)
		if shouldPropagateFailureMetadata(request, result.Failure) {
			workResult.FailureMetadata = &workers.WorkFailureMetadata{
				Family: result.Failure.Family,
				Type:   result.Failure.Type,
			}
		}
	}
	if executeErr != nil && terminal != workers.WorkstationDispatchTerminalOutcomeCanceled {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
		workResult.Outcome = workers.OutcomeFailed
		if strings.TrimSpace(workResult.Error) == "" {
			workResult.Error = executeErr.Error()
		}
	}
	if terminal == workers.WorkstationDispatchTerminalOutcomeCanceled && strings.TrimSpace(workResult.Error) == "" {
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          workResult,
		ProposedOutput:  &proposedOutput,
	}, executeErr
}

// shouldPropagateFailureMetadata preserves the script workstation boundary:
// ordinary process failures are terminal Work results without retry metadata,
// while timeout and missing-executable failures retain their explicit
// classifications.
func shouldPropagateFailureMetadata(
	request workers.WorkstationDispatchRequest,
	failure *workers.ExecutionFailure,
) bool {
	if failure == nil || !isScriptWorkstationDispatch(request) {
		return true
	}
	switch failure.Type {
	case workers.WorkFailureTypeTimeout, workers.WorkFailureTypeMissingExecutable:
		return true
	default:
		return false
	}
}

func isScriptWorkstationDispatch(request workers.WorkstationDispatchRequest) bool {
	return strings.TrimSpace(request.Execution.RunnerID) == "script" ||
		strings.EqualFold(strings.TrimSpace(request.WorkstationName), "script-station")
}

func primaryOutputText(parts []work.WorkContentPart) string {
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) > 0 {
				return string(part.JSON)
			}
		case work.WorkContentPartTypeImage, work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			if strings.TrimSpace(part.URL) != "" {
				return part.URL
			}
			if strings.TrimSpace(part.File) != "" {
				return part.File
			}
		}
	}
	return ""
}

func providerSessionFromContinuation(
	continuation *workers.ProviderContinuationRef,
) *workers.ProviderSessionMetadata {
	if continuation == nil {
		return nil
	}
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" && strings.TrimSpace(continuation.Provider) == "" {
		return nil
	}
	return &workers.ProviderSessionMetadata{
		Provider: continuation.Provider,
		Kind:     "session",
		ID:       id,
	}
}

// materializeWorkerOutputForDispatch validates Worker-proposed output through
// Work before the result enters Runtime state. Invalid proposals flip the
// dispatch to FAILED without carrying RecordedOutputWork into mutations.
func materializeWorkerOutputForDispatch(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	request workerexecution.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
) workerexecution.WorkResult {
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		request.Execution.Dispatch,
		result,
		workerexecution.ProposedOutputFromLegacyWorkResult(result),
		false,
	)
}

// materializeWorkerOutputForDispatchWithProposal preserves the detached
// Execute proposal until Runtime has handed it to Work. Legacy WorkResult
// callers continue through the compatibility mapper above.
func materializeWorkerOutputForDispatchWithProposal(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	request workerexecution.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	proposal *workerexecution.ProposedOutput,
) workerexecution.WorkResult {
	proposals := workerexecution.ProposedOutputFromLegacyWorkResult(result)
	fromDetachedOutput := proposal != nil
	if proposal != nil {
		proposals = proposal.Clone()
	}
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		request.Execution.Dispatch,
		result,
		proposals,
		fromDetachedOutput,
	)
}

func applyMaterializedWorkerOutput(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	proposals workerexecution.ProposedOutput,
	fromDetachedOutput bool,
) workerexecution.WorkResult {
	if len(proposals.ProposedWork) == 0 &&
		(!fromDetachedOutput || !hasMaterializableOutput(proposals)) {
		return result
	}

	if workService == nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = "worker output materialization: Work service is required"
		} else {
			failed.Error = fmt.Sprintf("%s; worker output materialization: Work service is required", failed.Error)
		}
		return failed
	}

	materialized, err := workService.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{
		Lineage:           lineageContextFromDispatch(dispatch),
		Primary:           proposals.Primary,
		Feedback:          proposals.Feedback,
		Classification:    proposals.Classification,
		ProposedWork:      workerexecution.WorkProposedItemsFromProposedWork(proposals.ProposedWork),
		ValidWorkTypes:    validWorkTypesFromNet(net),
		ValidStatesByType: validStatesByTypeFromNet(net),
		IDGenerator:       idGenerator,
	})
	if err != nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = fmt.Sprintf("worker output materialization: %v", err)
		} else {
			failed.Error = fmt.Sprintf(
				"%s; worker output materialization: %v",
				failed.Error,
				err,
			)
		}
		return failed
	}

	next := result
	if text := strings.TrimSpace(materialized.PrimaryOutput); text != "" {
		next.Output = text
	}
	if feedback := strings.TrimSpace(materialized.Feedback); feedback != "" {
		next.Feedback = feedback
	}
	if classification := strings.TrimSpace(materialized.Classification); classification != "" {
		next.SelectedClassificationLabel = classification
	}
	next.RecordedOutputWork = append([]work.FactoryWorkItem(nil), materialized.MaterializedWork...)
	return next
}

func hasMaterializableOutput(proposals workerexecution.ProposedOutput) bool {
	return len(proposals.Primary) > 0 ||
		strings.TrimSpace(proposals.Feedback) != "" ||
		strings.TrimSpace(proposals.Classification) != ""
}

func validStatesByTypeFromNet(net *state.Net) map[string]map[string]bool {
	if net == nil {
		return nil
	}
	return state.ValidStatesByType(net.WorkTypes)
}

func lineageContextFromDispatch(dispatch work.WorkDispatch) work.MaterializationLineageContext {
	parent := ""
	if len(dispatch.Execution.WorkIDs) > 0 {
		parent = dispatch.Execution.WorkIDs[0]
	}
	return work.MaterializationLineageContext{
		DispatchID:               dispatch.DispatchID,
		RequestID:                dispatch.Execution.RequestID,
		SourceWorkIDs:            append([]string(nil), dispatch.Execution.WorkIDs...),
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), dispatch.PreviousChainingTraceIDs...),
		ParentWorkID:             parent,
		TraceID:                  dispatch.Execution.TraceID,
	}
}

func validWorkTypesFromNet(net *state.Net) map[string]bool {
	if net == nil || len(net.WorkTypes) == 0 {
		return nil
	}
	valid := make(map[string]bool, len(net.WorkTypes))
	for id := range net.WorkTypes {
		valid[id] = true
	}
	return valid
}

type runtimeModelEventOwner interface {
	RuntimeOwnsModelEventRecording() bool
}

func prepareDetachedModelRecording(cfg *runtimeConfig, previous attemptPreparation) attemptPreparation {
	if !runtimeModelRecordingEnabled(cfg) {
		return previous
	}
	return func(ctx context.Context, request workers.ExecuteRequest) (attemptTerminalFunc, error) {
		var previousTerminal attemptTerminalFunc
		var err error
		if previous != nil {
			previousTerminal, err = previous(ctx, request)
			if err != nil {
				return nil, err
			}
		}
		recordDetachedModelRequest(cfg, request)
		return func(terminalContext context.Context, terminalRequest workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
			recordDetachedModelResponse(cfg, terminalRequest, result, executeErr)
			if previousTerminal != nil {
				previousTerminal(terminalContext, terminalRequest, result, executeErr)
			}
		}, nil
	}
}

func runtimeModelRecordingEnabled(cfg *runtimeConfig) bool {
	if cfg == nil || cfg.eventHistory == nil {
		return false
	}
	owner, ok := cfg.executeService.(runtimeModelEventOwner)
	if !ok || !owner.RuntimeOwnsModelEventRecording() {
		return false
	}
	_, ok = cfg.eventHistory.(recordings.WorkerEventRecorder)
	return ok
}

func recordDetachedModelRequest(cfg *runtimeConfig, request workers.ExecuteRequest) {
	recorder := runtimeModelRecorder(cfg)
	if recorder == nil || !isModelExecution(request) {
		return
	}
	attempt := request.Attempt.Number
	if attempt <= 0 {
		attempt = 1
	}
	modelRequestID := detachedModelRequestID(request.Correlation.DispatchID, attempt)
	recorder.RecordModelEvent(workers.ModelEvent{
		ID: "factory-event/model-request/" + modelRequestID, Kind: workers.ModelEventKindRequest,
		EventTime: cfg.clock.Now(), Tick: modelEventTick(request),
		DispatchID: request.Correlation.DispatchID, RequestID: request.Correlation.RequestID,
		TraceIDs: nonEmptyStrings(request.Correlation.TraceID), WorkIDs: append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Request: &workers.ModelRequestEventPayload{
			ModelRequestID: modelRequestID, Attempt: attempt,
			Operation: strings.TrimSpace(request.Input.ModelOperation), Worker: executionWorkerName(request),
			Model: strings.TrimSpace(request.Target.Model.Name), ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
			WorkingDirectory: optionalString(request.Target.Environment.WorkingDirectory), Worktree: optionalString(request.Target.Workspace.Worktree),
		},
	})
}

func recordDetachedModelResponse(cfg *runtimeConfig, request workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
	recorder := runtimeModelRecorder(cfg)
	if recorder == nil || !isModelExecution(request) {
		return
	}
	attempt := request.Attempt.Number
	if attempt <= 0 {
		attempt = 1
	}
	modelRequestID := detachedModelRequestID(request.Correlation.DispatchID, attempt)
	payload := workers.ModelResponseEventPayload{
		ModelRequestID: modelRequestID, Attempt: attempt,
		Operation: strings.TrimSpace(request.Input.ModelOperation), Worker: executionWorkerName(request),
		Model: strings.TrimSpace(request.Target.Model.Name), ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
		DurationMillis: result.Metrics.Duration.Milliseconds(), ProviderSession: providerSessionFromExecuteResult(result),
		Bindings: resolvedModelBindings(request.Input.ModelBindings),
	}
	if executeErr != nil || result.Outcome == workers.ExecutionOutcomeFailed || result.Outcome == workers.ExecutionOutcomeCanceled {
		payload.Outcome = workers.InferenceOutcomeFailed
		payload.FailureDetail = modelFailureDetailFromExecute(result, executeErr)
	} else {
		payload.Outcome = workers.InferenceOutcomeSucceeded
		content := work.CloneWorkContentParts(result.Output.Primary)
		if len(content) > 0 {
			payload.OutputContent = &content
		} else if output := strings.TrimSpace(primaryOutputText(result.Output.Primary)); output != "" {
			payload.OutputPreview = &output
		}
	}
	if result.Diagnostics != nil {
		payload.Diagnostics, _ = json.Marshal(result.Diagnostics)
	}
	recorder.RecordModelEvent(workers.ModelEvent{
		ID:   fmt.Sprintf("factory-event/model-response/%s/%d", request.Correlation.DispatchID, attempt),
		Kind: workers.ModelEventKindResponse, EventTime: cfg.clock.Now(), Tick: modelEventTick(request),
		DispatchID: request.Correlation.DispatchID, RequestID: request.Correlation.RequestID,
		TraceIDs: nonEmptyStrings(request.Correlation.TraceID), WorkIDs: append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Response: &payload,
	})
}

func runtimeModelRecorder(cfg *runtimeConfig) recordings.WorkerEventRecorder {
	if !runtimeModelRecordingEnabled(cfg) {
		return nil
	}
	recorder, _ := cfg.eventHistory.(recordings.WorkerEventRecorder)
	return recorder
}

func isModelExecution(request workers.ExecuteRequest) bool {
	return strings.TrimSpace(request.Target.Model.Name) != "" &&
		!strings.EqualFold(strings.TrimSpace(request.Target.RunnerID), "script") &&
		!strings.EqualFold(strings.TrimSpace(request.Target.RunnerID), "inference")
}

func detachedModelRequestID(dispatchID string, attempt int) string {
	return fmt.Sprintf("%s/model-request/%d", strings.TrimSpace(dispatchID), attempt)
}

func modelEventTick(request workers.ExecuteRequest) int {
	if request.Input.Dispatch.Execution.CurrentTick != 0 {
		return request.Input.Dispatch.Execution.CurrentTick
	}
	return request.Input.Dispatch.Execution.DispatchCreatedTick
}

func executionWorkerName(request workers.ExecuteRequest) string {
	if name := strings.TrimSpace(request.Target.WorkerName); name != "" {
		return name
	}
	return strings.TrimSpace(request.Target.WorkerType)
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}

func nonEmptyStrings(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func resolvedModelBindings(bindings []workers.ResolvedModelOperationBinding) *[]workers.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	clone := workers.CloneResolvedModelOperationBindings(bindings)
	return &clone
}

func providerSessionFromExecuteResult(result workers.ExecuteResult) *workers.ProviderSessionMetadata {
	if result.Continuation == nil {
		return nil
	}
	continuation := result.Continuation
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" && strings.TrimSpace(continuation.Provider) == "" {
		return nil
	}
	return &workers.ProviderSessionMetadata{Provider: continuation.Provider, Kind: "session", ID: id}
}

func modelFailureDetailFromExecute(result workers.ExecuteResult, executeErr error) *workers.FailureDetail {
	if result.Failure != nil {
		if result.Failure.Detail != nil {
			clone := *result.Failure.Detail
			return &clone
		}
		return &workers.FailureDetail{Reason: result.Failure.Type, Message: result.Failure.Message}
	}
	if executeErr != nil {
		return &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "The model request failed without an available explanation."}
	}
	return nil
}
