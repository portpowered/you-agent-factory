package agy_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
)

func TestPTYEffectClassifiesDistinctExecutionOutcomes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		allocator   agypty.PTYAllocator
		setup       func(t *testing.T) (executable string, deps agy.ExecutableDependencies)
		ctx         context.Context
		wantKind    providers.ExecuteFailureKind
		wantMessage string
	}{
		{
			name:      "missing executable",
			allocator: &stubAllocator{result: agypty.SessionResult{}},
			setup: func(t *testing.T) (string, agy.ExecutableDependencies) {
				factoryRoot := t.TempDir()
				missing := filepath.Join(factoryRoot, "missing-agy")
				return missing, executableDependencies(nil)
			},
			wantKind:    providers.ExecuteFailureKindDependency,
			wantMessage: "Agy executable could not be found.",
		},
		{
			name:        "pty allocation failure",
			allocator:   &errorAllocator{err: fmt.Errorf("allocate: %w", agypty.ErrPTYAllocationFailed)},
			wantKind:    providers.ExecuteFailureKindDependency,
			wantMessage: "Agy PTY allocation failed.",
		},
		{
			name:        "unsupported platform",
			allocator:   &errorAllocator{err: agypty.ErrUnsupportedPlatform},
			wantKind:    providers.ExecuteFailureKindDependency,
			wantMessage: "Agy PTY allocation is not supported on this platform.",
		},
		{
			name: "auth failure from cleaned output",
			allocator: &failureStubAllocator{result: agypty.SessionResult{
				ExitCode: 1, CleanedText: "Error: authentication failed: invalid api key",
			}, runErr: fmt.Errorf("%w: exit code 1", agypty.ErrNonzeroExit)},
			wantKind:    providers.ExecuteFailureKindAuthentication,
			wantMessage: "Agy authentication failed.",
		},
		{
			name: "idle or hard timeout",
			allocator: &failureStubAllocator{result: agypty.SessionResult{
				ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
			}, runErr: agypty.ErrSessionTimedOut},
			wantKind:    providers.ExecuteFailureKindTimeout,
			wantMessage: agy.TimeoutFailureMessage,
		},
		{
			name:      "context deadline exceeded",
			allocator: &blockingAllocator{},
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 0)
				cancel()
				return ctx
			}(),
			wantKind:    providers.ExecuteFailureKindTimeout,
			wantMessage: agy.TimeoutFailureMessage,
		},
		{
			name: "nonzero exit without auth signals",
			allocator: &failureStubAllocator{result: agypty.SessionResult{
				ExitCode: 2, CleanedText: "provider crashed unexpectedly",
			}, runErr: fmt.Errorf("%w: exit code 2", agypty.ErrNonzeroExit)},
			wantKind:    providers.ExecuteFailureKindUnknown,
			wantMessage: "Agy execution exited with code 2.",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			executable := "agy"
			deps := executableDependencies(nil)
			if tc.setup != nil {
				executable, deps = tc.setup(t)
			}
			factoryRoot := t.TempDir()
			effect := agy.NewPTYEffect(agy.PTYEffectOptions{
				FactoryRoot:            factoryRoot,
				Allocator:              tc.allocator,
				Executable:             executable,
				ExecutableDependencies: deps,
			})
			_, err := effect.Execute(ctx, providers.ExecuteRequest{
				Provider:    providers.IDAgy,
				AttemptID:   "attempt-failure",
				UserMessage: "deterministic failure prompt",
			}, func([]byte) error { return nil })
			assertExecuteFailure(t, err, tc.wantKind, tc.wantMessage)
		})
	}
}

func TestPTYEffectDeadlineTimeoutOutranksOutputDetail(t *testing.T) {
	t.Parallel()

	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: t.TempDir(),
		Allocator: &failureStubAllocator{
			result: agypty.SessionResult{
				ExitCode:    1,
				CleanedText: "token=customer-secret-value",
			},
			runErr: context.DeadlineExceeded,
		},
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "attempt-timeout",
		UserMessage: "deterministic failure prompt",
	}, func([]byte) error { return nil })
	assertExecuteFailure(t, err, providers.ExecuteFailureKindTimeout, agy.TimeoutFailureMessage)
	if strings.Contains(err.Error(), "token=") {
		t.Fatalf("failure leaked sensitive facts: %v", err)
	}
}

