package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	internalcontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewDurableSessionID allocates one durable Factory Session identifier.
func NewDurableSessionID(generateID internalcontracts.SessionIDGenerator) (string, error) {
	if generateID == nil {
		return "", errors.New("Factory Session ID generator is required")
	}
	identity := strings.ReplaceAll(strings.TrimSpace(generateID()), "-", "")
	if identity == "" {
		return "", errors.New("Factory Session ID generator returned an empty identity")
	}
	return "dur-sess-" + identity, nil
}

type runtimeSessionState struct {
	session                   SessionReadResult
	result                    ResultReadResult
	dispatches                []DispatchSummary
	dispatchJavaScript        map[string]DispatchJavaScriptProjection
	dispatchStatusTransitions map[string][]DispatchStatus
	artifacts                 []ArtifactSummary
	runtimeRecords            []factory.JavaScriptRuntimeRecord
	petriMutations            []interfaces.TokenMutationRecord
	checkpointSummary         *factory.JavaScriptCheckpointSummary
	startRequest              *StartRequest
	resolvedSource            ResolvedSource
	sourceContent             string
	events                    []json.RawMessage
	runCancel                 context.CancelFunc
	runDone                   chan struct{} // closed under mu after the async run and terminal persistence return
	eventConsumer             FactoryEventConsumer
	presentedEventIDs         map[string]struct{}
	responseEvents            *responseeventstore.SessionResponseEventStore
}
type startInflightFlight struct {
	done chan struct{}
}

var (
	// ErrDurableExecutionClosed reports a start raced with application
	// shutdown after the durable execution owner stopped accepting work.
	ErrDurableExecutionClosed = errors.New("durable execution service is closed")
	// ErrDurableExecutionShutdownTimeout keeps a non-cooperative workflow from
	// making shutdown appear complete while its owner is still running.
	ErrDurableExecutionShutdownTimeout = errors.New("durable execution shutdown timed out")
)

const durableExecutionShutdownTimeout = 10 * time.Second

func projectRuntimeSessionState(
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution factory.JavaScriptPolicyResolution,
	outcome factory.JavaScriptRuntimeOutcome,
	startedAt time.Time,
) runtimeSessionState {
	finishedAt := startedAt
	policyProjection := policyProjectionFromResolution(normalized, policyResolution)
	links := InspectionLinksForSession(sessionID, true)

	session := SessionReadResult{
		SessionID:        sessionID,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          resolvedDialect(resolved),
		ResolvedSource:   resolved,
		SourceHash:       resolved.SourceHash,
		Policy:           policyProjection,
		Usage:            EmptySessionUsage(),
		Links:            links,
		Lifecycle: &LifecycleTimestamps{
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		},
	}

	result := ResultReadResult{
		SessionID: sessionID,
		Mode:      ResultModeFinal,
	}

	state := runtimeSessionState{
		session: session,
		result:  result,
	}
	if outcome.OK {
		applyRuntimeSuccessProjection(&state, sessionID, outcome, finishedAt)
	} else {
		if len(outcome.Records) > 0 {
			applyRuntimeExecutionRecordProjection(&state, sessionID, outcome.Records, finishedAt)
		}
		projectRuntimeFailure(&state.session, &state.result, outcome)
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result, RuntimeDispatchEventInput{
		Dispatches:                state.dispatches,
		DispatchStatusTransitions: state.dispatchStatusTransitions,
		DispatchJavaScript:        state.dispatchJavaScript,
		Artifacts:                 state.artifacts,
	})
	return state
}

func projectRuntimeRunningSessionState(
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution factory.JavaScriptPolicyResolution,
	startedAt time.Time,
) runtimeSessionState {
	policyProjection := policyProjectionFromResolution(normalized, policyResolution)
	links := InspectionLinksForSession(sessionID, true)

	session := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          resolvedDialect(resolved),
		ResolvedSource:   resolved,
		SourceHash:       resolved.SourceHash,
		Policy:           policyProjection,
		Usage:            EmptySessionUsage(),
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusNotReady),
		},
		Lifecycle: &LifecycleTimestamps{
			StartedAt: &startedAt,
		},
		Links: links,
	}
	result := ResultReadResult{
		SessionID:     sessionID,
		Mode:          ResultModeFinal,
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: LifecycleStatusRunning,
		Availability: &ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
	state := runtimeSessionState{
		session: session,
		result:  result,
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result, RuntimeDispatchEventInput{
		Dispatches:                state.dispatches,
		DispatchStatusTransitions: state.dispatchStatusTransitions,
		DispatchJavaScript:        state.dispatchJavaScript,
		Artifacts:                 state.artifacts,
	})
	return state
}

