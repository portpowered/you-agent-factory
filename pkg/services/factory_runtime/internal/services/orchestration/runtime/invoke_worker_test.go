package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type attemptExecuteFunc func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)

func (fn attemptExecuteFunc) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return fn(ctx, request)
}

func TestAttemptLifecycleBoundsCapacityAndCleansActiveState(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan workers.ExecuteResult, 1)
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		started <- struct{}{}
		<-release
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	ids := 0
	lifecycle := newAttemptLifecycle(service, func() string {
		ids++
		return "attempt-" + string(rune('0'+ids))
	}, 1)
	terminal := func(_ context.Context, request workers.ExecuteRequest, result workers.ExecuteResult, err error) {
		if err != nil {
			t.Errorf("terminal error = %v", err)
		}
		if request.Correlation.DispatchID != result.Correlation.DispatchID {
			t.Errorf("terminal correlation = %#v, result = %#v", request.Correlation, result.Correlation)
		}
		results <- result
	}

	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), true, terminal); err != nil {
		t.Fatalf("start(first) error = %v", err)
	}
	<-started
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-2", ""), true, terminal); !errors.Is(err, ErrAttemptCapacityExceeded) {
		t.Fatalf("start(second) error = %v, want capacity error", err)
	}
	if got := lifecycle.activeCount(); got != 1 {
		t.Fatalf("active count while first is blocked = %d, want 1", got)
	}
	close(release)
	result := <-results
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("result outcome = %q", result.Outcome)
	}
	if got := lifecycle.activeCount(); got != 0 {
		t.Fatalf("active count after completion = %d, want 0", got)
	}
	if attemptID, ok := lifecycle.terminalAttemptID("dispatch-1"); !ok || attemptID != "attempt-1" {
		t.Fatalf("terminal attempt = %q, %v; want attempt-1, true", attemptID, ok)
	}
}

func TestAttemptLifecycleCancellationIsPerDispatch(t *testing.T) {
	started := make(chan string, 2)
	cancelObserved := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	results := make(chan workers.ExecuteResult, 2)
	service := attemptExecuteFunc(func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		started <- request.Correlation.DispatchID
		if request.Correlation.DispatchID == "dispatch-1" {
			<-ctx.Done()
			close(cancelObserved)
			<-releaseFirst
			return workers.ExecuteResult{}, ctx.Err()
		}
		<-releaseSecond
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	sequence := 0
	lifecycle := newAttemptLifecycle(service, func() string {
		sequence++
		return "attempt-" + string(rune('0'+sequence))
	}, 2)
	terminal := func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, err error) {
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("terminal error = %v", err)
		}
		results <- result
	}
	requireAttemptStart(t, lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), true, terminal), "first")
	requireAttemptStart(t, lifecycle.start(context.Background(), attemptTestRequest("dispatch-2", ""), true, terminal), "second")
	startedDispatches := map[string]bool{<-started: true, <-started: true}
	if !startedDispatches["dispatch-1"] || !startedDispatches["dispatch-2"] {
		t.Fatalf("started dispatches = %#v", startedDispatches)
	}
	requireCancelOutcome(t, lifecycle, "dispatch-1", workers.WorkstationDispatchCancelOutcomeCanceled, "cancel(first)")
	<-cancelObserved
	requireCancelOutcome(t, lifecycle, "dispatch-1", workers.WorkstationDispatchCancelOutcomeAlreadyCanceled, "repeat cancel(first)")
	close(releaseFirst)
	requireAttemptOutcome(t, <-results, workers.ExecutionOutcomeCanceled, "canceled result")
	requireAttemptCount(t, lifecycle, 1, "after first cancellation")
	close(releaseSecond)
	requireAttemptOutcome(t, <-results, workers.ExecutionOutcomeAccepted, "second result")
	requireAttemptCount(t, lifecycle, 0, "after second completion")
	requireCancelOutcome(t, lifecycle, "dispatch-1", workers.WorkstationDispatchCancelOutcomeAlreadyTerminal, "late cancel(first)")
}

func requireAttemptStart(t *testing.T, err error, label string) {
	t.Helper()
	if err != nil {
		t.Fatalf("start(%s) error = %v", label, err)
	}
}

