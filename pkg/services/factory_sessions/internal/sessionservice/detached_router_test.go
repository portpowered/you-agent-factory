package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type detachedRouterOwnerFake struct {
	factorysessions.Service
	name             string
	invokedSessionID string
	pausedSessionID  string
}

func (fake *detachedRouterOwnerFake) InvokeFactorySession(_ context.Context, sessionID string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	fake.invokedSessionID = sessionID
	return factorysessions.InvocationResult{SessionID: sessionID}, nil
}

func (fake *detachedRouterOwnerFake) GetFactorySession(_ context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{FactorySessionID: sessionID},
	}, nil
}

func (fake *detachedRouterOwnerFake) PauseLiveFactorySession(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	fake.pausedSessionID = sessionID
	return factorysessions.LifecycleControlResult{
		Outcome: factorysessions.LifecycleControlOutcomeAccepted,
		Status:  factorysessions.LifecycleStatusPaused,
	}, nil
}

func TestDetachedRouterRoutesSessionOperationsToOwningGateway(t *testing.T) {
	first := &detachedRouterOwnerFake{name: "first"}
	second := &detachedRouterOwnerFake{name: "second"}
	assembly := &Assembly{}
	assembly.registerDetachedGateway("session-first", first)
	assembly.registerDetachedGateway("session-second", second)

	operations, err := (&factorysessions.DetachedOperations{}).Bind(assembly)
	if err != nil {
		t.Fatalf("bind detached operations: %v", err)
	}

	for _, test := range []struct {
		name    string
		session string
		owner   *detachedRouterOwnerFake
		other   *detachedRouterOwnerFake
	}{
		{name: "first", session: "session-first", owner: first, other: second},
		{name: "second", session: "session-second", owner: second, other: first},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := operations.Get(context.Background(), factorysessions.SessionGetRequest{
				SessionID: test.session,
				Mode:      factorysessions.SessionOperationModeLive,
			})
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if got.Session.SessionID != test.session {
				t.Fatalf("session id = %q, want %q", got.Session.SessionID, test.session)
			}

			otherInvokedBefore := test.other.invokedSessionID
			if _, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{
				SessionID: test.session,
			}); err != nil {
				t.Fatalf("invoke session: %v", err)
			}
			if test.owner.invokedSessionID != test.session || test.other.invokedSessionID != otherInvokedBefore {
				t.Fatalf("invoke routing = owner %q, other %q", test.owner.invokedSessionID, test.other.invokedSessionID)
			}

			otherPausedBefore := test.other.pausedSessionID
			if _, err := operations.Control(context.Background(), factorysessions.SessionControlRequest{
				SessionID: test.session,
				Mode:      factorysessions.SessionOperationModeLive,
				Operation: factorysessions.SessionControlPause,
			}); err != nil {
				t.Fatalf("pause session: %v", err)
			}
			if test.owner.pausedSessionID != test.session || test.other.pausedSessionID != otherPausedBefore {
				t.Fatalf("pause routing = owner %q, other %q", test.owner.pausedSessionID, test.other.pausedSessionID)
			}
		})
	}

	if _, err := operations.Get(context.Background(), factorysessions.SessionGetRequest{
		SessionID: "missing",
		Mode:      factorysessions.SessionOperationModeLive,
	}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}

func TestDetachedRouterKeepsConcurrentSessionsIsolated(t *testing.T) {
	first := &detachedRouterOwnerFake{}
	second := &detachedRouterOwnerFake{}
	assembly := &Assembly{}
	assembly.registerDetachedGateway("session-first", first)
	assembly.registerDetachedGateway("session-second", second)
	operations, err := (&factorysessions.DetachedOperations{}).Bind(assembly)
	if err != nil {
		t.Fatalf("bind detached operations: %v", err)
	}

	var wait sync.WaitGroup
	errorsCh := make(chan error, 2)
	for _, sessionID := range []string{"session-first", "session-second"} {
		sessionID := sessionID
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: sessionID})
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent invoke: %v", err)
		}
	}
	if first.invokedSessionID != "session-first" || second.invokedSessionID != "session-second" {
		t.Fatalf("concurrent routing = first %q, second %q", first.invokedSessionID, second.invokedSessionID)
	}
}
