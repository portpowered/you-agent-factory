package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

func TestDefaultPostRunCleanupGracePeriod(t *testing.T) {
	if defaultPostRunCleanupGracePeriod != 10*time.Second {
		t.Fatalf("defaultPostRunCleanupGracePeriod = %v, want 10s", defaultPostRunCleanupGracePeriod)
	}
}

func TestPostRunCleanupGracePeriod_TestHookOverridesDefault(t *testing.T) {
	t.Cleanup(func() {
		postRunCleanupGracePeriodForTest = 0
	})

	if postRunCleanupGracePeriod() != defaultPostRunCleanupGracePeriod {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want default %v", postRunCleanupGracePeriod(), defaultPostRunCleanupGracePeriod)
	}

	postRunCleanupGracePeriodForTest = 25 * time.Millisecond
	if postRunCleanupGracePeriod() != 25*time.Millisecond {
		t.Fatalf("postRunCleanupGracePeriod() = %v, want test override 25ms", postRunCleanupGracePeriod())
	}
}

type deterministicCommandTimerClock struct {
	now   time.Time
	timer chan time.Time
}

func (clock *deterministicCommandTimerClock) Now() time.Time {
	return clock.now
}

func (clock *deterministicCommandTimerClock) After(time.Duration) <-chan time.Time {
	return clock.timer
}

func (clock *deterministicCommandTimerClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
	clock.timer <- clock.now
}

func TestWaitForCommandExitUsesInjectedTimerWhenClockNowIsStatic(t *testing.T) {
	clock := &deterministicCommandTimerClock{
		now:   time.Unix(42, 0),
		timer: make(chan time.Time, 1),
	}
	waitCh := make(chan error)
	result := make(chan bool, 1)
	go func() {
		result <- waitForCommandExit(waitCh, clock, time.Second)
	}()

	select {
	case got := <-result:
		t.Fatalf("waitForCommandExit returned %t before injected timer advanced", got)
	case <-time.After(50 * time.Millisecond):
	}

	clock.Advance(time.Second)
	select {
	case got := <-result:
		if got {
			t.Fatal("waitForCommandExit returned true after injected grace timer fired")
		}
	case <-time.After(time.Second):
		t.Fatal("waitForCommandExit did not observe the injected grace timer")
	}
}

func TestNewProcfsProcessStateReaderUsesInjectedFileEffect(t *testing.T) {
	tests := []struct {
		name      string
		pid       int
		data      []byte
		readErr   error
		wantState byte
		wantOK    bool
		wantPath  string
	}{
		{name: "invalid pid", pid: 0},
		{name: "read error", pid: 42, readErr: fmt.Errorf("not mounted"), wantPath: "/proc/42/stat"},
		{name: "malformed stat", pid: 42, data: []byte("42 missing close paren"), wantPath: "/proc/42/stat"},
		{name: "valid stat", pid: 42, data: []byte("42 (worker child) S 1 2 3"), wantState: 'S', wantOK: true, wantPath: "/proc/42/stat"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotPath string
			reader := NewProcfsProcessStateReader(func(path string) ([]byte, error) {
				gotPath = path
				return test.data, test.readErr
			})
			state, ok := reader(test.pid)
			if state != test.wantState || ok != test.wantOK {
				t.Fatalf("state = %q, ok = %t, want %q, %t", state, ok, test.wantState, test.wantOK)
			}
			if gotPath != test.wantPath {
				t.Fatalf("read path = %q, want %q", gotPath, test.wantPath)
			}
		})
	}

	if reader := NewProcfsProcessStateReader(nil); reader != nil {
		t.Fatal("nil file effect returned a process state reader")
	}
}

func TestCommandProcessLeaderRunningUsesInjectedStateWithoutProcessStateRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("procfs state probe is Unix-specific")
	}
	cmd := &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	if commandProcessLeaderRunning(cmd, func(int) (byte, bool) { return 'Z', true }) {
		t.Fatal("zombie process state reported as running")
	}
	if !commandProcessLeaderRunning(cmd, func(int) (byte, bool) { return 'S', true }) {
		t.Fatal("live process state reported as stopped")
	}
	if commandProcessLeaderRunning(nil, nil) {
		t.Fatal("nil command reported as running")
	}
}

