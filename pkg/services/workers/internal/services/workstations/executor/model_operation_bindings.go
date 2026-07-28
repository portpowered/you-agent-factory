package executor

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/inference"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	inputTokens []workerexecution.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	return runnerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
