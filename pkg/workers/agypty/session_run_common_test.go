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

	"github.com/portpowered/infinite-you/pkg/workers/process"
)

type closeOnlyPTY struct{}

func (closeOnlyPTY) Close() error { return nil }

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

	allocator := newPlatformPTYAllocator()
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

	allocator := newPlatformPTYAllocator()
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
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{})
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

func TestDefaultPlatformAllocatorFactory_ReturnsPlatformAllocator(t *testing.T) {
	t.Parallel()

	factory := NewDefaultPlatformAllocatorFactory()
	allocator, err := factory.NewAllocator()
	if err != nil {
		t.Fatalf("NewAllocator() error = %v", err)
	}
	if allocator == nil {
		t.Fatal("NewAllocator() returned nil allocator")
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
		IdleTimeout:     2 * time.Second,
		HardTimeout:     5 * time.Second,
	}
	result, err := executeSessionRun(context.Background(), cfg, reader, proc)
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
		HardTimeout:     200 * time.Millisecond,
	}
	result, err := executeSessionRun(context.Background(), cfg, reader, proc)
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
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	cfg := SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     time.Hour,
		HardTimeout:     time.Hour,
	}
	_, err = executeSessionRun(ctx, cfg, reader, proc)
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
	}, DefaultSessionConfig(), PTYKindConPTY, closeOnlyPTY{})
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
	}, DefaultSessionConfig(), PTYKindPOSIX, nil)
	if err == nil {
		t.Fatal("newPlatformSession() error = nil, want error")
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
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{})
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

	if _, err := executeSessionRun(context.Background(), cfg, nil, proc); err == nil {
		t.Fatal("executeSessionRun(nil reader) error = nil, want error")
	}

	reader := io.NopCloser(strings.NewReader(""))
	if _, err := executeSessionRun(context.Background(), cfg, reader, nil); err == nil {
		t.Fatal("executeSessionRun(nil proc) error = nil, want error")
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

	readDone := startPTYCapture(reader, SessionConfig{MaxCaptureBytes: 1024}, &mu, &captured, &capacityHit, &lastByteAt)
	<-readDone

	if got := string(captured); got != "delayed-output" {
		t.Fatalf("captured output = %q, want delayed-output", got)
	}
	if capacityHit {
		t.Fatal("capacityHit = true, want false")
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
	readDone := startPTYCapture(reader, SessionConfig{MaxCaptureBytes: 1024}, &mu, &captured, &capacityHit, &lastByteAt)

	result, err := finishSessionRun(reader, readDone, &mu, &captured, &capacityHit, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("finishSessionRun() error = %v", err)
	}
	if got := result.CleanedText; got != "post-exit-output" {
		t.Fatalf("CleanedText = %q, want post-exit-output drained after process exit", got)
	}
}
