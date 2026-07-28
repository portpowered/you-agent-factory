// Package authoring_layout defines the Factory Definitions-owned private
// authoring capability for prepare, flatten, expand, and atomic create/replace
// of one Factory aggregate behind the CTR-DEF root authoring slice.
//
// The public surface exposes only CTR-DEF authoring vocabulary and exact
// injected layout-parse, layout-transform, and durable-write ports. It does not
// declare peer service implementations, Wire/root construction ownership,
// filesystem effect concrete types, or sibling catalog/validation/compilation/
// snapshots_portability/distribution APIs.
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

// Dependencies are the exact collaborator ports required by authoring_layout.
// They are supplied by Factory Definitions composition and never selected here:
// authoring_layout does not choose host filesystem adapters or Wire/root
// constructors.
type Dependencies struct {
	Validator            factorydefinitions.Validator
	MapInput             factorydefinitions.FactoryLayoutPayloadMapper
	Prepare              factorydefinitions.FactoryLayoutPayloadPreparer
	Write                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
	Validate             func(string) error
	Flatten              factorydefinitions.FactoryLayoutFlattener
	Expand               factorydefinitions.FactoryLayoutExpander
	FileSystem           factorydefinitions.PersistenceFileSystem
	RequireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer
	Directories          factorydefinitions.DirectoryReplacementStore
}
