package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// CommandRunner executes a low-level subprocess request for worker code.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

// Clock is the exact wall-clock and timer effect required by process cleanup
// and diagnostic timing. Production clocks are selected by the application
// injector; callers must not rely on a process-global fallback.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// ProcessLifecycleObserver receives policy-free facts about the parent
// process started for one command. It is intentionally narrower than a
// command result: an observer may learn that the process is gone while the
// command runner is still waiting for inherited pipes or other cleanup.
// Implementations must return promptly.
type ProcessLifecycleObserver interface {
	ProcessStarted(ProcessInfo)
	ProcessExited(ProcessInfo)
}

// ProcessInfo identifies the exact host process observed by the platform
// effect. PID is useful for diagnostics only; callers must not use it as a
// durable identity after the observation ends.
type ProcessInfo struct {
	PID int
}

type processLifecycleObserverContextKey struct{}

// WithProcessLifecycleObserver attaches one process observer to a command
// context. The value is an optional effect supplied by the caller; process
// execution remains policy-free and does not decide what an exit means.
func WithProcessLifecycleObserver(ctx context.Context, observer ProcessLifecycleObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, processLifecycleObserverContextKey{}, observer)
}

func processLifecycleObserverFromContext(ctx context.Context) ProcessLifecycleObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(processLifecycleObserverContextKey{}).(ProcessLifecycleObserver)
	return observer
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
	cmd, stdout, stderr, err := r.prepareCommand(req, observer, streaming)
	if err != nil {
		return CommandResult{}, err
	}

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
	waitDone := make(chan struct{})
	go func() {
		waitErr := cmd.Wait()
		close(waitDone)
		waitCh <- waitErr
	}()
	processMonitor := startProcessLifecycleMonitor(
		cmd,
		waitDone,
		processLifecycleObserverFromContext(ctx),
	)
	defer processMonitor.stopAndWait()

	cancelCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonCancel)
	postRunCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonPostRun)

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		_ = terminateCommandProcessTree(cmd, tree, r.Clock, cancelCleanup)
		waitForCommandCancellation(waitCh, r.Clock, cleanupLogger, req)
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

func (r ExecCommandRunner) prepareCommand(
	req CommandRequest,
	observer OutputChunkObserver,
	streaming bool,
) (*exec.Cmd, *observedBuffer, *observedBuffer, error) {
	if r.NewCommand == nil {
		return nil, nil, nil, errors.New("platform process command factory is required")
	}
	if r.Clock == nil {
		return nil, nil, nil, errors.New("platform process clock is required")
	}
	cmd := r.NewCommand(req.Command, req.Args...)
	if cmd == nil {
		return nil, nil, nil, fmt.Errorf("platform process command factory returned nil for %q", req.Command)
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
	return cmd, stdout, stderr, nil
}

const defaultPostRunCleanupGracePeriod = 10 * time.Second

var postRunCleanupGracePeriodForTest time.Duration

func postRunCleanupGracePeriod() time.Duration {
	if postRunCleanupGracePeriodForTest > 0 {
		return postRunCleanupGracePeriodForTest
	}
	return defaultPostRunCleanupGracePeriod
}

// waitForCommandExit bounds the cancellation path even when an inherited
// stdout/stderr pipe remains open in a descendant that process-tree cleanup
// could not reach. The injected timer, rather than an ambient sleep, controls
// the grace boundary even when the clock's Now value is static.
func waitForCommandExit(waitCh <-chan error, clock Clock, grace time.Duration) bool {
	if grace <= 0 {
		select {
		case <-waitCh:
			return true
		default:
			return false
		}
	}
	select {
	case <-waitCh:
		return true
	case <-clock.After(grace):
		return false
	}
}

func waitForCommandCancellation(
	waitCh <-chan error,
	clock Clock,
	logger logging.Logger,
	req CommandRequest,
) {
	grace := postRunCleanupGracePeriod()
	if waitForCommandExit(waitCh, clock, grace) {
		return
	}
	logger.Warn(
		"command runner: process did not reap after cancellation",
		"event_name", "command_runner.cancel_reap_timeout",
		"command", req.Command,
		"args_count", len(req.Args),
		"grace_ms", grace.Milliseconds(),
	)
}

var _ CommandRunner = ExecCommandRunner{}

const (
	processLifecyclePollInterval = 10 * time.Millisecond
	processExitObservationGrace  = 50 * time.Millisecond
)

// processLifecycleMonitor watches the parent process independently from
// exec.Cmd.Wait. That distinction is the important failure boundary: Wait
// can remain blocked by inherited pipes after the parent process has exited.
type processLifecycleMonitor struct {
	observer ProcessLifecycleObserver
	info     ProcessInfo
	stop     chan struct{}
	done     chan struct{}
	exited   sync.Once
}

func startProcessLifecycleMonitor(
	cmd *exec.Cmd,
	waitDone <-chan struct{},
	observer ProcessLifecycleObserver,
) *processLifecycleMonitor {
	if cmd == nil || cmd.Process == nil || observer == nil {
		return nil
	}
	monitor := &processLifecycleMonitor{
		observer: observer,
		info:     ProcessInfo{PID: cmd.Process.Pid},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	observer.ProcessStarted(monitor.info)
	go monitor.watch(cmd, waitDone)
	return monitor
}

func (m *processLifecycleMonitor) watch(cmd *exec.Cmd, waitDone <-chan struct{}) {
	defer close(m.done)
	ticker := time.NewTicker(processLifecyclePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			if commandProcessLeaderRunning(cmd) {
				continue
			}
			grace := time.NewTimer(processExitObservationGrace)
			select {
			case <-m.stop:
				if !grace.Stop() {
					<-grace.C
				}
				return
			case <-waitDone:
				if !grace.Stop() {
					<-grace.C
				}
				return
			case <-grace.C:
				m.notifyExit()
				return
			}
		}
	}
}

func (m *processLifecycleMonitor) notifyExit() {
	if m == nil || m.observer == nil {
		return
	}
	m.exited.Do(func() {
		m.observer.ProcessExited(m.info)
	})
}

func (m *processLifecycleMonitor) stopAndWait() {
	if m == nil {
		return
	}
	close(m.stop)
	<-m.done
}
