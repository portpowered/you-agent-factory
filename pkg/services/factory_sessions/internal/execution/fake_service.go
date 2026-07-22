package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
)

// FakeService is a deterministic in-memory implementation of Service for API, CLI,
// MCP, and UI development before the real durable runtime exists.
type FakeService struct {
	mu    sync.RWMutex
	clock factory.Clock

	scenariosByRequestID map[string]FakeScenario
	sessions             map[string]*fakeSessionState
	startReplay          map[string]startReplayRecord
	controlReplay        map[string]controlReplayRecord
	persistedSeeds       []DurableSessionListSummary
}

type startReplayRecord struct {
	sessionID  string
	tupleHash  string
	syncStart  *SyncStartResult
	asyncStart *AsyncStartResult
}

// NewFakeService constructs one deterministic fake durable session service.
func NewFakeService(clock factory.Clock, scenarios ...FakeScenario) (*FakeService, error) {
	if clock == nil {
		return nil, fmt.Errorf("Factory Session execution clock is required")
	}
	service := &FakeService{
		clock:                clock,
		scenariosByRequestID: make(map[string]FakeScenario),
		sessions:             make(map[string]*fakeSessionState),
		startReplay:          make(map[string]startReplayRecord),
		controlReplay:        make(map[string]controlReplayRecord),
	}
	for _, scenario := range scenarios {
		if scenario.RequestID == "" {
			continue
		}
		service.scenariosByRequestID[scenario.RequestID] = scenario
		if scenario.ListSummary != nil && IsPersistedListCandidate(*scenario.ListSummary) {
			service.persistedSeeds = appendPersistedSeed(service.persistedSeeds, *scenario.ListSummary)
		}
	}
	return service, nil
}

// NewFakeServiceFromContractFixtures loads scenarios from the durable contract
// fixture catalog and registers the built-in interrupted/recoverable scenario.
func NewFakeServiceFromContractFixtures(
	path string,
	clock factory.Clock,
	files fileeffects.ContractFixtureReader,
) (*FakeService, error) {
	if clock == nil {
		return nil, fmt.Errorf("Factory Session execution clock is required")
	}
	scenarios, err := LoadFakeScenariosFromContractFixtures(path, files)
	if err != nil {
		return nil, err
	}
	return NewFakeService(clock, scenarios...)
}

var _ Service = (*FakeService)(nil)

func (s *FakeService) StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return AsyncStartResult{}, err
	}
	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return AsyncStartResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if replay, ok := s.startReplay[normalized.RequestID]; ok {
		if err := CheckRequestIDReplay(normalized.RequestID, replay.tupleHash, tupleHash); err != nil {
			return AsyncStartResult{}, err
		}
		if err := CheckAsyncStartReplayMode(replay.asyncStart); err != nil {
			return AsyncStartResult{}, err
		}
		return *replay.asyncStart, nil
	}

	scenario, ok := s.scenariosByRequestID[normalized.RequestID]
	if !ok {
		return AsyncStartResult{}, NewValidationError("requestId", fmt.Sprintf("unknown fake scenario for requestId %q", normalized.RequestID))
	}
	state := fakeSessionStateFromScenario(scenario)
	s.sessions[state.session.SessionID] = state
	result := s.asyncStartFromScenario(scenario, state)
	cloned := cloneAsyncStartResult(result)
	s.startReplay[normalized.RequestID] = startReplayRecord{
		sessionID:  state.session.SessionID,
		tupleHash:  tupleHash,
		asyncStart: &cloned,
	}
	return result, nil
}

