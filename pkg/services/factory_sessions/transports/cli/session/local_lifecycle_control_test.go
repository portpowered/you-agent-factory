package session

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNewLocalLifecycleControlsUsesLiveStopCapability(t *testing.T) {
	t.Parallel()

	const sessionID = "session-live-stop"
	root := &localLifecycleServiceStub{}
	controls := NewLocalLifecycleControls(root)
	if controls == nil {
		t.Fatal("NewLocalLifecycleControls = nil, want local controls")
	}

	var cancelOutput bytes.Buffer
	if err := controls.Cancel(LifecycleControlConfig{
		Context:   context.Background(),
		SessionID: sessionID,
		RequestID: "cancel-1",
		Reason:    "operator cancel",
		JSON:      true,
		Output:    &cancelOutput,
	}); err != nil {
		t.Fatalf("local cancel: %v", err)
	}
	if root.liveCancelCalls != 1 || root.durableCancelCalls != 0 || root.lastCancel.RequestID != "cancel-1" || root.lastCancel.Reason != "operator cancel" {
		t.Fatalf("cancel routing = live:%d durable:%d request:%#v, want live capability", root.liveCancelCalls, root.durableCancelCalls, root.lastCancel)
	}
	var cancelResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cancelOutput.Bytes(), &cancelResponse); err != nil {
		t.Fatalf("decode local cancel: %v", err)
	}
	if cancelResponse.Operation != factoryapi.FactorySessionLifecycleControlKindCancel || cancelResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("local cancel response = %#v, want accepted CANCEL", cancelResponse)
	}

	var terminateOutput bytes.Buffer
	if err := controls.Terminate(LifecycleControlConfig{
		Context:   context.Background(),
		SessionID: sessionID,
		RequestID: "terminate-1",
		Reason:    "operator terminate",
		JSON:      true,
		Output:    &terminateOutput,
	}); err != nil {
		t.Fatalf("local terminate: %v", err)
	}
	if root.liveTerminateCalls != 1 || root.durableTerminateCalls != 0 || root.lastTerminate.RequestID != "terminate-1" || root.lastTerminate.Reason != "operator terminate" {
		t.Fatalf("terminate routing = live:%d durable:%d request:%#v, want live capability", root.liveTerminateCalls, root.durableTerminateCalls, root.lastTerminate)
	}
	var terminateResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(terminateOutput.Bytes(), &terminateResponse); err != nil {
		t.Fatalf("decode local terminate: %v", err)
	}
	if terminateResponse.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate || terminateResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("local terminate response = %#v, want accepted TERMINATE", terminateResponse)
	}
}

type localLifecycleServiceStub struct {
	factorysessions.Service
	liveCancelCalls       int
	liveTerminateCalls    int
	durableCancelCalls    int
	durableTerminateCalls int
	lastCancel            factorysessions.ControlRequest
	lastTerminate         factorysessions.ControlRequest
}

func (s *localLifecycleServiceStub) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.durableCancelCalls++
	return s.liveResult(sessionID, factorysessions.LifecycleControlCancel), nil
}

func (s *localLifecycleServiceStub) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.durableTerminateCalls++
	return s.liveResult(sessionID, factorysessions.LifecycleControlTerminate), nil
}

func (s *localLifecycleServiceStub) CancelLiveFactorySession(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.liveCancelCalls++
	s.lastCancel = request
	return s.liveResult(sessionID, factorysessions.LifecycleControlCancel), nil
}

func (s *localLifecycleServiceStub) TerminateLiveFactorySession(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.liveTerminateCalls++
	s.lastTerminate = request
	return s.liveResult(sessionID, factorysessions.LifecycleControlTerminate), nil
}

func (s *localLifecycleServiceStub) liveResult(sessionID string, operation factorysessions.LifecycleControlKind) factorysessions.LifecycleControlResult {
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
		Status:    factorysessions.LifecycleStatusSucceeded,
	}
}

var _ factorysessions.Service = (*localLifecycleServiceStub)(nil)
var _ factorysessions.LiveLifecycleControlService = (*localLifecycleServiceStub)(nil)
