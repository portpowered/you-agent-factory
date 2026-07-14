package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
)

var errWrongDependency = errors.New("builder received the wrong dependency instance")

func TestBuildRejectsMissingAndInvalidInputsWithDependencyContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		modify     func(*phasedInputs)
		wantDetail string
	}{
		{name: "config", modify: func(inputs *phasedInputs) { inputs.Config = nil }, wantDetail: "config is required"},
		{name: "factory root", modify: func(inputs *phasedInputs) { inputs.Runtime.FactoryRootDir = " " }, wantDetail: "runtime.factoryRootDir is required"},
		{name: "execution base", modify: func(inputs *phasedInputs) { inputs.Runtime.ExecutionBaseDir = " " }, wantDetail: "runtime.executionBaseDir is required"},
		{name: "logger", modify: func(inputs *phasedInputs) { inputs.Runtime.Logger = nil }, wantDetail: "runtime.logger is required"},
		{name: "clock", modify: func(inputs *phasedInputs) { inputs.Runtime.Clock = nil }, wantDetail: "runtime.clock is required"},
		{name: "persistence builder", modify: func(inputs *phasedInputs) { inputs.Build.Persistence = nil }, wantDetail: "builders.persistence is required"},
		{name: "model builder", modify: func(inputs *phasedInputs) { inputs.Build.ModelWorkers = nil }, wantDetail: "builders.modelWorkers is required"},
		{name: "session builder", modify: func(inputs *phasedInputs) { inputs.Build.FactorySessions = nil }, wantDetail: "builders.factorySessions is required"},
		{name: "transport builder", modify: func(inputs *phasedInputs) { inputs.Build.Transports = nil }, wantDetail: "builders.transports is required"},
		{name: "sidecar builder", modify: func(inputs *phasedInputs) { inputs.Build.Sidecars = nil }, wantDetail: "builders.sidecars is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := validFixture(t)
			test.modify(&fixture.inputs)

			graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
			if graph != nil {
				t.Fatal("buildPhasedGraph() graph is non-nil")
			}
			if err == nil || !strings.Contains(err.Error(), "build application graph: validate inputs: "+test.wantDetail) {
				t.Fatalf("buildPhasedGraph() error = %v, want validation detail %q", err, test.wantDetail)
			}
			assertNoLifecycleCalls(t, fixture)
		})
	}
}

func TestBuildPreservesCanceledContextCause(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	graph, err := buildPhasedGraph(ctx, validFixture(t).inputs)
	if graph != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("buildPhasedGraph() = (%v, %v), want nil graph wrapping context.Canceled", graph, err)
	}
}

func TestBuildReportsConstructionPhaseAndNeverStartsLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantPhase string
		fail      func(*buildFixture, error)
	}{
		{name: "persistence", wantPhase: "construct persistence", fail: failPersistence},
		{name: "model and provider", wantPhase: "construct model and worker/provider services", fail: failModelWorkers},
		{name: "Factory Session and durable execution", wantPhase: "construct Factory Session and durable execution services", fail: failFactorySessions},
		{name: "transport", wantPhase: "construct transport dependencies", fail: failTransports},
		{name: "sidecar", wantPhase: "construct sidecar lifecycles", fail: failSidecars},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := validFixture(t)
			cause := fmt.Errorf("%s unavailable", test.name)
			test.fail(fixture, cause)

			graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
			if graph != nil {
				t.Fatal("buildPhasedGraph() graph is non-nil")
			}
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), test.wantPhase) {
				t.Fatalf("buildPhasedGraph() error = %v, want phase %q wrapping cause", err, test.wantPhase)
			}
			assertNoLifecycleCalls(t, fixture)
		})
	}
}

func TestBuildRejectsIncompleteConstructedCollaborator(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	fixture.inputs.Build.ModelWorkers = func(context.Context, phasedRuntimeDependencies) (constructed[phasedModelWorkerServices], error) {
		services := fixture.modelWorkers
		services.WorkerProvider = nil
		return constructed[phasedModelWorkerServices]{Value: services}, nil
	}

	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if graph != nil || err == nil || !strings.Contains(err.Error(), "construct model and worker/provider services: worker/provider runtime builder is required") {
		t.Fatalf("buildPhasedGraph() = (%v, %v), want missing provider construction error", graph, err)
	}
	assertNoLifecycleCalls(t, fixture)
}

func TestBuildRejectsTypedNilConstructedCollaborator(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	fixture.inputs.Build.Transports = func(context.Context, phasedTransportDependencies) (constructed[TransportLifecycles], error) {
		var api *recordingLifecycle
		return constructed[TransportLifecycles]{Value: TransportLifecycles{
			API: api,
			CLI: fixture.transports.CLI,
			MCP: fixture.transports.MCP,
		}}, nil
	}

	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if graph != nil || err == nil || !strings.Contains(err.Error(), "construct transport dependencies: API transport lifecycle is required") {
		t.Fatalf("buildPhasedGraph() = (%v, %v), want typed-nil transport construction error", graph, err)
	}
	assertNoLifecycleCalls(t, fixture)
}

