package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// ProcessStateReader observes the non-reaping host state for one process.
// Production supplies the procfs-backed implementation from the composition
// root; tests can provide a deterministic reader or nil when signal-0 alone
// is sufficient.
type ProcessStateReader func(pid int) (state byte, ok bool)

// NewProcfsProcessStateReader adapts an injected file reader to the Linux
// /proc/<pid>/stat state probe. Keeping file access at the composition
// boundary makes process observation explicit and leaves this package free of
// ambient filesystem effects.
func NewProcfsProcessStateReader(readFile func(string) ([]byte, error)) ProcessStateReader {
	if readFile == nil {
		return nil
	}
	return func(pid int) (byte, bool) {
		if pid <= 0 {
			return 0, false
		}
		data, err := readFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return 0, false
		}
		closeParen := bytes.LastIndexByte(data, ')')
		if closeParen < 0 || closeParen+2 >= len(data) {
			return 0, false
		}
		return data[closeParen+2], true
	}
}

// CommandFactory creates one inert host command. Production command creation
// is selected by the application injector so subprocess implementations do
// not call os/exec entrypoints themselves.
type CommandFactory func(name string, args ...string) *exec.Cmd

// CommandRequest describes one policy-free subprocess effect.
type CommandRequest struct {
	Command                  string                   `json:"command"`
	Args                     []string                 `json:"args,omitempty"`
	Stdin                    []byte                   `json:"stdin,omitempty"`
	Env                      []string                 `json:"env,omitempty"`
	WorkDir                  string                   `json:"work_dir,omitempty"`
	ExecutionScopeID         string                   `json:"execution_scope_id,omitempty"`
	ExecutionLogger          logging.Logger           `json:"-"`
	ProcessLifecycleObserver ProcessLifecycleObserver `json:"-"`
}

// CommandResult captures the observable output and exit status from a command.
type CommandResult struct {
	Stdout             []byte
	Stderr             []byte
	ExitCode           int
	CancellationReason CancellationReason
}

// CancellationReason identifies why a command context was deliberately
// canceled. It is carried by context cancellation so process cleanup can
// preserve intent without importing a higher-level Worker or Runtime type.
type CancellationReason string

const (
	CancellationReasonCanceled    CancellationReason = "CANCELED"
	CancellationReasonSuperseded  CancellationReason = "SUPERSEDED"
	CancellationReasonProcessGone CancellationReason = "PROCESS_GONE"
)

// CancellationCause is the safe context cause used for deliberate command
// cancellation. It still unwraps to context.Canceled so existing cancellation
// handling remains compatible while cleanup and logs retain the reason.
type CancellationCause struct {
	Reason CancellationReason
}

func (cause CancellationCause) Error() string {
	reason := cause.Reason
	if reason == "" {
		reason = CancellationReasonCanceled
	}
	return fmt.Sprintf("command execution canceled: %s", reason)
}

func (cause CancellationCause) Unwrap() error { return context.Canceled }

// NewCancellationCause creates a context cancellation cause with a bounded
// reason. Unknown or empty reasons are normalized to ordinary cancellation.
func NewCancellationCause(reason CancellationReason) error {
	if reason == "" {
		reason = CancellationReasonCanceled
	}
	switch reason {
	case CancellationReasonCanceled, CancellationReasonSuperseded, CancellationReasonProcessGone:
		return CancellationCause{Reason: reason}
	default:
		return CancellationCause{Reason: CancellationReasonCanceled}
	}
}

// CancellationReasonFromError extracts an explicit cancellation reason from
// an error chain. It returns empty for deadlines and ordinary failures.
func CancellationReasonFromError(err error) CancellationReason {
	if err == nil {
		return ""
	}
	var cause CancellationCause
	if errors.As(err, &cause) {
		return cause.Reason
	}
	var causePointer *CancellationCause
	if errors.As(err, &causePointer) && causePointer != nil {
		return causePointer.Reason
	}
	if errors.Is(err, context.Canceled) {
		return CancellationReasonCanceled
	}
	return ""
}

// CancellationReasonFromContext returns the explicit reason for a canceled
// context, or ordinary CANCELED when a caller used context.WithCancel.
func CancellationReasonFromContext(ctx context.Context) CancellationReason {
	if ctx == nil || ctx.Err() != context.Canceled {
		return ""
	}
	if reason := CancellationReasonFromError(context.Cause(ctx)); reason != "" {
		return reason
	}
	return CancellationReasonCanceled
}

