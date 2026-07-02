package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
)

// Services exposes initializer-produced domain collaborators for in-process callers.
type Services struct {
	core              *Core
	Sessions          *factorysessions.Registry
	FactoryDefinition service.FactoryDefinitionService
	Models            service.ModelService
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
	return service.StartupWorkerConfigFromCore(s.core, name)
}

func servicesFromCore(core *Core) *Services {
	if core == nil {
		return nil
	}
	return &Services{
		core:              core,
		Sessions:          core.Sessions(),
		FactoryDefinition: service.NewFactoryDefinitionServiceFromCore(core),
		Models:            service.NewModelServiceFromCore(core),
		Workers:           core.HostedWorkers(),
		RuntimeHost:       core.RuntimeBuild(),
	}
}
