package agypty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
)

var testPTYClock platformclock.Source = platformclock.Real{}

func newTestPlatformPTYAllocator(t *testing.T) PTYAllocator {
	t.Helper()
	allocator, err := NewAllocator(testPTYHost{}, testPTYClock)
	if err != nil {
		t.Fatalf("NewAllocator() error = %v", err)
	}
	return allocator
}

type closeOnlyPTY struct{}

func (closeOnlyPTY) Close() error           { return nil }
func (closeOnlyPTY) Kind() platformpty.Kind { return platformpty.KindPOSIX }

type testPTYHost struct{}

func (testPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	return closeOnlyPTY{}, nil
}
func (testPTYHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, errors.New("test host start is not configured")
}

type sessionProcess struct {
	cmd      *exec.Cmd
	tree     process.SubprocessTree
	exitCode int
}

func (p *sessionProcess) Wait() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("process is not started")
	}
	err := p.cmd.Wait()
	if err == nil {
		p.exitCode = 0
		return nil
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		p.exitCode = exitErr.ExitCode()
		return nil
	}
	return err
}
func (p *sessionProcess) Terminate() error {
	if p == nil || p.cmd == nil {
		return nil
	}
	return process.TerminateSubprocessTree(p.cmd, p.tree)
}
func (p *sessionProcess) Close() {
	if p != nil && p.cmd != nil {
		process.CloseSubprocessTree(p.cmd, p.tree)
	}
}
func (p *sessionProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}
func (p *sessionProcess) ExitCode() int {
	if p == nil {
		return -1
	}
	return p.exitCode
}

type timeoutThenDataReader struct {
	readCount int
}

func (r *timeoutThenDataReader) Read(destination []byte) (int, error) {
	r.readCount++
	if r.readCount == 1 {
		return 0, os.ErrDeadlineExceeded
	}
	return copy(destination, "delayed-output"), io.EOF
}

func (*timeoutThenDataReader) Close() error { return nil }

type delayedDataReader struct {
	closed    chan struct{}
	closeOnce sync.Once
	read      bool
}

func newDelayedDataReader() *delayedDataReader {
	return &delayedDataReader{closed: make(chan struct{})}
}

func (r *delayedDataReader) Read(destination []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	select {
	case <-time.After(10 * time.Millisecond):
		r.read = true
		return copy(destination, "post-exit-output"), nil
	case <-r.closed:
		return 0, io.EOF
	}
}

func (r *delayedDataReader) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestAllocator_RejectsInvalidLaunch(t *testing.T) {
	t.Parallel()

	allocator := newTestPlatformPTYAllocator(t)
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "",
		Argv:       []string{"agy"},
	}, DefaultSessionConfig())
	if err == nil {
		t.Fatal("Allocate() error = nil, want validation error")
	}
}

func TestAllocator_RejectsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	allocator := newTestPlatformPTYAllocator(t)
	_, err := allocator.Allocate(ctx, ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "hello"},
	}, DefaultSessionConfig())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Allocate() error = %v, want %v", err, context.Canceled)
	}
}

func TestPlatformSession_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "hello"},
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{}, testPTYHost{}, testPTYClock)
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestNormalizeSessionConfig_AppliesDefaultsAndCeiling(t *testing.T) {
	t.Parallel()

	cfg := normalizeSessionConfig(SessionConfig{})
	if cfg.MaxCaptureBytes != DefaultMaxCaptureBytes {
		t.Fatalf("MaxCaptureBytes = %d, want %d", cfg.MaxCaptureBytes, DefaultMaxCaptureBytes)
	}
	if cfg.IdleTimeout != DefaultIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", cfg.IdleTimeout, DefaultIdleTimeout)
	}
	if cfg.HardTimeout != DefaultHardTimeout {
		t.Fatalf("HardTimeout = %v, want %v", cfg.HardTimeout, DefaultHardTimeout)
	}

	over := normalizeSessionConfig(SessionConfig{MaxCaptureBytes: MaxMaxCaptureBytes + 1})
	if over.MaxCaptureBytes != MaxMaxCaptureBytes {
		t.Fatalf("capped MaxCaptureBytes = %d, want %d", over.MaxCaptureBytes, MaxMaxCaptureBytes)
	}
}

func TestAllocator_ReturnsOwnerSessionFromInjectedHost(t *testing.T) {
	t.Parallel()

	allocator, err := NewAllocator(testPTYHost{}, testPTYClock)
	if err != nil {
		t.Fatalf("NewAllocator() error = %v", err)
	}
	if _, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy", Argv: []string{"/bin/agy"},
	}, DefaultSessionConfig()); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
}

