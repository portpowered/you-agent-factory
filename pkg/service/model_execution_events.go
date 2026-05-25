package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

const (
	modelRequestEventIDPrefix      = "factory-event/model-request"
	modelResponseEventIDPrefix     = "factory-event/model-response"
	modelExecutionOutputPreviewMax = 200
)

type modelEventRecorder func(factoryapi.FactoryEvent)

type recordingModelRunner struct {
	inner      workers.Runner
	factoryCfg *interfaces.FactoryConfig
	workerDef  *interfaces.WorkerConfig
	recorder   modelEventRecorder
	now        func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

type modelExecutionEventTrace struct {
	mu sync.Mutex

	resourceWaitStartedAt time.Time
	resourceWaitMillis    int64
	resourceAcquired      bool

	loadRequested bool
	loadReused    bool
	loadStartedAt time.Time
	loadMillis    int64
}

type modelExecutionEventTraceKey struct{}

func newRecordingModelRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	recorder modelEventRecorder,
	now func() time.Time,
) workers.Runner {
	if inner == nil || workerDef == nil || recorder == nil {
		return inner
	}
	if now == nil {
		now = time.Now
	}
	return &recordingModelRunner{
		inner:      inner,
		factoryCfg: factoryCfg,
		workerDef: func() *interfaces.WorkerConfig {
			cloned := factoryconfig.CloneWorkerConfig(*workerDef)
			return &cloned
		}(),
		recorder: recorder,
		now:      now,
		attempts: make(map[string]int),
	}
}

func (r *recordingModelRunner) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if r == nil || r.inner == nil {
		return interfaces.RunnerExecutionResult{}, fmt.Errorf("model event recorder requires an inner runner")
	}

	attempt := r.nextAttempt(request.Dispatch.DispatchID)
	modelRequestID := modelRequestID(request.Dispatch.DispatchID, attempt)
	started := r.now()
	trace := &modelExecutionEventTrace{}
	ctx = context.WithValue(ctx, modelExecutionEventTraceKey{}, trace)

	r.record(modelRequestEvent(
		request,
		r.factoryCfg,
		r.workerDef,
		attempt,
		modelRequestID,
		started,
	))

	response, err := r.inner.Execute(ctx, request)
	finished := r.now()
	r.record(modelResponseEvent(
		request,
		response,
		err,
		r.factoryCfg,
		r.workerDef,
		trace,
		attempt,
		modelRequestID,
		finished.Sub(started),
		finished,
	))
	r.clearAttempts(request.Dispatch.DispatchID)
	return response, err
}

func (r *recordingModelRunner) record(event factoryapi.FactoryEvent) {
	if r != nil && r.recorder != nil {
		r.recorder(event)
	}
}

func (r *recordingModelRunner) nextAttempt(dispatchID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts[dispatchID]++
	return r.attempts[dispatchID]
}

func (r *recordingModelRunner) clearAttempts(dispatchID string) {
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

func modelRequestEvent(
	request interfaces.RunnerExecutionRequest,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	attempt int,
	modelRequestID string,
	eventTime time.Time,
) factoryapi.FactoryEvent {
	payload := factoryapi.ModelRequestEventPayload{
		ModelRequestId: modelRequestID,
		Attempt:        attempt,
		Operation:      strings.TrimSpace(request.ModelOperation),
		Worker:         modelEventFirstNonEmpty(request.WorkerType, workerNameForModelEvents(workerDef)),
		Model:          modelEventFirstNonEmpty(request.Model, modelNameForModelEvents(workerDef)),
		ProviderLocality: modelEventFirstNonEmpty(
			strings.TrimSpace(request.ModelLocality),
			modelLocalityForModelEvents(workerDef),
		),
		Resources:        modelEventResourceSummaries(factoryCfg, workerDef),
		Bindings:         modelEventResolvedBindings(request.ModelBindings),
		WorkingDirectory: modelEventStringPtr(request.WorkingDirectory),
		Worktree:         modelEventStringPtr(request.Worktree),
	}

	var union factoryapi.FactoryEvent_Payload
	if err := union.FromModelRequestEventPayload(payload); err != nil {
		panic(fmt.Sprintf("model request event payload: %v", err))
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeModelRequest,
		Id:            fmt.Sprintf("%s/%s", modelRequestEventIDPrefix, modelRequestID),
		Context:       modelEventContext(request, eventTime),
		Payload:       union,
	}
}

func modelResponseEvent(
	request interfaces.RunnerExecutionRequest,
	response interfaces.RunnerExecutionResult,
	err error,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	trace *modelExecutionEventTrace,
	attempt int,
	modelRequestID string,
	duration time.Duration,
	eventTime time.Time,
) factoryapi.FactoryEvent {
	payload := factoryapi.ModelResponseEventPayload{
		ModelRequestId: modelRequestID,
		Attempt:        attempt,
		Operation:      strings.TrimSpace(request.ModelOperation),
		Worker:         modelEventFirstNonEmpty(request.WorkerType, workerNameForModelEvents(workerDef)),
		Model:          modelEventFirstNonEmpty(request.Model, modelNameForModelEvents(workerDef)),
		ProviderLocality: modelEventFirstNonEmpty(
			strings.TrimSpace(request.ModelLocality),
			modelLocalityForModelEvents(workerDef),
		),
		DurationMillis: duration.Milliseconds(),
		Resources:      modelEventResourceSummaries(factoryCfg, workerDef),
		Bindings:       modelEventResolvedBindings(request.ModelBindings),
	}

	if err != nil {
		payload.Outcome = factoryapi.InferenceOutcomeFailed
		payload.ErrorClass = modelEventStringPtr(modelEventErrorClass(err))
		payload.Diagnostics = modelEventDiagnostics(nil, err)
	} else {
		payload.Outcome = factoryapi.InferenceOutcomeSucceeded
		payload.Diagnostics = modelEventDiagnostics(response.Diagnostics, nil)
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
	outputContent := modelEventOutputContent(response.Content)
	payload.OutputContent = outputContent
	if outputContent == nil {
		payload.OutputPreview = modelEventStringPtr(truncate(strings.TrimSpace(response.Content), modelExecutionOutputPreviewMax))
	}

	var union factoryapi.FactoryEvent_Payload
	if err := union.FromModelResponseEventPayload(payload); err != nil {
		panic(fmt.Sprintf("model response event payload: %v", err))
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeModelResponse,
		Id:            fmt.Sprintf("%s/%s", modelResponseEventIDPrefix, modelRequestID),
		Context:       modelEventContext(request, eventTime),
		Payload:       union,
	}
}

func modelEventContext(request interfaces.RunnerExecutionRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		Tick:       workersExecutionTick(request.Dispatch.Execution),
		EventTime:  eventTime,
		DispatchId: modelEventStringPtr(request.Dispatch.DispatchID),
		RequestId:  modelEventStringPtr(request.Dispatch.Execution.RequestID),
		TraceIds:   modelEventStringSlicePtr(request.Dispatch.Execution.TraceID),
		WorkIds:    modelEventStringSlicePtr(request.Dispatch.Execution.WorkIDs...),
	}
}

func workersExecutionTick(metadata interfaces.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func workerNameForModelEvents(workerDef *interfaces.WorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Name)
}

func modelNameForModelEvents(workerDef *interfaces.WorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Model)
}

