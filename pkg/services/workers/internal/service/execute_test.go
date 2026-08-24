package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
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
	assertAcceptedResult(t, result, "dispatch-1", "attempt-1", "accepted-output")

	observationsMu.Lock()
	defer observationsMu.Unlock()
	assertSafeCompletedObservations(t, observations)
}

func TestExecutePreservesNonTextProposedOutput(t *testing.T) {
	proposed := &workers.ProposedOutput{Primary: []work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		URL:         "data:audio/wav;base64,YXVkaW8=",
		ContentType: "audio/wav",
		Slot:        "audio",
	}}}
	service := mustExecuteService(t, &stubRunner{proposedOutput: proposed}, nil)

	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-audio", "attempt-audio"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || len(result.Output.Primary) != 1 {
		t.Fatalf("result = %#v, want accepted audio proposal", result)
	}
	part := result.Output.Primary[0]
	if part.Type != work.WorkContentPartTypeAudio || part.URL != proposed.Primary[0].URL ||
		part.ContentType != "audio/wav" || part.Slot != "audio" {
		t.Fatalf("output part = %#v, want detached audio proposal", part)
	}
	if &result.Output.Primary[0] == &proposed.Primary[0] {
		t.Fatal("Execute() reused the runner's mutable proposed output")
	}
}

