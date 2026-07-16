package wire

import (
	"context"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

func TestBuildReturnsCompleteGraphWithSharedCollaboratorIdentity(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	graph, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("buildPhasedGraph() error = %v", err)
	}
	if graph == nil {
		t.Fatal("buildPhasedGraph() graph is nil")
	}

	assertRuntimeIdentity(t, graph, fixture)
	assertDomainServiceIdentity(t, graph, fixture)
	assertTransportIdentity(t, graph)
	assertLifecycleIdentity(t, graph, fixture)
	if fixture.buildCount != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("builder calls = %v, want each builder called once", fixture.buildCount)
	}
}

func assertRuntimeIdentity(t *testing.T, graph *phasedGraph, fixture *buildFixture) {
	t.Helper()
	if graph.Config != fixture.inputs.Config || graph.Runtime.Logger != fixture.inputs.Runtime.Logger || graph.Runtime.Clock != fixture.inputs.Runtime.Clock || graph.Runtime.Persistence != fixture.persistence {
		t.Fatal("graph did not retain config and runtime dependency identity")
	}
}

func assertDomainServiceIdentity(t *testing.T, graph *phasedGraph, fixture *buildFixture) {
	t.Helper()
	if graph.Models != fixture.modelWorkers.Models || graph.Workers != fixture.modelWorkers.Workers || graph.WorkerProvider != fixture.modelWorkers.WorkerProvider {
		t.Fatal("graph did not retain model and worker/provider service identity")
	}
	if graph.FactorySessions != fixture.sessions.FactorySessions || graph.DurableExecution != fixture.sessions.DurableExecution {
		t.Fatal("graph did not retain Factory Session service identity")
	}
}

func assertTransportIdentity(t *testing.T, graph *phasedGraph) {
	t.Helper()
	if graph.Transport.Models != graph.Models || graph.Transport.FactoryDefinition != graph.FactoryDefinition || graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("transport dependencies do not share graph-owned service identity")
	}
}

func assertLifecycleIdentity(t *testing.T, graph *phasedGraph, fixture *buildFixture) {
	t.Helper()
	if graph.Transports != fixture.transports || graph.Sidecars != fixture.sidecars {
		t.Fatal("graph did not retain lifecycle collaborator identity")
	}
}

func TestBuildDoesNotActivateLifecycleCollaborators(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	_, err := buildPhasedGraph(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("buildPhasedGraph() error = %v", err)
	}

	assertNoLifecycleCalls(t, fixture)
}

type buildFixture struct {
	inputs       phasedInputs
	persistence  runtimepersist.Store
	modelWorkers phasedModelWorkerServices
	sessions     phasedFactorySessionServices
	transports   TransportLifecycles
	sidecars     SidecarLifecycles
	buildCount   [5]int
}

