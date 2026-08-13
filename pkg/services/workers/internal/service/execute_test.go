package service_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
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
	assertAcceptedResult(t, result, "dispatch-1", "attempt-1", "accepted-output")

	observationsMu.Lock()
	defer observationsMu.Unlock()
	assertSafeCompletedObservations(t, observations)
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
	if result.Correlation.DispatchID != dispatchID || result.Correlation.AttemptID != attemptID {
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
		worktree,
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

func mustExecuteService(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
) *executeservice.Service {
	return mustExecuteServiceWithEdges(t, runner, observe, nil, nil, nil)
}

func mustExecuteServiceWithEdges(
	t *testing.T,
	runner workers.Runner,
	observe workers.ObservationSink,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease workers.FactoryWorktreeReleaser,
	temporaryFiles workers.TemporaryFileSystem,
) *executeservice.Service {
	t.Helper()
	service, err := executeservice.New(
		&staticRunners{runner: runner},
		nil,
		observe,
		nil,
		func() time.Time { return time.Unix(10, 0) },
		worktree,
		worktreeRelease,
		temporaryFiles,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

type recordingWorktree struct {
	preparation workers.FactoryWorktreePreparation
	release     func(context.Context, workers.FactoryWorktreePreparation) error
}

func (worktree *recordingWorktree) Prepare(
	context.Context,
	string,
	string,
) (workers.FactoryWorktreePreparation, error) {
	return worktree.preparation, nil
}

func (worktree *recordingWorktree) Release(
	ctx context.Context,
	preparation workers.FactoryWorktreePreparation,
) error {
	if worktree.release == nil {
		return nil
	}
	return worktree.release(ctx, preparation)
}

type recordingTemporaryFiles struct {
	mu      sync.Mutex
	next    int
	removed []string
	remove  func(string) error
}

func (files *recordingTemporaryFiles) CreateTemp(_, _ string) (workers.TemporaryFile, error) {
	files.mu.Lock()
	defer files.mu.Unlock()
	files.next++
	return &recordingTemporaryFile{name: "attempt-temp-" + strconv.Itoa(files.next)}, nil
}

func (files *recordingTemporaryFiles) Remove(path string) error {
	files.mu.Lock()
	files.removed = append(files.removed, path)
	files.mu.Unlock()
	if files.remove == nil {
		return nil
	}
	return files.remove(path)
}

func (files *recordingTemporaryFiles) Removed() []string {
	files.mu.Lock()
	defer files.mu.Unlock()
	return append([]string(nil), files.removed...)
}

type recordingTemporaryFile struct {
	name string
}

func (file *recordingTemporaryFile) Name() string {
	return file.name
}

func (*recordingTemporaryFile) WriteString(value string) (int, error) {
	return len(value), nil
}

func (*recordingTemporaryFile) Close() error {
	return nil
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