func (s *FakeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncStartResult{}, err
	}
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return SyncStartResult{}, err
	}
	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return SyncStartResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if replay, ok := s.startReplay[normalized.RequestID]; ok {
		if err := CheckRequestIDReplay(normalized.RequestID, replay.tupleHash, tupleHash); err != nil {
			return SyncStartResult{}, err
		}
		if replay.syncStart != nil {
			return *replay.syncStart, nil
		}
		if err := CheckSyncStartReplayMode(replay.asyncStart, replay.syncStart, false); err != nil {
			return SyncStartResult{}, err
		}
		return SyncStartResult{}, ErrSessionNotFound
	}

	scenario, ok := s.scenariosByRequestID[normalized.RequestID]
	if !ok {
		return SyncStartResult{}, NewValidationError("requestId", fmt.Sprintf("unknown fake scenario for requestId %q", normalized.RequestID))
	}
	state := fakeSessionStateFromScenario(scenario)
	s.sessions[state.session.SessionID] = state
	result := s.syncStartFromScenario(scenario, state)
	applySyncWaitOutcome(&result, state, normalized)
	cloned := cloneSyncStartResult(result)
	s.startReplay[normalized.RequestID] = startReplayRecord{
		sessionID: state.session.SessionID,
		tupleHash: tupleHash,
		syncStart: &cloned,
	}
	return result, nil
}

func (s *FakeService) ResumeInterruptedSession(ctx context.Context, sessionID string, req ResumeSessionRequest) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	return AsyncStartResult{}, ErrUnsupportedControl
}

func (s *FakeService) GetSession(ctx context.Context, sessionID string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return SessionReadResult{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return SessionReadResult{}, err
	}
	return cloneSessionRead(state.session), nil
}

func (s *FakeService) Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlPause, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlResume, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlTerminate, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlApprove, req.ControlRequest, req, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlRetryDispatch, req.ControlRequest, ApproveRequest{}, req, InterruptDispatchRequest{})
}

func (s *FakeService) InterruptDispatch(ctx context.Context, sessionID string, req InterruptDispatchRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlInterruptDispatch, req.ControlRequest, ApproveRequest{}, RetryDispatchRequest{}, req)
}

func (s *FakeService) GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error) {
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
	state, err := s.sessionState(id)
	if err != nil {
		return ResultReadResult{}, err
	}
	return ProjectResultRead(state.result, state.session, state.artifacts, normalized)
}

func (s *FakeService) ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDispatchesResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	return ListDispatchesResult{
		SessionID:  id,
		Dispatches: cloneDispatchSummaries(state.dispatches),
	}, nil
}

func (s *FakeService) QueryDispatches(ctx context.Context, request DispatchQueryRequest) (ListDispatchesResult, error) {
	return queryDispatches(ctx, s, request)
}

func (s *FakeService) GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error) {
	if err := ctx.Err(); err != nil {
		return DispatchDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return DispatchDetail{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return DispatchDetail{}, err
	}
	if detail, ok := state.dispatchDetails[dispatchID]; ok {
		return detail, nil
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

func (s *FakeService) ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListArtifactsResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return ListArtifactsResult{}, err
	}
	return ListArtifactsResult{
		SessionID: id,
		Artifacts: cloneArtifactSummaries(state.artifacts),
	}, nil
}

func (s *FakeService) GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ArtifactDetail{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return ArtifactDetail{}, err
	}
	if detail, ok := state.artifactDetails[artifactID]; ok {
		return detail, nil
	}
	for _, summary := range state.artifacts {
		if summary.ID == artifactID {
			return ArtifactDetail{ArtifactSummary: summary, SessionID: id}, nil
		}
	}
	return ArtifactDetail{}, ErrArtifactNotFound
}

func (s *FakeService) ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error) {
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
	state, err := s.sessionState(id)
	if err != nil {
		return EventReadResult{}, err
	}
	filtered, err := FilterEventsAfterReconnect(state.events, req, id)
	if err != nil {
		return EventReadResult{}, err
	}
	return EventReadResult{
		SessionID: id,
		Events:    filtered,
	}, nil
}

