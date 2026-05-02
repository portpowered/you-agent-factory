//go:build functionallong

package runtime_api

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type sleepyExecutor struct{ sleep time.Duration }

func (e *sleepyExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	time.Sleep(e.sleep)
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}
