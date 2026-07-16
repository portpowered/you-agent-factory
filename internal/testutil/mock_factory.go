package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

type MockFactory struct {
	Submitted                         []work.SubmitRequest
	SubmitErr                         error
	WorkRequests                      []work.WorkRequest
	SubmitWorkRequestErr              error
	WorkRequestResults                map[string]work.WorkRequestSubmitResult
	Marking                           *petri.MarkingSnapshot
	State                             interfaces.FactoryState
	Net                               *state.Net
	EngineState                       *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	EngineStateSnapshotErr            error
	Uptime                            time.Duration
	FactoryEvents                     []factoryapi.FactoryEvent
	FactoryEventStream                *interfaces.FactoryEventStream
	FactoryEventStreamCtx             context.Context
	ResponseEventStore                *responseeventstore.SessionResponseEventStore
	ResponseEventSubscribeErr         error
	EngineStateSnapshotCalls          int
	CreatedFactories                  []factoryapi.Factory
	SaveFactoryForSessionErr          error
	CurrentFactory                    *factoryapi.Factory
	CurrentFactoryErr                 error
	FactoryVersion                    factoryapi.HybridLogicalTimestamp
	CurrentFactoryReadErr             error
	SavedCurrentFactories             []factoryapi.Factory
	Models                            factoryapi.ListModelsResponse
	ListModelsErr                     error
	ModelDetails                      map[string]factoryapi.ModelDetail
	GetModelErr                       error
	InvokedModels                     []factoryapi.ModelInvocationRequest
	InvokedModelNames                 []string
	InvokeModelResult                 apisurface.ModelInvocationResult
	InvokeModelErr                    error
	InvokedFactorySessions            []factoryapi.InvocationRequest
	InvokedFactorySessionIDs          []string
	InvokeFactoryResult               apisurface.FactoryInvocationResult
	InvokeFactoryErr                  error
	PulledModelNames                  []string
	PullModelResult                   apisurface.ModelPullResult
	PullModelErr                      error
	SessionFactories                  map[string]*MockFactory
	FactorySessions                   factoryapi.ListFactorySessionsResponse
	ListFactorySessionsErr            error
	FactorySession                    factoryapi.FactorySession
	GetFactorySessionErr              error
	FactorySessionSyncPreflight       factoryapi.FactorySessionSyncPreflightResponse
	GetFactorySessionSyncPreflightErr error
	FactorySessionLiveResult          factoryapi.FactorySessionLiveResult
	GetFactorySessionResultErr        error
	FactorySessionPartialResult       factoryapi.FactorySessionPartialResult
	GetFactorySessionPartialResultErr error
	OpenFactorySessionResult          factoryapi.OpenFactorySessionResponse
	OpenFactorySessionErr             error
	OpenedFactorySessions             []factoryapi.OpenFactorySessionRequest
	ClosedFactorySessions             []string
	CloseFactorySessionErr            error
	MoveWorkErr                       error
	AppliedOperatorMoveRequests       map[string]work.OperatorMoveResult
	DurableExecutionService           factorysessionexecution.Service
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

func (m *MockFactory) InterruptDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionInterruptDispatchRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	interrupt, err := factorysession.InterruptDispatchRequestFromAPI(request)
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
	params factoryapi.ListFactorySessionsParams,
) (factoryapi.ListFactorySessionsResponse, error) {
	req, err := factorysession.ListSessionsRequestFromAPI(params)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	result, err := m.ListDurableExecutionSessions(ctx, req)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ListSessionsResponseToAPI(result), nil
}

func (m *MockFactory) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessionexecution.ListSessionsRequest,
) (factorysessionexecution.ListSessionsResult, error) {
	if m == nil || m.DurableExecutionService == nil {
		return factorysessionexecution.ListSessionsResult{Scope: req.Scope}, nil
	}
	return m.DurableExecutionService.ListSessions(ctx, req)
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
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	req, err := factorysession.ResultRequestFromAPI(params)
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
	params factoryapi.GetEventsBySessionIdParams,
) (*interfaces.FactoryEventStream, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return nil, err
	}
	reconnect, err := factorysession.EventReconnectRequestFromAPI(params)
	if err != nil {
		return nil, err
	}
	result, err := service.ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
			return nil, apisurface.ErrInvalidEventReconnectCursor
		}
		return nil, err
	}
	return factorysession.FactoryEventStreamFromReadResult(result), nil
}

