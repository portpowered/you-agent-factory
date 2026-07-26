package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

type MockFactory struct {
	SubmitErr                               error
	WorkRequests                            []work.WorkRequest
	SubmitWorkRequestErr                    error
	SubmitWorkRequestResult                 work.WorkRequestSubmitResult
	WorkRequestResults                      map[string]work.WorkRequestSubmitResult
	Marking                                 *petri.PetriMarkingSnapshot
	State                                   interfaces.FactoryState
	Net                                     *petri.Net
	EngineState                             *interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net]
	EngineStateSnapshotErr                  error
	Uptime                                  time.Duration
	FactoryEvents                           []factoryapi.FactoryEvent
	FactoryEventStream                      *interfaces.FactoryEventStream
	FactoryEventStreamCtx                   context.Context
	SubscribeFactoryResponseEventsFunc      func(context.Context, factorysessions.ResponseEventSubscriptionRequest) (apisurface.FactoryResponseEventSubscription, error)
	FactoryEventReplay                      func([]interfaces.FactoryEvent, interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) ([]interfaces.FactoryEvent, error)
	EngineStateSnapshotCalls                int
	CreatedFactories                        []factoryapi.Factory
	SaveFactoryForSessionErr                error
	CurrentFactory                          *factoryapi.Factory
	CurrentFactoryErr                       error
	FactoryVersion                          factoryapi.HybridLogicalTimestamp
	CurrentFactoryReadErr                   error
	SavedCurrentFactories                   []factoryapi.Factory
	Models                                  factoryapi.ListModelsResponse
	ListModelsErr                           error
	ModelDetails                            map[string]factoryapi.ModelDetail
	GetModelErr                             error
	InvokedModels                           []factoryapi.ModelInvocationRequest
	InvokedModelNames                       []string
	InvokeModelResult                       modelinference.Result
	InvokeModelErr                          error
	InvokedFactorySessions                  []factoryapi.InvocationRequest
	InvokedFactorySessionIDs                []string
	InvokeFactoryResult                     apisurface.FactoryInvocationResult
	InvokeFactoryErr                        error
	PulledModelNames                        []string
	PullModelResult                         modelinference.PullResult
	PullModelErr                            error
	SessionFactories                        map[string]*MockFactory
	FactorySessions                         factoryapi.ListFactorySessionsResponse
	ListFactorySessionsErr                  error
	FactorySession                          factoryapi.FactorySession
	GetFactorySessionErr                    error
	FactorySessionSyncPreflight             factoryapi.FactorySessionSyncPreflightResponse
	GetFactorySessionSyncPreflightErr       error
	FactorySessionLiveResult                factoryapi.FactorySessionLiveResult
	GetFactorySessionResultErr              error
	FactorySessionPartialResult             factoryapi.FactorySessionPartialResult
	GetFactorySessionPartialResultErr       error
	OpenFactorySessionResult                factoryapi.OpenFactorySessionResponse
	OpenFactorySessionErr                   error
	OpenedFactorySessions                   []factoryapi.OpenFactorySessionRequest
	ClosedFactorySessions                   []string
	CloseFactorySessionErr                  error
	MoveWorkErr                             error
	AppliedOperatorMoveRequests             map[string]work.OperatorMoveResult
	DurableExecutionService                 factorysessions.ExecutionService
	FactorySessionRequestPreparation        FactorySessionRequestPreparation
	ListDurableFactorySessionDispatchesFunc func(
		context.Context,
		string,
		factoryapi.ListFactorySessionDispatchesParams,
	) (factoryapi.ListFactorySessionDispatchesResponse, error)
	PauseLiveFactorySessionFunc func(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error)
	ResumeLiveFactorySessionFunc func(
		context.Context,
		string,
		factorysessions.ControlRequest,
	) (factorysessions.LifecycleControlResult, error)
}

