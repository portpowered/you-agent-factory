package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/orchestrator"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/requiredtools"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/structural"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/topology"
)

// Service is the private nested validation implementation behind the CTR-DEF
// root validate slice.
type Service struct {
	operations            validationcontracts.DefinitionValidationOperation
	effective             validationcontracts.EffectiveDefinitionValidationOperation
	loadCanonical         validationcontracts.CanonicalFactoryLoader
	requiredToolChecker   validationcontracts.RequiredToolChecker
	orchestratorValidator validationcontracts.OrchestratorDefinitionValidator
}

var _ validationservice.Service = (*Service)(nil)

// New constructs the validation implementation from exact injected ports.
func New(
	operations validationcontracts.DefinitionValidationOperation,
	effective validationcontracts.EffectiveDefinitionValidationOperation,
	loadCanonical validationcontracts.CanonicalFactoryLoader,
	requiredToolChecker validationcontracts.RequiredToolChecker,
	orchestratorValidator validationcontracts.OrchestratorDefinitionValidator,
) *Service {
	if operations == nil || effective == nil || loadCanonical == nil {
		return nil
	}
	return &Service{
		operations:            operations,
		effective:             effective,
		loadCanonical:         loadCanonical,
		requiredToolChecker:   requiredToolChecker,
		orchestratorValidator: orchestratorValidator,
	}
}

func (s *Service) ValidateStructuralFactoryDefinition(
	ctx context.Context,
	request factoryroot.ValidateStructuralFactoryDefinitionRequest,
) (factoryroot.ValidateStructuralFactoryDefinitionResult, error) {
	if err := s.requirePorts(); err != nil {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, err
	}
	canonical := bytes.TrimSpace(request.Canonical)
	if len(canonical) == 0 || bytes.Equal(canonical, []byte("{")) {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, factoryroot.ErrInvalidFactoryDefinitionPayload
	}
	cfg, err := s.factoryConfigFromCanonical(canonical)
	if err != nil {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, err
	}
	profileResult, err := s.operations.ValidateDefinition(ctx, factoryroot.DefinitionValidationRequest{
		Profile:                factoryroot.ResolveValidationProfile(request.Profile),
		Config:                 cfg,
		CanonicalPayload:       canonical,
		CanonicalFactoryLoader: s.loadCanonical,
	})
	if err != nil {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, err
	}
	result := mergeValidationResults(
		structural.Validate(cfg),
		topology.Validate(cfg),
		requiredtools.Validate(cfg, s.requiredToolChecker),
		orchestrator.Validate(ctx, cfg, s.orchestratorValidator, nil),
		profileResult,
	)
	return finishStructuralResult(result)
}

func (s *Service) ValidateEffectiveFactoryDefinition(
	ctx context.Context,
	request factoryroot.ValidateEffectiveFactoryDefinitionRequest,
) (factoryroot.ValidateEffectiveFactoryDefinitionResult, error) {
	if err := s.requirePorts(); err != nil {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, err
	}
	canonical := bytes.TrimSpace(request.Canonical)
	if len(canonical) == 0 {
		canonical = bytes.TrimSpace([]byte(request.Effective.ContentIdentity))
	}
	if len(canonical) == 0 || bytes.Equal(canonical, []byte("{")) {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, factoryroot.ErrInvalidFactoryDefinitionPayload
	}
	cfg, err := s.factoryConfigFromCanonical(canonical)
	if err != nil {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, err
	}
	result, err := s.effective.ValidateEffectiveDefinition(ctx, factoryroot.EffectiveDefinitionValidationRequest{
		Config: cfg,
	})
	if err != nil {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, err
	}
	return finishEffectiveResult(result)
}

func (s *Service) factoryConfigFromCanonical(
	canonical []byte,
) (*factoryroot.FactoryConfig, error) {
	loaded, err := s.loadCanonical(canonical, nil)
	if err != nil {
		if errors.Is(err, factoryroot.ErrInvalidNamedFactory) {
			return nil, factoryroot.ErrInvalidFactoryDefinitionPayload
		}
		return nil, err
	}
	if loaded == nil {
		return nil, factoryroot.ErrInvalidFactoryDefinitionPayload
	}
	cfg := loaded.FactoryConfig()
	if cfg == nil {
		return nil, factoryroot.ErrInvalidFactoryDefinitionPayload
	}
	return cfg, nil
}

func finishStructuralResult(
	result factoryroot.ValidationResult,
) (factoryroot.ValidateStructuralFactoryDefinitionResult, error) {
	rootResult := factoryroot.ValidationResult{Targets: append([]factoryroot.ValidationTarget(nil), result.Targets...)}
	if rootResult.HasBlockingTargets() {
		return factoryroot.ValidateStructuralFactoryDefinitionResult{}, &factoryroot.FactoryDefinitionValidationFailure{
			Validation: rootResult,
		}
	}
	return factoryroot.ValidateStructuralFactoryDefinitionResult{Validation: rootResult}, nil
}

func finishEffectiveResult(
	result factoryroot.ValidationResult,
) (factoryroot.ValidateEffectiveFactoryDefinitionResult, error) {
	rootResult := factoryroot.ValidationResult{Targets: append([]factoryroot.ValidationTarget(nil), result.Targets...)}
	if rootResult.HasBlockingTargets() {
		return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, &factoryroot.FactoryDefinitionValidationFailure{
			Validation: rootResult,
		}
	}
	return factoryroot.ValidateEffectiveFactoryDefinitionResult{Validation: rootResult}, nil
}

func mergeValidationResults(parts ...factoryroot.ValidationResult) factoryroot.ValidationResult {
	seen := make(map[string]struct{})
	var targets []factoryroot.ValidationTarget
	for _, part := range parts {
		for _, target := range part.Targets {
			signature := structuralTargetSignature(target)
			if _, ok := seen[signature]; ok {
				continue
			}
			seen[signature] = struct{}{}
			targets = append(targets, target)
		}
	}
	return factoryroot.ValidationResult{Targets: targets}
}

func structuralTargetSignature(target factoryroot.ValidationTarget) string {
	return target.Code + "|" +
		string(target.Severity) + "|" +
		string(target.Subject.Type) + "|" +
		target.Subject.ID + "|" +
		string(target.Subject.Location) + "|" +
		target.Path + "|" +
		target.Message
}

func (s *Service) requirePorts() error {
	if s == nil || s.operations == nil || s.effective == nil {
		return fmt.Errorf("Factory Definition validation collaborator is required")
	}
	if s.loadCanonical == nil {
		return fmt.Errorf("canonical Factory loader is required")
	}
	return nil
}
