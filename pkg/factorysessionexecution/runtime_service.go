package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

// RuntimeService executes simple JavaScript workflows through the real runtime
// boundary and projects in-memory durable session state.
type RuntimeService struct {
	mu sync.RWMutex

	prepareCtx StartPrepareContext
	now        func() time.Time

	sessions    map[string]*runtimeSessionState
	startReplay map[string]runtimeStartReplayRecord
}

type runtimeStartReplayRecord struct {
	sessionID     string
	tupleHash     string
	asyncStart    *AsyncStartResult
	syncStart     *SyncStartResult
	syncStartDone chan struct{}
}

type runtimeSessionState struct {
	session       SessionReadResult
	result        ResultReadResult
	dispatches    []DispatchSummary
	artifacts     []ArtifactSummary
	events        []json.RawMessage
	runtimeDone chan struct{}
}

// RuntimeServiceOption configures one RuntimeService instance.
type RuntimeServiceOption func(*RuntimeService)

// WithRuntimeClock overrides the clock used for lifecycle timestamps and events.
func WithRuntimeClock(now func() time.Time) RuntimeServiceOption {
	return func(service *RuntimeService) {
		service.now = now
	}
}

// NewRuntimeService constructs one in-memory runtime-backed durable session service.
func NewRuntimeService(prepareCtx StartPrepareContext, options ...RuntimeServiceOption) *RuntimeService {
	service := &RuntimeService{
		prepareCtx:  prepareCtx,
		now:         time.Now,
		sessions:    make(map[string]*runtimeSessionState),
		startReplay: make(map[string]runtimeStartReplayRecord),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

var _ Service = (*RuntimeService)(nil)

func (s *RuntimeService) StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	prepared, err := PrepareStart(req, s.prepareCtx)
	if err != nil {
		return AsyncStartResult{}, err
	}

	s.mu.Lock()
	if replay, ok := s.startReplay[prepared.Request.RequestID]; ok {
		if err := CheckRequestIDReplay(prepared.Request.RequestID, replay.tupleHash, prepared.TupleHash); err != nil {
			s.mu.Unlock()
			return AsyncStartResult{}, err
		}
		if err := CheckAsyncStartReplayMode(replay.asyncStart); err != nil {
			s.mu.Unlock()
			return AsyncStartResult{}, err
		}
		result := cloneAsyncStartResult(*replay.asyncStart)
		s.mu.Unlock()
		return result, nil
	}

	state := s.newRunningSessionState(prepared)
	s.sessions[state.session.SessionID] = state
	asyncResult := s.asyncStartFromState(state)
	record := runtimeStartReplayRecord{
		sessionID:  state.session.SessionID,
		tupleHash:  prepared.TupleHash,
		asyncStart: cloneAsyncStartResultPtr(asyncResult),
	}
	s.startReplay[prepared.Request.RequestID] = record
	s.mu.Unlock()

	go s.runSession(context.Background(), prepared, state.session.SessionID)
	return asyncResult, nil
}

func (s *RuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	prepared, err := PrepareStart(req, s.prepareCtx)
	if err != nil {
		return SyncStartResult{}, err
	}

	s.mu.Lock()
	if replayResult, waitDone, replayErr := s.tryReplaySyncStartLocked(prepared); replayErr != nil {
		s.mu.Unlock()
		return SyncStartResult{}, replayErr
	} else if replayResult != nil {
		s.mu.Unlock()
		return *replayResult, nil
	} else if waitDone != nil {
		s.mu.Unlock()
		select {
		case <-waitDone:
		case <-ctx.Done():
			return SyncStartResult{}, ctx.Err()
		}
		return s.replayStoredSyncStart(prepared.Request.RequestID)
	}

	state := s.newRunningSessionState(prepared)
	sessionID := state.session.SessionID
	runtimeDone := state.runtimeDone
	s.sessions[sessionID] = state
	s.startReplay[prepared.Request.RequestID] = runtimeStartReplayRecord{
		sessionID:     sessionID,
		tupleHash:     prepared.TupleHash,
		syncStartDone: make(chan struct{}),
	}
	s.mu.Unlock()

	stopRuntime := s.launchSyncRuntimeSession(prepared, sessionID)
	result, err := s.awaitSyncStartOutcome(ctx, prepared, sessionID, runtimeDone, stopRuntime)
	if err != nil {
		s.finalizeSyncStartReplay(prepared.Request.RequestID, nil)
		return SyncStartResult{}, err
	}
	return result, nil
}

func (s *RuntimeService) tryReplaySyncStartLocked(prepared PreparedStart) (*SyncStartResult, <-chan struct{}, error) {
	replay, ok := s.startReplay[prepared.Request.RequestID]
	if !ok {
		return nil, nil, nil
	}
	if err := CheckRequestIDReplay(prepared.Request.RequestID, replay.tupleHash, prepared.TupleHash); err != nil {
		return nil, nil, err
	}
	if replay.syncStart != nil {
		result := cloneSyncStartResult(*replay.syncStart)
		return &result, nil, nil
	}
	if replay.syncStartDone != nil {
		return nil, replay.syncStartDone, nil
	}
	if err := CheckSyncStartReplayMode(replay.asyncStart, replay.syncStart, false); err != nil {
		return nil, nil, err
	}
	return nil, nil, ErrSessionNotFound
}

func (s *RuntimeService) replayStoredSyncStart(requestID string) (SyncStartResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	replay, ok := s.startReplay[requestID]
	if !ok {
		return SyncStartResult{}, ErrSessionNotFound
	}
	if replay.syncStart == nil {
		return SyncStartResult{}, fmt.Errorf("factorysessionexecution: sync start outcome unavailable for request %q", requestID)
	}
	return cloneSyncStartResult(*replay.syncStart), nil
}

func (s *RuntimeService) launchSyncRuntimeSession(prepared PreparedStart, sessionID string) context.CancelFunc {
	runCtx := context.Background()
	var stopRuntime context.CancelFunc
	if prepared.Request.Wait != nil && prepared.Request.Wait.CancelOnTimeout {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithCancel(context.Background())
		stopRuntime = cancel
		go func() {
			defer cancel()
			s.runSession(runCtx, prepared, sessionID)
		}()
		return stopRuntime
	}
	go s.runSession(runCtx, prepared, sessionID)
	return nil
}

func (s *RuntimeService) awaitSyncStartOutcome(
	ctx context.Context,
	prepared PreparedStart,
	sessionID string,
	runtimeDone <-chan struct{},
	stopRuntime context.CancelFunc,
) (SyncStartResult, error) {
	waitCtx := ctx
	var cancelWait context.CancelFunc
	if timeout := syncWaitTimeout(prepared.Request.Wait); timeout > 0 {
		waitCtx, cancelWait = context.WithTimeout(ctx, timeout)
	}
	if cancelWait != nil {
		defer cancelWait()
	}

	select {
	case <-runtimeDone:
		return s.syncStartOutcomeOnRuntimeDone(prepared.Request.RequestID, sessionID)
	case <-waitCtx.Done():
		return s.syncStartOutcomeOnWaitDone(ctx, waitCtx, prepared, sessionID, stopRuntime)
	}
}

func (s *RuntimeService) syncStartOutcomeOnRuntimeDone(requestID, sessionID string) (SyncStartResult, error) {
	s.mu.RLock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.RUnlock()
		return SyncStartResult{}, ErrSessionNotFound
	}
	result := s.syncStartFromStateLocked(state)
	s.mu.RUnlock()
	s.finalizeSyncStartReplay(requestID, &result)
	return result, nil
}

