package testutil

import (
	"context"
	"errors"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type MockFactory struct {
	Submitted                []interfaces.SubmitRequest
	SubmitErr                error
	WorkRequests             []interfaces.WorkRequest
	SubmitWorkRequestErr     error
	WorkRequestResults       map[string]interfaces.WorkRequestSubmitResult
	Marking                  *petri.MarkingSnapshot
	State                    interfaces.FactoryState
	Net                      *state.Net
	EngineState              *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	EngineStateSnapshotErr   error
	Uptime                   time.Duration
	FactoryEvents            []factoryapi.FactoryEvent
	FactoryEventStream       *interfaces.FactoryEventStream
	FactoryEventStreamCtx    context.Context
	EngineStateSnapshotCalls int
	CreatedFactories         []factoryapi.Factory
	SaveFactoryForSessionErr error
	CurrentFactory           *factoryapi.Factory
	CurrentFactoryErr        error
	FactoryVersion           factoryapi.HybridLogicalTimestamp
	CurrentFactoryReadErr    error
	SavedCurrentFactories    []factoryapi.Factory
	Models                   factoryapi.ListModelsResponse
	ListModelsErr            error
	ModelDetails             map[string]factoryapi.ModelDetail
	GetModelErr              error
	InvokedModels            []factoryapi.ModelInvocationRequest
	InvokedModelNames        []string
	InvokeModelResult        apisurface.ModelInvocationResult
	InvokeModelErr           error
	InvokedFactorySessions   []factoryapi.InvocationRequest
	InvokedFactorySessionIDs []string
	InvokeFactoryResult      apisurface.FactoryInvocationResult
	InvokeFactoryErr         error
	PulledModelNames         []string
	PullModelResult          apisurface.ModelPullResult
	PullModelErr             error
	SessionFactories         map[string]*MockFactory
	FactorySessions          factoryapi.ListFactorySessionsResponse
	ListFactorySessionsErr   error
	FactorySession               factoryapi.FactorySession
	GetFactorySessionErr         error
	FactorySessionLiveResult     factoryapi.FactorySessionLiveResult
	GetFactorySessionResultErr   error
	FactorySessionPartialResult  factoryapi.FactorySessionPartialResult
	GetFactorySessionPartialResultErr error
	OpenFactorySessionResult factoryapi.OpenFactorySessionResponse
	OpenFactorySessionErr    error
	OpenedFactorySessions    []factoryapi.OpenFactorySessionRequest
	ClosedFactorySessions    []string
	CloseFactorySessionErr      error
	MoveWorkErr                 error
	AppliedOperatorMoveRequests map[string]interfaces.OperatorMoveResult
	DurableExecutionService     factorysessionexecution.Service
}

var _ factory.APIFactory = (*MockFactory)(nil)
var _ factory.Factory = (*MockFactory)(nil)
var _ apisurface.ModelAPI = (*MockFactory)(nil)
var _ apisurface.FactorySaveAPI = (*MockFactory)(nil)
var _ apisurface.SessionAPI = (*MockFactory)(nil)
var _ apisurface.WorkAPI = (*MockFactory)(nil)
var _ apisurface.InvocationAPI = (*MockFactory)(nil)
var _ apisurface.APISurface = (*MockFactory)(nil)
var _ apisurface.SessionAPISurface = (*MockFactory)(nil)
var _ apisurface.DurableSessionExecutionAPI = (*MockFactory)(nil)
var _ apisurface.DurableSessionLifecycleAPI = (*MockFactory)(nil)

func (m *MockFactory) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	startReq, err := factorysession.StartRequestFromAPI(request)
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
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
	request factoryapi.FactorySessionApproveRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	approve, err := factorysession.ApproveRequestFromAPI(request)
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
	request factoryapi.FactorySessionRetryDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	retry, err := factorysession.RetryDispatchRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := service.RetryDispatch(ctx, sessionID, retry)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
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

func (m *MockFactory) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	return factoryapi.FactorySessionSyncExecutionResponse{}, errors.New("sync durable factory session start is not implemented")
}

func (m *MockFactory) requireDurableExecutionService() (factorysessionexecution.Service, error) {
	if m == nil || m.DurableExecutionService == nil {
		return nil, errors.New("durable execution service is unavailable")
	}
	return m.DurableExecutionService, nil
}

