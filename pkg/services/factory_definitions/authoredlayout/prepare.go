// Package authoredlayout owns Factory Definition split-layout preparation and
// writing behavior. Representation adapters are supplied by Wire.
package authoredlayout

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	switch {
	case validator == nil:
		return nil, fmt.Errorf("Factory Definition validator is required")
	case decodeFactory == nil:
		return nil, fmt.Errorf("Factory Definition decoder is required")
	case normalizeAuthored == nil:
		return nil, fmt.Errorf("authored Factory Definition normalizer is required")
	case encodeFactory == nil:
		return nil, fmt.Errorf("Factory Definition encoder is required")
	}

	factoryConfig, err := decodeFactory(payload)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: parse factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			segment,
			err,
		)
	}
	topology := factorydefinitions.BuildPendingFactoryGraphTopology(factoryConfig)
	validator.PruneLayout(ctx, factoryConfig, topology)

	authoredFactoryConfig, err := normalizeAuthored(factoryConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: normalize authored factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			segment,
			err,
		)
	}
	canonical, err := encodeFactory(authoredFactoryConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: normalize factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			segment,
			err,
		)
	}
	return &factorydefinitions.PreparedFactoryLayoutPayload{
		Config:    factoryConfig,
		Canonical: canonical,
	}, nil
}
