// Package wire constructs the Automations script-poller subservice.
package wire

import (
	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewService constructs an inert script-poller service with direct runtime
// collaborators.
func NewService(
	logger *zap.Logger,
	clock clockwork.Clock,
	commandRunner workers.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
) scriptpollers.Service {
	return scriptpollersservice.New(
		logger,
		clock,
		commandRunner,
		resolveTemplates,
	)
}
