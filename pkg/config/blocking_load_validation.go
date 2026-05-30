package config

import (
	"fmt"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func validateBlockingFactoryLoad(cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return nil
	}
	result := factoryvalidation.ValidateBlockingLoad(cfg)
	if len(result.Targets) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%w: factory topology contains invalid graph references (%d blocking validation targets)",
		ErrInvalidNamedFactory,
		len(result.Targets),
	)
}
