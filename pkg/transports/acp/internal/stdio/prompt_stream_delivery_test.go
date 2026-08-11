package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	acpsdk "github.com/coder/acp-go-sdk"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/mapping"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStreamTurnUpdatesDeliversSeededMessageAndSuppressesV1Fallback proves a
// canonical MESSAGE record already sequenced onto the Chat Session topic
// before a turn dispatches is delivered as exactly one agent_message_chunk
// through streaming, and that the V1 synchronous final-text fallback never
// also fires -- even though the fake Factory Sessions outcome carries its
// own non-empty primary-result text.
func TestStreamTurnUpdatesDeliversSeededMessageAndSuppressesV1Fallback(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("streamed hello"))

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(streamingTestSessionID, "hello"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1 (streamed only, no V1 duplicate)", len(notified))
	}
	chunk := notified[0].Update.AgentMessageChunk
	if chunk == nil {
		t.Fatal("notification Update.AgentMessageChunk = nil, want a populated chunk")
	}
	if chunk.Content.Text == nil || chunk.Content.Text.Text != "streamed hello" {
		t.Fatalf("notification chunk text = %+v, want %q", chunk.Content.Text, "streamed hello")
	}
}

// TestStreamTurnUpdatesPreservesStopReasonIndependentOfDelivery proves that
// a genuinely canceled Factory outcome still reports StopReasonCancelled on
// the terminal prompt response even though a canonical message record
// streamed successfully for the same turn -- delivering canonical updates
// never fabricates a false end_turn success over the downstream outcome's
// own truthful terminal status.
func TestStreamTurnUpdatesPreservesStopReasonIndependentOfDelivery(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCanceled}}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("partial before cancel"))

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(streamingTestSessionID, "hello"))

	result, rpcErr := server.handleSessionPrompt(ctx, env)
	if rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}
	if len(notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1 (the streamed message)", len(notified))
	}

	var resp acpsdk.PromptResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if resp.StopReason != acpsdk.StopReasonCancelled {
		t.Fatalf("StopReason = %q, want %q (streamed delivery must not mask a genuine cancellation)", resp.StopReason, acpsdk.StopReasonCancelled)
	}
}

// TestStreamTurnUpdatesFallsBackWhenTopicEmpty proves that with no canonical
// record sequenced onto the topic, the existing V1 synchronous final-text
// notification still fires exactly once -- streaming degrades to a no-op
// against a real, empty Events topic rather than fabricating anything.
func TestStreamTurnUpdatesFallsBackWhenTopicEmpty(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, _ := newStreamingTestServer(t, factoryTarget, "turn-1")

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(streamingTestSessionID, "hello"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1 (the V1 fallback)", len(notified))
	}
	chunk := notified[0].Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || chunk.Content.Text.Text != "v1 fallback text" {
		t.Fatalf("notification = %+v, want the V1 fallback text", notified[0])
	}
}

// TestStreamTurnUpdatesOrdersRecordsByAggregateSequence proves two seeded
// records -- a MESSAGE then a REASONING record -- are delivered in the
// order they were sequenced, not source-kind or notification order.
func TestStreamTurnUpdatesOrdersRecordsByAggregateSequence(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	eventsSvc.seed(t, streamingTestSessionID, workers.KindReasoning, workers.PhaseCompleted, workers.ReasoningPayload{Summary: "thinking..."})
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("final answer"))

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt,
		promptTextParams(streamingTestSessionID, "hello"))

	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2", len(notified))
	}
	if notified[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the REASONING record delivered first (sequenced first)", notified[0])
	}
	if notified[1].Update.AgentMessageChunk == nil {
		t.Fatalf("notification[1] = %+v, want the MESSAGE record delivered second (sequenced second)", notified[1])
	}
}

