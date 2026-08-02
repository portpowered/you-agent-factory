// Package distribution defines the Factory Definitions-owned private distribute
// capability for built-in package catalog listing/resolve, packaged installation,
// and scaffold creation behind the CTR-DEF root distribute slice.
//
// The public surface exposes only CTR-DEF distribute vocabulary and exact
// injected host-effect ports. It does not declare Runtime/Petri types, peer
// service implementations, Wire/root construction ownership, filesystem/SQL/OS
// effect concrete types, or sibling catalog/authoring_layout/compilation/
// validation/snapshots_portability APIs.
package distribution

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns built-in package catalog, packaged installation, and scaffold
// creation behind the CTR-DEF root distribute slice.
type Service interface {
	ListBuiltInPackagedFactories(
		context.Context,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest,
	) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error)
	ResolveBuiltInPackagedFactory(
		context.Context,
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
	) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error)
	InstallPackagedFactory(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error)
	CreateFactoryScaffold(
		context.Context,
		factorydefinitions.CreateFactoryScaffoldRequest,
	) (factorydefinitions.CreateFactoryScaffoldResult, error)
}
