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
}

// InitializeAPITransport loads factory configuration, composes domain services,
// and returns the transport bundle used to wire API handler dependencies.
func InitializeAPITransport(ctx context.Context, cfg *Config) (*APITransport, error) {
	core, err := BuildCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	host := NewSessionRuntimeHostFromCore(core, cfg)
	return &APITransport{
		Services: servicesFromCoreWithModels(core, host.ModelService()),
		Host:     host,
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
