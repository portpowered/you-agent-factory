package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript"
)

// NewJavaScriptWorkflows constructs the Runtime JavaScript orchestrator service
// for peer wire assembly.
func NewJavaScriptWorkflows(
	files factoryruntime.WorkflowSourceFileSystem,
	resolveHome factoryruntime.WorkflowHomeResolver,
	resolveSymlinks factoryruntime.WorkflowSourceResolveSymlinks,
) factoryruntime.JavaScriptWorkflows {
	return factoryruntimejavascript.New(files, resolveHome, resolveSymlinks)
}
