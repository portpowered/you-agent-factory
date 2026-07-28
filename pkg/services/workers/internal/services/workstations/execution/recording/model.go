// Package recording decorates worker runners with canonical model request and
// response event recording. It also owns the trace hooks used by managed local
// model runtimes so every construction path records the same evidence.
package recording

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

const (
	modelRequestEventIDPrefix      = "factory-event/model-request"
	modelResponseEventIDPrefix     = "factory-event/model-response"
	modelExecutionOutputPreviewMax = 200
)

// Recorder persists one canonical model execution event.
type Recorder = workerexecution.ModelEventRecorder

type runner struct {
	inner      workers.Runner
	factoryCfg *interfaces.FactoryConfig
	workerDef  *interfaces.FactoryWorkerConfig
	recorder   Recorder
	now        func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

type executionTrace struct {
	mu sync.Mutex

	resourceWaitStartedAt time.Time
	resourceWaitMillis    int64
	resourceAcquired      bool
	loadRequested         bool
	loadReused            bool
	loadStartedAt         time.Time
	loadMillis            int64
}

type executionTraceKey struct{}

// NewRunner wraps inner when model events can be recorded. A missing runner,
// worker definition, or recorder leaves the runner unchanged.
func NewRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	recorder Recorder,
	now func() time.Time,
) workers.Runner {
	if inner == nil || workerDef == nil || recorder == nil {
		return inner
	}
	cloned := interfaces.CloneWorkerConfig(*workerDef)
	return &runner{
		inner: inner, factoryCfg: factoryCfg, workerDef: &cloned,
		recorder: recorder, now: now, attempts: make(map[string]int),
	}
}

func (r *runner) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	if r == nil || r.inner == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("model event recorder requires an inner runner")
	}
	if r.now == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("model event recorder clock is required")
	}
	attempt := r.nextAttempt(request.Dispatch.DispatchID)
	requestID := modelRequestID(request.Dispatch.DispatchID, attempt)
	started := r.now()
	trace := &executionTrace{}
	ctx = context.WithValue(ctx, executionTraceKey{}, trace)
	r.record(requestEvent(request, r.factoryCfg, r.workerDef, attempt, requestID, started))
	response, err := r.inner.Execute(ctx, request)
	finished := r.now()
	r.record(responseEvent(request, response, err, r.factoryCfg, r.workerDef, trace, attempt, requestID, finished.Sub(started), finished))
	r.clearAttempts(request.Dispatch.DispatchID)
	return response, err
}

func (r *runner) record(event workerexecution.ModelEvent) {
	if r != nil && r.recorder != nil {
		r.recorder(event)
	}
}

func (r *runner) nextAttempt(dispatchID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts[dispatchID]++
	return r.attempts[dispatchID]
}

func (r *runner) clearAttempts(dispatchID string) {
	r.mu.Lock()
	delete(r.attempts, dispatchID)
	r.mu.Unlock()
}

func modelRequestID(dispatchID string, attempt int) string {
	if strings.TrimSpace(dispatchID) == "" {
		return fmt.Sprintf("model-request/%d", attempt)
	}
	return fmt.Sprintf("%s/model-request/%d", dispatchID, attempt)
}

func requestEvent(request workerexecution.RunnerExecutionRequest, factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.FactoryWorkerConfig, attempt int, requestID string, eventTime time.Time) workerexecution.ModelEvent {
	payload := workerexecution.ModelRequestEventPayload{
		ModelRequestID:   requestID,
		Attempt:          attempt,
		Operation:        strings.TrimSpace(request.ModelOperation),
		Worker:           firstNonEmpty(request.WorkerType, workerName(workerDef)),
		Model:            firstNonEmpty(request.Model, modelName(workerDef)),
		ProviderLocality: firstNonEmpty(strings.TrimSpace(request.ModelLocality), modelLocality(workerDef)),
		Resources:        resourceSummaries(factoryCfg, workerDef),
		Bindings:         resolvedBindings(request.ModelBindings),
		WorkingDirectory: stringPtr(request.WorkingDirectory),
		Worktree:         stringPtr(request.Worktree),
	}
	return Event(request, workerexecution.ModelEventKindRequest, fmt.Sprintf("%s/%s", modelRequestEventIDPrefix, requestID), eventTime, &payload, nil)
}

