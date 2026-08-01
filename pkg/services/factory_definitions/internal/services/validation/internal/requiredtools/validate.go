// Package requiredtools owns Factory Definition declarative required-tool
// validation behind the private validation subservice.
package requiredtools

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
)

// Validate runs declarative required-tool validation and returns Definition-owned
// targets with stable codes and severity.
func Validate(
	cfg *factorydefinitions.FactoryConfig,
	checker factorydefinitions.RequiredToolChecker,
) factorydefinitions.ValidationResult {
	result := impl.ValidateDeclarativeRequiredTools(cfg, checker)
	return factorydefinitions.ValidationResult{Targets: append([]factorydefinitions.ValidationTarget(nil), result.Targets...)}
}
