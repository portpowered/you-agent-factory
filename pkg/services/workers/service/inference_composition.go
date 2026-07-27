package service

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
)

func resolveInferenceRunner(
	inner workers.Runner,
	modelsService models.Service,
	modelsScope models.RuntimeScopeRef,
	factoryCfg *interfaces.FactoryConfig,
	workerCfg *interfaces.FactoryWorkerConfig,
) workers.Runner {
	if factoryCfg == nil {
		return runnerswire.NewInferenceCompositionRunner(
			inner, modelsService, modelsScope, workerCfg, nil,
		)
	}
	return runnerswire.NewInferenceCompositionRunner(
		inner,
		modelsService,
		modelsScope,
		workerCfg,
		factoryCfg.Resources,
	)
}
