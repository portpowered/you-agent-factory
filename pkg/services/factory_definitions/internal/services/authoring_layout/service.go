// Package authoring_layout defines the Factory Definitions-owned private
// authoring capability for prepare, flatten, expand, and atomic create/replace
// of one Factory aggregate behind the CTR-DEF root authoring slice.
//
// Construction capabilities stay in this internal package and never cross the
// Factory Definitions root Service.
package authoring_layout

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns parse/render, flatten/expand, and atomic create/replace of one
// Factory aggregate behind the CTR-DEF root authoring slice.
type Service interface {
	PrepareFactoryLayout(
		context.Context,
		factorydefinitions.PrepareFactoryLayoutRequest,
	) (factorydefinitions.PrepareFactoryLayoutResult, error)
	FlattenFactoryLayout(
		context.Context,
		factorydefinitions.FlattenFactoryLayoutRequest,
	) (factorydefinitions.FlattenFactoryLayoutResult, error)
	ExpandFactoryLayout(
		context.Context,
		factorydefinitions.ExpandFactoryLayoutRequest,
	) (factorydefinitions.ExpandFactoryLayoutResult, error)
	CreateNamedFactory(
		context.Context,
		factorydefinitions.CreateNamedFactoryRequest,
	) (factorydefinitions.CreateNamedFactoryResult, error)
	ReplaceNamedFactory(
		context.Context,
		factorydefinitions.ReplaceNamedFactoryRequest,
	) (factorydefinitions.ReplaceNamedFactoryResult, error)
}
