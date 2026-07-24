// Package distribution defines the Factory Definitions-owned private
// distribution capability for built-in packaged Factory listing, packaged
// installation, and Factory scaffold creation. Cross-service consumers use the
// outer Factory Definitions root distribute slice instead of this contract.
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