func firstCancellationReason(reasons ...CancellationReason) CancellationReason {
	for _, reason := range reasons {
		if reason != "" {
			return reason
		}
	}
	return ""
}

// WindowsCommandLineLimit is the maximum number of UTF-16 code units, including
// the terminating null, that the Windows process loader accepts for one
// composed command line. os/exec joins the command name, every argument, and
// its quoting into that single string, so an argument large enough to cross
// this bound is rejected before any child process exists.
const WindowsCommandLineLimit = 32767

// ComposedCommandLineLength reports the length, in UTF-16 code units, of the
// command line the Windows process loader receives for one command and its
// arguments. The quoting mirrors os/exec, and the command name is measured as
// written because os/exec composes the command line from the requested argv
// rather than the resolved executable path. The measurement is computed the
// same way on every platform so command-line growth stays observable from any
// host, while the injected ExecCommandRunner.CommandLineLimit decides whether
// that growth is fatal.
func ComposedCommandLineLength(command string, args []string) int {
	var line strings.Builder
	appendEscapedCommandLineArgument(&line, command)
	for _, arg := range args {
		line.WriteByte(' ')
		appendEscapedCommandLineArgument(&line, arg)
	}
	return utf16CodeUnitLength(line.String())
}

// appendEscapedCommandLineArgument mirrors the quoting os/exec applies when it
// composes a Windows command line. Reproducing the exact escaping keeps a
// measured length equal to the string the process loader receives instead of an
// approximation that could sit on the wrong side of the limit.
func appendEscapedCommandLineArgument(line *strings.Builder, arg string) {
	if arg == "" {
		line.WriteString(`""`)
		return
	}
	if !strings.ContainsAny(arg, " \t\"\\") {
		line.WriteString(arg)
		return
	}
	quoted := strings.ContainsAny(arg, " \t")
	if quoted {
		line.WriteByte('"')
	}
	backslashes := 0
	for index := 0; index < len(arg); index++ {
		char := arg[index]
		switch char {
		case '\\':
			backslashes++
		case '"':
			for ; backslashes > 0; backslashes-- {
				line.WriteByte('\\')
			}
			line.WriteByte('\\')
		default:
			backslashes = 0
		}
		line.WriteByte(char)
	}
	if quoted {
		for ; backslashes > 0; backslashes-- {
			line.WriteByte('\\')
		}
		line.WriteByte('"')
	}
}

// utf16CodeUnitLength counts the UTF-16 code units Windows stores for one
// command line. Characters outside the basic multilingual plane occupy two
// units, so a byte or rune count would misreport the measured limit.
func utf16CodeUnitLength(value string) int {
	units := 0
	for _, char := range value {
		units++
		if char > 0xFFFF {
			units++
		}
	}
	return units
}

// ExecCommandRunner implements CommandRunner by delegating to os/exec.
type ExecCommandRunner struct {
	// Logger emits structured process-group cleanup diagnostics. Nil disables cleanup logging.
	Logger             logging.Logger
	Clock              Clock
	NewCommand         CommandFactory
	ProcessStateReader ProcessStateReader
	// CommandLineLimit is the composed command-line bound the host process
	// loader enforces for a single spawn, injected by the application injector
	// because the running operating system is a policy this package must not
	// select for itself. Zero means the host states no single composed-line cap,
	// which is how Unix hosts report: they bound the total argument block and
	// each individual argument rather than the composed line, so their spawn
	// failures are named from the operating system error alone.
	CommandLineLimit int
}