// TestStreamTurnUpdatesNotifierFailureLeavesRecordRetryable proves that a
// notifier failure part-way through a drain fails the turn without
// acknowledging the record that failed, so a later turn's drain (on the
// same connection, same attachment cursor) redelivers it instead of
// silently skipping it.
func TestStreamTurnUpdatesNotifierFailureLeavesRecordRetryable(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1", "turn-2")
	connID := identity.NewConnectionID()

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("retry me"))

	failingNotify := func(acpsdk.SessionNotification) error {
		return errors.New("notify boom")
	}
	cache := &attachmentCache{}
	firstCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), failingNotify), cache)
	firstEnv := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "first"))
	if _, rpcErr := server.handleSessionPrompt(firstCtx, firstEnv); rpcErr == nil {
		t.Fatal("first handleSessionPrompt() error = nil, want a bounded failure from the injected notifier fault")
	}

	var secondNotified []acpsdk.SessionNotification
	secondNotify := func(n acpsdk.SessionNotification) error {
		secondNotified = append(secondNotified, n)
		return nil
	}
	secondCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), secondNotify), cache)
	secondEnv := numberIdentityEnvelope(t, connID, 2, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "second"))
	if _, rpcErr := server.handleSessionPrompt(secondCtx, secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want success once the notifier stops failing", rpcErr)
	}

	if len(secondNotified) != 1 {
		t.Fatalf("second turn notify call count = %d, want exactly 1 (the record the first turn failed to deliver)", len(secondNotified))
	}
	chunk := secondNotified[0].Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || chunk.Content.Text.Text != "retry me" {
		t.Fatalf("second turn notification = %+v, want the record the first turn never acknowledged", secondNotified[0])
	}
}
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestDeliverBoundWorkerChildProjectionPreservesNotificationOutcomes(t *testing.T) {
	item, update := projectedWorkerChildMessage(t)
	notifyErr := errors.New("notification failed")

	t.Run("no update is a no-op", func(t *testing.T) {
		cache := &attachmentCache{}
		bounded, err := cache.deliverBoundWorkerChildProjection("session", item, nil, nil)
		if err != nil || bounded != nil {
			t.Fatalf("deliverBoundWorkerChildProjection(nil) = (%#v, %v), want (nil, nil)", bounded, err)
		}
		if bounded, err := cache.boundWorkerChildProjection("session", item, nil); err != nil || bounded != nil {
			t.Fatalf("boundWorkerChildProjection(nil) = (%#v, %v), want (nil, nil)", bounded, err)
		}
	})

	t.Run("generic updates preserve notifier behavior", func(t *testing.T) {
		generic := &acpsdk.SessionUpdate{}
		bounded, err := (&attachmentCache{}).deliverBoundWorkerChildProjection("session", item, generic, nil)
		if err != nil || bounded != generic {
			t.Fatalf("generic update without notifier = (%#v, %v), want original update and nil", bounded, err)
		}

		_, err = (&attachmentCache{}).deliverBoundWorkerChildProjection("session", item, generic, func(*acpsdk.SessionUpdate) error {
			return notifyErr
		})
		if !errors.Is(err, notifyErr) {
			t.Fatalf("generic notifier error = %v, want wrapped %v", err, notifyErr)
		}
		if err.Error() != notifyErr.Error() {
			t.Fatalf("generic notifier error text = %q, want %q", err.Error(), notifyErr.Error())
		}
	})

	t.Run("fallback projection notifies exactly the bounded update", func(t *testing.T) {
		var cache *attachmentCache
		var delivered *acpsdk.SessionUpdate
		bounded, err := cache.deliverBoundWorkerChildProjection("session", item, update, func(got *acpsdk.SessionUpdate) error {
			delivered = got
			return nil
		})
		if err != nil || bounded == nil || delivered != bounded {
			t.Fatalf("fallback delivery = (%#v, %v), delivered %#v; want one bounded delivered update", bounded, err, delivered)
		}

		_, err = cache.deliverBoundWorkerChildProjection("session", item, update, func(*acpsdk.SessionUpdate) error {
			return notifyErr
		})
		if !errors.Is(err, notifyErr) {
			t.Fatalf("fallback notifier error = %v, want wrapped %v", err, notifyErr)
		}

		bounded, err = cache.deliverBoundWorkerChildProjection("session", item, update, nil)
		if err != nil || bounded == nil {
			t.Fatalf("fallback without notifier = (%#v, %v), want bounded update", bounded, err)
		}
	})

	t.Run("malformed bounded content is never notified or committed", func(t *testing.T) {
		cache := &attachmentCache{}
		unsafe := &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: update.ToolCallUpdate.ToolCallId,
			RawInput:   math.Inf(1),
		}}
		notified := false
		bounded, err := cache.deliverBoundWorkerChildProjection("session", item, unsafe, func(*acpsdk.SessionUpdate) error {
			notified = true
			return nil
		})
		if bounded != nil || !errors.Is(err, mapping.ErrMalformedRecord) {
			t.Fatalf("malformed delivery = (%#v, %v), want typed malformed error", bounded, err)
		}
		if notified {
			t.Fatal("malformed child update notified the client")
		}
		if got := cache.workerChildBudgets["session"]; len(got) != 0 {
			t.Fatalf("malformed child update committed a child budget: %#v", got)
		}
	})
}