func TestBuildFailureClosesAcquiredResourcesOnceInReverseOrder(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	var closeOrder []string
	persistenceResource := &recordingCloser{name: "persistence", order: &closeOrder}
	modelResource := &recordingCloser{name: "models", order: &closeOrder}
	fixture.inputs.Build.Persistence = func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error) {
		return constructed[runtimepersist.Store]{Value: fixture.persistence, Resource: persistenceResource}, nil
	}
	fixture.inputs.Build.ModelWorkers = func(context.Context, phasedRuntimeDependencies) (constructed[phasedModelWorkerServices], error) {
		return constructed[phasedModelWorkerServices]{Value: fixture.modelWorkers, Resource: modelResource}, nil
	}
	cause := errors.New("durable execution unavailable")
	failFactorySessions(fixture, cause)

	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if graph != nil || !errors.Is(err, cause) {
		t.Fatalf("buildPhasedGraph() = (%v, %v), want nil graph wrapping construction cause", graph, err)
	}
	if got, want := strings.Join(closeOrder, ","), "models,persistence"; got != want {
		t.Fatalf("close order = %q, want %q", got, want)
	}
	if modelResource.closes != 1 || persistenceResource.closes != 1 {
		t.Fatalf("resource closes = models %d, persistence %d; want one each", modelResource.closes, persistenceResource.closes)
	}
	assertNoLifecycleCalls(t, fixture)
}

func TestBuildFailureRetainsConstructionAndCleanupCauses(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	cleanupCause := errors.New("persistence close failed")
	resource := &recordingCloser{err: cleanupCause}
	fixture.inputs.Build.Persistence = func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error) {
		return constructed[runtimepersist.Store]{Value: fixture.persistence, Resource: resource}, nil
	}
	constructionCause := errors.New("model provider unavailable")
	failModelWorkers(fixture, constructionCause)

	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if graph != nil || !errors.Is(err, constructionCause) || !errors.Is(err, cleanupCause) {
		t.Fatalf("buildPhasedGraph() = (%v, %v), want both construction and cleanup causes", graph, err)
	}
	if resource.closes != 1 || !strings.Contains(err.Error(), "cleanup after construction failure") || !strings.Contains(err.Error(), "close persistence resource") {
		t.Fatalf("cleanup result = closes %d, error %v", resource.closes, err)
	}
	assertNoLifecycleCalls(t, fixture)
}

func TestGraphCloseReleasesConstructionResourcesIdempotently(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	resource := &recordingCloser{}
	fixture.inputs.Build.Persistence = func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error) {
		return constructed[runtimepersist.Store]{Value: fixture.persistence, Resource: resource}, nil
	}
	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("buildPhasedGraph() error = %v", err)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("Graph.Close() error = %v", err)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("second Graph.Close() error = %v", err)
	}
	if resource.closes != 1 {
		t.Fatalf("resource closes = %d, want 1", resource.closes)
	}
	assertNoLifecycleCalls(t, fixture)
}

func failPersistence(fixture *buildFixture, cause error) {
	fixture.inputs.Build.Persistence = func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error) {
		return constructed[runtimepersist.Store]{}, cause
	}
}

func failModelWorkers(fixture *buildFixture, cause error) {
	fixture.inputs.Build.ModelWorkers = func(context.Context, phasedRuntimeDependencies) (constructed[phasedModelWorkerServices], error) {
		return constructed[phasedModelWorkerServices]{}, cause
	}
}

func failFactorySessions(fixture *buildFixture, cause error) {
	fixture.inputs.Build.FactorySessions = func(context.Context, phasedRuntimeDependencies, phasedModelWorkerServices) (constructed[phasedFactorySessionServices], error) {
		return constructed[phasedFactorySessionServices]{}, cause
	}
}

func failTransports(fixture *buildFixture, cause error) {
	fixture.inputs.Build.Transports = func(context.Context, phasedTransportDependencies) (constructed[TransportLifecycles], error) {
		return constructed[TransportLifecycles]{}, cause
	}
}

func failSidecars(fixture *buildFixture, cause error) {
	fixture.inputs.Build.Sidecars = func(context.Context, phasedSidecarDependencies) (constructed[SidecarLifecycles], error) {
		return constructed[SidecarLifecycles]{}, cause
	}
}

type recordingCloser struct {
	name   string
	order  *[]string
	err    error
	closes int
}

func (c *recordingCloser) Close() error {
	c.closes++
	if c.order != nil {
		*c.order = append(*c.order, c.name)
	}
	return c.err
}

var _ io.Closer = (*recordingCloser)(nil)
