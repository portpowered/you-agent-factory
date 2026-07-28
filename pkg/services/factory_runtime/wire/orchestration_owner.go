package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
)

// NewOrchestrationJavaScriptExecution constructs the Runtime JavaScript
// orchestration execute/resume port for peer wire assembly.
func NewOrchestrationJavaScriptExecution(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.OrchestrationJavaScriptExecution {
	return factoryruntimeorchestrationowner.New(newID, workflows)
}

// NewOrchestrationCompilation constructs the Runtime orchestration kind
// selection and compilation port for peer wire assembly.
func NewOrchestrationCompilation(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.OrchestrationCompilation {
	return factoryruntimeorchestrationowner.NewCompilation(newID, workflows, workflows)
}