func (s *RuntimeService) syncStartOutcomeOnWaitDone(
	ctx context.Context,
	waitCtx context.Context,
	prepared PreparedStart,
	sessionID string,
	stopRuntime context.CancelFunc,
) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && syncWaitTimeout(prepared.Request.Wait) > 0 {
		cancelOnTimeout := prepared.Request.Wait != nil && prepared.Request.Wait.CancelOnTimeout
		if cancelOnTimeout && stopRuntime != nil {
			stopRuntime()
		}
		s.applySyncWaitTimeout(sessionID)
		s.mu.RLock()
		state, ok := s.sessions[sessionID]
		if !ok {
			s.mu.RUnlock()
			return SyncStartResult{}, ErrSessionNotFound
		}
		result := s.syncTimedOutFromStateLocked(state, cancelOnTimeout)
		s.mu.RUnlock()
		s.finalizeSyncStartReplay(prepared.Request.RequestID, &result)
		return result, nil
	}
	return SyncStartResult{}, waitCtx.Err()
}

func (s *RuntimeService) GetSession(ctx context.Context, sessionID string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return SessionReadResult{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
	if err != nil {
		return SessionReadResult{}, err
	}
	return cloneSessionRead(state.session), nil
}

func (s *RuntimeService) Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlPause)
}