func (m *MockFactory) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	startReq, err := factorysession.StartRequestFromAPI(request)
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
	service, err := m.requireDurableExecutionService()
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	result, err := service.ListDispatches(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	filters := factorysessionexecution.DispatchFilters{}
	if params.Phase != nil {
		filters.Phase = string(*params.Phase)
	}
	if params.Status != nil {
		filters.Status = factorysessionexecution.DispatchStatus(*params.Status)
	}
	result, err = factorysessionexecution.FilterDispatches(result, filters)
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

func (m *MockFactory) requireDurableExecutionService() (factorysessionexecution.Service, error) {
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
		return work.OperatorMoveResult{}, engine.ErrMoveWorkNotFound
	}
	token, ok := findMockWorkToken(m.Marking.Tokens, workID)
	if !ok {
		return work.OperatorMoveResult{}, engine.ErrMoveWorkNotFound
	}
	if m.Net == nil {
		return work.OperatorMoveResult{}, engine.ErrMoveWorkInvalidState
	}
	toPlaceID := state.PlaceID(token.Color.WorkTypeID, stateName)
	place, ok := m.Net.Places[toPlaceID]
	if !ok || place.State != stateName {
		return work.OperatorMoveResult{}, engine.ErrMoveWorkInvalidState
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

func findMockWorkToken(tokens map[string]*factorytoken.Token, workID string) (*factorytoken.Token, bool) {
	for _, token := range tokens {
		if token == nil || token.Color.WorkID != workID {
			continue
		}
		if token.Color.DataType == factorytoken.DataTypeResource {
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
	opts := work.WorkRequestNormalizeOptions{}
	if m.Net != nil {
		opts.ValidWorkTypes = make(map[string]bool, len(m.Net.WorkTypes))
		for workTypeID := range m.Net.WorkTypes {
			opts.ValidWorkTypes[workTypeID] = true
		}
		opts.ValidStatesByType = state.ValidStatesByType(m.Net.WorkTypes)
	}
	normalized, err := requests.NormalizeWorkRequest(request, opts)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	requestID := request.RequestID
	if requestID == "" && len(normalized) > 0 {
		requestID = normalized[0].RequestID
	}
	result := requests.WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
	if m.WorkRequestResults == nil {
		m.WorkRequestResults = make(map[string]work.WorkRequestSubmitResult)
	}
	m.WorkRequestResults[requestID] = result
	m.WorkRequests = append(m.WorkRequests, request)
	m.Submitted = append(m.Submitted, normalized...)
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
		replayed, err := factoryevents.BuildCanonicalReconnectReplay(domainHistory, *reconnect, scope)
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
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if m.SessionFactories != nil {
		if sessionFactory, ok := m.SessionFactories[sessionID]; ok {
			return sessionFactory.PauseLiveFactorySession(ctx, sessionID, request)
		}
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if _, err := m.GetFactorySession(ctx, sessionID); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := mockLiveLifecycleControl(m, sessionID, factorysessionexecution.LifecycleControlPause, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func (m *MockFactory) ResumeLiveFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.FactorySessionLifecycleControlRequest,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if m.SessionFactories != nil {
		if sessionFactory, ok := m.SessionFactories[sessionID]; ok {
			return sessionFactory.ResumeLiveFactorySession(ctx, sessionID, request)
		}
		return factoryapi.FactorySessionLifecycleControlResponse{}, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}
	if _, err := m.GetFactorySession(ctx, sessionID); err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	control, err := factorysession.ControlRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	result, err := mockLiveLifecycleControl(m, sessionID, factorysessionexecution.LifecycleControlResume, control)
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return factorysession.LifecycleControlResponseToAPI(result), nil
}

func mockLiveLifecycleControl(
	m *MockFactory,
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	control factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	if _, err := factorysessionexecution.NormalizeControlRequest(control); err != nil {
		return factorysessionexecution.LifecycleControlResult{}, err
	}
	currentStatus := factorysessionexecution.LifecycleStatusFromFactoryRuntimeState(string(m.State))
	outcome := factorysessionexecution.EvaluateLifecycleControl(operation, currentStatus)
	if outcome == factorysessionexecution.LifecycleControlOutcomeInvalidState ||
		outcome == factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		return factorysessionexecution.LifecycleControlResult{}, &factorysessionexecution.ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message:   string(operation) + " rejected for session " + sessionID,
		}
	}
	resultStatus := currentStatus
	if outcome == factorysessionexecution.LifecycleControlOutcomeAccepted {
		switch operation {
		case factorysessionexecution.LifecycleControlPause:
			_ = m.Pause(context.Background())
			resultStatus = factorysessionexecution.LifecycleStatusPaused
		case factorysessionexecution.LifecycleControlResume:
			_ = m.Resume(context.Background())
			resultStatus = factorysessionexecution.LifecycleStatusRunning
		}
	}
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    resultStatus,
		Links:     factorysessionexecution.LiveLifecycleControlLinksForSession(sessionID),
	}, nil
}
