package invocation

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

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