func (s *RuntimeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlResume)
}

func (s *RuntimeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlCancel)
}

func (s *RuntimeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlTerminate)
}

func (s *RuntimeService) Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlApprove)
}

func (s *RuntimeService) RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx, sessionID, LifecycleControlRetryDispatch)
}

func (s *RuntimeService) GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error) {
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
	if err != nil {
		return ResultReadResult{}, err
	}
	return ProjectResultRead(state.result, state.session, state.artifacts, normalized)
}

func (s *RuntimeService) ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDispatchesResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	return ListDispatchesResult{
		SessionID:  id,
		Dispatches: cloneDispatchSummaries(state.dispatches),
	}, nil
}

func (s *RuntimeService) GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error) {
	if err := ctx.Err(); err != nil {
		return DispatchDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return DispatchDetail{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
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

func (s *RuntimeService) ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListArtifactsResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	return ListArtifactsResult{
		SessionID: id,
		Artifacts: cloneArtifactSummaries(state.artifacts),
	}, nil
}

func (s *RuntimeService) GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ArtifactDetail{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.sessionStateLocked(id)
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

func (s *RuntimeService) ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error) {
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
	s.mu.RLock()
	state, err := s.sessionStateLocked(id)
	if err != nil {
		s.mu.RUnlock()
		return EventReadResult{}, err
	}
	events := append([]json.RawMessage(nil), state.events...)
	s.mu.RUnlock()

	filtered, err := FilterEventsAfterReconnect(events, req, id)
	if err != nil {
		return EventReadResult{}, err
	}
	return EventReadResult{
		SessionID: id,
		Events:    filtered,
	}, nil
}

func (s *RuntimeService) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListSessionsResult{}, err
	}
	normalized, err := NormalizeListSessionsRequest(req)
	if err != nil {
		return ListSessionsResult{}, err
	}

	s.mu.RLock()
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
	s.mu.RUnlock()

	return ApplySessionListScope(ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    live,
		DurableSessions: durable,
	}, normalized), nil
}

func (s *RuntimeService) newRunningSessionState(prepared PreparedStart) *runtimeSessionState {
	sessionID := newDurableSessionID()
	startedAt := s.now().UTC()
	links := InspectionLinksForSession(sessionID, true)
	session := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: orchestratorKindForPrepared(prepared),
		Dialect:          dialectForPrepared(prepared),
		ResolvedSource:   prepared.ResolvedSource,
		SourceHash:       prepared.ResolvedSource.SourceHash,
		Policy:           prepared.Policy,
		Progress: &ProgressCounts{
			TotalDispatches:     0,
			CompletedDispatches: 0,
			FailedDispatches:    0,
			InFlightDispatches:  0,
		},
		Budgets: &SessionBudgets{
			MaxAgents: prepared.EffectivePolicy.MaxAgents,
		},
		Usage: EmptySessionUsage(),
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
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: LifecycleStatusRunning,
		Availability: defaultNotReadyAvailability(session),
	}
	state := &runtimeSessionState{
		session:     session,
		result:      result,
		runtimeDone: make(chan struct{}),
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	return state
}

func (s *RuntimeService) runSession(ctx context.Context, prepared PreparedStart, sessionID string) {
	defer s.markRuntimeDone(sessionID)

	argsJSON, err := json.Marshal(prepared.Request.Args)
	if err != nil {
		s.applyRuntimeError(sessionID, fmt.Errorf("marshal workflow args: %w", err))
		return
	}

	outcome, err := workflowruntime.Run(ctx, workflowruntime.Request{
		Source:    prepared.SourceContent,
		SourceRef: prepared.SourceRef,
		SessionID: sessionID,
		Args:      argsJSON,
		Metadata:  prepared.ResolvedSource.Metadata,
		Policy:    prepared.EffectivePolicy,
	}, workflowruntime.Hooks{})
	if err != nil {
		s.applyRuntimeError(sessionID, err)
		return
	}
	if !outcome.OK {
		s.applyRuntimeFailure(sessionID, outcome.Failure)
		return
	}
	s.applyRuntimeSuccess(sessionID, outcome)
}

