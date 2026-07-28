// Package authoredlayout owns Factory Definition split-layout preparation and
// writing behavior. Representation adapters are supplied by Wire.
package authoredlayout

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
)

// Prepare normalizes and prunes one submitted Factory Definition before the
// persistence transaction writes its split authored layout.
func Prepare(
	ctx context.Context,
	segment string,
	payload []byte,
	validator factorydefinitions.Validator,
	decodeFactory func([]byte) (*factorydefinitions.FactoryConfig, error),
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return authoringlayoutprepare.FactoryLayout(
		ctx,
		segment,
		payload,
		validator,
		decodeFactory,
		normalizeAuthored,
		encodeFactory,
	)
}
