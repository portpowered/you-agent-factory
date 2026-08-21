package wire

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

// NewContextualMockWorkerCommandRunner decorates a process command edge with
// request-scoped mock behavior. The override is read from the detached
// execution context, so concurrent Factory Sessions never share mutable mock
// configuration.
func NewContextualMockWorkerCommandRunner(next platformprocess.CommandRunner) platformprocess.CommandRunner {
	return workerprocess.ProjectPlatformCommandRunner(
		newContextualMockWorkerCommandRunner(workerprocess.AdaptPlatformCommandRunner(next)),
	)
}

// NewLoggingCommandRunner keeps command diagnostics inside the Workers-owned
// private runner boundary while exposing only the platform process effect to
// composition callers.
func NewLoggingCommandRunner(
	next platformprocess.CommandRunner,
	logger logging.Logger,
	clock func() time.Time,
) platformprocess.CommandRunner {
	private := workerprocess.AdaptPlatformCommandRunner(next)
	if private == nil || clock == nil {
		return next
	}
	return workerprocess.ProjectPlatformCommandRunner(
		workerprocess.CommandRunnerWithLogging(private, logger, workerprocess.ClockFunc(clock)),
	)
}

type contextualMockWorkerCommandRunner struct {
	next workerprocess.CommandRunner
}

func newContextualMockWorkerCommandRunner(next workerprocess.CommandRunner) workerprocess.CommandRunner {
	return contextualMockWorkerCommandRunner{next: next}
}

func (runner contextualMockWorkerCommandRunner) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		return runner.runNext(ctx, request)
	}
	return (&runnermockworker.MockWorkerCommandRunner{
		Config: config, Next: runner.next,
		OutputPolicy: workerexecution.MockWorkerOutputPolicyFromContext(ctx),
	}).Run(ctx, request)
}

func (runner contextualMockWorkerCommandRunner) RunStreaming(
	ctx context.Context,
	request workerprocess.CommandRequest,
	observer workerprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		if streaming, ok := runner.next.(interface {
			RunStreaming(context.Context, workerprocess.CommandRequest, workerprocess.OutputChunkObserver) (workerprocess.CommandResult, error)
		}); ok {
			return streaming.RunStreaming(ctx, request, observer)
		}
		result, err := runner.runNext(ctx, request)
		publishCompleteCommandOutput(observer, result.Stdout, result.Stderr)
		return result, err
	}
	result, err := (&runnermockworker.MockWorkerCommandRunner{
		Config: config, Next: runner.next,
		OutputPolicy: workerexecution.MockWorkerOutputPolicyFromContext(ctx),
	}).Run(ctx, request)
	publishCompleteCommandOutput(observer, result.Stdout, result.Stderr)
	return result, err
}

func (runner contextualMockWorkerCommandRunner) CommandResultForLogging(
	ctx context.Context,
	request workerprocess.CommandRequest,
	result workerprocess.CommandResult,
) workerprocess.CommandResult {
	config := workerexecution.MockWorkersConfigFromContext(ctx)
	if config == nil {
		return result
	}
	return (&runnermockworker.MockWorkerCommandRunner{
		Config:       config,
		OutputPolicy: workerexecution.MockWorkerOutputPolicyFromContext(ctx),
	}).CommandResultForLogging(ctx, request, result)
}

func (runner contextualMockWorkerCommandRunner) runNext(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	if runner.next == nil {
		return workerprocess.CommandResult{}, errors.New("contextual mock worker next command runner is required")
	}
	return runner.next.Run(ctx, request)
}

func publishCompleteCommandOutput(observer workerprocess.OutputChunkObserver, stdout, stderr []byte) {
	if observer == nil {
		return
	}
	if len(stdout) > 0 {
		observer(workerprocess.OutputStreamStdout, append([]byte(nil), stdout...))
	}
	if len(stderr) > 0 {
		observer(workerprocess.OutputStreamStderr, append([]byte(nil), stderr...))
	}
}
