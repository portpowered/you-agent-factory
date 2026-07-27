// Package requiredtools owns Factory Definition declarative required-tool
// validation behind the private validation subservice.
package requiredtools

import (
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// Validate runs declarative required-tool validation and returns Definition-owned
// targets with stable codes and severity.
func Validate(
	cfg *factorycontracts.FactoryConfig,
	checker factorycontracts.RequiredToolChecker,
) factorycontracts.ValidationResult {
	result := factoryvalidation.ValidateDeclarativeRequiredTools(cfg, checker)
	return factorycontracts.ValidationResult{Targets: append([]factorycontracts.ValidationTarget(nil), result.Targets...)}
}