func TestWorkerChildProjectionCacheNoopsWithoutAttachmentState(t *testing.T) {
	var cache *attachmentCache
	cache.markLoaded("session")
	if cache.wasLoaded("session") {
		t.Fatal("nil attachment cache reported a session as loaded")
	}

	server, _ := newStreamingTestServer(t, &fakeFactoryTargetService{})
	if err := server.restoreWorkerChildProjectionBudget(context.Background(), streamingTestSessionID, 1); err != nil {
		t.Fatalf("restore without an attachment cache = %v, want nil", err)
	}
	if err := server.rebuildWorkerChildProjectionBudget(nil, streamingTestSessionID, events.Record{}); err != nil {
		t.Fatalf("rebuild without an attachment cache = %v, want nil", err)
	}

	materialized := &attachmentCache{}
	budget := mapping.NewChildProjectionBudget(mapping.DefaultChildProjectionLimits())
	materialized.commitWorkerChildProjection("session", "child", budget)
	if got := materialized.workerChildBudgets["session"]["child"]; got != budget {
		t.Fatalf("committed child budget = %#v, want %#v", got, budget)
	}
}

func projectedWorkerChildMessage(t *testing.T) (chatsessions.SequencedItem, *acpsdk.SessionUpdate) {
	t.Helper()
	payload, err := json.Marshal(workers.MessageDeltaPayload{
		ContentBlockKind: workers.ContentBlockText,
		TextDelta:        "bounded child content",
	})
	if err != nil {
		t.Fatalf("marshal child message payload: %v", err)
	}
	item := chatsessions.SequencedItem{
		ItemID:                   "child-message",
		ParentItemID:             "child-tool-call",
		WorkerSessionAssociation: &chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker-session"},
		Kind:                     workers.KindMessage,
		Phase:                    workers.PhaseDelta,
		Payload:                  payload,
	}
	update, err := mapping.ProjectWorkerChild(item)
	if err != nil || update == nil || update.ToolCallUpdate == nil {
		t.Fatalf("ProjectWorkerChild() = (%#v, %v), want child update", update, err)
	}
	return item, update
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

// TestLiveDrainKeepsConcurrentWorkerChildrenStableAcrossRetainedAndReconnect
// exercises the child-specific retained/live handoff rather than changing the
// generic Chat delivery path. Two canonical associations open independently,
// live records remain inside their stored parent tool calls, and a resumed
// attachment observes only the terminal records committed after its cursor.
func TestLiveDrainKeepsConcurrentWorkerChildrenStableAcrossRetainedAndReconnect(t *testing.T) {
	server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a", "", "dispatch-a", "worker-a-session", workers.PhaseStarted, "RESERVED")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-b", "", "dispatch-b", "worker-b-session", workers.PhaseStarted, "STARTING")

	cache := &attachmentCache{}
	retainedNotify, retained := captureNotifier()
	retainedCtx := contextWithAttachmentCache(context.Background(), cache)
	if _, err := server.streamTurnUpdates(retainedCtx, "conn-retained", streamingTestSessionID, 1, retainedNotify); err != nil {
		t.Fatalf("retained streamTurnUpdates() error = %v", err)
	}
	assertWorkerChildTimeline(t, *retained, []string{"open:worker-a:pending", "open:worker-b:pending"})

	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.mu.Lock()
	chatSessions.getSessionResult.Session.StreamHead = 6
	chatSessions.mu.Unlock()
	liveCtx, cancelLive := context.WithCancel(contextWithAttachmentCache(context.Background(), cache))
	liveNotifications := make(chan acpsdk.SessionNotification, 2)
	liveDone := make(chan struct{})
	go func() {
		server.liveDrainTurnUpdates(liveCtx, "conn-retained", streamingTestSessionID, 1, func(n acpsdk.SessionNotification) error {
			liveNotifications <- n
			return nil
		})
		close(liveDone)
	}()
	eventsSvc.waitForSubscriber(t)
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-message", "worker-a", "dispatch-a", "worker-a-session", workers.KindMessage, workers.PhaseDelta,
		workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "a-live"})
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-b-message", "worker-b", "dispatch-b", "worker-b-session", workers.KindMessage, workers.PhaseDelta,
		workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "b-live"})
	live := collectWorkerChildNotifications(t, liveNotifications, 2)
	cancelLive()
	<-liveDone
	assertWorkerChildTimeline(t, live, []string{"update:worker-a::a-live", "update:worker-b::b-live"})

	attachment, ok := cache.get(streamingTestSessionID)
	if !ok {
		t.Fatal("retained/live attachment was not cached")
	}
	server.detachAttachments(context.Background(), cache)
	resumedCache := &attachmentCache{}
	resumedCache.setResumeAttachmentID(streamingTestSessionID, attachment.ID)
	resumedCtx := contextWithAttachmentCache(context.Background(), resumedCache)
	resumedNotify, resumed := captureNotifier()
	if _, err := server.streamTurnUpdates(resumedCtx, "conn-resumed", streamingTestSessionID, 1, resumedNotify); err != nil {
		t.Fatalf("resumed streamTurnUpdates() before later records error = %v", err)
	}
	assertWorkerChildTimeline(t, *resumed, nil)

	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a-failed", "worker-a", "dispatch-a", "worker-a-session", workers.PhaseFailed, "FAILED")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-b-completed", "worker-b", "dispatch-b", "worker-b-session", workers.PhaseCompleted, "COMPLETED")
	if _, err := server.streamTurnUpdates(resumedCtx, "conn-resumed", streamingTestSessionID, 1, resumedNotify); err != nil {
		t.Fatalf("resumed streamTurnUpdates() after terminal records error = %v", err)
	}
	assertWorkerChildTimeline(t, *resumed, []string{"update:worker-a:failed:", "update:worker-b:completed:"})

	fullNotify, full := captureNotifier()
	fullCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(fullCtx, "conn-full", streamingTestSessionID, 1, fullNotify); err != nil {
		t.Fatalf("full retained streamTurnUpdates() error = %v", err)
	}
	combined := append(append(append([]acpsdk.SessionNotification{}, (*retained)...), live...), (*resumed)...)
	assertEqualWorkerChildNotifications(t, *full, combined)
}

