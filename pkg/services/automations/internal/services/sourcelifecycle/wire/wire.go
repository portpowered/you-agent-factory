// Package wire constructs the independent Automation source lifecycle owner.
package wire

import (
	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	lifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

func NewService(logger *zap.Logger, clock clockwork.Clock, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers, cursors cursorscopes.CursorScopes,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) sourcelifecycle.SourceLifecycle {
	return lifecycle.New(logger, clock, scriptPollers, cronService, filesystemWatchers, hostedPollers, cursors, executionPolicy)
}
