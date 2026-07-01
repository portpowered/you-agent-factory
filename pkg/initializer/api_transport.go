package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/service"
)

// APITransport bundles initializer-produced domain services with the session
// runtime host used by API startup without constructing root FactoryService at
// the composition boundary.
type APITransport struct {
	Services *Services
	Host     *service.SessionRuntimeHost
}

// InitializeAPITransport loads factory configuration, composes domain services,
// and returns the transport bundle used to wire API handler dependencies.
func InitializeAPITransport(ctx context.Context, cfg *Config) (*APITransport, error) {
	core, err := service.BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &APITransport{
		Services: servicesFromCore(core),
		Host:     service.NewSessionRuntimeHostFromCore(core, cfg),
	}, nil
}

// SessionAPISurface returns handler dependencies for api.NewServer.
func (t *APITransport) SessionAPISurface() apisurface.SessionAPISurface {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host.SessionAPISurface()
}

// Run starts the session runtime loop for service-mode API hosting.
func (t *APITransport) Run(ctx context.Context) error {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host.Run(ctx)
}

func servicesFromCore(core *service.FactoryCore) *Services {
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
