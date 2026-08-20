package internal

import (
	"context"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func stubWorkerSessionsFactory(workers.Service, platformclock.Source) (workersessions.Service, error) {
	return nil, nil
}

type stubWorkersService struct{ workers.Service }

type assemblyWorldStateOpening struct {
	recordings.RuntimeOpening
	state  interfaces.FactoryWorldState
	tick   int
	events []interfaces.FactoryEvent
}

func (opening *assemblyWorldStateOpening) ReconstructCanonicalFactoryWorldState(
	events []interfaces.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	opening.events = events
	opening.tick = selectedTick
	return opening.state, nil
}

func TestReconstructRestoredWorldStateUsesLatestReplayTick(t *testing.T) {
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 2}},
		{Context: interfaces.FactoryEventContext{Tick: 7}},
		{Context: interfaces.FactoryEventContext{Tick: 4}},
	}
	opening := &assemblyWorldStateOpening{state: interfaces.FactoryWorldState{Tick: 7}}

	state, err := reconstructRestoredWorldState(opening, events)
	if err != nil {
		t.Fatalf("reconstructRestoredWorldState: %v", err)
	}
	if state == nil || state.Tick != 7 {
		t.Fatalf("restored state = %#v, want tick 7", state)
	}
	if opening.tick != 7 {
		t.Fatalf("selected reconstruction tick = %d, want latest replay tick 7", opening.tick)
	}
	if len(opening.events) != len(events) {
		t.Fatalf("reconstruction events = %d, want %d", len(opening.events), len(events))
	}
}

func TestNewAssemblyRequiresWireConstructedRuntimeFactory(t *testing.T) {
	assembly, err := NewAssembly(nil, stubWorkerSessionsFactory, nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime factory is required") {
		t.Fatalf("NewAssembly(nil) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyRequiresWorkerSessionsFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory, nil, stubWorkersService{})
	if err == nil || !strings.Contains(err.Error(), "Worker Sessions factory is required") {
		t.Fatalf("NewAssembly(nil factory) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil factory) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyRequiresWorkersService(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory, stubWorkerSessionsFactory, nil)
	if err == nil || !strings.Contains(err.Error(), "Workers service is required") {
		t.Fatalf("NewAssembly(nil Workers service) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil Workers service) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyBindsRuntimeFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	workerService := stubWorkersService{}
	assembly, err := NewAssembly(runtimeFactory, stubWorkerSessionsFactory, workerService)
	if err != nil {
		t.Fatalf("NewAssembly() error = %v", err)
	}
	if assembly == nil || assembly.runtimeFactory != runtimeFactory {
		t.Fatalf("NewAssembly() = %#v, want supplied Runtime Factory", assembly)
	}
	if assembly.workerService != workerService {
		t.Fatalf("NewAssembly() worker service = %#v, want supplied service", assembly.workerService)
	}
}

func TestRuntimeCompositionComposesInertInstanceHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	lifecycle, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
	if err != nil {
		t.Fatalf("instancehostwire.New() error = %v", err)
	}
	var _ factoryruntime.RuntimeLifecycle = lifecycle
	if _, ok := lifecycle.(instancehost.Service); !ok {
		t.Fatalf("composed lifecycle type = %T, want instance_host.Service", lifecycle)
	}
}

func TestBoundRuntimeServiceUsesConcreteDelegateForWideOperations(t *testing.T) {
	t.Parallel()

	root, err := NewRoot(
		func() string { return "runtime-test-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	engine := &wideOperationRuntimeFake{}
	wrapper := &runtimeDelegateWrapper{Service: engine, delegate: engine}
	root.active["runtime-1"] = &runtimeActivationState{service: wrapper, ingress: engine}

	binding := &boundRuntimeService{root: root, runtimeID: "runtime-1"}
	if _, err := binding.SubmitWorkRequest(context.Background(), work.WorkRequest{}); err != nil {
		t.Fatalf("SubmitWorkRequest() error = %v", err)
	}
	if _, err := binding.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("SubscribeFactoryEvents() error = %v", err)
	}
	if engine.submitCalls != 1 || engine.eventCalls != 1 {
		t.Fatalf("engine wide-operation calls = (%d, %d), want (1, 1)", engine.submitCalls, engine.eventCalls)
	}
	if wrapper.submitCalls != 0 || wrapper.eventCalls != 0 {
		t.Fatalf("compatibility wrapper wide-operation calls = (%d, %d), want (0, 0)", wrapper.submitCalls, wrapper.eventCalls)
	}
}

type runtimeDelegateWrapper struct {
	factoryruntime.Service
	delegate    factoryruntime.Service
	submitCalls int
	eventCalls  int
}

func (wrapper *runtimeDelegateWrapper) RuntimeDelegate() factoryruntime.Service {
	return wrapper.delegate
}

func (wrapper *runtimeDelegateWrapper) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	wrapper.submitCalls++
	return work.WorkRequestSubmitResult{}, nil
}

func (wrapper *runtimeDelegateWrapper) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	wrapper.eventCalls++
	return nil, nil
}

type wideOperationRuntimeFake struct {
	factoryruntime.Service
	submitCalls int
	eventCalls  int
}

func (service *wideOperationRuntimeFake) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	service.submitCalls++
	return work.WorkRequestSubmitResult{}, nil
}

func (service *wideOperationRuntimeFake) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	service.eventCalls++
	return nil, nil
}
