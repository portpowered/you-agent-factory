// Package initializer is the canonical composition root for factory config loading
// and domain service construction. It assembles session, factory-definition, model,
// worker, and runtime-host collaborators without constructing root pkg/service
// FactoryService at transport composition boundaries. API, CLI local in-process,
// and MCP serve paths consume initializer-produced transport bundles. Process
// startup graphs are constructed by pkg/wire and handed here only for lifecycle
// execution.
package initializer

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
)

// Services exposes initializer-produced domain collaborators for in-process callers.
type Services struct {
	core              *Core
	Sessions          *factorysessions.Registry
	FactoryDefinition FactoryDefinitionService
	Models            ModelService
	Workers           hostedworkers.Config
	RuntimeHost       *runtimebuild.Service
}

// Initialize loads factory configuration and composes runnable domain services
// without constructing root pkg/service.FactoryService.
func Initialize(ctx context.Context, cfg *Config) (*Services, error) {
	core, err := BuildCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return servicesFromCore(core), nil
}

// StartupWorkerConfig returns the named worker from the composed startup runtime.
func (s *Services) StartupWorkerConfig(name string) (*interfaces.WorkerConfig, bool) {
	if s == nil {
		return nil, false
	}
	return StartupWorkerConfigFromCore(s.core, name)
}

func servicesFromCore(core *Core) *Services {
	return servicesFromCoreWithModels(core, NewModelServiceFromCore(core))
}

func servicesFromCoreWithModels(core *Core, models ModelService) *Services {
	if core == nil {
		return nil
	}
	return &Services{
		core:              core,
		Sessions:          core.Sessions(),
		FactoryDefinition: NewFactoryDefinitionServiceFromCore(core),
		Models:            models,
		Workers:           core.HostedWorkers(),
		RuntimeHost:       core.RuntimeBuild(),
	}
}
