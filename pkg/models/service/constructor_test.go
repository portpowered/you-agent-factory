package service_test

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func mustConstructModelService(t *testing.T, deps modelsservice.Dependencies) *modelsservice.Service {
	t.Helper()
	if deps.ModelAssetPuller == nil {
		deps.ModelAssetPuller = localmodels.NewAssetPuller(t.TempDir())
	}
	if deps.ModelInvocationExecutor == nil {
		deps.ModelInvocationExecutor = func(
			*factoryconfig.LoadedFactoryConfig,
			*interfaces.FactoryConfig,
			string,
		) (workers.WorkstationRequestExecutor, error) {
			return nil, nil
		}
	}
	svc, err := modelsservice.NewService(deps)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
