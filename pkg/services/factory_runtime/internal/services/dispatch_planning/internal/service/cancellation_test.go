package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPlanHonorsCancelledContextWithoutCreatingOutboxIntent(t *testing.T) {
	t.Parallel()

	publishCalls := 0
	planner := New(func(_ context.Context, _ workers.WorkstationDispatchRequest) error {
		publishCalls++
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := planner.Plan(ctx, dispatchplanning.PlanRequest{
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
	if publishCalls != 0 {
		t.Fatalf("Workers publisher calls = %d, want none", publishCalls)
	}
	if _, ok := planner.Intent("dispatch-1"); ok {
		t.Fatal("Plan() created an outbox intent for a cancelled context")
	}
}

func TestStopRetriesFailedWorkersCancellation(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("Workers cancellation unavailable")
	cancelCalls := 0
	var cancelledIDs []string
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			_ context.Context,
			request workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			cancelCalls++
			cancelledIDs = append(cancelledIDs, request.DispatchID)
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
	if !reflect.DeepEqual(cancelledIDs, []string{"dispatch-1", "dispatch-1"}) {
		t.Fatalf("cancellation retry dispatch IDs = %#v, want the same DispatchID twice", cancelledIDs)
	}
}

func TestStopDoesNotDoubleCancelWhileCancellationIsInFlight(t *testing.T) {
	t.Parallel()

	firstCancellationEntered := make(chan struct{})
	releaseFirstCancellation := make(chan struct{})
	secondCancellationErr := errors.New("unexpected second Workers cancellation")
	var cancelMu sync.Mutex
	cancelCalls := 0
	var cancelledIDs []string
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			_ context.Context,
			request workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			cancelMu.Lock()
			cancelCalls++
			callNumber := cancelCalls
			cancelledIDs = append(cancelledIDs, request.DispatchID)
			cancelMu.Unlock()
			if callNumber == 1 {
				close(firstCancellationEntered)
				<-releaseFirstCancellation
				return workers.WorkstationDispatchCancelResult{}, nil
			}
			return workers.WorkstationDispatchCancelResult{}, secondCancellationErr
		},
	)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-in-flight",
		"correlation-in-flight",
		"review",
		"reviewer",
		"work-1",
	))
	if _, err := planner.Publish(context.Background(), action); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	firstStop := make(chan error, 1)
	go func() {
		firstStop <- planner.Stop(context.Background(), dispatchplanning.RuntimeStopReasonCancelled)
	}()
	<-firstCancellationEntered

	secondErr := planner.Stop(context.Background(), dispatchplanning.RuntimeStopReasonCancelled)
	close(releaseFirstCancellation)
	firstErr := <-firstStop
	if secondErr != nil {
		t.Fatalf("Stop(concurrent) error = %v, want no-op while cancellation is in flight", secondErr)
	}
	if firstErr != nil {
		t.Fatalf("Stop(first) error = %v", firstErr)
	}

	cancelMu.Lock()
	defer cancelMu.Unlock()
	if cancelCalls != 1 || !reflect.DeepEqual(cancelledIDs, []string{"dispatch-in-flight"}) {
		t.Fatalf("Workers cancellations = (%d, %#v), want one exact DispatchID", cancelCalls, cancelledIDs)
	}
	intent, ok := planner.Intent("dispatch-in-flight")
	if !ok || !intent.CancellationRequested {
		t.Fatalf("cancelled intent = (%#v, %t), want CancellationRequested", intent, ok)
	}
}

