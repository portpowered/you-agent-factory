// Package wire constructs the Automations script-poller subservice.
package wire

import (
	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// NewService constructs inert script-poller supervision from direct collaborators.
func NewService(
	logger *zap.Logger,
	scheduler clockwork.Clock,
	commandRunner platformprocess.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	cursors cursorscopes.CursorScopes,
) scriptpollers.Service {
	return scriptpollersservice.New(logger, scheduler, commandRunner, resolveTemplates, executionPolicy, cursors)
}
