// Package validationentry validates factoryapi.Factory payloads through the
// canonical validation package without creating an import cycle between
// pkg/factory/validation and pkg/config.
package validationentry

import (
	"context"
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ValidateFactoryAPI validates a factoryapi.Factory payload using the profile in
// opts. Callers receive aggregated canonical targets in factoryvalidation.Result;
// map errors are returned when OpenAPI mapping fails.
func ValidateFactoryAPI(ctx context.Context, factory factoryapi.Factory, opts factoryvalidation.Options) (factoryvalidation.Result, error) {
	_ = ctx

	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return factoryvalidation.Result{}, err
	}

	switch opts.ResolvedProfile() {
	case factoryvalidation.ProfilePrePersist:
		return validateFactoryPrePersist(factory, cfg, opts)
	default:
		return factoryvalidation.Validate(&cfg), nil
	}
}

func validateFactoryPrePersist(
	factory factoryapi.Factory,
	cfg interfaces.FactoryConfig,
	opts factoryvalidation.Options,
) (factoryvalidation.Result, error) {
	payload, err := json.Marshal(factory)
	if err != nil {
		return factoryvalidation.Result{}, fmt.Errorf("marshal factory: %w", err)
	}
	_, loadErr := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{
		WorkstationLoader: opts.WorkstationLoader,
	})
	if loadErr != nil {
		if configload.IsInvalidNamedFactory(loadErr) {
			blocking := factoryvalidation.ValidateBlockingLoad(&cfg)
			if blocking.HasTargets() {
				return blocking, nil
			}
		}
		return factoryvalidation.Result{}, loadErr
	}
	return factoryvalidation.Validate(&cfg), nil
}