func projectRuntimeFailure(session *SessionReadResult, result *ResultReadResult, outcome factory.JavaScriptRuntimeOutcome) {
	failure := outcome.Failure
	switch failure.Code {
	case factory.JavaScriptRuntimeCodeTimeout:
		session.Status = LifecycleStatusTimedOut
		result.SessionStatus = LifecycleStatusTimedOut
		result.ResultStatus = ResultStatusUnavailable
	default:
		if failure.Code == factory.JavaScriptRuntimeCodeCanceled {
			session.Status = LifecycleStatusCanceled
			result.SessionStatus = LifecycleStatusCanceled
		} else {
			session.Status = LifecycleStatusFailed
			result.SessionStatus = LifecycleStatusFailed
		}
		result.ResultStatus = ResultStatusUnavailable
	}
	if code := strings.TrimSpace(failure.Code); code != "" {
		session.Failure = &FailureSummary{
			Reason:  code,
			Message: failure.Message,
		}
		result.Failure = session.Failure
	}
	if session.ResultSummary == nil {
		session.ResultSummary = &ResultSummary{ResultStatus: string(result.ResultStatus)}
	}
}

func policyProjectionFromResolution(req StartRequest, resolution factory.JavaScriptPolicyResolution) PolicyProjection {
	projection := PolicyProjection{
		Requested:     cloneArgs(req.RequestedPolicy),
		EffectiveHash: resolution.Hash,
	}
	if effective, err := effectivePolicyMap(resolution.Policy); err == nil && len(effective) > 0 {
		projection.Effective = effective
	}
	return projection
}

func resolvedDialect(resolved ResolvedSource) string {
	if dialect := strings.TrimSpace(resolved.Dialect); dialect != "" {
		return dialect
	}
	return "you-workflow-v1"
}

// JavaScriptRuntimeService executes simple JavaScript workflows through the real
// workflow runtime and projects outcomes through shared durable session read models.
type JavaScriptRuntimeService struct {
	projectRoot       string
	childExecutorMode string
	// directChildInvocation remains only for legacy in-package construction
	// helpers and tests. Production standalone opening supplies the narrow
	// Workers Execute capability through directChildExecution; P6-C can remove
	// this compatibility input after those callers are retired.
	directChildInvocation workers.InvocationExecutor
	directChildExecution  childExecuteService
	persistence           runtimepersist.Store
	durableSnapshotBounds
	clock                   factory.Clock
	syncWaits               SyncWaitScheduler
	checkpointSummaries     factory.JavaScriptCheckpointSummaries
	workflowDefinitions     factory.JavaScriptWorkflowDefinitions
	orchestration           factory.OrchestrationJavaScriptExecution
	childValues             factory.JavaScriptChildValues
	workerPresetIDs         map[string]struct{}
	workerSettings          factory.JavaScriptWorkerSettings
	recordingWriter         recording.PortableRecordingWriter
	generateSessionID       internalcontracts.SessionIDGenerator
	generateResponseEventID factorysessions.ResponseEventIDGenerator
	responseStreams         responsestreamservice.Service
	liveChangeCoordinator   factorysessioncontracts.LiveChangeCoordinator
	// workerInvokerService is guarded by its own lock, not the session lock. It
	// is attached once after construction and read on paths that already hold the
	// session lock; sharing one mutex between them deadlocks.
	invokerMu            sync.RWMutex
	workerInvokerService factory.Service
	workerExecution      *childWorkerExecutionBinding
	// workerSessions maps one Workers dispatch identity to the durable session
	// that owns that Worker. A Worker's progress arrives from Workers, which
	// knows only the dispatch it belongs to, so this is what routes a child's
	// output back to the response-event store its own session reads. It has its
	// own lock for the same reason invokerMu does.
	workerSessionsMu sync.RWMutex
	workerSessions   map[string]string

	mu                         sync.RWMutex
	sessions                   map[string]*runtimeSessionState
	startReplay                map[string]startReplayRecord
	startInflight              map[string]*startInflightFlight
	controlReplay              map[string]controlReplayRecord
	liveChangeMu               sync.Mutex
	dispatchDurabilityMu       sync.RWMutex
	dispatchDurability         recording.CompletedFlushWatermarkReader
	dispatchStreamGenerationID string

	runLifecycleMu sync.Mutex
	runWaitGroup   sync.WaitGroup
	runClosed      bool
	closeOnce      sync.Once
	closeErr       error
}

