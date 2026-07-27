// Package wire constructs the parent-private Factory Runtime orchestration
// compiler selected by the composition root.
package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/internal/service"
)

// New constructs the parent-private orchestration owner.
func New(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	runtime factoryruntime.JavaScriptWorkflowRuntime,
) orchestration.Service {
	return internalservice.New(newID, workflows, runtime)
}
