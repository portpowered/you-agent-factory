package maptests

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type testConfigMapper struct {
	config.ConfigMapper
}

func (m testConfigMapper) Map(ctx context.Context, cfg *interfaces.FactoryConfig) (*state.Net, error) {
	factoryvalidation.NormalizeFixtureConfig(cfg)
	return m.ConfigMapper.Map(ctx, cfg)
}
