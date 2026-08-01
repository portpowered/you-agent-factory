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
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/persist"
)

// PersistPorts is the authoring-owned durable-write port bundle used by the
// catalog composition facade. The concrete persistence algorithm remains
// private to authoring_layout/internal/persist.
type PersistPorts = persist.Ports

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
	ReplaceFactoryLayoutAtDir(
		context.Context,
		factorydefinitions.ReplaceFactoryLayoutAtDirRequest,
	) (factorydefinitions.ReplaceFactoryLayoutAtDirResult, error)
}

// Dependencies are the exact collaborator ports required by authoring_layout.
// They are supplied by Factory Definitions composition and never selected here:
// authoring_layout does not choose host filesystem adapters or Wire/root
// constructors.
type Dependencies struct {
	Validator            factorydefinitions.Validator
	MapInput             factorydefinitions.FactoryLayoutPayloadMapper
	DecodeFactory        factorydefinitions.FactoryConfigJSONDecoder
	NormalizeAuthored    func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
	EncodeFactory        func(*factorydefinitions.FactoryConfig) ([]byte, error)
	Write                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
	Validate             func(string) error
	Flatten              factorydefinitions.FactoryLayoutFlattener
	Expand               factorydefinitions.FactoryLayoutExpander
	FileSystem           factoryeffects.PersistenceFileSystem
	RequireDefinitionDir factoryeffects.DefinitionDirectoryRequirer
	Directories          factoryeffects.DirectoryReplacementStore
}
