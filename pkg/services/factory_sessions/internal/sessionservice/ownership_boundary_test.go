package service_test

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

type routingExecution struct {
	outerBoundaryExecution
	resumeCalls      int
	resumeSessionIDs []string
	pauseCalls       int
	pauseSessionIDs  []string
}

func (s *routingExecution) ResumeInterruptedSession(
	_ context.Context,
	sessionID string,
	_ factorysessionexecution.ResumeSessionRequest,
) (factorysessionexecution.AsyncStartResult, error) {
	s.resumeCalls++
	s.resumeSessionIDs = append(s.resumeSessionIDs, sessionID)
	s.calls++
	return factorysessionexecution.AsyncStartResult{SessionID: "dur-sess-outer"}, nil
}

func (s *routingExecution) Pause(
	_ context.Context,
	sessionID string,
	_ factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	s.pauseCalls++
	s.pauseSessionIDs = append(s.pauseSessionIDs, sessionID)
	s.calls++
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusPaused,
	}, nil
}

// TestDurableCapabilityDoesNotTouchCollidingLiveSession proves a caller that
// holds only the Factory Sessions-owned durable capability can resume and
// control a durable session whose logical target identifier collides with a
// live session, without traversing, pausing, resuming, or closing that live
// session. The runtime routes are deliberately observed through their real
// bounded gateway implementations rather than inferred from interface shape.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestDurableCapabilityDoesNotTouchCollidingLiveSession(t *testing.T) {
	t.Parallel()

	execution := &routingExecution{}
	liveFactory := &gatewayLifecycleFactory{}
	host := &unifiedLifecycleGatewayHost{
		lifecycleGatewayHost: lifecycleGatewayHost{factory: liveFactory},
		execution:            execution,
	}
	gateway := newServiceTestGateway(host)
	var durable factorysessions.DurableExecutionService = gateway
	const collidingSessionID = "dur-sess-colliding-logical-target"

	resumed, err := durable.ResumeInterruptedSession(
		context.Background(),
		collidingSessionID,
		factorysessions.DurableResumeRequest{RequestID: "resume-colliding-target"},
	)
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.SessionID != "dur-sess-outer" {
		t.Fatalf("resume session id = %q, want durable execution result", resumed.SessionID)
	}

	paused, err := durable.Pause(
		context.Background(),
		collidingSessionID,
		factorysessions.DurableControlRequest{RequestID: "pause-colliding-target"},
	)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.SessionID != collidingSessionID || paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("pause result = %#v, want accepted durable pause for %q", paused, collidingSessionID)
	}

	if execution.resumeCalls != 1 || len(execution.resumeSessionIDs) != 1 || execution.resumeSessionIDs[0] != collidingSessionID {
		t.Fatalf("durable resume calls = %#v, want exactly %q", execution.resumeSessionIDs, collidingSessionID)
	}
	if execution.pauseCalls != 1 || len(execution.pauseSessionIDs) != 1 || execution.pauseSessionIDs[0] != collidingSessionID {
		t.Fatalf("durable pause calls = %#v, want exactly %q", execution.pauseSessionIDs, collidingSessionID)
	}
	if len(host.sessionFactoryCalls) != 0 {
		t.Fatalf("live session factory calls = %#v, want none", host.sessionFactoryCalls)
	}
	if liveFactory.pauseCalls != 0 || liveFactory.resumeCalls != 0 || liveFactory.terminateCalls != 0 {
		t.Fatalf(
			"durable operations mutated live runtime: pause=%d resume=%d terminate=%d",
			liveFactory.pauseCalls,
			liveFactory.resumeCalls,
			liveFactory.terminateCalls,
		)
	}
	if len(host.stopCalls) != 0 {
		t.Fatalf("durable operations closed live sessions: %#v", host.stopCalls)
	}
}

func TestDirectDurableCapabilityPreservesAddressedResumeAndPauseOutcomes(t *testing.T) {
	t.Parallel()

	execution := &routingExecution{}
	host := &durableLifecycleGatewayHost{}
	registry := responsestream.NewRegistry(newServiceTestResponseStream, serviceTestClock)
	gateway := factorysessionservice.NewWithLiveChangeCoordinator(host, stream.NewManagerWithDependencies(host, host, registry), nil, nil, nil, nil, nil, execution, nil, nil, nil)

	ctx := context.Background()
	if got, err := gateway.StartAsync(ctx, factorysessionexecution.StartRequest{RequestID: "request-route"}); err != nil || got.SessionID != "dur-sess-outer" {
		t.Fatalf("StartAsync: %#v, %v", got, err)
	}
	if got, err := gateway.ResumeInterruptedSession(
		ctx,
		"dur-sess-outer",
		factorysessionexecution.ResumeSessionRequest{RequestID: "resume-route"},
	); err != nil || got.SessionID != "dur-sess-outer" {
		t.Fatalf("ResumeInterruptedSession: %#v, %v", got, err)
	}
	if got, err := gateway.GetSession(ctx, "dur-sess-outer"); err != nil || got.SessionID != "dur-sess-outer" {
		t.Fatalf("GetSession: %#v, %v", got, err)
	}
	if got, err := gateway.Pause(ctx, "dur-sess-outer", factorysessionexecution.ControlRequest{}); err != nil || got.SessionID != "dur-sess-outer" || got.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("Pause: %#v, %v", got, err)
	}
	if got, err := gateway.Pause(
		ctx,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	); err != nil || got.SessionID != "dur-sess-js-run-n-001" || got.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("Pause: %#v, %v", got, err)
	}

	if !reflect.DeepEqual(execution.resumeSessionIDs, []string{"dur-sess-outer"}) ||
		!reflect.DeepEqual(execution.pauseSessionIDs, []string{"dur-sess-outer", "dur-sess-js-run-n-001"}) {
		t.Fatalf("addressed durable effects: resume=%v pause=%v", execution.resumeSessionIDs, execution.pauseSessionIDs)
	}
}

func TestHistoricalExecutionRoutePreservesPeerAndReplacementOwnership(t *testing.T) {
	t.Parallel()
	processOwner := &routingExecution{}
	first := &routingExecution{}
	replacement := &routingExecution{}
	host := &unifiedLifecycleGatewayHost{execution: processOwner}
	gateway := newServiceTestGateway(host)
	releaseFirst := gateway.BindHistoricalExecution("recorded", first)
	if _, err := gateway.ResumeInterruptedSession(t.Context(), "peer", factorysessions.ResumeSessionRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.ResumeInterruptedSession(t.Context(), "recorded", factorysessions.ResumeSessionRequest{}); err != nil {
		t.Fatal(err)
	}
	releaseReplacement := gateway.BindHistoricalExecution("recorded", replacement)
	releaseFirst()
	if _, err := gateway.ResumeInterruptedSession(t.Context(), "recorded", factorysessions.ResumeSessionRequest{}); err != nil {
		t.Fatal(err)
	}
	if processOwner.resumeCalls != 1 || first.resumeCalls != 1 || replacement.resumeCalls != 1 {
		t.Fatal("scoped route displaced peer or replacement")
	}
	releaseReplacement()
	releaseReplacement()
	if _, err := gateway.ResumeInterruptedSession(t.Context(), "recorded", factorysessions.ResumeSessionRequest{}); err != nil {
		t.Fatal(err)
	}
	if processOwner.resumeCalls != 2 || replacement.resumeCalls != 1 {
		t.Fatal("released inspection retained execution ownership")
	}
}