func requireAttemptOutcome(t *testing.T, result workers.ExecuteResult, want workers.ExecutionOutcome, label string) {
	t.Helper()
	if result.Outcome != want {
		t.Fatalf("%s outcome = %q, want %q", label, result.Outcome, want)
	}
}

func requireAttemptCount(t *testing.T, lifecycle *attemptLifecycle, want int, label string) {
	t.Helper()
	if got := lifecycle.activeCount(); got != want {
		t.Fatalf("active count %s = %d, want %d", label, got, want)
	}
}

func requireCancelOutcome(
	t *testing.T,
	lifecycle *attemptLifecycle,
	dispatchID string,
	want workers.WorkstationDispatchCancelOutcome,
	label string,
) {
	t.Helper()
	outcome, err := lifecycle.cancel(context.Background(), dispatchID)
	if err != nil || outcome != want {
		t.Fatalf("%s = %q, %v; want %q", label, outcome, err, want)
	}
}

func TestAttemptLifecycleCancellationWinsACompletionRaceAfterCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan workers.ExecuteResult, 1)
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		<-release
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-race" }, 1)
	if err := lifecycle.start(
		context.Background(), attemptTestRequest("dispatch-race", ""), true,
		func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, _ error) {
			results <- result
		},
	); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	<-started
	if outcome, err := lifecycle.cancel(context.Background(), "dispatch-race"); err != nil || outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("cancel() = %q, %v", outcome, err)
	}
	close(release)
	if result := <-results; result.Outcome != workers.ExecutionOutcomeCanceled {
		t.Fatalf("racing completion outcome = %q, want CANCELED", result.Outcome)
	}
}

func TestAttemptLifecyclePanicAndDuplicateTerminalAreExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	callbackCount := 0
	service := attemptExecuteFunc(func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
		panic("boom")
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-1" }, 1)
	terminal := func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, _ error) {
		mu.Lock()
		defer mu.Unlock()
		callbackCount++
		if result.Outcome != workers.ExecutionOutcomeFailed {
			t.Errorf("panic result outcome = %q", result.Outcome)
		}
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", ""), false, terminal); err != nil {
		t.Fatalf("start(panic) error = %v", err)
	}
	if err := lifecycle.start(context.Background(), attemptTestRequest("dispatch-1", "attempt-late"), false, terminal); err != nil {
		t.Fatalf("duplicate start error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callbackCount != 1 {
		t.Fatalf("terminal callback count = %d, want 1", callbackCount)
	}
	if got := lifecycle.activeCount(); got != 0 {
		t.Fatalf("active count after panic = %d, want 0", got)
	}
}

func attemptTestRequest(dispatchID, attemptID string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-test",
			RuntimeID:        "runtime-test",
			GenerationID:     "generation-test",
			DispatchID:       dispatchID,
			AttemptID:        attemptID,
		},
		Target: workers.ExecutionTarget{RunnerID: workers.RunnerIDCodex},
	}
}
func TestStartThroughStatelessWorkersBuildsCorrelatedDetachedRequest(t *testing.T) {
	var observed workers.ExecuteRequest
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		observed = request
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
			Output: workers.ProposedOutput{
				Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
				ProposedWork: []workers.ProposedWork{{
					WorkTypeID: "review",
					Name:       "review-1",
					State:      "init",
				}},
			},
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: service,
		newID:          func() string { return "attempt-1" },
		attempts:       newAttemptLifecycle(service, func() string { return "attempt-1" }, 1),
		inlineDispatch: true,
		workflowContext: &workers.Context{
			FactoryDirectory: "factory-1",
			WorkDirectory:    "work-1",
			EnvVars:          map[string]string{"SESSION_VALUE": "session-1"},
			ProjectID:        "project-1",
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "workstation-a",
		Execution: workers.WorkstationExecutionRequest{
			WorkerName:       "worker-a",
			WorkerType:       "agent",
			RunnerID:         workers.RunnerIDCodex,
			FactorySessionID: "session-1",
			RecordingID:      "runtime-1",
			GenerationID:     "generation-1",
			Model:            "model-a",
			ModelProvider:    "provider-a",
			Dispatch: work.WorkDispatch{
				DispatchID:      "dispatch-1",
				TransitionID:    "transition-a",
				WorkerType:      "agent",
				WorkstationName: "workstation-a",
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: workers.InputTokens(workers.Token{Color: workers.Color{
					WorkID:     "work-1",
					WorkTypeID: "task",
					RequestID:  "request-1",
					DataType:   workers.DataTypeWork,
					TraceID:    "trace-1",
					Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "input",
					}},
				}}),
			},
		},
	}
	var got workers.WorkstationDispatchResult
	var gotErr error
	if err := startThroughStatelessWorkers(
		context.Background(), cfg, request,
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, err error) {
			got, gotErr = result, err
		},
	); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	assertDetachedDispatchResult(t, got, gotErr)
	assertDetachedCorrelation(t, observed)
	assertDetachedWorkInput(t, observed)
	if observed.Input.WorkflowContext == nil ||
		observed.Input.WorkflowContext.SessionID != "session-1" ||
		observed.Input.WorkflowContext.FactoryDirectory != "factory-1" ||
		observed.Input.WorkflowContext.WorkDirectory != "work-1" ||
		observed.Input.WorkflowContext.EnvVars["SESSION_VALUE"] != "session-1" ||
		observed.Input.WorkflowContext.ProjectID != "project-1" {
		t.Fatalf("workflow context = %#v, want detached session context", observed.Input.WorkflowContext)
	}
	observed.Input.WorkflowContext.EnvVars["SESSION_VALUE"] = "mutated"
	if cfg.workflowContext.EnvVars["SESSION_VALUE"] != "session-1" {
		t.Fatalf("runtime workflow context was mutated through Execute request: %#v", cfg.workflowContext)
	}
}

