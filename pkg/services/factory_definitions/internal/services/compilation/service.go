// Package compilation defines the Factory Definitions-owned private nested
// subservice for authored/canonical → normalized effective-source compile.
// Cross-service peers continue to call the public Definitions root
// CompileEffectiveFactorySource command; they do not import this package.
package compilation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service is the singular compilation subservice-root contract for the
// CTR-DEF compile/load-effective slice. It converts authored/canonical Factory
// source into one normalized EffectiveFactorySource without starting session
// or runtime lifecycle and without owning catalog, authoring_layout,
// validation, snapshots, or distribution responsibilities.
type Service interface {
	CompileEffectiveFactorySource(
		context.Context,
		factorydefinitions.CompileEffectiveFactorySourceRequest,
	) (factorydefinitions.CompileEffectiveFactorySourceResult, error)
}
