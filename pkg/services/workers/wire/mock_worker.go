package wire

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

// NewContextualMockWorkerCommandRunner decorates a process command edge with
// request-scoped mock behavior. The override is read from the detached
// execution context, so concurrent Factory Sessions never share mutable mock
// configuration.
func NewContextualMockWorkerCommandRunner(
	next platformprocess.CommandRunner,
	files workers.AgentToolFileSystem,
) platformprocess.CommandRunner {
	return workerprocess.ProjectPlatformCommandRunner(
		newContextualMockWorkerCommandRunner(
			workerprocess.AdaptPlatformCommandRunner(next),
			files,
		),
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

// NewProviderCommandRunner projects the Workers-private command request onto
// the Providers-owned effect shape at the composition boundary. The returned
// value intentionally has an opaque structural shape: Providers adapts it
// without importing Workers' private command types, while the request-scoped
// mock configuration continues to flow through context unchanged.
func NewProviderCommandRunner(next platformprocess.CommandRunner) any {
	return providerCommandRunner{
		runner: workerprocess.AdaptPlatformCommandRunner(next),
	}
}

type providerCommandRunner struct {
	runner workerprocess.CommandRunner
}

type providerCommandRequest struct {
	Command                  string
	Args                     []string
	Stdin                    []byte
	Env                      []string
	WorkDir                  string
	FactorySessionID         string
	DispatchID               string
	AttemptID                string
	TransitionID             string
	WorkerType               string
	WorkstationName          string
	ProjectID                string
	InputTokens              []any
	InputBindings            map[string][]string
	Execution                work.ExecutionMetadata
	ExecutionLogger          logging.Logger
	ProcessLifecycleObserver platformprocess.ProcessLifecycleObserver
}

func (runner providerCommandRunner) Run(
	ctx context.Context,
	request providerCommandRequest,
) (workerprocess.CommandResult, error) {
	if runner.runner == nil {
		return workerprocess.CommandResult{}, errors.New("provider command runner is required")
	}
	return runner.runner.Run(ctx, workerCommandRequest(request))
}

func (runner providerCommandRunner) RunStreaming(
	ctx context.Context,
	request providerCommandRequest,
	observer platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	if runner.runner == nil {
		return workerprocess.CommandResult{}, errors.New("provider command runner is required")
	}
	if streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, workerprocess.CommandRequest, platformprocess.OutputChunkObserver) (workerprocess.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, workerCommandRequest(request), observer)
	}
	result, err := runner.Run(ctx, request)
	publishCompleteCommandOutput(observer, result.Stdout, result.Stderr)
	return result, err
}

func workerCommandRequest(request providerCommandRequest) workerprocess.CommandRequest {
	dispatchID := request.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	private := workerprocess.SubprocessRequestBase(work.WorkDispatch{
		DispatchID:      dispatchID,
		TransitionID:    request.TransitionID,
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationName,
		ProjectID:       request.ProjectID,
		InputTokens:     request.InputTokens,
		InputBindings:   request.InputBindings,
		Execution:       request.Execution,
	})
	private.Command = request.Command
	private.Args = request.Args
	private.Stdin = request.Stdin
	private.Env = request.Env
	private.WorkDir = request.WorkDir
	private.FactorySessionID = request.FactorySessionID
	private.ExecutionLogger = request.ExecutionLogger
	private.ProcessLifecycleObserver = request.ProcessLifecycleObserver
	return private
}

type contextualMockWorkerCommandRunner struct {
	next  workerprocess.CommandRunner
	files workers.AgentToolFileSystem
}

func newContextualMockWorkerCommandRunner(
	next workerprocess.CommandRunner,
	files workers.AgentToolFileSystem,
) workerprocess.CommandRunner {
	return contextualMockWorkerCommandRunner{next: next, files: files}
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
		Files:        runner.files,
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
		Files:        runner.files,
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
