package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

// NewProviderFromService adapts the Providers root to the Workers Runner
// contract used by durable Factory Session construction. Execution enters
// through the private runners.Service.Execute boundary.
func NewProviderFromService(service providers.Service) (workers.Runner, error) {
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
	return registryRunner{registry: registry}, nil
}
