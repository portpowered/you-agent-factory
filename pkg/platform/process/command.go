package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// CommandRunner executes a low-level subprocess request for worker code.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// Clock is the exact wall-clock effect required by process cleanup and
// diagnostic timing. Production clocks are selected by the application
// injector; callers must not rely on a process-global fallback.
type Clock interface {
	Now() time.Time
}

// CommandFactory creates one inert host command. Production command creation
// is selected by the application injector so subprocess implementations do
// not call os/exec entrypoints themselves.
type CommandFactory func(name string, args ...string) *exec.Cmd

// CommandRequest describes one policy-free subprocess effect.
type CommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Stdin   []byte   `json:"stdin,omitempty"`
	Env     []string `json:"env,omitempty"`
	WorkDir string   `json:"work_dir,omitempty"`
}

// CommandResult captures the observable output and exit status from a command.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExecCommandRunner implements CommandRunner by delegating to os/exec.
type ExecCommandRunner struct {
	// Logger emits structured process-group cleanup diagnostics. Nil disables cleanup logging.
	Logger     logging.Logger
	Clock      Clock
	NewCommand CommandFactory
}

// NewExecCommandRunner constructs a host command runner from exact external
// effects. Missing effects fail closed rather than selecting ambient defaults.
func NewExecCommandRunner(newCommand CommandFactory, clock Clock, logger logging.Logger) (ExecCommandRunner, error) {
	if newCommand == nil {
		return ExecCommandRunner{}, errors.New("platform process command factory is required")
	}
	if clock == nil {
		return ExecCommandRunner{}, errors.New("platform process clock is required")
	}
	return ExecCommandRunner{Logger: logger, Clock: clock, NewCommand: newCommand}, nil
}

// Run executes the command with process-tree cancellation, capturing stdout and stderr.
func (r ExecCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	return r.run(ctx, req, nil, false)
}

// RunStreaming executes the same injected subprocess effect while forwarding
// incremental output. It prevents higher-level packages from constructing a
// second host command implementation merely to observe output.
func (r ExecCommandRunner) RunStreaming(ctx context.Context, req CommandRequest, observer OutputChunkObserver) (CommandResult, error) {
	return r.run(ctx, req, observer, true)
}

func (r ExecCommandRunner) run(
	ctx context.Context,
	req CommandRequest,
	observer OutputChunkObserver,
	streaming bool,
) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	if r.NewCommand == nil {
		return CommandResult{}, errors.New("platform process command factory is required")
	}
	if r.Clock == nil {
		return CommandResult{}, errors.New("platform process clock is required")
	}

	cmd := r.NewCommand(req.Command, req.Args...)
	if cmd == nil {
		return CommandResult{}, fmt.Errorf("platform process command factory returned nil for %q", req.Command)
	}
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	outputLimit := 0
	if streaming {
		outputLimit = maxStreamingOutputBytes
	}
	stdout := &observedBuffer{stream: OutputStreamStdout, observer: observer, maxBytes: outputLimit}
	stderr := &observedBuffer{stream: OutputStreamStderr, observer: observer, maxBytes: outputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	configureCommandProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return CommandResult{}, err
	}

	cleanupLogger := logging.EnsureLogger(r.Logger)
	tree, attachErr := attachCommandProcessTree(cmd)
	if attachErr != nil {
		cleanupLogger.Warn(
			"command runner: process tree attach failed",
			"event_name", "command_runner.process_tree_attach_failed",
			"command", req.Command,
			"args_count", len(req.Args),
			"error", attachErr.Error(),
		)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	cancelCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonCancel)
	postRunCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonPostRun)

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		_ = terminateCommandProcessTree(cmd, tree, r.Clock, cancelCleanup)
		grace := postRunCleanupGracePeriod()
		if !waitForCommandExit(waitCh, r.Clock, grace) {
			cleanupLogger.Warn(
				"command runner: process did not reap after cancellation",
				"event_name", "command_runner.cancel_reap_timeout",
				"command", req.Command,
				"args_count", len(req.Args),
				"grace_ms", grace.Milliseconds(),
			)
		}
		closeCommandProcessTree(cmd, tree, r.Clock, postRunCleanup)
		return CommandResult{
			Stdout: stdout.Bytes(),
			Stderr: stderr.Bytes(),
		}, ctx.Err()
	}
	closeCommandProcessTree(cmd, tree, r.Clock, postRunCleanup)

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

const commandReapPollInterval = 10 * time.Millisecond

// waitForCommandExit bounds the cancellation path even when an inherited
// stdout/stderr pipe remains open in a descendant that process-tree cleanup
// could not reach. The injected clock controls the deadline; the short sleep
// only prevents a busy loop while waiting for either the process or the clock.
func waitForCommandExit(waitCh <-chan error, clock Clock, grace time.Duration) bool {
	if grace <= 0 {
		select {
		case <-waitCh:
			return true
		default:
			return false
		}
	}
	deadline := clock.Now().Add(grace)
	for {
		select {
		case <-waitCh:
			return true
		default:
		}
		if !clock.Now().Before(deadline) {
			return false
		}
		time.Sleep(commandReapPollInterval)
	}
}

var _ CommandRunner = ExecCommandRunner{}
