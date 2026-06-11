package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// RuntimeService executes simple JavaScript workflows through the real runtime
// boundary and projects in-memory durable session state.
type RuntimeService struct {
	mu sync.RWMutex

	prepareCtx StartPrepareContext
	now        func() time.Time

	sessions    map[string]*runtimeSessionState
	startReplay map[string]startReplayRecord
}

type runtimeSessionState struct {
	session   SessionReadResult
	result    ResultReadResult
	dispatches []DispatchSummary
	artifacts []ArtifactSummary
	events    []json.RawMessage
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
		startReplay: make(map[string]startReplayRecord),
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
		state, ok := s.sessions[replay.sessionID]
		if !ok {
			s.mu.Unlock()
			return AsyncStartResult{}, ErrSessionNotFound
		}
		result := s.asyncStartFromState(state)
		s.mu.Unlock()
		return result, nil
	}

	state := s.newRunningSessionState(prepared)
	s.sessions[state.session.SessionID] = state
	s.startReplay[prepared.Request.RequestID] = startReplayRecord{
		sessionID: state.session.SessionID,
		tupleHash: prepared.TupleHash,
	}
	asyncResult := s.asyncStartFromState(state)
	s.mu.Unlock()

	go s.runSessionAsync(prepared, state.session.SessionID)
	return asyncResult, nil
}

func (s *RuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	return SyncStartResult{}, fmt.Errorf("sync start is not implemented by runtime service")
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
		session: session,
		result:  result,
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	return state
}

func (s *RuntimeService) runSessionAsync(prepared PreparedStart, sessionID string) {
	argsJSON, err := json.Marshal(prepared.Request.Args)
	if err != nil {
		s.applyRuntimeError(sessionID, fmt.Errorf("marshal workflow args: %w", err))
		return
	}

	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
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
	state.session.ResultSummary = &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
	}
	state.result = ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: append(json.RawMessage(nil), outcome.Value.JSON...),
	}
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
