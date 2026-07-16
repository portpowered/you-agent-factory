package wire

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
	"github.com/portpowered/infinite-you/pkg/initializer"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
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

// phasedRuntimeDependencies are retained only by the internal construction
// failure harness.
type phasedRuntimeDependencies struct {
	RuntimeInputs
	Persistence runtimepersist.Store
}

// TransportLifecycles names the long-lived transport adapters that initializer
// may activate for a selected process mode.
type TransportLifecycles struct {
	API Lifecycle
	CLI Lifecycle
	MCP Lifecycle
}

// SidecarLifecycles names non-transport components that initializer may
// activate after graph construction succeeds.
type SidecarLifecycles struct {
	Runtime   Lifecycle
	Workers   Lifecycle
	Dashboard Lifecycle
}

// phasedModelWorkerServices contains the model and worker/provider collaborators
// created in one dependency-ordered construction phase. WorkerProvider is the
// runtime builder that creates worker-specific provider runners; production
// does not have one process-wide provider executor.
type phasedModelWorkerServices struct {
	Models         *modelservice.Service
	Workers        *workersservice.Service
	WorkerProvider *runtimebuild.Service
}

// phasedFactorySessionServices contains the session collaborators created after
// persistence, model, and worker/provider construction succeeds.
type phasedFactorySessionServices struct {
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
}

// TransportDependencies is the typed set of domain collaborators injected
// into API, CLI, and MCP adapters.
type TransportDependencies struct {
	API               apisurface.SessionAPISurface
	Models            apisurface.ModelAPI
	FactoryDefinition apisurface.FactorySaveAPI
	FactorySessions   apisurface.SessionAPI
	DurableExecution  factorysessionexecution.Service
}

// phasedSidecarDependencies explicitly names the graph-owned collaborators available
// when sidecar lifecycle handles are constructed.
type phasedSidecarDependencies struct {
	Config           *factoryconfig.LoadedFactoryConfig
	Runtime          phasedRuntimeDependencies
	Models           *modelservice.Service
	Workers          *workersservice.Service
	WorkerProvider   *runtimebuild.Service
	FactorySessions  *factorysessionsservice.Service
	DurableExecution factorysessionexecution.Service
}

// constructed is the typed result of one graph-construction phase. Resource is
// retained by a successful graph or closed if a later phase fails.
type constructed[T any] struct {
	Value    T
	Resource io.Closer
}

// phasedBuilders explicitly names each fallible construction phase. phasedBuilders must
// construct collaborators without starting their runtime lifecycle.
type phasedBuilders struct {
	Persistence     func(context.Context, RuntimeInputs) (constructed[runtimepersist.Store], error)
	ModelWorkers    func(context.Context, phasedRuntimeDependencies) (constructed[phasedModelWorkerServices], error)
	FactorySessions func(
		context.Context,
		phasedRuntimeDependencies,
		phasedModelWorkerServices,
	) (constructed[phasedFactorySessionServices], error)
	Transports func(context.Context, phasedTransportDependencies) (constructed[TransportLifecycles], error)
	Sidecars   func(context.Context, phasedSidecarDependencies) (constructed[SidecarLifecycles], error)
}

// phasedInputs names the injected phases used only by the internal failure
// harness. The public Build API does not expose these callbacks.
type phasedInputs struct {
	Config  *factoryconfig.LoadedFactoryConfig
	Runtime RuntimeInputs
	Build   phasedBuilders
}

// phasedGraph is the internal observable result used by the construction
// failure harness. It is deliberately not part of the production API.
type phasedGraph struct {
	Config            *factoryconfig.LoadedFactoryConfig
	Runtime           phasedRuntimeDependencies
	Models            *modelservice.Service
	Workers           *workersservice.Service
	WorkerProvider    *runtimebuild.Service
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
	Transport         phasedTransportDependencies
	Transports        TransportLifecycles
	Sidecars          SidecarLifecycles
	resources         *resourceSet
}

func (g *phasedGraph) Close() error {
	if g == nil || g.resources == nil {
		return nil
	}
	return g.resources.Close()
}

// Graph is the immutable-after-build process application graph. All fields are
// eagerly assigned by Build and return stable collaborator identity.
type Graph struct {
	core              *runtimehost.Core
	Config            *factoryconfig.LoadedFactoryConfig
	Runtime           RuntimeInputs
	RuntimeLog        runtimehost.RuntimeLogDiagnostics
	Models            apisurface.ModelAPI
	Workers           *workersservice.Service
	WorkerProvider    *runtimebuild.Service
	SessionRegistry   *factorysessions.Registry
	Persistence       runtimepersist.Store
	FactoryDefinition apisurface.FactorySaveAPI
	FactorySessions   apisurface.SessionAPI
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

// Lifecycles exposes only the named activation edges consumed by initializer.
func (g *Graph) Lifecycles() initializer.ApplicationLifecycles {
	if g == nil {
		return initializer.ApplicationLifecycles{}
	}
	return initializer.ApplicationLifecycles{
		API: g.Transports.API, CLI: g.Transports.CLI, MCP: g.Transports.MCP,
		Runtime: g.Sidecars.Runtime, Workers: g.Sidecars.Workers, Dashboard: g.Sidecars.Dashboard,
	}
}

// RuntimeLogMetadata returns immutable startup diagnostics without exposing
// the runtime host that owns the underlying sinks.
func (g *Graph) RuntimeLogMetadata() runtimehost.RuntimeLogDiagnostics {
	if g == nil {
		return runtimehost.RuntimeLogDiagnostics{}
	}
	return g.RuntimeLog
}

func validatePhasedInputs(inputs phasedInputs) error {
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