func TestStreamTurnUpdatesDeliversAssociatedWorkerLifecycleWithStoredLineage(t *testing.T) {
	server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-tool-call", "", "dispatch-1", "worker-session-1", workers.PhaseStarted, "STARTING")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-running", "worker-tool-call", "dispatch-1", "worker-session-1", workers.PhaseUpdated, "RUNNING")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-completed", "worker-tool-call", "dispatch-1", "worker-session-1", workers.PhaseCompleted, "COMPLETED")

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	deliveredMessage, err := server.streamTurnUpdates(ctx, "conn-worker", streamingTestSessionID, 1, notify)
	if err != nil {
		t.Fatalf("streamTurnUpdates() error = %v", err)
	}
	if deliveredMessage {
		t.Fatal("streamTurnUpdates() deliveredMessage = true, want false for child lifecycle-only updates")
	}
	if len(notified) != 3 {
		t.Fatalf("notification count = %d, want opening plus two lifecycle updates", len(notified))
	}

	opening := notified[0].Update.ToolCall
	if opening == nil || opening.ToolCallId != "worker-tool-call" || opening.Status != acpsdk.ToolCallStatusPending {
		t.Fatalf("opening = %#v, want pending tool call with stored ItemID", opening)
	}
	for index, wantStatus := range []acpsdk.ToolCallStatus{acpsdk.ToolCallStatusInProgress, acpsdk.ToolCallStatusCompleted} {
		update := notified[index+1].Update.ToolCallUpdate
		if update == nil || update.ToolCallId != "worker-tool-call" || update.Status == nil || *update.Status != wantStatus {
			t.Fatalf("lifecycle notification %d = %#v, want parent worker-tool-call with status %q", index+1, update, wantStatus)
		}
	}
}

