package packagedfactorycatalog

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswirevalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire/validation"
)

const workTypeMissingCompletionStateCode = "factory.workType.missingCompletionState"

// FilterFirstPartyLifecycleBridgeTargets keeps the general customer validator
// strict. Only the first-party catalog may accept the two named lifecycle
// bridges whose direct runtime behavior is covered by the packaged loop and
// quorum contract tests.
func FilterFirstPartyLifecycleBridgeTargets(
	slug string,
	cfg *factorydefinitions.FactoryConfig,
	result factorydefinitions.ValidationResult,
) factorydefinitions.ValidationResult {
	if cfg == nil || !factorydefinitions.IsPetriOrchestratorFactory(cfg) {
		return result
	}

	filtered := make([]factorydefinitions.ValidationTarget, 0, len(result.Targets))
	for _, target := range result.Targets {
		if target.Code == workTypeMissingCompletionStateCode &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorkType &&
			firstPartyLifecycleBridgeName(slug, cfg, target.Subject.ID) != "" {
			continue
		}
		filtered = append(filtered, target)
	}
	result.Targets = filtered
	return result
}

// validateFactoryDefinitionForCatalog validates one authored first-party
// definition with the strict shared validator, then applies only the named
// catalog lifecycle bridges.
func validateFactoryDefinitionForCatalog(
	slug string,
	cfg *factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	return FilterFirstPartyLifecycleBridgeTargets(
		slug,
		cfg,
		factorydefinitionswirevalidation.ValidateFactoryDefinition(cfg),
	)
}
