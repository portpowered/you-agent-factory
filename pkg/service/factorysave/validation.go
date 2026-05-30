package factorysave

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
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
	payload, err := json.Marshal(submitted)
	if err != nil {
		return fmt.Errorf("marshal editable factory payload: %w", err)
	}
	_, loadErr := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{
		WorkstationLoader: workstationLoader,
	})
	cfg, mapErr := factoryconfig.FactoryConfigFromOpenAPI(submitted)
	if mapErr != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, mapErr)
	}
	if loadErr != nil {
		if configload.IsInvalidNamedFactory(loadErr) {
			blocking := factoryvalidation.ValidateBlockingLoad(&cfg)
			if len(blocking.Targets) > 0 {
				return topologyValidationErrorFromTargets(blocking.Targets)
			}
		}
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, loadErr)
	}
	result := factoryvalidation.Validate(&cfg)
	if len(result.Targets) == 0 {
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
