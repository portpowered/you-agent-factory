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

func TestPausePreservesAcceptedIntentsAndResumePublishesInOrder(t *testing.T) {
	t.Parallel()

	var published []string
	planner := New(func(_ context.Context, request workers.WorkstationDispatchRequest) error {
		published = append(published, request.Execution.Dispatch.DispatchID)
		return nil
	})
	if err := planner.Pause(context.Background()); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}

	for index, decision := range []dispatchplanning.RunnableDecision{
		runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
		runnableDecision("dispatch-2", "correlation-2", "implement", "implementer", "work-2"),
	} {
		action := plannedAction(t, planner, decision)
		result, err := planner.Publish(context.Background(), action)
		if err != nil {
			t.Fatalf("Publish(%d) error = %v", index, err)
		}
		if result.Outcome != dispatchplanning.PublicationOutcomeAccepted {
			t.Fatalf("Publish(%d) outcome = %q, want ACCEPTED", index, result.Outcome)
		}
		intent, ok := planner.Intent(action.Request.Execution.Dispatch.DispatchID)
		if !ok || intent.Status != dispatchplanning.OutboxIntentStatusPending || intent.Attempts != 0 {
			t.Fatalf("paused intent %d = (%#v, %t), want unattempted PENDING", index, intent, ok)
		}
	}
	if len(published) != 0 {
		t.Fatalf("paused Workers publications = %#v, want none", published)
	}

	if err := planner.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if !reflect.DeepEqual(published, []string{"dispatch-1", "dispatch-2"}) {
		t.Fatalf("resume publication order = %#v", published)
	}
	if state := planner.State(); state.Mode != dispatchplanning.RuntimeOutboxModeActive {
		t.Fatalf("State() = %#v, want ACTIVE", state)
	}
}

func TestResumeFailureRepausesWithoutPublishingLaterIntents(t *testing.T) {
	t.Parallel()

	publishErr := errors.New("Workers unavailable")
	var published []string
	planner := New(func(_ context.Context, request workers.WorkstationDispatchRequest) error {
		published = append(published, request.Execution.Dispatch.DispatchID)
		return publishErr
	})
	if err := planner.Pause(context.Background()); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	for _, decision := range []dispatchplanning.RunnableDecision{
		runnableDecision("dispatch-1", "correlation-1", "review", "reviewer", "work-1"),
		runnableDecision("dispatch-2", "correlation-2", "implement", "implementer", "work-2"),
	} {
		if _, err := planner.Publish(context.Background(), plannedAction(t, planner, decision)); err != nil {
			t.Fatalf("Publish(paused) error = %v", err)
		}
	}

	if err := planner.Resume(context.Background()); !errors.Is(err, publishErr) {
		t.Fatalf("Resume() error = %v, want Workers failure", err)
	}
	if !reflect.DeepEqual(published, []string{"dispatch-1"}) {
		t.Fatalf("failed resume publications = %#v, want no overtaking", published)
	}
	if state := planner.State(); state.Mode != dispatchplanning.RuntimeOutboxModePaused {
		t.Fatalf("State() = %#v, want PAUSED after failed ordered drain", state)
	}
}

func TestWorkersFailureIsTerminalAndLaterDecisionUsesDistinctIdentity(t *testing.T) {
	t.Parallel()

	var published []string
	planner := New(func(_ context.Context, request workers.WorkstationDispatchRequest) error {
		published = append(published, request.Execution.Dispatch.DispatchID)
		return nil
	})
	firstAction := plannedAction(t, planner, runnableDecision(
		"dispatch-attempt-1",
		"correlation-attempt-1",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), firstAction); err != nil {
		t.Fatalf("Publish(first) error = %v", err)
	}
	failure := dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-attempt-1",
		CorrelationID: "correlation-attempt-1",
		WorkID:        "work-1",
		Outcome:       dispatchplanning.TerminalResultOutcomeFailure,
	}
	if _, err := planner.Retire(context.Background(), failure); err != nil {
		t.Fatalf("Retire(failure) error = %v", err)
	}
	if !reflect.DeepEqual(published, []string{"dispatch-attempt-1"}) {
		t.Fatalf("failure caused Runtime retry publications = %#v", published)
	}

	duplicate, err := planner.Publish(context.Background(), firstAction)
	if err != nil || duplicate.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
		t.Fatalf("Publish(retired intent) = (%#v, %v), want DUPLICATE_IDEMPOTENT", duplicate, err)
	}
	secondAction := plannedAction(t, planner, runnableDecision(
		"dispatch-attempt-2",
		"correlation-attempt-2",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), secondAction); err != nil {
		t.Fatalf("Publish(later decision) error = %v", err)
	}
	if !reflect.DeepEqual(published, []string{"dispatch-attempt-1", "dispatch-attempt-2"}) {
		t.Fatalf("later decision publications = %#v", published)
	}
}