func (m *MockFactory) Run(_ context.Context) error   { return nil }
func (m *MockFactory) Pause(_ context.Context) error { return nil }

func (m *MockFactory) MoveWork(_ context.Context, workID, stateName string, _ interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	if m.MoveWorkErr != nil {
		return interfaces.OperatorMoveResult{}, m.MoveWorkErr
	}
	return m.applyMockOperatorMove(workID, stateName, requestID)
}

func (m *MockFactory) MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return interfaces.OperatorMoveResult{}, err
	}
	return session.MoveWork(ctx, workID, stateName, interfaces.WorkStateChangeSourceAPI, requestID)
}

func (m *MockFactory) applyMockOperatorMove(workID, stateName, requestID string) (interfaces.OperatorMoveResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		if m.AppliedOperatorMoveRequests == nil {
			m.AppliedOperatorMoveRequests = make(map[string]interfaces.OperatorMoveResult)
		}
		if _, ok := m.AppliedOperatorMoveRequests[requestID]; ok {
			return interfaces.OperatorMoveResult{}, interfaces.ErrMoveWorkRequestAlreadyApplied
		}
	}
	if m.Marking == nil || m.Marking.Tokens == nil {
		return interfaces.OperatorMoveResult{}, engine.ErrMoveWorkNotFound
	}
	token, ok := findMockWorkToken(m.Marking.Tokens, workID)
	if !ok {
		return interfaces.OperatorMoveResult{}, engine.ErrMoveWorkNotFound
	}
	if m.Net == nil {
		return interfaces.OperatorMoveResult{}, engine.ErrMoveWorkInvalidState
	}
	toPlaceID := state.PlaceID(token.Color.WorkTypeID, stateName)
	place, ok := m.Net.Places[toPlaceID]
	if !ok || place.State != stateName {
		return interfaces.OperatorMoveResult{}, engine.ErrMoveWorkInvalidState
	}
	fromPlaceID := token.PlaceID
	fromState := ""
	if fromPlace, ok := m.Net.Places[fromPlaceID]; ok {
		fromState = fromPlace.State
	}
	token.PlaceID = toPlaceID
	result := interfaces.OperatorMoveResult{
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

func findMockWorkToken(tokens map[string]*interfaces.Token, workID string) (*interfaces.Token, bool) {
	for _, token := range tokens {
		if token == nil || token.Color.WorkID != workID {
			continue
		}
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		return token, true
	}
	return nil, false
}

func (m *MockFactory) SubmitWorkRequest(_ context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	if m.SubmitWorkRequestErr != nil {
		return interfaces.WorkRequestSubmitResult{}, m.SubmitWorkRequestErr
	}
	if m.SubmitErr != nil {
		return interfaces.WorkRequestSubmitResult{}, m.SubmitErr
	}
	if existing, ok := m.acceptedWorkRequest(request.RequestID); ok {
		existing.Accepted = false
		return existing, nil
	}
	opts := interfaces.WorkRequestNormalizeOptions{}
	if m.Net != nil {
		opts.ValidWorkTypes = make(map[string]bool, len(m.Net.WorkTypes))
		for workTypeID := range m.Net.WorkTypes {
			opts.ValidWorkTypes[workTypeID] = true
		}
		opts.ValidStatesByType = state.ValidStatesByType(m.Net.WorkTypes)
	}
	normalized, err := requests.NormalizeWorkRequest(request, opts)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	requestID := request.RequestID
	if requestID == "" && len(normalized) > 0 {
		requestID = normalized[0].RequestID
	}
	result := requests.WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
	if m.WorkRequestResults == nil {
		m.WorkRequestResults = make(map[string]interfaces.WorkRequestSubmitResult)
	}
	m.WorkRequestResults[requestID] = result
	m.WorkRequests = append(m.WorkRequests, request)
	m.Submitted = append(m.Submitted, normalized...)
	return result, nil
}

func (m *MockFactory) acceptedWorkRequest(requestID string) (interfaces.WorkRequestSubmitResult, bool) {
	if requestID == "" {
		return interfaces.WorkRequestSubmitResult{}, false
	}
	if result, ok := m.WorkRequestResults[requestID]; ok {
		return result, true
	}
	return interfaces.WorkRequestSubmitResult{}, false
}

func (m *MockFactory) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	m.FactoryEventStreamCtx = ctx
	history := m.FactoryEvents
	if m.FactoryEventStream != nil && len(m.FactoryEventStream.History) > 0 {
		history = m.FactoryEventStream.History
	}
	if reconnect != nil {
		replayed, err := factoryevents.BuildReconnectReplay(history, *reconnect, scope)
		if err != nil {
			return nil, err
		}
		history = replayed
	}
	if m.FactoryEventStream != nil {
		return &interfaces.FactoryEventStream{History: history, Events: m.FactoryEventStream.Events}, nil
	}
	ch := make(chan factoryapi.FactoryEvent)
	return &interfaces.FactoryEventStream{History: history, Events: ch}, nil
}

