package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

// CommandRunner executes a low-level subprocess request for worker code.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// CommandRequest describes one worker-owned subprocess invocation.
type CommandRequest = workerexecution.SubprocessExecutionRequest

// CommandResult captures the observable output and exit status from a command.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// LoggingCommandRunner emits structured work-scoped records around a
// CommandRunner request while preserving the wrapped runner's behavior.
type LoggingCommandRunner struct {
	Runner CommandRunner
	Logger logging.Logger
}

// ExecCommandRunner implements CommandRunner by delegating to os/exec.
type ExecCommandRunner struct {
	// Logger emits structured process-group cleanup diagnostics. Nil disables cleanup logging.
	Logger logging.Logger
}

func (r LoggingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	logger := logging.EnsureLogger(r.Logger)
	runner := r.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	logger.Info("command runner: request received",
		commandRequestLogFields(req)...)
	logger.Verbose("command runner: verbose request details",
		commandRequestDetailsLogFields(req)...)

	started := time.Now()
	result, err := runner.Run(ctx, req)
	duration := time.Since(started)

	logger.Info("command runner: request completed",
		commandCompletionLogFields(req, result, duration, commandResultStatus(ctx, result, err), err)...)
	logger.Verbose("command runner: verbose output details",
		commandOutputDetailsLogFields(req, result, duration)...)

	return result, err
}

// Run executes the command with process-tree cancellation, capturing stdout and stderr.
func (r ExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}

	cmd := exec.Command(req.Command, req.Args...)
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	configureCommandProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return CommandResult{}, err
	}

	tree, _ := attachCommandProcessTree(cmd)
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	cleanupLogger := logging.EnsureLogger(r.Logger)
	cancelCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonCancel)
	postRunCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonPostRun)

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		_ = terminateCommandProcessTree(cmd, tree, cancelCleanup)
		<-waitCh
		closeCommandProcessTree(cmd, tree, postRunCleanup)
		return CommandResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
		}, ctx.Err()
	}
	closeCommandProcessTree(cmd, tree, postRunCleanup)

	result := CommandResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	return result, nil
}

func SubprocessRequestBase(dispatch work.WorkDispatch) CommandRequest {
	clonedDispatch := work.CloneWorkDispatch(dispatch)
	return CommandRequest{
		DispatchID:               clonedDispatch.DispatchID,
		TransitionID:             clonedDispatch.TransitionID,
		WorkerType:               clonedDispatch.WorkerType,
		WorkstationName:          clonedDispatch.WorkstationName,
		ProjectID:                clonedDispatch.ProjectID,
		CurrentChainingTraceID:   clonedDispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: clonedDispatch.PreviousChainingTraceIDs,
		Execution:                clonedDispatch.Execution,
		InputBindings:            clonedDispatch.InputBindings,
	}
}

func CommandRunnerWithLogging(runner CommandRunner, logger logging.Logger) CommandRunner {
	if existing, ok := runner.(*LoggingCommandRunner); ok {
		if existing.Logger == nil {
			existing.Logger = logger
		}
		if existing.Runner != nil {
			existing.Runner = execCommandRunnerWithLogger(existing.Runner, logger)
		}
		return existing
	}
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	return &LoggingCommandRunner{
		Runner: execCommandRunnerWithLogger(runner, logger),
		Logger: logger,
	}
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
	default:
		return runner
	}
}

var _ CommandRunner = ExecCommandRunner{}
var _ CommandRunner = (*LoggingCommandRunner)(nil)