func assertDetachedDispatchResult(t *testing.T, got workers.WorkstationDispatchResult, gotErr error) {
	t.Helper()
	if gotErr != nil {
		t.Fatalf("dispatch callback error = %v", gotErr)
	}
	if got.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted || got.Result.Output != "done" {
		t.Fatalf("dispatch result = %#v", got)
	}
	if got.ProposedOutput == nil || len(got.ProposedOutput.ProposedWork) != 1 ||
		got.ProposedOutput.ProposedWork[0].Name != "review-1" {
		t.Fatalf("detached proposed output = %#v", got.ProposedOutput)
	}
}

func assertDetachedCorrelation(t *testing.T, observed workers.ExecuteRequest) {
	t.Helper()
	if observed.Correlation.DispatchID != "dispatch-1" || observed.Correlation.AttemptID != "dispatch-1" {
		t.Fatalf("correlation = %#v", observed.Correlation)
	}
	if observed.Correlation.FactorySessionID != "session-1" ||
		observed.Correlation.RuntimeID != "runtime-1" ||
		observed.Correlation.GenerationID != "generation-1" ||
		observed.Correlation.RequestID != "request-1" ||
		observed.Correlation.TraceID != "trace-1" {
		t.Fatalf("correlation lineage = %#v", observed.Correlation)
	}
}

func TestStartThroughStatelessWorkersUsesRuntimeIdentityWhenRecordingIsDisabled(t *testing.T) {
	var observed workers.ExecuteRequest
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		observed = request
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	})
	cfg := &runtimeConfig{
		executeService: service,
		attempts:       newAttemptLifecycle(service, func() string { return "attempt-config" }, 1),
		newID:          func() string { return "attempt-config" },
		inlineDispatch: true,
		runtimeID:      "runtime-config",
		eventHistory:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "generation-config"},
		clock:          testRuntimeClock{},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			FactorySessionID: "session-config",
			RunnerID:         workers.RunnerIDCodex,
			Dispatch: work.WorkDispatch{
				DispatchID:   "dispatch-config",
				TransitionID: "review",
				Execution:    work.ExecutionMetadata{RequestID: "request-config"},
			},
		},
	}
	if err := startThroughStatelessWorkers(context.Background(), cfg, request, nil); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	if observed.Correlation.FactorySessionID != "session-config" ||
		observed.Correlation.RuntimeID != "runtime-config" ||
		observed.Correlation.GenerationID != "generation-config" ||
		observed.Correlation.DispatchID != "dispatch-config" ||
		observed.Correlation.AttemptID != "dispatch-config" {
		t.Fatalf("runtime correlation = %#v, want explicit process/runtime identity", observed.Correlation)
	}
}