func TestProcessLifecycleMonitorStopsWhenWaitCompletes(t *testing.T) {
	cmd := &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	waitDone := make(chan struct{})
	close(waitDone)
	observer := &lifecycleObserverRecorder{
		started: make(chan ProcessInfo, 1),
		exited:  make(chan ProcessInfo, 1),
	}
	monitor := startProcessLifecycleMonitor(cmd, waitDone, observer, NewProcfsProcessStateReader(os.ReadFile))
	if monitor == nil {
		t.Fatal("startProcessLifecycleMonitor() returned nil")
	}
	monitor.stopAndWait()
	select {
	case <-observer.started:
	default:
		t.Fatal("monitor did not report process start")
	}
	select {
	case <-observer.exited:
		t.Fatal("monitor reported exit after wait completed")
	default:
	}
}

func TestNewExecCommandRunnerRejectsMissingEffects(t *testing.T) {
	if _, err := NewExecCommandRunner(nil, platformclock.Real{}, nil, nil); err == nil {
		t.Fatal("nil command factory was accepted")
	}
	if _, err := NewExecCommandRunner(exec.Command, nil, nil, nil); err == nil {
		t.Fatal("nil process clock was accepted")
	}
}

func TestExecCommandRunnerRunRejectsMissingRuntimeEffects(t *testing.T) {
	tests := []struct {
		name   string
		runner ExecCommandRunner
		want   string
	}{
		{
			name: "command factory",
			runner: ExecCommandRunner{
				Clock: platformclock.Real{},
			},
			want: "command factory",
		},
		{
			name: "clock",
			runner: ExecCommandRunner{
				NewCommand: exec.Command,
			},
			want: "clock",
		},
		{
			name: "nil command",
			runner: ExecCommandRunner{
				Clock:      platformclock.Real{},
				NewCommand: func(string, ...string) *exec.Cmd { return nil },
			},
			want: "returned nil",
		},
		{
			name: "start error",
			runner: ExecCommandRunner{
				Clock:      platformclock.Real{},
				NewCommand: func(string, ...string) *exec.Cmd { return &exec.Cmd{} },
			},
			want: "exec",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.runner.Run(context.Background(), CommandRequest{Command: "helper"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run() error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestStreamingExecCommandRunnerRunForwardsInjectedOutput(t *testing.T) {
	requireProcessIntegration(t)
	var observed []string
	runner := StreamingExecCommandRunner{
		Clock:      platformclock.Real{},
		NewCommand: exec.Command,
		Observer: func(stream string, chunk []byte) {
			observed = append(observed, stream+":"+string(chunk))
		},
	}
	result, err := runner.Run(context.Background(), CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"fail",
		},
		Env: append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
	})
	if err != nil {
		t.Fatalf("StreamingExecCommandRunner.Run() error = %v, want nil for non-zero exit", err)
	}
	if result.ExitCode != 17 || len(observed) == 0 {
		t.Fatalf("streaming result = %#v, observations = %#v, want exit 17 and output", result, observed)
	}
}

func TestProcessLifecycleMonitorStopsDuringExitObservationGrace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("injected process state probe is Unix-specific")
	}
	cmd := &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	waitDone := make(chan struct{})
	stateRead := make(chan struct{}, 1)
	observer := &lifecycleObserverRecorder{
		started: make(chan ProcessInfo, 1),
		exited:  make(chan ProcessInfo, 1),
	}
	monitor := startProcessLifecycleMonitor(cmd, waitDone, observer, func(int) (byte, bool) {
		select {
		case stateRead <- struct{}{}:
		default:
		}
		return 'Z', true
	})
	if monitor == nil {
		t.Fatal("startProcessLifecycleMonitor() returned nil")
	}
	<-stateRead
	monitor.stopAndWait()
	select {
	case <-observer.exited:
		t.Fatal("monitor reported exit after stop")
	default:
	}
}

func TestProcessLifecycleMonitorNotifyExitIgnoresMissingObserver(t *testing.T) {
	var nilMonitor *processLifecycleMonitor
	nilMonitor.notifyExit()
	(&processLifecycleMonitor{}).notifyExit()
}

func TestExecCommandRunner_RunStreamingBoundsRetainedOutputAndForwardsAllChunks(t *testing.T) {
	requireProcessIntegration(t)
	const (
		stdoutMarker = "streaming stdout complete"
		stderrMarker = "authentication failed after large output"
	)
	var observedStdout, observedStderr int
	result, err := testExecCommandRunner(t, nil).RunStreaming(
		context.Background(),
		CommandRequest{
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestExecCommandRunner_HelperProcess",
				"--",
				"streaming-output",
			},
			Env: append(os.Environ(), "GO_WANT_COMMAND_HELPER=1"),
		},
		func(stream string, chunk []byte) {
			switch stream {
			case OutputStreamStdout:
				observedStdout += len(chunk)
			case OutputStreamStderr:
				observedStderr += len(chunk)
			}
		},
	)
	if err != nil {
		t.Fatalf("RunStreaming() error = %v, want nil with non-zero exit result", err)
	}
	wantObservedBytes := streamingHelperOutputBytes()
	if observedStdout != wantObservedBytes || observedStderr != wantObservedBytes {
		t.Fatalf("observed output bytes = stdout %d/stderr %d, want %d each", observedStdout, observedStderr, wantObservedBytes)
	}
	if len(result.Stdout) > maxStreamingOutputBytes || len(result.Stderr) > maxStreamingOutputBytes {
		t.Fatalf("retained output bytes = stdout %d/stderr %d, want at most %d each", len(result.Stdout), len(result.Stderr), maxStreamingOutputBytes)
	}
	if !strings.Contains(string(result.Stdout), stdoutMarker) {
		t.Fatalf("retained stdout = %q, want terminal marker %q", result.Stdout, stdoutMarker)
	}
	if !strings.Contains(string(result.Stderr), stderrMarker) {
		t.Fatalf("retained stderr = %q, want terminal classification marker %q", result.Stderr, stderrMarker)
	}
	if result.ExitCode != 23 {
		t.Fatalf("ExitCode = %d, want 23", result.ExitCode)
	}
}

