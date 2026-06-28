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

// ResolveModelOperationBindings resolves one MODEL_INVOKE-style slot binding
// set against ordered runtime input content using the same rules as workstation
// execution.
func ResolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	return invocations.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}
