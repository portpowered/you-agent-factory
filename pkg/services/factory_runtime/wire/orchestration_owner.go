package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
)

// NewOrchestrationJavaScriptExecution constructs the Runtime JavaScript
// orchestration execute/resume port for peer wire assembly.
func NewOrchestrationJavaScriptExecution(service Orchestration) factoryruntime.OrchestrationJavaScriptExecution {
	return factoryruntimeorchestrationowner.New(service)
}

// NewOrchestrationCompilation constructs the Runtime orchestration kind
// selection and compilation port for peer wire assembly.
func NewOrchestrationCompilation(service Orchestration) factoryruntime.OrchestrationCompilation {
	return factoryruntimeorchestrationowner.NewCompilation(service)
}
