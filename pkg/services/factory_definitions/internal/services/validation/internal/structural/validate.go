// Package structural owns Factory Definition structural validation behind the
// private validation subservice.
package structural

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
)

// Validate runs structural Factory Definition validation and returns
// Definition-owned targets with stable codes and severity.
func Validate(cfg *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	result := impl.ValidateStructural(cfg)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
