// Package wire constructs the Workers-parent-private Agent Runner service.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/internal/service"
)

// NewService constructs one inert Agent Runner over the singular Providers
// root.
func NewService(providersService providers.Service) (agent.Service, error) {
	return internalservice.New(providersService)
}
