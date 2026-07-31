// Package wire constructs the parent-private ACP service.
package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	acpservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service"
)

func NewService(
	integrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
) (acp.Service, error) {
	return acpservice.New(integrations, commandFactory, locator)
}