func TestDetachedResultMaterializationPreservesFailureAndContinuationFacts(t *testing.T) {
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:   "logical-dispatch/attempt/2",
				TransitionID: "review",
			},
		},
	}
	result, err := workstationDispatchResultFromExecute(request, workers.ExecuteResult{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "logical-dispatch",
			AttemptID:  "logical-dispatch/attempt/2",
		},
		Outcome: workers.ExecutionOutcomeFailed,
		Output: workers.ProposedOutput{
			Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial output"}},
		},
		Failure: &workers.ExecutionFailure{
			Family:  workers.WorkFailureFamilyRetryable,
			Type:    workers.WorkFailureTypeTimeout,
			Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "provider timed out"},
			Message: "provider timed out",
		},
		Continuation: &workers.ProviderContinuationRef{
			Provider:          "codex",
			ProviderSessionID: "provider-session-retry",
		},
		Diagnostics: &workers.SafeDiagnostics{
			Provider: &workers.SafeProviderDiagnostic{
				Provider: "codex",
				Model:    "fixture-model",
				ResponseMetadata: map[string]string{
					"duration_ms": "12",
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("workstationDispatchResultFromExecute() error = %v", err)
	}
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.Result.Outcome != workers.OutcomeFailed {
		t.Fatalf("terminal result = %#v, want failed", result)
	}
	if result.Result.DispatchID != request.Execution.Dispatch.DispatchID {
		t.Fatalf("physical result dispatch ID = %q, want %q", result.Result.DispatchID, request.Execution.Dispatch.DispatchID)
	}
	if result.Result.Error != "provider timed out" || result.Result.FailureMetadata == nil ||
		result.Result.FailureMetadata.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("failure facts = %#v, want timeout metadata", result.Result)
	}
	if result.Result.ProviderSession == nil || result.Result.ProviderSession.ID != "provider-session-retry" {
		t.Fatalf("provider session = %#v, want continuation identity", result.Result.ProviderSession)
	}
	if result.Result.Diagnostics == nil || result.Result.Diagnostics.Provider == nil ||
		result.Result.Diagnostics.Provider.ResponseMetadata["duration_ms"] != "12" {
		t.Fatalf("diagnostics = %#v, want safe provider metadata", result.Result.Diagnostics)
	}
}

func TestDetachedScriptProcessFailureDoesNotPropagateRetryMetadata(t *testing.T) {
	t.Parallel()
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "script-station",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-script-failure"},
		},
	}
	result, err := workstationDispatchResultFromExecute(request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Family:  workers.WorkFailureFamilyTerminal,
			Type:    workers.WorkFailureTypeInternalServerError,
			Message: "script exited with status 1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("workstationDispatchResultFromExecute() error = %v", err)
	}
	if result.Result.FailureMetadata != nil {
		t.Fatalf("failure metadata = %#v, want nil for ordinary script process failure", result.Result.FailureMetadata)
	}
	if result.Result.Outcome != workers.OutcomeFailed || result.Result.Error != "script exited with status 1" {
		t.Fatalf("result = %#v, want terminal failed result", result.Result)
	}
}

func TestAttemptLifecycleRejectsConflictingWorkerCorrelation(t *testing.T) {
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		return workers.ExecuteResult{
			Correlation: workers.ExecutionCorrelation{DispatchID: "different-dispatch", AttemptID: request.Correlation.AttemptID},
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	lifecycle := newAttemptLifecycle(service, func() string { return "attempt-conflict" }, 1)
	var result workers.ExecuteResult
	if err := lifecycle.start(
		context.Background(), attemptTestRequest("dispatch-conflict", ""), false,
		func(_ context.Context, _ workers.ExecuteRequest, got workers.ExecuteResult, _ error) { result = got },
	); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil ||
		result.Failure.Message != "worker execution returned conflicting correlation" {
		t.Fatalf("conflicting result = %#v, want terminal correlation failure", result)
	}
}

func TestRuntimeExecutionSelectionMergesAuthoredWorkerWorkstationAndFactoryDefaults(t *testing.T) {
	lookup := runtimefixtures.RuntimeConfigLookupFixture{
		FactoryPath: "factory-dir",
		Factory:     &interfaces.FactoryConfig{Runner: "factory-runner"},
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"mock": {
				Name:             "mock",
				Type:             "agent",
				ExecutorProvider: "worker-provider",
				Model:            "worker-model",
				ModelProvider:    "worker-model-provider",
				Command:          "worker-command",
				Args:             []string{"--worker"},
				StopToken:        "worker-stop",
				SkipPermissions:  true,
				Timeout:          "3s",
				AgentTools:       &interfaces.AgentToolsConfig{Policy: "READ_ONLY"},
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Name:             "review",
				WorkerTypeName:   "mock",
				Runner:           "workstation-runner",
				Body:             "authored system prompt",
				OutputSchema:     "authored-schema",
				OutputContract:   "decision",
				OutcomeFormat:    "json",
				Timeout:          "2s",
				WorkingDirectory: "workspace",
				Worktree:         "worktree",
				Env:              map[string]string{"STATION": "yes"},
			},
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "review",
		Execution: workers.WorkstationExecutionRequest{
			WorkerType: "mock",
			EnvVars:    map[string]string{"OVERRIDE": "yes"},
		},
	}
	selection := resolveRuntimeExecutionSelection(
		&runtimeConfig{runtimeConfig: lookup},
		request,
		nil,
		[]workers.WorkInput{{Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "input message",
		}}}},
		nil,
	)

	assertRuntimeExecutionSelection(t, selection)
}

