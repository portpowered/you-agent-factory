package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type staticModelAssetPuller struct {
	pullResult apisurface.ModelPullResult
	pullErr    error
	ensureErr  error
}

func (s staticModelAssetPuller) PullModel(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	return s.pullResult, s.pullErr
}

func (s staticModelAssetPuller) EnsureModelAvailable(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *interfaces.WorkerConfig) error {
	return s.ensureErr
}
