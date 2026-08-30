package processlifecycle

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// TestBatchColdStartCharacterizationLifecycleRolesAndCleanup freezes the
// current activation transaction for both no-listener batch and hosted
// server-shaped components. The fake transport records readiness and listener
// ownership so the assertions stay at the lifecycle package boundary.
type batchColdStartLifecycleCase struct {
	name              string
	server            bool
	wantRoles         []string
	wantEvents        []string
	wantVisualization bool
}

var batchColdStartLifecycleCases = []batchColdStartLifecycleCase{
	{
		name:      "batch",
		wantRoles: []string{runtimeComponentName, workersComponentName, transportComponentName},
		wantEvents: []string{
			"runtime:start", "workers:start", "transport:start", "transport:wait",
			"transport:stop", "workers:stop", "runtime:stop", "runtime:close",
		},
	},
	{
		name:              "with-server",
		server:            true,
		wantRoles:         []string{runtimeComponentName, workersComponentName, visualizationComponentName, transportComponentName},
		wantVisualization: true,
		wantEvents: []string{
			"runtime:start", "workers:start", "visualization:start", "transport:start", "transport:wait",
			"transport:stop", "visualization:stop", "workers:stop", "runtime:stop", "runtime:close",
		},
	},
}

func TestBatchColdStartCharacterizationLifecycleRolesAndCleanup(t *testing.T) {
	t.Parallel()

	for _, test := range batchColdStartLifecycleCases {
		t.Run(test.name, func(t *testing.T) {
			runBatchColdStartLifecycleCase(t, test)
		})
	}
}

func runBatchColdStartLifecycleCase(t *testing.T, test batchColdStartLifecycleCase) {
	t.Helper()

	events := make([]string, 0, len(test.wantEvents))
	runtime := &batchColdStartProcessRuntime{events: &events}
	transport := &batchColdStartLifecycleComponent{name: "transport", events: &events, server: test.server}
	components := factorysessions.BoundProcessComponents{Transport: transport}
	if test.wantVisualization {
		components.Visualization = &batchColdStartLifecycleComponent{name: "visualization", events: &events}
	}
	closeCalls := 0
	plan := buildBatchColdStartLifecyclePlan(t, runtime, components, &events, &closeCalls)
	rolesSeen := lifecycleRoleNames(plan)
	if !slices.Equal(rolesSeen, test.wantRoles) {
		t.Fatalf("lifecycle roles = %v, want %v", rolesSeen, test.wantRoles)
	}
	if err := lifecycle.NewManager().Run(context.Background(), plan); err != nil {
		t.Fatalf("lifecycle Run() error = %v", err)
	}
	if !slices.Equal(events, test.wantEvents) {
		t.Fatalf("lifecycle events = %v, want %v", events, test.wantEvents)
	}
	assertBatchColdStartRuntimeCalls(t, runtime)
	assertBatchColdStartTransportCalls(t, transport)
	assertBatchColdStartListenerState(t, transport, test.server)
	if closeCalls != 1 {
		t.Fatalf("runtime close calls = %d, want exactly 1", closeCalls)
	}
	t.Logf("roles=%v calls=runtime:%d workers:%d worker_stop:%d transport:%d/%d/%d listener:%d/%d close:%d",
		rolesSeen, runtime.startCalls, runtime.workerStartCalls, runtime.workerStopCalls,
		transport.startCalls, transport.waitCalls, transport.stopCalls,
		transport.listenerStarts, transport.listenerStops, closeCalls)
}

func buildBatchColdStartLifecyclePlan(
	t *testing.T,
	runtime *batchColdStartProcessRuntime,
	components factorysessions.BoundProcessComponents,
	events *[]string,
	closeCalls *int,
) lifecycle.Plan {
	t.Helper()
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime:    runtime,
		Components: components,
		Close: func() error {
			*closeCalls++
			*events = append(*events, "runtime:close")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan() error = %v", err)
	}
	return plan
}

