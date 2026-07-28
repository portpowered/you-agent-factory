package impl

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"


// ValidateGraphTopology runs graph reference validation for Petri-scoped factories
// and returns Definition-owned targets for dangling worker, resource, and route
// references without exposing Petri implementation types.
func ValidateGraphTopology(cfg *factorydefinitions.FactoryConfig) Result {
	if cfg == nil || !IsPetriOrchestratorValidationScope(cfg) {
		return Result{}
	}
	var targets []Target
	targets = append(targets, danglingReferenceTargets(cfg)...)
	targets = append(targets, invalidPlaceReferenceTargets(cfg)...)
	return Result{Targets: targets}
}
