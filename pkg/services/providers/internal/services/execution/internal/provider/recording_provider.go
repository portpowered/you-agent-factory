package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	inferenceRequestEventIDPrefix  = "factory-event/inference-request"
	inferenceResponseEventIDPrefix = "factory-event/inference-response"
)

// InferenceEventRecorder receives provider-boundary inference facts.
type InferenceEventRecorder = workerexecution.InferenceEventRecorder

// RecordingProvider wraps a Provider and emits inference request/response events
// around each delegated provider call.
type RecordingProvider struct {
	inner    providercontract.Provider
	recorder InferenceEventRecorder
	now      func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

// NewRecordingProvider creates a Provider wrapper that records inference facts
// before and after calls to inner.
func NewRecordingProvider(
	inner providercontract.Provider,
	recorder InferenceEventRecorder,
	now func() time.Time,
) *RecordingProvider {
	provider := &RecordingProvider{
		inner:    inner,
		recorder: recorder,
		now:      now,
		attempts: make(map[string]int),
	}
	return provider
}

// Infer records a request event, delegates to the wrapped provider, then records
// the matching response event with success or failure details.
func (p *RecordingProvider) Infer(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	if p == nil || p.now == nil {
		return workerexecution.InferenceResponse{}, NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"recording provider clock is required",
			nil,
		)
	}
	attempt := p.nextAttempt(req.Dispatch.DispatchID)
	inferenceRequestID := inferenceRequestID(req.Dispatch.DispatchID, attempt)
	started := p.now()
	p.record(inferenceRequestEvent(req, attempt, inferenceRequestID, started))

	resp, err := p.inferInner(ctx, req)

	ended := p.now()
	p.record(inferenceResponseEvent(req, resp, err, attempt, inferenceRequestID, ended.Sub(started), ended))
	if err == nil || !isRetryableProviderFailure(err) {
		p.clearAttempts(req.Dispatch.DispatchID)
	}
	return resp, err
}

func (p *RecordingProvider) inferInner(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	if p.inner == nil {
		return workerexecution.InferenceResponse{}, NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"recording provider requires an inner provider",
			nil,
		)
	}
	return p.inner.Infer(ctx, req)
}

func (p *RecordingProvider) record(event workerexecution.InferenceEvent) {
	if p.recorder != nil {
		p.recorder(event)
	}
}

func (p *RecordingProvider) nextAttempt(dispatchID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.attempts[dispatchID]++
	return p.attempts[dispatchID]
}

func (p *RecordingProvider) clearAttempts(dispatchID string) {
	p.mu.Lock()
	delete(p.attempts, dispatchID)
	p.mu.Unlock()
}

func inferenceRequestID(dispatchID string, attempt int) string {
	if dispatchID == "" {
		return fmt.Sprintf("inference-request/%d", attempt)
	}
	return fmt.Sprintf("%s/inference-request/%d", dispatchID, attempt)
}

func inferenceRequestEvent(req workerexecution.ProviderInferenceRequest, attempt int, inferenceRequestID string, eventTime time.Time) workerexecution.InferenceEvent {
	payload := workerexecution.InferenceRequestEventPayload{
		InferenceRequestID: inferenceRequestID,
		Attempt:            attempt,
		WorkingDirectory:   req.WorkingDirectory,
		Worktree:           req.Worktree,
		Prompt:             req.UserMessage,
	}
	return inferenceEvent(req, eventTime, workerexecution.InferenceEventKindRequest,
		fmt.Sprintf("%s/%s", inferenceRequestEventIDPrefix, inferenceRequestID), &payload, nil)
}

func inferenceEvent(
	req workerexecution.ProviderInferenceRequest,
	eventTime time.Time,
	kind workerexecution.InferenceEventKind,
	id string,
	request *workerexecution.InferenceRequestEventPayload,
	response *workerexecution.InferenceResponseEventPayload,
) workerexecution.InferenceEvent {
	return workerexecution.InferenceEvent{
		ID:         id,
		Kind:       kind,
		EventTime:  eventTime.UTC(),
		Tick:       inferenceEventTick(req.Dispatch.Execution),
		DispatchID: req.Dispatch.DispatchID,
		RequestID:  req.Dispatch.Execution.RequestID,
		TraceIDs:   stringSlice(req.Dispatch.Execution.TraceID),
		WorkIDs:    stringSlice(req.Dispatch.Execution.WorkIDs...),
		Request:    request,
		Response:   response,
	}
}

