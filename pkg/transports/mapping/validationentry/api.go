// Package validationentry maps and validates generated Factory payloads
// through the canonical Factory validation owner.
package validationentry

import (
	"context"
	"encoding/json"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ValidateEditableFactorySnapshot maps a detached Factory-owned definition at
// the transport boundary, then returns the established public save error shape.
func ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot, workstationLoader factoryconfig.WorkstationLoader) error {
	if snapshot == nil {
		return fmt.Errorf("%w: Factory snapshot is required", apisurface.ErrInvalidNamedFactory)
	}
	var submitted factoryapi.Factory
	if err := snapshot.Decode(&submitted); err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	result, err := ValidateFactoryAPI(context.Background(), submitted, factoryvalidation.Options{
		Profile:           factoryvalidation.ProfilePrePersist,
		WorkstationLoader: workstationLoader,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	if !result.HasBlockingTargets() {
		return nil
	}
	return apisurface.NewTopologyValidationError(
		"Factory topology contains invalid graph references.",
		apisurface.FactoryValidationTargetsToAPI(result.BlockingTargets()),
	)
}

// ValidateFactoryAPI is the single pre-mutation validator for OpenAPI factory
// payloads. Each invocation maps the payload once via FactoryConfigFromOpenAPI,
// then runs profile-specific checks without writing disk.
//
// ProfileTopology matches POST /factory-validations (structural validation only).
// ProfilePrePersist matches editable save and CLI save-from-file pre-checks
// (canonical JSON load plus blocking-load fallback, then structural validation).
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
		return validateFactoryTopologyFromAPI(factory, cfg, opts), nil
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
	return validateFactoryTopologyFromAPI(factory, cfg, opts), nil
}

func validateFactoryTopology(cfg interfaces.FactoryConfig, opts factoryvalidation.Options) factoryvalidation.Result {
	result := factoryvalidation.Validate(&cfg)
	if cfg.Orchestrator != nil && cfg.Orchestrator.JavaScript != nil && opts.WorkflowSourceReader != nil {
		result.Targets = append(result.Targets, factoryvalidation.WorkflowSourceTargets(cfg.Orchestrator.JavaScript, opts.WorkflowSourceReader)...)
	}
	return result
}

func validateFactoryTopologyFromAPI(factory factoryapi.Factory, cfg interfaces.FactoryConfig, opts factoryvalidation.Options) factoryvalidation.Result {
	result := validateFactoryTopology(cfg, opts)
	result.Targets = append(result.Targets, workerWorkstationCompatibilityTargetsFromAPI(factory)...)
	return result
}
