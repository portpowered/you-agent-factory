package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// APITransport bundles initializer-produced domain services with the session
// runtime host used by API startup without constructing root FactoryService at
// the composition boundary.
type APITransport struct {
	Services *Services
	Host     *SessionRuntimeHost
	surface  apisurface.SessionAPISurface
}

// InitializeAPITransport loads factory configuration, composes domain services,
// and returns the transport bundle used to wire API handler dependencies.
func InitializeAPITransport(ctx context.Context, cfg *Config) (*APITransport, error) {
	core, err := BuildCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	services := servicesFromCore(core)
	host := NewSessionRuntimeHostFromCore(core, cfg)
	surface, err := composeSessionAPISurface(services, host)
	if err != nil {
		return nil, err
	}
	return &APITransport{Services: services, Host: host, surface: surface}, nil
}

func composeSessionAPISurface(
	services *Services,
	host *SessionRuntimeHost,
) (apisurface.SessionAPISurface, error) {
	var model apisurface.ModelAPI
	if services != nil {
		model = services.Models
	}
	var session apisurface.SessionAPI
	var factoryDefinition apisurface.FactorySaveAPI
	var invocation apisurface.InvocationAPI
	var durableExecution apisurface.DurableSessionAPI
	if host != nil {
		session = host.SessionAPI()
		factoryDefinition = host.FactoryDefinitionAPI()
		invocation = host.InvocationAPI()
		durableExecution = host.DurableExecutionAPI()
	}
	return apisurface.NewSessionAPISurface(
		session,
		model,
		factoryDefinition,
		invocation,
		durableExecution,
	)
}

// SessionAPISurface returns handler dependencies for api.NewServer.
func (t *APITransport) SessionAPISurface() apisurface.SessionAPISurface {
	if t == nil {
		return nil
	}
	return t.surface
}

// Run starts the session runtime loop for service-mode API hosting.
func (t *APITransport) Run(ctx context.Context) error {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host.RunWithAPISurface(ctx, t.surface)
}
