package agy_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestAgyNewRegistrationBindsCanonicalIdentity(t *testing.T) {
	t.Parallel()

	registration := agy.NewRegistration(nil)
	if registration.Provider != providers.IDAgy {
		t.Fatalf("Provider = %q, want %q", registration.Provider, providers.IDAgy)
	}
	if registration.Attempt == nil {
		t.Fatal("Attempt = nil, want unavailable attempt")
	}
}

func TestAgyRootFailsClosedWhenEffectAbsent(t *testing.T) {
	t.Parallel()

	root := newAgyRoot(t, nil)
	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAgy,
			AttemptID: "attempt-agy-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyBuiltInRegistrationFailsClosedWithoutEffect(t *testing.T) {
	t.Parallel()

	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewBuiltInService(catalog)
	if err != nil {
		t.Fatalf("NewBuiltInService() = %v", err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatalf("providerservice.New() = %v", err)
	}

	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAgy,
			AttemptID: "attempt-agy-built-in-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyRootPTYExecutionEndToEnd(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Hello from Agy"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy-e2e",
		WorkingDirectory: ".",
		UserMessage:      privatePrompt,
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-e2e",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "Hello from Agy" {
		t.Fatalf("Content = %q, want cleaned final text", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-e2e" {
		t.Fatalf("SessionRef = %#v, want resumed session", result.SessionRef)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis < 0 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func TestAgyRootRejectsUnusableFinalOutput(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: ""}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-empty",
		UserMessage: "hello",
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want final parse failure")
	}
	if result.Content != "" {
		t.Fatalf("result = %#v, want empty content on failure", result)
	}
}

func TestAgyRootPreservesRequestAndFinalStdout(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "attempt-agy-success",
		Model:            "agy-model",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
	}
	const content = "agy final answer"
	var received providers.ExecuteRequest
	effect := agy.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		received = got.Clone()
		if err := observe([]byte(content)); err != nil {
			return agy.EffectResult{}, err
		}
		return agy.EffectResult{DurationMillis: 23}, nil
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(received, request) {
		t.Fatalf("native request = %#v, want %#v", received, request)
	}
	if result.Content != content {
		t.Fatalf("Content = %q, want %q", result.Content, content)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis != 23 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func TestAgyRootCancellationAndDeadlineReachEffectAndCleanUpOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		want       error
	}{
		{
			name: "cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
			want: providers.ErrExecuteCancelled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(t.Context(), 50*time.Millisecond)
			},
			want: providers.ErrExecuteTimeout,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			started := make(chan struct{})
			var cleanups atomic.Int32
			effect := agy.EffectFunc(func(
				ctx context.Context,
				_ providers.ExecuteRequest,
				_ func([]byte) error,
			) (agy.EffectResult, error) {
				close(started)
				defer cleanups.Add(1)
				<-ctx.Done()
				return agy.EffectResult{}, ctx.Err()
			})
			ctx, cancel := test.newContext()
			defer cancel()
			root := newAgyRoot(t, effect)
			outcome := make(chan error, 1)
			go func() {
				_, err := root.Execute(ctx, agyFailureRequest())
				outcome <- err
			}()
			<-started
			if test.want == providers.ErrExecuteCancelled {
				cancel()
			}

			select {
			case err := <-outcome:
				if !errors.Is(err, test.want) {
					t.Fatalf("Execute() error = %v, want %v", err, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("Execute() did not stop after context ended")
			}
			if got := cleanups.Load(); got != 1 {
				t.Fatalf("cleanup calls = %d, want 1", got)
			}
		})
	}
}

func TestAgyRootTimeoutPreservesResumeSessionOnFailure(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: factoryRoot,
		Allocator: &failureStubAllocator{result: agypty.SessionResult{
			ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
		}, runErr: agypty.ErrSessionTimedOut},
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-timeout-session",
		UserMessage: "plan the goal",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-on-failure",
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	if result.Content != "" {
		t.Fatalf("result content = %q, want empty result on timeout failure", result.Content)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindTimeout {
		t.Fatalf("failure kind = %q, want timeout", failure.Kind)
	}
	if failure.Diagnostics == nil || len(failure.Diagnostics.Progress) == 0 {
		t.Fatalf("failure diagnostics = %#v, want partial timeout progress", failure.Diagnostics)
	}
	if got := failure.Diagnostics.Progress[0].Detail; got != "partial answer before timeout" {
		t.Fatalf("partial timeout detail = %q, want partial answer before timeout", got)
	}
	if failure.SessionRef == nil || failure.SessionRef.ID != "session-on-failure" {
		t.Fatalf("failure SessionRef = %#v, want resumed session", failure.SessionRef)
	}
}

func TestAgyRootMissingExecutablePreservesResumeSessionOnFailure(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	missingExecutable := filepath.Join(factoryRoot, "missing-agy")
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              &stubAllocator{},
		Executable:             missingExecutable,
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	_, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-missing-session",
		UserMessage: "hello",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-on-setup-failure",
		},
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("failure kind = %q, want dependency", failure.Kind)
	}
	if failure.SessionRef == nil || failure.SessionRef.ID != "session-on-setup-failure" {
		t.Fatalf("failure SessionRef = %#v, want resumed session", failure.SessionRef)
	}
}

func agyFailureRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "attempt-agy-failure",
		UserMessage: "deterministic failure prompt",
	}
}

func assertAgyDependencyFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
) {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("Execute() error = %#v, want dependency failure", err)
	}
	if !strings.Contains(failure.Message, "Agy") {
		t.Fatalf("failure message = %q, want Agy unavailable message", failure.Message)
	}
}

func newAgyRoot(t *testing.T, effect agy.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		agy.NewRegistration(effect),
	)
	if err != nil {
		t.Fatal(err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
