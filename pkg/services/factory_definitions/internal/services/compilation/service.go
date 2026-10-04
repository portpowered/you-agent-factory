// Package compilation defines the Factory Definitions-owned private compilation
// capability for converting authored or canonical Factory source into one
// normalized effective loaded source without running the Factory.
//
// The public surface exposes only CTR-DEF compile vocabulary and exact injected
// load/encode ports. It does not declare Runtime/Petri types, peer service
// implementations, Wire/root construction ownership, or sibling catalog/
// authoring_layout/validation/snapshots_portability/distribution APIs.
package compilation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns effective-source compilation behind the CTR-DEF root compile
// slice.
type Service interface {
	LoadCanonicalFactorySource(
		[]byte,
		factorydefinitions.WorkstationLoader,
	) (factorydefinitions.MutableLoadedFactorySource, error)
	CompileEffectiveFactorySource(
		context.Context,
		factorydefinitions.CompileEffectiveFactorySourceRequest,
	) (factorydefinitions.CompileEffectiveFactorySourceResult, error)
}
