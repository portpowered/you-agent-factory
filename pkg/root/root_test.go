package root_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

func TestStartBuildsGraphBeforeInitializerActivation(t *testing.T) {
	t.Parallel()

	fixture := newRootFixture(t)
	application, err := root.Start(context.Background(), root.Inputs{
		Mode:  initializer.ModeAPI,
		Graph: fixture.inputs,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if application == nil || application.Graph() == nil {
		t.Fatal("Start() returned no constructed application graph")
	}
	if application.Graph().Models != fixture.models || application.Graph().DurableExecution != fixture.durable {
		t.Fatal("initializer did not observe the graph builder's service instances")
	}
	if fixture.starts != 4 {
		t.Fatalf("lifecycle starts = %d, want 4", fixture.starts)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if fixture.stops != 4 || fixture.closes != 1 {
		t.Fatalf("shutdown calls = stop %d, close %d; want 4 and 1", fixture.stops, fixture.closes)
	}
}

func TestStartConstructionFailureDoesNotActivateLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newRootFixture(t)
	cause := errors.New("persistence offline")
	fixture.inputs.Build.Persistence = func(context.Context, wire.RuntimeInputs) (wire.Constructed[runtimepersist.Store], error) {
		return wire.Constructed[runtimepersist.Store]{}, cause
	}

	application, err := root.Start(context.Background(), root.Inputs{
		Mode:  initializer.ModeAPI,
		Graph: fixture.inputs,
	})
	if application != nil || !errors.Is(err, cause) {
		t.Fatalf("Start() = (%v, %v), want nil application wrapping cause", application, err)
	}
	if fixture.starts != 0 || fixture.stops != 0 {
		t.Fatalf("lifecycle calls = start %d, stop %d; want zero", fixture.starts, fixture.stops)
	}
}

type rootFixture struct {
	inputs  wire.Inputs
	models  *modelservice.Service
	durable factorysessionexecution.Service
	starts  int
	stops   int
	closes  int
}

func newRootFixture(t *testing.T) *rootFixture {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig() error = %v", err)
	}
	durable, err := factorysessionexecution.NewExecutionService(
		factorysessionexecution.ExecutionProviderFake,
		factorysessionexecution.ServiceConfig{},
	)
	if err != nil {
		t.Fatalf("NewExecutionService() error = %v", err)
	}
	fixture := &rootFixture{models: modelservice.New(modelservice.Dependencies{}), durable: durable}
	persistence := rootMemoryStore{}
	workers := workersservice.New(workersservice.Config{})
	provider := rootProviderExecutor{}
	sessions := &factorysessionsservice.Service{}
	definition := &factorydefinition.Service{}
	lifecycle := func() wire.Lifecycle { return &rootLifecycle{fixture: fixture} }
	fixture.inputs = wire.Inputs{
		Config: loaded,
		Runtime: wire.RuntimeInputs{
			FactoryRootDir:   loaded.FactoryDir(),
			ExecutionBaseDir: loaded.FactoryDir(),
			Logger:           zap.NewNop(),
			Clock:            rootClock{},
		},
		Build: wire.Builders{
			Persistence: func(context.Context, wire.RuntimeInputs) (wire.Constructed[runtimepersist.Store], error) {
				return wire.Constructed[runtimepersist.Store]{Value: persistence, Resource: rootCloser{fixture: fixture}}, nil
			},
			ModelWorkers: func(context.Context, wire.RuntimeDependencies) (wire.Constructed[wire.ModelWorkerServices], error) {
				return wire.Constructed[wire.ModelWorkerServices]{Value: wire.ModelWorkerServices{Models: fixture.models, Workers: workers, Provider: provider}}, nil
			},
			FactorySessions: func(context.Context, wire.RuntimeDependencies, wire.ModelWorkerServices) (wire.Constructed[wire.FactorySessionServices], error) {
				return wire.Constructed[wire.FactorySessionServices]{Value: wire.FactorySessionServices{FactoryDefinition: definition, FactorySessions: sessions, DurableExecution: durable}}, nil
			},
			Transports: func(context.Context, wire.TransportDependencies) (wire.Constructed[wire.TransportLifecycles], error) {
				return wire.Constructed[wire.TransportLifecycles]{Value: wire.TransportLifecycles{API: lifecycle(), CLI: lifecycle(), MCP: lifecycle()}}, nil
			},
			Sidecars: func(context.Context, wire.SidecarDependencies) (wire.Constructed[wire.SidecarLifecycles], error) {
				return wire.Constructed[wire.SidecarLifecycles]{Value: wire.SidecarLifecycles{Runtime: lifecycle(), Workers: lifecycle(), Dashboard: lifecycle()}}, nil
			},
		},
	}
	return fixture
}

type rootMemoryStore struct{}

func (rootMemoryStore) Save(string, []byte) error   { return nil }
func (rootMemoryStore) Load(string) ([]byte, error) { return nil, nil }

type rootProviderExecutor struct{}

func (rootProviderExecutor) Execute(context.Context, providerexecution.ExecutionInput) (providerexecution.ExecutionResult, error) {
	return providerexecution.ExecutionResult{}, nil
}

type rootLifecycle struct{ fixture *rootFixture }

func (l *rootLifecycle) Start(context.Context) error { l.fixture.starts++; return nil }
func (l *rootLifecycle) Stop(context.Context) error  { l.fixture.stops++; return nil }

type rootCloser struct{ fixture *rootFixture }

func (c rootCloser) Close() error { c.fixture.closes++; return nil }

type rootClock struct{}

func (rootClock) Now() time.Time { return time.Unix(0, 0).UTC() }
