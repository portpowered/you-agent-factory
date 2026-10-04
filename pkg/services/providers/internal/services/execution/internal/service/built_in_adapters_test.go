package service

import (
	"context"
	"errors"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"

	"go.uber.org/goleak"
)

func TestBuiltInRegistrationsSelectOnlyAntigravityCodexAndClaudeAdapters(t *testing.T) {
	t.Parallel()

	registrations := BuiltInRegistrations(nil, nil, nil)
	if len(registrations) != 3 {
		t.Fatalf("registration count = %d, want 3", len(registrations))
	}

	byID := make(map[providers.ID]string, len(registrations))
	for _, registration := range registrations {
		_, err := registration.Attempt(
			context.Background(),
			providers.ExecuteRequest{Provider: registration.Provider},
		)
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency {
			t.Fatalf("adapter %q error = %#v, want dependency failure", registration.Provider, err)
		}
		byID[registration.Provider] = failure.Message
	}

	if !strings.Contains(byID[providers.IDCodex], "Codex") {
		t.Fatalf("Codex adapter message = %q", byID[providers.IDCodex])
	}
	if !strings.Contains(byID[providers.IDClaude], "Claude") {
		t.Fatalf("Claude adapter message = %q", byID[providers.IDClaude])
	}
	if !strings.Contains(byID[providers.IDAntigravity], "Antigravity") {
		t.Fatalf("Antigravity adapter message = %q", byID[providers.IDAntigravity])
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
			record := func(request execution.ContinuationRequest) error {
				received = append(received, request.Clone())
				return execution.AttemptFailure{NativeError: context.Canceled}
			}
			registrations := BuiltInRegistrations(
				agy.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (agy.EffectResult, error) {
					return agy.EffectResult{}, record(request)
				}),
				codex.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (codex.EffectResult, error) {
					return codex.EffectResult{}, record(request)
				}),
				claude.EffectFunc(func(_ context.Context, request execution.ContinuationRequest, _ func([]byte) error) (claude.EffectResult, error) {
					return claude.EffectResult{}, record(request)
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
			}
			want := []execution.ContinuationRequest{{ExecuteRequest: request}, {ExecuteRequest: request, ResumeSession: resume}}
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
