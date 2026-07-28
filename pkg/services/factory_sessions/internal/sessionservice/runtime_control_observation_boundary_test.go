package service_test

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

// TestSessionsRuntimeBoundaryExercisesRootControlAndObservation proves the
// Sessions gateway forwards live control and observation through Runtime root
// Service methods with observable request scope and result vocabulary.
func TestSessionsRuntimeBoundaryExercisesRootControlAndObservation(t *testing.T) {
	t.Parallel()

	runtimeFactory := &gatewayLifecycleFactory{factoryState: string(interfaces.FactoryStateRunning)}
	host := &lifecycleGatewayHost{factory: runtimeFactory}
	gateway := newServiceTestGateway(host)
	ctx := context.Background()
	const sessionID = "sess-runtime-root-boundary"

	paused, err := gateway.PauseLiveFactorySession(ctx, sessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if runtimeFactory.pauseCalls != 1 {
		t.Fatalf("ControlPause calls = %d, want 1 through Runtime root Service", runtimeFactory.pauseCalls)
	}
	if paused.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("pause outcome = %q, want ACCEPTED", paused.Outcome)
	}
	if paused.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", paused.Status)
	}

	observed, err := gateway.ObserveForSession(
		ctx,
		sessionID,
		factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeStatus},
	)
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	if runtimeFactory.observeCalls != 1 {
		t.Fatalf("Observe calls = %d, want 1 through Runtime root Service", runtimeFactory.observeCalls)
	}
	if runtimeFactory.lastObserveRequest.Scope != factoryruntime.ObservationScopeStatus {
		t.Fatalf("observe scope = %q, want STATUS", runtimeFactory.lastObserveRequest.Scope)
	}
	if observed.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation status = %q, want ACTIVE", observed.Observation.Status)
	}
	if observed.Observation.Health.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf(
			"observation factoryState = %q, want %q",
			observed.Observation.Health.FactoryState,
			interfaces.FactoryStateRunning,
		)
	}
}
