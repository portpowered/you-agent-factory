package factorysave

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
)

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factoryconfig.WorkstationLoader) error {
	result, err := validationentry.ValidateFactoryAPI(context.Background(), submitted, factoryvalidation.Options{
		Profile:           factoryvalidation.ProfilePrePersist,
		WorkstationLoader: workstationLoader,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	if !result.HasBlockingTargets() {
		return nil
	}
	return topologyValidationErrorFromTargets(result.BlockingTargets())
}

func topologyValidationErrorFromTargets(targets []factoryvalidation.Target) *apisurface.TopologyValidationError {
	return apisurface.NewTopologyValidationError(
		"Factory topology contains invalid graph references.",
		factoryvalidation.ToValidationTargets(targets),
	)
}

func validateUpsertNamedFactoryRequest(
	request factoryapi.Factory,
	workstationLoader factoryconfig.WorkstationLoader,
) error {
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return err
	}
	return validateEditableFactoryTopology(request, workstationLoader)
}
