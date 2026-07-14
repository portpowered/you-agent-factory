//go:build windows

package agypty

import (
	"context"
	"errors"
	"os"
	"testing"
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
