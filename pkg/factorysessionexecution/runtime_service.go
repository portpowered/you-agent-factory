package factorysessionexecution

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// NewDurableSessionID allocates one durable Factory Session identifier.
func NewDurableSessionID() string {
	return "dur-sess-" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

type runtimeSessionState struct {
	session            SessionReadResult
	result             ResultReadResult
	dispatches         []DispatchSummary
	dispatchJavaScript map[string]DispatchJavaScriptProjection
	artifacts          []ArtifactSummary
	events             []json.RawMessage
	runCancel          context.CancelFunc
}

type startInflightFlight struct {
	done chan struct{}
}

func projectRuntimeSessionState(
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution workflowpolicy.Resolution,
	outcome workflowruntime.Outcome,
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
		projectRuntimeFailure(&state.session, &state.result, outcome)
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	return state
}

func projectRuntimeRunningSessionState(
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	policyResolution workflowpolicy.Resolution,
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
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	return state
}

func projectRuntimeFailure(session *SessionReadResult, result *ResultReadResult, outcome workflowruntime.Outcome) {
	failure := outcome.Failure
	switch failure.Code {
	case workflowruntime.CodeTimeout:
		session.Status = LifecycleStatusTimedOut
		result.SessionStatus = LifecycleStatusTimedOut
		result.ResultStatus = ResultStatusUnavailable
	default:
		if failure.Code == workflowruntime.CodeCanceled {
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

func policyProjectionFromResolution(req StartRequest, resolution workflowpolicy.Resolution) PolicyProjection {
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

// JavaScriptRuntimeServiceConfig carries dependencies for the real JavaScript runtime
// durable session execution path.
type JavaScriptRuntimeServiceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	Provider          workers.Provider
}

// JavaScriptRuntimeService executes simple JavaScript workflows through the real
// workflow runtime and projects outcomes through shared durable session read models.
type JavaScriptRuntimeService struct {
	projectRoot       string
	childExecutorMode string
	provider          workers.Provider

	mu            sync.RWMutex
	sessions      map[string]*runtimeSessionState
	startReplay   map[string]startReplayRecord
	startInflight map[string]*startInflightFlight
	controlReplay map[string]controlReplayRecord
}

var _ Service = (*JavaScriptRuntimeService)(nil)

// NewJavaScriptRuntimeService constructs one JavaScript runtime-backed durable session service.
func NewJavaScriptRuntimeService(config JavaScriptRuntimeServiceConfig) *JavaScriptRuntimeService {
	return &JavaScriptRuntimeService{
		projectRoot:       strings.TrimSpace(config.ProjectRoot),
		childExecutorMode: normalizeChildExecutorMode(config.ChildExecutorMode),
		provider:          config.Provider,
		sessions:          make(map[string]*runtimeSessionState),
		startReplay:   make(map[string]startReplayRecord),
		startInflight: make(map[string]*startInflightFlight),
		controlReplay: make(map[string]controlReplayRecord),
	}
}

func (s *JavaScriptRuntimeService) StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
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
	if err := validateLiveChildExecutorConfig(resolveChildExecutorMode(s.childExecutorMode, normalized), s.provider); err != nil {
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

	startedAt := time.Now().UTC()
	running := projectRuntimeRunningSessionState(
		reserved.state.session.SessionID,
		normalized,
		resolved,
		policyResolution,
		startedAt,
	)
	runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
	s.mu.Lock()
	reserved.state.session = running.session
	reserved.state.result = running.result
	reserved.state.events = running.events
	reserved.state.runCancel = runCancel
	s.mu.Unlock()

	go s.runAsyncSession(runCtx, reserved.state.session.SessionID, normalized, resolved, sourceContent, policyResolution, startedAt)

	snapshot, err := s.snapshotSessionState(reserved.state.session.SessionID)
	if err != nil {
		return AsyncStartResult{}, err
	}
	result := s.asyncStartFromState(snapshot)
	s.recordAsyncStartReplay(normalized.RequestID, result)
	return result, nil
}

func (s *JavaScriptRuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
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
	if err := validateLiveChildExecutorConfig(resolveChildExecutorMode(s.childExecutorMode, normalized), s.provider); err != nil {
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

	if hasSyncWait {
		startedAt := time.Now().UTC()
		running := projectRuntimeRunningSessionState(
			reserved.state.session.SessionID,
			normalized,
			resolved,
			policyResolution,
			startedAt,
		)
		runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
		s.mu.Lock()
		reserved.state.session = running.session
		reserved.state.result = running.result
		reserved.state.events = running.events
		reserved.state.runCancel = runCancel
		s.mu.Unlock()

		go s.runAsyncSession(runCtx, reserved.state.session.SessionID, normalized, resolved, sourceContent, policyResolution, startedAt)

		result, err := s.waitSyncCompletion(ctx, reserved.state.session.SessionID, waitTimeout, normalized.Wait.CancelOnTimeout)
		if err != nil {
			return SyncStartResult{}, err
		}
		s.recordSyncStartReplay(normalized.RequestID, result)
		return result, nil
	}

	terminal, err := s.executeImmediateSyncSession(ctx, normalized, resolved, sourceContent, policyResolution, reserved.state.session.SessionID)
	if err != nil {
		return SyncStartResult{}, err
	}
	s.mu.Lock()
	applyRuntimeSessionFields(reserved.state, terminal)
	reserved.state.runCancel = nil
	s.mu.Unlock()

	snapshot, err := s.snapshotSessionState(reserved.state.session.SessionID)
	if err != nil {
		return SyncStartResult{}, err
	}
	result := s.syncStartFromState(snapshot)
	s.recordSyncStartReplay(normalized.RequestID, result)
	return result, nil
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
		Dispatches: cloneDispatchSummaries(state.dispatches),
	}, nil
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
	for _, summary := range state.dispatches {
		if summary.ID == dispatchID {
			detail := DispatchDetail{
				DispatchSummary:  summary,
				SessionID:        id,
				OrchestratorKind: state.session.OrchestratorKind,
			}
			if js, ok := state.dispatchJavaScript[dispatchID]; ok {
				projection := js
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
	policyResolution workflowpolicy.Resolution,
	sessionID string,
) (runtimeSessionState, error) {
	runCtx, cancel := workflowRunContext(ctx, policyResolution.Policy)
	defer cancel()

	startedAt := time.Now().UTC()
	outcome, err := s.invokeWorkflowRuntime(runCtx, normalized, resolved, sourceContent, policyResolution, sessionID)
	if err != nil {
		return runtimeSessionState{}, err
	}

	return projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt), nil
}

func normalizeStartTuple(req StartRequest) (StartRequest, string, error) {
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return StartRequest{}, "", err
	}
	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return StartRequest{}, "", err
	}
	return normalized, tupleHash, nil
}

func (s *JavaScriptRuntimeService) prepareStart(normalized StartRequest) (PreparedStart, error) {
	return PrepareStart(normalized, StartPrepareContext{
		StartSourceContext: StartSourceContext{ProjectRoot: s.projectRoot},
	})
}

func policyResolutionFromPrepared(prepared PreparedStart) workflowpolicy.Resolution {
	return workflowpolicy.Resolution{
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
	policyResolution workflowpolicy.Resolution,
	startedAt time.Time,
) {
	defer func() {
		s.mu.Lock()
		if state, ok := s.sessions[sessionID]; ok {
			state.runCancel = nil
		}
		s.mu.Unlock()
	}()

	outcome, err := s.invokeWorkflowRuntime(runCtx, normalized, resolved, sourceContent, policyResolution, sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	if err != nil {
		failureOutcome := workflowruntime.Outcome{
			OK: false,
			Failure: workflowruntime.Failure{
				Code:    workflowruntime.CodeScriptError,
				Message: err.Error(),
			},
		}
		terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, failureOutcome, startedAt)
		s.applyTerminalRuntimeState(state, terminal, startedAt)
		return
	}

	terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt)
	s.applyTerminalRuntimeState(state, terminal, startedAt)
}

func (s *JavaScriptRuntimeService) applyTerminalRuntimeState(state *runtimeSessionState, terminal runtimeSessionState, startedAt time.Time) {
	finishedAt := time.Now().UTC()
	applyRuntimeSessionFields(state, terminal)
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if state.session.Lifecycle.StartedAt == nil {
		state.session.Lifecycle.StartedAt = &startedAt
	}
	state.session.Lifecycle.FinishedAt = &finishedAt
	state.result.SessionStatus = state.session.Status
}

func (s *JavaScriptRuntimeService) invokeWorkflowRuntime(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowpolicy.Resolution,
	sessionID string,
) (workflowruntime.Outcome, error) {
	argsJSON, err := marshalStartArgs(normalized.Args)
	if err != nil {
		return workflowruntime.Outcome{}, err
	}
	return workflowruntime.Run(ctx, workflowruntime.Request{
		Source:    sourceContent,
		SourceRef: resolved.SourceRef,
		SessionID: sessionID,
		Args:      argsJSON,
		Metadata:  workflowMetadataFromResolved(resolved, normalized),
		Policy:    policyResolution.Policy,
	}, s.childExecutorHooks(resolveChildExecutorMode(s.childExecutorMode, normalized)))
}

func workflowRunContext(parent context.Context, policy workflowpolicy.EffectivePolicy) (context.Context, context.CancelFunc) {
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
	defer s.mu.RUnlock()
	state, ok := s.sessions[sessionID]
	if !ok {
		return runtimeSessionState{}, ErrSessionNotFound
	}
	return cloneRuntimeSessionState(state), nil
}

func cloneRuntimeSessionState(state *runtimeSessionState) runtimeSessionState {
	if state == nil {
		return runtimeSessionState{}
	}
	cloned := runtimeSessionState{
		session:            cloneSessionRead(state.session),
		result:             cloneResultRead(state.result),
		dispatches:         cloneDispatchSummaries(state.dispatches),
		dispatchJavaScript: cloneDispatchJavaScriptProjections(state.dispatchJavaScript),
		artifacts:          cloneArtifactSummaries(state.artifacts),
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