func TestStreamTurnUpdatesKeepsInterleavedWorkerContentInsideItsOwningToolCall(t *testing.T) {
	server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a", "", "dispatch-a", "worker-a-session", workers.PhaseStarted, "STARTING")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-b", "", "dispatch-b", "worker-b-session", workers.PhaseStarted, "STARTING")
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-message", "worker-a", "dispatch-a", "worker-a-session", workers.KindMessage, workers.PhaseDelta,
		workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "a-message"})
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-b-progress", "worker-b", "dispatch-b", "worker-b-session", workers.KindProgress, workers.PhaseUpdated,
		workers.ProgressPayload{Label: "b-progress"})
	// This record is valid JSON but an invalid ErrorPayload. The pure mapper
	// rejects it; retained delivery must acknowledge only this malformed child
	// item and continue its sibling's canonical update order.
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-bad", "worker-a", "dispatch-a", "worker-a-session", workers.KindError, workers.PhaseUpdated,
		workers.ErrorPayload{Code: "bad"})
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-b-message", "worker-b", "dispatch-b", "worker-b-session", workers.KindMessage, workers.PhaseDelta,
		workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "b-message"})

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(ctx, "conn-worker-content", streamingTestSessionID, 1, notify); err != nil {
		t.Fatalf("streamTurnUpdates() error = %v", err)
	}
	if len(notified) != 5 {
		t.Fatalf("notification count = %d, want two openings and three valid isolated updates", len(notified))
	}
	if notified[0].Update.ToolCall == nil || notified[0].Update.ToolCall.ToolCallId != "worker-a" ||
		notified[1].Update.ToolCall == nil || notified[1].Update.ToolCall.ToolCallId != "worker-b" {
		t.Fatalf("openings = %#v, %#v, want stable tool calls for both children", notified[0].Update, notified[1].Update)
	}
	for index, want := range []struct {
		parent string
		text   string
	}{
		{"worker-a", "a-message"},
		{"worker-b", "b-progress"},
		{"worker-b", "b-message"},
	} {
		update := notified[index+2].Update.ToolCallUpdate
		if update == nil || update.ToolCallId != acpsdk.ToolCallId(want.parent) {
			t.Fatalf("notification %d update = %#v, want parent %q", index+2, update, want.parent)
		}
		if len(update.Content) != 1 || update.Content[0].Content == nil || update.Content[0].Content.Content.Text == nil ||
			update.Content[0].Content.Content.Text.Text != want.text {
			t.Fatalf("notification %d content = %#v, want %q", index+2, update.Content, want.text)
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestStreamTurnUpdatesBoundsNoisyChildWithoutReducingSiblingBudget(t *testing.T) {
	server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a", "", "dispatch-a", "worker-a-session", workers.PhaseStarted, "STARTING")
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-b", "", "dispatch-b", "worker-b-session", workers.PhaseStarted, "STARTING")
	for index := 0; index < mapping.DefaultChildProjectionMaxRecords-1; index++ {
		eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, fmt.Sprintf("worker-a-progress-%03d", index), "worker-a", "dispatch-a", "worker-a-session",
			workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{Label: fmt.Sprintf("a-progress-%03d", index)})
	}
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-b-message", "worker-b", "dispatch-b", "worker-b-session",
		workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "b-survives"})
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-error", "worker-a", "dispatch-a", "worker-a-session",
		workers.KindError, workers.PhaseFailed, workers.ErrorPayload{Code: "upstream", Message: "failed"})
	eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a-failed", "worker-a", "dispatch-a", "worker-a-session", workers.PhaseFailed, "FAILED")

	var notified []acpsdk.SessionNotification
	notify := func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(ctx, "conn-bounded-workers", streamingTestSessionID, 1, notify); err != nil {
		t.Fatalf("streamTurnUpdates() error = %v", err)
	}

	childAContentNotifications := mapping.DefaultChildProjectionMaxRecords - 1
	siblingIndex := 2 + childAContentNotifications
	if len(notified) != siblingIndex+3 {
		t.Fatalf("notification count = %d, want %d (openings, bounded A, B, failure, terminal)", len(notified), siblingIndex+3)
	}
	childAElision := notified[siblingIndex-1].Update.ToolCallUpdate
	if childAElision == nil || childAElision.ToolCallId != "worker-a" ||
		len(childAElision.Content) != 1 || childAElision.Content[0].Content == nil || childAElision.Content[0].Content.Content.Text == nil ||
		childAElision.Content[0].Content.Content.Text.Text == "" {
		t.Fatalf("worker A bound notification = %#v, want explicit parent-addressed elision", childAElision)
	}
	sibling := notified[siblingIndex].Update.ToolCallUpdate
	if sibling == nil || sibling.ToolCallId != "worker-b" || len(sibling.Content) != 1 || sibling.Content[0].Content == nil || sibling.Content[0].Content.Content.Text == nil ||
		sibling.Content[0].Content.Content.Text.Text != "b-survives" {
		t.Fatalf("worker B notification = %#v, want unaffected sibling content", sibling)
	}
	failure := notified[siblingIndex+1].Update.ToolCallUpdate
	if failure == nil || failure.ToolCallId != "worker-a" || len(failure.Content) != 1 || failure.Content[0].Content == nil || failure.Content[0].Content.Content.Text == nil ||
		failure.Content[0].Content.Content.Text.Text != "Worker error content was elided because its per-child projection limit was reached." {
		t.Fatalf("worker A failure notification = %#v, want retained failure elision", failure)
	}
	terminal := notified[siblingIndex+2].Update.ToolCallUpdate
	if terminal == nil || terminal.ToolCallId != "worker-a" || terminal.Status == nil || *terminal.Status != acpsdk.ToolCallStatusFailed {
		t.Fatalf("worker A terminal notification = %#v, want unchanged failed status", terminal)
	}
}

