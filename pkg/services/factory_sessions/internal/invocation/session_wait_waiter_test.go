package invocation

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSessionOwnerWait_JoinedCancelResolvesRetryablePrimary(t *testing.T) {
	observation := stoppedSessionInvocationObservation()
	request := observation.WorldState.WorkRequestsByID["request-1"]
	request.TraceID = "invocation-trace"
	observation.WorldState.WorkRequestsByID["request-1"] = request
	root := observation.WorldState.WorkRequestsByID["request-1"].WorkItems[0]
	observation.WorldState.WorkItemsByID[root.ID] = root
	observation.ActiveWork = true
	observation.WorldState.CompletedDispatches = []interfaces.FactoryWorldDispatchCompletion{{
		DispatchID: "joined-dispatch", WorkItemIDs: []string{root.ID}, TraceIDs: []string{observation.WorldState.WorkRequestsByID["request-1"].TraceID},
		Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeCanceled), Cancellation: &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}},
	}}
	result := waitForSessionOwnerObservation(t, observation, nil)
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusFailed)
	assertSessionOwnerEqual(t, "code", result.ErrorCode, string(work.PrimaryResultErrorCodeInterrupted))
	assertSessionOwnerEqual(t, "Work", result.WorkID, root.ID)
	assertSessionOwnerEqual(t, "dispatch", result.DispatchID, "joined-dispatch")
	if len(result.PrimaryResult) != 0 || observation.WorldState.WorkItemsByID[root.ID].State != root.State {
		t.Fatal("interruption changed retryable Work or fabricated primary content")
	}
}

func TestSessionOwnerWait_JoinedCancelIgnoresPeersAndSupersededAttempts(t *testing.T) {
	for _, name := range []string{"unrelated Work", "prior invocation trace", "active retry", "accepted retry", "superseded", "uncorrelated cancellation"} {
		t.Run(name, func(t *testing.T) {
			observation := stoppedSessionInvocationObservation()
			request := observation.WorldState.WorkRequestsByID["request-1"]
			request.TraceID = "invocation-trace"
			observation.WorldState.WorkRequestsByID["request-1"] = request
			root := observation.WorldState.WorkRequestsByID["request-1"].WorkItems[0]
			completion := interfaces.FactoryWorldDispatchCompletion{
				DispatchID: "old-dispatch", WorkItemIDs: []string{root.ID}, TraceIDs: []string{observation.WorldState.WorkRequestsByID["request-1"].TraceID},
				Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeCanceled), Cancellation: &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}},
			}
			switch name {
			case "unrelated Work":
				completion.WorkItemIDs = []string{"peer"}
				completion.TraceIDs = []string{"peer-trace"}
			case "prior invocation trace":
				completion.TraceIDs = []string{"old-invocation-trace"}
			case "active retry":
				observation.WorldState.ActiveDispatches = map[string]interfaces.FactoryWorldDispatch{"retry": {WorkItemIDs: []string{root.ID}, TraceIDs: completion.TraceIDs}}
			case "superseded":
				completion.Result.Cancellation.Reason = workers.DispatchCancellationReasonSuperseded
			case "uncorrelated cancellation":
				completion.Result.Cancellation = nil
			}
			observation.WorldState.CompletedDispatches = []interfaces.FactoryWorldDispatchCompletion{completion}
			if name == "accepted retry" {
				observation.WorldState.CompletedDispatches = append(observation.WorldState.CompletedDispatches, interfaces.FactoryWorldDispatchCompletion{DispatchID: "retry", WorkItemIDs: []string{root.ID}, TraceIDs: completion.TraceIDs, Result: interfaces.WorkstationResult{Outcome: string(workers.OutcomeAccepted)}})
			}
			if got := joinedInvocationInterruption("session", SessionInvocationWaitInput{RequestID: "request-1"}, observation.WorldState); got != nil {
				t.Fatalf("stale or unrelated cancellation resolved invocation: %+v", got)
			}
		})
	}
}

func TestSessionOwnerWait_WaitSessionWaiterIsPreferredAndReleasedOnce(t *testing.T) {
	observations := 0
	waiterCalls := 0
	releases := 0
	waitNextCalls := 0
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			observations++
			if observations >= 3 {
				return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
			}
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(context.Context) error {
			waitNextCalls++
			return nil
		},
		WaitSession: func(ctx context.Context, sessionID string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter) {
			if sessionID != "session-1" {
				t.Fatalf("wait session ID = %q, want %q", sessionID, "session-1")
			}
			return func(context.Context) error {
					waiterCalls++
					return nil
				}, func() {
					releases++
				}
		},
	})

	result, err := owner.waitForResult(context.Background(), "session-1", sessionWaitInput(nil))
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
	assertSessionOwnerEqual(t, "observations", observations, 3)
	assertSessionOwnerEqual(t, "session waiter calls", waiterCalls, 2)
	assertSessionOwnerEqual(t, "session waiter releases", releases, 1)
	assertSessionOwnerEqual(t, "fallback waitNext calls", waitNextCalls, 0)
}

