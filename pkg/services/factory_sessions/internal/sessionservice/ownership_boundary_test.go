package service_test

import (
	"context"
	"testing"

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
	resumeCalls int
	pauseCalls  int
}

func (s *routingExecution) ResumeInterruptedSession(
	context.Context,
	string,
	factorysessionexecution.ResumeSessionRequest,
) (factorysessionexecution.AsyncStartResult, error) {
	s.resumeCalls++
	s.calls++
	return factorysessionexecution.AsyncStartResult{SessionID: "dur-sess-outer"}, nil
}

func (s *routingExecution) Pause(
	_ context.Context,
	sessionID string,
	_ factorysessionexecution.ControlRequest,
) (factorysessionexecution.LifecycleControlResult, error) {
	s.pauseCalls++
	s.calls++
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:    factorysessionexecution.LifecycleStatusPaused,
	}, nil
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
