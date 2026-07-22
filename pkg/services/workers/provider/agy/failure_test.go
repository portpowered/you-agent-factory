package agy_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

func TestAdapterClassifyFailureMapsDistinctExecutionOutcomes(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	cases := []struct {
		name        string
		input       adapter.FailureContext
		wantType    workerexecution.WorkFailureType
		wantMessage string
		retryable   bool
	}{
		{
			name: "missing executable",
			input: adapter.FailureContext{
				CommandError: fmt.Errorf("%w: /missing/agy", agy.ErrMissingExecutable),
			},
			wantType:    workerexecution.WorkFailureTypeMissingExecutable,
			wantMessage: "Agy executable could not be found.",
		},
		{
			name: "missing executable via exec.ErrNotFound",
			input: adapter.FailureContext{
				CommandError: fmt.Errorf("start child: %w", exec.ErrNotFound),
			},
			wantType:    workerexecution.WorkFailureTypeMissingExecutable,
			wantMessage: "Agy executable could not be found.",
		},
		{
			name: "pty allocation failure",
			input: adapter.FailureContext{
				CommandError: fmt.Errorf("allocate: %w", agypty.ErrPTYAllocationFailed),
			},
			wantType:    workerexecution.WorkFailureTypeMisconfigured,
			wantMessage: "Agy PTY allocation failed.",
		},
		{
			name: "unsupported platform",
			input: adapter.FailureContext{
				CommandError: agypty.ErrUnsupportedPlatform,
			},
			wantType:    workerexecution.WorkFailureTypeMisconfigured,
			wantMessage: "Agy PTY allocation is not supported on this platform.",
		},
		{
			name: "auth failure from cleaned output",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{
					ExitCode: 1,
					Stdout:   []byte("Error: authentication failed: invalid api key"),
				},
				CommandError: fmt.Errorf("%w: exit code 1", agypty.ErrNonzeroExit),
			},
			wantType:    workerexecution.WorkFailureTypeAuthFailure,
			wantMessage: "Agy authentication failed.",
		},
		{
			name: "idle or hard timeout",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 124, Stdout: []byte("partial spinner output")},
				CommandError:  agypty.ErrSessionTimedOut,
			},
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantMessage: "Agy request timed out.",
			retryable:   true,
		},
		{
			name: "context deadline exceeded",
			input: adapter.FailureContext{
				CommandError: context.DeadlineExceeded,
			},
			wantType:    workerexecution.WorkFailureTypeTimeout,
			wantMessage: "Agy request was canceled or timed out.",
			retryable:   true,
		},
		{
			name: "nonzero exit without auth signals",
			input: adapter.FailureContext{
				CommandResult: workerprocess.CommandResult{
					ExitCode: 2,
					Stdout:   []byte("provider crashed unexpectedly"),
				},
				CommandError: fmt.Errorf("%w: exit code 2", agypty.ErrNonzeroExit),
			},
			wantType:    workerexecution.WorkFailureTypeUnknown,
			wantMessage: "Agy execution exited with code 2.",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failure := providerAdapter.ClassifyFailure(context.Background(), tc.input)
			assertFailureFacts(t, failure, tc.wantType, tc.wantMessage, tc.retryable)
		})
	}
}

func TestAdapterClassifyFailureTimeoutDoesNotTreatPartialOutputAsSuccess(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &failureStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	providerAdapter := agy.NewAdapterWithDependencies(
		factoryRoot, mock, "agy", agypty.SessionConfig{}, executableDependencies(nil),
	)
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}
	result, executeErr := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-timeout"},
			WorkingDirectory: ".",
			UserMessage:      "plan the goal",
		}},
		Decoder: adapter.DecoderContext{RunID: "run-agy-timeout", DispatchID: "dispatch-agy-timeout"},
	})
	if executeErr == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	if result.Response.Content != "" {
		t.Fatalf("response content = %q, want no successful final output on timeout", result.Response.Content)
	}
	if result.Failure == nil {
		t.Fatal("failure = nil, want classified timeout failure")
	}
	assertFailureFacts(t, adapter.FailureResult{Failure: result.Failure},
		workerexecution.WorkFailureTypeTimeout, "Agy request timed out.", true)
}

func TestAdapterBuildCommandMissingExecutableClassifiesDistinctly(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	missingExecutable := filepath.Join(factoryRoot, "missing-agy")
	providerAdapter := agy.NewAdapterWithExecutable(
		factoryRoot, missingExecutable, executableDependencies(nil),
	)
	_, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-missing"},
			WorkingDirectory: ".",
			UserMessage:      "hello",
		},
	})
	if err == nil {
		t.Fatal("BuildCommand() error = nil, want missing executable failure")
	}
	if !errors.Is(err, agy.ErrMissingExecutable) {
		t.Fatalf("BuildCommand() error = %v, want %v", err, agy.ErrMissingExecutable)
	}
	failure := agy.ClassifyOrchestrationError(err)
	assertFailureFacts(t, failure, workerexecution.WorkFailureTypeMissingExecutable, "Agy executable could not be found.", false)
}

type failureStubSession struct {
	result agypty.SessionResult
}

func (s *failureStubSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *failureStubSession) Close() error { return nil }

type failureStubAllocator struct {
	result agypty.SessionResult
}

func (a *failureStubAllocator) Allocate(_ context.Context, _ agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	return &failureStubSession{result: a.result}, nil
}

func assertFailureFacts(
	t *testing.T,
	result adapter.FailureResult,
	wantType workerexecution.WorkFailureType,
	wantMessage string,
	retryable bool,
) {
	t.Helper()
	if result.Failure == nil {
		t.Fatalf("failure = nil, want type %q", wantType)
	}
	if result.Failure.Type != wantType {
		t.Fatalf("failure type = %q, want %q", result.Failure.Type, wantType)
	}
	if result.Failure.Message != wantMessage {
		t.Fatalf("failure message = %q, want %q", result.Failure.Message, wantMessage)
	}
	if result.Failure.Retry.Retryable != retryable {
		t.Fatalf("retryable = %v, want %v", result.Failure.Retry.Retryable, retryable)
	}
}
