package testutil

import (
	"context"
	"errors"
	"fmt"
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

type durableExecutionService interface {
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error)
	ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error)
	GetSession(context.Context, string) (factorysessions.SessionReadResult, error)
	Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error)
	RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error)
	InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error)
	GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error)
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error)
	ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error)
	GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error)
	ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
}

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
	DurableExecutionService                 durableExecutionService
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
	snap := interfaces.EngineStateSnapshot[petri.PetriMarkingSnapshot, *petri.Net]{
		RuntimeStatus:        runtimeState.RuntimeStatus,
		StreamGenerationID:   runtimeState.StreamGenerationID,
		Marking:              runtimeState.Marking,
		Dispatches:           runtimeState.Dispatches,
		InFlightCount:        runtimeState.InFlightCount,
		Results:              runtimeState.Results,
		DispatchHistory:      runtimeState.DispatchHistory,
		ActiveThrottlePauses: runtimeState.ActiveThrottlePauses,
		TickCount:            runtimeState.TickCount,
		FactoryState:         string(m.State),
		Uptime:               m.Uptime,
		Topology:             m.Net,
	}
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

func (m *MockFactory) GetFactorySession(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
	if m.GetFactorySessionErr != nil {
		return factoryapi.FactorySession{}, m.GetFactorySessionErr
	}
	if session, err := m.sessionFactory(sessionID); err == nil && session != nil {
		return session.FactorySession, nil
	}
	return m.FactorySession, nil
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
