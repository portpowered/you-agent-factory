package runtimehost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

const (
	modelRequestEventIDPrefix      = "factory-event/model-request"
	modelResponseEventIDPrefix     = "factory-event/model-response"
	modelExecutionOutputPreviewMax = 200
)

type modelEventRecorder func(workerexecution.ModelEvent)

type recordingModelRunner struct {
	inner      workers.Runner
	factoryCfg *interfaces.FactoryConfig
	workerDef  *workerconfig.Config
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
	workerDef *workerconfig.Config,
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
		workerDef: func() *workerconfig.Config {
			cloned := factoryconfig.CloneWorkerConfig(*workerDef)
			return &cloned
		}(),
		recorder: recorder,
		now:      now,
		attempts: make(map[string]int),
	}
}

func (r *recordingModelRunner) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	if r == nil || r.inner == nil {
		return workerexecution.RunnerExecutionResult{}, fmt.Errorf("model event recorder requires an inner runner")
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

func (r *recordingModelRunner) record(event workerexecution.ModelEvent) {
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
	request workerexecution.RunnerExecutionRequest,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *workerconfig.Config,
	attempt int,
	modelRequestID string,
	eventTime time.Time,
) workerexecution.ModelEvent {
	payload := workerexecution.ModelRequestEventPayload{
		ModelRequestID: modelRequestID,
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

	return modelEvent(request, workerexecution.ModelEventKindRequest, fmt.Sprintf("%s/%s", modelRequestEventIDPrefix, modelRequestID), eventTime, &payload, nil)
}

func modelResponseEvent(
	request workerexecution.RunnerExecutionRequest,
	response workerexecution.RunnerExecutionResult,
	err error,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *workerconfig.Config,
	trace *modelExecutionEventTrace,
	attempt int,
	modelRequestID string,
	duration time.Duration,
	eventTime time.Time,
) workerexecution.ModelEvent {
	payload := workerexecution.ModelResponseEventPayload{
		ModelRequestID: modelRequestID,
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
		payload.Outcome = workerexecution.InferenceOutcomeFailed
		payload.FailureDetail = &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureTypeUnknown,
			Message: "The model request failed without an available explanation.",
		}
		payload.Diagnostics = modelEventDiagnostics(nil, err)
	} else {
		payload.Outcome = workerexecution.InferenceOutcomeSucceeded
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

	return modelEvent(request, workerexecution.ModelEventKindResponse, fmt.Sprintf("%s/%s", modelResponseEventIDPrefix, modelRequestID), eventTime, nil, &payload)
}

func modelEvent(request workerexecution.RunnerExecutionRequest, kind workerexecution.ModelEventKind, id string, eventTime time.Time, requestPayload *workerexecution.ModelRequestEventPayload, responsePayload *workerexecution.ModelResponseEventPayload) workerexecution.ModelEvent {
	return workerexecution.ModelEvent{
		ID: id, Kind: kind, EventTime: interfaces.CanonicalEventTime(eventTime),
		Tick: workersExecutionTick(request.Dispatch.Execution), DispatchID: request.Dispatch.DispatchID,
		RequestID: request.Dispatch.Execution.RequestID,
		TraceIDs:  modelEventStrings(request.Dispatch.Execution.TraceID),
		WorkIDs:   modelEventStrings(request.Dispatch.Execution.WorkIDs...),
		Request:   requestPayload, Response: responsePayload,
	}
}

func snapshotHasActiveWork(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 {
		return true
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if snapshot.Topology == nil {
			return true
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}
	return false
}

func replacementFactoryChangePayload(events []interfaces.FactoryEvent) (interfaces.FactoryChangeEventPayload, bool) {
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeInitialStructureRequest {
			continue
		}
		var payload interfaces.InitialStructureRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return interfaces.FactoryChangeEventPayload{}, false
		}
		return interfaces.FactoryChangeEventPayload{
			Factory:         payload.Factory,
			Metadata:        payload.Metadata,
			SourceDirectory: payload.SourceDirectory,
		}, true
	}
	return interfaces.FactoryChangeEventPayload{}, false
}

func workersExecutionTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func workerNameForModelEvents(workerDef *workerconfig.Config) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Name)
}

func modelNameForModelEvents(workerDef *workerconfig.Config) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.Model)
}

func modelLocalityForModelEvents(workerDef *workerconfig.Config) string {
	if workerDef == nil {
		return ""
	}
	return strings.TrimSpace(workerDef.ModelLocality)
}

func modelEventResolvedBindings(bindings []workerexecution.ResolvedModelOperationBinding) *[]workerexecution.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	cloned := workerexecution.CloneResolvedModelOperationBindings(bindings)
	return &cloned
}

func modelEventResourceSummaries(factoryCfg *interfaces.FactoryConfig, workerDef *workerconfig.Config) *[]workerexecution.ModelResourceSummary {
	if factoryCfg == nil || workerDef == nil || len(workerDef.Resources) == 0 {
		return nil
	}
	resourcesByName := make(map[string]factoryresource.Config, len(factoryCfg.Resources))
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
			Model: modelEventStringPtr(resource.Model), Backend: modelEventStringPtr(resource.Backend),
			LoadPolicy: modelEventStringPtr(resource.LoadPolicy), Provider: modelEventStringPtr(resource.Provider),
		})
		seen[resource.Name] = struct{}{}
	}
	if len(summaries) == 0 {
		return nil
	}
	return &summaries
}

