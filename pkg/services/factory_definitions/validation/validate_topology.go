package validation

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// ValidateGraphTopology runs graph reference validation for Petri-scoped factories
// and returns Definition-owned targets for dangling worker, resource, and route
// references without exposing Petri implementation types.
func ValidateGraphTopology(cfg *interfaces.FactoryConfig) Result {
	if cfg == nil || !IsPetriOrchestratorValidationScope(cfg) {
		return Result{}
	}
	var targets []Target
	targets = append(targets, danglingReferenceTargets(cfg)...)
	targets = append(targets, invalidPlaceReferenceTargets(cfg)...)
	return Result{Targets: targets}
}
