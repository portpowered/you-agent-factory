package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// NewDurableSessionID allocates one durable Factory Session identifier.
func NewDurableSessionID() string {
	return "dur-sess-" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

type runtimeSessionState struct {
	session    SessionReadResult
	result     ResultReadResult
	dispatches []DispatchSummary
	artifacts  []ArtifactSummary
	events     []json.RawMessage
	runCancel  context.CancelFunc
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
	state.events = deriveProjectionEvents(state.session, state.result)
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
	state.events = deriveProjectionEvents(state.session, state.result)
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
	ProjectRoot string
}

// JavaScriptRuntimeService executes simple JavaScript workflows through the real
// workflow runtime and projects outcomes through shared durable session read models.
type JavaScriptRuntimeService struct {
	projectRoot string

	mu           sync.RWMutex
	sessions     map[string]*runtimeSessionState
	startReplay  map[string]startReplayRecord
	startInflight map[string]*startInflightFlight
}

var _ Service = (*JavaScriptRuntimeService)(nil)

// NewJavaScriptRuntimeService constructs one JavaScript runtime-backed durable session service.
func NewJavaScriptRuntimeService(config JavaScriptRuntimeServiceConfig) *JavaScriptRuntimeService {
	return &JavaScriptRuntimeService{
		projectRoot:   strings.TrimSpace(config.ProjectRoot),
		sessions:      make(map[string]*runtimeSessionState),
		startReplay:   make(map[string]startReplayRecord),
		startInflight: make(map[string]*startInflightFlight),
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

	resolved, sourceContent, policyResolution, err := s.prepareStartExecution(normalized)
	if err != nil {
		return AsyncStartResult{}, err
	}

	reserved, err := s.reserveStartSession(ctx, normalized, tupleHash, true)
	if err != nil {
		return AsyncStartResult{}, err
	}
	defer reserved.release()

	if reserved.isNew {
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
	}

	snapshot, err := s.snapshotSessionState(reserved.state.session.SessionID)
	if err != nil {
		return AsyncStartResult{}, err
	}
	return s.asyncStartFromState(snapshot), nil
}

func (s *JavaScriptRuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	normalized, tupleHash, err := normalizeStartTuple(req)
	if err != nil {
		return SyncStartResult{}, err
	}

	resolved, sourceContent, policyResolution, err := s.prepareStartExecution(normalized)
	if err != nil {
		return SyncStartResult{}, err
	}

	waitTimeout, hasSyncWait := syncWaitTimeout(normalized)
	reserved, err := s.reserveStartSession(ctx, normalized, tupleHash, !hasSyncWait)
	if err != nil {
		return SyncStartResult{}, err
	}
	defer reserved.release()

	if snapshot, snapErr := s.snapshotSessionState(reserved.state.session.SessionID); snapErr == nil {
		if IsTerminalLifecycleStatus(snapshot.session.Status) {
			return s.syncStartFromState(snapshot), nil
		}
		if snapshot.result.Availability != nil && snapshot.result.Availability.Reason == "SYNC_WAIT_TIMED_OUT" {
			return s.syncStartFromState(snapshot), nil
		}
	}

	if hasSyncWait {
		if reserved.isNew {
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
		}
		return s.waitSyncCompletion(ctx, reserved.state.session.SessionID, waitTimeout, normalized.Wait.CancelOnTimeout)
	}

	if reserved.isNew {
		terminal, err := s.executeImmediateSyncSession(ctx, normalized, resolved, sourceContent, policyResolution, reserved.state.session.SessionID)
		if err != nil {
			return SyncStartResult{}, err
		}
		s.mu.Lock()
		applyRuntimeSessionFields(reserved.state, terminal)
		reserved.state.runCancel = nil
		s.mu.Unlock()
	}

	snapshot, err := s.snapshotSessionState(reserved.state.session.SessionID)
	if err != nil {
		return SyncStartResult{}, err
	}
	return s.syncStartFromState(snapshot), nil
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

func (s *JavaScriptRuntimeService) Pause(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) Resume(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Terminate(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) Approve(ctx context.Context, _ string, _ ApproveRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) RetryDispatch(ctx context.Context, _ string, _ RetryDispatchRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
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
			return DispatchDetail{
				DispatchSummary:  summary,
				SessionID:        id,
				OrchestratorKind: state.session.OrchestratorKind,
			}, nil
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

func (s *JavaScriptRuntimeService) prepareStartExecution(
	normalized StartRequest,
) (ResolvedSource, string, workflowpolicy.Resolution, error) {
	resolved, err := ResolveStartSource(normalized, StartSourceContext{ProjectRoot: s.projectRoot})
	if err != nil {
		return ResolvedSource{}, "", workflowpolicy.Resolution{}, err
	}
	sourceContent, err := LoadStartSourceContent(normalized, resolved, StartSourceContext{ProjectRoot: s.projectRoot})
	if err != nil {
		return ResolvedSource{}, "", workflowpolicy.Resolution{}, err
	}
	policyResolution := workflowpolicy.Resolve(workflowpolicy.Request{Requested: normalized.RequestedPolicy})
	return resolved, sourceContent, policyResolution, nil
}

type reservedStartSession struct {
	state   *runtimeSessionState
	isNew   bool
	release func()
}

func (s *JavaScriptRuntimeService) reserveStartSession(
	ctx context.Context,
	normalized StartRequest,
	tupleHash string,
	waitIfInflight bool,
) (*reservedStartSession, error) {
	for {
		s.mu.Lock()
		if replay, ok := s.startReplay[normalized.RequestID]; ok {
			if err := CheckRequestIDReplay(normalized.RequestID, replay.tupleHash, tupleHash); err != nil {
				s.mu.Unlock()
				return nil, err
			}
			state, ok := s.sessions[replay.sessionID]
			if !ok {
				s.mu.Unlock()
				return nil, ErrSessionNotFound
			}
			if waitIfInflight {
				if flight, ok := s.startInflight[normalized.RequestID]; ok &&
					!IsTerminalLifecycleStatus(state.session.Status) &&
					(state.result.Availability == nil || state.result.Availability.Reason != "SYNC_WAIT_TIMED_OUT") {
					done := flight.done
					s.mu.Unlock()
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-done:
						continue
					}
				}
			}
			s.mu.Unlock()
			return &reservedStartSession{state: state, isNew: false, release: func() {}}, nil
		}

		flight := &startInflightFlight{done: make(chan struct{})}
		sessionID := NewDurableSessionID()
		placeholder := &runtimeSessionState{
			session: SessionReadResult{SessionID: sessionID},
		}
		s.sessions[sessionID] = placeholder
		s.startReplay[normalized.RequestID] = startReplayRecord{
			sessionID: sessionID,
			tupleHash: tupleHash,
		}
		s.startInflight[normalized.RequestID] = flight
		s.mu.Unlock()

		release := func() {
			s.mu.Lock()
			delete(s.startInflight, normalized.RequestID)
			close(flight.done)
			s.mu.Unlock()
		}
		return &reservedStartSession{state: placeholder, isNew: true, release: release}, nil
	}
}

func syncWaitTimeout(normalized StartRequest) (time.Duration, bool) {
	if normalized.Wait == nil || normalized.Wait.TimeoutMillis == nil || *normalized.Wait.TimeoutMillis <= 0 {
		return 0, false
	}
	return time.Duration(*normalized.Wait.TimeoutMillis) * time.Millisecond, true
}

func (s *JavaScriptRuntimeService) waitSyncCompletion(
	ctx context.Context,
	sessionID string,
	waitTimeout time.Duration,
	cancelOnTimeout bool,
) (SyncStartResult, error) {
	deadline := time.Now().Add(waitTimeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return SyncStartResult{}, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return s.projectSyncWaitTimeout(sessionID, cancelOnTimeout)
			}

			snapshot, err := s.snapshotSessionState(sessionID)
			if err != nil {
				return SyncStartResult{}, err
			}
			if IsTerminalLifecycleStatus(snapshot.session.Status) {
				return s.syncStartFromState(snapshot), nil
			}
		}
	}
}

func (s *JavaScriptRuntimeService) projectSyncWaitTimeout(sessionID string, cancelOnTimeout bool) (SyncStartResult, error) {
	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return SyncStartResult{}, ErrSessionNotFound
	}

	if cancelOnTimeout && state.runCancel != nil {
		state.runCancel()
	}
	s.mu.Unlock()

	if cancelOnTimeout {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			snapshot, err := s.snapshotSessionState(sessionID)
			if err != nil {
				return SyncStartResult{}, err
			}
			if IsTerminalLifecycleStatus(snapshot.session.Status) {
				result := s.syncStartFromState(snapshot)
				result.SyncOutcome = SyncOutcomeTimedOut
				result.TimedOut = true
				result.SessionCanceledByTimeout = true
				return result, nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	s.mu.Lock()
	state, ok = s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return SyncStartResult{}, ErrSessionNotFound
	}
	state.result = ResultReadResult{
		SessionID:     sessionID,
		Mode:          ResultModeFinal,
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: LifecycleStatusRunning,
		Availability: &ResultAvailabilityDetail{
			Reason:    "SYNC_WAIT_TIMED_OUT",
			Message:   "Sync wait ended before a terminal result was available.",
			Retryable: true,
		},
	}
	if state.session.ResultSummary == nil {
		state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusNotReady)}
	} else {
		state.session.ResultSummary.ResultStatus = string(ResultStatusNotReady)
	}
	state.events = deriveProjectionEvents(state.session, state.result)
	snapshot := cloneRuntimeSessionState(state)
	s.mu.Unlock()

	result := s.syncStartFromState(snapshot)
	result.SyncOutcome = SyncOutcomeTimedOut
	result.TimedOut = true
	return result, nil
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
	}, workflowruntime.Hooks{})
}