// NewJavaScriptRuntimeService constructs the durable session service.
func NewJavaScriptRuntimeService(
	projectRoot string,
	childExecutorMode string,
	directChildInvocation workers.InvocationExecutor,
	persistence runtimepersist.Store,
	clock factory.Clock,
	syncWaits SyncWaitScheduler,
	checkpointSummaries factory.JavaScriptCheckpointSummaries,
	workflowDefinitions factory.JavaScriptWorkflowDefinitions,
	orchestration factory.OrchestrationJavaScriptExecution,
	childValues factory.JavaScriptChildValues,
	workerPresetIDs map[string]struct{},
	workerSettings factory.JavaScriptWorkerSettings,
	recordingWriter recording.PortableRecordingWriter,
	generateSessionID internalcontracts.SessionIDGenerator,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams responsestreamservice.Service,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) *JavaScriptRuntimeService {
	if generateSessionID == nil {
		return nil
	}
	projectRoot = strings.TrimSpace(projectRoot)
	service := &JavaScriptRuntimeService{
		projectRoot:             projectRoot,
		childExecutorMode:       normalizeChildExecutorMode(childExecutorMode),
		directChildInvocation:   directChildInvocation,
		clock:                   clock,
		syncWaits:               syncWaits,
		checkpointSummaries:     checkpointSummaries,
		workflowDefinitions:     workflowDefinitions,
		orchestration:           orchestration,
		childValues:             childValues,
		workerPresetIDs:         workerPresetIDs,
		workerSettings:          workerSettings,
		recordingWriter:         recordingWriter,
		generateSessionID:       generateSessionID,
		generateResponseEventID: generateResponseEventID,
		responseStreams:         responseStreams,
		liveChangeCoordinator:   liveChangeCoordinator,
		persistence:             persistence,
		sessions:                make(map[string]*runtimeSessionState),
		startReplay:             make(map[string]startReplayRecord),
		startInflight:           make(map[string]*startInflightFlight),
		controlReplay:           make(map[string]controlReplayRecord),
	}
	return service
}

// PersistenceStore returns the graph-owned durable snapshot collaborator.
// It is nil when persistence was explicitly disabled.
func (s *JavaScriptRuntimeService) PersistenceStore() runtimepersist.Store {
	if s == nil {
		return nil
	}
	return s.persistence
}

func (s *JavaScriptRuntimeService) now() time.Time { return s.clock.Now().UTC() }

func (s *JavaScriptRuntimeService) StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	if err := s.ensureOpen(); err != nil {
		return AsyncStartResult{}, err
	}
	normalized, tupleHash, err := normalizeStartTuple(req)
	if err != nil {
		return AsyncStartResult{}, err
	}

	if result, ok, err := s.tryReplayAsyncStart(ctx, normalized.RequestID, tupleHash, true); ok {
		return result, nil
	} else if err != nil {
		return AsyncStartResult{}, err
	}

	prepared, err := s.prepareStart(normalized)
	if err != nil {
		return AsyncStartResult{}, err
	}
	if err := validateChildExecutorMode(resolveChildExecutorMode(s.childExecutorMode, normalized)); err != nil {
		return AsyncStartResult{}, err
	}
	resolved := prepared.ResolvedSource
	sourceContent := prepared.SourceContent
	policyResolution := policyResolutionFromPrepared(prepared)

	reserved, err := s.reserveStartSession(ctx, normalized, tupleHash, true)
	if err != nil {
		return AsyncStartResult{}, err
	}
	defer reserved.release()

	if !reserved.isNew {
		result, ok, err := s.tryReplayAsyncStart(ctx, normalized.RequestID, tupleHash, true)
		if ok {
			return result, nil
		}
		return AsyncStartResult{}, err
	}
	startedAt := s.now()
	running := projectRuntimeRunningSessionState(
		reserved.state.session.SessionID,
		normalized,
		resolved,
		policyResolution,
		startedAt,
	)
	runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
	runDone := make(chan struct{})
	s.mu.Lock()
	reserved.state.session = running.session
	reserved.state.result = running.result
	reserved.state.events = running.events
	reserved.state.runCancel = runCancel
	reserved.state.runDone = runDone
	reserved.state.startRequest = cloneStartRequest(normalized)
	reserved.state.resolvedSource = resolved
	reserved.state.sourceContent = sourceContent
	s.mu.Unlock()
	if err := s.ensureSessionResponseEventsIfNeeded(reserved.state); err != nil {
		s.mu.Lock()
		reserved.state.runCancel = nil
		close(runDone)
		s.mu.Unlock()
		return AsyncStartResult{}, err
	}
	s.mu.RLock()
	startState := cloneRuntimeSessionState(reserved.state)
	s.mu.RUnlock()
	if err := s.launchAsyncRun(func() {
		s.runAsyncSession(runCtx, reserved.state.session.SessionID, normalized, resolved, sourceContent, policyResolution, startedAt, runDone)
	}); err != nil {
		runCancel()
		s.mu.Lock()
		reserved.state.runCancel = nil
		close(runDone)
		s.mu.Unlock()
		return AsyncStartResult{}, err
	}

	result := s.asyncStartFromState(startState)
	s.recordAsyncStartReplay(normalized.RequestID, result)
	return result, nil
}

