package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestExecuteHappyPathPreservesCorrelationAndEmitsTerminalObservation(t *testing.T) {
	t.Parallel()

	var observations []workers.ExecutionObservation
	var observationsMu sync.Mutex
	service := mustExecuteService(t, &stubRunner{content: "accepted-output"}, func(
		_ context.Context,
		observation workers.ExecutionObservation,
	) error {
		observationsMu.Lock()
		defer observationsMu.Unlock()
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
	if result.Correlation.DispatchID != "dispatch-1" ||
		result.Correlation.AttemptID != "attempt-1" {
		t.Fatalf("correlation = %#v", result.Correlation)
	}
	if len(result.Output.Primary) != 1 ||
		result.Output.Primary[0].Text != "accepted-output" {
		t.Fatalf("output = %#v", result.Output)
	}

	observationsMu.Lock()
	defer observationsMu.Unlock()
	if len(observations) != 2 {
		t.Fatalf("observations = %#v, want started and terminal", observations)
	}
	if observations[0].Kind != workers.ExecutionObservationKindStarted {
		t.Fatalf("first observation = %#v", observations[0])
	}
	if observations[1].Kind != workers.ExecutionObservationKindCompleted {
		t.Fatalf("terminal observation = %#v", observations[1])
	}
	if observations[1].Sequence != 2 {
		t.Fatalf("terminal sequence = %d, want 2", observations[1].Sequence)
	}
	for _, observation := range observations {
		for _, value := range observation.Metadata {
			if value == "raw-secret" || value == "secret-system" || value == "secret-user" {
				t.Fatalf("unsafe value persisted in observation: %#v", observation)
			}
		}
	}
}

func TestExecuteClonesRequestBeforeRunnerStarts(t *testing.T) {
	t.Parallel()

	captured := make(chan workers.RunnerExecutionRequest, 1)
	release := make(chan struct{})
	runner := &stubRunner{
		execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			captured <- request
			<-release
			return workers.RunnerExecutionResult{Content: "done"}, nil
		},
	}
	service := mustExecuteService(t, runner, nil)
	request := validExecuteRequest("dispatch-clone", "attempt-clone")
	request.Target.Environment.Vars = map[string]string{"TOKEN": "original"}
	request.Target.Environment.ProcessEnvironment = []string{"ORIGINAL=1"}

	done := make(chan workers.ExecuteResult, 1)
	go func() {
		result, err := service.Execute(context.Background(), request)
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
		done <- result
	}()

	got := <-captured
	request.Target.Environment.Vars["TOKEN"] = "mutated"
	request.Target.Environment.ProcessEnvironment[0] = "MUTATED=1"
	close(release)
	<-done

	if got.EnvVars["TOKEN"] != "original" {
		t.Fatalf("runner env = %#v, want original request snapshot", got.EnvVars)
	}
	if got.ProcessEnvironment[0] != "ORIGINAL=1" {
		t.Fatalf("runner process environment = %#v, want original snapshot", got.ProcessEnvironment)
	}
	if got.WorkerType != "writer" {
		t.Fatalf("runner worker type = %q, want writer", got.WorkerType)
	}
}

func TestExecuteConcurrentCallsDoNotShareDispatchState(t *testing.T) {
	t.Parallel()

	const callCount = 6
	var active atomic.Int32
	var maxActive atomic.Int32
	started := make(chan struct{}, callCount)
	release := make(chan struct{})
	runner := &stubRunner{
		execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			return workers.RunnerExecutionResult{Content: request.Dispatch.DispatchID}, nil
		},
	}
	service := mustExecuteService(t, runner, nil)
	results := make(chan workers.ExecuteResult, callCount)
	errs := make(chan error, callCount)
	for index := 0; index < callCount; index++ {
		dispatchID := "dispatch-" + string(rune('a'+index))
		attemptID := "attempt-" + string(rune('a'+index))
		go func() {
			result, err := service.Execute(
				context.Background(),
				validExecuteRequest(dispatchID, attemptID),
			)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	for index := 0; index < callCount; index++ {
		<-started
	}
	close(release)

	seen := make(map[string]bool, callCount)
	for index := 0; index < callCount; index++ {
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
		}
	}
	if maxActive.Load() < 2 {
		t.Fatalf("max concurrent runner calls = %d, want overlap", maxActive.Load())
	}
}

func TestExecuteConstructionIsInert(t *testing.T) {
	t.Parallel()

	var runnerCalls atomic.Int32
	var observationCalls atomic.Int32
	runner := &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			runnerCalls.Add(1)
			return workers.RunnerExecutionResult{}, nil
		},
	}
	_, err := executeservice.New(
		&staticRunners{runner: runner},
		nil,
		func(context.Context, workers.ExecutionObservation) error {
			observationCalls.Add(1)
			return nil
		},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runnerCalls.Load() != 0 || observationCalls.Load() != 0 {
		t.Fatalf(
			"construction effects = runner %d observations %d, want zero",
			runnerCalls.Load(),
			observationCalls.Load(),
		)
	}
}

func mustExecuteService(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
) *executeservice.Service {
	t.Helper()
	service, err := executeservice.New(
		&staticRunners{runner: runner},
		nil,
		observe,
		nil,
		func() time.Time { return time.Unix(10, 0) },
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
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
			RunnerID:        runners.ScriptIdentity,
		},
	}
}
