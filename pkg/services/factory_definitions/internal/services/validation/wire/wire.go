// Package wire constructs the Factory Definitions validation subservice from
// exact injected validation and canonical-load ports.
package wire

import (
	"fmt"

	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/contracts"
	validationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/service"
)

// NewService constructs the private validation subservice from exact injected
// validation-operation and canonical-load ports. Each collaborator is a direct
// argument so this constructor does not select Runtime/Petri implementations
// or take Wire/root construction ownership.
func NewService(
	operations validationcontracts.DefinitionValidationOperation,
	effective validationcontracts.EffectiveDefinitionValidationOperation,
	loadCanonical validationcontracts.CanonicalFactoryLoader,
	requiredToolChecker validationcontracts.RequiredToolChecker,
	orchestratorValidator validationcontracts.OrchestratorDefinitionValidator,
) (validationservice.Service, error) {
	if operations == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: definition validation operation is required")
	}
	if effective == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: effective definition validation operation is required")
	}
	if loadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: canonical Factory loader is required")
	}
	service := validationserviceimpl.New(
		operations,
		effective,
		loadCanonical,
		requiredToolChecker,
		orchestratorValidator,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions validation: implementation rejected its dependencies")
	}
	return service, nil
}
