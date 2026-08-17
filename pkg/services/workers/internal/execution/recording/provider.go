package recording

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	inferenceRequestEventIDPrefix  = "factory-event/inference-request"
	inferenceResponseEventIDPrefix = "factory-event/inference-response"
)

// providerRunner decorates the final Workers runner so every provider attempt
// has the same canonical request/response evidence, including request-scoped
// provider overrides used by detached Factory Runtime executions.
type providerRunner struct {
	inner    workers.Runner
	recorder workers.InferenceEventRecorder
	now      func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

// NewProviderRunner attaches canonical inference recording to inner. The
// recorder is a detached capability: when it is absent, the original runner is
// returned unchanged and no recording state is allocated.
func NewProviderRunner(
	inner workers.Runner,
	recorder workers.InferenceEventRecorder,
	now func() time.Time,
) workers.Runner {
	if inner == nil || recorder == nil {
		return inner
	}
	return &providerRunner{
		inner: inner, recorder: recorder, now: now,
		attempts: make(map[string]int),
	}
}

func (r *providerRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if r == nil || r.inner == nil {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"inference recording requires an inner runner",
			nil,
		)
	}
	if r.now == nil {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"inference recording clock is required",
			nil,
		)
	}
	attempt := r.nextAttempt(request.Dispatch.DispatchID)
	inferenceRequestID := inferenceRequestID(request.Dispatch.DispatchID, attempt)
	started := r.now()
	r.record(inferenceRequestEvent(request, attempt, inferenceRequestID, started))

	response, err := r.inner.Execute(ctx, request)
	finished := r.now()
	r.record(inferenceResponseEvent(
		request, response, err, attempt, inferenceRequestID,
		finished.Sub(started), finished,
	))
	if err == nil || !retryableProviderFailure(err) {
		r.clearAttempts(request.Dispatch.DispatchID)
	}
	return response, err
}

func (r *providerRunner) record(event workers.InferenceEvent) {
	if r != nil && r.recorder != nil {
		// Recording is a detached observation side effect. A recorder panic must
		// not rewrite the inner runner result or prevent terminal response work.
		defer func() {
			_ = recover()
		}()
		r.recorder(event)
	}
}

func (r *providerRunner) nextAttempt(dispatchID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts[dispatchID]++
	return r.attempts[dispatchID]
}

func (r *providerRunner) clearAttempts(dispatchID string) {
	r.mu.Lock()
	delete(r.attempts, dispatchID)
	r.mu.Unlock()
}

func inferenceRequestID(dispatchID string, attempt int) string {
	if strings.TrimSpace(dispatchID) == "" {
		return fmt.Sprintf("inference-request/%d", attempt)
	}
	return fmt.Sprintf("%s/inference-request/%d", dispatchID, attempt)
}

func inferenceRequestEvent(
	request workers.RunnerExecutionRequest,
	attempt int,
	inferenceRequestID string,
	eventTime time.Time,
) workers.InferenceEvent {
	payload := workers.InferenceRequestEventPayload{
		InferenceRequestID: inferenceRequestID,
		Attempt:            attempt,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		Prompt:             request.UserMessage,
	}
	return inferenceEvent(
		request,
		eventTime,
		workers.InferenceEventKindRequest,
		fmt.Sprintf("%s/%s", inferenceRequestEventIDPrefix, inferenceRequestID),
		&payload,
		nil,
	)
}

func inferenceResponseEvent(
	request workers.RunnerExecutionRequest,
	response workers.RunnerExecutionResult,
	executionErr error,
	attempt int,
	inferenceRequestID string,
	duration time.Duration,
	eventTime time.Time,
) workers.InferenceEvent {
	payload := workers.InferenceResponseEventPayload{
		InferenceRequestID: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     duration.Milliseconds(),
	}
	if executionErr != nil {
		payload.Outcome = workers.InferenceOutcomeFailed
		payload.FailureDetail = providerFailureDetail(executionErr)
		payload.ExitCode = providerErrorExitCode(executionErr)
		payload.Continuation = cloneContinuation(continuationFromError(executionErr))
		if !continuationHasSessionIdentity(payload.Continuation) {
			payload.ProviderSession = providerSessionForRequest(request, nil)
		}
		payload.Diagnostics = Diagnostics(nil, executionErr)
	} else {
		payload.Outcome = workers.InferenceOutcomeSucceeded
		payload.Response = stringPtr(response.Content)
		payload.Continuation = cloneContinuation(response.Continuation)
		payload.Diagnostics = Diagnostics(response.Diagnostics, nil)
	}
	return inferenceEvent(
		request,
		eventTime,
		workers.InferenceEventKindResponse,
		fmt.Sprintf("%s/%s", inferenceResponseEventIDPrefix, inferenceRequestID),
		nil,
		&payload,
	)
}

func inferenceEvent(
	request workers.RunnerExecutionRequest,
	eventTime time.Time,
	kind workers.InferenceEventKind,
	id string,
	requestPayload *workers.InferenceRequestEventPayload,
	responsePayload *workers.InferenceResponseEventPayload,
) workers.InferenceEvent {
	return workers.InferenceEvent{
		ID:         id,
		Kind:       kind,
		EventTime:  eventTime.UTC(),
		Tick:       executionTick(request.Dispatch.Execution),
		DispatchID: request.Dispatch.DispatchID,
		RequestID:  request.Dispatch.Execution.RequestID,
		TraceIDs:   stringsIfPresent(request.Dispatch.Execution.TraceID),
		WorkIDs:    stringsIfPresent(request.Dispatch.Execution.WorkIDs...),
		Request:    requestPayload,
		Response:   responsePayload,
	}
}

func providerFailureDetail(err error) *workers.InferenceResponseFailureDetail {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		return &workers.InferenceResponseFailureDetail{
			Reason:  workers.WorkFailureTypeUnknown,
			Message: "The provider request failed without an available explanation.",
		}
	}
	message := strings.TrimSpace(providerErr.Message)
	if message == "" {
		message = "The provider request failed without an available explanation."
	}
	reason := providerErr.Type
	if reason == "" {
		reason = workers.WorkFailureTypeUnknown
	}
	return &workers.InferenceResponseFailureDetail{Reason: reason, Message: message}
}

func continuationFromError(err error) *workers.ProviderContinuationRef {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		return nil
	}
	return providerErr.Continuation
}

func providerErrorExitCode(err error) *int {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil || providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		return nil
	}
	if providerErr.Diagnostics.Command.ExitCode == 0 {
		return nil
	}
	exitCode := providerErr.Diagnostics.Command.ExitCode
	return &exitCode
}

func cloneContinuation(reference *workers.ProviderContinuationRef) *workers.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}

func retryableProviderFailure(err error) bool {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		return false
	}
	return workers.FailureDecisionFromMetadata(&workers.WorkFailureMetadata{
		Family: providerErr.Family,
		Type:   providerErr.Type,
	}).Retryable
}
