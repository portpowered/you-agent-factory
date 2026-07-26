package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

type recordingExecutor struct {
	requests []workers.WorkstationExecutionRequest
	result   workers.WorkResult
	err      error
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (executor *blockingExecutor) Execute(
	ctx context.Context,
	_ workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	close(executor.started)
	select {
	case <-executor.release:
		return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
}

func (executor *recordingExecutor) Execute(
	_ context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	executor.requests = append(executor.requests, request)
	return executor.result, executor.err
}

func TestPoolLifecycleMakesOnlyStartedRoutesAvailable(t *testing.T) {
	t.Parallel()

	pool := New()
	if err := pool.Route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed route error = %v, want ErrWorkstationPoolUnavailable", err)
	}

	outcome, err := pool.Start(context.Background(), []workstations.Route{
		{WorkstationName: "review"},
		{WorkstationName: "implement"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("Start() outcome = %q, want STARTED", outcome)
	}
	if err := pool.Route(context.Background(), "review"); err != nil {
		t.Fatalf("started route error = %v", err)
	}
	if err := pool.Route(context.Background(), "missing"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("unknown route error = %v, want ErrUnknownWorkstationRoute", err)
	}

	outcome, err = pool.Start(context.Background(), []workstations.Route{{WorkstationName: "other"}})
	if err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyRunning {
		t.Fatalf("repeated Start() outcome = %q, want ALREADY_RUNNING", outcome)
	}
	if err := pool.Route(context.Background(), "other"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("repeated start replaced routes: error = %v", err)
	}

	outcome, err = pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	if err := pool.Route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("stopped route error = %v, want ErrWorkstationPoolStopped", err)
	}
	if _, err := pool.Start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Start() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func TestPoolStopIsRepeatSafeUnderConcurrentCalls(t *testing.T) {
	t.Parallel()

	pool := New()
	if _, err := pool.Start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	const callers = 32
	outcomes := make(chan workers.WorkstationPoolLifecycleOutcome, callers)
	errorsCh := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			outcome, err := pool.Stop(context.Background())
			outcomes <- outcome
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(outcomes)
	close(errorsCh)

	stopped := 0
	alreadyStopped := 0
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Stop() error = %v", err)
		}
	}
	for outcome := range outcomes {
		switch outcome {
		case workers.WorkstationPoolLifecycleOutcomeStopped:
			stopped++
		case workers.WorkstationPoolLifecycleOutcomeAlreadyStopped:
			alreadyStopped++
		default:
			t.Fatalf("concurrent Stop() outcome = %q", outcome)
		}
	}
	if stopped != 1 || alreadyStopped != callers-1 {
		t.Fatalf("stop outcomes = stopped:%d already:%d", stopped, alreadyStopped)
	}
}

func TestPoolRejectsInvalidRoutesAndCancelledLifecycleCalls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		routes []workstations.Route
	}{
		{name: "empty"},
		{name: "blank", routes: []workstations.Route{{WorkstationName: " "}}},
		{
			name: "duplicate",
			routes: []workstations.Route{
				{WorkstationName: "review"},
				{WorkstationName: " review "},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New().Start(context.Background(), testCase.routes); !errors.Is(err, workers.ErrInvalidWorkstationPoolStart) {
				t.Fatalf("Start() error = %v, want ErrInvalidWorkstationPoolStart", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := New()
	if _, err := pool.Start(ctx, []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Start() error = %v, want context.Canceled", err)
	}
	if _, err := pool.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Stop() error = %v, want context.Canceled", err)
	}
}

func TestPoolCanStopBeforeStartWithoutActivating(t *testing.T) {
	t.Parallel()

	pool := New()
	outcome, err := pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	outcome, err = pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("repeated Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyStopped {
		t.Fatalf("repeated Stop() outcome = %q, want ALREADY_STOPPED", outcome)
	}
}

func TestPoolDispatchesToImmutableRouteBindingWithDetachedAttribution(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{result: workers.WorkResult{
		DispatchID:   "wrong-dispatch",
		TransitionID: "wrong-transition",
		Outcome:      workers.OutcomeAccepted,
		Output:       "done",
	}}
	pool := New()
	if _, err := pool.Start(context.Background(), []workstations.Route{
		{
			WorkstationName: "review",
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDCodex,
				Source:   workers.RunnerSelectionSourceWorkstation,
			},
			Executor: executor,
		},
		{
			WorkstationName: "implement",
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDGemini,
				Source:   workers.RunnerSelectionSourceFactory,
			},
			Executor: executor,
		},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	request := dispatchRequest("dispatch-review", "transition-review", "review")
	wantExecution := workers.CloneWorkstationExecutionRequest(request.Execution)
	wantExecution.RunnerID = workers.RunnerIDCodex
	wantExecution.RunnerSelectionSource = workers.RunnerSelectionSourceWorkstation
	result, err := pool.Dispatch(context.Background(), request)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	assertExecutionRequests(t, executor.requests, wantExecution)
	assertDispatchResult(
		t,
		result,
		"dispatch-review",
		"transition-review",
		"review",
		workers.OutcomeAccepted,
		"done",
	)

	request.Execution.InputTokens[0] = "mutated-after-call"
	if executor.requests[0].InputTokens[0] != "input-token" {
		t.Fatalf("executor request aliases caller input: %#v", executor.requests[0])
	}

	second := dispatchRequest("dispatch-implement", "transition-implement", "implement")
	if _, err := pool.Dispatch(context.Background(), second); err != nil {
		t.Fatalf("second Dispatch() error = %v", err)
	}
	if got := executor.requests[1]; got.RunnerID != workers.RunnerIDGemini ||
		got.RunnerSelectionSource != workers.RunnerSelectionSourceFactory ||
		got.Dispatch.WorkstationName != "implement" {
		t.Fatalf("second executor request = %#v", got)
	}
}

func assertExecutionRequests(
	t *testing.T,
	got []workers.WorkstationExecutionRequest,
	want workers.WorkstationExecutionRequest,
) {
	t.Helper()
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("executor requests = %#v, want %#v", got, want)
	}
}

func assertDispatchResult(
	t *testing.T,
	got workers.WorkstationDispatchResult,
	dispatchID string,
	transitionID string,
	workstationName string,
	outcome workers.WorkOutcome,
	output string,
) {
	t.Helper()
	if got.DispatchID != dispatchID ||
		got.WorkstationName != workstationName ||
		got.Result.DispatchID != dispatchID ||
		got.Result.TransitionID != transitionID ||
		got.Result.Outcome != outcome ||
		got.Result.Output != output {
		t.Fatalf("Dispatch() result = %#v", got)
	}
}

func TestPoolDispatchReturnsTypedRoutingFailuresWithoutExecutorEntry(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{}
	pool := New()
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-1", "transition-1", "review"),
	); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed Dispatch() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := pool.Start(context.Background(), []workstations.Route{
		{WorkstationName: "review", Executor: executor},
		{WorkstationName: "unbound"},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	testCases := []struct {
		name    string
		request workers.WorkstationDispatchRequest
		want    error
	}{
		{
			name:    "unknown route",
			request: dispatchRequest("dispatch-1", "transition-1", "missing"),
			want:    workers.ErrUnknownWorkstationRoute,
		},
		{
			name:    "missing binding",
			request: dispatchRequest("dispatch-2", "transition-2", "unbound"),
			want:    workers.ErrMissingWorkstationBinding,
		},
		{
			name:    "missing dispatch identity",
			request: dispatchRequest("", "transition-3", "review"),
			want:    workers.ErrInvalidWorkstationDispatch,
		},
		{
			name: "mismatched workstation identity",
			request: workers.WorkstationDispatchRequest{
				WorkstationName: "review",
				Execution: workers.WorkstationExecutionRequest{
					Dispatch: work.WorkDispatch{
						DispatchID:      "dispatch-4",
						WorkstationName: "implement",
					},
				},
			},
			want: workers.ErrInvalidWorkstationDispatch,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := pool.Dispatch(context.Background(), testCase.request)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Dispatch() error = %v, want %v", err, testCase.want)
			}
			if !reflect.DeepEqual(result, workers.WorkstationDispatchResult{}) {
				t.Fatalf("Dispatch() result = %#v, want empty", result)
			}
		})
	}
	if len(executor.requests) != 0 {
		t.Fatalf("executor calls = %d, want zero", len(executor.requests))
	}
}

func TestPoolDispatchReturnsAttributedResultWithExecutorFailure(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("executor failed")
	executor := &recordingExecutor{err: executeErr}
	pool := New()
	if _, err := pool.Start(context.Background(), []workstations.Route{{
		WorkstationName: "review",
		Executor:        executor,
	}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-failed", "transition-failed", "review"),
	)
	if !errors.Is(err, executeErr) {
		t.Fatalf("Dispatch() error = %v, want executor failure", err)
	}
	if result.DispatchID != "dispatch-failed" ||
		result.WorkstationName != "review" ||
		result.Result.DispatchID != "dispatch-failed" ||
		result.Result.TransitionID != "transition-failed" {
		t.Fatalf("Dispatch() result = %#v", result)
	}
	if len(executor.requests) != 1 {
		t.Fatalf("executor calls = %d, want one", len(executor.requests))
	}
}

func TestPoolStopWaitsForAcceptedDispatchAndRejectsLaterDispatch(t *testing.T) {
	t.Parallel()

	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	pool := New()
	if _, err := pool.Start(context.Background(), []workstations.Route{{
		WorkstationName: "review",
		Executor:        executor,
	}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := pool.Dispatch(
			context.Background(),
			dispatchRequest("dispatch-running", "transition-running", "review"),
		)
		dispatchDone <- err
	}()
	<-executor.started

	stopDone := make(chan error, 1)
	go func() {
		_, err := pool.Stop(context.Background())
		stopDone <- err
	}()
	select {
	case err := <-stopDone:
		t.Fatalf("Stop() returned before accepted dispatch completed: %v", err)
	default:
	}

	close(executor.release)
	if err := <-dispatchDone; err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-late", "transition-late", "review"),
	); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Dispatch() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func dispatchRequest(
	dispatchID string,
	transitionID string,
	workstationName string,
) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: workstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:               dispatchID,
				TransitionID:             transitionID,
				WorkerType:               "selected-worker",
				WorkstationName:          workstationName,
				ProjectID:                "project-1",
				CurrentChainingTraceID:   "trace-current",
				PreviousChainingTraceIDs: []string{"trace-previous"},
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: []any{"dispatch-token"},
				InputBindings: map[string][]string{
					"source": {"work-1"},
				},
			},
			WorkerType:         "selected-worker",
			WorkstationType:    "INFERENCE_RUN",
			ProjectID:          "project-1",
			FactorySessionID:   "session-1",
			InputTokens:        []any{"input-token"},
			ModelOperation:     "summarize",
			SystemPrompt:       "system",
			UserMessage:        "user",
			OutputSchema:       `{"type":"string"}`,
			EnvVars:            map[string]string{"KEY": "value"},
			ProcessEnvironment: []string{"PATH=value"},
			Worktree:           "worktree",
			WorkingDirectory:   "working-directory",
		},
	}
}