type FactorySessionRequestPreparation interface {
	PrepareStart(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	PrepareControl(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	PrepareApprove(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	PrepareRetryDispatch(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	PrepareListSessions(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	PrepareResult(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	PrepareEventReconnect(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

var _ petri.APIFactory = (*MockFactory)(nil)
var _ petri.Factory = (*MockFactory)(nil)
var _ apisurface.FactorySaveAPI = (*MockFactory)(nil)
var _ apisurface.SessionAPI = (*MockFactory)(nil)
var _ apisurface.WorkAPI = (*MockFactory)(nil)
var _ apisurface.InvocationAPI = (*MockFactory)(nil)
var _ apisurface.DurableSessionExecutionAPI = (*MockFactory)(nil)
var _ apisurface.DurableSessionLifecycleAPI = (*MockFactory)(nil)

func (m *MockFactory) StartDurableFactorySessionAsync(
	ctx context.Context,
	startReq factorysessions.StartRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	result, err := service.StartAsync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return factorysession.AsyncStartResponseToAPI(result), nil
}

func (m *MockFactory) CancelDurableFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.Cancel(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) TerminateDurableFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.Terminate(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) PauseDurableFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.Pause(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) ResumeDurableFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.Resume(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) ApproveDurableFactorySession(
	ctx context.Context,
	sessionID string,
	approve factorysessions.ApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.Approve(ctx, sessionID, approve)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) RetryDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	retry factorysessions.RetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.RetryDispatch(ctx, sessionID, retry)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	interrupt factorysessions.InterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.InterruptDispatch(ctx, sessionID, interrupt)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) ListDurableFactorySessions(
	ctx context.Context,
	req factorysessions.ListSessionsRequest,
) (factoryapi.ListFactorySessionsResponse, error) {
	result, err := m.ListDurableExecutionSessions(ctx, req)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ListSessionsResponseToAPI(result), nil
}

func (m *MockFactory) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	if m == nil || m.DurableExecutionService == nil {
		return factorysessions.ListSessionsResult{Scope: req.Scope}, nil
	}
	return m.DurableExecutionService.ListSessions(ctx, req)
}

// ListSessions implements the exact Factory Sessions execution root consumed
// by transport tests. ListDurableExecutionSessions remains the mapping-facing
// representation operation implemented by this programmable fake.
func (m *MockFactory) ListSessions(
	ctx context.Context,
	req factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	return m.ListDurableExecutionSessions(ctx, req)
}

func (m *MockFactory) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	result, err := service.GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return factorysession.SessionReadResponseToAPI(result), nil
}

func (m *MockFactory) GetDurableFactorySessionResult(
	ctx context.Context,
	sessionID string,
	req factorysessions.ResultRequest,
) (factoryapi.FactorySessionResult, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	result, err := service.GetResult(ctx, sessionID, req)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	return factorysession.ResultResponseToAPI(result), nil
}

func (m *MockFactory) ReadDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	reconnect factorysessions.EventReconnectRequest,
) (*interfaces.FactoryEventStream, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return nil, err
	}
	result, err := service.ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessions.ErrReconnectCursorNotFound) {
			return nil, apisurface.ErrInvalidEventReconnectCursor
		}
		return nil, err
	}
	return factorysessions.MaterializeEventReadStream(result), nil
}

func (m *MockFactory) ProbeDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	reconnect factorysessions.EventReconnectRequest,
) error {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return err
	}
	_, err = service.ReadEvents(ctx, sessionID, reconnect)
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		return apisurface.ErrFactorySessionNotFound
	}
	if errors.Is(err, factorysessions.ErrReconnectCursorNotFound) {
		return apisurface.ErrInvalidEventReconnectCursor
	}
	return err
}

func (m *MockFactory) StartDurableFactorySessionSync(
	ctx context.Context,
	startReq factorysessions.StartRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	result, err := service.StartSync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	return factorysession.SyncStartResponseToAPI(result), nil
}

func (m *MockFactory) ListDurableFactorySessionDispatches(
	ctx context.Context,
	sessionID string,
	params factoryapi.ListFactorySessionDispatchesParams,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	if m != nil && m.ListDurableFactorySessionDispatchesFunc != nil {
		return m.ListDurableFactorySessionDispatchesFunc(ctx, sessionID, params)
	}
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	result, err := service.ListDispatches(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	return factorysession.ListDispatchesResponseToAPI(result), nil
}

func (m *MockFactory) GetDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID, dispatchID string,
) (factoryapi.FactoryDispatch, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	result, err := service.GetDispatch(ctx, sessionID, dispatchID)
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	return factorysession.DispatchDetailResponseToAPI(result), nil
}

