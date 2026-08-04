package stdio

import (
	"context"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestDispatchFactoryInvocation_NilResponseBridgeCallsInvokeDirectly(t *testing.T) {
	server := &Server{chatSessions: &fakeChatSessionsService{}, factoryTarget: &fakeFactoryTargetService{}}

	wantResult := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}
	invokeCalls := 0
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		invokeCalls++
		return wantResult, nil
	}

	result, err := server.dispatchFactoryInvocation(context.Background(), "session-1", 1, "factory-1", invoke)
	if err != nil {
		t.Fatalf("dispatchFactoryInvocation() error = %v, want nil", err)
	}
	if invokeCalls != 1 {
		t.Fatalf("invoke called %d times, want 1", invokeCalls)
	}
	if result.Status != wantResult.Status {
		t.Errorf("result.Status = %v, want %v", result.Status, wantResult.Status)
	}
}

func TestDispatchFactoryInvocation_NilChatSessionsOrFactoryTargetSkipsBridge(t *testing.T) {
	bridgeCalled := false
	bridge := func(
		context.Context, chatsessions.Service, acp.FactoryTargetService, string, uint64, string,
		func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		bridgeCalled = true
		return factorysessions.InvocationResult{}, nil
	}

	server := &Server{responseBridge: bridge, factoryTarget: &fakeFactoryTargetService{}}
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		return factorysessions.InvocationResult{}, nil
	}

	if _, err := server.dispatchFactoryInvocation(context.Background(), "session-1", 1, "factory-1", invoke); err != nil {
		t.Fatalf("dispatchFactoryInvocation() error = %v, want nil", err)
	}
	if bridgeCalled {
		t.Error("responseBridge was called with a nil chatSessions collaborator, want it skipped")
	}
}

func TestDispatchFactoryInvocation_CallsInjectedResponseBridge(t *testing.T) {
	var gotChatSessionID string
	var gotSessionVersion uint64
	var gotFactorySessionID string
	bridge := func(
		ctx context.Context,
		_ chatsessions.Service,
		_ acp.FactoryTargetService,
		chatSessionID string,
		sessionVersion uint64,
		factorySessionID string,
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		gotChatSessionID = chatSessionID
		gotSessionVersion = sessionVersion
		gotFactorySessionID = factorySessionID
		return invoke(ctx)
	}

	server := &Server{
		chatSessions:   &fakeChatSessionsService{},
		factoryTarget:  &fakeFactoryTargetService{},
		responseBridge: bridge,
	}

	invokeCalls := 0
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		invokeCalls++
		return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
	}

	if _, err := server.dispatchFactoryInvocation(context.Background(), "session-1", 7, "factory-1", invoke); err != nil {
		t.Fatalf("dispatchFactoryInvocation() error = %v, want nil", err)
	}
	if invokeCalls != 1 {
		t.Fatalf("invoke called %d times, want 1 (reached through the bridge)", invokeCalls)
	}
	if gotChatSessionID != "session-1" || gotSessionVersion != 7 || gotFactorySessionID != "factory-1" {
		t.Errorf("bridge called with (%q, %d, %q), want (%q, %d, %q)",
			gotChatSessionID, gotSessionVersion, gotFactorySessionID, "session-1", uint64(7), "factory-1")
	}
}