func TestInvalidateWorkMarksExactIntentsAndCancelsPublishedDispatches(t *testing.T) {
	t.Parallel()

	var cancelRequests []workers.WorkstationDispatchCancelRequest
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			cancelRequests = append(cancelRequests, request)
			return workers.WorkstationDispatchCancelResult{
				DispatchID: request.DispatchID,
				Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
			}, nil
		},
	)
	if err := planner.Pause(context.Background()); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	pendingTarget := plannedAction(t, planner, runnableDecision(
		"dispatch-pending-target", "correlation-pending-target", "review", "reviewer", "work-target",
	))
	unrelated := plannedAction(t, planner, runnableDecision(
		"dispatch-unrelated", "correlation-unrelated", "review", "reviewer", "work-other",
	))
	if _, err := planner.Publish(context.Background(), pendingTarget); err != nil {
		t.Fatalf("Publish(pending target) error = %v", err)
	}
	if _, err := planner.Publish(context.Background(), unrelated); err != nil {
		t.Fatalf("Publish(unrelated) error = %v", err)
	}

	first, err := planner.InvalidateWork(context.Background(), "work-target")
	if err != nil {
		t.Fatalf("InvalidateWork(first) error = %v", err)
	}
	if first.Outcome != dispatchplanning.WorkInvalidationOutcomeInvalidated || len(first.Intents) != 1 {
		t.Fatalf("InvalidateWork(first) = %#v, want one pending target intent", first)
	}
	if first.Intents[0].PreviousStatus != dispatchplanning.OutboxIntentStatusPending ||
		first.Intents[0].Status != dispatchplanning.OutboxIntentStatusInvalidated ||
		first.Intents[0].CancellationRequested {
		t.Fatalf("pending invalidation = %#v, want INVALIDATED without cancellation", first.Intents[0])
	}
	if len(cancelRequests) != 0 {
		t.Fatalf("pending cancellation requests = %#v, want none", cancelRequests)
	}
	if _, err := planner.Retire(context.Background(), dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-pending-target",
		CorrelationID: "correlation-pending-target",
		WorkID:        "work-target",
		Outcome:       dispatchplanning.TerminalResultOutcomeCancelled,
	}); !errors.Is(err, dispatchplanning.ErrInvalidDispatchResultBoundary) {
		t.Fatalf("Retire(pending invalidated intent) error = %v, want unpublished boundary error", err)
	}

	if err := planner.Resume(context.Background()); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	publishedTarget := plannedAction(t, planner, runnableDecision(
		"dispatch-published-target", "correlation-published-target", "review", "reviewer", "work-target",
	))
	if _, err := planner.Publish(context.Background(), publishedTarget); err != nil {
		t.Fatalf("Publish(published target) error = %v", err)
	}
	retiredTarget := plannedAction(t, planner, runnableDecision(
		"dispatch-retired-target", "correlation-retired-target", "review", "reviewer", "work-target",
	))
	if _, err := planner.Publish(context.Background(), retiredTarget); err != nil {
		t.Fatalf("Publish(retired target) error = %v", err)
	}
	if _, err := planner.Retire(context.Background(), dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-retired-target",
		CorrelationID: "correlation-retired-target",
		WorkID:        "work-target",
		Outcome:       dispatchplanning.TerminalResultOutcomeSuccess,
	}); err != nil {
		t.Fatalf("Retire(retired target) error = %v", err)
	}
	duplicate, err := planner.Publish(context.Background(), publishedTarget)
	if err != nil || duplicate.Outcome != dispatchplanning.PublicationOutcomeDuplicateIdempotent {
		t.Fatalf("Publish(duplicate target) = (%#v, %v), want idempotent duplicate", duplicate, err)
	}

	second, err := planner.InvalidateWork(context.Background(), "work-target")
	if err != nil {
		t.Fatalf("InvalidateWork(second) error = %v", err)
	}
	if len(second.Intents) != 3 {
		t.Fatalf("InvalidateWork(second) intents = %#v, want pending, published, retired target intents", second.Intents)
	}
	want := []dispatchplanning.WorkInvalidationIntent{
		{
			DispatchID:            "dispatch-pending-target",
			CorrelationID:         "correlation-pending-target",
			PreviousStatus:        dispatchplanning.OutboxIntentStatusInvalidated,
			Status:                dispatchplanning.OutboxIntentStatusInvalidated,
			CancellationRequested: false,
		},
		{
			DispatchID:            "dispatch-published-target",
			CorrelationID:         "correlation-published-target",
			PreviousStatus:        dispatchplanning.OutboxIntentStatusPublished,
			Status:                dispatchplanning.OutboxIntentStatusInvalidated,
			CancellationRequested: true,
		},
		{
			DispatchID:            "dispatch-retired-target",
			CorrelationID:         "correlation-retired-target",
			PreviousStatus:        dispatchplanning.OutboxIntentStatusRetired,
			Status:                dispatchplanning.OutboxIntentStatusRetired,
			CancellationRequested: false,
		},
	}
	if !reflect.DeepEqual(second.Intents, want) {
		t.Fatalf("InvalidateWork(second) intents = %#v, want %#v", second.Intents, want)
	}
	if !reflect.DeepEqual(cancelRequests, []workers.WorkstationDispatchCancelRequest{
		{DispatchID: "dispatch-published-target", Reason: workers.WorkstationDispatchCancelReasonSuperseded},
	}) {
		t.Fatalf("published cancellation requests = %#v, want exact superseded request", cancelRequests)
	}
	unrelatedIntent, ok := planner.Intent("dispatch-unrelated")
	if !ok || unrelatedIntent.Status != dispatchplanning.OutboxIntentStatusPublished || unrelatedIntent.CancellationRequested {
		t.Fatalf("unrelated intent = (%#v, %t), want unchanged PUBLISHED without cancellation", unrelatedIntent, ok)
	}

	noMatch, err := planner.InvalidateWork(context.Background(), "work-missing")
	if err != nil {
		t.Fatalf("InvalidateWork(no match) error = %v", err)
	}
	if noMatch.Outcome != dispatchplanning.WorkInvalidationOutcomeNoOp || len(noMatch.Intents) != 0 {
		t.Fatalf("InvalidateWork(no match) = %#v, want NO_OP", noMatch)
	}
	if _, err := planner.InvalidateWork(context.Background(), ""); !errors.Is(err, dispatchplanning.ErrInvalidRunnableDecision) {
		t.Fatalf("InvalidateWork(empty) error = %v, want invalid Work ID", err)
	}
}