func lifecycleRoleNames(plan lifecycle.Plan) []string {
	rolesSeen := make([]string, 0, len(plan.Components))
	for _, component := range plan.Components {
		rolesSeen = append(rolesSeen, component.Name)
	}
	return rolesSeen
}

func assertBatchColdStartRuntimeCalls(t *testing.T, runtime *batchColdStartProcessRuntime) {
	t.Helper()
	if runtime.startCalls != 1 || runtime.workerStartCalls != 1 || runtime.workerStopCalls != 1 || runtime.stopCalls != 1 {
		t.Fatalf("runtime calls start/workers/worker-stop/stop = %d/%d/%d/%d, want 1/1/1/1",
			runtime.startCalls, runtime.workerStartCalls, runtime.workerStopCalls, runtime.stopCalls)
	}
}

func assertBatchColdStartTransportCalls(t *testing.T, transport *batchColdStartLifecycleComponent) {
	t.Helper()
	if transport.startCalls != 1 || transport.waitCalls != 1 || transport.stopCalls != 1 {
		t.Fatalf("transport calls start/wait/stop = %d/%d/%d, want 1/1/1",
			transport.startCalls, transport.waitCalls, transport.stopCalls)
	}
}

func assertBatchColdStartListenerState(t *testing.T, transport *batchColdStartLifecycleComponent, server bool) {
	t.Helper()
	if server {
		if transport.listenerStarts != 1 || transport.listenerStops != 1 || !transport.readyAtWait || transport.ready {
			t.Fatalf("server readiness/listener values = starts:%d stops:%d ready_at_wait:%t ready:%t, want 1/1/true/false",
				transport.listenerStarts, transport.listenerStops, transport.readyAtWait, transport.ready)
		}
		return
	}
	if transport.listenerStarts != 0 || transport.listenerStops != 0 || transport.ready {
		t.Fatalf("batch listener values = starts:%d stops:%d ready:%t, want 0/0/false",
			transport.listenerStarts, transport.listenerStops, transport.ready)
	}
}

// TestBatchColdStartCharacterizationLifecycleFailureUnwinds freezes the
// current controlled failure paths: no dispatch-like component reaches Stop
// unless Start succeeded, while every prior acquisition unwinds in reverse and
// the runtime resource closes once.
func TestBatchColdStartCharacterizationLifecycleFailureUnwinds(t *testing.T) {
	t.Parallel()

	t.Run("worker-start-failure", func(t *testing.T) {
		startErr := errors.New("controlled worker start failure")
		events := []string{}
		runtime := &batchColdStartProcessRuntime{events: &events, workerErr: startErr}
		transport := &batchColdStartLifecycleComponent{name: "transport", events: &events}
		closeCalls := 0
		plan := batchColdStartPlan(t, runtime, factorysessions.BoundProcessComponents{Transport: transport}, &closeCalls)

		err := lifecycle.NewManager().Run(context.Background(), plan)
		if !errors.Is(err, startErr) {
			t.Fatalf("lifecycle error = %v, want %v", err, startErr)
		}
		want := []string{"runtime:start", "workers:start", "workers:stop", "runtime:stop", "runtime:close"}
		if !slices.Equal(events, want) {
			t.Fatalf("failure events = %v, want %v", events, want)
		}
		if transport.startCalls != 0 || closeCalls != 1 || runtime.workerStopCalls != 1 || runtime.stopCalls != 1 {
			t.Fatalf("worker failure cleanup transport-start/worker-stop/runtime-stop/close = %d/%d/%d/%d, want 0/1/1/1",
				transport.startCalls, runtime.workerStopCalls, runtime.stopCalls, closeCalls)
		}
	})

	t.Run("hosted-transport-start-failure", func(t *testing.T) {
		startErr := errors.New("controlled hosted transport start failure")
		events := []string{}
		runtime := &batchColdStartProcessRuntime{events: &events}
		transport := &batchColdStartLifecycleComponent{name: "transport", events: &events, startErr: startErr}
		visualization := &batchColdStartLifecycleComponent{name: "visualization", events: &events}
		closeCalls := 0
		plan := batchColdStartPlan(t, runtime, factorysessions.BoundProcessComponents{
			Transport: transport, Visualization: visualization,
		}, &closeCalls)

		err := lifecycle.NewManager().Run(context.Background(), plan)
		if !errors.Is(err, startErr) {
			t.Fatalf("lifecycle error = %v, want %v", err, startErr)
		}
		want := []string{
			"runtime:start", "workers:start", "visualization:start", "transport:start",
			"visualization:stop", "workers:stop", "runtime:stop", "runtime:close",
		}
		if !slices.Equal(events, want) {
			t.Fatalf("hosted failure events = %v, want %v", events, want)
		}
		if transport.stopCalls != 0 || transport.listenerStarts != 0 || closeCalls != 1 {
			t.Fatalf("hosted failure transport-stop/listener-start/close = %d/%d/%d, want 0/0/1",
				transport.stopCalls, transport.listenerStarts, closeCalls)
		}
	})

}

