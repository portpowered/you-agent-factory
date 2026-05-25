package workers

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestAgentExecutor_RawDeadlineExceeded_RetriesBeforeSuccess(t *testing.T) {
	provider := &agentMockProvider{
		errors: []error{
			context.DeadlineExceeded,
			context.DeadlineExceeded,
			nil,
		},
		responses: []interfaces.InferenceResponse{
			{},
			{},
			{Content: "Recovered. COMPLETE"},
		},
	}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)
	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(baseDelay time.Duration) time.Duration {
		return baseDelay / 2
	}

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-raw-timeout-success",
			TransitionID: "t-raw-timeout-success",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if result.Output != "Recovered. COMPLETE" {
		t.Fatalf("Output = %q, want %q", result.Output, "Recovered. COMPLETE")
	}
	if provider.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3", provider.callCount)
	}
	if result.Metrics.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", result.Metrics.RetryCount)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
	}
}

func TestAgentExecutor_RawDeadlineExceeded_ExhaustsRetriesIntoStructuredTimeoutFailure(t *testing.T) {
	provider := &agentMockProvider{err: context.DeadlineExceeded}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "test-model"},
		},
	}, provider)
	var sleeps []time.Duration
	executor.retryConfig.sleep = func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}
	executor.retryConfig.jitter = func(time.Duration) time.Duration { return 0 }

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-raw-timeout-fail",
			TransitionID: "t-raw-timeout-fail",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.Error != "execution timeout" {
		t.Fatalf("Error = %q, want %q", result.Error, "execution timeout")
	}
	if provider.callCount != 3 {
		t.Fatalf("provider call count = %d, want 3", provider.callCount)
	}
	if result.Metrics.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", result.Metrics.RetryCount)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleep count = %d, want 2", len(sleeps))
	}
	if result.FailureMetadata == nil {
		t.Fatal("FailureMetadata = nil, want timeout metadata")
	}
	if result.FailureMetadata.Type != interfaces.WorkFailureTypeTimeout {
		t.Fatalf("FailureMetadata.Type = %q, want %q", result.FailureMetadata.Type, interfaces.WorkFailureTypeTimeout)
	}
	if result.FailureMetadata.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("FailureMetadata.Family = %q, want %q", result.FailureMetadata.Family, interfaces.WorkFailureFamilyRetryable)
	}
	if result.ProviderFailure == nil {
		t.Fatal("ProviderFailure = nil, want timeout metadata")
	}
	if result.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("ProviderFailure.Type = %q, want %q", result.ProviderFailure.Type, interfaces.ProviderErrorTypeTimeout)
	}
	if result.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("ProviderFailure.Family = %q, want %q", result.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
}