func TestInvalidateWorkWaitsForPublishingAndRetiresLateResult(t *testing.T) {
	t.Parallel()

	publishEntered := make(chan struct{})
	releasePublish := make(chan struct{})
	var publishOnce sync.Once
	var cancelRequests []workers.WorkstationDispatchCancelRequest
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error {
			publishOnce.Do(func() { close(publishEntered) })
			<-releasePublish
			return nil
		},
		func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			cancelRequests = append(cancelRequests, request)
			return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID}, nil
		},
	)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-publishing", "correlation-publishing", "review", "reviewer", "work-publishing",
	))
	type publishResponse struct {
		result dispatchplanning.PublicationResult
		err    error
	}
	publishDone := make(chan publishResponse, 1)
	go func() {
		result, err := planner.Publish(context.Background(), action)
		publishDone <- publishResponse{result: result, err: err}
	}()
	<-publishEntered

	invalidateDone := make(chan struct {
		result dispatchplanning.WorkInvalidationResult
		err    error
	}, 1)
	go func() {
		result, err := planner.InvalidateWork(context.Background(), "work-publishing")
		invalidation := struct {
			result dispatchplanning.WorkInvalidationResult
			err    error
		}{result: result, err: err}
		invalidateDone <- invalidation
	}()
	close(releasePublish)

	published := <-publishDone
	if published.err != nil || published.result.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("Publish(publishing) = (%#v, %v), want accepted", published.result, published.err)
	}
	invalidation := <-invalidateDone
	if invalidation.err != nil {
		t.Fatalf("InvalidateWork(publishing) error = %v", invalidation.err)
	}
	if len(invalidation.result.Intents) != 1 ||
		(invalidation.result.Intents[0].PreviousStatus != dispatchplanning.OutboxIntentStatusPublishing &&
			invalidation.result.Intents[0].PreviousStatus != dispatchplanning.OutboxIntentStatusPublished) ||
		invalidation.result.Intents[0].Status != dispatchplanning.OutboxIntentStatusInvalidated ||
		!invalidation.result.Intents[0].CancellationRequested {
		t.Fatalf("publishing invalidation = %#v, want INVALIDATED with cancellation", invalidation.result)
	}
	if !reflect.DeepEqual(cancelRequests, []workers.WorkstationDispatchCancelRequest{
		{DispatchID: "dispatch-publishing", Reason: workers.WorkstationDispatchCancelReasonSuperseded},
	}) {
		t.Fatalf("publishing cancellation requests = %#v, want one superseded request", cancelRequests)
	}

	retirement, err := planner.Retire(context.Background(), dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-publishing",
		CorrelationID: "correlation-publishing",
		WorkID:        "work-publishing",
		Outcome:       dispatchplanning.TerminalResultOutcomeCancelled,
	})
	if err != nil || retirement.Outcome != dispatchplanning.RetirementOutcomeRetired {
		t.Fatalf("Retire(late publishing result) = (%#v, %v), want RETIRED", retirement, err)
	}
}

func TestInvalidateWorkCancellationFailureLeavesGuardableIntent(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("Workers cancellation unavailable")
	cancelCalls := 0
	planner := NewWithCancellation(
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(_ context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			cancelCalls++
			return workers.WorkstationDispatchCancelResult{DispatchID: request.DispatchID}, cancelErr
		},
	)
	action := plannedAction(t, planner, runnableDecision(
		"dispatch-cancel-failure", "correlation-cancel-failure", "review", "reviewer", "work-cancel-failure",
	))
	if _, err := planner.Publish(context.Background(), action); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	invalidation, err := planner.InvalidateWork(context.Background(), "work-cancel-failure")
	if !errors.Is(err, cancelErr) {
		t.Fatalf("InvalidateWork() error = %v, want cancellation failure", err)
	}
	if len(invalidation.Intents) != 1 || invalidation.Intents[0].Status != dispatchplanning.OutboxIntentStatusInvalidated || invalidation.Intents[0].CancellationRequested {
		t.Fatalf("failed invalidation = %#v, want retained INVALIDATED intent", invalidation)
	}
	if cancelCalls != 1 {
		t.Fatalf("cancellation calls = %d, want one", cancelCalls)
	}
	retirement, err := planner.Retire(context.Background(), dispatchplanning.TerminalResult{
		DispatchID:    "dispatch-cancel-failure",
		CorrelationID: "correlation-cancel-failure",
		WorkID:        "work-cancel-failure",
		Outcome:       dispatchplanning.TerminalResultOutcomeFailure,
	})
	if err != nil || retirement.Outcome != dispatchplanning.RetirementOutcomeRetired {
		t.Fatalf("Retire(late failed result) = (%#v, %v), want RETIRED", retirement, err)
	}
	intent, ok := planner.Intent("dispatch-cancel-failure")
	if !ok || intent.Status != dispatchplanning.OutboxIntentStatusRetired || intent.Result == nil {
		t.Fatalf("retired failed intent = (%#v, %t), want result tombstone", intent, ok)
	}
}
