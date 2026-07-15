//go:build functionallong

package runtime_api

import (
	"context"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

type sleepyExecutor struct{ sleep time.Duration }

func (e *sleepyExecutor) Execute(_ context.Context, d work.WorkDispatch) (interfaces.WorkResult, error) {
	time.Sleep(e.sleep)
	return interfaces.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}
