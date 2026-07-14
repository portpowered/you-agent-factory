package executor

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	return workerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
