//go:build !windows && !linux && !darwin

package agypty

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestUnsupportedPTYAllocator_FailsClosed(t *testing.T) {
	t.Parallel()

	allocator := NewUnsupportedPTYAllocator()
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "hello"},
	}, DefaultSessionConfig())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Allocate() error = %v, want %v", err, ErrUnsupportedPlatform)
	}
}

func startBlockingTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	t.Skip("blocking process helper is unavailable on unsupported platforms")
	return &exec.Cmd{}
}
