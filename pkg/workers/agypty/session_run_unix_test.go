//go:build linux || darwin

package agypty

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPOSIXPTYSessionRun_CapturesCleanedOutput(t *testing.T) {
	t.Parallel()

	allocator := NewPOSIXPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/echo",
		Argv:       []string{"/bin/echo", "hello-pty"},
	}, SessionConfig{
		MaxCaptureBytes: 4096,
		IdleTimeout:     5 * time.Second,
		HardTimeout:     10 * time.Second,
	})
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
	if !strings.Contains(result.CleanedText, "hello-pty") {
		t.Fatalf("CleanedText = %q, want hello-pty output", result.CleanedText)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestPOSIXPTYSessionRun_IdleTimeoutTerminatesProcess(t *testing.T) {
	t.Parallel()

	allocator := NewPOSIXPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/sleep",
		Argv:       []string{"/bin/sleep", "120"},
	}, SessionConfig{
		MaxCaptureBytes: DefaultMaxCaptureBytes,
		IdleTimeout:     300 * time.Millisecond,
		HardTimeout:     2 * time.Minute,
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

func TestPOSIXPTYSessionRun_ClosesPTYAfterRun(t *testing.T) {
	t.Parallel()

	allocator := NewPOSIXPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/echo",
		Argv:       []string{"/bin/echo", "done"},
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