func (s *FakeService) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error) {
	if err := ctx.Err(); err != nil {
		return ListSessionsResult{}, err
	}
	normalized, err := NormalizeListSessionsRequest(req)
	if err != nil {
		return ListSessionsResult{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	durable := append([]DurableSessionListSummary(nil), s.persistedSeeds...)
	live := make([]LiveSessionSummary, 0, len(s.sessions))
	seenDurable := make(map[string]struct{}, len(durable))
	for _, summary := range durable {
		seenDurable[summary.SessionID] = struct{}{}
	}
	for _, state := range s.sessions {
		read := cloneSessionRead(state.session)
		live = append(live, LiveListSummaryFromSessionRead(read))
		summary := DurableListSummaryFromSessionRead(read)
		if !IsPersistedListCandidate(summary) {
			continue
		}
		if _, exists := seenDurable[summary.SessionID]; exists {
			continue
		}
		durable = append(durable, summary)
		seenDurable[summary.SessionID] = struct{}{}
	}

	return ApplySessionListScope(ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    live,
		DurableSessions: durable,
	}, normalized), nil
}

func (s *FakeService) sessionState(sessionID string) (*fakeSessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return state, nil
}

func validateLifecycleControlRequest(
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) error {
	switch operation {
	case LifecycleControlApprove:
		if _, err := NormalizeApproveRequest(approve); err != nil {
			return err
		}
	case LifecycleControlRetryDispatch:
		if _, err := NormalizeRetryDispatchRequest(retry); err != nil {
			return err
		}
	case LifecycleControlInterruptDispatch:
		if _, err := NormalizeInterruptDispatchRequest(interrupt); err != nil {
			return err
		}
	default:
		if _, err := NormalizeControlRequest(control); err != nil {
			return err
		}
	}
	return nil
}

func lifecycleControlResultFromState(
	state *fakeSessionState,
	id string,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) LifecycleControlResult {
	result := LifecycleControlResult{
		SessionID: id,
		Operation: operation,
		Outcome:   outcome,
		Status:    state.session.Status,
		Links:     LifecycleControlLinksForSession(id, true),
	}
	switch operation {
	case LifecycleControlRetryDispatch:
		result.DispatchID = retry.DispatchID
		if outcome == LifecycleControlOutcomeAccepted {
			result.RetryDispatchID = retry.DispatchID
		}
	case LifecycleControlInterruptDispatch:
		result.DispatchID = interrupt.DispatchID
	}
	if outcome == LifecycleControlOutcomeAccepted || outcome == LifecycleControlOutcomeNoOp {
		session := cloneSessionRead(state.session)
		result.Session = &session
	}
	return result
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake lifecycle path keeps control validation, mutation, and projection assembly together on one seam.
func (s *FakeService) applyLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	if err := validateLifecycleControlRequest(operation, control, approve, retry, interrupt); err != nil {
		return LifecycleControlResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[id]
	if !ok {
		return LifecycleControlResult{}, ErrSessionNotFound
	}

	currentStatus := state.session.Status
	requestID := strings.TrimSpace(control.RequestID)
	var tupleHash string
	if requestID != "" {
		var err error
		tupleHash, err = ControlIdempotencyTupleHash(operation, id, approve, retry, interrupt)
		if err != nil {
			return LifecycleControlResult{}, err
		}
		if replayResult, replayErr, replayed := lookupControlReplay(
			s.controlReplay,
			id,
			requestID,
			tupleHash,
			operation,
			currentStatus,
		); replayed {
			if replayErr != nil {
				return LifecycleControlResult{}, replayErr
			}
			return replayResult, nil
		}
	}

	var dispatchSummary DispatchSummary
	switch operation {
	case LifecycleControlRetryDispatch:
		var ok bool
		dispatchSummary, ok = findDispatchSummary(state.dispatches, retry.DispatchID)
		if !ok {
			return LifecycleControlResult{}, ErrDispatchNotFound
		}
	case LifecycleControlInterruptDispatch:
		var ok bool
		dispatchSummary, ok = findDispatchSummary(state.dispatches, interrupt.DispatchID)
		if !ok {
			return LifecycleControlResult{}, ErrDispatchNotFound
		}
	}

	outcome := evaluateExtendedLifecycleControlOutcome(operation, state.session.Status, dispatchSummary.Status)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		controlErr := &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, state.session.Status),
			Links:     LifecycleControlLinksForSession(id, true),
		}
		storeControlReplay(s.controlReplay, requestID, tupleHash, LifecycleControlResult{}, controlErr)
		return LifecycleControlResult{}, controlErr
	}

	if outcome == LifecycleControlOutcomeAccepted {
		previousStatus := state.session.Status
		s.mutateSessionForControl(state, operation, retry, interrupt)
		state.events = appendAcceptedSessionLifecycleEventIfNeeded(
			state.events,
			state.session,
			previousStatus,
			operation,
			outcome,
			canonicalEventSourceFakeService,
			control.Reason,
			time.Time{},
		)
	}

	result := lifecycleControlResultFromState(state, id, operation, outcome, retry, interrupt)
	storeControlReplay(s.controlReplay, requestID, tupleHash, result, nil)
	return result, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake mutation helper keeps lifecycle control state transitions together for deterministic fixtures.
