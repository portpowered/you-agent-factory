package wire

import (
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// CompositionOption configures optional Factory Definitions composition ports.
type CompositionOption = factorydefinitionsinternal.CompositionOption

// WithDistributionScaffold wires scaffold creation through Distribution without
// changing the primary service constructor surface used by process-root Wire.
func WithDistributionScaffold(
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) CompositionOption {
	return factorydefinitionsinternal.WithDistributionScaffold(
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
}
