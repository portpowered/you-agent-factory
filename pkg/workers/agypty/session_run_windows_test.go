//go:build windows

package agypty

import (
	"context"
	"os"
	"testing"
	"time"
)

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
