package testutil

import (
	"context"
	"errors"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
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
	CreateNamedFactoryErr    error
	CurrentNamedFactory      *factoryapi.Factory
	CurrentNamedFactoryErr   error
}

var _ factory.Factory = (*MockFactory)(nil)

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
	normalized, err := factory.NormalizeWorkRequest(request, opts)
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	result := interfaces.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
	}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	if m.WorkRequestResults == nil {
		m.WorkRequestResults = make(map[string]interfaces.WorkRequestSubmitResult)
	}
	m.WorkRequestResults[request.RequestID] = result
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

func (m *MockFactory) CreateNamedFactory(_ context.Context, namedFactory factoryapi.Factory) (factoryapi.Factory, error) {
	if m.CreateNamedFactoryErr != nil {
		return factoryapi.Factory{}, m.CreateNamedFactoryErr
	}
	m.CreatedFactories = append(m.CreatedFactories, namedFactory)
	copied := namedFactory
	m.CurrentNamedFactory = &copied
	return namedFactory, nil
}

func (m *MockFactory) GetCurrentNamedFactory(_ context.Context) (factoryapi.Factory, error) {
	if m.CurrentNamedFactoryErr != nil {
		return factoryapi.Factory{}, m.CurrentNamedFactoryErr
	}
	if m.CurrentNamedFactory == nil {
		return factoryapi.Factory{}, errors.New("current named factory not found")
	}
	return *m.CurrentNamedFactory, nil
}

func (m *MockFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}
