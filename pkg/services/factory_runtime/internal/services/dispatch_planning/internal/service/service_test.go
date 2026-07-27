package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPlanPreservesSchedulerOrderAndCanonicalWorkersFacts(t *testing.T) {
	t.Parallel()

	decisions := []dispatchplanning.RunnableDecision{
		runnableDecision("dispatch-review", "correlation-review", "review", "reviewer", "work-2"),
		runnableDecision("dispatch-implement", "correlation-implement", "implement", "implementer", "work-1"),
	}
	decisions[0].Execution.FactorySessionID = "session-1"
	decisions[0].Dispatch.Execution.RequestID = "request-1"
	decisions[0].Dispatch.Execution.TraceID = "trace-1"
	decisions[0].Dispatch.InputBindings = map[string][]string{"work": {"token-2"}}
	decisions[0].Execution.EnvVars = map[string]string{"MODE": "review"}
	wantExecutions := []workers.WorkstationExecutionRequest{
		expectedExecution(decisions[0]),
		expectedExecution(decisions[1]),
	}

	result, err := New(nil).Plan(context.Background(), dispatchplanning.PlanRequest{Decisions: decisions})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(result.Actions) != 2 {
		t.Fatalf("Plan() actions = %d, want 2", len(result.Actions))
	}
	for index, want := range wantExecutions {
		action := result.Actions[index]
		if action.CorrelationID != decisions[index].CorrelationID {
			t.Fatalf("action %d correlation = %q, want %q", index, action.CorrelationID, decisions[index].CorrelationID)
		}
		if action.Request.WorkstationName != want.Dispatch.WorkstationName {
			t.Fatalf("action %d workstation = %q, want %q", index, action.Request.WorkstationName, want.Dispatch.WorkstationName)
		}
		if !reflect.DeepEqual(action.Request.Execution, want) {
			t.Fatalf("action %d execution = %#v, want %#v", index, action.Request.Execution, want)
		}
	}

	decisions[0].Dispatch.InputTokens[0] = "mutated"
	decisions[0].Execution.InputPayload[0] = "mutated"
	decisions[0].Dispatch.Execution.WorkIDs[0] = "mutated"
	decisions[0].Dispatch.InputBindings["work"][0] = "mutated"
	decisions[0].Execution.EnvVars["MODE"] = "mutated"
	if !reflect.DeepEqual(result.Actions[0].Request.Execution, wantExecutions[0]) {
		t.Fatal("planned action retained mutable decision payload aliases")
	}
}

func TestPlanRejectsWholeBatchBeforeReturningActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decisions []dispatchplanning.RunnableDecision
	}{
		{
			name: "incomplete later decision",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
				runnableDecision("dispatch-2", "", "implement", "implementer", "work-2"),
			},
		},
		{
			name: "duplicate identity",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
				runnableDecision("dispatch-1", "correlation-2", "implement", "implementer", "work-2"),
			},
		},
		{
			name: "missing input payload",
			decisions: []dispatchplanning.RunnableDecision{
				runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
			},
		},
	}
	tests[2].decisions[0].Dispatch.InputTokens = nil

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := New(nil).Plan(context.Background(), dispatchplanning.PlanRequest{Decisions: test.decisions})
			if !errors.Is(err, dispatchplanning.ErrInvalidRunnableDecision) {
				t.Fatalf("Plan() error = %v, want ErrInvalidRunnableDecision", err)
			}
			if len(result.Actions) != 0 {
				t.Fatalf("Plan() actions = %#v, want no partially visible actions", result.Actions)
			}
		})
	}
}

func TestPlanHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := New(nil).Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{
			runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Plan() error = %v, want context.Canceled", err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("Plan() actions = %#v, want none", result.Actions)
	}
}

