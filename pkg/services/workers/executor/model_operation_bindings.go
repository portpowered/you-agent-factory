package executor

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerinference "github.com/portpowered/infinite-you/pkg/services/workers/services/inference"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	inputTokens []workerexecution.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	return workerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
