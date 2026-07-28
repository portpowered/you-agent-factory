package executor

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerExecutor is the side-effect interface for one dispatched worker step.
type WorkerExecutor interface {
	Execute(context.Context, work.WorkDispatch) (workerexecution.WorkResult, error)
}

// WorkstationRequestExecutor handles worker-owned execution after workstation
// prompt and runtime context resolution.
type WorkstationRequestExecutor interface {
	Execute(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error)
}
