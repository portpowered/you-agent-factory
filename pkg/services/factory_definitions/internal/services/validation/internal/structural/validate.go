// Package structural owns Factory Definition structural validation behind the
// private validation subservice.
package structural

import (
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Validate runs structural Factory Definition validation and returns
// Definition-owned targets with stable codes and severity.
func Validate(cfg *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	result := factoryvalidation.ValidateStructural(cfg)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
