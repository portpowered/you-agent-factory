package validation

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// ValidateOrchestratorTargets runs orchestrator/strategy configuration validation
// and returns Definition-owned targets with stable codes and severity.
func ValidateOrchestratorTargets(cfg *interfaces.FactoryConfig) Result {
	return Result{Targets: append([]Target(nil), OrchestratorTargets(cfg)...)}
}