func TestRuntimeExecutionSelectionResolvesInvocationWorkerDefaults(t *testing.T) {
	lookup := runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"goal-executor": {
				Name:                        "goal-executor",
				Type:                        interfaces.WorkerTypeAgent,
				Model:                       "${executorModel}",
				ModelProvider:               "${executorProvider}",
				RuntimeDefaultModel:         "operator-model",
				RuntimeDefaultModelProvider: "operator-provider",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"execute": {
				Name:           "execute",
				WorkerTypeName: "goal-executor",
			},
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "execute",
		Execution:       workers.WorkstationExecutionRequest{},
	}
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"executorProvider": {Values: []string{""}},
		"executorModel":    {Values: []string{""}},
	}}
	selection := resolveRuntimeExecutionSelection(
		&runtimeConfig{runtimeConfig: lookup}, request, nil, nil, invocation,
	)
	if selection.providerID != "operator-provider" || selection.modelProvider != "operator-provider" ||
		selection.model != "operator-model" || selection.runnerID != workers.RunnerIDCodex {
		t.Fatalf("resolved selection = %#v, want operator provider/model and codex runner", selection)
	}

	invocation.Arguments["executorProvider"] = work.InvocationArgument{Values: []string{"explicit-provider"}}
	invocation.Arguments["executorModel"] = work.InvocationArgument{Values: []string{"explicit-model"}}
	selection = resolveRuntimeExecutionSelection(
		&runtimeConfig{runtimeConfig: lookup}, request, nil, nil, invocation,
	)
	if selection.providerID != "explicit-provider" || selection.modelProvider != "explicit-provider" ||
		selection.model != "explicit-model" {
		t.Fatalf("explicit selection = %#v, want invocation provider/model", selection)
	}
}

func assertRuntimeExecutionSelection(t *testing.T, selection runtimeExecutionSelection) {
	t.Helper()
	assertSelectionStrings(t, []selectionStringCheck{
		{name: "worker name", got: selection.workerName, want: "mock"},
		{name: "worker type", got: selection.workerType, want: "agent"},
		{name: "runner", got: selection.runnerID, want: "workstation-runner"},
		{name: "provider", got: selection.providerID, want: "worker-provider"},
		{name: "model", got: selection.model, want: "worker-model"},
		{name: "model provider", got: selection.modelProvider, want: "worker-model-provider"},
		{name: "command", got: selection.command, want: "worker-command"},
		{name: "stop token", got: selection.stopToken, want: "worker-stop"},
		{name: "system prompt", got: selection.systemPrompt, want: "authored system prompt"},
		{name: "output schema", got: selection.outputSchema, want: "authored-schema"},
		{name: "output contract", got: selection.outputContract, want: "decision"},
		{name: "output format", got: selection.outputFormat, want: "json"},
		{name: "working directory", got: selection.workingDirectory, want: "workspace"},
		{name: "worktree", got: selection.worktree, want: "worktree"},
		{name: "user message", got: selection.userMessage, want: "input message"},
	})
	if len(selection.args) != 1 || selection.args[0] != "--worker" {
		t.Fatalf("worker args = %#v, want [--worker]", selection.args)
	}
	if !selection.skipPermissions || selection.timeout != 3*time.Second || !selection.decisionEnvelope ||
		selection.toolExecutionMode != workers.RunnerToolExecutionModeRequired {
		t.Fatalf("execution policy = %#v, want worker timeout/tools and decision envelope", selection)
	}
	if selection.environment["STATION"] != "yes" || selection.environment["OVERRIDE"] != "yes" {
		t.Fatalf("merged environment = %#v", selection.environment)
	}
}