func (s *FakeService) mutateSessionForControl(
	state *fakeSessionState,
	operation LifecycleControlKind,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) {
	var interruptedDispatch DispatchSummary
	var priorDispatchStatus DispatchStatus
	switch operation {
	case LifecycleControlPause:
		if state.session.Status == LifecycleStatusRunning || state.session.Status == LifecycleStatusResuming {
			pausedAt := s.clock.Now().UTC()
			state.session.Status = LifecycleStatusPaused
			state.result.SessionStatus = LifecycleStatusPaused
			if state.session.Lifecycle == nil {
				state.session.Lifecycle = &LifecycleTimestamps{}
			}
			state.session.Lifecycle.PausedAt = &pausedAt
		}
	case LifecycleControlResume:
		if state.session.Status == LifecycleStatusPaused {
			resumedAt := s.clock.Now().UTC()
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
			if state.session.Lifecycle == nil {
				state.session.Lifecycle = &LifecycleTimestamps{}
			}
			state.session.Lifecycle.ResumedAt = &resumedAt
		}
	case LifecycleControlCancel:
		switch state.session.Status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming, LifecycleStatusQueued, LifecycleStatusAwaitingApproval:
			state.session.Status = LifecycleStatusCanceling
			state.result.SessionStatus = LifecycleStatusCanceling
		}
	case LifecycleControlTerminate:
		switch state.session.Status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming, LifecycleStatusQueued, LifecycleStatusAwaitingApproval, LifecycleStatusCanceling:
			state.session.Status = LifecycleStatusTerminated
			state.result.SessionStatus = LifecycleStatusTerminated
			state.result.ResultStatus = ResultStatusUnavailable
		}
	case LifecycleControlApprove:
		if state.session.Status == LifecycleStatusAwaitingApproval {
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
		}
	case LifecycleControlRetryDispatch:
		if state.session.Status == LifecycleStatusFailed {
			for index, dispatch := range state.dispatches {
				if dispatch.ID != retry.DispatchID {
					continue
				}
				dispatch.Status = DispatchStatusQueued
				dispatch.Attempt++
				state.dispatches[index] = dispatch
				if detail, ok := state.dispatchDetails[retry.DispatchID]; ok {
					detail.Status = DispatchStatusQueued
					detail.Attempt = dispatch.Attempt
					state.dispatchDetails[retry.DispatchID] = detail
				}
			}
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
		}
	case LifecycleControlInterruptDispatch:
		if summary, ok := findDispatchSummary(state.dispatches, interrupt.DispatchID); ok {
			interruptedDispatch = summary
			priorDispatchStatus = summary.Status
		}
		state.dispatches, _ = MarkDispatchInterrupted(
			state.dispatches,
			nil,
			interrupt.DispatchID,
			interrupt,
		)
		if detail, ok := state.dispatchDetails[interrupt.DispatchID]; ok {
			detail.Status = DispatchStatusInterrupted
			if summary, ok := findDispatchSummary(state.dispatches, interrupt.DispatchID); ok {
				detail.FailureDetail = summary.FailureDetail
			}
			state.dispatchDetails[interrupt.DispatchID] = detail
		}
	}
	if operation == LifecycleControlPause || operation == LifecycleControlResume {
		return
	}
	state.events = deriveProjectionEvents(state.session, state.result)
	if operation == LifecycleControlInterruptDispatch && interruptedDispatch.ID != "" {
		state.events = AppendDispatchInterruptedEvent(
			state.events,
			state.session,
			interruptedDispatch,
			interrupt,
			priorDispatchStatus,
			canonicalEventSourceFakeService,
			canonicalSessionEventTime(state.session),
		)
	}
}

