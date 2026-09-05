package service

import (
	"context"
	"errors"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func TestService_CanonicalLiveControlAndResponseRouting(t *testing.T) {
	t.Parallel()

	service, live, response := canonicalLiveControlFixture()
	assertCanonicalLivePause(t, service, live)
	assertCanonicalLiveCancel(t, service, live)
	assertCanonicalLiveTerminate(t, service, live)
	assertCanonicalLiveResponseSubscription(t, service, live, response)
}

func canonicalLiveControlFixture() (*Service, *canonicalInspectionLiveRuntimeFake, *canonicalInspectionResponseStreamFake) {
	const sessionID = "live-control"
	liveSession := &livesession.LiveSession{
		ID:             sessionID,
		ResponseEvents: responseeventstore.NewSessionResponseEventStore(sessionID, platformclock.Real{}, func() string { return "event-1" }),
	}
	live := &canonicalInspectionLiveRuntimeFake{
		resolved: map[string]*livesession.LiveSession{sessionID: liveSession},
		controlResult: factorysessions.LifecycleControlResult{
			Outcome: factorysessions.LifecycleControlOutcomeAccepted,
			Status:  factorysessions.LifecycleStatusPaused,
		},
	}
	response := &canonicalInspectionResponseStreamFake{cursor: &factorysessions.ResponseEventCursor{}}
	return &Service{liveRuntime: live, responseEvents: response}, live, response
}

func assertCanonicalLivePause(t *testing.T, service *Service, live *canonicalInspectionLiveRuntimeFake) {
	t.Helper()
	paused, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: " live-control ", Mode: factorysessions.SessionOperationModeLive,
		Operation:   factorysessions.SessionControlPause,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "control-1", TurnID: "turn-1"},
	})
	if err != nil {
		t.Fatalf("canonical live pause: %v", err)
	}
	if paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted || paused.Status != factorysessions.LifecycleStatusPaused || paused.Closed {
		t.Fatalf("live pause = %#v, want typed lifecycle result", paused)
	}
	live.mu.Lock()
	controlCalls, closeCalls, operation, control := live.controlCalls, live.closeCalls, live.lastOperation, live.lastControl
	live.mu.Unlock()
	if controlCalls != 1 || closeCalls != 0 || operation != factorysessions.LifecycleControlKind(factorysessions.SessionControlPause) || control.RequestID != "control-1" || control.TurnID != "turn-1" {
		t.Fatalf("live pause owner call = control:%d close:%d operation:%q request:%#v, want direct pause owner", controlCalls, closeCalls, operation, control)
	}
}

func assertCanonicalLiveCancel(t *testing.T, service *Service, live *canonicalInspectionLiveRuntimeFake) {
	t.Helper()
	cancelled, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: "live-control", Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlCancel,
	})
	if err != nil {
		t.Fatalf("canonical live cancel: %v", err)
	}
	if cancelled.Closed || cancelled.Operation != factorysessions.SessionControlCancel || cancelled.Status != factorysessions.LifecycleStatusCanceled {
		t.Fatalf("live cancel = %#v, want typed lifecycle control result", cancelled)
	}
	live.mu.Lock()
	controlCalls, closeCalls, operation := live.controlCalls, live.closeCalls, live.lastOperation
	live.mu.Unlock()
	if controlCalls != 2 || closeCalls != 0 || operation != factorysessions.LifecycleControlKind(factorysessions.SessionControlCancel) {
		t.Fatalf("live cancel owner calls = control:%d close:%d operation:%q, want direct cancel control", controlCalls, closeCalls, operation)
	}
}

func assertCanonicalLiveTerminate(t *testing.T, service *Service, live *canonicalInspectionLiveRuntimeFake) {
	t.Helper()
	terminated, err := service.Control(context.Background(), factorysessions.SessionControlRequest{
		SessionID: "live-control", Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlTerminate,
	})
	if err != nil {
		t.Fatalf("canonical live terminate: %v", err)
	}
	if terminated.Closed || terminated.Operation != factorysessions.SessionControlTerminate || terminated.Status != factorysessions.LifecycleStatusTerminated {
		t.Fatalf("live terminate = %#v, want typed lifecycle control result", terminated)
	}
	live.mu.Lock()
	controlCalls, closeCalls, operation := live.controlCalls, live.closeCalls, live.lastOperation
	live.mu.Unlock()
	if controlCalls != 3 || closeCalls != 0 || operation != factorysessions.LifecycleControlKind(factorysessions.SessionControlTerminate) {
		t.Fatalf("live terminate owner calls = control:%d close:%d operation:%q, want direct terminate control", controlCalls, closeCalls, operation)
	}
}

