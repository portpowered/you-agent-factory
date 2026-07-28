// Package wire constructs the Automations script-poller subservice.
package wire

import (
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollersservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/internal/service"
)

// NewService constructs an inert script-poller service with injected runtime
// dependencies. Construction never invokes the supplied functions.
func NewService(dependencies scriptpollers.Dependencies) scriptpollers.Service {
	return scriptpollersservice.New(dependencies)
}
