package wire

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// Lifecycle is the narrow activation contract owned by initializer. Graph
// construction stores lifecycle instances but never calls either method.
type Lifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// RuntimeInputs are the process-scoped infrastructure values available before
// fallible graph construction starts.
type RuntimeInputs struct {
	FactoryRootDir   string
	ExecutionBaseDir string
	Logger           *zap.Logger
	Clock            factory.Clock
}

// RuntimeDependencies are the constructed process-scoped infrastructure
// dependencies retained by an application graph.
type RuntimeDependencies struct {
	RuntimeInputs
	Persistence runtimepersist.Store
}

// TransportLifecycles names the long-lived transport adapters that initializer
// may activate for a selected process mode.
type TransportLifecycles struct {
	API Lifecycle
	MCP Lifecycle
}

// SidecarLifecycles names non-transport components that initializer may
// activate after graph construction succeeds.
type SidecarLifecycles struct {
	Runtime   Lifecycle
	Workers   Lifecycle
	Dashboard Lifecycle
}

// ModelWorkerServices contains the model and worker/provider collaborators
// created in one dependency-ordered construction phase.
type ModelWorkerServices struct {
	Models   *modelservice.Service
	Workers  *workersservice.Service
	Provider providerexecution.Executor
}

// FactorySessionServices contains the session collaborators created after
// persistence, model, and worker/provider construction succeeds.
type FactorySessionServices struct {
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
}

// TransportDependencies is the typed set of domain collaborators injected
// into API, CLI, and MCP adapters.
type TransportDependencies struct {
	Models            *modelservice.Service
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
}

// SidecarDependencies explicitly names the graph-owned collaborators available
// when sidecar lifecycle handles are constructed.
type SidecarDependencies struct {
	Config           *factoryconfig.LoadedFactoryConfig
	Runtime          RuntimeDependencies
	Models           *modelservice.Service
	Workers          *workersservice.Service
	Provider         providerexecution.Executor
	FactorySessions  *factorysessionsservice.Service
	DurableExecution factorysessionexecution.Service
}

// Constructed is the typed result of one graph-construction phase. Resource is
// retained by a successful graph or closed if a later phase fails.
type Constructed[T any] struct {
	Value    T
	Resource io.Closer
}

// Builders explicitly names each fallible construction phase. Builders must
// construct collaborators without starting their runtime lifecycle.
type Builders struct {
	Persistence     func(context.Context, RuntimeInputs) (Constructed[runtimepersist.Store], error)
	ModelWorkers    func(context.Context, RuntimeDependencies) (Constructed[ModelWorkerServices], error)
	FactorySessions func(
		context.Context,
		RuntimeDependencies,
		ModelWorkerServices,
	) (Constructed[FactorySessionServices], error)
	Transports func(context.Context, TransportDependencies) (Constructed[TransportLifecycles], error)
	Sidecars   func(context.Context, SidecarDependencies) (Constructed[SidecarLifecycles], error)
}

// Inputs explicitly names every startup value and construction phase assembled
// by Build. It is not a host facade or service locator.
type Inputs struct {
	Config  *factoryconfig.LoadedFactoryConfig
	Runtime RuntimeInputs
	Build   Builders
}

// Graph is the immutable-after-build process application graph. All fields are
// eagerly assigned by Build and return stable collaborator identity.
type Graph struct {
	Config            *factoryconfig.LoadedFactoryConfig
	Runtime           RuntimeDependencies
	Models            *modelservice.Service
	Workers           *workersservice.Service
	Provider          providerexecution.Executor
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
	Transport         TransportDependencies
	Transports        TransportLifecycles
	Sidecars          SidecarLifecycles
	resources         *resourceSet
}

// Close releases construction resources in reverse acquisition order. Callers
// must stop activated lifecycles before closing the graph.
func (g *Graph) Close() error {
	if g == nil || g.resources == nil {
		return nil
	}
	return g.resources.Close()
}

func validateInputs(inputs Inputs) error {
	required := []struct {
		name    string
		missing bool
	}{
		{name: "config", missing: inputs.Config == nil},
		{name: "runtime.factoryRootDir", missing: strings.TrimSpace(inputs.Runtime.FactoryRootDir) == ""},
		{name: "runtime.executionBaseDir", missing: strings.TrimSpace(inputs.Runtime.ExecutionBaseDir) == ""},
		{name: "runtime.logger", missing: inputs.Runtime.Logger == nil},
		{name: "runtime.clock", missing: isNil(inputs.Runtime.Clock)},
		{name: "builders.persistence", missing: inputs.Build.Persistence == nil},
		{name: "builders.modelWorkers", missing: inputs.Build.ModelWorkers == nil},
		{name: "builders.factorySessions", missing: inputs.Build.FactorySessions == nil},
		{name: "builders.transports", missing: inputs.Build.Transports == nil},
		{name: "builders.sidecars", missing: inputs.Build.Sidecars == nil},
	}
	for _, dependency := range required {
		if dependency.missing {
			return fmt.Errorf("validate inputs: %s is required", dependency.name)
		}
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