func (s *FakeService) asyncStartFromScenario(scenario FakeScenario, state *fakeSessionState) AsyncStartResult {
	if scenario.AsyncStart != nil {
		return *scenario.AsyncStart
	}
	return s.asyncStartFromState(state)
}

func (s *FakeService) asyncStartFromState(state *fakeSessionState) AsyncStartResult {
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

func applySyncWaitOutcome(result *SyncStartResult, state *fakeSessionState, req StartRequest) {
	if result == nil || state == nil {
		return
	}
	if result.SyncOutcome != SyncOutcomeTimedOut && !result.TimedOut {
		return
	}
	if req.Wait == nil || !req.Wait.CancelOnTimeout {
		return
	}

	result.SessionCanceledByTimeout = true
	result.Result = nil
	switch state.session.Status {
	case LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusResuming,
		LifecycleStatusQueued,
		LifecycleStatusAwaitingApproval:
		state.session.Status = LifecycleStatusCanceling
		state.result.SessionStatus = LifecycleStatusCanceling
		state.result.ResultStatus = ResultStatusUnavailable
		state.result.Availability = &ResultAvailabilityDetail{
			Reason:    "SESSION_CANCELED",
			Message:   "Session cancel was submitted after sync wait timed out.",
			Retryable: false,
		}
	}
	result.Status = string(state.session.Status)
	state.events = deriveProjectionEvents(state.session, state.result)
}

func (s *FakeService) syncStartFromScenario(scenario FakeScenario, state *fakeSessionState) SyncStartResult {
	if scenario.SyncStart != nil {
		return *scenario.SyncStart
	}
	return s.syncStartFromState(state)
}

func (s *FakeService) syncStartFromState(state *fakeSessionState) SyncStartResult {
	async := s.asyncStartFromState(state)
	result := SyncStartResult{AsyncStartResult: async}
	if IsTerminalLifecycleStatus(state.session.Status) {
		result.SyncOutcome = SyncOutcomeCompleted
		encoded, err := json.Marshal(state.result)
		if err == nil {
			result.Result = encoded
		}
	}
	return result
}

func appendPersistedSeed(existing []DurableSessionListSummary, summary DurableSessionListSummary) []DurableSessionListSummary {
	for _, row := range existing {
		if row.SessionID == summary.SessionID {
			return existing
		}
	}
	return append(existing, summary)
}

func findDispatchSummary(dispatches []DispatchSummary, dispatchID string) (DispatchSummary, bool) {
	for _, dispatch := range dispatches {
		if dispatch.ID == dispatchID {
			return dispatch, true
		}
	}
	return DispatchSummary{}, false
}

// mode and includeArtifacts parameters.
func ProjectResultRead(canonical ResultReadResult, session SessionReadResult, artifacts []ArtifactSummary, req ResultRequest) (ResultReadResult, error) {
	normalized, err := NormalizeResultRequest(req)
	if err != nil {
		return ResultReadResult{}, err
	}

	status := canonicalResultStatus(canonical, session)
	projected := cloneResultRead(canonical)
	projected.Mode = normalized.Mode
	projected.IncludeArtifacts = normalized.IncludeArtifacts
	projected = applyResultModeShaping(projected, canonical, session, status, normalized.Mode)
	projected = applyResultArtifactShaping(projected, artifacts, normalized.IncludeArtifacts)
	return projected, nil
}

func canonicalResultStatus(canonical ResultReadResult, session SessionReadResult) ResultStatus {
	if session.ResultSummary != nil {
		if status := strings.TrimSpace(session.ResultSummary.ResultStatus); status != "" {
			return ResultStatus(status)
		}
	}
	return canonical.ResultStatus
}

func applyResultModeShaping(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus, mode ResultMode) ResultReadResult {
	switch mode {
	case ResultModePartial:
		return shapePartialModeResult(projected, canonical, session, status)
	case ResultModeFinal:
		return shapeFinalModeResult(projected, canonical, session, status)
	default:
		projected.ResultStatus = status
		return projected
	}
}

func shapePartialModeResult(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus) ResultReadResult {
	projected.ResultStatus = status
	switch status {
	case ResultStatusPartial, ResultStatusFinal, ResultStatusFailedWithPartial:
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusNotReady, ResultStatusUnavailable:
		projected.PrimaryResult = nil
		projected.Failure = nil
		projected.Availability = cloneResultAvailability(canonical.Availability)
		if projected.Availability == nil && status == ResultStatusNotReady {
			projected.Availability = defaultNotReadyAvailability(session)
		}
	}
	return projected
}

func shapeFinalModeResult(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus) ResultReadResult {
	switch status {
	case ResultStatusPartial:
		if !IsTerminalLifecycleStatus(session.Status) {
			projected.ResultStatus = ResultStatusNotReady
			projected.PrimaryResult = nil
			projected.Failure = nil
			projected.Availability = cloneResultAvailability(canonical.Availability)
			if projected.Availability == nil {
				projected.Availability = defaultNotReadyAvailability(session)
			}
			return projected
		}
		projected.ResultStatus = status
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusFinal, ResultStatusFailedWithPartial:
		projected.ResultStatus = status
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusNotReady, ResultStatusUnavailable:
		projected.ResultStatus = status
		projected.PrimaryResult = nil
		projected.Failure = nil
		projected.Availability = cloneResultAvailability(canonical.Availability)
		if projected.Availability == nil && status == ResultStatusNotReady {
			projected.Availability = defaultNotReadyAvailability(session)
		}
	default:
		projected.ResultStatus = status
	}
	return projected
}

func applyResultArtifactShaping(projected ResultReadResult, artifacts []ArtifactSummary, includeArtifacts bool) ResultReadResult {
	projected.ArtifactIDs = nil
	projected.ArtifactRefs = nil

	if includeArtifacts {
		projected.ArtifactRefs = artifactRefsFromSummaries(artifacts)
		return projected
	}

	projected.ArtifactIDs = artifactIDsFromSummaries(artifacts)
	return projected
}

func artifactRefsFromSummaries(artifacts []ArtifactSummary) []ArtifactRefSummary {
	if len(artifacts) == 0 {
		return nil
	}
	refs := make([]ArtifactRefSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, ArtifactRefSummary{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
		})
	}
	return refs
}