func assertCanonicalLiveResponseSubscription(t *testing.T, service *Service, live *canonicalInspectionLiveRuntimeFake, response *canonicalInspectionResponseStreamFake) {
	t.Helper()
	subscription, err := service.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: "live-control", AfterSequence: 0, Kinds: []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("canonical live SubscribeResponses: %v", err)
	}
	if subscription.Cursor != response.cursor {
		t.Fatal("canonical live SubscribeResponses did not return live owner cursor")
	}
	live.mu.Lock()
	closeCalls := live.closeCalls
	live.mu.Unlock()
	response.mu.Lock()
	responseCalls := response.calls
	response.mu.Unlock()
	if closeCalls != 0 || responseCalls != 1 {
		t.Fatalf("live response owner calls = close:%d response:%d, want response owner without close", closeCalls, responseCalls)
	}
}
func TestService_CanonicalOperationsRejectInvalidValuesBeforeDependencies(t *testing.T) {
	t.Parallel()

	durable := &canonicalDurableExecutionFake{}
	service := &Service{durable: durable}
	invalidStarts := []factorysessions.SessionStartRequest{
		{Mode: factorysessions.SessionOperationMode("invalid")},
		{Mode: factorysessions.SessionOperationModeDurable, Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"}, Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}},
		{Mode: factorysessions.SessionOperationModeDurable},
		{Mode: factorysessions.SessionOperationModeLive, ValidateOnly: true, InitNewFactory: true},
	}
	for index, request := range invalidStarts {
		if _, err := service.Start(context.Background(), request); err == nil {
			t.Fatalf("invalid Start case %d returned nil error", index)
		}
	}
	if durable.asyncCalls != 0 || durable.syncCalls != 0 {
		t.Fatalf("invalid Start calls = async:%d sync:%d, want zero", durable.asyncCalls, durable.syncCalls)
	}

	invoker := &canonicalSessionInvokerFake{}
	service.invoker = invoker
	invalidInvokes := []factorysessions.SessionInvokeRequest{
		{},
		{SessionID: "session", Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}},
	}
	for index, request := range invalidInvokes {
		if _, err := service.Invoke(context.Background(), request); err == nil {
			t.Fatalf("invalid Invoke case %d returned nil error", index)
		}
	}
	if invoker.calls != 0 {
		t.Fatalf("invalid Invoke calls = %d, want zero", invoker.calls)
	}
}

func TestService_CanonicalOperationsReportTypedAvailabilityFailures(t *testing.T) {
	t.Parallel()

	service := &Service{}
	if _, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"},
	}); !errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured) {
		t.Fatalf("Start() error = %v, want durable availability error", err)
	}
	if _, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "session"}); err == nil {
		t.Fatal("Invoke() error = nil, want invocation availability error")
	}
}

