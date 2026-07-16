package maptests

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

type testConfigMapper struct {
	config.ConfigMapper
}

func (m testConfigMapper) Map(ctx context.Context, cfg *interfaces.FactoryConfig) (*state.Net, error) {
	factoryvalidation.NormalizeFixtureConfig(cfg)
	return m.ConfigMapper.Map(ctx, cfg)
}
