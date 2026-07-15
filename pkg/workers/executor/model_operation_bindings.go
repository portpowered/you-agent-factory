package executor

import (
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
