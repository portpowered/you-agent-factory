//go:build linux || darwin

package agypty

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func containsPTYMarker(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "ptmx") || strings.Contains(lower, "pts") || strings.Contains(lower, "pty")
}

func TestPOSIXPTYAllocator_AllocateWithMockOpener(t *testing.T) {
	t.Parallel()

	var openCalls int
	allocator := &POSIXPTYAllocator{
		Open: func() (*os.File, *os.File, error) {
			openCalls++
			return os.Pipe()
		},
	}

	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "hello"},
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
	if platformSession.kind != PTYKindPOSIX {
		t.Fatalf("PTY kind = %v, want %v", platformSession.kind, PTYKindPOSIX)
	}
	if platformSession.cfg.MaxCaptureBytes != DefaultMaxCaptureBytes {
		t.Fatalf("MaxCaptureBytes = %d, want %d", platformSession.cfg.MaxCaptureBytes, DefaultMaxCaptureBytes)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestPOSIXPTYAllocator_AllocateOpensPTY(t *testing.T) {
	t.Parallel()

	allocator := NewPOSIXPTYAllocator()
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/true",
		Argv:       []string{"/bin/true"},
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
	if platformSession.kind != PTYKindPOSIX {
		t.Fatalf("PTY kind = %v, want %v", platformSession.kind, PTYKindPOSIX)
	}

	posix, ok := platformSession.pty.(*posixPTYAllocation)
	if !ok {
		t.Fatalf("allocation type = %T, want *posixPTYAllocation", platformSession.pty)
	}
	if posix.Master() == nil || posix.Slave() == nil {
		t.Fatal("expected non-nil POSIX master and slave handles")
	}
	if !containsPTYMarker(posix.Master().Name()) && !containsPTYMarker(posix.Slave().Name()) {
		t.Fatalf("PTY names = master %q slave %q, want ptmx/pts marker", posix.Master().Name(), posix.Slave().Name())
	}
}

func TestPOSIXPTYAllocator_PropagatesOpenFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("openpty failed")
	allocator := &POSIXPTYAllocator{
		Open: func() (*os.File, *os.File, error) {
			return nil, nil, want
		},
	}
	_, err := allocator.Allocate(context.Background(), ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy"},
	}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v, want %v", err, ErrPTYAllocationFailed)
	}
}
