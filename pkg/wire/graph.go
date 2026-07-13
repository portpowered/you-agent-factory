package wire

import (
	"context"
	"fmt"
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

// RuntimeDependencies are the explicit process-scoped infrastructure inputs
// retained by an application graph.
type RuntimeDependencies struct {
	FactoryRootDir   string
	ExecutionBaseDir string
	Logger           *zap.Logger
	Clock            factory.Clock
	Persistence      runtimepersist.Store
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

// TransportDependencies is the typed set of domain collaborators injected
// into API, CLI, and MCP adapters. Its fields are the same instances exposed
// by Graph; it is not a service locator and performs no lazy construction.
type TransportDependencies struct {
	Models            *modelservice.Service
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
}

// Inputs explicitly names every collaborator assembled by Build. Domain-owned
// values are injected directly so graph tests and later production wiring do
// not depend on process globals or filesystem discovery.
type Inputs struct {
	Config            *factoryconfig.LoadedFactoryConfig
	Runtime           RuntimeDependencies
	Models            *modelservice.Service
	Workers           *workersservice.Service
	Provider          providerexecution.Executor
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
	Transports        TransportLifecycles
	Sidecars          SidecarLifecycles
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
}

// Build validates and assembles one application graph without activating any
// transport, runtime, scheduler, poller, or dashboard lifecycle.
func Build(ctx context.Context, inputs Inputs) (*Graph, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build application graph: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}
	if err := validateInputs(inputs); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}

	return &Graph{
		Config:            inputs.Config,
		Runtime:           inputs.Runtime,
		Models:            inputs.Models,
		Workers:           inputs.Workers,
		Provider:          inputs.Provider,
		FactoryDefinition: inputs.FactoryDefinition,
		FactorySessions:   inputs.FactorySessions,
		DurableExecution:  inputs.DurableExecution,
		Transport: TransportDependencies{
			Models:            inputs.Models,
			FactoryDefinition: inputs.FactoryDefinition,
			FactorySessions:   inputs.FactorySessions,
			DurableExecution:  inputs.DurableExecution,
		},
		Transports: inputs.Transports,
		Sidecars:   inputs.Sidecars,
	}, nil
}

func validateInputs(inputs Inputs) error {
	required := []struct {
		name    string
		missing bool
	}{
		{name: "config", missing: inputs.Config == nil},
		{name: "runtime.factoryRootDir", missing: strings.TrimSpace(inputs.Runtime.FactoryRootDir) == ""},
		{name: "runtime.logger", missing: inputs.Runtime.Logger == nil},
		{name: "runtime.clock", missing: inputs.Runtime.Clock == nil},
		{name: "runtime.persistence", missing: inputs.Runtime.Persistence == nil},
		{name: "models", missing: inputs.Models == nil},
		{name: "workers", missing: inputs.Workers == nil},
		{name: "provider", missing: inputs.Provider == nil},
		{name: "factoryDefinition", missing: inputs.FactoryDefinition == nil},
		{name: "factorySessions", missing: inputs.FactorySessions == nil},
		{name: "durableExecution", missing: inputs.DurableExecution == nil},
		{name: "transports.api", missing: inputs.Transports.API == nil},
		{name: "transports.mcp", missing: inputs.Transports.MCP == nil},
		{name: "sidecars.runtime", missing: inputs.Sidecars.Runtime == nil},
		{name: "sidecars.workers", missing: inputs.Sidecars.Workers == nil},
		{name: "sidecars.dashboard", missing: inputs.Sidecars.Dashboard == nil},
	}
	for _, dependency := range required {
		if dependency.missing {
			return fmt.Errorf("validate inputs: %s is required", dependency.name)
		}
	}
	return nil
}
