package validation

import "github.com/portpowered/infinite-you/pkg/interfaces"

var deferredBlockingLoadOutcomeCodes = map[string]struct{}{
	CodeWorkstationMissingFailureRoute:   {},
	CodeWorkstationMissingRejectionRoute: {},
}

// ValidateBlockingLoad returns structural validation targets that should fail
// factory load before runtime definition merge. Outcome-route invariants remain
// on save and explicit validation paths until legacy fixtures migrate.
func ValidateBlockingLoad(cfg *interfaces.FactoryConfig) Result {
	result := ValidateStructural(cfg)
	if len(result.Targets) == 0 {
		return result
	}
	filtered := make([]Target, 0, len(result.Targets))
	for _, target := range result.Targets {
		if _, deferred := deferredBlockingLoadOutcomeCodes[target.Code]; deferred {
			continue
		}
		filtered = append(filtered, target)
	}
	result.Targets = filtered
	return result
}
