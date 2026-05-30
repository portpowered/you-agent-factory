package testutil

import (
	"context"
	"errors"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
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
	PulledModelNames         []string
	PullModelResult          apisurface.ModelPullResult
	PullModelErr             error
	SessionFactories         map[string]*MockFactory
	FactorySessions          factoryapi.ListFactorySessionsResponse
	ListFactorySessionsErr   error
	OpenFactorySessionResult factoryapi.OpenFactorySessionResponse
	OpenFactorySessionErr    error
	OpenedFactorySessions    []factoryapi.OpenFactorySessionRequest
	ClosedFactorySessions    []string
	CloseFactorySessionErr   error
}

var _ factory.APIFactory = (*MockFactory)(nil)
var _ factory.Factory = (*MockFactory)(nil)
var _ apisurface.APISurface = (*MockFactory)(nil)
var _ apisurface.SessionAPISurface = (*MockFactory)(nil)

func (m *MockFactory) Run(_ context.Context) error   { return nil }
func (m *MockFactory) Pause(_ context.Context) error { return nil }

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

func (m *MockFactory) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	m.FactoryEventStreamCtx = ctx
	if m.FactoryEventStream != nil {
		return m.FactoryEventStream, nil
	}
	ch := make(chan factoryapi.FactoryEvent)
	return &interfaces.FactoryEventStream{History: m.FactoryEvents, Events: ch}, nil
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

func (m *MockFactory) SubscribeFactoryEventsForSession(ctx context.Context, sessionID string) (*interfaces.FactoryEventStream, error) {
	session, err := m.sessionFactory(sessionID)
	if err != nil {
		return nil, err
	}
	return session.SubscribeFactoryEvents(ctx)
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