func (s *JavaScriptRuntimeService) applyRuntimeLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	if _, err := NormalizeControlRequest(control); err != nil {
		return LifecycleControlResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[id]
	if !ok {
		return LifecycleControlResult{}, ErrSessionNotFound
	}

	currentStatus := state.session.Status
	outcome := EvaluateLifecycleControl(operation, currentStatus)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		return LifecycleControlResult{}, &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, currentStatus),
		}
	}

	if outcome == LifecycleControlOutcomeAccepted && operation == LifecycleControlCancel {
		state.session.Status = LifecycleStatusCanceling
		state.result.SessionStatus = LifecycleStatusCanceling
		if state.runCancel != nil {
			state.runCancel()
		}
		state.events = deriveProjectionEvents(state.session, state.result)
	}

	return runtimeLifecycleControlResultFromState(state, id, operation, outcome, retry), nil
}

func runtimeLifecycleControlResultFromState(
	state *runtimeSessionState,
	id string,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	retry RetryDispatchRequest,
) LifecycleControlResult {
	result := LifecycleControlResult{
		SessionID: id,
		Operation: operation,
		Outcome:   outcome,
		Status:    state.session.Status,
		Links:     LifecycleControlLinksForSession(id, true),
	}
	if operation == LifecycleControlRetryDispatch {
		result.DispatchID = retry.DispatchID
	}
	if outcome == LifecycleControlOutcomeAccepted || outcome == LifecycleControlOutcomeNoOp {
		session := cloneSessionRead(state.session)
		result.Session = &session
	}
	return result
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
		session:    cloneSessionRead(state.session),
		result:     cloneResultRead(state.result),
		dispatches: cloneDispatchSummaries(state.dispatches),
		artifacts:  cloneArtifactSummaries(state.artifacts),
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

func (s *JavaScriptRuntimeService) unsupportedLifecycleControl(ctx context.Context) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	return LifecycleControlResult{}, ErrUnsupportedControl
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