func (s *JavaScriptRuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	return s.startSync(ctx, req)
}

func (s *JavaScriptRuntimeService) startSync(
	ctx context.Context,
	req StartRequest,
) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	if err := s.ensureOpen(); err != nil {
		return SyncStartResult{}, err
	}
	normalized, tupleHash, err := normalizeStartTuple(req)
	if err != nil {
		return SyncStartResult{}, err
	}

	if result, ok, err := s.tryReplaySyncStart(ctx, normalized.RequestID, tupleHash, true); ok {
		return result, nil
	} else if err != nil {
		return SyncStartResult{}, err
	}

	prepared, err := s.prepareStart(normalized)
	if err != nil {
		return SyncStartResult{}, err
	}
	if err := validateChildExecutorMode(resolveChildExecutorMode(s.childExecutorMode, normalized)); err != nil {
		return SyncStartResult{}, err
	}
	resolved := prepared.ResolvedSource
	sourceContent := prepared.SourceContent
	policyResolution := policyResolutionFromPrepared(prepared)

	waitTimeout, hasSyncWait := syncWaitTimeout(normalized)
	reserved, err := s.reserveStartSession(ctx, normalized, tupleHash, !hasSyncWait)
	if err != nil {
		return SyncStartResult{}, err
	}
	defer reserved.release()

	if !reserved.isNew {
		result, ok, err := s.tryReplaySyncStart(ctx, normalized.RequestID, tupleHash, true)
		if ok {
			return result, nil
		}
		return SyncStartResult{}, err
	}
	stopObservingFactoryEvents := s.observeFactoryEvents(reserved.state, normalized.EventConsumer)
	defer stopObservingFactoryEvents()
	if err := s.ensureSessionResponseEventsIfNeeded(reserved.state); err != nil {
		return SyncStartResult{}, err
	}

	if hasSyncWait {
		return s.startWaitingSyncSession(
			ctx, reserved, normalized, resolved, sourceContent, policyResolution, waitTimeout,
		)
	}
	return s.completeImmediateSyncStart(
		ctx, normalized, resolved, sourceContent, policyResolution, reserved,
	)
}

func (s *JavaScriptRuntimeService) GetSession(ctx context.Context, sessionID string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return SessionReadResult{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return SessionReadResult{}, err
	}
	return state.session, nil
}

func (s *JavaScriptRuntimeService) GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error) {
	if err := ctx.Err(); err != nil {
		return ResultReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ResultReadResult{}, err
	}
	normalized, err := NormalizeResultRequest(req)
	if err != nil {
		return ResultReadResult{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return ResultReadResult{}, err
	}
	return ProjectResultRead(state.result, state.session, state.artifacts, normalized)
}

func (s *JavaScriptRuntimeService) ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDispatchesResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	return ListDispatchesResult{
		SessionID:  id,
		Dispatches: s.dispatchesForRead(state.dispatches, state.events),
	}, nil
}

func (s *JavaScriptRuntimeService) QueryDispatches(ctx context.Context, request DispatchQueryRequest) (ListDispatchesResult, error) {
	return queryDispatches(ctx, s, request)
}

