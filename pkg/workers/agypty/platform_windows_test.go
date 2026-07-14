//go:build windows

package agypty

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWindowsConPTYAllocator_AllocateWithMockOpener(t *testing.T) {
	t.Parallel()

	var openCalls int
	allocator := &WindowsConPTYAllocator{
		Open: func() (*conPTYAllocation, error) {
			openCalls++
			inR, inW, err := os.Pipe()
			if err != nil {
				return nil, err
			}
			outR, outW, err := os.Pipe()
			if err != nil {
				_ = inR.Close()
				_ = inW.Close()
				return nil, err
			}
			_ = inR.Close()
			_ = outW.Close()
			return &conPTYAllocation{
				inPipe:  inW,
				outPipe: outR,
			}, nil
		},
	}

	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: `C:\agy\agy.exe`,
		Argv:       []string{`C:\agy\agy.exe`, "chat", "hello"},
	}, SessionConfig{})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if openCalls != 1 {
		t.Fatalf("openCalls = %d, want 1", openCalls)
	}

	platformSession, ok := session.(*platformSession)
	if !ok {
		t.Fatalf("session type = %T, want *platformSession", session)
	}
	if platformSession.kind != PTYKindConPTY {
		t.Fatalf("PTY kind = %v, want %v", platformSession.kind, PTYKindConPTY)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestWindowsConPTYAllocator_AllocateOpensConPTY(t *testing.T) {
	t.Parallel()

	allocator := NewWindowsConPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: `C:\agy\agy.exe`,
		Argv:       []string{`C:\agy\agy.exe`, "chat", "--headless", "hello"},
	}, DefaultSessionConfig())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	platformSession, ok := session.(*platformSession)
	if !ok {
		t.Fatalf("session type = %T, want *platformSession", session)
	}
	if platformSession.kind != PTYKindConPTY {
		t.Fatalf("PTY kind = %v, want %v", platformSession.kind, PTYKindConPTY)
	}

	conpty, ok := platformSession.pty.(*conPTYAllocation)
	if !ok {
		t.Fatalf("allocation type = %T, want *conPTYAllocation", platformSession.pty)
	}
	if conpty.Handle() == 0 {
		t.Fatal("ConPTY handle = 0, want non-zero pseudo-console handle")
	}
	if conpty.InputPipe() == nil || conpty.OutputPipe() == nil {
		t.Fatal("expected non-nil ConPTY host pipes")
	}
}

func TestWindowsConPTYAllocator_PropagatesOpenFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("CreatePseudoConsole failed")
	allocator := &WindowsConPTYAllocator{
		Open: func() (*conPTYAllocation, error) {
			return nil, want
		},
	}
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: `C:\agy\agy.exe`,
		Argv:       []string{`C:\agy\agy.exe`},
	}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v, want %v", err, ErrPTYAllocationFailed)
	}
}

func TestConPTYSessionRun_CompletesChildProcess(t *testing.T) {
	t.Parallel()

	executable := `C:\Windows\System32\ping.exe`
	if _, err := os.Stat(executable); err != nil {
		t.Skip("ping.exe is unavailable")
	}

	allocator := NewWindowsConPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: executable,
		Argv:       []string{executable, "-n", "1", "127.0.0.1"},
	}, DefaultSessionConfig())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TimedOut {
		t.Fatal("TimedOut = true, want false")
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestConPTYSessionRun_HardTimeoutTerminatesProcess(t *testing.T) {
	t.Parallel()

	executable := `C:\Windows\System32\ping.exe`
	if _, err := os.Stat(executable); err != nil {
		t.Skip("ping.exe is unavailable")
	}

	allocator := NewWindowsConPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: executable,
		Argv:       []string{executable, "-n", "120", "127.0.0.1"},
	}, SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     time.Hour,
		HardTimeout:     500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
}

func TestConPTYSessionRun_ClosesPTYAfterRun(t *testing.T) {
	t.Parallel()

	executable := `C:\Windows\System32\ping.exe`
	if _, err := os.Stat(executable); err != nil {
		t.Skip("ping.exe is unavailable")
	}

	allocator := NewWindowsConPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: executable,
		Argv:       []string{executable, "-n", "1", "127.0.0.1"},
	}, DefaultSessionConfig())
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	if _, err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() after Run error = %v", err)
	}
}

func startBlockingTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	executable := `C:\Windows\System32\ping.exe`
	if _, err := os.Stat(executable); err != nil {
		t.Skip("ping.exe is unavailable")
	}
	cmd := exec.Command(executable, "-n", "120", "127.0.0.1")
	processConfigureForTest(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return cmd
}
