// Package distribution defines the Factory Definitions-owned private
// distribution capability for built-in packaged Factory listing, packaged
// installation, and Factory scaffold creation. Cross-service consumers use the
// outer Factory Definitions root distribute slice instead of this contract.
//
// The public surface exposes only CTR-DEF distribute vocabulary and exact
// injected host-effect ports. It does not declare Runtime/Petri types, peer
// service implementations, Wire/root construction ownership, or sibling
// catalog/authoring_layout/compilation/validation/snapshots_portability APIs.
package distribution

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns built-in listing, packaged installation, and scaffold creation
// behind the CTR-DEF root distribute vocabulary.
type Service interface {
	ListBuiltInPackagedFactories(
		context.Context,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest,
	) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error)
	InstallPackagedFactory(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error)
	CreateFactoryScaffold(
		context.Context,
		factorydefinitions.CreateFactoryScaffoldRequest,
	) (factorydefinitions.CreateFactoryScaffoldResult, error)
}

// Dependencies are the exact host-effect ports required by distribution.
// They are supplied by Factory Definitions composition and never selected here:
// distribution does not choose host filesystem adapters or Wire/root constructors.
type Dependencies struct {
	Installer factorydefinitions.PackagedFactoryInstaller
	Scaffold  factorydefinitions.ScaffoldInitializer
}