func (s *JavaScriptRuntimeService) GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error) {
	if err := ctx.Err(); err != nil {
		return DispatchDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return DispatchDetail{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return DispatchDetail{}, err
	}
	for _, summary := range s.dispatchesForRead(state.dispatches, state.events) {
		if summary.ID == dispatchID {
			detail := DispatchDetail{
				DispatchSummary:  summary,
				SessionID:        id,
				OrchestratorKind: state.session.OrchestratorKind,
			}
			if len(summary.OutputArtifactIDs) > 0 {
				detail.ArtifactIDs = append([]string(nil), summary.OutputArtifactIDs...)
			}
			if transitions := state.dispatchStatusTransitions[dispatchID]; len(transitions) > 0 {
				detail.StatusTransitions = append([]DispatchStatus(nil), transitions...)
			}
			if js, ok := state.dispatchJavaScript[dispatchID]; ok {
				projection := js
				detail.JavaScript = &projection
			} else if summary.JavaScript != nil {
				projection := *summary.JavaScript
				detail.JavaScript = &projection
			}
			return detail, nil
		}
	}
	return DispatchDetail{}, ErrDispatchNotFound
}

func (s *JavaScriptRuntimeService) ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListArtifactsResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	return ListArtifactsResult{
		SessionID: id,
		Artifacts: cloneArtifactSummaries(state.artifacts),
	}, nil
}

func (s *JavaScriptRuntimeService) GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ArtifactDetail{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return ArtifactDetail{}, err
	}
	for _, summary := range state.artifacts {
		if summary.ID == artifactID {
			return ArtifactDetail{ArtifactSummary: summary, SessionID: id}, nil
		}
	}
	return ArtifactDetail{}, ErrArtifactNotFound
}

func (s *JavaScriptRuntimeService) ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error) {
	if err := ctx.Err(); err != nil {
		return EventReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return EventReadResult{}, err
	}
	if _, err := NormalizeEventReconnectRequest(req); err != nil {
		return EventReadResult{}, err
	}
	state, err := s.snapshotSessionState(id)
	if err != nil {
		return EventReadResult{}, err
	}
	filtered, err := FilterEventsAfterReconnect(state.events, req, id)
	if err != nil {
		return EventReadResult{}, err
	}
	return EventReadResult{SessionID: id, Events: filtered}, nil
}

func (s *JavaScriptRuntimeService) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListSessionsResult{}, err
	}
	normalized, err := NormalizeListSessionsRequest(req)
	if err != nil {
		return ListSessionsResult{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	live := make([]LiveSessionSummary, 0, len(s.sessions))
	durable := make([]DurableSessionListSummary, 0, len(s.sessions))
	for _, state := range s.sessions {
		read := cloneSessionRead(state.session)
		live = append(live, LiveListSummaryFromSessionRead(read))
		summary := DurableListSummaryFromSessionRead(read)
		if IsPersistedListCandidate(summary) {
			durable = append(durable, summary)
		}
	}
	return ApplySessionListScope(ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    live,
		DurableSessions: durable,
	}, normalized), nil
}

func (s *JavaScriptRuntimeService) executeImmediateSyncSession(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution factory.JavaScriptPolicyResolution,
	sessionID string,
) (runtimeSessionState, error) {
	runCtx, cancel := workflowRunContext(ctx, policyResolution.Policy)
	defer cancel()

	startedAt := s.now()
	outcome, err := s.invokeWorkflowRuntime(runCtx, normalized, resolved, sourceContent, policyResolution, sessionID)
	if err != nil {
		return runtimeSessionState{}, err
	}

	return projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt), nil
}

func (s *JavaScriptRuntimeService) prepareStart(normalized StartRequest) (PreparedStart, error) {
	return PrepareStart(normalized, StartPrepareContext{
		StartSourceContext: StartSourceContext{ProjectRoot: s.projectRoot},
		WorkerPresetIDs:    s.workerPresetIDs,
	}, s.workflowDefinitions)
}

func policyResolutionFromPrepared(prepared PreparedStart) factory.JavaScriptPolicyResolution {
	return factory.JavaScriptPolicyResolution{
		Policy: prepared.EffectivePolicy,
		Hash:   prepared.Policy.EffectiveHash,
	}
}

