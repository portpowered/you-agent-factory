package maptests

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

type testConfigMapper struct{}

func (testConfigMapper) Map(ctx context.Context, cfg *factorydefinitions.FactoryConfig) (*state.Net, error) {
	nextID := 0
	mapper, err := definitionmapping.New(func() string {
		nextID++
		return fmt.Sprintf("mapping-test-id-%d", nextID)
	})
	if err != nil {
		return nil, err
	}
	return mapper.Map(ctx, cfg)
}
