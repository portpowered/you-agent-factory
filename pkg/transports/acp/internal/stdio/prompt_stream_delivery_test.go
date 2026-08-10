package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
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
