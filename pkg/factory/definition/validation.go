package factorydefinition

import (
	"context"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// ValidateUpsertNamedFactoryRequest rejects named-factory upsert payloads whose
// name or topology fail pre-persist validation.
func (s *Service) ValidateUpsertNamedFactoryRequest(request factoryapi.Factory) error {
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return err
	}
	return s.ValidateEditableFactoryTopology(request)
}

// ValidateEditableFactoryTopology rejects editable factory definitions whose
// topology fails pre-persist validation.
func (s *Service) ValidateEditableFactoryTopology(submitted factoryapi.Factory) error {
	if s == nil {
		return fmt.Errorf("factory definition service is required")
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if s.host != nil {
		workstationLoader = s.host.WorkstationLoader()
	}
	return validateEditableFactoryTopology(submitted, workstationLoader)
}

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
		apisurface.FactoryValidationTargetsToAPI(targets),
	)
}
