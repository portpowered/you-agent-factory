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
	session SessionReadResult
	result  ResultReadResult
	events  []json.RawMessage
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

	if outcome.OK {
		session.Status = LifecycleStatusSucceeded
		result.SessionStatus = LifecycleStatusSucceeded
		result.ResultStatus = ResultStatusFinal
		result.PrimaryResult = cloneRawJSON(outcome.Value.JSON)
		session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusFinal)}
	} else {
		projectRuntimeFailure(&session, &result, outcome)
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
		result.Availability = &ResultAvailabilityDetail{
			Reason:    "SYNC_WAIT_TIMED_OUT",
			Message:   failure.Message,
			Retryable: false,
		}
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
	if effective := effectivePolicyMap(resolution.Policy); len(effective) > 0 {
		projection.Effective = effective
	}
	return projection
}

func effectivePolicyMap(policy workflowpolicy.EffectivePolicy) map[string]any {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil
	}
	return decoded
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

	mu          sync.RWMutex
	sessions    map[string]*runtimeSessionState
	startReplay map[string]startReplayRecord
}

var _ Service = (*JavaScriptRuntimeService)(nil)

// NewJavaScriptRuntimeService constructs one JavaScript runtime-backed durable session service.
func NewJavaScriptRuntimeService(config JavaScriptRuntimeServiceConfig) *JavaScriptRuntimeService {
	return &JavaScriptRuntimeService{
		projectRoot: strings.TrimSpace(config.ProjectRoot),
		sessions:    make(map[string]*runtimeSessionState),
		startReplay: make(map[string]startReplayRecord),
	}
}

func (s *JavaScriptRuntimeService) StartAsync(_ context.Context, _ StartRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, fmt.Errorf("asynchronous JavaScript runtime session start is not implemented")
}

func (s *JavaScriptRuntimeService) StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error) {
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
	if replay, ok := s.startReplay[normalized.RequestID]; ok {
		if err := CheckRequestIDReplay(normalized.RequestID, replay.tupleHash, tupleHash); err != nil {
			s.mu.Unlock()
			return SyncStartResult{}, err
		}
		state, ok := s.sessions[replay.sessionID]
		s.mu.Unlock()
		if !ok {
			return SyncStartResult{}, ErrSessionNotFound
		}
		return s.syncStartFromState(state), nil
	}
	s.mu.Unlock()

	state, err := s.executeSyncSession(ctx, normalized)
	if err != nil {
		return SyncStartResult{}, err
	}

	s.mu.Lock()
	s.sessions[state.session.SessionID] = state
	s.startReplay[normalized.RequestID] = startReplayRecord{
		sessionID: state.session.SessionID,
		tupleHash: tupleHash,
	}
	s.mu.Unlock()

	return s.syncStartFromState(state), nil
}

func (s *JavaScriptRuntimeService) GetSession(ctx context.Context, sessionID string) (SessionReadResult, error) {
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

func (s *JavaScriptRuntimeService) Pause(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) Resume(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
}

func (s *JavaScriptRuntimeService) Cancel(ctx context.Context, _ string, _ ControlRequest) (LifecycleControlResult, error) {
	return s.unsupportedLifecycleControl(ctx)
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
	state, err := s.sessionState(id)
	if err != nil {
		return ResultReadResult{}, err
	}
	return ProjectResultRead(state.result, state.session, nil, normalized)
}

func (s *JavaScriptRuntimeService) ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error) {
	if err := ctx.Err(); err != nil {
		return ListDispatchesResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ListDispatchesResult{}, err
	}
	if _, err := s.sessionState(id); err != nil {
		return ListDispatchesResult{}, err
	}
	return ListDispatchesResult{SessionID: id, Dispatches: nil}, nil
}

func (s *JavaScriptRuntimeService) GetDispatch(ctx context.Context, sessionID, _ string) (DispatchDetail, error) {
	if err := ctx.Err(); err != nil {
		return DispatchDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return DispatchDetail{}, err
	}
	if _, err := s.sessionState(id); err != nil {
		return DispatchDetail{}, err
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
	if _, err := s.sessionState(id); err != nil {
		return ListArtifactsResult{}, err
	}
	return ListArtifactsResult{SessionID: id, Artifacts: nil}, nil
}

func (s *JavaScriptRuntimeService) GetArtifact(ctx context.Context, sessionID, _ string) (ArtifactDetail, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactDetail{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return ArtifactDetail{}, err
	}
	if _, err := s.sessionState(id); err != nil {
		return ArtifactDetail{}, err
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
	state, err := s.sessionState(id)
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

func (s *JavaScriptRuntimeService) executeSyncSession(ctx context.Context, normalized StartRequest) (*runtimeSessionState, error) {
	resolved, err := ResolveStartSource(normalized, StartSourceContext{ProjectRoot: s.projectRoot})
	if err != nil {
		return nil, err
	}
	sourceContent, err := LoadStartSourceContent(normalized, resolved, StartSourceContext{ProjectRoot: s.projectRoot})
	if err != nil {
		return nil, err
	}

	runCtx := ctx
	if normalized.Wait != nil && normalized.Wait.TimeoutMillis != nil && *normalized.Wait.TimeoutMillis > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(*normalized.Wait.TimeoutMillis)*time.Millisecond)
		defer cancel()
	}

	argsJSON, err := marshalStartArgs(normalized.Args)
	if err != nil {
		return nil, err
	}
	policyResolution := workflowpolicy.Resolve(workflowpolicy.Request{Requested: normalized.RequestedPolicy})
	sessionID := NewDurableSessionID()
	startedAt := time.Now().UTC()

	outcome, err := workflowruntime.Run(runCtx, workflowruntime.Request{
		Source:    sourceContent,
		SourceRef: resolved.SourceRef,
		SessionID: sessionID,
		Args:      argsJSON,
		Metadata:  workflowMetadataFromResolved(resolved, normalized),
		Policy:    policyResolution.Policy,
	}, workflowruntime.Hooks{})
	if err != nil {
		return nil, err
	}

	state := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt)
	return &state, nil
}

func (s *JavaScriptRuntimeService) sessionState(sessionID string) (*runtimeSessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return state, nil
}

func (s *JavaScriptRuntimeService) syncStartFromState(state *runtimeSessionState) SyncStartResult {
	async := s.asyncStartFromState(state)
	result := SyncStartResult{AsyncStartResult: async}
	if state.session.Status == LifecycleStatusTimedOut ||
		(state.result.Availability != nil && state.result.Availability.Reason == "SYNC_WAIT_TIMED_OUT") {
		result.SyncOutcome = SyncOutcomeTimedOut
		result.TimedOut = true
		return result
	}
	if IsTerminalLifecycleStatus(state.session.Status) {
		result.SyncOutcome = SyncOutcomeCompleted
		if encoded, err := json.Marshal(state.result); err == nil {
			result.Result = encoded
		}
	}
	return result
}

func (s *JavaScriptRuntimeService) asyncStartFromState(state *runtimeSessionState) AsyncStartResult {
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
