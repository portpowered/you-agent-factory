package executor

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	return invocations.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