func TestPTYEffectMissingExecutableViaExecNotFound(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	missingExecutable := filepath.Join(factoryRoot, "missing-agy")
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "ok"}},
		Executable:             missingExecutable,
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "attempt-missing",
		UserMessage:      "hello",
		WorkingDirectory: ".",
	}, func([]byte) error { return nil })
	assertExecuteFailure(t, err, providers.ExecuteFailureKindDependency, "Agy executable could not be found.")
}

func TestPTYEffectTimeoutDoesNotTreatPartialOutputAsSuccess(t *testing.T) {
	t.Parallel()

	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: t.TempDir(),
		Allocator: &failureStubAllocator{result: agypty.SessionResult{
			ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
		}, runErr: agypty.ErrSessionTimedOut},
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-timeout",
		UserMessage: "plan the goal",
	}, func(chunk []byte) error {
		t.Fatalf("observe() called with %q, want no terminal success on timeout", string(chunk))
		return nil
	})
	assertExecuteFailure(t, err, providers.ExecuteFailureKindTimeout, agy.TimeoutFailureMessage)
}

type failureStubSession struct {
	result agypty.SessionResult
	runErr error
}

func (s *failureStubSession) Run(context.Context) (agypty.SessionResult, error) {
	if s.runErr != nil {
		return s.result, s.runErr
	}
	if s.result.ExitCode != 0 || s.result.TimedOut {
		if s.result.TimedOut {
			return s.result, fmt.Errorf("%w", agypty.ErrSessionTimedOut)
		}
		return s.result, fmt.Errorf("%w: exit code %d", agypty.ErrNonzeroExit, s.result.ExitCode)
	}
	return s.result, nil
}

func (s *failureStubSession) Close() error { return nil }

type failureStubAllocator struct {
	result agypty.SessionResult
	runErr error
}

func (a *failureStubAllocator) Allocate(_ context.Context, _ agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	return &failureStubSession{result: a.result, runErr: a.runErr}, nil
}

type errorAllocator struct {
	err error
}

func (a *errorAllocator) Allocate(context.Context, agypty.ProcessLaunch, agypty.SessionConfig) (agypty.PTYSession, error) {
	return nil, a.err
}

type blockingAllocator struct{}

func (a *blockingAllocator) Allocate(ctx context.Context, _ agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	return &blockingSession{}, nil
}

type blockingSession struct{}

func (s *blockingSession) Run(ctx context.Context) (agypty.SessionResult, error) {
	<-ctx.Done()
	return agypty.SessionResult{}, ctx.Err()
}

func (s *blockingSession) Close() error { return nil }

func assertExecuteFailure(
	t *testing.T,
	err error,
	wantKind providers.ExecuteFailureKind,
	wantMessage string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("Execute() error = nil, want failure")
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Kind != wantKind {
		t.Fatalf("failure kind = %q, want %q", failure.Kind, wantKind)
	}
	if failure.Message != wantMessage {
		t.Fatalf("failure message = %q, want %q", failure.Message, wantMessage)
	}
}

func TestPTYEffectClassifiesExecErrNotFoundAsMissingExecutable(t *testing.T) {
	t.Parallel()

	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: t.TempDir(),
		Allocator:   &errorAllocator{err: fmt.Errorf("start child: %w", exec.ErrNotFound)},
		Executable:  "agy",
		ExecutableDependencies: executableDependencies(
			map[string]string{"agy": "/missing/agy"},
			"/missing/agy",
		),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "attempt-exec-not-found",
		UserMessage:      "hello",
		WorkingDirectory: ".",
	}, func([]byte) error { return nil })
	assertExecuteFailure(t, err, providers.ExecuteFailureKindDependency, "Agy executable could not be found.")
}
