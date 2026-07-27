// Package topology owns Factory Definition graph topology validation behind the
// private validation subservice.
package topology

import (
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

// Validate runs graph topology validation and returns Definition-owned targets
// with stable codes and severity.
func Validate(cfg *factorycontracts.FactoryConfig) factorycontracts.ValidationResult {
	result := factoryvalidation.ValidateGraphTopology(cfg)
	return factorycontracts.ValidationResult{Targets: append([]factorycontracts.ValidationTarget(nil), result.Targets...)}
}
