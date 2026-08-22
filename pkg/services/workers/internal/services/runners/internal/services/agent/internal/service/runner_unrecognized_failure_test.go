package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFailureTypeForProviderKindRetainsTransientPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind         providers.ExecuteFailureKind
		wantRetry    bool
		wantThrottle bool
	}{
		{kind: providers.ExecuteFailureKindThrottled, wantRetry: true, wantThrottle: true},
		{kind: providers.ExecuteFailureKindDependency, wantRetry: true},
		{kind: providers.ExecuteFailureKindTimeout, wantRetry: true},
		{kind: providers.ExecuteFailureKindUnknown},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()
			failureType := failureTypeForProviderKind(test.kind)
			decision := workers.FailureDecisionFromMetadata(&workers.WorkFailureMetadata{Type: failureType})
			if decision.Retryable != test.wantRetry || decision.TriggersThrottlePause != test.wantThrottle {
				t.Fatalf("failure decision for %q = %#v, want retry=%t throttle=%t", test.kind, decision, test.wantRetry, test.wantThrottle)
			}
			if test.kind == providers.ExecuteFailureKindUnknown && !decision.Terminal {
				t.Fatalf("failure decision for unrecognized %q = %#v, want terminal", test.kind, decision)
			}
		})
	}
}

func TestExecuteUnrecognizedProviderFailureIsTerminalAndProviderNeutral(t *testing.T) {
	t.Parallel()

	const providerDetail = "future-provider-refusal with credential=secret"
	fake := &failingProvidersFake{failure: providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: providerDetail,
	}}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), baseAgentRequest())
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if fake.executeCalls != 1 {
		t.Fatalf("provider calls = %d, want one attempt", fake.executeCalls)
	}
	if providerErr.Type != workers.WorkFailureTypePermanentBadRequest ||
		providerErr.Message != unrecognizedProviderFailureMessage {
		t.Fatalf("ProviderError = %#v, want terminal neutral refusal", providerErr)
	}
	decision := workers.WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Terminal || decision.Retryable || decision.TriggersThrottlePause {
		t.Fatalf("failure decision = %#v, want terminal non-retryable refusal", decision)
	}
	if strings.Contains(providerErr.Message, providerDetail) {
		t.Fatalf("provider detail leaked into neutral refusal message: %q", providerErr.Message)
	}
}