type selectionStringCheck struct {
	name string
	got  string
	want string
}

func assertSelectionStrings(t *testing.T, checks []selectionStringCheck) {
	t.Helper()
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("%s = %q, want %q", check.name, check.got, check.want)
		}
	}
}

func TestContinuationFromLegacySessionKeepsProviderIdentity(t *testing.T) {
	if got := continuationFromLegacySession(nil); got != nil {
		t.Fatalf("nil legacy session = %#v, want nil", got)
	}
	ref := &providers.SessionRef{Provider: providers.IDCodex, ID: "session-1"}
	got := continuationFromLegacySession(ref)
	if got == nil || got.Provider != string(providers.IDCodex) ||
		got.ProviderSessionID != "session-1" || got.ExternalRef != "session-1" {
		t.Fatalf("legacy continuation = %#v, want provider session identity", got)
	}
}

func TestInvokeWorkerResultFromPreservesSessionOutcomeAndSafeDiagnostics(t *testing.T) {
	completed := invokeWorkerResultFrom(
		"dispatch-completed",
		"session-completed",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{ID: "session-completed", State: workersessions.StateCompleted},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{
				Output: "completed output",
				ProviderSession: &workers.ProviderSessionMetadata{
					Provider: "codex",
					ID:       "provider-session-completed",
				},
			}},
			Attempts: 2,
		},
	)
	if completed.Outcome != factory.InvokeWorkerOutcomeCompleted || completed.Output != "completed output" ||
		completed.Attempts != 2 || completed.Provider != "codex" ||
		completed.ProviderSessionRef != "provider-session-completed" {
		t.Fatalf("completed InvokeWorker result = %#v", completed)
	}

	failed := invokeWorkerResultFrom(
		"dispatch-failed",
		"session-failed",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{
				ID:    "session-failed",
				State: workersessions.StateFailed,
				Result: &workersessions.TerminalResult{Cause: &workersessions.FailureCause{
					Kind:   workersessions.FailureCauseWorkersExecutionFailure,
					Detail: "provider timed out",
				}},
			},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{
				FailureMetadata: &workers.WorkFailureMetadata{
					Family: workers.WorkFailureFamilyRetryable,
					Type:   workers.WorkFailureTypeTimeout,
				},
			}},
			Attempts: 1,
		},
	)
	if failed.Outcome != factory.InvokeWorkerOutcomeFailed || failed.Diagnostic != "provider timed out" ||
		failed.FailureReason != string(workers.WorkFailureTypeTimeout) || failed.Retryable == nil || !*failed.Retryable {
		t.Fatalf("failed InvokeWorker result = %#v, want retryable timeout", failed)
	}

	withoutCause := invokeWorkerResultFrom(
		"dispatch-no-cause",
		"session-no-cause",
		workersessions.InvokeSessionResult{
			Session:  workersessions.Session{ID: "session-no-cause", State: workersessions.StateFailed},
			Dispatch: workers.WorkstationDispatchResult{Result: workers.WorkResult{Error: "adapter failed"}},
		},
	)
	if withoutCause.Diagnostic != "adapter failed" {
		t.Fatalf("failure without cause diagnostic = %q, want adapter error", withoutCause.Diagnostic)
	}

	withoutDetails := invokeWorkerResultFrom(
		"dispatch-no-details",
		"session-no-details",
		workersessions.InvokeSessionResult{
			Session: workersessions.Session{ID: "session-no-details", State: workersessions.StateFailed},
		},
	)
	if withoutDetails.Diagnostic != "Provider execution failed." {
		t.Fatalf("failure without details diagnostic = %q, want bounded fallback", withoutDetails.Diagnostic)
	}
}

