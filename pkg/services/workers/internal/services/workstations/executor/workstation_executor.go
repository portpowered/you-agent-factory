package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkstationExecutor retains the workerless logical-workstation route used by
// the Workers workstation pool. Provider, script, inference, and agent
// attempts are request-scoped Workers.Execute requests and do not enter this
// compatibility executor.
type WorkstationExecutor struct {
	RuntimeConfig interfaces.RuntimeConfigLookup
	Now           func() time.Time
}

// Execute implements WorkerExecutor for logical workstation routes.
func (we *WorkstationExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return we.executeResolved(ctx, workerexecution.WorkstationExecutionRequest{Dispatch: dispatch})
}

// ExecuteResolved preserves the workstation pool's resolved request boundary
// while allowing only workerless logical routes to use this executor.
func (we *WorkstationExecutor) ExecuteResolved(
	ctx context.Context,
	request workerexecution.WorkstationExecutionRequest,
) (workerexecution.WorkResult, error) {
	return we.executeResolved(ctx, request)
}

func (we *WorkstationExecutor) executeResolved(
	ctx context.Context,
	request workerexecution.WorkstationExecutionRequest,
) (workerexecution.WorkResult, error) {
	if ctx == nil {
		return workerexecution.WorkResult{}, fmt.Errorf("workstation executor context is required")
	}
	if err := ctx.Err(); err != nil {
		return workerexecution.WorkResult{}, err
	}
	if we == nil || we.Now == nil {
		return workerexecution.WorkResult{}, fmt.Errorf("workstation executor clock is required")
	}
	start := we.Now()
	workstation, ok := we.runtimeWorkstation(request.Dispatch)
	if !ok {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "workstation not found: " + workstationLookupKey(request.Dispatch),
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}
	if workstation.Type != interfaces.WorkstationTypeLogical {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "non-logical workstation execution is owned by Workers.Execute",
			Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
		}, nil
	}
	return workerexecution.WorkResult{
		DispatchID:   request.Dispatch.DispatchID,
		TransitionID: request.Dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Metrics:      workerexecution.WorkMetrics{Duration: we.Now().Sub(start)},
	}, nil
}

func (we *WorkstationExecutor) runtimeWorkstation(dispatch work.WorkDispatch) (*interfaces.FactoryWorkstationConfig, bool) {
	if we == nil || we.RuntimeConfig == nil {
		return nil, false
	}
	workstation, ok := we.RuntimeConfig.Workstation(workstationLookupKey(dispatch))
	if !ok || workstation == nil || strings.TrimSpace(workstation.Type) == "" {
		return nil, false
	}
	return workstation, true
}

func workstationLookupKey(dispatch work.WorkDispatch) string {
	return strings.TrimSpace(dispatch.WorkstationName)
}

// Compile-time check.
var _ WorkerExecutor = (*WorkstationExecutor)(nil)
