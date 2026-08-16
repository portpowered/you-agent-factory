package executor

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
)

// RunnerFromProvider adapts the Providers execution contract to the detached
// Workers runner contract. It is used only for an explicit process-scoped
// provider override; normal attempts resolve the private runner registry.
func RunnerFromProvider(provider providers.Service) workerexecution.Runner {
	return providerRunnerAdapter{executor: workerinvocation.NewExecutor(provider)}
}

type providerRunnerAdapter struct {
	executor workerexecution.InvocationExecutor
}

func (adapter providerRunnerAdapter) Execute(
	ctx context.Context,
	request workerexecution.RunnerExecutionRequest,
) (workerexecution.RunnerExecutionResult, error) {
	if adapter.executor == nil {
		return workerexecution.RunnerExecutionResult{}, workerexecution.NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"runner requires a provider implementation",
			nil,
		)
	}
	result, err := adapter.executor.Execute(ctx, workerexecution.InvocationInput{
		Request: request,
		Attempt: 1,
	})
	return result.Response, err
}