// TestReconnectRestoresWorkerChildBudget proves a fresh ACP connection uses
// the resumed attachment's cursor to rebuild child accounting before
// projecting a later record. Both record and byte pressure must emit the same
// explicit elision in prefix-plus-resume delivery as a fresh full replay.
func TestReconnectRestoresWorkerChildBudget(t *testing.T) {
	tests := []struct {
		name   string
		prefix func(*fakeEventsService)
		suffix workers.MessageDeltaPayload
	}{
		{
			name: "record pressure",
			prefix: func(eventsSvc *fakeEventsService) {
				for index := 0; index < mapping.DefaultChildProjectionMaxRecords-2; index++ {
					eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, fmt.Sprintf("worker-a-prefix-%03d", index), "worker-a", "dispatch-a", "worker-a-session",
						workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: fmt.Sprintf("prefix-%03d", index)})
				}
			},
			suffix: workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "after record pressure"},
		},
		{
			name: "byte pressure",
			prefix: func(eventsSvc *fakeEventsService) {
				eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-byte-prefix", "worker-a", "dispatch-a", "worker-a-session",
					workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: strings.Repeat("p", 40000)})
			},
			suffix: workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: strings.Repeat("s", 40000)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
			eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a", "", "dispatch-a", "worker-a-session", workers.PhaseStarted, "STARTING")
			tt.prefix(eventsSvc)

			initialCache := &attachmentCache{}
			initialNotify, initial := captureNotifier()
			initialCtx := contextWithAttachmentCache(context.Background(), initialCache)
			if _, err := server.streamTurnUpdates(initialCtx, "conn-initial", streamingTestSessionID, 1, initialNotify); err != nil {
				t.Fatalf("initial streamTurnUpdates() error = %v", err)
			}
			attachment, ok := initialCache.get(streamingTestSessionID)
			if !ok {
				t.Fatal("initial attachment was not cached")
			}
			server.detachAttachments(context.Background(), initialCache)

			eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, "worker-a-resumed", "worker-a", "dispatch-a", "worker-a-session",
				workers.KindMessage, workers.PhaseDelta, tt.suffix)
			resumedCache := &attachmentCache{}
			resumedCache.setResumeAttachmentID(streamingTestSessionID, attachment.ID)
			resumedNotify, resumed := captureNotifier()
			resumedCtx := contextWithAttachmentCache(context.Background(), resumedCache)
			if _, err := server.streamTurnUpdates(resumedCtx, "conn-resumed", streamingTestSessionID, 1, resumedNotify); err != nil {
				t.Fatalf("resumed streamTurnUpdates() error = %v", err)
			}
			assertExplicitWorkerChildElision(t, *resumed, "worker-a")

			fullNotify, full := captureNotifier()
			fullCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
			if _, err := server.streamTurnUpdates(fullCtx, "conn-full", streamingTestSessionID, 1, fullNotify); err != nil {
				t.Fatalf("full streamTurnUpdates() error = %v", err)
			}
			combined := append(append([]acpsdk.SessionNotification{}, (*initial)...), (*resumed)...)
			assertEqualWorkerChildNotifications(t, *full, combined)
		})
	}
}

