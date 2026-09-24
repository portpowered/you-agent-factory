package internal

import (
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
)

type compositionOptions struct {
	scaffoldInitializer         factoryroot.ScaffoldInitializer
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver
	discardNamedFactory         func(string, string) error
	removeCurrentFactoryPointer func(string) error
}

// CompositionOption configures optional Factory Definitions composition ports.
type CompositionOption func(*compositionOptions)

// WithDistributionScaffold wires scaffold creation through Distribution without
// changing the primary service constructor surface used by process-root Wire.
func WithDistributionScaffold(
	scaffoldInitializer factoryroot.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) CompositionOption {
	return func(opts *compositionOptions) {
		opts.scaffoldInitializer = scaffoldInitializer
		opts.scaffoldFactoryNameResolver = scaffoldFactoryNameResolver
	}
}

// WithActivationRollback supplies the durable inverse effects used by the
// request-scoped named activation transaction. The option keeps these effects
// at composition time instead of expanding the stable Definition Host port.
func WithActivationRollback(
	discardNamedFactory func(string, string) error,
	removeCurrentFactoryPointer func(string) error,
) CompositionOption {
	return func(opts *compositionOptions) {
		opts.discardNamedFactory = discardNamedFactory
		opts.removeCurrentFactoryPointer = removeCurrentFactoryPointer
	}
}

func applyCompositionOptions(options []CompositionOption) compositionOptions {
	var composition compositionOptions
	for _, option := range options {
		if option != nil {
			option(&composition)
		}
	}
	return composition
}
