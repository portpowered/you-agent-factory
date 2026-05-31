package factorysave

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
)

func requireFreshEditableFactoryVersionAtRoot(
	host Host,
	baseVersion *factoryapi.HybridLogicalTimestamp,
	rootDir string,
	name factoryapi.FactoryName,
) error {
	if baseVersion == nil {
		return fmt.Errorf("%w: save request must include an advanced factory version", apisurface.ErrFactoryVersionStale)
	}
	currentVersion, err := host.CurrentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	if !isEditableFactoryVersionAdvanced(*baseVersion, currentVersion) {
		return fmt.Errorf("%w: submitted version logical=%d physical=%s must advance current logical=%d physical=%s",
			apisurface.ErrFactoryVersionStale,
			baseVersion.Logical,
			baseVersion.Physical.UTC().Format(time.RFC3339Nano),
			currentVersion.Logical,
			currentVersion.Physical.UTC().Format(time.RFC3339Nano),
		)
	}
	return nil
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical.Int64() + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: physical,
	}
}

func marshalPersistedFactoryPayload(
	sanitized factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) ([]byte, error) {
	persisted := sanitized
	persisted.Version = &version
	payload, err := json.Marshal(persisted)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return payload, nil
}

func isEditableFactoryVersionAdvanced(candidate, current factoryapi.HybridLogicalTimestamp) bool {
	return candidate.Logical > current.Logical && candidate.Physical.UTC().After(current.Physical.UTC())
}

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factoryconfig.WorkstationLoader) error {
	result, err := validationentry.ValidateFactoryAPI(context.Background(), submitted, factoryvalidation.Options{
		Profile:             factoryvalidation.ProfilePrePersist,
		WorkstationLoader:   workstationLoader,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	if !result.HasTargets() {
		return nil
	}
	return topologyValidationErrorFromTargets(result.Targets)
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
