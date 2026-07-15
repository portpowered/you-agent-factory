//go:build functionallong

package runtime_api

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type sleepyExecutor struct{ sleep time.Duration }

func (e *sleepyExecutor) Execute(_ context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
	time.Sleep(e.sleep)
	return workerexecution.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: workerexecution.OutcomeAccepted}, nil
}