func TestBatchColdStartCharacterizationInvalidPlanDoesNotActivate(t *testing.T) {
	t.Parallel()

	events := []string{}
	runtime := &batchColdStartProcessRuntime{events: &events}
	if _, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{Runtime: runtime}); err == nil {
		t.Fatal("BuildLifecyclePlan() error = nil, want missing transport failure")
	}
	if len(events) != 0 || runtime.startCalls != 0 || runtime.workerStartCalls != 0 {
		t.Fatalf("invalid plan activated runtime: events=%v runtime-start/workers=%d/%d", events, runtime.startCalls, runtime.workerStartCalls)
	}
}

func batchColdStartPlan(
	t *testing.T,
	runtime *batchColdStartProcessRuntime,
	components factorysessions.BoundProcessComponents,
	closeCalls *int,
) lifecycle.Plan {
	t.Helper()
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime, Components: components,
		Close: func() error {
			*closeCalls++
			*runtime.events = append(*runtime.events, "runtime:close")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan() error = %v", err)
	}
	return plan
}

type batchColdStartProcessRuntime struct {
	events           *[]string
	startErr         error
	workerErr        error
	stopErr          error
	startCalls       int
	workerStartCalls int
	workerStopCalls  int
	stopCalls        int
}

func (runtime *batchColdStartProcessRuntime) Start(context.Context, context.Context) error {
	runtime.startCalls++
	*runtime.events = append(*runtime.events, "runtime:start")
	return runtime.startErr
}

func (runtime *batchColdStartProcessRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	runtime.workerStartCalls++
	*runtime.events = append(*runtime.events, "workers:start")
	stop := func(context.Context) error {
		runtime.workerStopCalls++
		*runtime.events = append(*runtime.events, "workers:stop")
		return nil
	}
	return stop, runtime.workerErr
}

func (*batchColdStartProcessRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (runtime *batchColdStartProcessRuntime) Stop(context.Context) error {
	runtime.stopCalls++
	*runtime.events = append(*runtime.events, "runtime:stop")
	return runtime.stopErr
}

type batchColdStartLifecycleComponent struct {
	name           string
	events         *[]string
	server         bool
	startErr       error
	startCalls     int
	waitCalls      int
	stopCalls      int
	listenerStarts int
	listenerStops  int
	ready          bool
	readyAtWait    bool
}

func (component *batchColdStartLifecycleComponent) Start(context.Context) error {
	component.startCalls++
	*component.events = append(*component.events, component.name+":start")
	if component.startErr != nil {
		return component.startErr
	}
	if component.server {
		component.listenerStarts++
		component.ready = true
	}
	return nil
}

func (component *batchColdStartLifecycleComponent) Stop(context.Context) error {
	component.stopCalls++
	*component.events = append(*component.events, component.name+":stop")
	if component.server {
		component.listenerStops++
		component.ready = false
	}
	return nil
}

func (component *batchColdStartLifecycleComponent) Wait(context.Context) error {
	component.waitCalls++
	*component.events = append(*component.events, component.name+":wait")
	component.readyAtWait = component.ready || !component.server
	return nil
}

var _ roles.ProcessRuntime = (*batchColdStartProcessRuntime)(nil)