// TestWorkerChildNotifierFailureDoesNotConsumeProjectionBudget proves a
// failed notification does not charge its associated child record. Retrying
// from the same attachment must keep the content and elision position equal
// to a fresh full replay under both independent projection limits.
func TestWorkerChildNotifierFailureDoesNotConsumeProjectionBudget(t *testing.T) {
	for _, tt := range []struct {
		name, retry, suffix string
		prefix              []string
	}{
		{"record pressure", "retry content", "suffix becomes elision", recordPressurePrefix()},
		{"byte pressure", strings.Repeat("r", 20000), strings.Repeat("s", 20000), []string{strings.Repeat("p", 40000)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, eventsSvc := newStreamingTestServer(t, &fakeFactoryTargetService{})
			eventsSvc.seedWorkerChildItem(t, streamingTestSessionID, "worker-a", "", "dispatch-a", "worker-a-session", workers.PhaseStarted, "STARTING")
			seedWorkerChildMessages(t, eventsSvc, tt.prefix...)
			seedWorkerChildMessage(t, eventsSvc, "worker-a-retry", tt.retry)
			seedWorkerChildMessage(t, eventsSvc, "worker-a-suffix", tt.suffix)

			cache := &attachmentCache{}
			var initial []acpsdk.SessionNotification
			wantErr := errors.New("notify failed")
			failingNotify := func(notification acpsdk.SessionNotification) error {
				if workerChildNotificationText(notification) == tt.retry {
					return wantErr
				}
				initial = append(initial, notification)
				return nil
			}
			ctx := contextWithAttachmentCache(context.Background(), cache)
			if _, err := server.streamTurnUpdates(ctx, "conn-retry", streamingTestSessionID, 1, failingNotify); !errors.Is(err, wantErr) {
				t.Fatalf("initial streamTurnUpdates() error = %v, want %v", err, wantErr)
			}

			retryNotify, retried := captureNotifier()
			if _, err := server.streamTurnUpdates(ctx, "conn-retry", streamingTestSessionID, 1, retryNotify); err != nil {
				t.Fatalf("retry streamTurnUpdates() error = %v", err)
			}
			if len(*retried) != 2 || workerChildNotificationText((*retried)[0]) != tt.retry {
				t.Fatalf("retry notifications = %#v, want retried content before its one elision", *retried)
			}
			assertExplicitWorkerChildElision(t, (*retried)[1:], "worker-a")

			fullNotify, full := captureNotifier()
			fullCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
			if _, err := server.streamTurnUpdates(fullCtx, "conn-full", streamingTestSessionID, 1, fullNotify); err != nil {
				t.Fatalf("full streamTurnUpdates() error = %v", err)
			}
			combined := append(append([]acpsdk.SessionNotification{}, initial...), (*retried)...)
			assertEqualWorkerChildNotifications(t, *full, combined)
		})
	}
}

func recordPressurePrefix() []string {
	prefix := make([]string, mapping.DefaultChildProjectionMaxRecords-3)
	for index := range prefix {
		prefix[index] = fmt.Sprintf("prefix-%03d", index)
	}
	return prefix
}

func seedWorkerChildMessages(t *testing.T, eventsSvc *fakeEventsService, messages ...string) {
	t.Helper()
	for index, message := range messages {
		seedWorkerChildMessage(t, eventsSvc, fmt.Sprintf("worker-a-prefix-%03d", index), message)
	}
}

func seedWorkerChildMessage(t *testing.T, eventsSvc *fakeEventsService, itemID, text string) {
	t.Helper()
	eventsSvc.seedWorkerChildRecord(t, streamingTestSessionID, itemID, "worker-a", "dispatch-a", "worker-a-session",
		workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: text})
}

func workerChildNotificationText(notification acpsdk.SessionNotification) string {
	update := notification.Update.ToolCallUpdate
	if update == nil || len(update.Content) != 1 || update.Content[0].Content == nil || update.Content[0].Content.Content.Text == nil {
		return ""
	}
	return update.Content[0].Content.Content.Text.Text
}

