// Package editable owns validation of detached, editable Factory definitions.
package editable

import (
	"context"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

// ValidateSnapshot applies the canonical pre-persist rules to one detached
// Factory definition. It returns service-owned validation errors; transports
// are responsible for mapping them into their public response contracts.
func ValidateSnapshot(
	ctx context.Context,
	snapshot *interfaces.FactorySnapshot,
	workstationLoader interfaces.WorkstationLoader,
	mapRequest interfaces.EditableFactoryValidationRequestMapper,
	validate interfaces.DefinitionValidationOperation,
) error {
	if snapshot == nil {
		return fmt.Errorf(
			"%w: Factory snapshot is required",
			interfaces.ErrInvalidNamedFactory,
		)
	}
	if mapRequest == nil || validate == nil {
		return fmt.Errorf(
			"%w: Factory definition adapters are required",
			interfaces.ErrInvalidNamedFactory,
		)
	}

	request, err := mapRequest(snapshot, workstationLoader)
	if err != nil {
		return fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	request.Profile = interfaces.ValidationProfilePrePersist
	result, err := validate.ValidateDefinition(ctx, request)
	if err != nil {
		if errors.Is(err, interfaces.ErrInvalidNamedFactory) {
			return err
		}
		return fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	if !result.HasBlockingTargets() {
		return nil
	}
	return interfaces.NewValidationTopologyError(
		interfaces.DefaultTopologyValidationMessage,
		result.BlockingTargets(),
	)
}