func TestExecuteStructuredOutputReturnsValidatedNativeValue(t *testing.T) {
	service := mustExecuteService(t, &stubRunner{
		content: `{"answer":"ok","schemaValidated":"customer-value"}`,
	}, nil)
	request := validExecuteRequest("dispatch-structured", "attempt-structured")
	request.Target.Prompt.OutputSchema = `{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || !result.StructuredResultPresent {
		t.Fatalf("result = %#v, want accepted validated structured result", result)
	}
	structured, ok := result.StructuredResult.(map[string]any)
	if !ok || structured["answer"] != "ok" || structured["schemaValidated"] != "customer-value" {
		t.Fatalf("structured result = %#v, want native customer object", result.StructuredResult)
	}
}

func TestExecuteStructuredOutputMismatchUsesSafeSchemaViolation(t *testing.T) {
	service := mustExecuteService(t, &stubRunner{
		content: `{"answer":1,"rejected":"sensitive-rejected-value"}`,
	}, nil)
	request := validExecuteRequest("dispatch-structured-invalid", "attempt-structured-invalid")
	request.Target.Prompt.OutputSchema = `{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`
	request.Target.Prompt.UserMessage = "sensitive prompt content"

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v, want normalized failed result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		t.Fatalf("result = %#v, want failed result", result)
	}
	if result.Failure.Type != workers.WorkFailureTypeStructuredOutputSchemaViolation || result.Failure.RetryHint {
		t.Fatalf("failure = %#v, want terminal structured schema violation", result.Failure)
	}
	if result.Failure.Detail == nil || !strings.Contains(result.Failure.Detail.Message, "/answer") {
		t.Fatalf("failure detail = %#v, want instance path", result.Failure.Detail)
	}
	for _, secret := range []string{"sensitive-rejected-value", "sensitive prompt content"} {
		if strings.Contains(result.Failure.Detail.Message, secret) {
			t.Fatalf("failure detail = %q, must not expose %q", result.Failure.Detail.Message, secret)
		}
	}
	if result.StructuredResultPresent || result.StructuredResult != nil {
		t.Fatalf("structured result = %#v present=%v, want no invalid result", result.StructuredResult, result.StructuredResultPresent)
	}
}

func TestExecuteNoopAcceptsWithoutRunnerOrObservations(t *testing.T) {
	t.Parallel()

	var runnerCalls atomic.Int32
	var observationCalls atomic.Int32
	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			runnerCalls.Add(1)
			return workers.RunnerExecutionResult{Content: "unexpected"}, nil
		},
	}, func(context.Context, workers.ExecutionObservation) error {
		observationCalls.Add(1)
		return nil
	})

	request := validExecuteRequest("dispatch-noop", "attempt-noop")
	request.Target.RunnerID = workers.RunnerIDCodex
	request.Target.Noop = true
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Correlation != request.Correlation {
		t.Fatalf("correlation = %#v, want %#v", result.Correlation, request.Correlation)
	}
	if len(result.Output.Primary) != 0 {
		t.Fatalf("output = %#v, want empty output", result.Output)
	}
	if runnerCalls.Load() != 0 || observationCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, observation calls = %d, want no calls", runnerCalls.Load(), observationCalls.Load())
	}
}

func TestExecuteScriptProcessFailureRemainsTerminal(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			return workers.RunnerExecutionResult{}, workers.NewProviderError(
				workers.WorkFailureTypeInternalServerError,
				"script process failed",
				errors.New("exit status 1"),
			)
		},
	}, nil)

	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-script-failure", "attempt-script-failure"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want normalized result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		t.Fatalf("result = %#v, want failed result", result)
	}
	if result.Failure.Family != workers.WorkFailureFamilyTerminal || result.Failure.RetryHint {
		t.Fatalf("script failure = %#v, want terminal non-retryable failure", result.Failure)
	}
}

func TestExecuteFailureAndTimeoutEmitExactlyOneTerminalObservation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		runner       workers.Runner
		timeout      time.Duration
		wantFailure  workers.WorkFailureType
		wantTerminal workers.ExecutionObservationKind
	}{
		{
			name: "runner failure",
			runner: &stubRunner{execute: func(
				context.Context,
				workers.RunnerExecutionRequest,
			) (workers.RunnerExecutionResult, error) {
				return workers.RunnerExecutionResult{}, errors.New("provider failed")
			}},
			wantFailure:  workers.WorkFailureTypeUnknown,
			wantTerminal: workers.ExecutionObservationKindFailed,
		},
		{
			name: "deadline",
			runner: &stubRunner{execute: func(
				ctx context.Context,
				_ workers.RunnerExecutionRequest,
			) (workers.RunnerExecutionResult, error) {
				<-ctx.Done()
				return workers.RunnerExecutionResult{}, ctx.Err()
			}},
			timeout:      50 * time.Millisecond,
			wantFailure:  workers.WorkFailureTypeTimeout,
			wantTerminal: workers.ExecutionObservationKindFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var observations []workers.ExecutionObservation
			var observationsMu sync.Mutex
			service := mustExecuteService(t, test.runner, func(
				_ context.Context,
				observation workers.ExecutionObservation,
			) error {
				observationsMu.Lock()
				defer observationsMu.Unlock()
				observations = append(observations, observation.Clone())
				return nil
			})

			request := validExecuteRequest("dispatch-"+test.name, "attempt-"+test.name)
			request.Target.Timeout = test.timeout
			result, err := service.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("Execute() error = %v, want normalized result", err)
			}
			if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
				t.Fatalf("result = %#v, want failed result", result)
			}
			if result.Failure.Type != test.wantFailure {
				t.Fatalf("failure = %#v, want type %q", result.Failure, test.wantFailure)
			}

			observationsMu.Lock()
			defer observationsMu.Unlock()
			if len(observations) != 2 {
				t.Fatalf("observations = %#v, want one start and one terminal observation", observations)
			}
			if observations[0].Kind != workers.ExecutionObservationKindStarted ||
				observations[1].Kind != test.wantTerminal {
				t.Fatalf("observation kinds = %#v, want STARTED then %s", observations, test.wantTerminal)
			}
			if observations[1].Sequence != 2 ||
				observations[1].Correlation != request.Correlation {
				t.Fatalf("terminal observation = %#v, want sequence 2 with request correlation", observations[1])
			}
		})
	}
}

func TestExecuteTimeoutReleasesRequestWorktreeBeforeTerminalObservation(t *testing.T) {
	t.Parallel()

	var eventsMu sync.Mutex
	var events []string
	appendEvent := func(event string) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
	}
	worktree := &recordingWorktree{
		preparation: workers.FactoryWorktreePreparation{CheckoutPath: "C:/fixture/worktree"},
		release: func(context.Context, workers.FactoryWorktreePreparation) error {
			appendEvent("worktree-release")
			return nil
		},
	}
	service := mustExecuteServiceWithEdges(
		t,
		&stubRunner{execute: func(
			ctx context.Context,
			_ workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			<-ctx.Done()
			return workers.RunnerExecutionResult{}, ctx.Err()
		}},
		func(_ context.Context, observation workers.ExecutionObservation) error {
			appendEvent("observation-" + string(observation.Kind))
			return nil
		},
		worktree,
		worktree.Release,
		nil,
	)

	request := validExecuteRequest("dispatch-timeout-cleanup", "attempt-timeout-cleanup")
	request.Target.Timeout = 50 * time.Millisecond
	request.Target.Workspace = workers.WorkspacePolicy{
		PrepareWorktree:    true,
		FactoryDirectory:   "C:/fixture",
		CheckoutIdentifier: "attempt-timeout-cleanup",
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v, want normalized result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil ||
		result.Failure.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("result = %#v, want typed timeout failure", result)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if got, want := events, []string{
		"observation-STARTED",
		"worktree-release",
		"observation-FAILED",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestExecuteServiceRendersDetachedPromptWithoutRuntimeLookup(t *testing.T) {
	t.Parallel()

	service, err := executeservice.New(
		&staticRunners{runner: &stubRunner{}},
		nil,
		nil,
		nil,
		func() time.Time { return time.Unix(10, 0) },
		nil,
		nil,
		nil,
		func(string) (map[string]string, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	rendered, err := service.RenderPrompt(
		"{{ .Context.Project }} / {{ .Context.SessionID }}",
		nil,
		&workers.Context{ProjectID: "project-1", SessionID: "session-1"},
	)
	if err != nil {
		t.Fatalf("RenderPrompt() error = %v", err)
	}
	if rendered != "project-1 / session-1" {
		t.Fatalf("RenderPrompt() = %q, want detached context rendering", rendered)
	}
	var nilService *executeservice.Service
	if _, err := nilService.RenderPrompt("hello", nil, nil); err == nil {
		t.Fatal("nil Service RenderPrompt() error = nil, want unavailable service")
	}
}

func assertAcceptedResult(
	t *testing.T,
	result workers.ExecuteResult,
	dispatchID string,
	attemptID string,
	content string,
) {
	t.Helper()
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Correlation.FactorySessionID != "session-1" ||
		result.Correlation.RuntimeID != "runtime-1" ||
		result.Correlation.GenerationID != "generation-1" ||
		result.Correlation.DispatchID != dispatchID ||
		result.Correlation.AttemptID != attemptID ||
		result.Correlation.RequestID != "request-1" ||
		result.Correlation.TraceID != "trace-1" {
		t.Fatalf("correlation = %#v", result.Correlation)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != content {
		t.Fatalf("output = %#v", result.Output)
	}
}

func assertSafeCompletedObservations(t *testing.T, observations []workers.ExecutionObservation) {
	t.Helper()
	if len(observations) != 2 {
		t.Fatalf("observations = %#v, want started and terminal", observations)
	}
	assertCompletedObservationShape(t, observations)
	for _, observation := range observations {
		assertDetachedObservation(t, observation)
	}
}

func assertCompletedObservationShape(t *testing.T, observations []workers.ExecutionObservation) {
	t.Helper()
	if observations[0].Kind != workers.ExecutionObservationKindStarted {
		t.Fatalf("first observation = %#v", observations[0])
	}
	if observations[1].Kind != workers.ExecutionObservationKindCompleted {
		t.Fatalf("terminal observation = %#v", observations[1])
	}
	if observations[1].Sequence != 2 {
		t.Fatalf("terminal sequence = %d, want 2", observations[1].Sequence)
	}
}

func assertDetachedObservation(t *testing.T, observation workers.ExecutionObservation) {
	t.Helper()
	if observation.Correlation.FactorySessionID != "session-1" ||
		observation.Correlation.RuntimeID != "runtime-1" ||
		observation.Correlation.GenerationID != "generation-1" ||
		observation.Correlation.RequestID != "request-1" ||
		observation.Correlation.TraceID != "trace-1" {
		t.Fatalf("observation correlation = %#v", observation.Correlation)
	}
	for _, value := range observation.Metadata {
		if value == "raw-secret" || value == "secret-system" || value == "secret-user" {
			t.Fatalf("unsafe value persisted in observation: %#v", observation)
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

func TestExecuteForwardsTargetTimeoutToRunner(t *testing.T) {
	t.Parallel()

	captured := make(chan workers.RunnerExecutionRequest, 1)
	service := mustExecuteService(t, &stubRunner{
		execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			captured <- request
			return workers.RunnerExecutionResult{Content: "done"}, nil
		},
	}, nil)
	request := validExecuteRequest("dispatch-timeout", "attempt-timeout")
	request.Target.Timeout = 8 * time.Minute

	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := (<-captured).PrintTimeout; got != 8*time.Minute {
		t.Fatalf("runner PrintTimeout = %s, want 8m", got)
	}
}

func TestExecuteIngressDetachesWorkflowContextBeforeRunnerStarts(t *testing.T) {
	t.Parallel()

	captured := make(chan workers.RunnerExecutionRequest, 1)
	release := make(chan struct{})
	runner := &stubRunner{
		execute: func(
			_ context.Context,
			request workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			snapshot := workers.CloneProviderInferenceRequest(request)
			request.WorkflowContext.EnvVars["TOKEN"] = "runner-mutated"
			captured <- snapshot
			<-release
			return workers.RunnerExecutionResult{Content: "done"}, nil
		},
	}
	service := mustExecuteService(t, runner, nil)
	request := validExecuteRequest("dispatch-context", "attempt-context")
	request.Input.WorkflowContext = &workers.Context{
		FactoryDirectory: "factory-original",
		WorkDirectory:    "work-original",
		EnvVars:          map[string]string{"TOKEN": "original"},
		ArtifactDir:      "artifacts-original",
		ProjectID:        "project-original",
		SessionID:        "session-1",
	}

	done := make(chan workers.ExecuteResult, 1)
	go func() {
		result, err := service.Execute(context.Background(), request)
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
		done <- result
	}()

	got := <-captured
	request.Input.WorkflowContext.EnvVars["TOKEN"] = "caller-mutated"
	close(release)
	<-done

	if got.WorkflowContext == nil || got.WorkflowContext.EnvVars["TOKEN"] != "original" {
		t.Fatalf("runner workflow context = %#v, want detached original", got.WorkflowContext)
	}
	if request.Input.WorkflowContext.EnvVars["TOKEN"] != "caller-mutated" {
		t.Fatalf("caller workflow context = %#v, want caller mutation isolated", request.Input.WorkflowContext)
	}
}

func TestExecuteRejectsConflictingDetachedIdentityBeforeEffects(t *testing.T) {
	t.Parallel()

	var runnerCalls atomic.Int32
	var observationCalls atomic.Int32
	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			runnerCalls.Add(1)
			return workers.RunnerExecutionResult{Content: "should-not-run"}, nil
		},
	}, func(context.Context, workers.ExecutionObservation) error {
		observationCalls.Add(1)
		return nil
	})
	request := validExecuteRequest("dispatch-identity", "attempt-identity")
	request.Input.Dispatch.DispatchID = "different-dispatch"

	_, err := service.Execute(context.Background(), request)
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidExecuteRequest", err)
	}
	if runnerCalls.Load() != 0 || observationCalls.Load() != 0 {
		t.Fatalf("invalid request effects = runner %d observations %d, want zero", runnerCalls.Load(), observationCalls.Load())
	}

	request = validExecuteRequest("dispatch-missing-generation", "attempt-missing-generation")
	request.Correlation.GenerationID = ""
	_, err = service.Execute(context.Background(), request)
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("missing generation Execute() error = %v, want ErrInvalidExecuteRequest", err)
	}
	if runnerCalls.Load() != 0 || observationCalls.Load() != 0 {
		t.Fatalf("missing identity effects = runner %d observations %d, want zero", runnerCalls.Load(), observationCalls.Load())
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
		func() time.Time { return time.Unix(10, 0) },
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

func TestExecuteCancellationReachesRunnerAndEmitsOneCanceledTerminalObservation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	runner := &stubRunner{
		execute: func(ctx context.Context, _ workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			close(started)
			<-ctx.Done()
			return workers.RunnerExecutionResult{}, ctx.Err()
		},
	}
	var observationsMu sync.Mutex
	var observations []workers.ExecutionObservation
	service := mustExecuteService(t, runner, func(
		ctx context.Context,
		observation workers.ExecutionObservation,
	) error {
		if ctx.Err() != nil {
			t.Errorf("observation context error = %v, want detached context", ctx.Err())
		}
		observationsMu.Lock()
		defer observationsMu.Unlock()
		observations = append(observations, observation.Clone())
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan workers.ExecuteResult, 1)
	go func() {
		result, err := service.Execute(ctx, validExecuteRequest("dispatch-cancel", "attempt-cancel"))
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
		done <- result
	}()
	<-started
	cancel()

	select {
	case result := <-done:
		if result.Outcome != workers.ExecutionOutcomeCanceled {
			t.Fatalf("outcome = %q, want CANCELED", result.Outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute() did not return after runner observed cancellation")
	}

	observationsMu.Lock()
	defer observationsMu.Unlock()
	if len(observations) != 2 {
		t.Fatalf("observations = %#v, want started and canceled terminal", observations)
	}
	if observations[0].Kind != workers.ExecutionObservationKindStarted ||
		observations[1].Kind != workers.ExecutionObservationKindCanceled {
		t.Fatalf("observation kinds = %#v, want STARTED then CANCELED", observations)
	}
	if observations[1].Sequence != 2 || observations[1].Correlation.DispatchID != "dispatch-cancel" {
		t.Fatalf("terminal observation = %#v, want sequence 2 with correlation", observations[1])
	}
}

func TestExecuteCanceledBeforeStartReturnsCanonicalContextError(t *testing.T) {
	t.Parallel()

	service := mustExecuteService(t, &stubRunner{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Execute(ctx, validExecuteRequest("dispatch-before-cancel", "attempt-before-cancel"))
	if err != context.Canceled {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestExecuteRunnerPanicBecomesSafeFailedResult(t *testing.T) {
	t.Parallel()

	const secret = "panic-secret-value"
	service := mustExecuteService(t, &stubRunner{
		execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			panic(secret)
		},
	}, nil)

	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-panic", "attempt-panic"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want normalized result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if result.Failure == nil || result.Failure.Message != "worker runner panicked" {
		t.Fatalf("failure = %#v, want stable panic failure", result.Failure)
	}
	if result.Diagnostics == nil || result.Diagnostics.Panic == nil {
		t.Fatalf("diagnostics = %#v, want safe panic diagnostics", result.Diagnostics)
	}
	if strings.Contains(result.Failure.Message, secret) ||
		strings.Contains(result.Diagnostics.Panic.Message, secret) ||
		strings.Contains(result.Diagnostics.Panic.Stack, secret) {
		t.Fatalf("panic secret escaped safe result: %#v", result)
	}
}

func TestExecuteCleanupRunsBeforeTerminalAndCleanupFailureNormalizesResult(t *testing.T) {
	t.Parallel()

	cleanupError := errors.New("temporary cleanup failed")
	var eventsMu sync.Mutex
	var events []string
	appendEvent := func(event string) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
	}
	worktree := &recordingWorktree{
		preparation: workers.FactoryWorktreePreparation{CheckoutPath: "C:/fixture/worktree"},
		release: func(context.Context, workers.FactoryWorktreePreparation) error {
			appendEvent("worktree-release")
			return nil
		},
	}
	temporaryFiles := &recordingTemporaryFiles{
		remove: func(string) error {
			appendEvent("temporary-cleanup")
			return cleanupError
		},
	}
	service := mustExecuteServiceWithEdges(
		t,
		&stubRunner{
			content: "output",
			execute: func(ctx context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				file, err := request.TemporaryFiles.CreateTemp("", "attempt-*")
				if err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				if err := file.Close(); err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				return workers.RunnerExecutionResult{Content: "output"}, nil
			},
		},
		func(_ context.Context, observation workers.ExecutionObservation) error {
			appendEvent("observation-" + string(observation.Kind))
			return nil
		},
		worktree,
		worktree.Release,
		temporaryFiles,
	)
	request := validExecuteRequest("dispatch-cleanup", "attempt-cleanup")
	request.Target.Workspace = workers.WorkspacePolicy{
		PrepareWorktree:    true,
		FactoryDirectory:   "C:/fixture",
		CheckoutIdentifier: "attempt-cleanup",
	}

	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v, want normalized result", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		t.Fatalf("result = %#v, want cleanup FAILED result", result)
	}
	if result.Failure.Type != workers.WorkFailureTypeInternalServerError ||
		result.Failure.Message != "execution cleanup failed" {
		t.Fatalf("failure = %#v, want typed cleanup failure", result.Failure)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if got, want := events, []string{
		"observation-STARTED",
		"worktree-release",
		"temporary-cleanup",
		"observation-FAILED",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestExecuteTracksOnlyTemporaryFilesCreatedByTheAttempt(t *testing.T) {
	t.Parallel()

	temporaryFiles := &recordingTemporaryFiles{}
	service := mustExecuteServiceWithEdges(
		t,
		&stubRunner{
			execute: func(_ context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				first, err := request.TemporaryFiles.CreateTemp("", "first")
				if err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				if err := first.Close(); err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				second, err := request.TemporaryFiles.CreateTemp("", "second")
				if err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				if err := second.Close(); err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				if err := request.TemporaryFiles.Remove("attempt-temp-1"); err != nil {
					return workers.RunnerExecutionResult{}, err
				}
				return workers.RunnerExecutionResult{Content: "output"}, nil
			},
		},
		nil,
		nil,
		nil,
		temporaryFiles,
	)

	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-temp", "attempt-temp"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if got, want := temporaryFiles.Removed(), []string{"attempt-temp-1", "attempt-temp-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed temporary paths = %#v, want %#v", got, want)
	}
}

func TestExecuteObservationSinkFailureDoesNotChangeResult(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	service := mustExecuteService(t, &stubRunner{content: "accepted"}, func(
		ctx context.Context,
		_ workers.ExecutionObservation,
	) error {
		if ctx.Err() != nil {
			t.Errorf("observation context error = %v, want nil", ctx.Err())
		}
		calls.Add(1)
		if calls.Load() == 1 {
			return errors.New("observation sink failed")
		}
		panic("observation sink panic")
	})

	result, err := service.Execute(context.Background(), validExecuteRequest("dispatch-observation", "attempt-observation"))
	if err != nil {
		t.Fatalf("Execute() error = %v, want best-effort observation policy", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if calls.Load() != 2 {
		t.Fatalf("observation sink calls = %d, want started and terminal", calls.Load())
	}
}
