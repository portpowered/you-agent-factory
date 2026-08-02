// Package prepare owns parse-render normalization for one authored Factory
// aggregate before durable layout writes.
package prepare

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/contracts"
)

// FactoryLayout prepares one submitted Factory Definition payload for durable
// split-layout writes.
func FactoryLayout(
	ctx context.Context,
	segment string,
	payload []byte,
	validator authoringlayoutcontracts.LayoutValidator,
	decodeFactory authoringlayoutcontracts.FactoryConfigJSONDecoder,
	normalizeAuthored authoringlayoutcontracts.AuthoredFactoryNormalizer,
	encodeFactory authoringlayoutcontracts.FactoryConfigJSONEncoder,
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
	if err := ctx.Err(); err != nil {
		return nil, err
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
