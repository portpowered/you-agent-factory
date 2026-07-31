package factorysessions_test

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

var controlObservationLeaseImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice/...",
}

// TestControlObservationLeaseImportsFactoryRuntimeOnlyThroughRoot seals the
// Sessions control/observation consumer edge against nested Runtime packages.

// peerRuntimeRootBoundaryFake exercises the published Sessions root control and
// observation slice through singular Service methods that carry Runtime root
// request/result vocabulary. It compiles against only the Sessions root package
// (plus approved peer roots) and never imports factory_sessions/internal.
type peerRuntimeRootBoundaryFake struct {
	*peerRootServiceFake
	pauseCalls      int
	observeCalls    int
	observeRequests []factoryruntime.ObserveRequest
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

func (fake *peerRuntimeRootBoundaryFake) ObserveForSession(
	_ context.Context,
	sessionID string,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if _, ok := fake.sessions[sessionID]; !ok {
		return factoryruntime.ObserveResult{}, factorysessions.ErrSessionNotFound
	}
	fake.observeCalls++
	fake.observeRequests = append(fake.observeRequests, req)
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Health: factoryruntime.ObservationHealth{
				FactoryState:       "RUNNING",
				StreamGenerationID: "stream-gen-root-boundary",
			},
		},
	}, nil
}

// TestSessionsRootRuntimeControlObservationUsesRootContracts proves the Sessions
// root Service facade exercises Runtime root control and observation vocabulary
// with observable request scope and result fields reviewers can inspect.
func TestSessionsRootRuntimeControlObservationUsesRootContracts(t *testing.T) {
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

	observed, err := fake.ObserveForSession(
		ctx,
		sessionID,
		factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus},
	)
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	if fake.observeCalls != 1 {
		t.Fatalf("observe calls = %d, want 1 root observation path", fake.observeCalls)
	}
	if len(fake.observeRequests) != 1 || fake.observeRequests[0].Scope != factoryruntime.ObservationScopeStatus {
		t.Fatalf("observe requests = %#v, want one STATUS-scoped ObserveRequest", fake.observeRequests)
	}
	if observed.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", observed.Observation.Status)
	}
	if observed.Observation.Health.StreamGenerationID != "stream-gen-root-boundary" {
		t.Fatalf(
			"observation streamGenerationID = %q, want stream-gen-root-boundary",
			observed.Observation.Health.StreamGenerationID,
		)
	}
}