func TestCancellationBlocksPublicationCancelsVisibleIntentAndAcceptsLateResultOnce(t *testing.T) {
	t.Parallel()
	assertStoppedRuntimeBehavior(t, dispatchplanning.RuntimeStopReasonCancelled)
}

func TestTerminationBlocksPublicationCancelsVisibleIntentAndAcceptsLateResultOnce(t *testing.T) {
	t.Parallel()
	assertStoppedRuntimeBehavior(t, dispatchplanning.RuntimeStopReasonTerminated)
}

func assertStoppedRuntimeBehavior(t *testing.T, reason dispatchplanning.RuntimeStopReason) {
	t.Helper()
	planner, published, cancelled := stoppedRuntimeFixture(t, reason)
	assertStoppedPublicationBoundary(t, planner, published, reason)
	assertLateResultIsRetiredOnce(t, planner)
	if err := planner.Stop(context.Background(), reason); err != nil {
		t.Fatalf("Stop(repeated) error = %v", err)
	}
	if len(*cancelled) != 1 {
		t.Fatalf("repeated Stop() cancellations = %#v, want one", *cancelled)
	}
}

func stoppedRuntimeFixture(
	t *testing.T,
	reason dispatchplanning.RuntimeStopReason,
) (*Planner, dispatchplanning.OutboxAction, *[]string) {
	t.Helper()
	var cancelled []string
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			_ context.Context,
			request workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			cancelled = append(cancelled, request.DispatchID)
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	published := plannedAction(t, planner, runnableDecision(
		"dispatch-visible",
		"correlation-visible",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), published); err != nil {
		t.Fatalf("Publish(visible) error = %v", err)
	}
	if err := planner.Pause(context.Background()); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	pending := plannedAction(t, planner, runnableDecision(
		"dispatch-pending",
		"correlation-pending",
		"review",
		"reviewer",
		"work-2",
	))
	if _, err := planner.Publish(context.Background(), pending); err != nil {
		t.Fatalf("Publish(pending) error = %v", err)
	}
	if err := planner.Stop(context.Background(), reason); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !reflect.DeepEqual(cancelled, []string{"dispatch-visible"}) {
		t.Fatalf("Workers cancellations = %#v, want visible dispatch only", cancelled)
	}
	return planner, published, &cancelled
}

func assertStoppedPublicationBoundary(
	t *testing.T,
	planner *Planner,
	published dispatchplanning.OutboxAction,
	reason dispatchplanning.RuntimeStopReason,
) {
	t.Helper()
	state := planner.State()
	if state.Mode != dispatchplanning.RuntimeOutboxModeStopped || state.StopReason != reason {
		t.Fatalf("State() = %#v, want STOPPED by %s", state, reason)
	}
	if _, err := planner.Retry(context.Background(), "dispatch-pending"); !errors.Is(err, dispatchplanning.ErrDispatchRuntimeStopped) {
		t.Fatalf("Retry(after stop) error = %v, want ErrDispatchRuntimeStopped", err)
	}
	duplicate, err := planner.Publish(context.Background(), published)
	if err != nil || duplicate.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
		t.Fatalf("Publish(existing after stop) = (%#v, %v), want DUPLICATE_IDEMPOTENT", duplicate, err)
	}
	newAction := plannedAction(t, planner, runnableDecision(
		"dispatch-new",
		"correlation-new",
		"review",
		"reviewer",
		"work-3",
	))
	if _, err := planner.Publish(context.Background(), newAction); !errors.Is(err, dispatchplanning.ErrDispatchRuntimeStopped) {
		t.Fatalf("Publish(after stop) error = %v, want ErrDispatchRuntimeStopped", err)
	}
}

