package service_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

type countingDurableHost struct {
	durableLifecycleGatewayHost
	durableAccessors int
}

func (h *countingDurableHost) DurableExecution() factorysessionexecution.Service {
	h.durableAccessors++
	return h.execution
}

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

func TestDurableCallSitesRouteThroughOwnerCapabilityNotHostAccessor(t *testing.T) {
	t.Parallel()

	execution := &routingExecution{}
	host := &countingDurableHost{
		durableLifecycleGatewayHost: durableLifecycleGatewayHost{execution: execution},
	}
	gateway := newServiceTestGateway(host)
	if host.durableAccessors != 1 {
		t.Fatalf("host DurableExecution accessors during construction = %d, want 1", host.durableAccessors)
	}

	ctx := context.Background()
	if _, err := gateway.StartAsync(ctx, factorysessionexecution.StartRequest{RequestID: "request-route"}); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if _, err := gateway.ResumeInterruptedSession(
		ctx,
		"dur-sess-outer",
		factorysessionexecution.ResumeSessionRequest{RequestID: "resume-route"},
	); err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if _, err := gateway.GetSession(ctx, "dur-sess-outer"); err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if _, err := gateway.Pause(ctx, "dur-sess-outer", factorysessionexecution.ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := gateway.PauseDurableFactorySession(
		ctx,
		"dur-sess-js-run-n-001",
		factorysessionexecution.ControlRequest{},
	); err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}

	if host.durableAccessors != 1 {
		t.Fatalf(
			"host DurableExecution accessors after durable operations = %d, want 1 (construction only)",
			host.durableAccessors,
		)
	}
	if execution.calls != 5 {
		t.Fatalf("owner execution calls = %d, want start/resume/inspect/pause/control routing", execution.calls)
	}
	if execution.resumeCalls != 1 || execution.pauseCalls != 2 {
		t.Fatalf(
			"resume calls = %d pause calls = %d, want 1 resume and 2 pause paths",
			execution.resumeCalls,
			execution.pauseCalls,
		)
	}
}
