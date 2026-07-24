// Package wire constructs the owner-private Factory Session invocation
// capability from explicit runtime and effect ports.
package wire

import (
	invocationservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/internal/service"
)

// New constructs an inert invocation service and exposes only its contract.
// Opening / one-shot process lifecycle stays outside this nested subservice;
// callers bind prepare/command/observe ports through Dependencies only.
func New(deps invocationservice.Dependencies) (invocationservice.Service, error) {
	service, err := internalservice.New(deps)
	if err != nil {
		return nil, err
	}
	return service, nil
}
