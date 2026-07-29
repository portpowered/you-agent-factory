package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

// NewProviderFromService adapts the Providers root to the retained Workers
// Provider port used by durable Factory Session construction.
func NewProviderFromService(service providers.Service) (workers.Provider, error) {
	if service == nil {
		return nil, fmt.Errorf("construct Worker provider: Providers service is required")
	}
	registry, err := runnerswire.NewAgentRegistry(runners.AgentDependencies{
		Providers: service,
		Publish:   func(workers.ProgressFragment) {},
	})
	if err != nil {
		return nil, fmt.Errorf("construct Worker provider: %w", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: runners.AgentIdentity})
	if err != nil {
		return nil, fmt.Errorf("construct Worker provider: %w", err)
	}
	return runnerProvider{runner: binding.Runner}, nil
}
