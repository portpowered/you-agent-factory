package executor

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	inputTokens []factorytoken.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	return workerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