func TestAllocator_RejectsMissingHostOrClock(t *testing.T) {
	t.Parallel()

	if _, err := NewAllocator(nil, testPTYClock); !errors.Is(err, ErrHostRequired) {
		t.Fatalf("NewAllocator(nil host) error = %v, want %v", err, ErrHostRequired)
	}
	if _, err := NewAllocator(testPTYHost{}, nil); !errors.Is(err, ErrClockRequired) {
		t.Fatalf("NewAllocator(nil clock) error = %v, want %v", err, ErrClockRequired)
	}
}

func TestExecuteSessionRun_CapsCaptureAndCleans(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	cmd := startBlockingTestProcess(t)
	t.Cleanup(func() {
		_ = writer.Close()
		terminateSessionTestProcess(cmd.Process.Pid)
		_ = cmd.Wait()
	})

	tree, err := processAttachForTest(cmd)
	if err != nil {
		t.Fatalf("AttachSubprocessTree() error = %v", err)
	}
	proc := &sessionProcess{cmd: cmd, tree: tree}

	go func() {
		defer writer.Close()
		payload := []byte("ABCDEFGH")
		for i := 0; i < 1024; i++ {
			_, _ = writer.Write(payload)
		}
	}()

	cfg := SessionConfig{
		MaxCaptureBytes: 128,
		IdleTimeout:     25 * time.Millisecond,
		HardTimeout:     5 * time.Second,
	}
	result, err := executeSessionRun(context.Background(), cfg, reader, proc, platformclock.Real{})
	if err != nil {
		t.Fatalf("executeSessionRun() error = %v", err)
	}
	if !result.CapacityHit {
		t.Fatal("CapacityHit = false, want true")
	}
	if len(result.RawBytes) != cfg.MaxCaptureBytes {
		t.Fatalf("len(RawBytes) = %d, want %d", len(result.RawBytes), cfg.MaxCaptureBytes)
	}
	if result.CleanedText == "" {
		t.Fatal("CleanedText is empty, want cleaned output")
	}
}

func TestExecuteSessionRun_HardTimeoutMarksTimedOut(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	cmd := startBlockingTestProcess(t)
	t.Cleanup(func() {
		_ = writer.Close()
		terminateSessionTestProcess(cmd.Process.Pid)
		_ = cmd.Wait()
	})

	tree, err := processAttachForTest(cmd)
	if err != nil {
		t.Fatalf("AttachSubprocessTree() error = %v", err)
	}
	proc := &sessionProcess{cmd: cmd, tree: tree}

	cfg := SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     time.Hour,
		HardTimeout:     50 * time.Millisecond,
	}
	result, err := executeSessionRun(context.Background(), cfg, reader, proc, platformclock.Real{})
	if err != nil {
		t.Fatalf("executeSessionRun() error = %v", err)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
	if sessionProcessRunning(proc.PID()) {
		t.Fatalf("process %d still running after hard timeout", proc.PID())
	}
}

func TestExecuteSessionRun_CancelTerminatesProcessTree(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	cmd := startBlockingTestProcess(t)
	t.Cleanup(func() {
		_ = writer.Close()
		terminateSessionTestProcess(cmd.Process.Pid)
		_ = cmd.Wait()
	})

	tree, err := processAttachForTest(cmd)
	if err != nil {
		t.Fatalf("AttachSubprocessTree() error = %v", err)
	}
	proc := &sessionProcess{cmd: cmd, tree: tree}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     time.Hour,
		HardTimeout:     time.Hour,
	}
	_, err = executeSessionRun(ctx, cfg, reader, proc, platformclock.Real{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("executeSessionRun() error = %v, want %v", err, context.Canceled)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !sessionProcessRunning(proc.PID()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("process %d still running after cancel", proc.PID())
}

func TestHelperPTYBlocker(t *testing.T) {
	if os.Getenv("GO_WANT_PTY_HELPER") != "1" {
		return
	}
	select {}
}

func processAttachForTest(cmd *exec.Cmd) (process.SubprocessTree, error) {
	return process.AttachSubprocessTree(cmd)
}

func processConfigureForTest(cmd *exec.Cmd) {
	process.ConfigureSubprocessTree(cmd)
}

func TestPTYKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind PTYKind
		want string
	}{
		{PTYKindPOSIX, "posix"},
		{PTYKindConPTY, "conpty"},
		{PTYKindUnknown, "unknown"},
		{PTYKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Fatalf("PTYKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestPlatformSessionPTYKind(t *testing.T) {
	t.Parallel()

	var nilSession *platformSession
	if got := nilSession.PTYKind(); got != PTYKindUnknown {
		t.Fatalf("nil PTYKind() = %v, want %v", got, PTYKindUnknown)
	}

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindConPTY, closeOnlyPTY{}, testPTYHost{}, testPTYClock)
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}
	if got := session.PTYKind(); got != PTYKindConPTY {
		t.Fatalf("PTYKind() = %v, want %v", got, PTYKindConPTY)
	}
}

func TestNewPlatformSession_RejectsNilPTY(t *testing.T) {
	t.Parallel()

	_, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindPOSIX, nil, testPTYHost{}, testPTYClock)
	if err == nil {
		t.Fatal("newPlatformSession() error = nil, want error")
	}
}