func (s *JavaScriptRuntimeService) runAsyncSession(
	runCtx context.Context,
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution factory.JavaScriptPolicyResolution,
	startedAt time.Time,
	runDone chan struct{},
) {
	defer func() {
		s.mu.Lock()
		if state, ok := s.sessions[sessionID]; ok {
			state.runCancel = nil
		}
		close(runDone)
		s.mu.Unlock()
	}()

	outcome, err := s.invokeWorkflowRuntime(runCtx, normalized, resolved, sourceContent, policyResolution, sessionID)

	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if len(state.runtimeRecords) > 0 {
		if len(outcome.Records) == 0 {
			outcome.Records = cloneRuntimeRecords(state.runtimeRecords)
		} else if len(outcome.Records) < len(state.runtimeRecords) {
			outcome.Records = mergeRuntimeRecords(state.runtimeRecords, outcome.Records)
		}
	}
	if err != nil {
		failureOutcome := factory.JavaScriptRuntimeOutcome{
			OK: false,
			Failure: factory.JavaScriptRuntimeFailure{
				Code:    factory.JavaScriptRuntimeCodeScriptError,
				Message: err.Error(),
			},
		}
		terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, failureOutcome, startedAt)
		candidate := s.buildTerminalRuntimeCandidate(state, terminal, failureOutcome, startedAt)
		s.publishAsyncTerminalCandidate(state, candidate, normalized, resolved, policyResolution, startedAt)
		return
	}

	terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt)
	candidate := s.buildTerminalRuntimeCandidate(state, terminal, outcome, startedAt)
	s.publishAsyncTerminalCandidate(state, candidate, normalized, resolved, policyResolution, startedAt)
}

func (s *JavaScriptRuntimeService) invokeWorkflowRuntime(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution factory.JavaScriptPolicyResolution,
	sessionID string,
) (factory.JavaScriptRuntimeOutcome, error) {
	argsJSON, err := marshalStartArgs(normalized.Args)
	if err != nil {
		return factory.JavaScriptRuntimeOutcome{}, err
	}
	return s.orchestration.RunJavaScript(ctx, factory.JavaScriptRuntimeRequest{
		Source:         sourceContent,
		SourceRef:      resolved.SourceRef,
		SessionID:      sessionID,
		Args:           argsJSON,
		ArgsSchema:     resolved.ArgsSchema,
		Metadata:       workflowMetadataFromResolved(resolved, normalized),
		FactoryName:    factoryNameFromStart(normalized, resolved),
		Policy:         policyResolution.Policy,
		Agents:         resolved.Agents,
		WorkerSettings: s.workerSettings,
	}, s.childExecutorHooks(resolveChildExecutorMode(s.childExecutorMode, normalized), sessionID))
}

func workflowRunContext(parent context.Context, policy factory.JavaScriptPolicy) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if policy.MaxRunDurationMs == nil || *policy.MaxRunDurationMs <= 0 {
		return ctx, cancel
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Duration(*policy.MaxRunDurationMs)*time.Millisecond)
	return timeoutCtx, func() {
		timeoutCancel()
		cancel()
	}
}