func (m *MockFactory) ListDurableFactorySessionArtifacts(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	result, err := service.ListArtifacts(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return factorysession.ListArtifactsResponseToAPI(result), nil
}

func (m *MockFactory) GetDurableFactorySessionArtifact(
	ctx context.Context,
	sessionID, artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	result, err := service.GetArtifact(ctx, sessionID, artifactID)
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	return factorysession.ArtifactDetailResponseToAPI(result), nil
}

func (m *MockFactory) requireDurableExecutionService() (factorysessions.ExecutionService, error) {
	if m == nil || m.DurableExecutionService == nil {
		return nil, errors.New("durable execution service is unavailable")
	}
	return m.DurableExecutionService, nil
}

func (m *MockFactory) Run(_ context.Context) error { return nil }

func (m *MockFactory) Pause(_ context.Context) error {
	if m == nil {
		return nil
	}
	m.State = interfaces.FactoryStatePaused
	if m.EngineState != nil {
		m.EngineState.FactoryState = string(interfaces.FactoryStatePaused)
	}
	return nil
}

func (m *MockFactory) Resume(_ context.Context) error {
	if m == nil {
		return nil
	}
	m.State = interfaces.FactoryStateRunning
	if m.EngineState != nil {
		m.EngineState.FactoryState = string(interfaces.FactoryStateRunning)
	}
	return nil
}

func (m *MockFactory) MoveWork(_ context.Context, workID, stateName string, _ work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	if m.MoveWorkErr != nil {
		return work.OperatorMoveResult{}, m.MoveWorkErr
	}
	return m.applyMockOperatorMove(workID, stateName, requestID)
}

func (m *MockFactory) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return session.MoveWork(ctx, workID, stateName, work.WorkStateChangeSourceAPI, requestID)
}

func (m *MockFactory) applyMockOperatorMove(workID, stateName, requestID string) (work.OperatorMoveResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		if m.AppliedOperatorMoveRequests == nil {
			m.AppliedOperatorMoveRequests = make(map[string]work.OperatorMoveResult)
		}
		if _, ok := m.AppliedOperatorMoveRequests[requestID]; ok {
			return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
		}
	}
	if m.Marking == nil || m.Marking.Tokens == nil {
		return work.OperatorMoveResult{}, petri.ErrMoveWorkNotFound
	}
	token, ok := findMockWorkToken(m.Marking.Tokens, workID)
	if !ok {
		return work.OperatorMoveResult{}, petri.ErrMoveWorkNotFound
	}
	if m.Net == nil {
		return work.OperatorMoveResult{}, petri.ErrMoveWorkInvalidState
	}
	toPlaceID := petri.PlaceID(token.Color.WorkTypeID, stateName)
	place, ok := m.Net.Places[toPlaceID]
	if !ok || place.State != stateName {
		return work.OperatorMoveResult{}, petri.ErrMoveWorkInvalidState
	}
	fromPlaceID := token.PlaceID
	fromState := ""
	if fromPlace, ok := m.Net.Places[fromPlaceID]; ok {
		fromState = fromPlace.State
	}
	token.PlaceID = toPlaceID
	result := work.OperatorMoveResult{
		WorkID:      workID,
		WorkTypeID:  token.Color.WorkTypeID,
		FromState:   fromState,
		ToState:     stateName,
		FromPlaceID: fromPlaceID,
		ToPlaceID:   toPlaceID,
		TokenID:     token.ID,
	}
	if requestID != "" {
		m.AppliedOperatorMoveRequests[requestID] = result
	}
	return result, nil
}

func findMockWorkToken(tokens map[string]*petri.RuntimeToken, workID string) (*petri.RuntimeToken, bool) {
	for _, token := range tokens {
		if token == nil || token.Color.WorkID != workID {
			continue
		}
		if token.Color.DataType == petri.RuntimeTokenDataTypeResource {
			continue
		}
		return token, true
	}
	return nil, false
}

func (m *MockFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if m.SubmitWorkRequestErr != nil {
		return work.WorkRequestSubmitResult{}, m.SubmitWorkRequestErr
	}
	if m.SubmitErr != nil {
		return work.WorkRequestSubmitResult{}, m.SubmitErr
	}
	if existing, ok := m.acceptedWorkRequest(request.RequestID); ok {
		existing.Accepted = false
		return existing, nil
	}
	requestID := request.RequestID
	result := m.SubmitWorkRequestResult
	if result.RequestID == "" && result.TraceID == "" && result.WorkID == "" && result.Name == "" &&
		result.WorkTypeName == "" && !result.Accepted && len(result.Works) == 0 {
		result = work.WorkRequestSubmitResult{RequestID: requestID, Accepted: true}
		for _, item := range request.Works {
			result.Works = append(result.Works, work.WorkRequestSubmittedWork{
				Name: item.Name, WorkTypeName: item.WorkTypeID, WorkID: item.WorkID,
			})
		}
		if len(request.Works) > 0 {
			result.WorkID = request.Works[0].WorkID
			result.Name = request.Works[0].Name
			result.WorkTypeName = request.Works[0].WorkTypeID
			result.TraceID = request.Works[0].CurrentChainingTraceID
			if result.TraceID == "" {
				result.TraceID = request.Works[0].TraceID
			}
		}
	}
	if m.WorkRequestResults == nil {
		m.WorkRequestResults = make(map[string]work.WorkRequestSubmitResult)
	}
	m.WorkRequestResults[requestID] = result
	m.WorkRequests = append(m.WorkRequests, request)
	return result, nil
}

