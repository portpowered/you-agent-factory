package apisurface

import (
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// ValidateWritableNamedFactoryName enforces the public named-factory contract
// for create/import paths. The reserved default-current identifier is valid for
// readback only and must never be persisted as a customer-named factory.
func ValidateWritableNamedFactoryName(name factoryapi.FactoryName) error {
	if err := factoryconfig.ValidateNamedFactoryName(string(name)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidNamedFactoryName, err)
	}
	if name == DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", ErrInvalidNamedFactoryName, name)
	}
	return nil
}
