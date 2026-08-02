// Package contracts contains private compilation/loading ports. These
// capabilities are implementation details of Factory Definitions and never
// cross the public unary root.
package contracts

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

type CanonicalFactoryLoader func(
	[]byte,
	factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error)

type LoadedFactoryLoader func(
	string,
	factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error)

type LoadedFactorySourceFactory func(
	string,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RuntimeDefinitionLookup,
	[]factorydefinitions.PortableBundledFileReplacement,
) (factorydefinitions.MutableLoadedFactorySource, error)

type FactoryConfigEncoder func(*factorydefinitions.FactoryConfig) ([]byte, error)
