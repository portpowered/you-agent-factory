// Package wire constructs the Automations script-poller subservice.
package wire

import (
	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewService constructs an inert script-poller service with injected runtime
// dependencies. Construction never invokes the supplied functions.
func NewService(
	logger func(workstationName, workerName string) *zap.Logger,
	clock func() clockwork.Clock,
	commandRunner func() workers.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) scriptpollers.Service {
	return scriptpollersservice.New(
		logger,
		clock,
		commandRunner,
		resolveTemplates,
		executionPolicy,
	)
}
