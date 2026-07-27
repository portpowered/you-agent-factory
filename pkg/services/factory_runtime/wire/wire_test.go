package wire

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	valid := validNewServiceInputs()
	tests := []struct {
		name   string
		mutate func(*newServiceInputs)
	}{
		{name: "ID generator", mutate: func(in *newServiceInputs) { in.newID = nil }},
		{name: "clock", mutate: func(in *newServiceInputs) { in.clock = nil }},
		{name: "Workers publisher", mutate: func(in *newServiceInputs) { in.workersPublisher = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := valid
			test.mutate(&inputs)
			service, err := inputs.callNewService()
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
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
	return NewService(
		in.newID,
		in.workflows,
		in.workflowRuntime,
		in.clock,
		in.workersPublisher,
		in.workersCanceler,
	)
}

type recordingClock struct{ calls int }

func (c *recordingClock) Now() time.Time {
	c.calls++
	panic("clock read during inert construction")
}