func (m *MockFactory) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	m.EngineStateSnapshotCalls++
	if m.EngineStateSnapshotErr != nil {
		return nil, m.EngineStateSnapshotErr
	}
	if m.EngineState != nil {
		return m.EngineState, nil
	}
	runtimeState := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}
	if m.Marking != nil {
		runtimeState.Marking = *m.Marking
	}
	snap := state.NewEngineStateSnapshot(runtimeState, string(m.State), m.Uptime, m.Net)
	return &snap, nil
}

func (m *MockFactory) GetFactoryEvents(_ context.Context) ([]factoryapi.FactoryEvent, error) {
	events := make([]factoryapi.FactoryEvent, len(m.FactoryEvents))
	copy(events, m.FactoryEvents)
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
		return factoryapi.ModelDetail{}, apisurface.ErrModelNotFound
	}
	model, ok := m.ModelDetails[modelName]
	if !ok {
		return factoryapi.ModelDetail{}, apisurface.ErrModelNotFound
	}
	return model, nil
}

func (m *MockFactory) InvokeModel(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	if m.InvokeModelErr != nil {
		return apisurface.ModelInvocationResult{}, m.InvokeModelErr
	}
	m.InvokedModelNames = append(m.InvokedModelNames, modelName)
	m.InvokedModels = append(m.InvokedModels, request)
	return m.InvokeModelResult, nil
}

func (m *MockFactory) PullModel(_ context.Context, modelName string) (apisurface.ModelPullResult, error) {
	if m.PullModelErr != nil {
		return apisurface.ModelPullResult{}, m.PullModelErr
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

func (m *MockFactory) GetFactorySession(_ context.Context, _ string) (factoryapi.FactorySession, error) {
	if m.GetFactorySessionErr != nil {
		return factoryapi.FactorySession{}, m.GetFactorySessionErr
	}
	return m.FactorySession, nil
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

func (m *MockFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}

func (m *MockFactory) SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	return session.SubmitWorkRequest(ctx, request)
}

func (m *MockFactory) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	return session.SubscribeFactoryEvents(ctx, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
}

func (m *MockFactory) GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	return session.GetEngineStateSnapshot(ctx)
}

func (m *MockFactory) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return session.GetCurrentFactory(ctx)
}

func (m *MockFactory) InvokeFactorySession(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	if m.InvokeFactoryErr != nil {
		return apisurface.FactoryInvocationResult{}, m.InvokeFactoryErr
	}
	if _, err := m.sessionFactory(sessionID); err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	m.InvokedFactorySessionIDs = append(m.InvokedFactorySessionIDs, sessionID)
	m.InvokedFactorySessions = append(m.InvokedFactorySessions, request)
	return m.InvokeFactoryResult, nil
}

func (m *MockFactory) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return m.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

func (m *MockFactory) sessionFactory(sessionID string) (*MockFactory, error) {
	if m == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	if sessionID == "" || sessionID == "~default" {
		if m.SessionFactories == nil {
			return m, nil
		}
		if session, ok := m.SessionFactories["~default"]; ok && session != nil {
			return session, nil
		}
		return m, nil
	}
	if m.SessionFactories == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	session, ok := m.SessionFactories[sessionID]
	if !ok || session == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	return session, nil
}
