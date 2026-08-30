package invocation

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