func validFixture(t *testing.T) *buildFixture {
	t.Helper()

	loaded, err := factoryconfig.NewLoadedFactoryConfig("/factory", &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig() error = %v", err)
	}
	durable, err := factorysessionexecution.NewExecutionService(factorysessionexecution.ExecutionProviderFake, factorysessionexecution.ServiceConfig{})
	if err != nil {
		t.Fatalf("NewExecutionService() error = %v", err)
	}
	workerProvider, err := runtimebuild.New(runtimebuild.Config{}, fixedClock{}, zap.NewNop(), func(context.Context, runtimebuild.SessionBuildSpec) (any, error) {
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("runtimebuild.New() error = %v", err)
	}
	fixture := &buildFixture{
		persistence: &memoryStore{},
		modelWorkers: phasedModelWorkerServices{
			Models:         &modelservice.Service{},
			Workers:        workersservice.New(workersservice.Config{}),
			WorkerProvider: workerProvider,
		},
		sessions: phasedFactorySessionServices{
			FactoryDefinition: &factorydefinition.Service{},
			FactorySessions:   &factorysessionsservice.Service{},
			DurableExecution:  durable,
		},
		transports: TransportLifecycles{API: &recordingLifecycle{}, CLI: &recordingLifecycle{}, MCP: &recordingLifecycle{}},
		sidecars: SidecarLifecycles{
			Runtime:   &recordingLifecycle{},
			Workers:   &recordingLifecycle{},
			Dashboard: &recordingLifecycle{},
		},
	}
	fixture.inputs = validInputs(loaded, fixture)
	return fixture
}

func validInputs(loaded *factoryconfig.LoadedFactoryConfig, fixture *buildFixture) phasedInputs {
	return phasedInputs{
		Config: loaded,
		Runtime: RuntimeInputs{
			FactoryRootDir:   "/factory",
			ExecutionBaseDir: "/factory",
			Logger:           zap.NewNop(),
			Clock:            fixedClock{now: time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)},
		},
		Build: phasedBuilders{
			Persistence: func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error) {
				fixture.buildCount[0]++
				return constructed[runtimepersist.Store]{Value: fixture.persistence}, nil
			},
			ModelWorkers: func(_ context.Context, runtime phasedRuntimeDependencies) (constructed[phasedModelWorkerServices], error) {
				fixture.buildCount[1]++
				if runtime.Persistence != fixture.persistence {
					return constructed[phasedModelWorkerServices]{}, errWrongDependency
				}
				return constructed[phasedModelWorkerServices]{Value: fixture.modelWorkers}, nil
			},
			FactorySessions: func(_ context.Context, _ phasedRuntimeDependencies, models phasedModelWorkerServices) (constructed[phasedFactorySessionServices], error) {
				fixture.buildCount[2]++
				if models != fixture.modelWorkers {
					return constructed[phasedFactorySessionServices]{}, errWrongDependency
				}
				return constructed[phasedFactorySessionServices]{Value: fixture.sessions}, nil
			},
			Transports: func(_ context.Context, deps phasedTransportDependencies) (constructed[TransportLifecycles], error) {
				fixture.buildCount[3]++
				if deps.Models != fixture.modelWorkers.Models || deps.FactorySessions != fixture.sessions.FactorySessions {
					return constructed[TransportLifecycles]{}, errWrongDependency
				}
				return constructed[TransportLifecycles]{Value: fixture.transports}, nil
			},
			Sidecars: func(_ context.Context, deps phasedSidecarDependencies) (constructed[SidecarLifecycles], error) {
				fixture.buildCount[4]++
				if deps.Runtime.Persistence != fixture.persistence || deps.WorkerProvider != fixture.modelWorkers.WorkerProvider || deps.DurableExecution != fixture.sessions.DurableExecution {
					return constructed[SidecarLifecycles]{}, errWrongDependency
				}
				return constructed[SidecarLifecycles]{Value: fixture.sidecars}, nil
			},
		},
	}
}

func assertNoLifecycleCalls(t *testing.T, fixture *buildFixture) {
	t.Helper()
	for name, lifecycle := range map[string]*recordingLifecycle{
		"api":       fixture.transports.API.(*recordingLifecycle),
		"cli":       fixture.transports.CLI.(*recordingLifecycle),
		"mcp":       fixture.transports.MCP.(*recordingLifecycle),
		"runtime":   fixture.sidecars.Runtime.(*recordingLifecycle),
		"workers":   fixture.sidecars.Workers.(*recordingLifecycle),
		"dashboard": fixture.sidecars.Dashboard.(*recordingLifecycle),
	} {
		if lifecycle.starts != 0 || lifecycle.stops != 0 {
			t.Errorf("%s lifecycle calls = start %d, stop %d; want zero", name, lifecycle.starts, lifecycle.stops)
		}
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

var _ factory.Clock = fixedClock{}

type memoryStore struct{}

func (*memoryStore) Save(string, []byte) error   { return nil }
func (*memoryStore) Load(string) ([]byte, error) { return nil, nil }

var _ runtimepersist.Store = (*memoryStore)(nil)

type recordingLifecycle struct {
	starts int
	stops  int
}

func (l *recordingLifecycle) Start(context.Context) error {
	l.starts++
	return nil
}

func (l *recordingLifecycle) Stop(context.Context) error {
	l.stops++
	return nil
}
