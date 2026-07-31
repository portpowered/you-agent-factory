package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestExecuteHappyPathPreservesCorrelationAndObservations(t *testing.T) {
	t.Parallel()

	var observations []workers.ExecutionObservation
	var mu sync.Mutex
	runner := &stubRunner{content: "accepted-output"}
	service := mustExecuteService(t, runner, func(_ context.Context, observation workers.ExecutionObservation) error {
		mu.Lock()
		defer mu.Unlock()
		observations = append(observations, observation.Clone())
		return nil
	})

	request := validExecuteRequest("dispatch-1", "attempt-1")
	request.Target.Prompt.SystemPrompt = "secret-system"
	request.Target.Prompt.UserMessage = "secret-user"
	request.Target.Environment.Vars = map[string]string{"CRED": "raw-secret"}

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Correlation.DispatchID != "dispatch-1" || result.Correlation.AttemptID != "attempt-1" {
		t.Fatalf("correlation = %#v", result.Correlation)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "accepted-output" {
		t.Fatalf("output = %#v", result.Output)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observations) < 2 {
		t.Fatalf("observations = %#v, want started and terminal", observations)
	}
	if observations[0].Kind != workers.ExecutionObservationKindStarted {
		t.Fatalf("first observation = %#v", observations[0])
	}
	terminal := observations[len(observations)-1]
	if terminal.Kind != workers.ExecutionObservationKindCompleted {
		t.Fatalf("terminal observation = %#v", terminal)
	}
	for _, observation := range observations {
		for _, value := range observation.Metadata {
			if value == "raw-secret" || value == "secret-system" || value == "secret-user" {
				t.Fatalf("unsafe value persisted in observation: %#v", observation)
			}
		}
	}
}

func TestExecutePreStartValidationReturnsTypedError(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{}, nil)
	_, err := service.Execute(context.Background(), workers.ExecuteRequest{})
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidExecuteRequest", err)
	}
}

func TestExecuteCanceledContextBeforeStart(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Execute(ctx, validExecuteRequest("dispatch-cancel", "attempt-cancel"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestExecuteCancellationTerminatesRunner(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	runner := &stubRunner{
		execute: func(ctx context.Context, _ workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			close(started)
			<-ctx.Done()
			return workers.RunnerExecutionResult{}, ctx.Err()
		},
	}
	service := mustExecuteService(t, runner, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan workers.ExecuteResult, 1)
	go func() {
		result, err := service.Execute(ctx, validExecuteRequest("dispatch-2", "attempt-2"))
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
		done <- result
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}
	cancel()
	select {
	case result := <-done:
		if result.Outcome != workers.ExecutionOutcomeCanceled {
			t.Fatalf("outcome = %q, want CANCELED", result.Outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute() did not return after cancel")
	}
}

func TestExecutePanicBecomesFailedResult(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			panic("boom")
		},
	}, nil)
	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-3", "attempt-3"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if result.Failure == nil || result.Diagnostics == nil || result.Diagnostics.Panic == nil {
		t.Fatalf("result = %#v, want panic diagnostics", result)
	}
}

func TestExecuteProviderFailureNormalized(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypeThrottled,
				"provider throttled",
				nil,
			)
		},
	}, nil)
	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-4", "attempt-4"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if result.Failure == nil || result.Failure.Type != workers.WorkFailureTypeThrottled || !result.Failure.RetryHint {
		t.Fatalf("failure = %#v", result.Failure)
	}
}

func TestExecuteSequentialCallsShareNoDispatchState(t *testing.T) {
	t.Parallel()

	var seen []string
	var mu sync.Mutex
	runner := &stubRunner{
		execute: func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			mu.Lock()
			seen = append(seen, request.Dispatch.DispatchID)
			mu.Unlock()
			return workers.RunnerExecutionResult{Content: request.Dispatch.DispatchID}, nil
		},
	}
	service := mustExecuteService(t, runner, nil)
	first, err := service.Execute(context.Background(), validExecuteRequest("dispatch-a", "attempt-a"))
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	second, err := service.Execute(context.Background(), validExecuteRequest("dispatch-b", "attempt-b"))
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if first.Correlation.DispatchID == second.Correlation.DispatchID {
		t.Fatalf("shared dispatch correlation: %#v %#v", first.Correlation, second.Correlation)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "dispatch-a" || seen[1] != "dispatch-b" {
		t.Fatalf("seen = %#v", seen)
	}
}

