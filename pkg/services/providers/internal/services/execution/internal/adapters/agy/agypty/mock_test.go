package agypty_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
)

func TestMockAllocatorRecordsLaunchAndCapture(t *testing.T) {
	t.Parallel()

	allocator := &agypty.MockAllocator{}
	launch := agypty.ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "--headless", "hello"},
		WorkDir:    "/factory/workspaces/a",
		Env:        []string{"GIT_TERMINAL_PROMPT=0"},
	}
	cfg := agypty.DefaultSessionConfig()

	session, err := allocator.Allocate(context.Background(), launch, cfg)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	mockSession, ok := session.(*agypty.MockSession)
	if !ok {
		t.Fatalf("session type = %T, want *agypty.MockSession", session)
	}
	mockSession.Result = agypty.SessionResult{
		ExitCode: 0,
		RawBytes: []byte("answer\r\n\x1b[2K"),
	}

	result, err := session.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.CleanedText != "answer" {
		t.Fatalf("CleanedText = %q, want %q", result.CleanedText, "answer")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !mockSession.Closed {
		t.Fatal("MockSession.Closed = false, want true")
	}
	if len(allocator.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(allocator.Sessions))
	}
	if allocator.Sessions[0].Launch.Executable != launch.Executable {
		t.Fatalf("recorded executable = %q, want %q", allocator.Sessions[0].Launch.Executable, launch.Executable)
	}
	if allocator.Sessions[0].Config.MaxCaptureBytes != cfg.MaxCaptureBytes {
		t.Fatalf("recorded MaxCaptureBytes = %d, want %d", allocator.Sessions[0].Config.MaxCaptureBytes, cfg.MaxCaptureBytes)
	}
}
