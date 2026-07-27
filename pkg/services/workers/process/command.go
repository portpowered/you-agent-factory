package process

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type CommandRunner = workers.CommandRunner
type CommandRequest = workers.CommandRequest
type CommandResult = workers.CommandResult
type OutputChunkObserver = platformprocess.OutputChunkObserver
type CommandEnvEntry = platformprocess.CommandEnvEntry

// Clock is the exact wall-time effect consumed by Workers command diagnostics.
// Composition supplies the policy-free implementation; command runners never
// select a process-global default.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts an already-injected time function to Clock.
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

var CommandEnvEntriesFromMap = platformprocess.CommandEnvEntriesFromMap
var MergeCommandEnv = platformprocess.MergeCommandEnv

const (
	OutputStreamStdout = platformprocess.OutputStreamStdout
	OutputStreamStderr = platformprocess.OutputStreamStderr
)

type ExecCommandRunner struct {
	Runner platformprocess.CommandRunner
	Logger logging.Logger
}

// AdaptCommandRunner projects a low-level subprocess effect into the
// Workers-owned command boundary.
func AdaptCommandRunner(runner platformprocess.CommandRunner) CommandRunner {
	adapted := ExecCommandRunner{Runner: runner}
	if _, ok := runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return StreamingAdaptedCommandRunner{ExecCommandRunner: adapted}
	}
	return adapted
}

func (r ExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	runner := r.Runner
	if runner == nil {
		return CommandResult{}, errors.New("workers process command runner is required")
	}
	runner = platformCommandRunnerWithLogger(runner, commandContextLogger(r.Logger, req))
	result, err := runner.Run(ctx, effectRequest(req))
	return commandResult(result), err
}

func platformCommandRunnerWithLogger(runner platformprocess.CommandRunner, logger logging.Logger) platformprocess.CommandRunner {
	switch typed := runner.(type) {
	case platformprocess.ExecCommandRunner:
		typed.Logger = logger
		return typed
	case *platformprocess.ExecCommandRunner:
		if typed == nil {
			return nil
		}
		cloned := *typed
		cloned.Logger = logger
		return cloned
	default:
		return runner
	}
}

type StreamingExecCommandRunner struct {
	Observer OutputChunkObserver
	Logger   logging.Logger
	Runner   CommandRunner
}

func (r StreamingExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Runner == nil {
		return CommandResult{}, errors.New("workers streaming command runner is required")
	}
	streaming, ok := r.Runner.(interface {
		RunStreaming(context.Context, CommandRequest, OutputChunkObserver) (CommandResult, error)
	})
	if !ok {
		return CommandResult{}, errors.New("injected workers command runner does not support streaming")
	}
	return streaming.RunStreaming(ctx, req, r.Observer)
}

// RunStreaming forwards incremental output through the injected Platform
// command capability when available.
// StreamingAdaptedCommandRunner preserves the optional streaming capability
// only when the injected platform effect actually implements it.
type StreamingAdaptedCommandRunner struct {
	ExecCommandRunner
}

func (r StreamingAdaptedCommandRunner) RunStreaming(ctx context.Context, req CommandRequest, observer OutputChunkObserver) (CommandResult, error) {
	if r.Runner == nil {
		return CommandResult{}, errors.New("workers process command runner is required")
	}
	platformRunner := platformCommandRunnerWithLogger(r.Runner, commandContextLogger(r.Logger, req))
	streaming, ok := platformRunner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if !ok {
		return CommandResult{}, errors.New("injected platform command runner does not support streaming")
	}
	result, err := streaming.RunStreaming(ctx, effectRequest(req), platformprocess.OutputChunkObserver(observer))
	return commandResult(result), err
}

type LoggingCommandRunner struct {
	Runner CommandRunner
	Logger logging.Logger
	Clock  Clock
}

func (r LoggingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	logger := logging.EnsureLogger(r.Logger)
	runner := r.Runner
	if runner == nil {
		return CommandResult{}, errors.New("workers logging command runner is required")
	}
	if r.Clock == nil {
		return CommandResult{}, errors.New("workers logging command clock is required")
	}
	logger.Info("command runner: request received", commandRequestLogFields(req)...)
	logger.Verbose("command runner: verbose request details", commandRequestDetailsLogFields(req)...)
	started := r.Clock.Now()
	result, err := runner.Run(ctx, req)
	duration := r.Clock.Now().Sub(started)
	logger.Info("command runner: request completed", commandCompletionLogFields(req, result, duration, commandResultStatus(ctx, result, err), err)...)
	logger.Verbose("command runner: verbose output details", commandOutputDetailsLogFields(req, result, duration)...)
	return result, err
}

func SubprocessRequestBase(dispatch work.WorkDispatch) CommandRequest {
	cloned := work.CloneWorkDispatch(dispatch)
	return CommandRequest{
		DispatchID: cloned.DispatchID, TransitionID: cloned.TransitionID,
		WorkerType: cloned.WorkerType, WorkstationName: cloned.WorkstationName,
		ProjectID: cloned.ProjectID, CurrentChainingTraceID: cloned.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloned.PreviousChainingTraceIDs,
		Execution:                cloned.Execution, InputBindings: cloned.InputBindings,
	}
}

func CommandRunnerWithLogging(runner CommandRunner, logger logging.Logger, clock Clock) CommandRunner {
	if existing, ok := runner.(*LoggingCommandRunner); ok {
		if existing.Logger == nil {
			existing.Logger = logger
		}
		if existing.Runner != nil {
			existing.Runner = execCommandRunnerWithLogger(existing.Runner, logger)
		}
		if existing.Clock == nil {
			existing.Clock = clock
		}
		return existing
	}
	if runner == nil {
		return &LoggingCommandRunner{Logger: logger, Clock: clock}
	}
	return &LoggingCommandRunner{Runner: execCommandRunnerWithLogger(runner, logger), Logger: logger, Clock: clock}
}

func execCommandRunnerWithLogger(runner CommandRunner, logger logging.Logger) CommandRunner {
	switch typed := runner.(type) {
	case ExecCommandRunner:
		if typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	case *ExecCommandRunner:
		if typed != nil && typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	case StreamingAdaptedCommandRunner:
		if typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	case *StreamingAdaptedCommandRunner:
		if typed != nil && typed.Logger == nil {
			typed.Logger = logger
		}
		return typed
	default:
		return runner
	}
}

func effectRequest(req CommandRequest) platformprocess.CommandRequest {
	return platformprocess.CommandRequest{Command: req.Command, Args: req.Args, Stdin: req.Stdin, Env: req.Env, WorkDir: req.WorkDir}
}

func commandResult(result platformprocess.CommandResult) CommandResult {
	return CommandResult{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}
}

var _ CommandRunner = ExecCommandRunner{}
var _ CommandRunner = StreamingAdaptedCommandRunner{}
var _ CommandRunner = StreamingExecCommandRunner{}
var _ CommandRunner = (*LoggingCommandRunner)(nil)