func assertDetachedWorkInput(t *testing.T, observed workers.ExecuteRequest) {
	t.Helper()
	if len(observed.Input.Work) != 1 ||
		observed.Input.Work[0].WorkID != "work-1" ||
		observed.Input.Work[0].Content[0].Text != "input" {
		t.Fatalf("work input = %#v", observed.Input.Work)
	}
}

func TestStartThroughStatelessWorkersPreservesDetachedDispatchFacts(t *testing.T) {
	var observed workers.ExecuteRequest
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		observed = request
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: service,
		newID:          func() string { return "attempt-facts" },
		attempts:       newAttemptLifecycle(service, func() string { return "attempt-facts" }, 1),
		inlineDispatch: true,
	}
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-facts",
		TransitionID:    "transition-facts",
		WorkerType:      "agent",
		WorkstationName: "workstation-facts",
		ProjectID:       "project-facts",
		Execution: work.ExecutionMetadata{
			RequestID: "request-facts",
			TraceID:   "trace-facts",
			ReplayKey: "replay-facts",
			WorkIDs:   []string{"work-facts"},
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "workstation-facts",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerType:       "agent",
			RunnerID:         workers.RunnerIDCodex,
			FactorySessionID: "session-facts",
			RecordingID:      "runtime-facts",
			GenerationID:     "generation-facts",
		},
	}
	if err := startThroughStatelessWorkers(context.Background(), cfg, request, nil); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	if observed.Input.Dispatch.ProjectID != "project-facts" ||
		observed.Input.Dispatch.Execution.ReplayKey != "replay-facts" ||
		observed.Input.Dispatch.TransitionID != "transition-facts" {
		t.Fatalf("detached dispatch facts = %#v", observed.Input.Dispatch)
	}
}

// TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity pins the
// identity a Worker carries into Workers.
//
// A JavaScript workflow resumed after an interruption re-runs the child that
// was cut off under its original orchestrator-minted dispatch ID. Workers
// treats a dispatch ID as single-use for the whole life of its pool -- an
// accepted dispatch is never removed from the pool's record map -- so a re-run
// that reuses that ID is refused before it reaches an executor, and the caller
// sees START_FAILURE rather than a second Worker.
//
// The Worker Session identity is the one Runtime already mints uniquely, so it
// is the identity Workers is given. What the caller sees is unchanged: its own
// dispatch ID comes back on the result, because that is the identity its own
// records are keyed by.
func TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	firstWorkersID, first := runOneInvokeWorker(t, impl, boundary, "child-1")
	secondWorkersID, second := runOneInvokeWorker(t, impl, boundary, "child-1")

	if secondWorkersID == firstWorkersID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want an identity distinct from the first attempt's %q",
			secondWorkersID,
			firstWorkersID,
		)
	}
	if secondWorkersID != second.WorkerSessionID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want the reserved Worker Session identity %q",
			secondWorkersID,
			second.WorkerSessionID,
		)
	}
	for _, result := range []factory.InvokeWorkerResult{first, second} {
		if result.DispatchID != "child-1" {
			t.Fatalf("result dispatch ID = %q, want the caller's own %q", result.DispatchID, "child-1")
		}
		if result.Outcome != factory.InvokeWorkerOutcomeCompleted {
			t.Fatalf("result outcome = %q, want COMPLETED", result.Outcome)
		}
	}
}

// TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity keeps the common
// case honest: an uncontended Worker still reaches Workers under exactly the
// identity its caller minted, so the resume suffix above is visibly the
// exception rather than the rule.
func TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	workersID, result := runOneInvokeWorker(t, impl, boundary, "child-1")
	if workersID != "child-1" {
		t.Fatalf("Workers dispatch ID = %q, want the caller's own %q", workersID, "child-1")
	}
	if result.WorkerSessionID != "child-1" {
		t.Fatalf("Worker Session ID = %q, want %q", result.WorkerSessionID, "child-1")
	}
}

