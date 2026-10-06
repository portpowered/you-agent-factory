package agy_test

import (
	"context"
	"errors"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"

	"go.uber.org/goleak"
)

func TestAgyNewRegistrationBindsCanonicalIdentity(t *testing.T) {
	t.Parallel()

	registration := agy.NewRegistration(disabledAgyEffect())
	if registration.Provider != providers.IDAntigravity {
		t.Fatalf("Provider = %q, want %q", registration.Provider, providers.IDAntigravity)
	}
	if registration.Attempt == nil {
		t.Fatal("Attempt = nil, want completed attempt")
	}
}

func TestAgyRootPreservesDisabledEffectFailure(t *testing.T) {
	t.Parallel()

	root := newAgyRoot(t, disabledAgyEffect())
	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAntigravity,
			AttemptID: "attempt-agy-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyBuiltInRegistrationPreservesDisabledEffectFailure(t *testing.T) {
	t.Parallel()

	catalog, err := catalogwire.NewService(catalogwire.IdentityProbe, nil, nil)
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	rejectPeer := func() error {
		t.Fatal("disabled AGY attempt invoked a peer effect")
		return nil
	}
	registrations := executionwire.BuiltInRegistrations(disabledAgyEffect(),
		codex.EffectFunc(func(context.Context, execution.ContinuationRequest, func([]byte) error) (codex.EffectResult, error) {
			return codex.EffectResult{}, rejectPeer()
		}),
		claude.EffectFunc(func(context.Context, execution.ContinuationRequest, func([]byte) error) (claude.EffectResult, error) {
			return claude.EffectResult{}, rejectPeer()
		}))
	executionService, err := executionwire.NewService(catalog, registrations...)
	if err != nil {
		t.Fatalf("NewBuiltInService() = %v", err)
	}
	root, err := providerservice.NewWithACP(catalog, executionService, disabledACP{}, nil, logging.NoopLogger{}, disabledACP{})
	if err != nil {
		t.Fatalf("providerservice.NewWithACP() = %v", err)
	}

	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAntigravity,
			AttemptID: "attempt-agy-built-in-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyRootPTYExecutionEndToEnd(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Hello from Agy"}}
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	root := newAgyRoot(t, effect)

	continued, err := root.Continue(t.Context(), providers.ContinueRequest{
		Reference: providers.SessionRef{
			Provider: providers.IDAntigravity,
			Kind:     providers.SessionIDKind,
			ID:       "session-e2e",
		},
		Attempt: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "dispatch-agy-e2e",
			WorkingDirectory: ".",
			UserMessage:      privatePrompt,
		},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if continued.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue().Outcome = %q, want resumed", continued.Outcome)
	}
	result := continued.Result
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDAntigravity,
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
		Provider:         providers.IDAntigravity,
		AttemptID:        "attempt-agy-success",
		Model:            "agy-model",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
	}
	const content = "agy final answer"
	var received providers.ExecuteRequest
	effect := agy.EffectFunc(func(
		_ context.Context,
		got execution.ContinuationRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		received = got.ExecuteRequest.Clone()
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
	if received.OwnedProcessObserver == nil {
		t.Fatal("native dispatch lost its Provider-owned process observer")
	}
	received.OwnedProcessObserver = nil
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
				_ execution.ContinuationRequest,
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
	effect := agy.NewPTYEffect(&failureStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}, runErr: agypty.ErrSessionTimedOut}, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	root := newAgyRoot(t, effect)

	continued, err := root.Continue(t.Context(), providers.ContinueRequest{
		Reference: providers.SessionRef{
			Provider: providers.IDAntigravity,
			Kind:     providers.SessionIDKind,
			ID:       "session-on-failure",
		},
		Attempt: providers.ExecuteRequest{
			Provider:    providers.IDAntigravity,
			AttemptID:   "dispatch-agy-timeout-session",
			UserMessage: "plan the goal",
		},
	})
	if err == nil {
		t.Fatal("Continue() error = nil, want timeout failure")
	}
	result := continued.Result
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
	effect := agy.NewPTYEffect(&stubAllocator{}, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  missingExecutable,
	})
	root := newAgyRoot(t, effect)

	_, err := root.Continue(t.Context(), providers.ContinueRequest{
		Reference: providers.SessionRef{
			Provider: providers.IDAntigravity,
			Kind:     providers.SessionIDKind,
			ID:       "session-on-setup-failure",
		},
		Attempt: providers.ExecuteRequest{
			Provider:    providers.IDAntigravity,
			AttemptID:   "dispatch-agy-missing-session",
			UserMessage: "hello",
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
		Provider:    providers.IDAntigravity,
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
	if !strings.Contains(failure.Message, "Antigravity") {
		t.Fatalf("failure message = %q, want Antigravity unavailable message", failure.Message)
	}
}

func newAgyRoot(t *testing.T, effect agy.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService(catalogwire.IdentityProbe, nil, nil)
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
	root, err := providerservice.NewWithACP(catalog, executionService, disabledACP{}, nil, logging.NoopLogger{}, disabledACP{})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func disabledAgyEffect() agy.Effect {
	return agy.EffectFunc(func(context.Context, execution.ContinuationRequest, func([]byte) error) (agy.EffectResult, error) {
		return agy.EffectResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "Antigravity native execution is unavailable",
		}
	})
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// disabledACP is an inert native-only fixture. Root routing cannot dispatch
// into its unused ACP operations; Close is an explicit completed capability.
type disabledACP struct{ acp.Service }

func (disabledACP) Resolve(providers.ID) (providers.ID, bool) { return "", false }
func (disabledACP) Integrations() []providers.ACPIntegration  { return nil }
func (disabledACP) Close(context.Context) error               { return nil }

func (disabledACP) Continue(context.Context, providers.ID, providers.ExecuteRequest, providers.SessionRef) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP provider continuation is unavailable"}
}