func TestPublishAcceptsOnceAndRejectsIdentityConflicts(t *testing.T) {
	t.Parallel()

	var published []workers.WorkstationDispatchRequest
	planner := New(func(_ context.Context, request workers.WorkstationDispatchRequest) error {
		published = append(published, workers.WorkstationDispatchRequest{
			WorkstationName: request.WorkstationName,
			Execution:       workers.CloneWorkstationExecutionRequest(request.Execution),
		})
		return nil
	})
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))

	first, err := planner.Publish(context.Background(), action)
	if err != nil {
		t.Fatalf("Publish(first) error = %v", err)
	}
	if first.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("Publish(first) outcome = %q, want ACCEPTED", first.Outcome)
	}
	duplicate, err := planner.Publish(context.Background(), action)
	if err != nil {
		t.Fatalf("Publish(duplicate) error = %v", err)
	}
	if duplicate.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
		t.Fatalf("Publish(duplicate) outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
	}

	dispatchConflict := cloneTestAction(action)
	dispatchConflict.Request.Execution.WorkerType = "implementer"
	if _, err := planner.Publish(context.Background(), dispatchConflict); !errors.Is(err, dispatchplanning.ErrDuplicateDispatchIntent) {
		t.Fatalf("Publish(dispatch conflict) error = %v, want ErrDuplicateDispatchIntent", err)
	}
	correlationConflict := plannedAction(t, planner, runnableDecision(
		"dispatch-2",
		"correlation-1",
		"implement",
		"implementer",
		"work-2",
	))
	if _, err := planner.Publish(context.Background(), correlationConflict); !errors.Is(err, dispatchplanning.ErrDuplicateDispatchIntent) {
		t.Fatalf("Publish(correlation conflict) error = %v, want ErrDuplicateDispatchIntent", err)
	}
	if len(published) != 1 {
		t.Fatalf("Workers publications = %d, want 1", len(published))
	}
	intent, ok := planner.Intent("dispatch-1")
	if !ok {
		t.Fatal("Intent(dispatch-1) not found")
	}
	if intent.Status != dispatchplanning.OutboxIntentStatusPublished || intent.Attempts != 1 {
		t.Fatalf("intent state = (%q, %d), want (PUBLISHED, 1)", intent.Status, intent.Attempts)
	}
}

func TestPublishConcurrentEquivalentIntentPublishesOnce(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	var publishMu sync.Mutex
	publishCount := 0
	planner := New(func(_ context.Context, _ workers.WorkstationDispatchRequest) error {
		publishMu.Lock()
		publishCount++
		publishMu.Unlock()
		close(entered)
		<-release
		return nil
	})
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))

	type publishResponse struct {
		result dispatchplanning.PublicationResult
		err    error
	}
	firstCh := make(chan publishResponse, 1)
	go func() {
		result, err := planner.Publish(context.Background(), action)
		firstCh <- publishResponse{result: result, err: err}
	}()
	<-entered

	const duplicates = 32
	results := make(chan publishResponse, duplicates)
	var submissions sync.WaitGroup
	for index := 0; index < duplicates; index++ {
		submissions.Add(1)
		go func() {
			defer submissions.Done()
			result, err := planner.Publish(context.Background(), action)
			results <- publishResponse{result: result, err: err}
		}()
	}
	submissions.Wait()
	close(release)
	first := <-firstCh
	if first.err != nil || first.result.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("first Publish() = (%#v, %v), want ACCEPTED", first.result, first.err)
	}
	close(results)
	for response := range results {
		if response.err != nil || response.result.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
			t.Fatalf("concurrent Publish() = (%#v, %v), want DUPLICATE_IDEMPOTENT", response.result, response.err)
		}
	}
	publishMu.Lock()
	defer publishMu.Unlock()
	if publishCount != 1 {
		t.Fatalf("Workers publications = %d, want 1", publishCount)
	}
}

func TestPublishFailureRemainsPendingAndRetryUsesStableRequest(t *testing.T) {
	t.Parallel()

	publishErr := errors.New("Workers temporarily unavailable")
	var mu sync.Mutex
	var published []workers.WorkstationDispatchRequest
	planner := New(func(_ context.Context, request workers.WorkstationDispatchRequest) error {
		mu.Lock()
		defer mu.Unlock()
		published = append(published, workers.WorkstationDispatchRequest{
			WorkstationName: request.WorkstationName,
			Execution:       workers.CloneWorkstationExecutionRequest(request.Execution),
		})
		if len(published) == 1 {
			return publishErr
		}
		return nil
	})
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))

	first, err := planner.Publish(context.Background(), action)
	if !errors.Is(err, publishErr) {
		t.Fatalf("Publish() error = %v, want publication failure", err)
	}
	if first.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("Publish() outcome = %q, want ACCEPTED", first.Outcome)
	}
	pending, ok := planner.Intent("dispatch-1")
	if !ok || pending.Status != dispatchplanning.OutboxIntentStatusPending || pending.Attempts != 1 {
		t.Fatalf("pending intent = (%#v, %t), want PENDING after one attempt", pending, ok)
	}

	action.Request.Execution.WorkerType = "mutated-after-acceptance"
	retried, err := planner.Retry(context.Background(), "dispatch-1")
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retried.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
		t.Fatalf("Retry() outcome = %q, want DUPLICATE_IDEMPOTENT", retried.Outcome)
	}
	publishedIntent, ok := planner.Intent("dispatch-1")
	if !ok || publishedIntent.Status != dispatchplanning.OutboxIntentStatusPublished || publishedIntent.Attempts != 2 {
		t.Fatalf("retried intent = (%#v, %t), want PUBLISHED after two attempts", publishedIntent, ok)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(published) != 2 || !reflect.DeepEqual(published[0], published[1]) {
		t.Fatalf("retry publications = %#v, want the same stable logical Workers request twice", published)
	}
}

