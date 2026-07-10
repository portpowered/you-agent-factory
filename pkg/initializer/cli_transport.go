package initializer

import (
	"context"
)

// CLITransport bundles initializer-produced domain services with the session
// runtime host used by CLI local in-process startup without constructing root
// FactoryService at the composition boundary.
type CLITransport struct {
	Services *Services
	Host     *SessionRuntimeHost
}

// InitializeCLITransport loads factory configuration, composes domain services,
// and returns the transport bundle used by local in-process CLI startup.
func InitializeCLITransport(ctx context.Context, cfg *Config) (*CLITransport, error) {
	core, err := BuildCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	host := NewSessionRuntimeHostFromCore(core, cfg)
	return &CLITransport{
		Services: servicesFromCoreWithModels(core, host.ModelService()),
		Host:     host,
	}, nil
}

// Runner returns the local in-process runtime seam used by pkg/cli/run.
func (t *CLITransport) Runner() LocalRuntimeRunner {
	if t == nil || t.Host == nil {
		return nil
	}
	return t.Host.LocalRuntimeRunner()
}