func responseEvent(request workerexecution.RunnerExecutionRequest, response workerexecution.RunnerExecutionResult, err error, factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.FactoryWorkerConfig, trace *executionTrace, attempt int, requestID string, duration time.Duration, eventTime time.Time) workerexecution.ModelEvent {
	payload := workerexecution.ModelResponseEventPayload{
		ModelRequestID:   requestID,
		Attempt:          attempt,
		Operation:        strings.TrimSpace(request.ModelOperation),
		Worker:           firstNonEmpty(request.WorkerType, workerName(workerDef)),
		Model:            firstNonEmpty(request.Model, modelName(workerDef)),
		ProviderLocality: firstNonEmpty(strings.TrimSpace(request.ModelLocality), modelLocality(workerDef)),
		DurationMillis:   duration.Milliseconds(),
		Resources:        resourceSummaries(factoryCfg, workerDef),
		Bindings:         resolvedBindings(request.ModelBindings),
	}
	if err != nil {
		payload.Outcome = workerexecution.InferenceOutcomeFailed
		payload.FailureDetail = &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureTypeUnknown, Message: "The model request failed without an available explanation."}
		payload.Diagnostics = Diagnostics(nil, err)
	} else {
		payload.Outcome = workerexecution.InferenceOutcomeSucceeded
		payload.Diagnostics = Diagnostics(response.Diagnostics, nil)
	}
	if trace != nil {
		trace.mu.Lock()
		payload.ResourceWaitMillis = int64PtrIfPositive(trace.resourceWaitMillis)
		payload.ResourceAcquired = boolPtr(trace.resourceAcquired)
		payload.LoadRequested = boolPtr(trace.loadRequested)
		payload.LoadReused = boolPtr(trace.loadReused)
		payload.LoadDurationMillis = int64PtrIfPositive(trace.loadMillis)
		trace.mu.Unlock()
	}
	payload.OutputContent = outputContent(response.Content)
	if payload.OutputContent == nil {
		payload.OutputPreview = stringPtr(truncate(strings.TrimSpace(response.Content), modelExecutionOutputPreviewMax))
	}
	return Event(request, workerexecution.ModelEventKindResponse, fmt.Sprintf("%s/%s", modelResponseEventIDPrefix, requestID), eventTime, nil, &payload)
}

// Event constructs the canonical event envelope. It is exported for focused
// contract tests and callers that already have request or response payloads.
func Event(request workerexecution.RunnerExecutionRequest, kind workerexecution.ModelEventKind, id string, eventTime time.Time, requestPayload *workerexecution.ModelRequestEventPayload, responsePayload *workerexecution.ModelResponseEventPayload) workerexecution.ModelEvent {
	return workerexecution.ModelEvent{
		ID: id, Kind: kind, EventTime: interfaces.CanonicalEventTime(eventTime),
		Tick: executionTick(request.Dispatch.Execution), DispatchID: request.Dispatch.DispatchID,
		RequestID: request.Dispatch.Execution.RequestID,
		TraceIDs:  stringsIfPresent(request.Dispatch.Execution.TraceID),
		WorkIDs:   stringsIfPresent(request.Dispatch.Execution.WorkIDs...),
		Request:   requestPayload, Response: responsePayload,
	}
}

func executionTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func workerName(workerDef *interfaces.FactoryWorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Name)
}

func modelName(workerDef *interfaces.FactoryWorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Model)
}

func modelLocality(workerDef *interfaces.FactoryWorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.ModelLocality)
}

