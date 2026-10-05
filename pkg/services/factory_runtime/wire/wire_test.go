package wire

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewAssemblyRejectsMissingResourceOwner(t *testing.T) {
	t.Parallel()
	opening, err := NewAssembly(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if opening != nil || err == nil || err.Error() != "factory runtime factory is required" {
		t.Fatalf("NewAssembly(nil) = %v, %v; want no operation and missing owner error", opening, err)
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factoryruntime.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	idCalls := 0
	publisherCalls := 0
	cancelerCalls := 0
	clock := &recordingClock{}
	inputs := validNewServiceInputs()
	inputs.newID = func() string {
		idCalls++
		return "runtime-wire-inert-id"
	}
	inputs.clock = clock
	inputs.workersPublisher = func(context.Context, workers.WorkstationDispatchRequest) error {
		publisherCalls++
		panic("Workers publisher invoked during inert construction")
	}
	inputs.workersCanceler = func(
		context.Context,
		workers.WorkstationDispatchCancelRequest,
	) (workers.WorkstationDispatchCancelResult, error) {
		cancelerCalls++
		panic("Workers canceler invoked during inert construction")
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := inputs.callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root factoryruntime.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
	if idCalls != 0 {
		t.Fatalf("construction invoked ID generator %d times, want inert construction", idCalls)
	}
	if clock.calls != 0 {
		t.Fatalf("construction read clock %d times, want inert construction", clock.calls)
	}
	if publisherCalls != 0 || cancelerCalls != 0 {
		t.Fatalf(
			"construction invoked Workers edges (publisher=%d canceler=%d), want inert construction",
			publisherCalls, cancelerCalls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}
}

func TestNewServiceServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var runtime factoryruntime.Service = service
	if runtime == nil {
		t.Fatal("constructed root is nil")
	}

	_, err = runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(valid scope) error = %v, want ErrNotRunning", err)
	}

	_, err = runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScope("INVALID"),
	})
	if !errors.Is(err, factoryruntime.ErrInvalidObservationScope) {
		t.Fatalf("Observe(invalid scope) error = %v, want ErrInvalidObservationScope", err)
	}

	_, err = runtime.ControlPause(ctx, factoryruntime.PauseRequest{})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
	}

	_, err = runtime.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{DispatchID: "dispatch-1"})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
	}

	_, err = runtime.AcceptDispatchResult(ctx, factoryruntime.AcceptDispatchResultRequest{})
	if !errors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation) {
		t.Fatalf("AcceptDispatchResult(empty correlation) error = %v, want ErrUnknownDispatchCorrelation", err)
	}
}

type newServiceInputs struct {
	newID            factoryruntime.IDGenerator
	workflows        factoryruntime.JavaScriptWorkflowDefinitions
	workflowRuntime  factoryruntime.JavaScriptWorkflowRuntime
	clock            factoryruntime.Clock
	workersPublisher WorkersPublisher
	workersCanceler  WorkersCanceler
}

func validNewServiceInputs() newServiceInputs {
	return newServiceInputs{
		newID: func() string { return "runtime-wire-test-id" },
		clock: clockwork.NewFakeClock(),
		workersPublisher: func(context.Context, workers.WorkstationDispatchRequest) error {
			return nil
		},
		workersCanceler: func(
			context.Context,
			workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	}
}

func (in newServiceInputs) callNewService() (factoryruntime.Service, error) {
	lifecycle, err := NewLifecycle(in.clock, platformclock.Real{})
	if err != nil {
		return nil, err
	}
	host, err := NewInstanceHost(in.clock, platformclock.Real{}, lifecycle)
	if err != nil {
		return nil, err
	}
	mapper, err := NewDefinitionMapper(in.newID)
	if err != nil {
		return nil, err
	}
	return NewService(NewOrchestration(mapper, in.workflows, in.workflowRuntime), host, NewDispatchPlanning(in.workersPublisher, in.workersCanceler))
}

type recordingClock struct{ calls int }

func (c *recordingClock) Now() time.Time {
	c.calls++
	panic("clock read during inert construction")
}

func TestDefinitionMappingConstructorFailureReturnsNoCapability(t *testing.T) {
	t.Parallel()
	mapper, err := NewDefinitionMapper(nil)
	if err == nil || mapper != nil {
		t.Fatalf("NewDefinitionMapper(nil) = %v, %v; want no capability and required-ID error", mapper, err)
	}
}