func TestNewPlatformSession_RejectsMissingClock(t *testing.T) {
	t.Parallel()

	_, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{}, testPTYHost{}, nil)
	if !errors.Is(err, ErrClockRequired) {
		t.Fatalf("newPlatformSession(nil clock) error = %v, want %v", err, ErrClockRequired)
	}
}

func TestValidateProcessLaunch_RejectsEmptyArgv(t *testing.T) {
	t.Parallel()

	err := validateProcessLaunch(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       nil,
	})
	if err == nil || !strings.Contains(err.Error(), "argv is required") {
		t.Fatalf("validateProcessLaunch() error = %v, want argv required error", err)
	}
}

func TestWrapPTYAllocationFailure(t *testing.T) {
	t.Parallel()

	if err := wrapPTYAllocationFailure(nil); err != nil {
		t.Fatalf("wrapPTYAllocationFailure(nil) = %v, want nil", err)
	}

	want := errors.New("openpty failed")
	err := wrapPTYAllocationFailure(want)
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("wrapPTYAllocationFailure() error = %v, want %v", err, ErrPTYAllocationFailed)
	}
}

func TestCheckAllocateContext_AllowsNilContext(t *testing.T) {
	t.Parallel()

	if err := checkAllocateContext(nil); err != nil {
		t.Fatalf("checkAllocateContext(nil) error = %v, want nil", err)
	}
}

func TestValidateArgv_RejectsEmptyExecutable(t *testing.T) {
	t.Parallel()

	if err := ValidateArgv([]string{"  "}); err == nil {
		t.Fatal("ValidateArgv() error = nil, want error")
	}
}

func TestRejectShellWrapper_AllowsNonShellPrograms(t *testing.T) {
	t.Parallel()

	if err := RejectShellWrapper([]string{"/bin/agy", "chat", "hello"}); err != nil {
		t.Fatalf("RejectShellWrapper() error = %v, want nil", err)
	}
}

func TestContainsTerminalEscapeOrControl_DetectsEscapeAndControlBytes(t *testing.T) {
	t.Parallel()

	if !ContainsTerminalEscapeOrControl("\x1b[31m") {
		t.Fatal("ContainsTerminalEscapeOrControl() = false, want true for ESC")
	}
	if !ContainsTerminalEscapeOrControl("ok\x07") {
		t.Fatal("ContainsTerminalEscapeOrControl() = false, want true for BEL")
	}
	if ContainsTerminalEscapeOrControl("plain\nanswer") {
		t.Fatal("ContainsTerminalEscapeOrControl() = true, want false for clean text")
	}
}

func TestMockAllocator_PropagatesAllocateError(t *testing.T) {
	t.Parallel()

	want := errors.New("allocate failed")
	allocator := &MockAllocator{Err: want}
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig())
	if !errors.Is(err, want) {
		t.Fatalf("Allocate() error = %v, want %v", err, want)
	}
}

func TestMockSession_PropagatesRunError(t *testing.T) {
	t.Parallel()

	want := errors.New("run failed")
	session := &MockSession{RunErr: want}
	_, err := session.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestSessionProcessNilGuards(t *testing.T) {
	t.Parallel()

	var proc *sessionProcess
	if err := proc.Terminate(); err != nil {
		t.Fatalf("Terminate(nil) error = %v, want nil", err)
	}
	proc.Close()
	if proc.PID() != 0 {
		t.Fatalf("PID(nil) = %d, want 0", proc.PID())
	}
}

func TestExitCodeFromWait(t *testing.T) {
	t.Parallel()

	if got := exitCodeFromWait(errors.New("wait failed"), nil); got != -1 {
		t.Fatalf("exitCodeFromWait(waitErr) = %d, want -1", got)
	}
	if got := exitCodeFromWait(nil, &sessionProcess{exitCode: 2}); got != 2 {
		t.Fatalf("exitCodeFromWait(proc.exitCode=2) = %d, want 2", got)
	}
	if got := exitCodeFromWait(nil, nil); got != 0 {
		t.Fatalf("exitCodeFromWait(nil,nil) = %d, want 0", got)
	}
}

func TestPlatformSessionRun_RejectsClosedSession(t *testing.T) {
	t.Parallel()

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{}, testPTYHost{}, testPTYClock)
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := session.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want closed-session error")
	}
}

