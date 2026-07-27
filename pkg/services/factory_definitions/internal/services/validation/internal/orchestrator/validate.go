// Package orchestrator owns Factory Definition orchestrator/strategy validation
// behind the private validation subservice.
package orchestrator

import (
	"context"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// Validate runs orchestrator configuration validation and returns Definition-owned
// targets. Runtime-owned JavaScript semantics are obtained only through the
// injected orchestrator validator port.
func Validate(
	ctx context.Context,
	cfg *factorycontracts.FactoryConfig,
	validator factorycontracts.OrchestratorDefinitionValidator,
	workflowSourceReader factorycontracts.WorkflowSourceReader,
) factorycontracts.ValidationResult {
	result := factoryvalidation.ValidateOrchestratorTargets(cfg)
	if validator == nil || cfg == nil || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return factorycontracts.ValidationResult{Targets: append([]factorycontracts.ValidationTarget(nil), result.Targets...)}
	}
	result.Targets = append(
		result.Targets,
		validator.ValidateJavaScriptFactoryDefinition(
			ctx,
			cfg.Orchestrator.JavaScript,
			workflowSourceReader,
		)...,
	)
	return factorycontracts.ValidationResult{Targets: append([]factorycontracts.ValidationTarget(nil), result.Targets...)}
}