func TestInvokeWorker_UsesRuntimeRetryBudget(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)
	done := make(chan struct {
		result factory.InvokeWorkerResult
		err    error
	}, 1)
	go func() {
		result, err := impl.InvokeWorker(context.Background(), factory.InvokeWorkerRequest{
			DispatchID:  "child-retry",
			Prompt:      "run",
			MaxAttempts: 2,
		})
		done <- struct {
			result factory.InvokeWorkerResult
			err    error
		}{result: result, err: err}
	}()

	first := awaitCanonicalWorkersRequest(t, boundary.requests)
	if got := first.Execution.Dispatch.DispatchID; got != "child-retry" {
		t.Fatalf("first dispatch ID = %q, want child-retry", got)
	}
	boundary.results <- workers.WorkstationDispatchResult{
		DispatchID:      first.Execution.Dispatch.DispatchID,
		WorkstationName: first.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: first.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyRetryable,
				Type:   workers.WorkFailureTypeInternalServerError,
			},
		},
	}

	second := awaitCanonicalWorkersRequest(t, boundary.requests)
	if got := second.Execution.Dispatch.DispatchID; got != "child-retry/attempt/2" {
		t.Fatalf("second dispatch ID = %q, want child-retry/attempt/2", got)
	}
	boundary.results <- completedWorkersResult(second)

	got := <-done
	if got.err != nil {
		t.Fatalf("InvokeWorker: %v", got.err)
	}
	if got.result.Outcome != factory.InvokeWorkerOutcomeCompleted || got.result.Attempts != 2 {
		t.Fatalf("InvokeWorker result = %#v, want completed after two attempts", got.result)
	}
}

// TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy pins the
// two facts a Worker with no authored workstation can only get from its
// caller.
//
// The worker name is what --with-mock-workers matches a named preset on, at
// the subprocess boundary, so a Worker that arrives without it is never the
// mock the operator configured. The permission policy is the invocation
// -effective one the caller already resolved; dropping it runs the Worker
// under a policy its own dispatch record says it does not have.
func TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy(t *testing.T) {
	for _, test := range []struct {
		name string
		skip bool
	}{
		{name: "true", skip: true},
		{name: "false", skip: false},
		{name: "omitted", skip: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledWorkstationBoundary()
			impl := newInvokeWorkerTestFactory(t, boundary)
			capabilities := &workers.Capabilities{NativeStreaming: true, ToolLifecycle: true}

			observed, _ := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
				DispatchID:      "child-1",
				Prompt:          "run",
				WorkerName:      "worker-a",
				SkipPermissions: test.skip,
				RecordingID:     "recording-1",
				Capabilities:    capabilities,
			})
			if observed.Execution.WorkerType != "worker-a" {
				t.Fatalf("Workers worker type = %q, want the authored worker name %q", observed.Execution.WorkerType, "worker-a")
			}
			if observed.Execution.Dispatch.WorkerType != "worker-a" {
				t.Fatalf(
					"dispatch worker type = %q, want the authored worker name %q",
					observed.Execution.Dispatch.WorkerType,
					"worker-a",
				)
			}
			if observed.Execution.SkipPermissions != test.skip {
				t.Fatalf("Workers skip-permissions = %v, want %v", observed.Execution.SkipPermissions, test.skip)
			}
			if observed.Execution.RecordingID != "recording-1" {
				t.Fatalf("Workers recording ID = %q, want recording-1", observed.Execution.RecordingID)
			}
			if observed.Execution.Capabilities == nil || !observed.Execution.Capabilities.NativeStreaming || !observed.Execution.Capabilities.ToolLifecycle {
				t.Fatalf("Workers capabilities = %+v, want caller-supplied capability facts", observed.Execution.Capabilities)
			}
		})
	}
}

// runOneInvokeWorker drives one minimal InvokeWorker to its terminal result
// and reports the dispatch identity Workers actually observed.
func runOneInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	dispatchID string,
) (string, factory.InvokeWorkerResult) {
	t.Helper()
	observed, result := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
		DispatchID: dispatchID,
		Prompt:     "run",
	})
	return observed.Execution.Dispatch.DispatchID, result
}

// runInvokeWorker drives one InvokeWorker to its terminal result and reports
// the request Workers actually observed alongside it.
func runInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	req factory.InvokeWorkerRequest,
) (workers.WorkstationDispatchRequest, factory.InvokeWorkerResult) {
	t.Helper()
	type outcome struct {
		result factory.InvokeWorkerResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := impl.InvokeWorker(context.Background(), req)
		done <- outcome{result: result, err: err}
	}()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	boundary.results <- completedWorkersResult(request)
	got := <-done
	if got.err != nil {
		t.Fatalf("InvokeWorker(%q): %v", req.DispatchID, got.err)
	}
	return request, got.result
}
