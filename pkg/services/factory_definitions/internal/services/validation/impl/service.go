package impl

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/contracts"
)

// Service implements the public Factory Definition validation boundary.
type Service struct {
	orchestrators validationcontracts.OrchestratorDefinitionValidator
	loadCanonical validationcontracts.CanonicalFactoryLoader
}

// New constructs Factory Definition validation with the runtime-owned
// orchestrator semantic validator supplied explicitly.
func New(
	orchestrators validationcontracts.OrchestratorDefinitionValidator,
	loadCanonical ...validationcontracts.CanonicalFactoryLoader,
) *Service {
	service := &Service{orchestrators: orchestrators}
	if len(loadCanonical) > 0 {
		service.loadCanonical = loadCanonical[0]
	}
	return service
}

// ValidateDefinition owns the complete profile-specific validation sequence.
// Callers supply already-mapped values and never call the lower-level
// validation phases themselves.
func (s *Service) ValidateDefinition(
	ctx context.Context,
	request factorydefinitions.DefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if s == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition validator is required")
	}
	if ctx == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition validation context is required")
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	if request.Config == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition config is required")
	}

	switch factorydefinitions.ResolveValidationProfile(request.Profile) {
	case factorydefinitions.ValidationProfilePrePersist:
		loadCanonical := s.loadCanonical
		if loadCanonical == nil {
			loadCanonical = request.CanonicalFactoryLoader
		}
		if loadCanonical == nil {
			return factorydefinitions.ValidationResult{}, fmt.Errorf("canonical Factory loader is required for pre-persist validation")
		}
		if len(request.CanonicalPayload) == 0 {
			return factorydefinitions.ValidationResult{}, fmt.Errorf("canonical Factory payload is required for pre-persist validation")
		}
		_, loadErr := loadCanonical(request.CanonicalPayload, request.WorkstationLoader)
		if loadErr != nil {
			if errors.Is(loadErr, factorydefinitions.ErrInvalidNamedFactory) {
				blocking := s.ValidateBlockingLoad(ctx, request.Config)
				if blocking.HasTargets() {
					return blocking, nil
				}
			}
			return factorydefinitions.ValidationResult{}, loadErr
		}
		if blocking := s.ValidateBlockingLoad(ctx, request.Config); blocking.HasTargets() {
			return blocking, nil
		}
	}

	result := s.Validate(ctx, request.Config, request.WorkflowSourceReader)
	result.Targets = append(result.Targets, SubmittedTaxonomyCompatibilityTargets(request.SubmittedTaxonomy)...)
	return result, nil
}

// ValidateSubmittedDefinition applies the fixed explicit-validation profile.
// This is the exact operation supplied to CLI and HTTP validation endpoints.
func (s *Service) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return s.ValidateDefinition(ctx, factorydefinitions.DefinitionValidationRequest{
		Profile:              factorydefinitions.ValidationProfileTopology,
		Config:               request.Config,
		WorkflowSourceReader: request.WorkflowSourceReader,
		SubmittedTaxonomy:    request.Taxonomy,
	})
}

// ValidateEffectiveDefinition applies the fixed effective-definition policy
// used by prompt invocation, including its required DEFAULT handling work type.
func (s *Service) ValidateEffectiveDefinition(
	ctx context.Context,
	request factorydefinitions.EffectiveDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if s == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition validator is required")
	}
	if request.Config == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition config is required")
	}
	if ctx == nil {
		return factorydefinitions.ValidationResult{}, fmt.Errorf("Factory Definition validation context is required")
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	result := s.Validate(ctx, request.Config, request.WorkflowSourceReader)
	result.Targets = append(result.Targets, WorkTypeHandlingBehaviorTargets(
		request.Config,
		WorkTypeHandlingBehaviorOptions{RequireDefault: true},
	)...)
	return result, nil
}

func (s *Service) Validate(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
	workflowSourceReader validationcontracts.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	result := Validate(cfg)
	if s == nil || s.orchestrators == nil || cfg == nil ||
		cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return result
	}
	result.Targets = append(
		result.Targets,
		s.orchestrators.ValidateJavaScriptFactoryDefinition(
			ctx,
			cfg.Orchestrator.JavaScript,
			workflowSourceReader,
		)...,
	)
	return result
}

func (s *Service) ValidateBlockingLoad(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	result := ValidateBlockingLoad(cfg)
	if s == nil || s.orchestrators == nil || cfg == nil ||
		cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return result
	}
	result.Targets = append(
		result.Targets,
		s.orchestrators.ValidateJavaScriptFactoryDefinition(
			ctx,
			cfg.Orchestrator.JavaScript,
			nil,
		)...,
	)
	return result
}

func (s *Service) ValidateTopology(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
	requiredToolChecker validationcontracts.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	result := NewConfigValidator(requiredToolChecker).Validate(cfg)
	result.Findings = append(
		result.Findings,
		FactoryDefinitionFindings(s.Validate(ctx, cfg, nil).Targets)...,
	)
	return *result
}

func (*Service) WorkerWorkstationBehaviorCompatibility(
	_ context.Context,
	cfg *factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return WorkerWorkstationBehaviorCompatibilityTargets(cfg)
}

func (*Service) WorkTypeHandlingBehavior(
	_ context.Context,
	cfg *factorydefinitions.FactoryConfig,
	requireDefault bool,
) []factorydefinitions.ValidationTarget {
	return WorkTypeHandlingBehaviorTargets(cfg, WorkTypeHandlingBehaviorOptions{
		RequireDefault: requireDefault,
	})
}

func (*Service) PruneLayout(
	_ context.Context,
	cfg *factorydefinitions.FactoryConfig,
	topology factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return PruneLayout(cfg, topology)
}