func modelEventOutputContent(raw string) *[]work.WorkContentPart {
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
	return &[]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: raw,
	}}
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

func modelEventStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func modelEventStrings(values ...string) []string {
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
	return out
}

func modelEventFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func submitWorkRequestWithFactory(activeFactory factory.Factory) workRequestSubmitter {
	if activeFactory == nil {
		return nil
	}
	return func(ctx context.Context, request work.WorkRequest) error {
		_, err := activeFactory.SubmitWorkRequest(ctx, request)
		return err
	}
}

func (fs *Host) currentRuntimeSubmitter() workRequestSubmitter {
	return submitWorkRequestWithFactory(fs.currentFactory())
}

func (fs *Host) preseedCurrentRuntimeInputs(ctx context.Context) error {
	runtimeBundle := fs.currentRuntimeBundle()
	if runtimeBundle == nil || runtimeBundle.Listener == nil {
		return nil
	}
	if err := runtimeBundle.Listener.PreseedInputs(ctx); err != nil {
		return fmt.Errorf("preseed inputs: %w", err)
	}
	return nil
}

func (fs *Host) startupRuntimeBundle() *factoryRuntimeBundle {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.startupBundle
}

func (fs *Host) setStartupBundle(runtimeBundle *factoryRuntimeBundle) {
	if fs == nil {
		return
	}
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.startupBundle = runtimeBundle
}

func (fs *Host) clearStartupBundle() {
	if fs == nil {
		return
	}
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.startupBundle = nil
}

func (fs *Host) syncActiveSessionDir(runtimeBundle *factoryRuntimeBundle) {
	if fs == nil || fs.cfg == nil {
		return
	}
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	if runtimeBundle == nil || strings.TrimSpace(runtimeBundle.Dir) == "" {
		if strings.TrimSpace(fs.factoryRootDir) != "" {
			fs.cfg.Dir = fs.factoryRootDir
		}
		return
	}
	fs.cfg.Dir = runtimeBundle.Dir
}

func (fs *Host) currentRunState() *hostRunState {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	return fs.runState
}

func (fs *Host) currentLiveRuntime() *liveRuntimeHandle {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	if fs.runState == nil {
		return nil
	}
	return fs.runState.runtime
}

func (fs *Host) setRunState(ctx context.Context, sessionID string, runtime *liveRuntimeHandle) {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	if ctx == nil {
		fs.runState = nil
		return
	}
	fs.runState = &hostRunState{
		ctx:       ctx,
		sessionID: sessionID,
		runtime:   runtime,
	}
}

func (fs *Host) clearRunState() {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	fs.runState = nil
}

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *Host) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return fs.requireCoordinator().SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *Host) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return fs.requireCoordinator().SubscribeFactoryEvents(ctx, reconnect, scope)
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *Host) WaitToComplete() <-chan struct{} {
	return factoryservice.WaitToComplete(fs.currentRuntimeBundle())
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *Host) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return fs.requireCoordinator().GetEngineStateSnapshot(ctx)
}

// Pause pauses the current runtime instance.
func (fs *Host) Pause(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// Resume resumes the current runtime instance and wakes buffered work.
func (fs *Host) Resume(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Resume(ctx); err != nil {
		return fmt.Errorf("resume factory: %w", err)
	}
	return nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *Host) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get factory events: %w", err)
	}
	mapped, err := apisurface.FactoryEventsToAPI(events)
	if err != nil {
		return nil, fmt.Errorf("map factory events to public contract: %w", err)
	}
	return mapped, nil
}

func (fs *Host) submitWorkFile(ctx context.Context) error {
	workFile := fs.coordinatorPolicy().workFile
	data, err := os.ReadFile(workFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", workFile, err)
	}
	workRequest, err := requests.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", workFile, err)
	}
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if _, err := activeFactory.SubmitWorkRequest(ctx, workRequest); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", workFile))
	return nil
}

func (fs *Host) currentFactory() factory.Factory {
	if bundle := fs.currentRuntimeBundle(); bundle != nil {
		return bundle.Factory
	}
	return nil
}

func (fs *Host) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if bundle := fs.currentRuntimeBundle(); bundle != nil {
		return bundle.RuntimeCfg
	}
	if session := fs.defaultSession(); session != nil {
		if spec := liveSessionBuildSpec(session); spec != nil {
			return spec.LoadedFactoryCfg
		}
	}
	return nil
}

// StartupWorkerConfig returns the named worker from the built startup runtime config.
func (fs *Host) StartupWorkerConfig(name string) (*workerconfig.Config, bool) {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

func (fs *Host) workflowID() string {
	if fs == nil {
		return ""
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.coordinatorPolicy().workflowID
}

func validateReplayModeConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("factory service config is required")
	}
	if cfg.RecordPath != "" && cfg.ReplayPath != "" {
		return fmt.Errorf("--record and --replay cannot be used together")
	}
	return nil
}

// ValidateReplayModeConfig validates record/replay startup inputs for core composition.
func ValidateReplayModeConfig(cfg *Config) error {
	return validateReplayModeConfig(cfg)
}

func runtimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}
