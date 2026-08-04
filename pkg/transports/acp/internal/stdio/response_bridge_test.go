package stdio

import (
	"context"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestDispatchFactoryInvocation_NilResponseBridgeCallsInvokeDirectly(t *testing.T) {
	server := &Server{chatSessions: &fakeChatSessionsService{}, factoryTarget: &fakeFactoryTargetService{}}

	wantResult := factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}
	invokeCalls := 0
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		invokeCalls++
		return wantResult, nil
	}

	result, _, err := server.dispatchFactoryInvocation(context.Background(), "conn-1", "session-1", 1, "factory-1", invoke)
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
		context.Context, string, uint64, string,
		func(context.Context),
		func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		bridgeCalled = true
		return factorysessions.InvocationResult{}, nil
	}

	server := &Server{responseBridge: bridge, factoryTarget: &fakeFactoryTargetService{}}
	invoke := func(context.Context) (factorysessions.InvocationResult, error) {
		return factorysessions.InvocationResult{}, nil
	}

	if _, _, err := server.dispatchFactoryInvocation(context.Background(), "conn-1", "session-1", 1, "factory-1", invoke); err != nil {
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
	var gotLiveDrainNonNil bool
	bridge := func(
		ctx context.Context,
		chatSessionID string,
		sessionVersion uint64,
		factorySessionID string,
		liveDrain func(context.Context),
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		gotChatSessionID = chatSessionID
		gotSessionVersion = sessionVersion
		gotFactorySessionID = factorySessionID
		gotLiveDrainNonNil = liveDrain != nil
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

	if _, _, err := server.dispatchFactoryInvocation(context.Background(), "conn-1", "session-1", 7, "factory-1", invoke); err != nil {
		t.Fatalf("dispatchFactoryInvocation() error = %v, want nil", err)
	}
	if invokeCalls != 1 {
		t.Fatalf("invoke called %d times, want 1 (reached through the bridge)", invokeCalls)
	}
	if gotChatSessionID != "session-1" || gotSessionVersion != 7 || gotFactorySessionID != "factory-1" {
		t.Errorf("bridge called with (%q, %d, %q), want (%q, %d, %q)",
			gotChatSessionID, gotSessionVersion, gotFactorySessionID, "session-1", uint64(7), "factory-1")
	}
	if !gotLiveDrainNonNil {
		t.Error("dispatchFactoryInvocation passed a nil liveDrain to the injected responseBridge, want a non-nil callback")
	}
}

// TestHandleSessionPromptLiveDrainDeliversRecordBeforeInvokeReturns proves
// genuine mid-generation delivery through the full handleSessionPrompt path:
// a record seeded onto the Chat Session topic only after the live drain's
// events.Service.Subscribe call is already listening is delivered through
// notify strictly before the wrapped Factory invoke call returns -- not
// merely "fully retained by the time the turn finishes," which is what
// streamTurnUpdates' own pre-existing post-invocation sweep already proved
// (see TestStreamTurnUpdatesDeliversSeededMessageAndSuppressesV1Fallback,
// which pre-seeds the record before dispatch starts at all). The fake
// responseBridge here plays the same role RunWithResponseBridge plays in
// production: it starts s.liveDrainTurnUpdates (via the injected liveDrain
// callback) concurrently with invoke, and only calls invoke once the seeded
// record has actually been observed and notified -- so a passing assertion
// here is only possible if liveDrainTurnUpdates genuinely delivered the
// record while the Factory dispatch was still in flight.
func TestHandleSessionPromptLiveDrainDeliversRecordBeforeInvokeReturns(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	liveDelivered := make(chan struct{})
	var mu sync.Mutex
	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		mu.Lock()
		notified = append(notified, n)
		mu.Unlock()
		if n.Update.AgentMessageChunk != nil {
			select {
			case <-liveDelivered:
			default:
				close(liveDelivered)
			}
		}
		return nil
	}

	server.responseBridge = func(
		ctx context.Context,
		chatSessionID string,
		_ uint64,
		_ string,
		liveDrain func(context.Context),
		invoke func(context.Context) (factorysessions.InvocationResult, error),
	) (factorysessions.InvocationResult, error) {
		drainCtx, cancel := context.WithCancel(ctx)
		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)
			liveDrain(drainCtx)
		}()

		eventsSvc.waitForSubscriber(t)
		eventsSvc.seed(t, chatSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("live hello"))
		// A real response bridge advances the session head immediately after it
		// sequences this record. Model that producer commit explicitly so this
		// test keeps proving live delivery rather than a fake-only event-store
		// shortcut that never makes the event acknowledgeable.
		chatSessions := server.chatSessions.(*fakeChatSessionsService)
		chatSessions.mu.Lock()
		chatSessions.getSessionResult.Session.StreamHead = 1
		chatSessions.mu.Unlock()

		select {
		case <-liveDelivered:
		case <-time.After(2 * time.Second):
			t.Fatal("liveDrain never delivered the seeded record before invoke ran")
		}

		result, err := invoke(ctx)
		cancel()
		<-drainDone
		return result, err
	}

	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(streamingTestSessionID, "hello"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1 (live delivery only, no post-invocation or V1 duplicate)", len(notified))
	}
	chunk := notified[0].Update.AgentMessageChunk
	if chunk == nil {
		t.Fatal("notification Update.AgentMessageChunk = nil, want a populated chunk")
	}
	if chunk.Content.Text == nil || chunk.Content.Text.Text != "live hello" {
		t.Fatalf("notification chunk text = %+v, want %q", chunk.Content.Text, "live hello")
	}
}