func resolvedBindings(bindings []workerexecution.ResolvedModelOperationBinding) *[]workerexecution.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	cloned := workerexecution.CloneResolvedModelOperationBindings(bindings)
	return &cloned
}

func resourceSummaries(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.FactoryWorkerConfig) *[]workerexecution.ModelResourceSummary {
	if factoryCfg == nil || workerDef == nil || len(workerDef.Resources) == 0 {
		return nil
	}
	resourcesByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}
	summaries := make([]workerexecution.ModelResourceSummary, 0, len(workerDef.Resources))
	seen := make(map[string]struct{}, len(workerDef.Resources))
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok {
			continue
		}
		if _, ok := seen[resource.Name]; ok {
			continue
		}
		summaries = append(summaries, workerexecution.ModelResourceSummary{
			Name: resource.Name, Type: strings.TrimSpace(resource.Type), Capacity: resource.Capacity,
			Model: stringPtr(resource.Model), Backend: stringPtr(resource.Backend),
			LoadPolicy: stringPtr(resource.LoadPolicy), Provider: stringPtr(resource.Provider),
		})
		seen[resource.Name] = struct{}{}
	}
	if len(summaries) == 0 {
		return nil
	}
	return &summaries
}

func outputContent(raw string) *[]work.WorkContentPart {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil && len(content) != 0 {
		return &content
	}
	var envelope struct {
		Content []work.WorkContentPart `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Content) != 0 {
		return &envelope.Content
	}
	return &[]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: raw}}
}

// Diagnostics returns the redacted event-safe diagnostics payload shared by
// every model execution recording path.
func Diagnostics(success *workerexecution.WorkDiagnostics, executionErr error) json.RawMessage {
	var safe *workerdiagnostics.SafeWorkDiagnostics
	if success != nil {
		safe = workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(success)
	} else {
		var providerErr *workerprovider.ProviderError
		if errors.As(executionErr, &providerErr) {
			safe = workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
		}
	}
	payload, encodeErr := workerdiagnostics.SafeWorkDiagnosticsEventPayload(safe)
	if encodeErr != nil || string(payload) == "null" {
		return nil
	}
	return payload
}

// Hooks returns the local-model trace hooks paired with NewRunner.
func Hooks() models.LocalRuntimeHooks {
	return models.LocalRuntimeHooks{
		MarkResourceWaitStarted:  markResourceWaitStarted,
		MarkResourceWaitFinished: markResourceWaitFinished,
		MarkLoadRequested:        markLoadRequested,
		MarkLoadFinished:         markLoadFinished,
		MarkLoadReused:           markLoadReused,
	}
}

func traceFromContext(ctx context.Context) *executionTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(executionTraceKey{}).(*executionTrace)
	return trace
}

func markResourceWaitStarted(ctx context.Context, startedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.resourceWaitStartedAt = startedAt
		trace.mu.Unlock()
	}
}

func markResourceWaitFinished(ctx context.Context, finishedAt time.Time, acquired bool) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		if !trace.resourceWaitStartedAt.IsZero() {
			trace.resourceWaitMillis = finishedAt.Sub(trace.resourceWaitStartedAt).Milliseconds()
		}
		trace.resourceAcquired = acquired
		trace.mu.Unlock()
	}
}

func markLoadRequested(ctx context.Context, startedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.loadRequested = true
		trace.loadStartedAt = startedAt
		trace.mu.Unlock()
	}
}

func markLoadFinished(ctx context.Context, finishedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		if !trace.loadStartedAt.IsZero() {
			trace.loadMillis = finishedAt.Sub(trace.loadStartedAt).Milliseconds()
		}
		trace.mu.Unlock()
	}
}

func markLoadReused(ctx context.Context) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.loadRequested = true
		trace.loadReused = true
		trace.mu.Unlock()
	}
}

func boolPtr(value bool) *bool { return &value }
func int64PtrIfPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}
func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringsIfPresent(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
