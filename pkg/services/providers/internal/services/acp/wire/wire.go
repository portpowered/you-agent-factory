// Package wire constructs the parent-private ACP service.
package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	acpservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service"
)

func NewService(
	integrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
	stdioPipes platformprocess.StdioPipeFactory,
	scheduler platformclock.TimerSource,
	logger logging.Logger,
) (acp.ContinuationService, error) {
	return acpservice.New(integrations, commandFactory, locator, stdioPipes, scheduler, logger)
}
