package factorysessions_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerRuntimeRootBoundaryFake exercises the published Sessions root control and
// slice through singular Service methods. It compiles against only the Sessions root package
// (plus approved peer roots) and never imports factory_sessions/internal.
type peerRuntimeRootBoundaryFake struct {
	*peerRootServiceFake
	pauseCalls int
}

func newPeerRuntimeRootBoundaryFake() *peerRuntimeRootBoundaryFake {
	return &peerRuntimeRootBoundaryFake{
		peerRootServiceFake: newPeerRootServiceFake(),
	}
}

var _ factorysessions.Service = (*peerRuntimeRootBoundaryFake)(nil)

func (fake *peerRuntimeRootBoundaryFake) PauseLiveFactorySession(
	_ context.Context,
	sessionID string,
	_ factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	if _, ok := fake.sessions[sessionID]; !ok {
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
	}
	fake.pauseCalls++
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessions.LifecycleControlPause,
		Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
		Status:    factorysessions.LifecycleStatusPaused,
	}, nil
}

// TestSessionsRootRuntimeControlUsesRootContracts proves the Sessions root
// Service facade exercises the Runtime-owned control vocabulary.
func TestSessionsRootRuntimeControlUsesRootContracts(t *testing.T) {
	t.Parallel()

	const sessionID = "sess-root-runtime-boundary"
	fake := newPeerRuntimeRootBoundaryFake()
	fake.sessions[sessionID] = factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{FactorySessionID: sessionID},
	}
	ctx := context.Background()

	paused, err := fake.PauseLiveFactorySession(ctx, sessionID, factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if fake.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1 root control path", fake.pauseCalls)
	}
	if paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("pause outcome = %q, want ACCEPTED", paused.Outcome)
	}
	if paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", paused.Status)
	}
}
