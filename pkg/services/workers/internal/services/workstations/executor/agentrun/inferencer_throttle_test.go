package agentrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

func capacityOverloadErrors(count int) ([]error, error) {
	overloaded := workerexecution.NewProviderError(workerexecution.WorkFailureTypeThrottled, "Selected model is at capacity", nil)
	errs := make([]error, count)
	for i := range errs {
		errs[i] = overloaded
	}
	return errs, overloaded
}

func fakeClockInferencer(runner runnerContract, slept *time.Duration) *runnerInferencer {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	inferencer := newRunnerInferencer(runner, workerexecution.ProviderInferenceRequest{}).(*runnerInferencer)
	inferencer.throttlePolicy.Now = func() time.Time { return now }
	inferencer.retrySleep = func(_ context.Context, d time.Duration) error {
		*slept += d
		now = now.Add(d)
		return nil
	}
	return inferencer
}

func TestRunnerInferencer_OutlastsCapacityEventThenSucceeds(t *testing.T) {
	t.Parallel()

	var slept time.Duration
	errs, _ := capacityOverloadErrors(8)
	runner := &sequenceRunner{errors: errs, response: "recovered"}
	inferencer := fakeClockInferencer(runner, &slept)

	result, err := inferencer.Infer(context.Background(), messages.InferenceRequest{})
	if err != nil || result.Message.TextContent() != "recovered" {
		t.Fatalf("Infer() = %#v, %v, want recovered response after capacity returns", result, err)
	}
	if runner.calls != 9 {
		t.Fatalf("runner calls = %d, want 9 (8 overloads then success)", runner.calls)
	}
	if slept > 30*time.Minute {
		t.Fatalf("slept %s, want within the 30 minute window", slept)
	}
}

func TestRunnerInferencer_PersistentCapacityStopsAtWindow(t *testing.T) {
	t.Parallel()

	var slept time.Duration
	errs, overloaded := capacityOverloadErrors(1000)
	runner := &sequenceRunner{errors: errs, response: "never"}
	inferencer := fakeClockInferencer(runner, &slept)

	_, err := inferencer.Infer(context.Background(), messages.InferenceRequest{})
	if !errors.Is(err, overloaded) {
		t.Fatalf("Infer() error = %v, want the throttled provider error after the window", err)
	}
	if slept != 30*time.Minute {
		t.Fatalf("slept %s, want exactly the 30 minute window", slept)
	}
	var providerErr *workerexecution.ProviderError
	if errors.As(err, &providerErr) {
		decision := workerexecution.WorkFailureDecisionFromProviderError(providerErr)
		if !decision.Retryable || decision.Terminal || !decision.TriggersThrottlePause {
			t.Fatalf("exhausted window lost backpressure: %+v", decision)
		}
	}
}

func TestRunnerInferencer_OutlastsDependencyEventThenSucceeds(t *testing.T) {
	t.Parallel()

	var slept time.Duration
	errs, _ := dependencyOutageErrors(8)
	runner := &sequenceRunner{errors: errs, response: "recovered"}
	inferencer := fakeClockInferencer(runner, &slept)

	result, err := inferencer.Infer(context.Background(), messages.InferenceRequest{})
	if err != nil || result.Message.TextContent() != "recovered" {
		t.Fatalf("Infer() = %#v, %v, want recovered response after capacity returns", result, err)
	}
	if runner.calls != 9 {
		t.Fatalf("runner calls = %d, want 9 (8 overloads then success)", runner.calls)
	}
	if slept > 30*time.Minute {
		t.Fatalf("slept %s, want within the 30 minute window", slept)
	}
}

func TestRunnerInferencer_PersistentDependencyStopsAtWindow(t *testing.T) {
	t.Parallel()

	var slept time.Duration
	errs, overloaded := dependencyOutageErrors(1000)
	runner := &sequenceRunner{errors: errs, response: "never"}
	inferencer := fakeClockInferencer(runner, &slept)

	_, err := inferencer.Infer(context.Background(), messages.InferenceRequest{})
	if !errors.Is(err, overloaded) {
		t.Fatalf("Infer() error = %v, want the throttled provider error after the window", err)
	}
	if slept != 30*time.Minute {
		t.Fatalf("slept %s, want exactly the 30 minute window", slept)
	}
	var providerErr *workerexecution.ProviderError
	if errors.As(err, &providerErr) {
		decision := workerexecution.WorkFailureDecisionFromProviderError(providerErr)
		if !decision.Retryable || decision.Terminal || !decision.TriggersThrottlePause {
			t.Fatalf("exhausted window lost backpressure: %+v", decision)
		}
	}
}

func dependencyOutageErrors(count int) ([]error, error) {
	outage := workerexecution.NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider outage", nil)
	outage.ProviderFailureKind = providers.ExecuteFailureKindDependency
	outage.Diagnostics = &workerexecution.WorkDiagnostics{Provider: &workerexecution.ProviderDiagnostic{
		ResponseMetadata: map[string]string{providers.ExecuteDiagnosticMetadataUpstreamOutage: "true"},
	}}
	errs := make([]error, count)
	for i := range errs {
		errs[i] = outage
	}
	return errs, outage
}

func TestRunnerInferencer_DependencyWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	errs, _ := dependencyOutageErrors(8)
	runner := &sequenceRunner{errors: errs, response: "never"}
	var slept time.Duration
	inferencer := fakeClockInferencer(runner, &slept)
	inferencer.retrySleep = func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }
	_, err := inferencer.Infer(ctx, messages.InferenceRequest{})
	if !errors.Is(err, context.Canceled) || runner.calls != 1 {
		t.Fatalf("Infer = %v, calls=%d, want canceled after first outage", err, runner.calls)
	}
}
