package executor

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkerExecutor is the side-effect interface for one dispatched worker step.
type WorkerExecutor interface {
	Execute(ctx context.Context, dispatch work.WorkDispatch) (interfaces.WorkResult, error)
}

// WorkstationRequestExecutor handles worker-owned execution after workstation
// prompt and runtime context resolution.
type WorkstationRequestExecutor interface {
	Execute(ctx context.Context, request interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error)
}

// Runner executes one normalized runner request.
type Runner interface {
	Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error)
}
