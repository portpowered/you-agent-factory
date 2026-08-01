// Package orchestrator owns Factory Definition orchestrator/strategy validation
// behind the private validation subservice.
package orchestrator

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
)

// Validate runs orchestrator configuration validation and returns Definition-owned
// targets. Runtime-owned JavaScript semantics are obtained only through the
// injected orchestrator validator port.
func Validate(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
	validator factorydefinitions.OrchestratorDefinitionValidator,
	workflowSourceReader factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	result := impl.ValidateOrchestratorTargets(cfg)
	if validator == nil || cfg == nil || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
	}
	result.Targets = append(
		result.Targets,
		validator.ValidateJavaScriptFactoryDefinition(
			ctx,
			cfg.Orchestrator.JavaScript,
			workflowSourceReader,
		)...,
	)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
