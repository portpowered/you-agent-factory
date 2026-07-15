package executor

import (
	"context"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkerExecutor is the side-effect interface for one dispatched worker step.
type WorkerExecutor interface {
	Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error)
}

// WorkstationRequestExecutor handles worker-owned execution after workstation
// prompt and runtime context resolution.
type WorkstationRequestExecutor interface {
	Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error)
}

// Runner executes one normalized runner request.
type Runner interface {
	Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error)
}
