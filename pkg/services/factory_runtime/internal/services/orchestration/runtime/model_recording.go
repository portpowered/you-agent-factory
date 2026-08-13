package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type runtimeModelEventOwner interface {
	RuntimeOwnsModelEventRecording() bool
}

func prepareDetachedModelRecording(
	cfg *runtimeConfig,
	previous attemptPreparation,
) attemptPreparation {
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
		return func(
			terminalContext context.Context,
			terminalRequest workers.ExecuteRequest,
			result workers.ExecuteResult,
			executeErr error,
		) {
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
		ID:         "factory-event/model-request/" + modelRequestID,
		Kind:       workers.ModelEventKindRequest,
		EventTime:  cfg.clock.Now(),
		Tick:       modelEventTick(request),
		DispatchID: request.Correlation.DispatchID,
		RequestID:  request.Correlation.RequestID,
		TraceIDs:   nonEmptyStrings(request.Correlation.TraceID),
		WorkIDs:    append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Request: &workers.ModelRequestEventPayload{
			ModelRequestID:   modelRequestID,
			Attempt:          attempt,
			Operation:        strings.TrimSpace(request.Input.ModelOperation),
			Worker:           executionWorkerName(request),
			Model:            strings.TrimSpace(request.Target.Model.Name),
			ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
			WorkingDirectory: optionalString(request.Target.Environment.WorkingDirectory),
			Worktree:         optionalString(request.Target.Workspace.Worktree),
		},
	})
}

func recordDetachedModelResponse(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
) {
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
		ModelRequestID:   modelRequestID,
		Attempt:          attempt,
		Operation:        strings.TrimSpace(request.Input.ModelOperation),
		Worker:           executionWorkerName(request),
		Model:            strings.TrimSpace(request.Target.Model.Name),
		ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
		DurationMillis:   result.Metrics.Duration.Milliseconds(),
		ProviderSession:  providerSessionFromExecuteResult(result),
		Bindings:         resolvedModelBindings(request.Input.ModelBindings),
	}
	if executeErr != nil || result.Outcome == workers.ExecutionOutcomeFailed ||
		result.Outcome == workers.ExecutionOutcomeCanceled {
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
		ID:         fmt.Sprintf("factory-event/model-response/%s/%d", request.Correlation.DispatchID, attempt),
		Kind:       workers.ModelEventKindResponse,
		EventTime:  cfg.clock.Now(),
		Tick:       modelEventTick(request),
		DispatchID: request.Correlation.DispatchID,
		RequestID:  request.Correlation.RequestID,
		TraceIDs:   nonEmptyStrings(request.Correlation.TraceID),
		WorkIDs:    append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Response:   &payload,
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
	return &workers.ProviderSessionMetadata{
		Provider: continuation.Provider,
		Kind:     "session",
		ID:       id,
	}
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