func TestPublishCancellationLeavesAcceptedIntentPending(t *testing.T) {
	t.Parallel()

	publishCalls := 0
	planner := New(func(_ context.Context, _ workers.WorkstationDispatchRequest) error {
		publishCalls++
		return nil
	})
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := planner.Publish(ctx, action)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish() error = %v, want context.Canceled", err)
	}
	if result.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("Publish() outcome = %q, want ACCEPTED", result.Outcome)
	}
	intent, ok := planner.Intent("dispatch-1")
	if !ok || intent.Status != dispatchplanning.OutboxIntentStatusPending || intent.Attempts != 1 {
		t.Fatalf("cancelled intent = (%#v, %t), want PENDING after one attempt", intent, ok)
	}
	if publishCalls != 0 {
		t.Fatalf("Workers publisher calls = %d, want 0 for pre-cancelled context", publishCalls)
	}
}

func plannedAction(
	t *testing.T,
	planner *Planner,
	decision dispatchplanning.RunnableDecision,
) dispatchplanning.OutboxAction {
	t.Helper()
	result, err := planner.Plan(context.Background(), dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{decision},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("Plan() actions = %d, want 1", len(result.Actions))
	}
	return result.Actions[0]
}

func cloneTestAction(action dispatchplanning.OutboxAction) dispatchplanning.OutboxAction {
	return dispatchplanning.OutboxAction{
		CorrelationID: action.CorrelationID,
		Request: workers.WorkstationDispatchRequest{
			WorkstationName: action.Request.WorkstationName,
			Execution:       workers.CloneWorkstationExecutionRequest(action.Request.Execution),
		},
	}
}

func runnableDecision(
	dispatchID string,
	correlationID string,
	workstationName string,
	workerType string,
	workID string,
) dispatchplanning.RunnableDecision {
	return dispatchplanning.RunnableDecision{
		CorrelationID: correlationID,
		Dispatch: work.WorkDispatch{
			DispatchID:      dispatchID,
			WorkerType:      workerType,
			WorkstationName: workstationName,
			Execution: work.ExecutionMetadata{
				WorkIDs:   []string{workID},
				ReplayKey: "replay/" + workID,
			},
			InputTokens: []any{"payload-" + workID},
		},
		Execution: dispatchplanning.ExecutionFacts{
			WorkerType:   workerType,
			InputPayload: []any{"payload-" + workID},
		},
	}
}

func expectedExecution(decision dispatchplanning.RunnableDecision) workers.WorkstationExecutionRequest {
	facts := decision.Execution
	return workers.CloneWorkstationExecutionRequest(workers.WorkstationExecutionRequest{
		Dispatch:                 decision.Dispatch,
		WorkerType:               facts.WorkerType,
		WorkstationType:          facts.WorkstationType,
		RunnerID:                 facts.RunnerID,
		RunnerSelectionSource:    facts.RunnerSelectionSource,
		ProjectID:                facts.ProjectID,
		FactorySessionID:         facts.FactorySessionID,
		InputTokens:              facts.InputPayload,
		ModelOperation:           facts.ModelOperation,
		ModelBindings:            facts.ModelBindings,
		Model:                    facts.Model,
		ModelProvider:            facts.ModelProvider,
		SystemPrompt:             facts.SystemPrompt,
		UserMessage:              facts.UserMessage,
		OutputSchema:             facts.OutputSchema,
		EnvVars:                  facts.EnvVars,
		ProcessEnvironment:       facts.ProcessEnvironment,
		Worktree:                 facts.Worktree,
		WorkingDirectory:         facts.WorkingDirectory,
		WorkingDirectoryAuthored: facts.WorkingDirectoryAuthored,
	})
}
