package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

// CommandRunner executes a low-level subprocess request for worker code.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// CommandRequest describes one worker-owned subprocess invocation.
type CommandRequest = interfaces.SubprocessExecutionRequest

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
type ExecCommandRunner struct{}

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
func (ExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
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

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		_ = terminateCommandProcessTree(cmd, tree)
		<-waitCh
		closeCommandProcessTree(tree)
		return CommandResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
		}, ctx.Err()
	}
	closeCommandProcessTree(tree)

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

func SubprocessRequestBase(dispatch interfaces.WorkDispatch) CommandRequest {
	clonedDispatch := interfaces.CloneWorkDispatch(dispatch)
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
		return existing
	}
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	return &LoggingCommandRunner{
		Runner: runner,
		Logger: logger,
	}
}

var _ CommandRunner = ExecCommandRunner{}
var _ CommandRunner = (*LoggingCommandRunner)(nil)