func TestSessionOwnerWait_NilSessionWaiterFallsBackToWaitNext(t *testing.T) {
	observations := 0
	waitNextCalls := 0
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			observations++
			if observations >= 2 {
				return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
			}
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(context.Context) error {
			waitNextCalls++
			return nil
		},
		WaitSession: func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter) {
			return nil, nil
		},
	})

	result, err := owner.waitForResult(context.Background(), "session-1", sessionWaitInput(nil))
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
	assertSessionOwnerEqual(t, "fallback waitNext calls", waitNextCalls, 1)
}

func TestSessionOwnerWait_SessionWaiterWithNilReleaseCompletes(t *testing.T) {
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return completedSessionInvocationObservation("request-1", "trace-1", "done"), nil
		},
		WaitSession: func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter) {
			return func(context.Context) error { return nil }, nil
		},
	})

	result, err := owner.waitForResult(context.Background(), "session-1", sessionWaitInput(nil))
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusCompleted)
}

func TestSessionOwnerWait_CancelOnTimeoutControlReceivesBoundedDeadline(t *testing.T) {
	var controlCtx context.Context
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(context.Context) error { return context.DeadlineExceeded },
		CancelOnTimeout: func(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			controlCtx = ctx
			return factorysessions.LifecycleControlResult{Status: factorysessions.LifecycleStatusCanceling}, nil
		},
	})
	input := sessionWaitInput(nil)
	input.CancelOnTimeout = true

	result, err := owner.waitForResult(context.Background(), "session-1", input)
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusTimedOut)
	if controlCtx == nil {
		t.Fatal("cancel-on-timeout control was not called")
	}
	deadline, ok := controlCtx.Deadline()
	if !ok {
		t.Fatal("cancel-on-timeout context has no deadline")
	}
	if remaining := time.Until(deadline); remaining > sessionTimeoutCancelControlTimeout || remaining < sessionTimeoutCancelControlTimeout-time.Second {
		t.Fatalf("cancel-on-timeout deadline = %v from now, want the configured %v bound", remaining, sessionTimeoutCancelControlTimeout)
	}
	if controlCtx.Err() == nil {
		t.Fatal("cancel-on-timeout context was not released after the control call returned")
	}
}

func TestSessionOwnerWait_CancelOnTimeoutControlDeadlineUnblocksStalledCallback(t *testing.T) {
	previous := sessionTimeoutCancelControlTimeout
	sessionTimeoutCancelControlTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sessionTimeoutCancelControlTimeout = previous })

	controlDone := make(chan error, 1)
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(context.Context) error { return context.DeadlineExceeded },
		CancelOnTimeout: func(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			<-ctx.Done()
			controlDone <- ctx.Err()
			return factorysessions.LifecycleControlResult{}, ctx.Err()
		},
	})
	input := sessionWaitInput(nil)
	input.CancelOnTimeout = true

	started := time.Now()
	result, err := owner.waitForResult(context.Background(), "session-1", input)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("waitForResult: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("waitForResult returned after %v, want a prompt timed-out result once the control deadline expired", elapsed)
	}
	controlErr := <-controlDone
	if !errors.Is(controlErr, context.DeadlineExceeded) {
		t.Fatalf("cancel-on-timeout context error = %v, want %v", controlErr, context.DeadlineExceeded)
	}
	assertSessionOwnerEqual(t, "status", result.Status, interfaces.InvocationTerminalStatusTimedOut)
	assertSessionOwnerEqual(t, "error code", result.ErrorCode, string(interfaces.InvocationErrorCodeTimedOut))
	assertSessionOwnerEqual(t, "message", result.Message, "invocation timed out while waiting for primary result; cancel-on-timeout control failed")
}

func TestSessionOwnerWait_CancelOnTimeoutReturnsWhenControlIgnoresContext(t *testing.T) {
	previous := sessionTimeoutCancelControlTimeout
	sessionTimeoutCancelControlTimeout = 30 * time.Millisecond
	t.Cleanup(func() { sessionTimeoutCancelControlTimeout = previous })
	release := make(chan struct{})
	finished := make(chan struct{})
	owner := newTestSessionOwner(sessionOwnerFixture{
		Observe: func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error) {
			return activeSessionInvocationObservation(), nil
		},
		WaitNext: func(context.Context) error { return context.DeadlineExceeded },
		CancelOnTimeout: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			defer close(finished)
			<-release // Simulates a lifecycle gateway that ignores context cancellation.
			return factorysessions.LifecycleControlResult{}, nil
		},
	})
	input := sessionWaitInput(nil)
	input.CancelOnTimeout = true
	returned := make(chan FactoryInvocationResult, 1)
	go func() {
		result, _ := owner.waitForResult(context.Background(), "session-1", input)
		returned <- result
	}()
	defer func() {
		close(release)
		<-finished
	}()
	select {
	case result := <-returned:
		if result.Status != interfaces.InvocationTerminalStatusTimedOut {
			t.Fatalf("timeout result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("non-cooperative cancel control held the terminal timeout result")
	}
}
