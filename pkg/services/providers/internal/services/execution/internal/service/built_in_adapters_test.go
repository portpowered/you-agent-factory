package service

import (
	"context"
	"errors"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"

	"go.uber.org/goleak"
)

func TestBuiltInRegistrationsPreserveExplicitDisabledEffects(t *testing.T) {
	t.Parallel()
	failures := map[providers.ID]providers.ExecuteFailure{
		providers.IDAntigravity: {Kind: providers.ExecuteFailureKindDependency, Message: "Antigravity native execution is unavailable"},
		providers.IDCodex:       {Kind: providers.ExecuteFailureKindDependency, Message: "Codex native execution is unavailable"},
		providers.IDClaude:      {Kind: providers.ExecuteFailureKindDependency, Message: "Claude native execution is unavailable"},
	}
	var received []execution.ContinuationRequest
	record := func(provider providers.ID, request execution.ContinuationRequest) error {
		received = append(received, request.Clone())
		return failures[provider]
	}
	registrations := BuiltInRegistrations(
		agy.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (agy.EffectResult, error) {
			return agy.EffectResult{}, record(providers.IDAntigravity, request)
		}),
		codex.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (codex.EffectResult, error) {
			return codex.EffectResult{}, record(providers.IDCodex, request)
		}),
		claude.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (claude.EffectResult, error) {
			return claude.EffectResult{}, record(providers.IDClaude, request)
		}),
	)
	if len(registrations) != 3 {
		t.Fatalf("registration count = %d, want 3", len(registrations))
	}
	if len(received) != 0 {
		t.Fatalf("construction invoked disabled effects: %#v", received)
	}
	seen := make(map[providers.ID]bool, len(registrations))
	for _, registration := range registrations {
		wantFailure, exists := failures[registration.Provider]
		if !exists || seen[registration.Provider] {
			t.Fatalf("unexpected or duplicate registration: %q", registration.Provider)
		}
		seen[registration.Provider] = true
		request := providers.ExecuteRequest{Provider: registration.Provider, AttemptID: "disabled-attempt", UserMessage: "disabled-message"}
		resume := &providers.SessionRef{Provider: registration.Provider, Kind: providers.SessionIDKind, ID: "disabled-session"}
		for _, continuation := range []bool{false, true, false} {
			var result providers.ExecuteResult
			var err error
			if continuation {
				result, err = registration.Continue(t.Context(), execution.ContinuationRequest{ExecuteRequest: request, ResumeSession: resume})
			} else {
				result, err = registration.Attempt(t.Context(), request)
			}
			wantResult := providers.ExecuteResult{}
			expectedFailure := wantFailure.Clone()
			if continuation && registration.Provider == providers.IDAntigravity {
				wantResult.SessionRef = resume
				expectedFailure.SessionRef = resume
			}
			if registration.Provider == providers.IDCodex {
				wantResult.Diagnostics = &providers.ExecuteDiagnostics{
					Progress: []providers.ExecuteProgress{},
					Metadata: map[string]string{
						"inspection_source_bytes": "0",
						"inspection_line_count":   "0",
						"inspection_record_count": "0",
						"completion_evidence":     "agent_message",
					},
				}
			}
			assertDisabledNativeResult(t, result, err, wantResult, expectedFailure)
		}
		want := []execution.ContinuationRequest{{ExecuteRequest: request}, {ExecuteRequest: request, ResumeSession: resume}, {ExecuteRequest: request}}
		if !reflect.DeepEqual(received, want) {
			t.Fatalf("adapter %q requests = %#v, want %#v", registration.Provider, received, want)
		}
		received = nil
	}
}

func assertDisabledNativeResult(t *testing.T, result providers.ExecuteResult, err error, wantResult providers.ExecuteResult, wantFailure providers.ExecuteFailure) {
	t.Helper()
	var failure execution.AttemptFailure
	if !errors.As(err, &failure) || !reflect.DeepEqual(failure.Declared, &wantFailure) {
		t.Fatalf("disabled failure = %#v, want %#v", err, wantFailure)
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("disabled result = %#v, want %#v", result, wantResult)
	}
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuiltInRegistrationsRouteSuppliedEffectsAndContinuation(t *testing.T) {
	t.Parallel()
	for _, provider := range []providers.ID{providers.IDAntigravity, providers.IDCodex, providers.IDClaude} {
		t.Run(string(provider), func(t *testing.T) {
			t.Parallel()
			var received []execution.ContinuationRequest
			record := func(selected providers.ID, request execution.ContinuationRequest) error {
				if selected != provider {
					t.Fatalf("effect provider = %q, want %q", selected, provider)
				}
				received = append(received, request.Clone())
				return execution.AttemptFailure{NativeError: context.Canceled}
			}
			registrations := BuiltInRegistrations(
				agy.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (agy.EffectResult, error) {
					return agy.EffectResult{}, record(providers.IDAntigravity, request)
				}),
				codex.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (codex.EffectResult, error) {
					return codex.EffectResult{}, record(providers.IDCodex, request)
				}),
				claude.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (claude.EffectResult, error) {
					return claude.EffectResult{}, record(providers.IDClaude, request)
				}),
			)
			request := providers.ExecuteRequest{Provider: provider, AttemptID: "selected", Model: "request-model", UserMessage: "request-message"}
			resume := &providers.SessionRef{Provider: provider, Kind: providers.SessionIDKind, ID: "session-selected"}
			for _, registration := range registrations {
				if registration.Provider != provider {
					continue
				}
				_, err := registration.Attempt(t.Context(), request)
				assertSuppliedEffectNativeCancellation(t, err)
				_, err = registration.Continue(t.Context(), execution.ContinuationRequest{ExecuteRequest: request, ResumeSession: resume})
				assertSuppliedEffectNativeCancellation(t, err)
				_, err = registration.Attempt(t.Context(), request)
				assertSuppliedEffectNativeCancellation(t, err)
			}
			want := []execution.ContinuationRequest{{ExecuteRequest: request}, {ExecuteRequest: request, ResumeSession: resume}, {ExecuteRequest: request}}
			if !reflect.DeepEqual(received, want) {
				t.Fatalf("effect requests = %#v, want %#v", received, want)
			}
		})
	}
}

func assertSuppliedEffectNativeCancellation(t *testing.T, err error) {
	t.Helper()
	var failure execution.AttemptFailure
	if !errors.As(err, &failure) || !errors.Is(err, context.Canceled) {
		t.Fatalf("supplied effect error = %#v, want native cancellation for normalization", err)
	}
}
