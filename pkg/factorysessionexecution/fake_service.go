package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// FakeService is a deterministic in-memory implementation of Service for API, CLI,
// MCP, and UI development before the real durable runtime exists.
type FakeService struct {
	mu sync.RWMutex

	scenariosByRequestID map[string]FakeScenario
	sessions             map[string]*fakeSessionState
	startReplay          map[string]startReplayRecord
	controlReplay        map[string]string
	persistedSeeds       []DurableSessionListSummary
}

type startReplayRecord struct {
	sessionID string
	tupleHash string
}

// FakeServiceOption configures one FakeService instance.
type FakeServiceOption func(*FakeService)

// WithFakeScenarios registers deterministic scenarios keyed by execution requestId.
func WithFakeScenarios(scenarios ...FakeScenario) FakeServiceOption {
	return func(service *FakeService) {
		for _, scenario := range scenarios {
			if scenario.RequestID == "" {
				continue
			}
			service.scenariosByRequestID[scenario.RequestID] = scenario
			if scenario.ListSummary != nil && IsPersistedListCandidate(*scenario.ListSummary) {
				service.persistedSeeds = appendPersistedSeed(service.persistedSeeds, *scenario.ListSummary)
			}
		}
	}
}

// WithPersistedSessionSeeds preloads persisted-scope listing rows without requiring
// a start call in the current fake service instance.
func WithPersistedSessionSeeds(summaries ...DurableSessionListSummary) FakeServiceOption {
	return func(service *FakeService) {
		for _, summary := range summaries {
			service.persistedSeeds = appendPersistedSeed(service.persistedSeeds, summary)
		}
	}
}

// NewFakeService constructs one deterministic fake durable session service.
func NewFakeService(options ...FakeServiceOption) *FakeService {
	service := &FakeService{
		scenariosByRequestID: make(map[string]FakeScenario),
		sessions:             make(map[string]*fakeSessionState),
		startReplay:          make(map[string]startReplayRecord),
		controlReplay:        make(map[string]string),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// NewFakeServiceFromContractFixtures loads scenarios from the durable contract
// fixture catalog and registers the built-in interrupted/recoverable scenario.
func NewFakeServiceFromContractFixtures(path string) (*FakeService, error) {
	scenarios, err := LoadFakeScenariosFromContractFixtures(path)
	if err != nil {
		return nil, err
	}
	return NewFakeService(WithFakeScenarios(scenarios...)), nil
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
		state, ok := s.sessions[replay.sessionID]
		if !ok {
			return AsyncStartResult{}, ErrSessionNotFound
		}
		return s.asyncStartFromState(state), nil
	}

	scenario, ok := s.scenariosByRequestID[normalized.RequestID]
	if !ok {
		return AsyncStartResult{}, NewValidationError("requestId", fmt.Sprintf("unknown fake scenario for requestId %q", normalized.RequestID))
	}
	state := fakeSessionStateFromScenario(scenario)
	s.sessions[state.session.SessionID] = state
	s.startReplay[normalized.RequestID] = startReplayRecord{
		sessionID: state.session.SessionID,
		tupleHash: tupleHash,
	}
	return s.asyncStartFromScenario(scenario, state), nil
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
		state, ok := s.sessions[replay.sessionID]
		if !ok {
			return SyncStartResult{}, ErrSessionNotFound
		}
		return s.syncStartFromState(state), nil
	}

	scenario, ok := s.scenariosByRequestID[normalized.RequestID]
	if !ok {
		return SyncStartResult{}, NewValidationError("requestId", fmt.Sprintf("unknown fake scenario for requestId %q", normalized.RequestID))
	}
	state := fakeSessionStateFromScenario(scenario)
	s.sessions[state.session.SessionID] = state
	s.startReplay[normalized.RequestID] = startReplayRecord{
		sessionID: state.session.SessionID,
		tupleHash: tupleHash,
	}
	return s.syncStartFromScenario(scenario, state), nil
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
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlPause, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *FakeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlResume, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *FakeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *FakeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlTerminate, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *FakeService) Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlApprove, req.ControlRequest, req, RetryDispatchRequest{})
}