func (s *RuntimeService) applyRuntimeSuccess(sessionID string, outcome workflowruntime.Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	finishedAt := s.now().UTC()
	state.session.Status = LifecycleStatusSucceeded
	if state.session.Lifecycle != nil {
		state.session.Lifecycle.FinishedAt = &finishedAt
	}
	recordProjection := ProjectRuntimeExecutionRecords(sessionID, outcome.Records, finishedAt)
	if recordProjection.Phase != "" {
		state.session.Phase = recordProjection.Phase
	}
	state.dispatches = cloneDispatchSummaries(recordProjection.Dispatches)
	state.artifacts = cloneArtifactSummaries(recordProjection.Artifacts)
	state.session.Progress = &recordProjection.Progress
	projected, resultSummary, err := projectRuntimeSuccessResult(sessionID, outcome.Value, state.artifacts)
	if err != nil {
		state.session.Status = LifecycleStatusFailed
		state.session.Lifecycle.FinishedAt = &finishedAt
		state.session.Failure = &FailureSummary{
			Reason:  "WORKFLOW_RUNTIME_INVALID_RESULT",
			Message: err.Error(),
		}
		state.session.ResultSummary = &ResultSummary{
			ResultStatus: string(ResultStatusUnavailable),
		}
		state.result = ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusFailed,
			Failure:       cloneFailureSummary(state.session.Failure),
			Availability:  defaultUnavailableAvailability(),
		}
		state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
		return
	}
	state.session.ResultSummary = resultSummary
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
	state.result = projected
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
}

func (s *RuntimeService) applyRuntimeFailure(sessionID string, failure workflowruntime.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	finishedAt := s.now().UTC()
	state.session.Status = LifecycleStatusFailed
	if state.session.Lifecycle != nil {
		state.session.Lifecycle.FinishedAt = &finishedAt
	}
	state.session.Failure = &FailureSummary{
		Reason:  failure.Code,
		Message: failure.Message,
	}
	state.session.ResultSummary = &ResultSummary{
		ResultStatus: string(ResultStatusUnavailable),
	}
	state.result = ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusUnavailable,
		SessionStatus: LifecycleStatusFailed,
		Failure:       cloneFailureSummary(state.session.Failure),
		Availability:  defaultUnavailableAvailability(),
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
}

func (s *RuntimeService) applyRuntimeError(sessionID string, err error) {
	s.applyRuntimeFailure(sessionID, workflowruntime.Failure{
		Code:    "WORKFLOW_RUNTIME_ERROR",
		Message: err.Error(),
	})
}

