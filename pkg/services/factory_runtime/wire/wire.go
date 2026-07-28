// Package wire is the Factory Runtime service composition boundary.
//
// Wire performs construction only, returns the singular factory.Service root
// interface, and starts no lifecycle components. Parent-private orchestration,
// instance_host, and dispatch_planning owner wiring stays inside the owner
// service assembly path; peers depend on Service rather than owner internals or
// construction ports. Wire does not compose checkpoint_recovery.
package wire

import (
	"context"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeroot "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/service"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkersPublisher is the Workers-facing publication edge supplied at
// construction time for dispatch planning.
type WorkersPublisher func(context.Context, workers.WorkstationDispatchRequest) error

// WorkersCanceler is the Workers-facing cancellation edge supplied at
// construction time for dispatch planning.
type WorkersCanceler func(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error)

// NewService constructs an inert Factory Runtime root from construction and
// process-edge ports. It composes the accepted root through parent-private
// orchestration, instance_host, and dispatch_planning owner construction
// without publishing owner types on the returned peer surface. Missing required
// construction ports fail with a deterministic construction error and a nil
// service.
func NewService(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowRuntime factoryruntime.JavaScriptWorkflowRuntime,
	clock factoryruntime.Clock,
	workersPublisher WorkersPublisher,
	workersCanceler WorkersCanceler,
) (factoryruntime.Service, error) {
	var publisher dispatchplanning.WorkersPublisher
	if workersPublisher != nil {
		publisher = func(ctx context.Context, request workers.WorkstationDispatchRequest) error {
			return workersPublisher(ctx, request)
		}
	}
	var canceler dispatchplanning.WorkersCanceler
	if workersCanceler != nil {
		canceler = func(
			ctx context.Context,
			request workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			return workersCanceler(ctx, request)
		}
	}
	service, err := factoryruntimeroot.NewRoot(
		newID,
		workflows,
		workflowRuntime,
		clock,
		publisher,
		canceler,
	)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, fmt.Errorf("construct Factory Runtime: implementation rejected its dependencies")
	}
	return service, nil
}
