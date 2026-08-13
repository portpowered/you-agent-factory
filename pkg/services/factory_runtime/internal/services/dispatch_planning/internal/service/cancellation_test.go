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