func (s *JavaScriptRuntimeService) snapshotSessionState(sessionID string) (runtimeSessionState, error) {
	s.mu.RLock()
	state, ok := s.sessions[sessionID]
	if ok {
		cloned := cloneRuntimeSessionState(state)
		s.mu.RUnlock()
		return cloned, nil
	}
	persistence := s.persistence
	s.mu.RUnlock()

	if persistence == nil {
		return runtimeSessionState{}, ErrSessionNotFound
	}

	snapshot, err := persistence.Load(sessionID)
	if err != nil {
		return runtimeSessionState{}, ErrSessionNotFound
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(snapshot, &persisted); err != nil {
		return runtimeSessionState{}, ErrSessionNotFound
	}
	loaded := runtimeStateFromPersistedSnapshot(persisted)

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok {
		return cloneRuntimeSessionState(existing), nil
	}
	cached := loaded
	s.sessions[sessionID] = &cached
	return cloneRuntimeSessionState(&cached), nil
}

// RecordPetriTokenMutations appends applied Petri transition records through
// the canonical Factory Session persistence owner. Persistence succeeds before
// the updated history becomes visible to live readers.
func (s *JavaScriptRuntimeService) RecordPetriTokenMutations(
	sessionID string,
	mutations []interfaces.TokenMutationRecord,
) error {
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	if len(mutations) == 0 {
		return nil
	}
	if _, err := s.snapshotSessionState(id); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[id]
	if !ok {
		initial := projectPetriRunningSessionState(id, s.now())
		state = &initial
	}
	candidate := cloneRuntimeSessionState(state)
	candidate.petriMutations = append(candidate.petriMutations, clonePetriMutations(mutations)...)
	if err := s.persistSessionSnapshot(candidate); err != nil {
		return err
	}
	if ok {
		*state = candidate
	} else {
		s.sessions[id] = &candidate
	}
	return nil
}

func cloneRuntimeSessionState(state *runtimeSessionState) runtimeSessionState {
	if state == nil {
		return runtimeSessionState{}
	}
	cloned := runtimeSessionState{
		session:                   cloneSessionRead(state.session),
		result:                    cloneResultRead(state.result),
		dispatches:                cloneDispatchSummaries(state.dispatches),
		dispatchJavaScript:        cloneDispatchJavaScriptProjections(state.dispatchJavaScript),
		dispatchStatusTransitions: cloneDispatchStatusTransitions(state.dispatchStatusTransitions),
		artifacts:                 cloneArtifactSummaries(state.artifacts),
		runtimeRecords:            cloneRuntimeRecords(state.runtimeRecords),
		petriMutations:            clonePetriMutations(state.petriMutations),
		checkpointSummary:         cloneCheckpointSummary(state.checkpointSummary),
		startRequest:              cloneStartRequestPtr(state.startRequest),
		resolvedSource:            state.resolvedSource,
		sourceContent:             state.sourceContent,
		runDone:                   state.runDone,
		responseEvents:            state.responseEvents,
	}
	if len(state.events) > 0 {
		cloned.events = make([]json.RawMessage, len(state.events))
		for i, event := range state.events {
			cloned.events[i] = append(json.RawMessage(nil), event...)
		}
	}
	return cloned
}

func (s *JavaScriptRuntimeService) syncStartFromState(state runtimeSessionState) SyncStartResult {
	async := s.asyncStartFromState(state)
	result := SyncStartResult{AsyncStartResult: async}
	if state.result.Availability != nil && state.result.Availability.Reason == "SYNC_WAIT_TIMED_OUT" {
		result.SyncOutcome = SyncOutcomeTimedOut
		result.TimedOut = true
		return result
	}
	if IsTerminalLifecycleStatus(state.session.Status) {
		result.SyncOutcome = SyncOutcomeCompleted
		projected, err := ProjectResultRead(state.result, state.session, state.artifacts, ResultRequest{
			Mode: ResultModeFinal,
		})
		if err == nil {
			if encoded, err := json.Marshal(projected); err == nil {
				result.Result = encoded
			}
		}
	}
	return result
}

func (s *JavaScriptRuntimeService) asyncStartFromState(state runtimeSessionState) AsyncStartResult {
	return AsyncStartResult{
		SessionID:        state.session.SessionID,
		Status:           string(state.session.Status),
		OrchestratorKind: state.session.OrchestratorKind,
		Dialect:          state.session.Dialect,
		ResolvedSource:   state.session.ResolvedSource,
		SourceHash:       state.session.SourceHash,
		Policy:           state.session.Policy,
		Links:            state.session.Links,
	}
}

func defaultUnavailableAvailability() *ResultAvailabilityDetail {
	return &ResultAvailabilityDetail{
		Reason:    "UNAVAILABLE",
		Message:   "session result is unavailable",
		Retryable: false,
	}
}

func marshalStartArgs(args map[string]any) (json.RawMessage, error) {
	if len(args) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, NewValidationError("args", "args must be JSON-compatible")
	}
	return encoded, nil
}

func workflowMetadataFromResolved(resolved ResolvedSource, req StartRequest) map[string]string {
	metadata := map[string]string{}
	if req.Source.InlineWorkflow != nil {
		for key, value := range req.Source.InlineWorkflow.Metadata {
			metadata[key] = value
		}
	}
	for key, value := range resolved.Metadata {
		metadata[key] = value
	}
	if name := strings.TrimSpace(req.Source.WorkflowName); name != "" {
		metadata["name"] = name
	} else if _, ok := metadata["name"]; !ok {
		base := strings.TrimSpace(resolved.SourceRef)
		if base != "" {
			metadata["name"] = base
		}
	}
	return metadata
}
