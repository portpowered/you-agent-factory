package agypty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/workers/process"
)

type closeOnlyPTY struct{}

func (closeOnlyPTY) Close() error { return nil }

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
