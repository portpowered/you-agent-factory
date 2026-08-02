// Package validation defines the Factory Definitions-owned private validation
// capability for structural, topology, pre-persist, required-tool,
// worker/workstation compatibility, and layout-pruning validation behind the
// CTR-DEF root validate slice.
//
// The public surface exposes only CTR-DEF validation vocabulary and exact
// injected Runtime and canonical-load ports. It does not declare public Petri
// types, peer service implementations, Wire/root construction ownership, or
// sibling catalog/authoring_layout/compilation/snapshots_portability/
// distribution APIs.
package validation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns Factory Definition validation behind the CTR-DEF root validate
// slice.
type Service interface {
	ValidateStructuralFactoryDefinition(
		context.Context,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest,
	) (factorydefinitions.ValidateStructuralFactoryDefinitionResult, error)
	ValidateEffectiveFactoryDefinition(
		context.Context,
		factorydefinitions.ValidateEffectiveFactoryDefinitionRequest,
	) (factorydefinitions.ValidateEffectiveFactoryDefinitionResult, error)
}