func assertExplicitWorkerChildElision(t *testing.T, notifications []acpsdk.SessionNotification, parent string) {
	t.Helper()
	if len(notifications) != 1 {
		t.Fatalf("resumed notification count = %d, want one explicit elision", len(notifications))
	}
	update := notifications[0].Update.ToolCallUpdate
	if update == nil || update.ToolCallId != acpsdk.ToolCallId(parent) || len(update.Content) != 1 ||
		update.Content[0].Content == nil || update.Content[0].Content.Content.Text == nil ||
		!strings.Contains(update.Content[0].Content.Content.Text.Text, "elided") {
		t.Fatalf("resumed notification = %#v, want explicit elision for %q", notifications[0].Update, parent)
	}
}

// seedWorkerChildItem builds the exact association-carrying Chat envelope the
// response bridge commits for one canonical Worker Session record. It keeps
// transport delivery assertions on the retained path rather than calling a
// child mapper directly.
func (f *fakeEventsService) seedWorkerChildItem(
	t *testing.T,
	sessionID, itemID, parentItemID, dispatchID, workerSessionID string,
	phase workers.Phase,
	status string,
) {
	t.Helper()
	f.seedWorkerChildRecord(t, sessionID, itemID, parentItemID, dispatchID, workerSessionID,
		workers.KindSession, phase, workers.SessionPayload{Status: status})
}

// seedWorkerChildRecord is the complete associated Worker envelope fixture:
// retained ACP delivery must retain every current workers.Kind, not only the
// SESSION lifecycle records that establish the parent tool call.
func (f *fakeEventsService) seedWorkerChildRecord(
	t *testing.T,
	sessionID, itemID, parentItemID, dispatchID, workerSessionID string,
	kind workers.Kind,
	phase workers.Phase,
	payload any,
) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal worker child payload: %v", err)
	}
	itemRaw, err := json.Marshal(chatsessions.SequencedItem{
		ItemID:                   itemID,
		ParentItemID:             parentItemID,
		WorkerSessionAssociation: &chatsessions.WorkerSessionAssociation{DispatchID: dispatchID, WorkerSessionID: workerSessionID},
		Kind:                     kind,
		Phase:                    phase,
		Payload:                  raw,
	})
	if err != nil {
		t.Fatalf("marshal worker child sequenced envelope: %v", err)
	}
	f.seedRaw(sessionID, itemRaw)
}

func (*fakeFactoryTargetService) SubscribeFactoryEventsForSession(
	context.Context,
	string,
	*factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	return &factorydefinitions.FactoryEventStream{}, nil
}

func collectWorkerChildNotifications(t *testing.T, notifications <-chan acpsdk.SessionNotification, count int) []acpsdk.SessionNotification {
	t.Helper()
	got := make([]acpsdk.SessionNotification, 0, count)
	for range count {
		select {
		case notification := <-notifications:
			got = append(got, notification)
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d live child notifications, want %d", len(got), count)
		}
	}
	return got
}

func assertWorkerChildTimeline(t *testing.T, notifications []acpsdk.SessionNotification, want []string) {
	t.Helper()
	got := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		got = append(got, workerChildNotificationSummary(t, notification))
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("worker child timeline = %v, want %v", got, want)
	}
}

func assertEqualWorkerChildNotifications(t *testing.T, got, want []acpsdk.SessionNotification) {
	t.Helper()
	gotJSON, gotErr := json.Marshal(got)
	wantJSON, wantErr := json.Marshal(want)
	if gotErr != nil || wantErr != nil || string(gotJSON) != string(wantJSON) {
		t.Fatalf("full retained notifications = %s (%v), want retained/live/reconnect notifications %s (%v)", gotJSON, gotErr, wantJSON, wantErr)
	}
}

func workerChildNotificationSummary(t *testing.T, notification acpsdk.SessionNotification) string {
	t.Helper()
	if opening := notification.Update.ToolCall; opening != nil {
		return fmt.Sprintf("open:%s:%s", opening.ToolCallId, opening.Status)
	}
	if update := notification.Update.ToolCallUpdate; update != nil {
		status, content := "", ""
		if update.Status != nil {
			status = string(*update.Status)
		}
		if len(update.Content) == 1 && update.Content[0].Content != nil && update.Content[0].Content.Content.Text != nil {
			content = update.Content[0].Content.Content.Text.Text
		}
		return fmt.Sprintf("update:%s:%s:%s", update.ToolCallId, status, content)
	}
	t.Fatalf("notification = %#v, want worker tool call or update", notification)
	return ""
}
