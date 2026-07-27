// Package structural owns Factory Definition structural validation behind the
// private validation subservice.
package structural

import (
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// Validate runs structural Factory Definition validation and returns
// Definition-owned targets with stable codes and severity.
func Validate(cfg *factorycontracts.FactoryConfig) factorycontracts.ValidationResult {
	result := factoryvalidation.ValidateStructural(cfg)
	return factorycontracts.ValidationResult{Targets: append([]factorycontracts.ValidationTarget(nil), result.Targets...)}
}
