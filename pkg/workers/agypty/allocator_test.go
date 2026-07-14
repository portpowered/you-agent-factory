package agypty

import (
	"context"
	"errors"
	"testing"
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

func TestPlatformSession_RunPendingAndClose(t *testing.T) {
	t.Parallel()

	session, err := newPlatformSession(ProcessLaunch{
		Executable: "/bin/agy",
		Argv:       []string{"/bin/agy", "chat", "hello"},
	}, DefaultSessionConfig(), PTYKindPOSIX, closeOnlyPTY{})
	if err != nil {
		t.Fatalf("newPlatformSession() error = %v", err)
	}

	_, runErr := session.Run(context.Background())
	if !errors.Is(runErr, errSessionRunPending) {
		t.Fatalf("Run() error = %v, want %v", runErr, errSessionRunPending)
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
