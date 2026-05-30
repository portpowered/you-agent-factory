package validation

import "github.com/portpowered/infinite-you/pkg/interfaces"

const validationRoot = "factory"

// ValidateStructural runs shared structural validation without work-type outcome
// invariants that legacy mapper fixtures may omit until they are migrated.
func ValidateStructural(cfg *interfaces.FactoryConfig) Result {
	if cfg == nil {
		return Result{}
	}
	var targets []Target
	targets = append(targets, duplicateIdentifierTargets(cfg)...)
	targets = append(targets, duplicateWorkStateTargets(cfg)...)
	targets = append(targets, danglingReferenceTargets(cfg)...)
	targets = append(targets, invalidPlaceReferenceTargets(cfg)...)
	targets = append(targets, conflictingWorkstationOutputTargets(cfg)...)
	targets = append(targets, missingOutcomeRouteTargets(cfg)...)
	return Result{Targets: targets}
}

// Validate runs structural factory validation for a complete factory definition and
// returns aggregated canonical targets.
func Validate(cfg *interfaces.FactoryConfig) Result {
	result := ValidateStructural(cfg)
	if cfg == nil {
		return result
	}
	result.Targets = append(result.Targets, missingWorkTypeOutcomeStateTargets(cfg)...)
	result.Targets = append(result.Targets, missingTerminalCompletionPathTargets(cfg)...)
	return result
}
