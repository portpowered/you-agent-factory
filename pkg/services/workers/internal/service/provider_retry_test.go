package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecuteProviderWithRetryCarriesSessionAndBoundsAttempts(t *testing.T) {
	t.Parallel()

	service := &Service{}
	request := workers.RunnerExecutionRequest{}
	var attempts []workers.RunnerExecutionRequest
	providerErr := workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"provider temporarily unavailable",
		nil,
	)
	providerErr.ProviderSession = &workers.ProviderSessionMetadata{ID: "provider-session-1"}

	result, err := service.executeProviderWithRetry(
		context.Background(),
		request,
		func(attempt workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts = append(attempts, attempt)
			if len(attempts) == 1 {
				return workers.RunnerExecutionResult{}, providerErr
			}
			return workers.RunnerExecutionResult{Content: "accepted"}, nil
		},
	)
	if err != nil {
		t.Fatalf("executeProviderWithRetry() error = %v", err)
	}
	if result.Content != "accepted" || len(attempts) != 2 {
		t.Fatalf("result = %#v, attempts = %d, want accepted after two attempts", result, len(attempts))
	}
	if attempts[1].SessionID != "provider-session-1" {
		t.Fatalf("retry session = %q, want provider-session-1", attempts[1].SessionID)
	}
	if len(attempts[1].RequiredOptionalCapabilities) != 1 ||
		attempts[1].RequiredOptionalCapabilities[0] != workers.RunnerOptionalCapabilitySessionResume {
		t.Fatalf("retry capabilities = %#v, want session resume", attempts[1].RequiredOptionalCapabilities)
	}
}

func TestExecuteProviderWithRetryStopsAtMaximumAndHonorsCancellation(t *testing.T) {
	t.Parallel()

	service := &Service{}
	request := workers.RunnerExecutionRequest{}
	providerErr := workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"provider temporarily unavailable",
		nil,
	)
	var attempts int
	_, err := service.executeProviderWithRetry(
		context.Background(),
		request,
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, providerErr) || attempts != detachedProviderMaxRetries+1 {
		t.Fatalf("bounded retry = (%v, %d attempts), want original error and %d attempts", err, attempts, detachedProviderMaxRetries+1)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	attempts = 0
	_, err = service.executeProviderWithRetry(
		canceled,
		request,
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled retry = (%v, %d attempts), want context.Canceled after one attempt", err, attempts)
	}
}

func TestProviderSessionForRetryPrefersErrorThenResult(t *testing.T) {
	t.Parallel()

	errorSession := &workers.ProviderSessionMetadata{ID: "error-session"}
	resultSession := &workers.ProviderSessionMetadata{ID: "result-session"}
	if got := providerSessionForRetry(
		workers.NewProviderErrorWithSession(workers.WorkFailureTypeThrottled, "busy", nil, errorSession),
		workers.RunnerExecutionResult{ProviderSession: resultSession},
	); got == nil || got.ID != "error-session" {
		t.Fatalf("providerSessionForRetry(error, result) = %#v, want error session", got)
	}
	if got := providerSessionForRetry(nil, workers.RunnerExecutionResult{ProviderSession: resultSession}); got == nil || got.ID != "result-session" {
		t.Fatalf("providerSessionForRetry(nil, result) = %#v, want result session", got)
	}
	if got := providerSessionForRetry(nil, workers.RunnerExecutionResult{}); got != nil {
		t.Fatalf("providerSessionForRetry(empty) = %#v, want nil", got)
	}
}

func TestNormalizeProviderOverrideResultInfersStopAndContinuationOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     workers.RunnerExecutionResult
		request    workers.RunnerExecutionRequest
		wantResult workers.WorkOutcome
	}{
		{
			name:       "existing outcome is preserved",
			result:     workers.RunnerExecutionResult{Outcome: workers.OutcomeAccepted, Content: "ignored"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeAccepted,
		},
		{
			name:       "stop token accepts",
			result:     workers.RunnerExecutionResult{Content: "answer DONE"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeAccepted,
		},
		{
			name:       "continue marker continues",
			result:     workers.RunnerExecutionResult{Content: "<CONTINUE>"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeContinue,
		},
		{
			name:       "missing marker rejects",
			result:     workers.RunnerExecutionResult{Content: "not complete"},
			request:    workers.RunnerExecutionRequest{StopToken: "DONE"},
			wantResult: workers.OutcomeRejected,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeProviderOverrideResult(test.result, test.request)
			if got.Outcome != test.wantResult {
				t.Fatalf("outcome = %q, want %q", got.Outcome, test.wantResult)
			}
		})
	}
}

func TestHasProviderCompletionEvidenceAcceptsProviderMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		result       workers.RunnerExecutionResult
		wantEvidence bool
	}{
		{name: "empty content", result: workers.RunnerExecutionResult{}, wantEvidence: false},
		{
			name: "worker metadata",
			result: workers.RunnerExecutionResult{
				Content: "answer",
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
					workers.ProviderResponseMetadataCompletionEvidence: "agent_message",
				}},
			},
			wantEvidence: true,
		},
		{
			name: "provider metadata",
			result: workers.RunnerExecutionResult{
				Content: "answer",
				Diagnostics: &workers.WorkDiagnostics{Provider: &workers.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
					},
				}},
			},
			wantEvidence: true,
		},
		{
			name: "unrelated metadata",
			result: workers.RunnerExecutionResult{
				Content:     "answer",
				Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{"other": "value"}},
			},
			wantEvidence: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasProviderCompletionEvidence(test.result); got != test.wantEvidence {
				t.Fatalf("hasProviderCompletionEvidence() = %t, want %t", got, test.wantEvidence)
			}
		})
	}
}
