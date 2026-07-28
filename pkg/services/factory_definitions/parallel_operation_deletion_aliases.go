package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

// Deletion-only aliases retain temporary parallel catalog operation surfaces
// replaced by singular Service request/result operations in CLN-DEF-CONTRACTS
// story 006. Peers must use Service.ListNamedFactories, ResolveNamedFactory,
// DeleteNamedFactory, and GetCurrentFactoryPointer instead; remove this file
// when pkg/wire and remaining construction surfaces finish cutover.

type (
	NamedFactoryCatalog             = contracts.NamedFactoryCatalog
	CurrentFactoryDirectoryResolver = contracts.CurrentFactoryDirectoryResolver
)
