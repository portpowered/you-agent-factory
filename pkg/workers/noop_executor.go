package workers

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// NoopExecutor is a WorkerExecutor that always returns OutcomeAccepted
// without calling any LLM or script. It is used as a fallback when no
// AGENTS.md is configured for a worker, allowing tests to exercise the
// petri-net topology without providing real worker configuration.
type NoopExecutor struct{}

// Execute implements WorkerExecutor. It propagates the first input token's
// color and returns OutcomeAccepted immediately.
func (n *NoopExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

// Compile-time check.
var _ WorkerExecutor = (*NoopExecutor)(nil)