func TestExecuteSessionRun_RejectsMissingInputs(t *testing.T) {
	t.Parallel()

	cfg := DefaultSessionConfig()
	proc := &sessionProcess{}

	if _, err := executeSessionRun(context.Background(), cfg, nil, proc, testPTYClock); err == nil {
		t.Fatal("executeSessionRun(nil reader) error = nil, want error")
	}

	reader := io.NopCloser(strings.NewReader(""))
	if _, err := executeSessionRun(context.Background(), cfg, reader, nil, testPTYClock); err == nil {
		t.Fatal("executeSessionRun(nil proc) error = nil, want error")
	}

	reader = io.NopCloser(strings.NewReader(""))
	if _, err := executeSessionRun(context.Background(), cfg, reader, proc, nil); !errors.Is(err, ErrClockRequired) {
		t.Fatalf("executeSessionRun(nil clock) error = %v, want %v", err, ErrClockRequired)
	}
}

func TestClosePTYReader_AllowsNil(t *testing.T) {
	t.Parallel()

	closePTYReader(nil)
}

func TestStartPTYCapture_ContinuesAfterReadDeadline(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		captured    []byte
		capacityHit bool
		lastByteAt  = time.Now()
	)
	reader := &timeoutThenDataReader{}

	readDone := startPTYCapture(reader, SessionConfig{MaxCaptureBytes: 1024}, &mu, &captured, &capacityHit, &lastByteAt, testPTYClock)
	<-readDone

	if got := string(captured); got != "delayed-output" {
		t.Fatalf("captured output = %q, want delayed-output", got)
	}
	if capacityHit {
		t.Fatal("capacityHit = true, want false")
	}
}

func TestSessionTiming_UsesInjectedDeterministicClockForActivityAndDeadlines(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	firstActivity := clock.Now()
	clock.SetTick(7)

	var (
		mu          sync.Mutex
		captured    []byte
		capacityHit bool
		lastByteAt  = firstActivity
	)
	readDone := startPTYCapture(
		io.NopCloser(strings.NewReader("ordered-output")),
		SessionConfig{MaxCaptureBytes: 1024},
		&mu,
		&captured,
		&capacityHit,
		&lastByteAt,
		clock,
	)
	<-readDone

	wantLastActivity := base.Add(7 * time.Second)
	if !lastByteAt.Equal(wantLastActivity) {
		t.Fatalf("last activity = %v, want injected clock time %v", lastByteAt, wantLastActivity)
	}
	if got := string(captured); got != "ordered-output" {
		t.Fatalf("captured output = %q, want deterministic read order", got)
	}

	clock.SetTick(9)
	now := clock.Now()
	hardDeadline := base.Add(30 * time.Second)
	if got, want := timeUntilTimeout(now, lastByteAt, hardDeadline, SessionConfig{IdleTimeout: 10 * time.Second}), 100*time.Millisecond; got != want {
		t.Fatalf("next timeout = %v, want bounded deterministic poll %v", got, want)
	}
	if sessionRunTimedOut(now, hardDeadline, SessionConfig{IdleTimeout: 10 * time.Second}, lastByteAt) {
		t.Fatal("session timed out before injected idle or hard deadline")
	}
	clock.SetTick(17)
	if !sessionRunTimedOut(clock.Now(), hardDeadline, SessionConfig{IdleTimeout: 10 * time.Second}, lastByteAt) {
		t.Fatal("session did not time out at injected idle deadline")
	}
}

func TestFinishSessionRun_DrainsBufferedOutputBeforeClosingReader(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		captured    []byte
		capacityHit bool
		lastByteAt  = time.Now()
	)
	reader := newDelayedDataReader()
	readDone := startPTYCapture(reader, SessionConfig{MaxCaptureBytes: 1024}, &mu, &captured, &capacityHit, &lastByteAt, testPTYClock)

	result, err := finishSessionRun(reader, readDone, &mu, &captured, &capacityHit, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("finishSessionRun() error = %v", err)
	}
	if got := result.CleanedText; got != "post-exit-output" {
		t.Fatalf("CleanedText = %q, want post-exit-output drained after process exit", got)
	}
}