func assertLateResultIsRetiredOnce(t *testing.T, planner *Planner) {
	t.Helper()
	late := dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-visible",
		CorrelationID: "correlation-visible",
		WorkID:        "work-1",
		Outcome:       dispatchplanning.TerminalResultOutcomeCancelled,
	}
	first, err := planner.Retire(context.Background(), late)
	if err != nil || first.Outcome != dispatchplanning.RetirementOutcomeRetired {
		t.Fatalf("Retire(late) = (%#v, %v), want RETIRED", first, err)
	}
	duplicate, err := planner.Retire(context.Background(), late)
	if err != nil || duplicate.Outcome != dispatchplanning.RetirementOutcomeDuplicateIdempotent {
		t.Fatalf("Retire(late duplicate) = (%#v, %v), want DUPLICATE_IDEMPOTENT", duplicate, err)
	}
}

func TestStopRetriesFailedWorkersCancellation(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("Workers cancellation unavailable")
	cancelCalls := 0
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			context.Context,
			workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			cancelCalls++
			if cancelCalls == 1 {
				return workers.WorkstationDispatchCancelResult{}, cancelErr
			}
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), action); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if err := planner.Stop(context.Background(), dispatchplanning.RuntimeStopReasonCancelled); !errors.Is(err, cancelErr) {
		t.Fatalf("Stop(first) error = %v, want cancellation failure", err)
	}
	if err := planner.Stop(context.Background(), dispatchplanning.RuntimeStopReasonCancelled); err != nil {
		t.Fatalf("Stop(retry) error = %v", err)
	}
	intent, ok := planner.Intent("dispatch-1")
	if !ok || !intent.CancellationRequested || cancelCalls != 2 {
		t.Fatalf("cancellation retry = (%#v, %t, %d calls), want successful second attempt", intent, ok, cancelCalls)
	}
}

func TestStopRacingPublicationCancelsAfterWorkersAcceptance(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	cancelled := make(chan string, 1)
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error {
			close(entered)
			<-release
			return nil
		},
		func(
			_ context.Context,
			request workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			cancelled <- request.DispatchID
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-racing",
		"correlation-racing",
		"review",
		"reviewer",
		"work-1",
	))
	published := make(chan error, 1)
	go func() {
		_, err := planner.Publish(context.Background(), action)
		published <- err
	}()
	<-entered

	stopped := make(chan error, 1)
	go func() {
		stopped <- planner.Stop(context.Background(), dispatchplanning.RuntimeStopReasonTerminated)
	}()
	close(release)
	if err := <-stopped; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := <-published; err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if dispatchID := <-cancelled; dispatchID != "dispatch-racing" {
		t.Fatalf("cancelled dispatch = %q, want dispatch-racing", dispatchID)
	}
}

func TestRetireAcceptsEachTerminalOutcomeExactlyOnce(t *testing.T) {
	t.Parallel()

	outcomes := []dispatchplanning.TerminalResultOutcome{
		dispatchplanning.TerminalResultOutcomeSuccess,
		dispatchplanning.TerminalResultOutcomeFailure,
		dispatchplanning.TerminalResultOutcomeCancelled,
	}
	for _, outcome := range outcomes {
		outcome := outcome
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			planner, result := publishedTerminalResult(t, outcome)

			first, err := planner.Retire(context.Background(), result)
			if err != nil {
				t.Fatalf("Retire(first) error = %v", err)
			}
			if first.Outcome != dispatchplanning.RetirementOutcomeRetired {
				t.Fatalf("Retire(first) outcome = %q, want RETIRED", first.Outcome)
			}
			duplicate, err := planner.Retire(context.Background(), result)
			if err != nil {
				t.Fatalf("Retire(duplicate) error = %v", err)
			}
			if duplicate.Outcome != dispatchplanning.RetirementOutcomeDuplicateIdempotent {
				t.Fatalf("Retire(duplicate) outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
			}

			intent, ok := planner.Intent(result.DispatchID)
			if !ok || intent.Status != dispatchplanning.OutboxIntentStatusRetired {
				t.Fatalf("retired intent = (%#v, %t), want RETIRED", intent, ok)
			}
			if intent.Result == nil || !reflect.DeepEqual(*intent.Result, result) {
				t.Fatalf("retained terminal result = %#v, want %#v", intent.Result, result)
			}
		})
	}
}

