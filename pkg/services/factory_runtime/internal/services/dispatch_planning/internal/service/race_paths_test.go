package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Done is evaluated only after Stop captured the Publishing state and its
// completion channel. This acknowledgement makes release causal, not timed.
type publicationWaitContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (ctx *publicationWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}

func TestRacePathsDispatch(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"PlanValidContext", TestPlanPreservesSchedulerOrderAndCanonicalWorkersFacts},
		{"PlanCancelledContext", TestPlanHonorsCancelledContextWithoutCreatingOutboxIntent},
		{"CancelPublishedOnce", TestCancellationBlocksPublicationCancelsVisibleIntentAndAcceptsLateResultOnce},
		{"CancelAlreadyInFlightOrRetired", racePathsCancellationGuard},
		{"MissingCancelerAndRetry", racePathsMissingCanceler},
		{"CancelFailureThenRetry", TestStopRetriesFailedWorkersCancellation},
		{"StopWaitsForPublishSuccess", TestStopRacingPublicationCancelsAfterWorkersAcceptance},
		{"StopWaitContextCancelled", racePathsStopWaitCancelled},
		{"StopWaitsForPublishFailureOrRetirement", racePathsStopWaitOutcomes},
		{"PublishCancelledContext", racePathsPublishCancelled},
		{"PublishAndRetrySuccess", TestPublishAcceptsOnceAndRejectsIdentityConflicts},
		{"PublishFailureAndRetryFailure", racePathsPublishRetries},
	} {
		t.Run(test.name, test.run)
	}
}

func racePathsCancellationGuard(t *testing.T) {
	t.Parallel()
	t.Run("in flight", TestStopDoesNotDoubleCancelWhileCancellationIsInFlight)
	for _, outcome := range []dispatchplanning.TerminalResultOutcome{
		dispatchplanning.TerminalResultOutcomeSuccess, dispatchplanning.TerminalResultOutcomeFailure,
		dispatchplanning.TerminalResultOutcomeCancelled,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			p, action := racePathsPlanner(t, func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
				func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
					t.Error("retired intent reached cancellation edge")
					return workers.WorkstationDispatchCancelResult{}, nil
				})
			if _, err := p.Publish(t.Context(), action); err != nil {
				t.Fatal(err)
			}
			result := dispatchplanning.TerminalResult{DispatchID: "dispatch", CorrelationID: "correlation", WorkID: "work", Outcome: outcome}
			if _, err := p.Retire(t.Context(), result); err != nil {
				t.Fatal(err)
			}
			// Exercise the result guard itself as well as Stop's retired no-op.
			if err := p.cancelPublished(t.Context(), p.byDispatch["dispatch"]); err != nil {
				t.Fatal(err)
			}
			if err := p.Stop(t.Context(), dispatchplanning.RuntimeStopReasonCancelled); err != nil {
				t.Fatal(err)
			}
			intent, _ := p.Intent("dispatch")
			if !reflect.DeepEqual(intent.Result, &result) || intent.CancellationRequested {
				t.Fatalf("retained result = %#v", intent)
			}
		})
	}
}

func racePathsPlanner(t *testing.T, publish dispatchplanning.WorkersPublisher, cancel dispatchplanning.WorkersCanceler) (*Planner, dispatchplanning.OutboxAction) {
	t.Helper()
	p := NewWithCancellation(publish, cancel)
	return p, plannedAction(t, p, runnableDecision("dispatch", "correlation", "review", "reviewer", "work"))
}

func racePathsMissingCanceler(t *testing.T) {
	t.Parallel()
	p, action := racePathsPlanner(t, func(context.Context, workers.WorkstationDispatchRequest) error { return nil }, nil)
	if _, err := p.Publish(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := p.Stop(t.Context(), dispatchplanning.RuntimeStopReasonCancelled); err == nil || !strings.Contains(err.Error(), "Workers canceler is unavailable") {
			t.Fatalf("missing canceler = %v", err)
		}
		intent, _ := p.Intent("dispatch")
		if intent.CancellationRequested || p.byDispatch["dispatch"].cancelling {
			t.Fatal("failed cancellation retained reservation")
		}
	}
}

func racePathsStopWaitCancelled(t *testing.T) {
	t.Parallel()
	racePathsHeldPublication(t, "cancel", nil)
}

func racePathsStopWaitOutcomes(t *testing.T) {
	t.Parallel()
	t.Run("publisher failure", func(t *testing.T) {
		t.Parallel()
		racePathsHeldPublication(t, "failure", errors.New("publication rejected"))
	})
	for _, outcome := range []dispatchplanning.TerminalResultOutcome{dispatchplanning.TerminalResultOutcomeSuccess, dispatchplanning.TerminalResultOutcomeFailure, dispatchplanning.TerminalResultOutcomeCancelled} {
		t.Run(string(outcome), func(t *testing.T) { t.Parallel(); racePathsHeldPublication(t, string(outcome), nil) })
	}
}