func TestExecuteConcurrentCallsAreIsolated(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maxActive atomic.Int32
	runner := &stubRunner{
		execute: func(ctx context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			defer active.Add(-1)
			select {
			case <-ctx.Done():
				return workers.RunnerExecutionResult{}, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				return workers.RunnerExecutionResult{Content: request.Dispatch.DispatchID}, nil
			}
		},
	}
	service := mustExecuteService(t, runner, nil)

	const workersN = 8
	results := make(chan workers.ExecuteResult, workersN)
	errs := make(chan error, workersN)
	for index := 0; index < workersN; index++ {
		dispatchID := "dispatch-" + string(rune('a'+index))
		attemptID := "attempt-" + string(rune('a'+index))
		go func(dispatchID, attemptID string) {
			result, err := service.Execute(context.Background(), validExecuteRequest(dispatchID, attemptID))
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}(dispatchID, attemptID)
	}
	seen := map[string]bool{}
	for index := 0; index < workersN; index++ {
		select {
		case err := <-errs:
			t.Fatalf("Execute() error = %v", err)
		case result := <-results:
			if result.Outcome != workers.ExecutionOutcomeAccepted {
				t.Fatalf("outcome = %q", result.Outcome)
			}
			if seen[result.Correlation.DispatchID] {
				t.Fatalf("duplicate dispatch result %q", result.Correlation.DispatchID)
			}
			seen[result.Correlation.DispatchID] = true
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for concurrent Execute results")
		}
	}
	if maxActive.Load() < 2 {
		t.Fatalf("max concurrent runner calls = %d, want overlap", maxActive.Load())
	}
}

func TestConstructionIsInert(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			t.Fatal("runner executed during construction")
			return workers.RunnerExecutionResult{}, nil
		},
	}
	var observed atomic.Int32
	_, err := executeservice.New(executeservice.Dependencies{
		Runners:   &staticRunners{runner: runner},
		Providers: &executeProvidersFake{},
		Observe: func(context.Context, workers.ExecutionObservation) error {
			observed.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if observed.Load() != 0 {
		t.Fatalf("observations during construction = %d", observed.Load())
	}
}

func mustExecuteService(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
) *executeservice.Service {
	t.Helper()
	service, err := executeservice.New(executeservice.Dependencies{
		Runners:   &staticRunners{runner: runner},
		Providers: &executeProvidersFake{},
		Observe:   observe,
		Clock:     func() time.Time { return time.Unix(10, 0) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

type executeProvidersFake struct{}

func (*executeProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

func (*executeProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{
		Providers: []providers.Descriptor{{
			ID:           providers.IDCodex,
			Aliases:      []string{"openai"},
			DisplayName:  "Codex",
			Availability: providers.AvailabilitySelectable,
			Readiness:    providers.ReadinessReady,
		}},
	}, nil
}

func (*executeProvidersFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCodex {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{
		ID:           providers.IDCodex,
		Aliases:      []string{"openai"},
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}}, nil
}

func validExecuteRequest(dispatchID, attemptID string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-1",
			RuntimeID:        "runtime-1",
			DispatchID:       dispatchID,
			AttemptID:        attemptID,
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "writer",
			WorkstationName: "review",
			RunnerID:        string(providers.IDCodex),
			Provider:        workers.ProviderReference{ID: string(providers.IDCodex)},
			Prompt: workers.PromptPolicy{
				UserMessage: "do work",
			},
		},
	}
}

type stubRunner struct {
	content string
	execute func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error)
}

func (runner *stubRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if runner.execute != nil {
		return runner.execute(ctx, request)
	}
	return workers.RunnerExecutionResult{Content: runner.content}, nil
}

type staticRunners struct {
	runner workers.Runner
}

func (registry *staticRunners) Resolve(
	request runners.ResolutionRequest,
) (runners.Binding, error) {
	return runners.Binding{
		Identity: request.Identity,
		Metadata: workers.RunnerMetadata{ID: request.Identity},
		Runner:   registry.runner,
	}, nil
}

func (registry *staticRunners) Execute(
	ctx context.Context,
	request runners.ExecuteRequest,
) (runners.ExecuteResult, error) {
	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity:             request.Identity,
		RequiredCapabilities: request.RequiredCapabilities,
	})
	if err != nil {
		return runners.ExecuteResult{}, err
	}
	return binding.Runner.Execute(ctx, request.Attempt)
}
