// Package wire constructs the Workers-parent-private Agent Runner service.
package wire

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/internal/service"
)

// NewService constructs one inert Agent Runner over the singular Providers
// root, the Workers-owned progress observation edge, and the Factory
// Definitions owner of decision-envelope interpretation.
func NewService(
	providersService providers.Service,
	publish workers.ProgressPublisher,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) (agent.Service, error) {
	return internalservice.New(providersService, publish, decisionEnvelopes...)
}
