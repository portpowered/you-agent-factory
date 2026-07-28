package impl

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"


// ValidateOrchestratorTargets runs orchestrator/strategy configuration validation
// and returns Definition-owned targets with stable codes and severity.
func ValidateOrchestratorTargets(cfg *factorydefinitions.FactoryConfig) Result {
	return Result{Targets: append([]Target(nil), OrchestratorTargets(cfg)...)}
}
