//go:build !windows && !linux && !darwin

package agypty

import (
	"context"
	"errors"
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