func streamingHelperOutputBytes() int {
	return maxStreamingOutputBytes*4 + len("streaming stdout complete\n")
}

func writeCommandHelperOutput(writer io.Writer, total int, marker string) {
	markerBytes := []byte(marker + "\n")
	remaining := total - len(markerBytes)
	chunk := []byte(strings.Repeat("x", 32<<10))
	for remaining > 0 {
		count := len(chunk)
		if count > remaining {
			count = remaining
		}
		if _, err := writer.Write(chunk[:count]); err != nil {
			fmt.Fprintf(os.Stderr, "write helper output: %v\n", err)
			os.Exit(2)
		}
		remaining -= count
	}
	if _, err := writer.Write(markerBytes); err != nil {
		fmt.Fprintf(os.Stderr, "write helper marker: %v\n", err)
		os.Exit(2)
	}
}

type coverageProcessClock struct {
	times []time.Time
	next  int
}

func (clock *coverageProcessClock) Now() time.Time {
	if clock.next >= len(clock.times) {
		return clock.times[len(clock.times)-1]
	}
	value := clock.times[clock.next]
	clock.next++
	return value
}

func (clock *coverageProcessClock) After(time.Duration) <-chan time.Time {
	return make(chan time.Time)
}

func TestLoggingCommandRunnerAndStatusProjection(t *testing.T) {
	logger := &recordingCommandLogger{}
	clock := &coverageProcessClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}}
	runner := CommandRunnerWithLogging(
		fixedCommandRunnerWithError{result: CommandResult{Stdout: []byte("ok")}},
		logger,
		clock,
	)
	result, err := runner.Run(context.Background(), CommandRequest{Command: "tool", Args: []string{"--version"}})
	if err != nil || string(result.Stdout) != "ok" {
		t.Fatalf("logged success = %#v, %v", result, err)
	}
	if len(logger.infos) != 2 || len(logger.verboses) != 2 {
		t.Fatalf("logged success entries = info %d/verbose %d, want two each", len(logger.infos), len(logger.verboses))
	}

	failedLogger := &recordingCommandLogger{}
	failed, err := (&LoggingCommandRunner{
		Runner: fixedCommandRunnerWithError{
			result: CommandResult{ExitCode: 7, Stderr: []byte("failed")},
			err:    errors.New("runner failed"),
		},
		Logger: failedLogger,
		Clock:  &coverageProcessClock{times: []time.Time{time.Unix(1, 0), time.Unix(2, 0)}},
	}).Run(context.Background(), CommandRequest{Command: "tool"})
	if err == nil || failed.ExitCode != 7 || len(failedLogger.warns) != 0 {
		t.Fatalf("logged failure = %#v, %v, warns=%d", failed, err, len(failedLogger.warns))
	}
	if len(failedLogger.infos) != 2 {
		t.Fatalf("logged failure info entries = %d, want request and completion", len(failedLogger.infos))
	}

	if got := commandResultStatus(context.Background(), CommandResult{}, context.DeadlineExceeded); got != "timed_out" {
		t.Fatalf("commandResultStatus(deadline) = %q", got)
	}
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := commandResultStatus(deadline, CommandResult{}, errors.New("failed")); got != "timed_out" {
		t.Fatalf("commandResultStatus(expired context) = %q", got)
	}
	if got := commandResultStatus(context.Background(), CommandResult{ExitCode: 1}, nil); got != "failed" {
		t.Fatalf("commandResultStatus(exit) = %q", got)
	}
}

