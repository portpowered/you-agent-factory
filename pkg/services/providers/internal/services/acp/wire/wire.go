// Package wire constructs the parent-private ACP service.
package wire

import (
	"fmt"

	"github.com/mattn/go-shellwords"
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
	commands := make(map[providers.ID]acpservice.Command, len(integrations))
	for _, integration := range integrations {
		parts, err := shellwords.Parse(integration.Command)
		if err != nil || len(parts) == 0 {
			return nil, fmt.Errorf("construct ACP provider %q: invalid command", integration.Name)
		}
		commands[integration.Name] = acpservice.Command{Name: parts[0], Args: append([]string(nil), parts[1:]...)}
	}
	return acpservice.New(commands, commandFactory, locator)
}
