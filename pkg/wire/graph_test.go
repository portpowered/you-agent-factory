package wire_test

import (
	"context"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

func TestBuildReturnsCompleteGraphWithSharedCollaboratorIdentity(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	graph, err := wire.Build(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if graph == nil {
		t.Fatal("Build() graph is nil")
	}

	assertRuntimeIdentity(t, graph, fixture)
	assertDomainServiceIdentity(t, graph, fixture)
	assertTransportIdentity(t, graph)
	assertLifecycleIdentity(t, graph, fixture)
	if fixture.buildCount != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("builder calls = %v, want each builder called once", fixture.buildCount)
	}
}

func assertRuntimeIdentity(t *testing.T, graph *wire.Graph, fixture *buildFixture) {
	t.Helper()
	if graph.Config != fixture.inputs.Config || graph.Runtime.Logger != fixture.inputs.Runtime.Logger || graph.Runtime.Clock != fixture.inputs.Runtime.Clock || graph.Runtime.Persistence != fixture.persistence {
		t.Fatal("graph did not retain config and runtime dependency identity")
	}
}

func assertDomainServiceIdentity(t *testing.T, graph *wire.Graph, fixture *buildFixture) {
	t.Helper()
	if graph.Models != fixture.modelWorkers.Models || graph.Workers != fixture.modelWorkers.Workers || graph.Provider != fixture.modelWorkers.Provider {
		t.Fatal("graph did not retain model and worker/provider service identity")
	}
	if graph.FactorySessions != fixture.sessions.FactorySessions || graph.DurableExecution != fixture.sessions.DurableExecution {
		t.Fatal("graph did not retain Factory Session service identity")
	}
}

func assertTransportIdentity(t *testing.T, graph *wire.Graph) {
	t.Helper()
	if graph.Transport.Models != graph.Models || graph.Transport.FactoryDefinition != graph.FactoryDefinition || graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("transport dependencies do not share graph-owned service identity")
	}
}

func assertLifecycleIdentity(t *testing.T, graph *wire.Graph, fixture *buildFixture) {
	t.Helper()
	if graph.Transports != fixture.transports || graph.Sidecars != fixture.sidecars {
		t.Fatal("graph did not retain lifecycle collaborator identity")
	}
}

func TestBuildDoesNotActivateLifecycleCollaborators(t *testing.T) {
	t.Parallel()

	fixture := validFixture(t)
	_, err := wire.Build(context.Background(), fixture.inputs)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	assertNoLifecycleCalls(t, fixture)
}

type buildFixture struct {
	inputs       wire.Inputs
	persistence  runtimepersist.Store
	modelWorkers wire.ModelWorkerServices
	sessions     wire.FactorySessionServices
	transports   wire.TransportLifecycles
	sidecars     wire.SidecarLifecycles
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
	fixture := &buildFixture{
		persistence: &memoryStore{},
		modelWorkers: wire.ModelWorkerServices{
			Models:   &modelservice.Service{},
			Workers:  workersservice.New(workersservice.Config{}),
			Provider: providerExecutor{},
		},
		sessions: wire.FactorySessionServices{
			FactoryDefinition: &factorydefinition.Service{},
			FactorySessions:   &factorysessionsservice.Service{},
			DurableExecution:  durable,
		},
		transports: wire.TransportLifecycles{API: &recordingLifecycle{}, MCP: &recordingLifecycle{}},
		sidecars: wire.SidecarLifecycles{
			Runtime:   &recordingLifecycle{},
			Workers:   &recordingLifecycle{},
			Dashboard: &recordingLifecycle{},
		},
	}
	fixture.inputs = validInputs(loaded, fixture)
	return fixture
}

func validInputs(loaded *factoryconfig.LoadedFactoryConfig, fixture *buildFixture) wire.Inputs {
	return wire.Inputs{
		Config: loaded,
		Runtime: wire.RuntimeInputs{
			FactoryRootDir:   "/factory",
			ExecutionBaseDir: "/factory",
			Logger:           zap.NewNop(),
			Clock:            fixedClock{now: time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)},
		},
		Build: wire.Builders{
			Persistence: func(context.Context, wire.RuntimeInputs) (wire.Constructed[runtimepersist.Store], error) {
				fixture.buildCount[0]++
				return wire.Constructed[runtimepersist.Store]{Value: fixture.persistence}, nil
			},
			ModelWorkers: func(_ context.Context, runtime wire.RuntimeDependencies) (wire.Constructed[wire.ModelWorkerServices], error) {
				fixture.buildCount[1]++
				if runtime.Persistence != fixture.persistence {
					return wire.Constructed[wire.ModelWorkerServices]{}, errWrongDependency
				}
				return wire.Constructed[wire.ModelWorkerServices]{Value: fixture.modelWorkers}, nil
			},
			FactorySessions: func(_ context.Context, _ wire.RuntimeDependencies, models wire.ModelWorkerServices) (wire.Constructed[wire.FactorySessionServices], error) {
				fixture.buildCount[2]++
				if models != fixture.modelWorkers {
					return wire.Constructed[wire.FactorySessionServices]{}, errWrongDependency
				}
				return wire.Constructed[wire.FactorySessionServices]{Value: fixture.sessions}, nil
			},
			Transports: func(_ context.Context, deps wire.TransportDependencies) (wire.Constructed[wire.TransportLifecycles], error) {
				fixture.buildCount[3]++
				if deps.Models != fixture.modelWorkers.Models || deps.FactorySessions != fixture.sessions.FactorySessions {
					return wire.Constructed[wire.TransportLifecycles]{}, errWrongDependency
				}
				return wire.Constructed[wire.TransportLifecycles]{Value: fixture.transports}, nil
			},
			Sidecars: func(_ context.Context, deps wire.SidecarDependencies) (wire.Constructed[wire.SidecarLifecycles], error) {
				fixture.buildCount[4]++
				if deps.Runtime.Persistence != fixture.persistence || deps.Provider != fixture.modelWorkers.Provider || deps.DurableExecution != fixture.sessions.DurableExecution {
					return wire.Constructed[wire.SidecarLifecycles]{}, errWrongDependency
				}
				return wire.Constructed[wire.SidecarLifecycles]{Value: fixture.sidecars}, nil
			},
		},
	}
}

func assertNoLifecycleCalls(t *testing.T, fixture *buildFixture) {
	t.Helper()
	for name, lifecycle := range map[string]*recordingLifecycle{
		"api":       fixture.transports.API.(*recordingLifecycle),
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

type providerExecutor struct{}

func (providerExecutor) Execute(context.Context, providerexecution.ExecutionInput) (providerexecution.ExecutionResult, error) {
	return providerexecution.ExecutionResult{}, nil
}

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