func TestCommandRunnerWithLoggingPreservesExecRunnerLogger(t *testing.T) {
	logger := &recordingCommandLogger{}
	clock := &coverageProcessClock{times: []time.Time{time.Unix(1, 0)}}
	execRunner := ExecCommandRunner{}
	if got := execCommandRunnerWithLogger(execRunner, logger).(ExecCommandRunner).Logger; got != logger {
		t.Fatal("execCommandRunnerWithLogger(value) did not inject logger")
	}
	if got := execCommandRunnerWithLogger(&execRunner, logger).(*ExecCommandRunner).Logger; got != logger {
		t.Fatal("execCommandRunnerWithLogger(pointer) did not inject logger")
	}
	if got := execCommandRunnerWithLogger(fixedCommandRunnerWithError{}, logger); got == nil {
		t.Fatal("execCommandRunnerWithLogger(default) returned nil")
	}
	if got := CommandRunnerWithLogging(execRunner, logger, clock); got == nil {
		t.Fatal("CommandRunnerWithLogging() returned nil")
	}
}

func TestCommandEnvironmentAndExecutableEffects(t *testing.T) {
	if got := CommandEnvEntriesFromMap(nil); got != nil {
		t.Fatalf("CommandEnvEntriesFromMap(nil) = %#v, want nil", got)
	}
	entries := CommandEnvEntriesFromMap(map[string]string{"Z": "last", "A": "first"})
	if len(entries) != 2 || entries[0].Name != "A" || entries[1].Name != "Z" {
		t.Fatalf("sorted environment entries = %#v", entries)
	}
	merged := MergeCommandEnv(
		[]string{"A=base", "invalid", "B=base"},
		[]CommandEnvEntry{{Name: "B", Value: "overlay"}, {Name: "", Value: "ignored"}},
		[]CommandEnvEntry{{Name: "C", Value: "added"}},
	)
	if len(merged) != 3 || merged[0] != "A=base" || merged[1] != "B=overlay" || merged[2] != "C=added" {
		t.Fatalf("MergeCommandEnv() = %#v", merged)
	}
	if _, err := (HostExecutableLocator{}).LookPath(os.Args[0]); err != nil {
		t.Fatalf("HostExecutableLocator.LookPath(%q) = %v", os.Args[0], err)
	}
}

