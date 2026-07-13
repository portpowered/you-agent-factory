package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	transportmapping "github.com/portpowered/infinite-you/pkg/transports/mapping"
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
	host := NewSessionRuntimeHostFromCore(core, cfg)
	services := servicesFromCoreWithModels(core, host.ModelService())
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
	return composeSessionAPISurfaceWithConstructor(services, host, transportmapping.NewSessionAPISurface)
}

type sessionAPISurfaceConstructor func(
	apisurface.SessionAPI,
	apisurface.ModelAPI,
	apisurface.FactorySaveAPI,
	apisurface.InvocationAPI,
	transportmapping.DurableSessionAPI,
) (apisurface.SessionAPISurface, error)

func composeSessionAPISurfaceWithConstructor(
	services *Services,
	host *SessionRuntimeHost,
	constructor sessionAPISurfaceConstructor,
) (apisurface.SessionAPISurface, error) {
	var model apisurface.ModelAPI
	if services != nil {
		model = services.Models
	}
	var session apisurface.SessionAPI
	var factoryDefinition apisurface.FactorySaveAPI
	var invocation apisurface.InvocationAPI
	var durableExecution transportmapping.DurableSessionAPI
	if host != nil {
		session = host.SessionAPI()
		factoryDefinition = host.FactoryDefinitionAPI()
		invocation = host.InvocationAPI()
		durableExecution = host.DurableExecutionAPI()
	}
	return constructor(
		session,
		model,
		factoryDefinition,
		invocation,
		durableExecution,
	)
}

// SessionAPISurface returns handler dependencies for transport/http.NewServer.
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