// NewExecCommandRunner constructs a host command runner from exact external
// effects. Missing effects fail closed rather than selecting ambient defaults.
func NewExecCommandRunner(newCommand CommandFactory, clock Clock, logger logging.Logger, processStateReader ProcessStateReader) (ExecCommandRunner, error) {
	if newCommand == nil {
		return ExecCommandRunner{}, errors.New("platform process command factory is required")
	}
	if clock == nil {
		return ExecCommandRunner{}, errors.New("platform process clock is required")
	}
	return ExecCommandRunner{Logger: logger, Clock: clock, NewCommand: newCommand, ProcessStateReader: processStateReader}, nil
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
		return CommandResult{CancellationReason: CancellationReasonFromContext(ctx)}, err
	}
	cmd, stdout, stderr, err := r.prepareCommand(req, observer, streaming)
	if err != nil {
		return CommandResult{}, err
	}

	cleanupLogger := logging.EnsureLogger(r.Logger)
	configureCommandProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		return CommandResult{}, r.reportCommandStartFailure(cleanupLogger, req, err)
	}

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
		req.ProcessLifecycleObserver,
		r.ProcessStateReader,
	)
	defer processMonitor.stopAndWait()

	cancellationReason := CancellationReasonFromContext(ctx)
	cancelCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonCancel, cancellationReason)
	postRunCleanup := newCommandProcessCleanupContext(cleanupLogger, req, commandProcessCleanupReasonPostRun)

	var runErr error
	select {
	case runErr = <-waitCh:
	case <-ctx.Done():
		// The context cause is not necessarily installed when the command
		// process is started. Read it at the cancellation edge so cleanup
		// telemetry carries the same lifecycle reason as CommandResult.
		cancellationReason = CancellationReasonFromContext(ctx)
		cancelCleanup.cancellationReason = cancellationReason
		_ = terminateCommandProcessTree(cmd, tree, r.Clock, cancelCleanup)
		waitForCommandCancellation(waitCh, r.Clock, cleanupLogger, req)
		closeCommandProcessTree(cmd, tree, r.Clock, postRunCleanup)
		return CommandResult{
			Stdout:             stdout.Bytes(),
			Stderr:             stderr.Bytes(),
			CancellationReason: cancellationReason,
		}, ctx.Err()
	}
	closeCommandProcessTree(cmd, tree, r.Clock, postRunCleanup)

	result := CommandResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
	if runErr != nil {
		if ctx.Err() != nil {
			result.CancellationReason = CancellationReasonFromContext(ctx)
			return result, ctx.Err()
		}
		return r.resultFromWaitError(result, runErr, cleanupLogger, req)
	}
	return result, nil
}

// CommandStartError names a subprocess that never started. The measured
// command line travels with the error because a command line the process loader
// rejects produces no child, no exit status, and no output, which otherwise
// leaves the caller with a bare operating-system error and no way to tell an
// oversized command line apart from a missing executable.
type CommandStartError struct {
	Command           string
	ArgsCount         int
	CommandLineLength int
	CommandLineLimit  int
	StdinBytes        int
	Cause             error
}

// OverCommandLineLimit reports whether the composed command line reached the
// bound the host enforces. That is the one spawn failure a caller can repair by
// moving argument content to stdin or a file, so it is named separately from
// every other start failure.
func (e *CommandStartError) OverCommandLineLimit() bool {
	if e == nil || e.CommandLineLimit <= 0 {
		return false
	}
	return e.CommandLineLength >= e.CommandLineLimit
}

func (e *CommandStartError) Error() string {
	if e == nil {
		return ""
	}
	if e.OverCommandLineLimit() {
		return fmt.Sprintf(
			"start %q: composed command line is %d characters across %d arguments, "+
				"at or above the %d-character host command-line limit: %v",
			e.Command, e.CommandLineLength, e.ArgsCount, e.CommandLineLimit, e.Cause,
		)
	}
	return fmt.Sprintf(
		"start %q: process start failed with a %d-character command line across %d arguments: %v",
		e.Command, e.CommandLineLength, e.ArgsCount, e.Cause,
	)
}