func (m *MockFactory) acceptedWorkRequest(requestID string) (work.WorkRequestSubmitResult, bool) {
	if requestID == "" {
		return work.WorkRequestSubmitResult{}, false
	}
	if result, ok := m.WorkRequestResults[requestID]; ok {
		return result, true
	}
	return work.WorkRequestSubmitResult{}, false
}

func (m *MockFactory) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	m.FactoryEventStreamCtx = ctx
	history := append([]factoryapi.FactoryEvent(nil), m.FactoryEvents...)
	if m.FactoryEventStream != nil && len(m.FactoryEventStream.History) > 0 {
		history = make([]factoryapi.FactoryEvent, 0, len(m.FactoryEventStream.History))
		for _, event := range m.FactoryEventStream.History {
			var generated factoryapi.FactoryEvent
			if err := event.Decode(&generated); err != nil {
				return nil, err
			}
			history = append(history, generated)
		}
	}
	domainHistory := make([]interfaces.FactoryEvent, 0, len(history))
	for _, event := range history {
		domainEvent, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			return nil, err
		}
		domainHistory = append(domainHistory, domainEvent)
	}
	if reconnect != nil {
		if m.FactoryEventReplay == nil {
			return nil, fmt.Errorf("factory event reconnect script is unavailable")
		}
		replayed, err := m.FactoryEventReplay(domainHistory, *reconnect, scope)
		if err != nil {
			return nil, err
		}
		domainHistory = replayed
	}
	if m.FactoryEventStream != nil {
		return &interfaces.FactoryEventStream{
			BackendScopeID:      m.FactoryEventStream.BackendScopeID,
			LogicalSessionKeyID: m.FactoryEventStream.LogicalSessionKeyID,
			FactorySessionID:    m.FactoryEventStream.FactorySessionID,
			StreamGenerationID:  m.FactoryEventStream.StreamGenerationID,
			History:             domainHistory,
			Events:              m.FactoryEventStream.Events,
		}, nil
	}
	ch := make(chan interfaces.FactoryEvent)
	return &interfaces.FactoryEventStream{History: domainHistory, Events: ch}, nil
}

func (m *MockFactory) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net], error) {
	m.EngineStateSnapshotCalls++
	if m.EngineStateSnapshotErr != nil {
		return nil, m.EngineStateSnapshotErr
	}
	if m.EngineState != nil {
		return m.EngineState, nil
	}
	runtimeState := interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net]{}
	if m.Marking != nil {
		runtimeState.Marking = *m.Marking
	}
	snap := petri.NewEngineStateSnapshot(runtimeState, string(m.State), m.Uptime, m.Net)
	return &snap, nil
}

func (m *MockFactory) GetFactoryEvents(_ context.Context) ([]interfaces.FactoryEvent, error) {
	events := make([]interfaces.FactoryEvent, 0, len(m.FactoryEvents))
	for _, event := range m.FactoryEvents {
		canonical, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			return nil, err
		}
		events = append(events, canonical)
	}
	return events, nil
}

func (m *MockFactory) GetCurrentFactory(_ context.Context) (factoryapi.Factory, error) {
	if m.CurrentFactoryErr != nil {
		return factoryapi.Factory{}, m.CurrentFactoryErr
	}
	if m.CurrentFactory == nil {
		return factoryapi.Factory{}, errors.New("current factory not found")
	}
	if m.CurrentFactoryReadErr != nil {
		return factoryapi.Factory{}, m.CurrentFactoryReadErr
	}
	current := *m.CurrentFactory
	version := m.FactoryVersion
	if version.Physical.IsZero() {
		version.Physical = time.Unix(0, 1).UTC()
		version.Logical = 1
	}
	current.Version = &version
	return current, nil
}

func (m *MockFactory) SaveFactoryForSession(
	_ context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if m.SaveFactoryForSessionErr != nil {
		return factoryapi.Factory{}, m.SaveFactoryForSessionErr
	}
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if mode == factoryapi.FactorySaveModeUpsertNamedAndActivate {
		m.CreatedFactories = append(m.CreatedFactories, request)
	}
	session.SavedCurrentFactories = append(session.SavedCurrentFactories, request)
	copied := request
	session.CurrentFactory = &copied
	version := session.FactoryVersion
	if version.Physical.IsZero() {
		version.Physical = time.Unix(0, 2).UTC()
		version.Logical = 2
	}
	copied.Version = &version
	return copied, nil
}