func TestCommandCleanupLogsGracefulTermination(t *testing.T) {
	logger := &recordingCommandLogger{}
	cleanup := newCommandProcessCleanupContext(logger, CommandRequest{
		Command: "tool",
		WorkDir: "/work",
	}, commandProcessCleanupReasonCancel)
	cleanup.logGraceful(42)
	if len(logger.verboses) != 1 || logger.verboses[0].fields["status"] != "graceful" ||
		logger.verboses[0].fields["process_group_id"] != 42 {
		t.Fatalf("graceful cleanup log = %#v, want graceful status and process group", logger.verboses)
	}
}

type lifecycleObserverRecorder struct {
	started chan ProcessInfo
	exited  chan ProcessInfo
}

func (observer *lifecycleObserverRecorder) ProcessStarted(info ProcessInfo) {
	observer.started <- info
}

func (observer *lifecycleObserverRecorder) ProcessExited(info ProcessInfo) {
	observer.exited <- info
}

func TestProcessLifecycleMonitorObservesGoneParentBeforeWaitCompletes(t *testing.T) {
	requireProcessIntegration(t)

	pidFile := t.TempDir() + string(os.PathSeparator) + "child.pid"
	cmd := exec.Command(
		os.Args[0],
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		"spawn-child",
	)
	cmd.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitDone := make(chan struct{})
	observer := &lifecycleObserverRecorder{
		started: make(chan ProcessInfo, 1),
		exited:  make(chan ProcessInfo, 1),
	}
	monitor := startProcessLifecycleMonitor(cmd, waitDone, observer, NewProcfsProcessStateReader(os.ReadFile))
	t.Cleanup(func() {
		close(waitDone)
		monitor.stopAndWait()
		if cmd.Process != nil {
			commandTestTerminateProcess(cmd.Process.Pid)
		}
		if childPID, err := readCommandHelperPIDFile(pidFile); err == nil {
			commandTestTerminateProcess(childPID)
		}
		_ = cmd.Wait()
	})

	childPID := waitForCommandHelperPID(t, pidFile, 3*time.Second)
	select {
	case started := <-observer.started:
		if cmd.Process == nil || started.PID != cmd.Process.Pid {
			t.Fatalf("started process info = %#v, want command PID", started)
		}
	case <-time.After(time.Second):
		t.Fatal("process lifecycle monitor did not report process start")
	}

	commandTestTerminateProcess(cmd.Process.Pid)
	select {
	case exited := <-observer.exited:
		if exited.PID != cmd.Process.Pid {
			t.Fatalf("exited process info = %#v, want command PID %d", exited, cmd.Process.Pid)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("process lifecycle monitor did not report gone parent; child %d was retained", childPID)
	}
}

type cancelOnProcessExitObserver struct {
	cancel context.CancelFunc
}

func (observer cancelOnProcessExitObserver) ProcessStarted(ProcessInfo) {}

func (observer cancelOnProcessExitObserver) ProcessExited(ProcessInfo) {
	observer.cancel()
}

func TestExecCommandRunnerCancelsInheritedPipeAfterParentExitObservation(t *testing.T) {
	requireProcessIntegration(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	runner, err := NewExecCommandRunner(
		exec.Command,
		platformclock.Real{},
		nil,
		NewProcfsProcessStateReader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("NewExecCommandRunner() error = %v", err)
	}
	result, runErr := runner.Run(ctx, CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestExecCommandRunner_HelperProcess",
			"--",
			"spawn-child-success",
		},
		Env: append(
			os.Environ(),
			"GO_WANT_COMMAND_HELPER=1",
			"COMMAND_HELPER_PID_FILE="+pidFile,
		),
		ProcessLifecycleObserver: cancelOnProcessExitObserver{cancel: cancel},
	})
	if runErr == nil || !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run() result=%#v error=%v, want context canceled after parent exit", result, runErr)
	}
	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() { commandTestTerminateProcess(childPID) })
	if !waitForCommandHelperProcessExit(childPID, 2*time.Second) {
		t.Fatalf("inherited-pipe child process %d is still running after observer cancellation", childPID)
	}
}