func TestService_CanonicalOperationsPropagateOwnerFailuresWithoutLegacyCalls(t *testing.T) {
	t.Parallel()

	startFailure := errors.New("durable request identity conflict")
	durable := &canonicalDurableExecutionFake{asyncErr: startFailure}
	service := &Service{durable: durable}
	if _, err := service.Start(context.Background(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"},
	}); !errors.Is(err, startFailure) {
		t.Fatalf("Start() error = %v, want owner failure %v", err, startFailure)
	}
	if durable.canonicalCalls != 1 || durable.asyncCalls != 0 || durable.syncCalls != 0 {
		t.Fatalf("start calls = canonical:%d async:%d sync:%d, want 1/0/0", durable.canonicalCalls, durable.asyncCalls, durable.syncCalls)
	}

	invokeFailure := errors.New("invocation dependency failed")
	invoker := &canonicalSessionInvokerFake{err: invokeFailure}
	service.invoker = invoker
	if _, err := service.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "session"}); !errors.Is(err, invokeFailure) {
		t.Fatalf("Invoke() error = %v, want owner failure %v", err, invokeFailure)
	}
	if invoker.canonicalCalls != 1 || invoker.legacyCalls != 0 {
		t.Fatalf("invoke calls = canonical:%d legacy:%d, want 1/0", invoker.canonicalCalls, invoker.legacyCalls)
	}
}
func assertCanonicalDurableDispatchAndResponseReads(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) {
	t.Helper()
	dispatches, lastDispatch := readCanonicalDispatches(t, service, durable)
	subscription, responseRequest := readCanonicalResponses(t, service, durable)
	if subscription.Cursor != durable.cursor {
		t.Fatal("canonical durable SubscribeResponses did not return owner cursor")
	}
	if lastDispatch.Filters.Status != factorysessions.DispatchStatus("COMPLETED") {
		t.Fatalf("dispatch filter = %#v, want normalized status", lastDispatch.Filters)
	}
	if responseRequest.SessionID != "durable-dispatch" || responseRequest.AfterSequence != 4 || responseRequest.DispatchID != "dispatch-1" {
		t.Fatalf("response request = %#v, want normalized cursor/filter", responseRequest)
	}
	if dispatches.Dispatches[0].Warnings[0].Code != "warning" {
		t.Fatal("canonical QueryDispatches returned owner-owned warning data")
	}
	assertCanonicalDurableInspectionCalls(t, durable)
}

func readCanonicalDispatches(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) (factorysessions.ListDispatchesResult, factorysessions.DispatchQueryRequest) {
	t.Helper()
	dispatches, err := service.QueryDispatches(context.Background(), factorysessions.DispatchQueryRequest{
		SessionID: " durable-dispatch ", Filters: factorysessions.DispatchFilters{Status: " completed "},
	})
	if err != nil {
		t.Fatalf("canonical QueryDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want one", len(dispatches.Dispatches))
	}
	if dispatches.Dispatches[0].ID != "dispatch-1" {
		t.Fatalf("dispatch ID = %q, want dispatch-1", dispatches.Dispatches[0].ID)
	}
	if dispatches.Dispatches[0].Usage == nil || dispatches.Dispatches[0].Usage.TotalTokens != 5 {
		t.Fatalf("dispatch usage = %#v, want five tokens", dispatches.Dispatches[0].Usage)
	}
	durable.mu.Lock()
	durable.dispatches.Dispatches[0].Warnings[0].Code = "owner mutation"
	lastDispatch := durable.lastDispatch
	durable.mu.Unlock()
	return dispatches, lastDispatch
}

func readCanonicalResponses(t *testing.T, service *Service, durable *canonicalInspectionDurableFake) (factorysessions.SessionResponseSubscriptionResult, factorysessions.ResponseEventSubscriptionRequest) {
	t.Helper()
	subscription, err := service.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: " durable-dispatch ", AfterSequence: 4, DispatchID: " dispatch-1 ", Kinds: []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("canonical durable SubscribeResponses: %v", err)
	}
	durable.mu.Lock()
	responseCalls, responseRequest := durable.responseCalls, durable.lastResponse
	durable.mu.Unlock()
	if responseCalls != 1 {
		t.Fatalf("response owner calls = %d, want one", responseCalls)
	}
	return subscription, responseRequest
}

func assertCanonicalDurableInspectionCalls(t *testing.T, durable *canonicalInspectionDurableFake) {
	t.Helper()
	durable.mu.Lock()
	resultCalls, dispatchCalls, legacyCalls := durable.resultCalls, durable.dispatchCalls, durable.legacyCalls
	durable.mu.Unlock()
	if resultCalls != 1 {
		t.Fatalf("result owner calls = %d, want one", resultCalls)
	}
	if dispatchCalls != 1 {
		t.Fatalf("dispatch owner calls = %d, want one", dispatchCalls)
	}
	if legacyCalls != 0 {
		t.Fatalf("legacy owner calls = %d, want zero", legacyCalls)
	}
}
