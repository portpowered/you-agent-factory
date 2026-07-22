package agy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

func TestPTYRunnerConstructionRejectsMissingAllocator(t *testing.T) {
	t.Parallel()

	if _, err := agy.NewPTYRunner(nil, agypty.SessionConfig{}); err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("NewPTYRunner(nil) error = %v, want required allocator", err)
	}
}

func TestPTYRunnerTimeoutCleansCaptureBeforeResult(t *testing.T) {
	t.Parallel()

	raw := []byte("spinning\rpartial answer\x1b[2K\n")
	mock := &timeoutCleaningAllocator{result: agypty.SessionResult{
		ExitCode: 124,
		TimedOut: true,
		RawBytes: raw,
		// Deliberately omit CleanedText so the runner must clean from raw bytes.
	}}
	providerAdapter := agy.NewAdapterWithDependencies(
		t.TempDir(), mock, "agy", agypty.SessionConfig{}, executableDependencies(nil),
	)
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}

	result, runErr := runner.Run(context.Background(), workerprocess.CommandRequest{
		Command: "agy",
		Args:    []string{"agy", "chat", "--headless", "hello"},
	}, func(adapter.Observation) error {
		t.Fatal("observe() called on timeout, want no public stream emit")
		return nil
	})
	if runErr == nil {
		t.Fatal("Run() error = nil, want timeout failure")
	}
	if string(result.Stdout) != "partial answer" {
		t.Fatalf("stdout = %q, want cleaned timeout capture", string(result.Stdout))
	}
	if agypty.ContainsTerminalEscapeOrControl(string(result.Stdout)) {
		t.Fatalf("stdout = %q still contains terminal escape or control bytes", string(result.Stdout))
	}
	if result.ExitCode != 124 {
		t.Fatalf("exit code = %d, want 124", result.ExitCode)
	}
}

type timeoutCleaningSession struct {
	result agypty.SessionResult
}

func (s *timeoutCleaningSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *timeoutCleaningSession) Close() error { return nil }

type timeoutCleaningAllocator struct {
	result agypty.SessionResult
}

func (a *timeoutCleaningAllocator) Allocate(_ context.Context, _ agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	return &timeoutCleaningSession{result: a.result}, nil
}
