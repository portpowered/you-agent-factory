// Package validationentry maps and validates generated Factory payloads
// through the canonical Factory validation owner.
package validationentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	if err := factoryconfig.ValidatePortableLayoutBoundaryJSON([]byte(*snapshot)); err != nil {
		return portableLayoutTopologyError(err)
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
		if _, ok := PortableLayoutValidationTarget(err); ok {
			return portableLayoutTopologyError(err)
		}
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

// PortableLayoutValidationTarget converts a path-bearing raw layout boundary
// failure into the canonical Factory validation vocabulary.
func PortableLayoutValidationTarget(err error) (factoryvalidation.Target, bool) {
	var layoutErr *factoryconfig.PortableLayoutValidationError
	if !errors.As(err, &layoutErr) {
		return factoryvalidation.Target{}, false
	}
	path := "factory." + strings.TrimPrefix(layoutErr.Path, "factory.")
	code := factoryvalidation.CodeLayoutInvalidValue
	if strings.Contains(path, ".position") || strings.Contains(path, ".size") {
		code = factoryvalidation.CodeLayoutInvalidGeometry
	} else if strings.Contains(layoutErr.Message, "Factory embedded-image budget") {
		code = factoryvalidation.CodeLayoutImageBudgetExceeded
	}
	subjectID := strings.TrimPrefix(path, "factory.layout.")
	if subjectID == path || subjectID == "" {
		subjectID = "layout"
	}
	return factoryvalidation.Target{
		Code:     code,
		Severity: factoryvalidation.SeverityError,
		Message:  layoutErr.Message,
		Subject: factoryvalidation.Subject{
			Type:     factoryvalidation.SubjectTypeFactory,
			ID:       subjectID,
			Location: factoryvalidation.SubjectLocationDefinition,
		},
		Path: path,
	}, true
}

func portableLayoutTopologyError(err error) error {
	target, ok := PortableLayoutValidationTarget(err)
	if !ok {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	return apisurface.NewTopologyValidationError(
		"Factory layout contains an invalid authored value.",
		apisurface.FactoryValidationTargetsToAPI([]factoryvalidation.Target{target}),
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