func modelLocalityForModelEvents(workerDef *interfaces.WorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.ModelLocality)
}

func modelEventResolvedBindings(bindings []interfaces.ResolvedModelOperationBinding) *[]factoryapi.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	generated := make([]factoryapi.ResolvedModelOperationBinding, 0, len(bindings))
	for _, binding := range bindings {
		generated = append(generated, factoryapi.ResolvedModelOperationBinding{
			Slot:    binding.Slot,
			Source:  factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: modelEventGeneratedWorkContent(binding.Content),
		})
	}
	return &generated
}

func modelEventResourceSummaries(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) *[]factoryapi.ModelResourceSummary {
	if factoryCfg == nil || workerDef == nil || len(workerDef.Resources) == 0 {
		return nil
	}
	resourcesByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}
	summaries := make([]factoryapi.ModelResourceSummary, 0, len(workerDef.Resources))
	seen := make(map[string]struct{}, len(workerDef.Resources))
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok {
			continue
		}
		if _, ok := seen[resource.Name]; ok {
			continue
		}
		summaries = append(summaries, generatedModelResourceSummary(resource))
		seen[resource.Name] = struct{}{}
	}
	if len(summaries) == 0 {
		return nil
	}
	return &summaries
}

func modelEventDiagnostics(success *interfaces.WorkDiagnostics, err error) *factoryapi.SafeWorkDiagnostics {
	if success != nil {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(success)
	}
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
	}
	return nil
}

func modelEventErrorClass(err error) string {
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	if err == nil {
		return ""
	}
	return "MODEL_EXECUTION_FAILED"
}

func modelEventOutputContent(raw string) *factoryapi.WorkContent {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var content factoryapi.WorkContent
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil && len(content) != 0 {
		return &content
	}
	var envelope struct {
		Content factoryapi.WorkContent `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Content) != 0 {
		return &envelope.Content
	}
	return workcontent.GeneratedPtrFromParts([]interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: raw,
	}})
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func modelExecutionTraceFromContext(ctx context.Context) *modelExecutionEventTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(modelExecutionEventTraceKey{}).(*modelExecutionEventTrace)
	return trace
}

func markModelExecutionResourceWaitStarted(ctx context.Context, startedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.resourceWaitStartedAt = startedAt
	trace.mu.Unlock()
}

func markModelExecutionResourceWaitFinished(ctx context.Context, finishedAt time.Time, acquired bool) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	if !trace.resourceWaitStartedAt.IsZero() {
		trace.resourceWaitMillis = finishedAt.Sub(trace.resourceWaitStartedAt).Milliseconds()
	}
	trace.resourceAcquired = acquired
	trace.mu.Unlock()
}

func markModelExecutionLoadRequested(ctx context.Context, startedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.loadRequested = true
	trace.loadStartedAt = startedAt
	trace.mu.Unlock()
}

func markModelExecutionLoadFinished(ctx context.Context, finishedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	if !trace.loadStartedAt.IsZero() {
		trace.loadMillis = finishedAt.Sub(trace.loadStartedAt).Milliseconds()
	}
	trace.mu.Unlock()
}

func markModelExecutionLoadReused(ctx context.Context) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.loadRequested = true
	trace.loadReused = true
	trace.mu.Unlock()
}

func boolPtr(value bool) *bool {
	return &value
}

func int64PtrIfPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func modelEventGeneratedWorkContent(parts []interfaces.WorkContentPart) factoryapi.WorkContent {
	content := workcontent.GeneratedPtrFromParts(parts)
	if content == nil {
		return nil
	}
	return *content
}

func modelEventStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func modelEventStringSlicePtr(values ...string) *[]string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func modelEventFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
