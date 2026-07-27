// Package wire constructs the Factory Definitions validation subservice from
// exact injected validation and canonical-load ports.
package wire

import (
	"fmt"

	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/service"
)

// NewService constructs the private validation subservice from exact injected
// validation-operation and canonical-load ports. Callers must supply
// Dependencies; this constructor does not select Runtime/Petri implementations
// or take Wire/root construction ownership.
func NewService(deps validationservice.Dependencies) (validationservice.Service, error) {
	if deps.Operations == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: definition validation operation is required")
	}
	if deps.Effective == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: effective definition validation operation is required")
	}
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: canonical Factory loader is required")
	}
	service := validationserviceimpl.New(deps.Operations, deps.Effective, deps.LoadCanonical)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: implementation rejected its dependencies")
	}
	return service, nil
}