func racePathsHeldPublication(t *testing.T, mode string, publishErr error) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	cancelled := make(chan string, 2)
	p, action := racePathsPlanner(t, func(context.Context, workers.WorkstationDispatchRequest) error {
		close(entered)
		<-release
		return publishErr
	}, func(_ context.Context, req workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		cancelled <- req.DispatchID
		return workers.WorkstationDispatchCancelResult{}, nil
	})
	published := make(chan error, 1)
	go func() { _, err := p.Publish(t.Context(), action); published <- err }()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wait := &publicationWaitContext{Context: ctx, observed: make(chan struct{})}
	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(wait, dispatchplanning.RuntimeStopReasonCancelled) }()
	<-wait.observed
	select {
	case id := <-cancelled:
		t.Errorf("early cancellation: %s", id)
	default:
	}
	var result *dispatchplanning.TerminalResult
	if mode != "cancel" && mode != "failure" {
		result = &dispatchplanning.TerminalResult{DispatchID: "dispatch", CorrelationID: "correlation", WorkID: "work", Outcome: dispatchplanning.TerminalResultOutcome(mode)}
		if _, err := p.Retire(t.Context(), *result); err != nil {
			t.Error(err)
		}
	}
	if mode == "cancel" {
		cancel()
		if err := <-stopped; !errors.Is(err, context.Canceled) {
			t.Errorf("stop cancellation = %v", err)
		}
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-published; !errors.Is(err, publishErr) {
		t.Fatalf("publish = %v, want %v", err, publishErr)
	}
	if mode != "cancel" {
		if err := <-stopped; err != nil {
			t.Fatal(err)
		}
	}
	racePathsJoinedPublication(t, p, mode, result, cancelled)
}

func racePathsJoinedPublication(t *testing.T, p *Planner, mode string, result *dispatchplanning.TerminalResult, cancelled chan string) {
	t.Helper()
	intent, _ := p.Intent("dispatch")
	if mode == "cancel" {
		if intent.Status != dispatchplanning.OutboxIntentStatusPublished || intent.CancellationRequested {
			t.Fatalf("after wait cancelled: %#v", intent)
		}
		if err := p.Stop(t.Context(), dispatchplanning.RuntimeStopReasonCancelled); err != nil {
			t.Fatal(err)
		}
		if id := <-cancelled; id != "dispatch" {
			t.Fatalf("cancelled %s", id)
		}
	} else if result != nil {
		if intent.Status != dispatchplanning.OutboxIntentStatusRetired || !reflect.DeepEqual(intent.Result, result) {
			t.Fatalf("retirement overwritten: %#v", intent)
		}
	} else if intent.Status != dispatchplanning.OutboxIntentStatusPending {
		t.Fatalf("failed publication: %#v", intent)
	}
	select {
	case id := <-cancelled:
		t.Fatalf("unexpected cancellation: %s", id)
	default:
	}
}

func racePathsPublishCancelled(t *testing.T) {
	t.Parallel()
	calls := 0
	var expected workers.WorkstationDispatchRequest
	p, action := racePathsPlanner(t, func(_ context.Context, req workers.WorkstationDispatchRequest) error {
		calls++
		if !reflect.DeepEqual(req, expected) {
			t.Error("retry changed request")
		}
		return nil
	}, nil)
	expected = action.Request
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := p.Publish(ctx, action); !errors.Is(err, context.Canceled) || result.Outcome != dispatchplanning.PublicationOutcomeAccepted {
		t.Fatalf("publication = %#v, %v", result, err)
	}
	intent, _ := p.Intent("dispatch")
	if calls != 0 || intent.Attempts != 1 || intent.Status != dispatchplanning.OutboxIntentStatusPending {
		t.Fatalf("cancelled publication: %#v, calls=%d", intent, calls)
	}
	if _, err := p.Retry(t.Context(), "dispatch"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("retry calls = %d", calls)
	}
}

func racePathsPublishRetries(t *testing.T) {
	t.Parallel()
	failures := []error{errors.New("first failure"), errors.New("retry failure"), nil}
	var requests []workers.WorkstationDispatchRequest
	p, action := racePathsPlanner(t, func(_ context.Context, req workers.WorkstationDispatchRequest) error {
		requests = append(requests, req)
		return failures[len(requests)-1]
	}, nil)
	for i, wantErr := range failures {
		var err error
		if i == 0 {
			_, err = p.Publish(t.Context(), action)
		} else {
			_, err = p.Retry(t.Context(), "dispatch")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
		intent, _ := p.Intent("dispatch")
		wantStatus := dispatchplanning.OutboxIntentStatusPending
		if wantErr == nil {
			wantStatus = dispatchplanning.OutboxIntentStatusPublished
		}
		if intent.Status != wantStatus || intent.Attempts != i+1 || !reflect.DeepEqual(requests[i], action.Request) {
			t.Fatalf("attempt %d: %#v request=%#v", i+1, intent, requests[i])
		}
	}
	if _, err := p.Retry(t.Context(), "dispatch"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Publish(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatal("published intent retried")
	}
}
