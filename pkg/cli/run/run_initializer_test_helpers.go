package run

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/initializer"
)

// setInitializerRuntimeBuilder wires Run through the same initializer transport
// composition used by cmd/factory/main.go (InjectRuntimeRunner).
func setInitializerRuntimeBuilder(t *testing.T) func() {
	t.Helper()

	originalBuilder := buildFactoryService
	buildFactoryService = func(ctx context.Context, cfg *initializer.Config) (factoryServiceRunner, error) {
		runner, err := compose.InjectRuntimeRunner(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if runner == nil {
			return nil, errors.New("initializer runtime runner missing")
		}
		return runner, nil
	}
	return func() {
		buildFactoryService = originalBuilder
	}
}
