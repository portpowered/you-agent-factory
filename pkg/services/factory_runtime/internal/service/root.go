// Package service is a transitional compile shim for the process-scoped Factory
// Runtime root. The real implementation lives in factory_runtime/internal.
package service

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
)

// Root retains process-scoped Factory Runtime dependencies.
type Root = factoryruntimeinternal.Root

// NewRoot constructs the inert Factory Runtime root from construction ports.
func NewRoot(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowRuntime factoryruntime.JavaScriptWorkflowRuntime,
	clock factoryruntime.Clock,
	workersPublisher dispatchplanning.WorkersPublisher,
	workersCanceler dispatchplanning.WorkersCanceler,
) (*Root, error) {
	return factoryruntimeinternal.NewRoot(
		newID,
		workflows,
		workflowRuntime,
		clock,
		workersPublisher,
		workersCanceler,
	)
}