func inferenceResponseEvent(req workerexecution.ProviderInferenceRequest, resp workerexecution.InferenceResponse, err error, attempt int, inferenceRequestID string, duration time.Duration, eventTime time.Time) workerexecution.InferenceEvent {
	payload := workerexecution.InferenceResponseEventPayload{
		InferenceRequestID: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     duration.Milliseconds(),
	}
	baseDiagnostics := workDiagnosticsForInferenceRequest(req)
	if err != nil {
		payload.Outcome = workerexecution.InferenceOutcomeFailed
		payload.FailureDetail = providerFailureDetail(err)
		payload.ExitCode = providerErrorExitCode(err)
		payload.ProviderSession = canonicalProviderSession(providerSessionFromInferenceError(err))
		payload.Diagnostics = safeInferenceDiagnosticsEventPayload(
			mergeWorkDiagnostics(
				withInferenceErrorDiagnostics(baseDiagnostics, err, attempt-1),
				diagnosticsFromInferenceError(err),
			),
		)
	} else {
		payload.Outcome = workerexecution.InferenceOutcomeSucceeded
		payload.Response = stringPtr(resp.Content)
		payload.ProviderSession = canonicalProviderSession(resp.ProviderSession)
		payload.Diagnostics = safeInferenceDiagnosticsEventPayload(
			withInferenceResponseDiagnostics(baseDiagnostics, resp, attempt-1),
		)
	}
	return inferenceEvent(req, eventTime, workerexecution.InferenceEventKindResponse,
		fmt.Sprintf("%s/%s", inferenceResponseEventIDPrefix, inferenceRequestID), nil, &payload)
}

func providerFailureDetail(err error) *workerexecution.InferenceResponseFailureDetail {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message := strings.TrimSpace(providerErr.Message)
		if message == "" {
			message = "The provider request failed without an available explanation."
		}
		return &workerexecution.InferenceResponseFailureDetail{Reason: workerexecution.WorkFailureType(providerErrorClass(err)), Message: message}
	}
	return &workerexecution.InferenceResponseFailureDetail{
		Reason:  workerexecution.WorkFailureTypeUnknown,
		Message: "The provider request failed without an available explanation.",
	}
}

func providerSessionFromInferenceError(err error) *workerexecution.ProviderSessionMetadata {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.ProviderSession
}

func diagnosticsFromInferenceError(err error) *workerexecution.WorkDiagnostics {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.Diagnostics
}

func inferenceEventTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func isRetryableProviderFailure(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	return WorkFailureDecisionFromProviderError(providerErr).Retryable
}

func providerErrorClass(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	return string(workerexecution.WorkFailureTypeUnknown)
}

func providerErrorExitCode(err error) *int {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		return nil
	}
	return workerEventExitCode(
		providerErr.Diagnostics.Command.ExitCode,
		true,
		omitZeroWorkerEventExitCode,
	)
}

func safeInferenceDiagnosticsEventPayload(diagnostics *workerexecution.WorkDiagnostics) json.RawMessage {
	payload, err := workerexecution.SafeWorkDiagnosticsEventPayload(
		workerexecution.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics),
	)
	if err != nil {
		panic(fmt.Sprintf("encode safe inference event diagnostics: %v", err))
	}
	return payload
}

func canonicalProviderSession(session *workerexecution.ProviderSessionMetadata) *workerexecution.ProviderSessionMetadata {
	cloned := workerexecution.CloneProviderSessionMetadata(session)
	if cloned != nil {
		cloned.Provider = workerexecution.CanonicalProviderSessionProvider(cloned.Provider)
	}
	return cloned
}

func stringPtr(value string) *string {
	return &value
}

func stringSlice(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var _ providercontract.Provider = (*RecordingProvider)(nil)
