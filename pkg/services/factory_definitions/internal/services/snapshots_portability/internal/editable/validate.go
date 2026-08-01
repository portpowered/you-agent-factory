// Package editable owns validation of detached, editable Factory definitions
// inside the snapshots_portability subservice.
package editable

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ValidateSnapshot applies the canonical pre-persist rules to one detached
// Factory definition. It returns service-owned validation errors; transports
// are responsible for mapping them into their public response contracts.
func ValidateSnapshot(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
	workstationLoader factorydefinitions.WorkstationLoader,
	mapRequest factorydefinitions.EditableFactoryValidationRequestMapper,
	validate factorydefinitions.DefinitionValidationOperation,
) error {
	if snapshot == nil {
		return fmt.Errorf(
			"%w: Factory snapshot is required",
			factorydefinitions.ErrInvalidNamedFactory,
		)
	}
	if mapRequest == nil || validate == nil {
		return fmt.Errorf(
			"%w: Factory definition adapters are required",
			factorydefinitions.ErrInvalidNamedFactory,
		)
	}

	request, err := mapRequest(snapshot, workstationLoader)
	if err != nil {
		return fmt.Errorf("%w: %v", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	request.Profile = factorydefinitions.ValidationProfilePrePersist
	result, err := validate.ValidateDefinition(ctx, request)
	if err != nil {
		if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
			return err
		}
		return fmt.Errorf("%w: %v", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if !result.HasBlockingTargets() {
		return nil
	}
	return factorydefinitions.NewValidationTopologyError(
		factorydefinitions.DefaultTopologyValidationMessage,
		result.BlockingTargets(),
	)
}
