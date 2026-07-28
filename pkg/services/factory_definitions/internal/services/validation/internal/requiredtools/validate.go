// Package requiredtools owns Factory Definition declarative required-tool
// validation behind the private validation subservice.
package requiredtools

import (
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Validate runs declarative required-tool validation and returns Definition-owned
// targets with stable codes and severity.
func Validate(
	cfg *factorydefinitions.FactoryConfig,
	checker factorydefinitions.RequiredToolChecker,
) factorydefinitions.ValidationResult {
	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