func (m *MockFactory) ListModels(_ context.Context) (factoryapi.ListModelsResponse, error) {
	if m.ListModelsErr != nil {
		return factoryapi.ListModelsResponse{}, m.ListModelsErr
	}
	return m.Models, nil
}

func (m *MockFactory) GetModel(_ context.Context, modelName string) (factoryapi.ModelDetail, error) {
	if m.GetModelErr != nil {
		return factoryapi.ModelDetail{}, m.GetModelErr
	}
	if m.ModelDetails == nil {
		return factoryapi.ModelDetail{}, modelinference.ErrNotFound
	}
	model, ok := m.ModelDetails[modelName]
	if !ok {
		return factoryapi.ModelDetail{}, modelinference.ErrNotFound
	}
	return model, nil
}

func (m *MockFactory) InvokeModel(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
	if m.InvokeModelErr != nil {
		return modelinference.Result{}, m.InvokeModelErr
	}
	m.InvokedModelNames = append(m.InvokedModelNames, modelName)
	m.InvokedModels = append(m.InvokedModels, request)
	return m.InvokeModelResult, nil
}

func (m *MockFactory) PullModel(_ context.Context, modelName string) (modelinference.PullResult, error) {
	if m.PullModelErr != nil {
		return modelinference.PullResult{}, m.PullModelErr
	}
	m.PulledModelNames = append(m.PulledModelNames, modelName)
	return m.PullModelResult, nil
}

func (m *MockFactory) ListFactorySessions(_ context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	if m.ListFactorySessionsErr != nil {
		return factoryapi.ListFactorySessionsResponse{}, m.ListFactorySessionsErr
	}
	return m.FactorySessions, nil
}

func (m *MockFactory) GetFactorySession(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if m.GetFactorySessionErr != nil {
		return factoryapi.FactorySession{}, m.GetFactorySessionErr
	}
	if session, err := m.sessionFactory(sessionID); err == nil && session != nil {
		return session.FactorySession, nil
	}
	return m.FactorySession, nil
}

func (m *MockFactory) GetFactorySessionSyncPreflight(
	_ context.Context,
	_ string,
	_ interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	if m.GetFactorySessionSyncPreflightErr != nil {
		return factoryapi.FactorySessionSyncPreflightResponse{}, m.GetFactorySessionSyncPreflightErr
	}
	return m.FactorySessionSyncPreflight, nil
}

func (m *MockFactory) GetFactorySessionResult(_ context.Context, _ string) (factoryapi.FactorySessionLiveResult, error) {
	if m.GetFactorySessionResultErr != nil {
		return factoryapi.FactorySessionLiveResult{}, m.GetFactorySessionResultErr
	}
	return m.FactorySessionLiveResult, nil
}

func (m *MockFactory) GetFactorySessionPartialResult(_ context.Context, _ string) (factoryapi.FactorySessionPartialResult, error) {
	if m.GetFactorySessionPartialResultErr != nil {
		return factoryapi.FactorySessionPartialResult{}, m.GetFactorySessionPartialResultErr
	}
	return m.FactorySessionPartialResult, nil
}

func (m *MockFactory) OpenFactorySession(_ context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	if m.OpenFactorySessionErr != nil {
		return factoryapi.OpenFactorySessionResponse{}, m.OpenFactorySessionErr
	}
	m.OpenedFactorySessions = append(m.OpenedFactorySessions, request)
	return m.OpenFactorySessionResult, nil
}

func (m *MockFactory) CloseFactorySession(_ context.Context, sessionID string) error {
	if m.CloseFactorySessionErr != nil {
		return m.CloseFactorySessionErr
	}
	m.ClosedFactorySessions = append(m.ClosedFactorySessions, sessionID)
	return nil
}

func (m *MockFactory) PauseLiveFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if m.SessionFactories != nil {
		if sessionFactory, ok := m.SessionFactories[sessionID]; ok {
			return sessionFactory.PauseLiveFactorySession(ctx, sessionID, control)
		}
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if _, err := m.GetFactorySession(ctx, sessionID); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	if m.PauseLiveFactorySessionFunc == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, errors.New("live pause script is unavailable")
	}
	result, err := m.PauseLiveFactorySessionFunc(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	control factorysessions.ControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if m.SessionFactories != nil {
		if sessionFactory, ok := m.SessionFactories[sessionID]; ok {
			return sessionFactory.ResumeLiveFactorySession(ctx, sessionID, control)
		}
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if _, err := m.GetFactorySession(ctx, sessionID); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	if m.ResumeLiveFactorySessionFunc == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, errors.New("live resume script is unavailable")
	}
	result, err := m.ResumeLiveFactorySessionFunc(ctx, sessionID, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}