func TestRetireRejectsUnknownAndConflictingResultsWithoutMutation(t *testing.T) {
	t.Parallel()

	planner, accepted := publishedTerminalResult(t, dispatchplanning.TerminalResultOutcomeFailure)
	unknown := accepted
	unknown.CorrelationID = "correlation-unknown"
	if _, err := planner.Retire(context.Background(), unknown); !errors.Is(err, dispatchplanning.ErrUnknownDispatchCorrelation) {
		t.Fatalf("Retire(unknown) error = %v, want ErrUnknownDispatchCorrelation", err)
	}

	first, err := planner.Retire(context.Background(), accepted)
	if err != nil || first.Outcome != dispatchplanning.RetirementOutcomeRetired {
		t.Fatalf("Retire(first) = (%#v, %v), want RETIRED", first, err)
	}
	conflicts := []dispatchplanning.TerminalResult{
		{
			DispatchID:    "dispatch-other",
			CorrelationID: accepted.CorrelationID,
			WorkID:        accepted.WorkID,
			Outcome:       accepted.Outcome,
		},
		{
			DispatchID:    accepted.DispatchID,
			CorrelationID: accepted.CorrelationID,
			WorkID:        "work-other",
			Outcome:       accepted.Outcome,
		},
		{
			DispatchID:    accepted.DispatchID,
			CorrelationID: accepted.CorrelationID,
			WorkID:        accepted.WorkID,
			Outcome:       dispatchplanning.TerminalResultOutcomeSuccess,
		},
	}
	for _, conflict := range conflicts {
		if _, err := planner.Retire(context.Background(), conflict); !errors.Is(err, dispatchplanning.ErrInvalidDispatchResultBoundary) {
			t.Fatalf("Retire(conflict %#v) error = %v, want ErrInvalidDispatchResultBoundary", conflict, err)
		}
	}
	intent, ok := planner.Intent(accepted.DispatchID)
	if !ok || intent.Result == nil || !reflect.DeepEqual(*intent.Result, accepted) {
		t.Fatalf("first accepted result was not preserved: (%#v, %t)", intent, ok)
	}
}

func TestRetireRejectsResultForUnpublishedIntent(t *testing.T) {
	t.Parallel()

	planner := New(nil)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), action); err == nil {
		t.Fatal("Publish() error = nil, want unavailable publisher")
	}
	result := dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-1",
		CorrelationID: "correlation-1",
		WorkID:        "work-1",
		Outcome:       dispatchplanning.TerminalResultOutcomeSuccess,
	}
	if _, err := planner.Retire(context.Background(), result); !errors.Is(err, dispatchplanning.ErrInvalidDispatchResultBoundary) {
		t.Fatalf("Retire(unpublished) error = %v, want ErrInvalidDispatchResultBoundary", err)
	}
	intent, ok := planner.Intent(result.DispatchID)
	if !ok || intent.Status != dispatchplanning.OutboxIntentStatusPending || intent.Result != nil {
		t.Fatalf("unpublished intent mutated = (%#v, %t)", intent, ok)
	}
}

func TestRetireConcurrentDuplicateProducesOneCompletionOutcome(t *testing.T) {
	t.Parallel()

	planner, result := publishedTerminalResult(t, dispatchplanning.TerminalResultOutcomeSuccess)
	const submissions = 64
	type retireResponse struct {
		result dispatchplanning.RetirementResult
		err    error
	}
	responses := make(chan retireResponse, submissions)
	var wg sync.WaitGroup
	for index := 0; index < submissions; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := planner.Retire(context.Background(), result)
			responses <- retireResponse{result: got, err: err}
		}()
	}
	wg.Wait()
	close(responses)

	retired := 0
	duplicates := 0
	for response := range responses {
		if response.err != nil {
			t.Fatalf("Retire() error = %v", response.err)
		}
		switch response.result.Outcome {
		case dispatchplanning.RetirementOutcomeRetired:
			retired++
		case dispatchplanning.RetirementOutcomeDuplicateIdempotent:
			duplicates++
		default:
			t.Fatalf("Retire() outcome = %q", response.result.Outcome)
		}
	}
	if retired != 1 || duplicates != submissions-1 {
		t.Fatalf("retirement outcomes = (%d retired, %d duplicates), want (1, %d)", retired, duplicates, submissions-1)
	}
}

func publishedTerminalResult(
	t *testing.T,
	outcome dispatchplanning.TerminalResultOutcome,
) (*Planner, dispatchplanning.TerminalResult) {
	t.Helper()
	planner := New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	})
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-1",
		"correlation-1",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), action); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	return planner, dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-1",
		CorrelationID: "correlation-1",
		WorkID:        "work-1",
		Outcome:       outcome,
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
