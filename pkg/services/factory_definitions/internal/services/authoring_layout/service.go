// Package authoring_layout defines the Factory Definitions-owned private
// authoring parse/render and atomic replace capability. Consumers outside
// Factory Definitions use the public Definitions root authoring slice instead
// of this parent-private nested subservice contract.
package authoring_layout

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns prepare/flatten/expand and atomic create/replace for authored
// Factory layout aggregates behind the CTR-DEF root authoring vocabulary.
type Service interface {
	PrepareFactoryLayout(context.Context, factorydefinitions.PrepareFactoryLayoutRequest) (factorydefinitions.PrepareFactoryLayoutResult, error)
	FlattenFactoryLayout(context.Context, factorydefinitions.FlattenFactoryLayoutRequest) (factorydefinitions.FlattenFactoryLayoutResult, error)
	ExpandFactoryLayout(context.Context, factorydefinitions.ExpandFactoryLayoutRequest) (factorydefinitions.ExpandFactoryLayoutResult, error)
	CreateNamedFactory(context.Context, factorydefinitions.CreateNamedFactoryRequest) (factorydefinitions.CreateNamedFactoryResult, error)
	ReplaceNamedFactory(context.Context, factorydefinitions.ReplaceNamedFactoryRequest) (factorydefinitions.ReplaceNamedFactoryResult, error)
}

// Ports injects exact filesystem and representation effects at construction
// time. authoring_layout does not select peer implementations or own Wire/root
// composition.
type Ports struct {
	Prepare func(context.Context, string, []byte) (factorydefinitions.PreparedFactoryLayoutPayload, error)
	Flatten func(string) ([]byte, error)
	Expand  func(string) (string, factorydefinitions.LayoutExpansionReport, error)
	Create  func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error)
	Replace func(string, string, factorydefinitions.PreparedFactoryLayoutPayload) (string, error)
}
