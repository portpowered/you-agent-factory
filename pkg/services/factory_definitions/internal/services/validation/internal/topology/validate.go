// Package topology owns Factory Definition graph topology validation behind the
// private validation subservice.
package topology

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

// Validate runs graph topology validation and returns Definition-owned targets
// with stable codes and severity.
func Validate(cfg *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	result := impl.ValidateGraphTopology(cfg)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