func (e *CommandStartError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// reportCommandStartFailure names and records a spawn that produced no child
// process. Returning the operating-system error bare left an oversized command
// line indistinguishable from any other execution failure and wrote nothing to
// the log, so the only remaining evidence was how quickly the attempt died. The
// bound the failure is judged against is the runner's injected limit, so this
// package never has to ask which operating system it is running on.
func (r ExecCommandRunner) reportCommandStartFailure(logger logging.Logger, req CommandRequest, cause error) error {
	startErr := &CommandStartError{
		Command:           req.Command,
		ArgsCount:         len(req.Args),
		CommandLineLength: ComposedCommandLineLength(req.Command, req.Args),
		CommandLineLimit:  r.CommandLineLimit,
		StdinBytes:        len(req.Stdin),
		Cause:             cause,
	}
	logger.Error(
		"command runner: process start failed",
		"event_name", commandRunnerStartFailedEvent,
		"command", req.Command,
		"args_count", startErr.ArgsCount,
		"command_line_chars", startErr.CommandLineLength,
		"command_line_limit", startErr.CommandLineLimit,
		"over_command_line_limit", startErr.OverCommandLineLimit(),
		"stdin_bytes", startErr.StdinBytes,
		"working_dir", req.WorkDir,
		"error", cause.Error(),
	)
	return startErr
}

// commandRunnerStartFailedEvent is the greppable event name an operator uses to
// find a spawn that died before the child process existed.
const commandRunnerStartFailedEvent = "command_runner.start_failed"

// resultFromWaitError converts one terminal cmd.Wait error into the runner's
// result contract.
//
// exec.ErrWaitDelay is the exact process-gone-with-open-pipe fact this runner
// must not stall on: the started process already exited successfully, and
// os/exec force-closed the inherited stdout/stderr descriptors that a surviving
// descendant kept open past the wait delay. The exit status is authoritative,
// so the run reports the command's own success rather than the pipe's fate.
func (r ExecCommandRunner) resultFromWaitError(
	result CommandResult,
	runErr error,
	logger logging.Logger,
	req CommandRequest,
) (CommandResult, error) {
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if errors.Is(runErr, exec.ErrWaitDelay) {
		logger.Warn(
			"command runner: process exited while a descendant held its output pipe open",
			"event_name", "command_runner.output_pipe_wait_delay_elapsed",
			"command", req.Command,
			"args_count", len(req.Args),
			"wait_delay_ms", orphanedOutputPipeGracePeriod().Milliseconds(),
		)
		return result, nil
	}
	return result, runErr
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
	// Assigning plain io.Writers makes os/exec create pipes plus copy
	// goroutines, and cmd.Wait joins those goroutines after the process itself
	// is reaped. A descendant that inherited the write end therefore keeps
	// cmd.Wait blocked indefinitely once the started process is gone. WaitDelay
	// bounds exactly that window: os/exec force-closes the inherited
	// descriptors once the delay elapses after process exit, so a dead process
	// always produces a terminal result instead of an unbounded wait.
	cmd.WaitDelay = orphanedOutputPipeGracePeriod()
	return cmd, stdout, stderr, nil
}

const defaultPostRunCleanupGracePeriod = 10 * time.Second

// defaultOrphanedOutputPipeGracePeriod bounds how long cmd.Wait keeps waiting
// on inherited stdout/stderr descriptors after the started process has already
// been reaped. It is the wait-delay boundary between "output still draining"
// and "a surviving descendant is holding these descriptors open forever". Five
// seconds is far longer than a drained kernel pipe buffer needs once the
// writers are gone, and vastly below the two-hour workstation execution timeout
// that used to be the only exit from a wedged wait.
const defaultOrphanedOutputPipeGracePeriod = 5 * time.Second

var (
	postRunCleanupGracePeriodForTest     time.Duration
	orphanedOutputPipeGracePeriodForTest time.Duration
)

func postRunCleanupGracePeriod() time.Duration {
	if postRunCleanupGracePeriodForTest > 0 {
		return postRunCleanupGracePeriodForTest
	}
	return defaultPostRunCleanupGracePeriod
}

func orphanedOutputPipeGracePeriod() time.Duration {
	if orphanedOutputPipeGracePeriodForTest > 0 {
		return orphanedOutputPipeGracePeriodForTest
	}
	return defaultOrphanedOutputPipeGracePeriod
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
	observer    ProcessLifecycleObserver
	info        ProcessInfo
	stateReader ProcessStateReader
	stop        chan struct{}
	done        chan struct{}
	exited      sync.Once
}

func startProcessLifecycleMonitor(
	cmd *exec.Cmd,
	waitDone <-chan struct{},
	observer ProcessLifecycleObserver,
	stateReader ProcessStateReader,
) *processLifecycleMonitor {
	if cmd == nil || cmd.Process == nil || observer == nil {
		return nil
	}
	monitor := &processLifecycleMonitor{
		observer:    observer,
		info:        ProcessInfo{PID: cmd.Process.Pid},
		stateReader: stateReader,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
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
			if commandProcessLeaderRunning(cmd, m.stateReader) {
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
