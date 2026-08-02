// Package compilation defines the Factory Definitions-owned private compilation
// capability for converting authored or canonical Factory source into one
// normalized effective loaded source without running the Factory.
//
// Construction capabilities stay in this internal package and never cross the
// Factory Definitions root Service.
package compilation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns effective-source compilation behind the CTR-DEF root compile
// slice.
type Service interface {
	CompileEffectiveFactorySource(
		context.Context,
		factorydefinitions.CompileEffectiveFactorySourceRequest,
	) (factorydefinitions.CompileEffectiveFactorySourceResult, error)
}