func (s *RuntimeService) asyncStartFromState(state *runtimeSessionState) AsyncStartResult {
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

func (s *RuntimeService) syncStartFromStateLocked(state *runtimeSessionState) SyncStartResult {
	result := SyncStartResult{AsyncStartResult: s.asyncStartFromState(state)}
	if !IsTerminalLifecycleStatus(state.session.Status) {
		return result
	}
	result.SyncOutcome = SyncOutcomeCompleted
	projected, err := ProjectResultRead(state.result, state.session, state.artifacts, ResultRequest{
		Mode: ResultModeFinal,
	})
	if err != nil {
		return result
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return result
	}
	result.Result = encoded
	return result
}

func (s *RuntimeService) finalizeSyncStartReplay(requestID string, result *SyncStartResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.startReplay[requestID]
	if !ok {
		return
	}
	if result != nil {
		record.syncStart = cloneSyncStartResultPtr(*result)
		s.startReplay[requestID] = record
	}
	signalSyncStartDone(record.syncStartDone)
}

func signalSyncStartDone(done chan struct{}) {
	if done == nil {
		return
	}
	select {
	case <-done:
	default:
		close(done)
	}
}

func cloneAsyncStartResultPtr(result AsyncStartResult) *AsyncStartResult {
	cloned := cloneAsyncStartResult(result)
	return &cloned
}

func cloneSyncStartResultPtr(result SyncStartResult) *SyncStartResult {
	cloned := cloneSyncStartResult(result)
	return &cloned
}

func (s *RuntimeService) syncTimedOutFromStateLocked(state *runtimeSessionState, sessionCanceledByTimeout bool) SyncStartResult {
	return SyncStartResult{
		AsyncStartResult:         s.asyncStartFromState(state),
		SyncOutcome:              SyncOutcomeTimedOut,
		TimedOut:                 true,
		SessionCanceledByTimeout: sessionCanceledByTimeout,
	}
}

func (s *RuntimeService) applySyncWaitTimeout(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[sessionID]
	if !ok || IsTerminalLifecycleStatus(state.session.Status) {
		return
	}
	state.result = ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: state.session.Status,
		Availability: &ResultAvailabilityDetail{
			Reason:    "SYNC_WAIT_TIMED_OUT",
			Message:   "Sync wait ended before a terminal result was available.",
			Retryable: true,
		},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
}

func (s *RuntimeService) markRuntimeDone(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[sessionID]
	if !ok || state.runtimeDone == nil {
		return
	}
	select {
	case <-state.runtimeDone:
	default:
		close(state.runtimeDone)
	}
}

func syncWaitTimeout(wait *WaitOptions) time.Duration {
	if wait == nil || wait.TimeoutMillis == nil {
		return 0
	}
	return time.Duration(*wait.TimeoutMillis) * time.Millisecond
}

func (s *RuntimeService) sessionStateLocked(sessionID string) (*runtimeSessionState, error) {
	state, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return state, nil
}

func (s *RuntimeService) unsupportedLifecycleControl(ctx context.Context, sessionID string, operation LifecycleControlKind) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	s.mu.RLock()
	state, stateErr := s.sessionStateLocked(id)
	s.mu.RUnlock()
	if stateErr != nil {
		return LifecycleControlResult{}, stateErr
	}
	return LifecycleControlResult{}, &ControlError{
		Operation: operation,
		Outcome:   LifecycleControlOutcomeInvalidState,
		Status:    state.session.Status,
		Message:   ErrUnsupportedControl.Error(),
	}
}

func newDurableSessionID() string {
	token := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(token) > 12 {
		token = token[:12]
	}
	return "dur-sess-" + token
}

func orchestratorKindForPrepared(prepared PreparedStart) string {
	if prepared.Request.Orchestrator != nil {
		if kind := strings.TrimSpace(prepared.Request.Orchestrator.Kind); kind != "" {
			return strings.ToUpper(kind)
		}
	}
	return "JAVASCRIPT"
}

func dialectForPrepared(prepared PreparedStart) string {
	if dialect := strings.TrimSpace(prepared.ResolvedSource.Dialect); dialect != "" {
		return dialect
	}
	return "you-workflow-v1"
}

func defaultUnavailableAvailability() *ResultAvailabilityDetail {
	return &ResultAvailabilityDetail{
		Reason:    "UNAVAILABLE",
		Message:   "session result is unavailable",
		Retryable: false,
	}
}

func projectRuntimeSuccessResult(sessionID string, value workflowresult.TypedValue, artifacts []ArtifactSummary) (ResultReadResult, *ResultSummary, error) {
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, value, artifactStatesFromSummaries(artifacts))
	if validation.HasIssues() {
		return ResultReadResult{}, nil, fmt.Errorf("project primary result: %v", validation.Issues)
	}

	primaryJSON := workContentJSONFromParts(parts)
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: primaryJSON,
		ArtifactIDs:   artifactIDsFromSummaries(artifacts),
	}
	summary := &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
		Summary:      resultSummaryTextFromParts(parts),
	}
	return result, summary, nil
}

func workContentJSONFromParts(parts []interfaces.WorkContentPart) json.RawMessage {
	content := workcontent.GeneratedPtrFromParts(parts)
	if content == nil {
		return nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return encoded
}

func resultSummaryTextFromParts(parts []interfaces.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeText {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func artifactStatesFromSummaries(artifacts []ArtifactSummary) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	states := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		states = append(states, interfaces.FactorySessionArtifactState{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			Label:       artifact.Label,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
			AuditMode:   artifact.AuditMode,
		})
	}
	return states
}