func (s *FakeService) RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error) {
	return s.applyLifecycleControl(ctx, sessionID, LifecycleControlRetryDispatch, req.ControlRequest, ApproveRequest{}, req)
}

func (s *FakeService) GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error) {
	if err := ctx.Err(); err != nil {
		return ResultReadResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ResultReadResult{}, err
	}
	if _, err := NormalizeResultRequest(req); err != nil {
		return ResultReadResult{}, err
	}
	state, err := s.sessionState(id)
	if err != nil {
		return ResultReadResult{}, err
	}
	return cloneResultRead(state.result), nil
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
	return EventReadResult{
		SessionID: id,
		Events:    append([]json.RawMessage(nil), state.events...),
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

// pkgmaintcheck:ignore-cyclomatic-complexity this fake lifecycle path keeps control validation, mutation, and projection assembly together on one seam.
func (s *FakeService) applyLifecycleControl(
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
	switch operation {
	case LifecycleControlApprove:
		if _, err := NormalizeApproveRequest(approve); err != nil {
			return LifecycleControlResult{}, err
		}
	case LifecycleControlRetryDispatch:
		if _, err := NormalizeRetryDispatchRequest(retry); err != nil {
			return LifecycleControlResult{}, err
		}
	default:
		if _, err := NormalizeControlRequest(control); err != nil {
			return LifecycleControlResult{}, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[id]
	if !ok {
		return LifecycleControlResult{}, ErrSessionNotFound
	}

	if operation == LifecycleControlRetryDispatch {
		if _, ok := findDispatchSummary(state.dispatches, retry.DispatchID); !ok {
			return LifecycleControlResult{}, ErrDispatchNotFound
		}
	}

	outcome := EvaluateLifecycleControl(operation, state.session.Status)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		return LifecycleControlResult{}, &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, state.session.Status),
		}
	}

	if requestID := strings.TrimSpace(control.RequestID); requestID != "" {
		tupleHash, err := ControlIdempotencyTupleHash(operation, id, approve, retry)
		if err != nil {
			return LifecycleControlResult{}, err
		}
		if recorded, ok := s.controlReplay[requestID]; ok {
			if err := CheckControlRequestIDReplay(requestID, recorded, tupleHash); err != nil {
				return LifecycleControlResult{}, err
			}
		} else {
			s.controlReplay[requestID] = tupleHash
		}
	}

	if outcome == LifecycleControlOutcomeAccepted {
		s.mutateSessionForControl(state, operation, retry.DispatchID)
	}

	result := LifecycleControlResult{
		SessionID: id,
		Operation: operation,
		Outcome:   outcome,
		Status:    state.session.Status,
		Links:     LifecycleControlLinksForSession(id, true),
	}
	if operation == LifecycleControlRetryDispatch {
		result.DispatchID = retry.DispatchID
		if outcome == LifecycleControlOutcomeAccepted {
			result.RetryDispatchID = retry.DispatchID
		}
	}
	if outcome == LifecycleControlOutcomeAccepted || outcome == LifecycleControlOutcomeNoOp {
		session := cloneSessionRead(state.session)
		result.Session = &session
	}
	return result, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake mutation helper keeps lifecycle control state transitions together for deterministic fixtures.
func (s *FakeService) mutateSessionForControl(state *fakeSessionState, operation LifecycleControlKind, dispatchID string) {
	switch operation {
	case LifecycleControlPause:
		if state.session.Status == LifecycleStatusRunning || state.session.Status == LifecycleStatusResuming {
			state.session.Status = LifecycleStatusPaused
			state.result.SessionStatus = LifecycleStatusPaused
		}
	case LifecycleControlResume:
		if state.session.Status == LifecycleStatusPaused {
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
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
				if dispatch.ID != dispatchID {
					continue
				}
				dispatch.Status = DispatchStatusQueued
				dispatch.Attempt++
				state.dispatches[index] = dispatch
				if detail, ok := state.dispatchDetails[dispatchID]; ok {
					detail.Status = DispatchStatusQueued
					detail.Attempt = dispatch.Attempt
					state.dispatchDetails[dispatchID] = detail
				}
			}
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
		}
	}
	state.events = deriveProjectionEvents(state.session, state.result)
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
