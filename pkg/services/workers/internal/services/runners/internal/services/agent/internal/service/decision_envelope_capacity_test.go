package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// partialOutputFailingProvidersFake models a provider turn that streamed
// non-envelope assistant text and then died with a declared failure, the shape
// a long Codex turn takes when the model hits capacity at turn end.
type partialOutputFailingProvidersFake struct {
	providers.Service
	content string
	failure providers.ExecuteFailure
}

func (fake *partialOutputFailingProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{Content: fake.content}, fake.failure
}

func malformedEnvelopeResult() workers.WorkResult {
	return workers.WorkResult{
		Outcome: workers.OutcomeFailed,
		Error:   "reviewer decision envelope invalid: decision envelope: invalid JSON",
		FailureMetadata: &workers.WorkFailureMetadata{
			Family: workers.WorkFailureFamilyTerminal,
			Type:   workers.WorkFailureTypeUnknown,
		},
	}
}

func TestExecuteKeepsDeclaredProviderFailureWhenPartialOutputIsNotAnEnvelope(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		kind     providers.ExecuteFailureKind
		wantType workers.WorkFailureType
		throttle bool
		outage   bool
	}{
		{"capacity at turn end stays throttled", providers.ExecuteFailureKindThrottled, workers.WorkFailureTypeThrottled, true, false},
		{"local dependency keeps bounded retries", providers.ExecuteFailureKindDependency, workers.WorkFailureTypeInternalServerError, false, false},
		{"upstream outage keeps backpressure", providers.ExecuteFailureKindDependency, workers.WorkFailureTypeInternalServerError, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			envelopes := &recordingDecisionEnvelopeService{result: malformedEnvelopeResult()}
			fake := &partialOutputFailingProvidersFake{
				content: "Thinking about the plan",
				failure: providers.ExecuteFailure{Kind: tc.kind, Message: "declared"},
			}
			if tc.outage {
				fake.failure.Diagnostics = &providers.ExecuteDiagnostics{Metadata: map[string]string{
					providers.ExecuteDiagnosticMetadataUpstreamOutage: "true",
				}}
			}
			runner, err := New(fake, noopPublisher, envelopes)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			request := baseAgentRequest()
			request.DecisionEnvelope = true

			_, err = runner.Execute(t.Context(), request)
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("ProviderError.Type = %q, want %q (envelope parse of partial output must not mask the provider failure)", providerErr.Type, tc.wantType)
			}
			if got := workers.WorkFailureDecisionFromProviderError(providerErr).TriggersThrottlePause; got != tc.throttle {
				t.Fatalf("TriggersThrottlePause = %v, want %v", got, tc.throttle)
			}
		})
	}
}
