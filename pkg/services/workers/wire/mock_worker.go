package wire

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

// NewContextualMockWorkerCommandRunner decorates a process command edge with
// request-scoped mock behavior. The override is read from the detached
// execution context, so concurrent Factory Sessions never share mutable mock
// configuration.
func NewContextualMockWorkerCommandRunner(next workers.CommandRunner) workers.CommandRunner {
	return contextualMockWorkerCommandRunner{next: next}
}

type contextualMockWorkerCommandRunner struct {
	next workers.CommandRunner
}

func (runner contextualMockWorkerCommandRunner) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		return runner.runNext(ctx, request)
	}
	return (&workers.MockWorkerCommandRunner{
		Config: config, Next: runner.next,
		OutputPolicy: workerexecution.MockWorkerOutputPolicyFromContext(ctx),
	}).Run(ctx, request)
}

func (runner contextualMockWorkerCommandRunner) RunStreaming(
	ctx context.Context,
	request workers.CommandRequest,
	observer workers.OutputChunkObserver,
) (workers.CommandResult, error) {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		if streaming, ok := runner.next.(interface {
			RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
		}); ok {
			return streaming.RunStreaming(ctx, request, observer)
		}
		result, err := runner.runNext(ctx, request)
		publishCompleteCommandOutput(observer, result.Stdout, result.Stderr)
		return result, err
	}
	result, err := (&workers.MockWorkerCommandRunner{
		Config: config, Next: runner.next,
		OutputPolicy: workerexecution.MockWorkerOutputPolicyFromContext(ctx),
	}).Run(ctx, request)
	publishCompleteCommandOutput(observer, result.Stdout, result.Stderr)
	return result, err
}

func (runner contextualMockWorkerCommandRunner) CommandResultForLogging(
	ctx context.Context,
	request workers.CommandRequest,
	result workers.CommandResult,
) workers.CommandResult {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		return result
	}
	return (&workers.MockWorkerCommandRunner{Config: config}).CommandResultForLogging(ctx, request, result)
}

func (runner contextualMockWorkerCommandRunner) runNext(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	if runner.next == nil {
		return workers.CommandResult{}, errors.New("contextual mock worker next command runner is required")
	}
	return runner.next.Run(ctx, request)
}

func publishCompleteCommandOutput(observer workers.OutputChunkObserver, stdout, stderr []byte) {
	if observer == nil {
		return
	}
	if len(stdout) > 0 {
		observer(workers.OutputStreamStdout, append([]byte(nil), stdout...))
	}
	if len(stderr) > 0 {
		observer(workers.OutputStreamStderr, append([]byte(nil), stderr...))
	}
}
