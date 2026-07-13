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

	inputs := validInputs(t)
	graph, err := wire.Build(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if graph == nil {
		t.Fatal("Build() graph is nil")
	}

	assertRuntimeIdentity(t, graph, inputs)
	assertDomainServiceIdentity(t, graph, inputs)
	assertTransportIdentity(t, graph)
	assertLifecycleIdentity(t, graph, inputs)
}

func assertRuntimeIdentity(t *testing.T, graph *wire.Graph, inputs wire.Inputs) {
	t.Helper()
	if graph.Config != inputs.Config || graph.Runtime.Logger != inputs.Runtime.Logger || graph.Runtime.Clock != inputs.Runtime.Clock || graph.Runtime.Persistence != inputs.Runtime.Persistence {
		t.Fatal("graph did not retain config and runtime dependency identity")
	}
}

func assertDomainServiceIdentity(t *testing.T, graph *wire.Graph, inputs wire.Inputs) {
	t.Helper()
	if graph.Models != inputs.Models || graph.Workers != inputs.Workers || graph.Provider != inputs.Provider {
		t.Fatal("graph did not retain model and worker/provider service identity")
	}
	if graph.FactorySessions != inputs.FactorySessions || graph.DurableExecution != inputs.DurableExecution {
		t.Fatal("graph did not retain Factory Session service identity")
	}
}

func assertTransportIdentity(t *testing.T, graph *wire.Graph) {
	t.Helper()
	if graph.Transport.Models != graph.Models || graph.Transport.FactoryDefinition != graph.FactoryDefinition || graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("transport dependencies do not share graph-owned service identity")
	}
}

func assertLifecycleIdentity(t *testing.T, graph *wire.Graph, inputs wire.Inputs) {
	t.Helper()
	if graph.Transports != inputs.Transports || graph.Sidecars != inputs.Sidecars {
		t.Fatal("graph did not retain lifecycle collaborator identity")
	}
}

func TestBuildDoesNotActivateLifecycleCollaborators(t *testing.T) {
	t.Parallel()

	inputs := validInputs(t)
	_, err := wire.Build(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for name, lifecycle := range map[string]*recordingLifecycle{
		"api":       inputs.Transports.API.(*recordingLifecycle),
		"mcp":       inputs.Transports.MCP.(*recordingLifecycle),
		"runtime":   inputs.Sidecars.Runtime.(*recordingLifecycle),
		"workers":   inputs.Sidecars.Workers.(*recordingLifecycle),
		"dashboard": inputs.Sidecars.Dashboard.(*recordingLifecycle),
	} {
		if lifecycle.starts != 0 || lifecycle.stops != 0 {
			t.Errorf("%s lifecycle calls = start %d, stop %d; want zero", name, lifecycle.starts, lifecycle.stops)
		}
	}
}

func validInputs(t *testing.T) wire.Inputs {
	t.Helper()

	loaded, err := factoryconfig.NewLoadedFactoryConfig("/factory", &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig() error = %v", err)
	}
	durable, err := factorysessionexecution.NewExecutionService(factorysessionexecution.ExecutionProviderFake, factorysessionexecution.ServiceConfig{})
	if err != nil {
		t.Fatalf("NewExecutionService() error = %v", err)
	}
	clock := fixedClock{now: time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)}

	return wire.Inputs{
		Config: loaded,
		Runtime: wire.RuntimeDependencies{
			FactoryRootDir:   "/factory",
			ExecutionBaseDir: "/factory",
			Logger:           zap.NewNop(),
			Clock:            clock,
			Persistence:      memoryStore{},
		},
		Models:            &modelservice.Service{},
		Workers:           workersservice.New(workersservice.Config{}),
		Provider:          providerExecutor{},
		FactoryDefinition: &factorydefinition.Service{},
		FactorySessions:   &factorysessionsservice.Service{},
		DurableExecution:  durable,
		Transports: wire.TransportLifecycles{
			API: &recordingLifecycle{},
			MCP: &recordingLifecycle{},
		},
		Sidecars: wire.SidecarLifecycles{
			Runtime:   &recordingLifecycle{},
			Workers:   &recordingLifecycle{},
			Dashboard: &recordingLifecycle{},
		},
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

var _ factory.Clock = fixedClock{}

type memoryStore struct{}

func (memoryStore) Save(string, []byte) error   { return nil }
func (memoryStore) Load(string) ([]byte, error) { return nil, nil }

var _ runtimepersist.Store = memoryStore{}

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
