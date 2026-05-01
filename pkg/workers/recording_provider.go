package workers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

const (
	inferenceRequestEventIDPrefix  = "factory-event/inference-request"
	inferenceResponseEventIDPrefix = "factory-event/inference-response"
)

// InferenceEventRecorder receives generated provider-boundary inference events.
type InferenceEventRecorder func(factoryapi.FactoryEvent)

// RecordingProvider wraps a Provider and emits inference request/response events
// around each delegated provider call.
type RecordingProvider struct {
	inner    Provider
	recorder InferenceEventRecorder
	now      func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

// RecordingProviderOption configures a RecordingProvider.
type RecordingProviderOption func(*RecordingProvider)

// WithRecordingProviderClock sets the clock used for event occurrence times and
// provider-call duration measurement.
func WithRecordingProviderClock(now func() time.Time) RecordingProviderOption {
	return func(p *RecordingProvider) {
		if now != nil {
			p.now = now
		}
	}
}

// NewRecordingProvider creates a Provider wrapper that records generated
// inference events before and after calls to inner.
func NewRecordingProvider(inner Provider, recorder InferenceEventRecorder, opts ...RecordingProviderOption) *RecordingProvider {
	provider := &RecordingProvider{
		inner:    inner,
		recorder: recorder,
		now:      time.Now,
		attempts: make(map[string]int),
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

// Infer records a request event, delegates to the wrapped provider, then records
// the matching response event with success or failure details.
func (p *RecordingProvider) Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
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

func (p *RecordingProvider) inferInner(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	if p.inner == nil {
		return interfaces.InferenceResponse{}, NewProviderError(
			interfaces.ProviderErrorTypeMisconfigured,
			"recording provider requires an inner provider",
			nil,
		)
	}
	return p.inner.Infer(ctx, req)
}

func (p *RecordingProvider) record(event factoryapi.FactoryEvent) {
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

func inferenceRequestEvent(req interfaces.ProviderInferenceRequest, attempt int, inferenceRequestID string, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InferenceRequestEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		WorkingDirectory:   req.WorkingDirectory,
		Worktree:           req.Worktree,
		Prompt:             req.UserMessage,
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceRequest,
		Id:            fmt.Sprintf("%s/%s", inferenceRequestEventIDPrefix, inferenceRequestID),
		Context:       inferenceEventContext(req, eventTime),
		Payload:       inferenceRequestFactoryEventPayload(payload),
	}
}

func inferenceResponseEvent(req interfaces.ProviderInferenceRequest, resp interfaces.InferenceResponse, err error, attempt int, inferenceRequestID string, duration time.Duration, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InferenceResponseEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     duration.Milliseconds(),
	}
	baseDiagnostics := workDiagnosticsForInferenceRequest(req)
	if err != nil {
		payload.Outcome = factoryapi.InferenceOutcomeFailed
		payload.ErrorClass = stringPtr(providerErrorClass(err))
		payload.ExitCode = providerErrorExitCode(err)
		payload.ProviderSession = interfaces.GeneratedProviderSessionMetadata(providerSessionFromInferenceError(err))
		payload.Diagnostics = interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(
			mergeWorkDiagnostics(
				withInferenceErrorDiagnostics(baseDiagnostics, err, attempt-1),
				diagnosticsFromInferenceError(err),
			),
		)
	} else {
		payload.Outcome = factoryapi.InferenceOutcomeSucceeded
		payload.Response = stringPtr(resp.Content)
		payload.ProviderSession = interfaces.GeneratedProviderSessionMetadata(resp.ProviderSession)
		payload.Diagnostics = interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(
			withInferenceResponseDiagnostics(baseDiagnostics, resp, attempt-1),
		)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceResponse,
		Id:            fmt.Sprintf("%s/%s", inferenceResponseEventIDPrefix, inferenceRequestID),
		Context:       inferenceEventContext(req, eventTime),
		Payload:       inferenceResponseFactoryEventPayload(payload),
	}
}

func providerSessionFromInferenceError(err error) *interfaces.ProviderSessionMetadata {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.ProviderSession
}

func diagnosticsFromInferenceError(err error) *interfaces.WorkDiagnostics {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.Diagnostics
}

func inferenceEventContext(req interfaces.ProviderInferenceRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		Tick:       inferenceEventTick(req.Dispatch.Execution),
		EventTime:  eventTime,
		DispatchId: stringPtrIfNotEmpty(req.Dispatch.DispatchID),
		RequestId:  stringPtrIfNotEmpty(req.Dispatch.Execution.RequestID),
		TraceIds:   stringSlicePtr(req.Dispatch.Execution.TraceID),
		WorkIds:    stringSlicePtr(req.Dispatch.Execution.WorkIDs...),
	}
}

func inferenceEventTick(metadata interfaces.ExecutionMetadata) int {
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
	return ClassifyProviderFailure(providerErr).Retryable
}

func providerErrorClass(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	return string(interfaces.ProviderErrorTypeUnknown)
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

func inferenceRequestFactoryEventPayload(payload factoryapi.InferenceRequestEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromInferenceRequestEventPayload(payload); err != nil {
		panic(fmt.Sprintf("inference request event payload: %v", err))
	}
	return out
}

func inferenceResponseFactoryEventPayload(payload factoryapi.InferenceResponseEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromInferenceResponseEventPayload(payload); err != nil {
		panic(fmt.Sprintf("inference response event payload: %v", err))
	}
	return out
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringSlicePtr(values ...string) *[]string {
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
	return &out
}

var _ Provider = (*RecordingProvider)(nil)
