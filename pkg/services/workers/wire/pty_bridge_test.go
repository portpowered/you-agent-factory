package wire

import (
	"context"
	"testing"
	"time"

	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
)

func TestAdaptPTYAllocatorProjectsForeignOwnerShape(t *testing.T) {
	t.Parallel()

	candidate := &foreignPTYAllocator{
		session: &foreignPTYSession{
			result: foreignPTYSessionResult{
				ExitCode:    7,
				RawBytes:    []byte("raw"),
				CleanedText: "clean",
				TimedOut:    true,
				CapacityHit: true,
			},
		},
	}
	allocator := adaptPTYAllocator(candidate)
	if allocator == nil {
		t.Fatal("adaptPTYAllocator() = nil, want structural adapter")
	}

	session, err := allocator.Allocate(nil, workersinternal.PTYProcessLaunch{
		Executable: "agy",
		Argv:       []string{"--headless"},
		WorkDir:    "factory",
		Env:        []string{"MODE=test"},
	}, workersinternal.PTYSessionConfig{
		MaxCaptureBytes: 12,
		IdleTimeout:     time.Second,
		HardTimeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if candidate.launch.Executable != "agy" || candidate.launch.WorkDir != "factory" ||
		len(candidate.launch.Argv) != 1 || candidate.launch.Argv[0] != "--headless" ||
		len(candidate.launch.Env) != 1 || candidate.launch.Env[0] != "MODE=test" {
		t.Fatalf("foreign launch = %#v", candidate.launch)
	}
	if candidate.config.MaxCaptureBytes != 12 || candidate.config.IdleTimeout != time.Second ||
		candidate.config.HardTimeout != 2*time.Second {
		t.Fatalf("foreign config = %#v", candidate.config)
	}

	result, err := session.Run(nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 7 || string(result.RawBytes) != "raw" || result.CleanedText != "clean" ||
		!result.TimedOut || !result.CapacityHit {
		t.Fatalf("session result = %#v", result)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !candidate.session.closed {
		t.Fatal("foreign session was not closed")
	}
}

type foreignPTYAllocator struct {
	launch  foreignPTYProcessLaunch
	config  foreignPTYSessionConfig
	session *foreignPTYSession
}

func (allocator *foreignPTYAllocator) Allocate(
	_ context.Context,
	launch foreignPTYProcessLaunch,
	config foreignPTYSessionConfig,
) (*foreignPTYSession, error) {
	allocator.launch = launch
	allocator.config = config
	return allocator.session, nil
}

type foreignPTYProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

type foreignPTYSessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

type foreignPTYSession struct {
	result foreignPTYSessionResult
	closed bool
}

func (session *foreignPTYSession) Run(context.Context) (foreignPTYSessionResult, error) {
	return session.result, nil
}

func (session *foreignPTYSession) Close() error {
	session.closed = true
	return nil
}

type foreignPTYSessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}