func artifactIDsFromSummaries(artifacts []ArtifactSummary) []string {
	if len(artifacts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func defaultNotReadyAvailability(session SessionReadResult) *ResultAvailabilityDetail {
	message := "Session is still running."
	if IsTerminalLifecycleStatus(session.Status) {
		message = "Final result is not available."
	}
	return &ResultAvailabilityDetail{
		Reason:    "RESULT_NOT_READY",
		Message:   message,
		Retryable: !IsTerminalLifecycleStatus(session.Status),
	}
}

func cloneFailureSummary(failure *FailureSummary) *FailureSummary {
	if failure == nil {
		return nil
	}
	cloned := *failure
	return &cloned
}

func cloneResultAvailability(availability *ResultAvailabilityDetail) *ResultAvailabilityDetail {
	if availability == nil {
		return nil
	}
	cloned := *availability
	return &cloned
}

func cloneAsyncStartResult(result AsyncStartResult) AsyncStartResult {
	cloned := result
	cloned.ResolvedSource = cloneResolvedSource(result.ResolvedSource)
	cloned.Policy = clonePolicyProjection(result.Policy)
	return cloned
}

func cloneSyncStartResult(result SyncStartResult) SyncStartResult {
	cloned := result
	cloned.AsyncStartResult = cloneAsyncStartResult(result.AsyncStartResult)
	cloned.Result = cloneRawJSON(result.Result)
	return cloned
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
