package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/service"
)

// CLITransport bundles initializer-produced domain services with the session
// runtime host used by CLI local in-process startup without constructing root
// FactoryService at the composition boundary.
type CLITransport struct {
	Services *Services
	Host     *service.SessionRuntimeHost
}

// InitializeCLITransport loads factory configuration, composes domain services,
// and returns the transport bundle used by local in-process CLI startup.
func InitializeCLITransport(ctx context.Context, cfg *Config) (*CLITransport, error) {
	core, err := service.BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &CLITransport{
		Services: servicesFromCore(core),
		Host:     service.NewSessionRuntimeHostFromCore(core, cfg),
	}, nil
}

// Runner returns the session runtime shell used by pkg/cli/run local paths.
func (t *CLITransport) Runner() *service.FactoryService {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host.FactoryService()
}
