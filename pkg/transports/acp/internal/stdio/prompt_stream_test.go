// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

func TestAttachmentCacheResumeAttachmentID(t *testing.T) {
	var nilCache *attachmentCache
	if got := nilCache.resumeAttachmentID("session-1"); got != "" {
		t.Fatalf("nil cache resume identity = %q, want empty", got)
	}

	cache := &attachmentCache{}
	cache.setResumeAttachmentID("session-1", "attachment-1")
	if got := cache.resumeAttachmentID("session-1"); got != "attachment-1" {
		t.Fatalf("cached resume identity = %q, want attachment-1", got)
	}
	cache.setResumeAttachmentID("session-1", "")
	if got := cache.resumeAttachmentID("session-1"); got != "attachment-1" {
		t.Fatalf("blank resume identity overwrote stored identity %q", got)
	}
}

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

// TestStreamTurnUpdatesReusesAttachmentCursorAcrossTurns proves this
// connection's attachment cursor persists across two turns on the same Chat
// Session: a record seeded and delivered during the first turn is never
// redelivered during the second turn's own drain.
func TestStreamTurnUpdatesReusesAttachmentCursorAcrossTurns(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1", "turn-2")
	connID := identity.NewConnectionID()

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("first turn text"))

	var firstNotified []acpsdk.SessionNotification
	firstNotify := func(n acpsdk.SessionNotification) error {
		firstNotified = append(firstNotified, n)
		return nil
	}
	firstCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), firstNotify), &attachmentCache{})
	firstEnv := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "first"))
	if _, rpcErr := server.handleSessionPrompt(firstCtx, firstEnv); rpcErr != nil {
		t.Fatalf("first handleSessionPrompt() error = %+v, want success", rpcErr)
	}
	if len(firstNotified) != 1 {
		t.Fatalf("first turn notify call count = %d, want exactly 1", len(firstNotified))
	}

	// Reuse the same attachmentCache instance across both calls, exactly as
	// serveConnection threads one cache through every request on one
	// connection -- a fresh cache per call would defeat what this test
	// proves.
	cache := attachmentCacheFromContext(firstCtx)

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("second turn text"))

	var secondNotified []acpsdk.SessionNotification
	secondNotify := func(n acpsdk.SessionNotification) error {
		secondNotified = append(secondNotified, n)
		return nil
	}
	secondCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), secondNotify), cache)
	secondEnv := numberIdentityEnvelope(t, connID, 2, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "second"))
	if _, rpcErr := server.handleSessionPrompt(secondCtx, secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(secondNotified) != 1 {
		t.Fatalf("second turn notify call count = %d, want exactly 1 (only the newly seeded record)", len(secondNotified))
	}
	chunk := secondNotified[0].Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || chunk.Content.Text.Text != "second turn text" {
		t.Fatalf("second turn notification = %+v, want only %q", secondNotified[0], "second turn text")
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

func TestStreamTurnUpdatesTwoIndependentAttachmentsObserveIdenticalRecordsWithOneExecution(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	eventsSvc.seedItem(t, streamingTestSessionID, "item-1", "", workers.KindReasoning, workers.PhaseCompleted, workers.ReasoningPayload{Summary: "thinking..."})
	eventsSvc.seedItem(t, streamingTestSessionID, "item-2", "", workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("final answer"))

	aNotify, aNotified := captureNotifier()
	aCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), aNotify), &attachmentCache{})
	aEnv := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "hello"))
	if _, rpcErr := server.handleSessionPrompt(aCtx, aEnv); rpcErr != nil {
		t.Fatalf("connection A handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	bNotify, bNotified := captureNotifier()
	bCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	deliveredMessage, err := server.streamTurnUpdates(bCtx, "conn-b", streamingTestSessionID, 1, bNotify)
	if err != nil {
		t.Fatalf("connection B streamTurnUpdates() error = %v, want success", err)
	}
	if !deliveredMessage {
		t.Fatal("connection B streamTurnUpdates() deliveredMessage = false, want true")
	}

	if len(*aNotified) != 2 || len(*bNotified) != 2 {
		t.Fatalf("notify counts = A:%d B:%d, want exactly 2 each", len(*aNotified), len(*bNotified))
	}
	for i, want := range []string{"item-1", "item-2"} {
		aID, bID := notificationItemID(t, (*aNotified)[i]), notificationItemID(t, (*bNotified)[i])
		if aID != want || bID != want {
			t.Fatalf("notification[%d] item id = A:%q B:%q, want both %q", i, aID, bID, want)
		}
	}

	if got := len(factoryTarget.invokeCalls); got != 1 {
		t.Fatalf("Factory invoke call count = %d, want exactly 1 (only connection A ever dispatches a turn)", got)
	}
}

// notificationItemID extracts the MessageId a projected agent_thought_chunk
// or agent_message_chunk update carries, failing the test if neither is set.
func notificationItemID(t *testing.T, n acpsdk.SessionNotification) string {
	t.Helper()
	switch {
	case n.Update.AgentThoughtChunk != nil && n.Update.AgentThoughtChunk.MessageId != nil:
		return *n.Update.AgentThoughtChunk.MessageId
	case n.Update.AgentMessageChunk != nil && n.Update.AgentMessageChunk.MessageId != nil:
		return *n.Update.AgentMessageChunk.MessageId
	default:
		t.Fatalf("notification %+v carries no MessageId", n)
		return ""
	}
}

// TestStreamTurnUpdatesMalformedRecordFailsEachIndependentAttachmentTheSameWay
// proves story 004's AC5 at the attachment level: a malformed record on the
// shared topic stops each attachment's own drain at the same point --
// delivering the one good record ahead of it and then failing with
// errMalformedSequencedEnvelope -- without one attachment's failed attempt
// corrupting the shared topic or the other attachment's independent state.
func TestStreamTurnUpdatesMalformedRecordFailsEachIndependentAttachmentTheSameWay(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("good record"))
	eventsSvc.seedMalformed(streamingTestSessionID)

	aNotify, aNotified := captureNotifier()
	aCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(aCtx, "conn-a", streamingTestSessionID, 1, aNotify); err == nil || !errors.Is(err, errMalformedSequencedEnvelope) {
		t.Fatalf("connection A streamTurnUpdates() error = %v, want errMalformedSequencedEnvelope", err)
	}
	if len(*aNotified) != 1 {
		t.Fatalf("connection A notify count = %d, want exactly 1 (the good record delivered before the malformed one)", len(*aNotified))
	}

	bNotify, bNotified := captureNotifier()
	bCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(bCtx, "conn-b", streamingTestSessionID, 1, bNotify); err == nil || !errors.Is(err, errMalformedSequencedEnvelope) {
		t.Fatalf("connection B streamTurnUpdates() error = %v, want errMalformedSequencedEnvelope", err)
	}
	if len(*bNotified) != 1 {
		t.Fatalf("connection B notify count = %d, want exactly 1, unaffected by connection A's earlier failed attempt", len(*bNotified))
	}
}

// TestStreamTurnUpdatesDeliversStreamGapRecordBeforeSubsequentCatchUp proves
// story 004's AC4 for a producer-committed STREAM_GAP record (the only kind
// of gap this transport-owned slice can construct without inventing history
// -- see mapping.ProjectStreamGap's own doc comment): the gap notice is
// delivered in its sequenced position, strictly before the later record that
// follows it, and the later record's own text is never fabricated or
// skipped around the gap.
func TestStreamTurnUpdatesDeliversStreamGapRecordBeforeSubsequentCatchUp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1")

	eventsSvc.seed(t, streamingTestSessionID, workers.KindStreamGap, workers.PhaseUpdated, workers.StreamGapPayload{
		FromSequence: 1, ToSequence: 1, FirstAvailableSequence: 2, Reason: "retention evicted",
	})
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("after the gap"))

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, identity.NewConnectionID(), 1, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "hello"))
	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(*notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2 (gap notice, then the message)", len(*notified))
	}
	if (*notified)[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the gap notice delivered first (sequenced first)", (*notified)[0])
	}
	msgChunk := (*notified)[1].Update.AgentMessageChunk
	if msgChunk == nil || msgChunk.Content.Text == nil || msgChunk.Content.Text.Text != "after the gap" {
		t.Fatalf("notification[1] = %+v, want the message that followed the gap, verbatim", (*notified)[1])
	}
}

// TestStreamTurnUpdatesRecoversFromReadTimeRetentionGap proves story 004's
// AC4 for a retention gap events.Read itself detects
// (events.ReadOutcomeGap/events.GapFacts) -- distinct from
// TestStreamTurnUpdatesDeliversStreamGapRecordBeforeSubsequentCatchUp's
// producer-committed STREAM_GAP record: a fresh attachment whose cursor
// sits before positions Events has already evicted receives an explicit
// gap notice bounding exactly the evicted range before catch-up resumes at
// the first retained record, the evicted records' own content is never
// fabricated or silently skipped past, and the attachment's cursor genuinely
// advances past the gap rather than getting stuck retrying it.
func TestStreamTurnUpdatesRecoversFromReadTimeRetentionGap(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{invokeResult: fallbackInvokeResult("v1 fallback text")}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget, "turn-1", "turn-2")
	connID := identity.NewConnectionID()

	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("one"))
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("two"))
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("three"))
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("four"))
	eventsSvc.markEvictedThrough(streamingTestSessionID, 2)

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), notify), &attachmentCache{})
	env := numberIdentityEnvelope(t, connID, 1, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "hello"))
	if _, rpcErr := server.handleSessionPrompt(ctx, env); rpcErr != nil {
		t.Fatalf("handleSessionPrompt() error = %+v, want success", rpcErr)
	}

	if len(*notified) != 3 {
		t.Fatalf("notify call count = %d, want exactly 3 (gap notice, then the two retained messages)", len(*notified))
	}

	gapChunk := (*notified)[0].Update.AgentThoughtChunk
	if gapChunk == nil {
		t.Fatalf("notification[0] = %+v, want the read-time gap notice delivered before catch-up", (*notified)[0])
	}
	wantGapText := "Records from sequence 1 to 2 are unavailable; history resumes at sequence 3. Reason: retention eviction"
	if gapChunk.Content.Text == nil || gapChunk.Content.Text.Text != wantGapText {
		t.Fatalf("gap notice text = %+v, want %q", gapChunk.Content.Text, wantGapText)
	}

	wantTexts := []string{"three", "four"}
	for i, want := range wantTexts {
		msgChunk := (*notified)[i+1].Update.AgentMessageChunk
		if msgChunk == nil || msgChunk.Content.Text == nil || msgChunk.Content.Text.Text != want {
			t.Fatalf("notification[%d] = %+v, want retained message %q (evicted records 1-2 must never be fabricated)", i+1, (*notified)[i+1], want)
		}
	}

	// Reuse the same attachmentCache instance for a second turn (mirroring
	// TestStreamTurnUpdatesReusesAttachmentCursorAcrossTurns): if the gap
	// recovery had left the cursor unresolved -- stuck at 0, or somewhere
	// inside the evicted range -- this second turn would either redeliver
	// the gap notice/retained records again or fail outright, rather than
	// observing only the newly seeded fifth record.
	cache := attachmentCacheFromContext(ctx)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("five"))

	secondNotify, secondNotified := captureNotifier()
	secondCtx := contextWithAttachmentCache(contextWithPromptNotifier(context.Background(), secondNotify), cache)
	secondEnv := numberIdentityEnvelope(t, connID, 2, acpsdk.AgentMethodSessionPrompt, promptTextParams(streamingTestSessionID, "hello again"))
	if _, rpcErr := server.handleSessionPrompt(secondCtx, secondEnv); rpcErr != nil {
		t.Fatalf("second handleSessionPrompt() error = %+v, want success", rpcErr)
	}
	if len(*secondNotified) != 1 {
		t.Fatalf("second turn notify call count = %d, want exactly 1 (only the newly seeded fifth record)", len(*secondNotified))
	}
	fifthChunk := (*secondNotified)[0].Update.AgentMessageChunk
	if fifthChunk == nil || fifthChunk.Content.Text == nil || fifthChunk.Content.Text.Text != "five" {
		t.Fatalf("second turn notification = %+v, want only %q", (*secondNotified)[0], "five")
	}
}

// TestDetachAttachmentsReleasesEveryCachedAttachmentWithoutTouchingTheTurn
// proves story 004's AC2 (disconnect): detachAttachments -- the cleanup
// serveConnection defers once per connection (see server.go) -- calls
// chatsessions.Service.Detach for every attachment this connection ever
// registered, and only Detach: this fake's AdvanceTurn/RequestControl would
// themselves return "not implemented" errors if reached, so a nil error
// here already proves no turn-mutating call happened as a side effect of
// disconnect cleanup.
func TestDetachAttachmentsReleasesEveryCachedAttachmentWithoutTouchingTheTurn(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	cache := &attachmentCache{}
	first, ok, err := server.ensureAttachment(contextWithAttachmentCache(context.Background(), cache), "conn-a", "session-a")
	if err != nil || !ok {
		t.Fatalf("ensureAttachment(session-a) = %+v, %v, %v, want a registered attachment", first, ok, err)
	}
	second, ok, err := server.ensureAttachment(contextWithAttachmentCache(context.Background(), cache), "conn-a", "session-b")
	if err != nil || !ok {
		t.Fatalf("ensureAttachment(session-b) = %+v, %v, %v, want a registered attachment", second, ok, err)
	}

	server.detachAttachments(context.Background(), cache)

	if len(chatSessions.detachCalls) != 2 {
		t.Fatalf("detach call count = %d, want exactly 2 (one per cached attachment)", len(chatSessions.detachCalls))
	}
	gotSessions := map[string]string{}
	for _, req := range chatSessions.detachCalls {
		gotSessions[req.SessionID] = req.AttachmentID
	}
	if gotSessions["session-a"] != first.ID || gotSessions["session-b"] != second.ID {
		t.Fatalf("detach calls = %+v, want attachment %q for session-a and %q for session-b", chatSessions.detachCalls, first.ID, second.ID)
	}
}

// TestDetachAttachmentsCompletesAfterContextCancellation proves
// detachAttachments still releases attachments when ctx is already canceled
// (context.WithoutCancel's own contract): a physically dropped connection
// (context cancellation is exactly how one manifests in serveConnection's
// own loop) must not leave its attachment stranded on the session forever.
func TestDetachAttachmentsCompletesAfterContextCancellation(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	cache := &attachmentCache{}
	attachment, ok, err := server.ensureAttachment(contextWithAttachmentCache(context.Background(), cache), "conn-a", streamingTestSessionID)
	if err != nil || !ok {
		t.Fatalf("ensureAttachment() = %+v, %v, %v, want a registered attachment", attachment, ok, err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	server.detachAttachments(canceledCtx, cache)

	if len(chatSessions.detachCalls) != 1 || chatSessions.detachCalls[0].AttachmentID != attachment.ID {
		t.Fatalf("detach calls = %+v, want exactly one call for attachment %q despite the canceled context", chatSessions.detachCalls, attachment.ID)
	}
}

// TestStreamTurnUpdatesReconnectAfterDetachReplaysRetainedHistoryOnce proves
// story 004's AC2/AC3 together: after a connection's attachment is detached
// (disconnect), a later connection attaching fresh to the same session
// (reconnect -- this transport has no session/load or session/resume yet
// (see server.go's own doc comment), so a genuinely new connection always
// attaches fresh rather than resuming a specific prior attachment's cursor)
// still observes every currently retained record exactly once, in order,
// with its original sequencer-assigned ItemID: detaching one consumer never
// corrupts the shared topic or session for the next one.
// TestStreamTurnUpdatesReconnectAfterDetachResumesWithoutReplayingAcknowledgedHistory
// proves a reconnecting connection (a fresh attachmentCache and connection
// id, exactly what a new "session/load"/"session/resume" or "session/prompt"
// call on a brand-new connection observes) resumes from its previous
// connection's last acknowledged position instead of replaying already-
// delivered history from position zero: its client-supplied opaque attachment
// identity (see prompt_stream.go) reactivates the detached interactive
// attachment Detach left behind, so the second connection's
// streamTurnUpdates call observes zero notifications for the record the first
// connection already acknowledged, and exactly one for a record committed
// afterward.
func TestStreamTurnUpdatesReconnectAfterDetachResumesWithoutReplayingAcknowledgedHistory(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	eventsSvc.seedItem(t, streamingTestSessionID, "item-1", "", workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("before disconnect"))

	firstNotify, firstNotified := captureNotifier()
	firstCache := &attachmentCache{}
	firstCtx := contextWithAttachmentCache(context.Background(), firstCache)
	if _, err := server.streamTurnUpdates(firstCtx, "conn-first", streamingTestSessionID, 1, firstNotify); err != nil {
		t.Fatalf("first connection streamTurnUpdates() error = %v, want success", err)
	}
	if len(*firstNotified) != 1 {
		t.Fatalf("first connection notify count = %d, want exactly 1", len(*firstNotified))
	}
	firstAttachment, _ := firstCache.get(streamingTestSessionID)

	server.detachAttachments(context.Background(), firstCache)
	if len(chatSessions.detachCalls) != 1 {
		t.Fatalf("detach call count = %d, want exactly 1 after the first connection disconnects", len(chatSessions.detachCalls))
	}

	secondNotify, secondNotified := captureNotifier()
	secondCache := &attachmentCache{}
	secondCache.setResumeAttachmentID(streamingTestSessionID, firstAttachment.ID)
	secondCtx := contextWithAttachmentCache(context.Background(), secondCache)
	if _, err := server.streamTurnUpdates(secondCtx, "conn-second", streamingTestSessionID, 1, secondNotify); err != nil {
		t.Fatalf("reconnecting connection streamTurnUpdates() error = %v, want success", err)
	}
	if len(*secondNotified) != 0 {
		t.Fatalf("reconnecting connection notify count = %d, want exactly 0 (the already-acknowledged record must not replay)", len(*secondNotified))
	}
	secondAttachment, ok := attachmentCacheFromContext(secondCtx).get(streamingTestSessionID)
	if !ok {
		t.Fatal("reconnecting connection never registered an attachment")
	}
	if secondAttachment.ID != firstAttachment.ID {
		t.Fatalf("reconnecting connection attachment ID = %q, want the original attachment %q reactivated", secondAttachment.ID, firstAttachment.ID)
	}
	if secondAttachment.AfterSequence != firstAttachment.AfterSequence {
		t.Fatalf("reconnecting connection AfterSequence = %d, want preserved %d", secondAttachment.AfterSequence, firstAttachment.AfterSequence)
	}

	eventsSvc.seedItem(t, streamingTestSessionID, "item-2", "", workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("after reconnect"))
	if _, err := server.streamTurnUpdates(secondCtx, "conn-second", streamingTestSessionID, 1, secondNotify); err != nil {
		t.Fatalf("reconnecting connection streamTurnUpdates() (second call) error = %v, want success", err)
	}
	if len(*secondNotified) != 1 {
		t.Fatalf("reconnecting connection notify count after a later record = %d, want exactly 1", len(*secondNotified))
	}
	if got := notificationItemID(t, (*secondNotified)[0]); got != "item-2" {
		t.Fatalf("reconnecting connection notification item id = %q, want the later record %q", got, "item-2")
	}
}

// TestStreamTurnUpdatesConcurrentIndependentAttachmentsRaceFree drives two
// independent connections' drains against the same session concurrently
// (story 004's own AC6 calls for race/repeat coverage over exactly this
// shape: two consumers, no sleeps), proving neither the shared
// fakeEventsService/fakeChatSessionsService test doubles nor
// streamTurnUpdates itself have a data race, and that both attachments
// still observe every seeded record exactly once regardless of goroutine
// interleaving.
func TestStreamTurnUpdatesConcurrentIndependentAttachmentsRaceFree(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)

	const recordCount = 5
	for i := range recordCount {
		eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload(fmt.Sprintf("record-%d", i)))
	}

	drain := func(connID string) int {
		notify, notified := captureNotifier()
		ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
		if _, err := server.streamTurnUpdates(ctx, connID, streamingTestSessionID, 1, notify); err != nil {
			t.Errorf("streamTurnUpdates(%s) error = %v, want success", connID, err)
		}
		return len(*notified)
	}

	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i, connID := range []string{"conn-race-a", "conn-race-b"} {
		wg.Add(1)
		go func(i int, connID string) {
			defer wg.Done()
			counts[i] = drain(connID)
		}(i, connID)
	}
	wg.Wait()

	for i, got := range counts {
		if got != recordCount {
			t.Fatalf("connection %d notify count = %d, want exactly %d", i, got, recordCount)
		}
	}
}

// The tests below close CI's Backend Unit Coverage gap for this package by
// exercising the dependency-failure and no-attachment branches of
// attachmentCache, ensureAttachment, streamTurnUpdates, drainRecords,
// deliverReadTimeGap, and liveDrainTurnUpdates that the tests above -- all
// built around a working attachment and a healthy Events/Chat Sessions pair
// -- never reach.

// TestAttachmentCacheGetOnNilCacheReportsNotOk proves a nil *attachmentCache
// (attachmentCacheFromContext's own documented no-cache-attached case) never
// panics and always reports ok=false.
func TestAttachmentCacheGetOnNilCacheReportsNotOk(t *testing.T) {
	var cache *attachmentCache
	attachment, ok := cache.get("session-x")
	if ok {
		t.Fatalf("get() on a nil cache = (%+v, %v), want ok=false", attachment, ok)
	}
}

// TestAttachmentCacheSetOnNilCacheIsNoOp proves set on a nil *attachmentCache
// is a safe no-op, matching a nil promptNotifier's own no-op convention.
func TestAttachmentCacheSetOnNilCacheIsNoOp(t *testing.T) {
	var cache *attachmentCache
	cache.set("session-x", chatsessions.Attachment{ID: "attachment-1"})
}

// TestDetachAttachmentsContinuesAfterOneDetachFailure proves a Detach
// failure for one cached attachment is logged, not propagated, and does not
// stop the remaining sessions in cache from being released -- detachCalls
// still records the attempt even though it failed.
func TestDetachAttachmentsContinuesAfterOneDetachFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.detachErr = errors.New("detach failed")

	cache := &attachmentCache{}
	if _, ok, err := server.ensureAttachment(contextWithAttachmentCache(context.Background(), cache), "conn-a", streamingTestSessionID); err != nil || !ok {
		t.Fatalf("ensureAttachment() = %v, %v, want a registered attachment", ok, err)
	}

	server.detachAttachments(context.Background(), cache)

	if len(chatSessions.detachCalls) != 1 {
		t.Fatalf("detach call count = %d, want exactly 1 despite the failure", len(chatSessions.detachCalls))
	}
}

// TestEnsureAttachmentBlankConnectionIDReportsNotOk proves a blank
// connectionID (a request identity with no connection pairing) never
// attaches and reports ok=false rather than failing the turn.
func TestEnsureAttachmentBlankConnectionIDReportsNotOk(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	attachment, ok, err := server.ensureAttachment(ctx, "", streamingTestSessionID)
	if err != nil || ok {
		t.Fatalf("ensureAttachment(blank connectionID) = %+v, %v, %v, want ok=false, err=nil", attachment, ok, err)
	}
}

// TestEnsureAttachmentPropagatesAttachFailure proves a genuine Factory
// Sessions Attach failure is returned to the caller, not swallowed like the
// no-attachment (ok=false) case.
func TestEnsureAttachmentPropagatesAttachFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	wantErr := errors.New("attach failed")
	server.chatSessions.(*fakeChatSessionsService).attachErr = wantErr

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	_, ok, err := server.ensureAttachment(ctx, "conn-a", streamingTestSessionID)
	if !errors.Is(err, wantErr) || ok {
		t.Fatalf("ensureAttachment() = ok=%v err=%v, want ok=false and the exact Attach failure %v", ok, err, wantErr)
	}
}

// TestStreamTurnUpdatesPropagatesEnsureAttachmentFailure proves a genuine
// Attach failure reaching streamTurnUpdates through ensureAttachment is
// returned as this call's own error.
func TestStreamTurnUpdatesPropagatesEnsureAttachmentFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	wantErr := errors.New("attach failed")
	server.chatSessions.(*fakeChatSessionsService).attachErr = wantErr

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if delivered || !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=false and the exact Attach failure %v", delivered, err, wantErr)
	}
}

// TestStreamTurnUpdatesNoAttachmentIsNoOp proves a blank connectionID (no
// attachment ever registered) is a silent, successful no-op -- the same
// convention s.events == nil already has.
func TestStreamTurnUpdatesNoAttachmentIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("unreachable"))

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "", streamingTestSessionID, 1, notify)
	if delivered || err != nil || len(*notified) != 0 {
		t.Fatalf("streamTurnUpdates(blank connectionID) = %v, %v, notified=%d, want false, nil, 0", delivered, err, len(*notified))
	}
}

// TestStreamTurnUpdatesStopsOnCanceledContext proves the drain loop's own
// ctx.Err() check reports a canceled context rather than issuing a doomed
// Read call.
func TestStreamTurnUpdatesStopsOnCanceledContext(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx = contextWithAttachmentCache(ctx, &attachmentCache{})

	notify, _ := captureNotifier()
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("streamTurnUpdates(canceled ctx) error = %v, want context.Canceled", err)
	}
}

// TestStreamTurnUpdatesPropagatesReadFailure proves a genuine Events.Read
// dependency failure is returned as this call's own error.
func TestStreamTurnUpdatesPropagatesReadFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	wantErr := errors.New("read failed")
	eventsSvc.readErr = wantErr

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() error = %v, want the exact Read failure %v", err, wantErr)
	}
}

// TestStreamTurnUpdatesReportsInvalidCursorAsStreamGapError proves a cursor
// events.Read cannot resolve at all (events.ReadOutcomeInvalidCursor)
// surfaces as errStreamGapEncountered, distinct from an ordinary retention
// gap.
func TestStreamTurnUpdatesReportsInvalidCursorAsStreamGapError(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)

	cache := &attachmentCache{}
	cache.set(streamingTestSessionID, chatsessions.Attachment{ID: "attachment-1", AfterSequence: 999})

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), cache)
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, errStreamGapEncountered) {
		t.Fatalf("streamTurnUpdates() error = %v, want errStreamGapEncountered", err)
	}
}

// gapEventsService wraps a *fakeEventsService's Read to always report
// events.ReadOutcomeGap on the first call, then delegates every later call
// unchanged -- letting a test deterministically reach streamTurnUpdates'
// deliverReadTimeGap branch without needing retention-eviction bookkeeping
// beyond what markEvictedThrough already provides.
func seedRetentionGap(t *testing.T, eventsSvc *fakeEventsService, sessionID string) {
	t.Helper()
	eventsSvc.seed(t, sessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("evicted"))
	eventsSvc.markEvictedThrough(sessionID, 1)
	eventsSvc.seed(t, sessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("retained"))
}

// TestStreamTurnUpdatesPropagatesDeliverReadTimeGapNotifierFailure proves a
// notifier failure while delivering a read-time retention gap notice is
// returned as this call's own error, matching drainRecords' identical
// notifier-failure convention for an ordinary record.
func TestStreamTurnUpdatesPropagatesDeliverReadTimeGapNotifierFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)

	wantErr := errors.New("notify failed")
	notify := func(acpsdk.SessionNotification) error { return wantErr }
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() error = %v, want the exact notifier failure %v", err, wantErr)
	}
}

// TestStreamTurnUpdatesStopsWhenGapAcknowledgeLagsStreamHead proves a
// *chatsessions.AttachmentPositionError acknowledging a read-time gap's
// resume position is the same non-error "stop" case drainRecords documents,
// not a failure.
func TestStreamTurnUpdatesStopsWhenGapAcknowledgeLagsStreamHead(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	server.chatSessions.(*fakeChatSessionsService).acknowledgeAttachmentPositionErr = true

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if err != nil || delivered {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=false, err=nil (StreamHead-lag stop)", delivered, err)
	}
	if len(*notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1 (the gap notice, delivered before the lagging acknowledge)", len(*notified))
	}
}

// TestStreamTurnUpdatesPropagatesGapAcknowledgeGenericFailure proves a
// genuine (non-*AttachmentPositionError) AcknowledgeAttachment failure while
// resolving a read-time gap is returned as this call's own error.
func TestStreamTurnUpdatesPropagatesGapAcknowledgeGenericFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	wantErr := errors.New("acknowledge failed")
	server.chatSessions.(*fakeChatSessionsService).acknowledgeAttachmentErr = wantErr

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() error = %v, want the exact acknowledge failure %v", err, wantErr)
	}
}

// TestLiveDrainTurnUpdatesRetriesGapAcknowledgementConflictBeforeCatchUp
// proves a response bridge version advance after a live retention-gap notice
// reaches the client cannot leave that already-delivered gap unacknowledged.
// The refreshed acknowledgement must preserve exactly-once delivery through
// the subsequent retained sweep and allow the retained record after the gap
// to continue in the live drain.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestLiveDrainTurnUpdatesRetriesGapAcknowledgementConflictBeforeCatchUp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.getSessionResults = []chatsessions.GetSessionResult{
		{Session: chatsessions.Session{Version: 1}},
		{Session: chatsessions.Session{Version: 2}},
	}
	chatSessions.getSessionResult = chatsessions.GetSessionResult{Session: chatsessions.Session{Version: 2, StreamHead: 2}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := &attachmentCache{}
	ctx = contextWithAttachmentCache(ctx, cache)
	var notified []acpsdk.SessionNotification
	notify := func(notification acpsdk.SessionNotification) error {
		notified = append(notified, notification)
		if len(notified) == 1 {
			chatSessions.mu.Lock()
			chatSessions.acknowledgeAttachmentErrs = []error{
				&chatsessions.ConflictError{Value: "Session", ID: streamingTestSessionID, Expected: 1, Actual: 2},
			}
			chatSessions.mu.Unlock()
		}
		if len(notified) == 2 {
			cancel()
		}
		return nil
	}

	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); !delivered {
		t.Fatal("liveDrainTurnUpdates() deliveredMessage = false, want retained catch-up after the gap")
	}
	if len(notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2 (the gap once, then retained catch-up)", len(notified))
	}
	if notified[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the retention-gap notice", notified[0])
	}
	retained := notified[1].Update.AgentMessageChunk
	if retained == nil || retained.Content.Text == nil || retained.Content.Text.Text != "retained" {
		t.Fatalf("notification[1] = %+v, want retained catch-up after the gap", notified[1])
	}
	if len(chatSessions.acknowledgeAttachmentReqs) != 3 {
		t.Fatalf("acknowledge attempt count = %d, want exactly 3 (stale gap, refreshed gap, retained record)", len(chatSessions.acknowledgeAttachmentReqs))
	}
	if got := chatSessions.acknowledgeAttachmentReqs[0].ExpectedVersion; got != 1 {
		t.Fatalf("first gap acknowledgement expected version = %d, want 1", got)
	}
	if got := chatSessions.acknowledgeAttachmentReqs[1].ExpectedVersion; got != 2 {
		t.Fatalf("retried gap acknowledgement expected version = %d, want refreshed version 2", got)
	}
	attachment, ok := cache.get(streamingTestSessionID)
	if !ok || attachment.AfterSequence != 2 {
		t.Fatalf("cached attachment = %+v, %v, want retained sequence 2 acknowledged", attachment, ok)
	}

	catchUpNotify, catchUpNotifications := captureNotifier()
	catchUpCtx := contextWithAttachmentCache(context.Background(), cache)
	delivered, err := server.streamTurnUpdates(catchUpCtx, "conn-a", streamingTestSessionID, 1, catchUpNotify)
	if err != nil || delivered {
		t.Fatalf("post-live streamTurnUpdates() = %v, %v, want delivered=false and err=nil", delivered, err)
	}
	if len(*catchUpNotifications) != 0 {
		t.Fatalf("post-live catch-up notify count = %d, want 0 (the acknowledged gap must not replay)", len(*catchUpNotifications))
	}
}

// TestStreamTurnUpdatesStopsWhenRecordAcknowledgeLagsStreamHead proves a
// *chatsessions.AttachmentPositionError acknowledging an ordinary record is
// the same non-error "stop" case, not a failure: the record was already
// delivered, but the session's StreamHead has not yet advanced past it.
func TestStreamTurnUpdatesStopsWhenRecordAcknowledgeLagsStreamHead(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))
	server.chatSessions.(*fakeChatSessionsService).acknowledgeAttachmentPositionErr = true

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if err != nil || !delivered {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=true (already notified), err=nil (StreamHead-lag stop)", delivered, err)
	}
	if len(*notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1", len(*notified))
	}
}

// TestDrainRecordsPropagatesGenericAcknowledgeFailure proves a genuine
// (non-*AttachmentPositionError) AcknowledgeAttachment failure for an
// ordinary record is returned as this call's own error.
func TestDrainRecordsPropagatesGenericAcknowledgeFailure(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))
	wantErr := errors.New("acknowledge failed")
	server.chatSessions.(*fakeChatSessionsService).acknowledgeAttachmentErr = wantErr

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	_, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() error = %v, want the exact acknowledge failure %v", err, wantErr)
	}
}

// TestStreamTurnUpdatesRetriesStaleAcknowledgementBeforeReturning proves a
// live record that was already notified remains acknowledged when a concurrent
// response bridge advances the session version between StreamHead observation
// and the attachment acknowledgement. The retry must use the freshly read
// version and produce exactly one client update, preventing the retained
// catch-up drain from replaying the same canonical message.
func TestStreamTurnUpdatesRetriesStaleAcknowledgementBeforeReturning(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.acknowledgeAttachmentErrs = []error{
		&chatsessions.ConflictError{Value: "Session", ID: streamingTestSessionID, Expected: 1, Actual: 2},
	}
	chatSessions.getSessionResult = chatsessions.GetSessionResult{Session: chatsessions.Session{Version: 2}}

	notify, notified := captureNotifier()
	cache := &attachmentCache{}
	ctx := contextWithAttachmentCache(context.Background(), cache)
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if err != nil || !delivered {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=true and err=nil", delivered, err)
	}
	if len(*notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1", len(*notified))
	}
	if len(chatSessions.acknowledgeAttachmentReqs) != 2 {
		t.Fatalf("acknowledge attempt count = %d, want exactly 2", len(chatSessions.acknowledgeAttachmentReqs))
	}
	if got := chatSessions.acknowledgeAttachmentReqs[0].ExpectedVersion; got != 1 {
		t.Fatalf("first acknowledgement expected version = %d, want 1", got)
	}
	if got := chatSessions.acknowledgeAttachmentReqs[1].ExpectedVersion; got != 2 {
		t.Fatalf("retried acknowledgement expected version = %d, want refreshed version 2", got)
	}
	attachment, ok := cache.get(streamingTestSessionID)
	if !ok || attachment.AfterSequence != 1 {
		t.Fatalf("cached attachment = %+v, %v, want acknowledged sequence 1", attachment, ok)
	}
}

// TestStreamTurnUpdatesReturnsRefreshFailureAfterDeliveredConflict proves a
// stale acknowledgement never turns a failed version refresh into a false
// success. The client has already received the record, so the delivery result
// remains true while the bounded dependency failure is surfaced for the
// retained catch-up path to retry safely later.
func TestStreamTurnUpdatesReturnsRefreshFailureAfterDeliveredConflict(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.acknowledgeAttachmentErrs = []error{
		&chatsessions.ConflictError{Value: "Session", ID: streamingTestSessionID, Expected: 1, Actual: 2},
	}
	wantErr := errors.New("refresh session failed")
	chatSessions.getSessionErr = wantErr

	notify, notified := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if !delivered || !errors.Is(err, wantErr) {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=true and refresh error %v", delivered, err, wantErr)
	}
	if len(*notified) != 1 {
		t.Fatalf("notify call count = %d, want exactly 1", len(*notified))
	}
}

// TestDrainRecordsRejectsIllegalKindPhaseProjection proves a record that
// decodes fine as a chatsessions.SequencedItem envelope but whose
// Kind/Phase pair mapping.Project itself declares illegal (rather than the
// envelope itself being malformed JSON, which
// TestStreamTurnUpdatesMalformedRecordFailsEachIndependentAttachmentTheSameWay
// already covers) fails the drain with mapping's own error, not a panic or
// a silently skipped record.
func TestDrainRecordsRejectsIllegalKindPhaseProjection(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseFailed, assistantMessagePayload("illegal"))

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	delivered, err := server.streamTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	if delivered || err == nil {
		t.Fatalf("streamTurnUpdates() = %v, %v, want delivered=false and a projection error", delivered, err)
	}
}

// TestLiveDrainTurnUpdatesNoEventsIsNoOp proves a Server constructed without
// the Events collaborator reports no live delivery rather than panicking.
func TestLiveDrainTurnUpdatesNoEventsIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)
	server.events = nil

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(no events) = %v, want false", delivered)
	}
}

// TestLiveDrainTurnUpdatesNoAttachmentIsNoOp proves a blank connectionID
// (ensureAttachment reports ok=false) is a silent no-op, matching
// streamTurnUpdates' own convention.
func TestLiveDrainTurnUpdatesNoAttachmentIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, _ := newStreamingTestServer(t, factoryTarget)

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(blank connectionID) = %v, want false", delivered)
	}
}

// TestLiveDrainTurnUpdatesSubscribeFailureIsNoOp proves a genuine
// Events.Subscribe failure is a silent no-op: the guaranteed-correct
// post-invocation streamTurnUpdates sweep is the backstop for whatever this
// best-effort live drain could not start.
func TestLiveDrainTurnUpdatesSubscribeFailureIsNoOp(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	eventsSvc.subscribeErr = errors.New("subscribe failed")

	notify, _ := captureNotifier()
	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if delivered := server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify); delivered {
		t.Fatalf("liveDrainTurnUpdates(subscribe failure) = %v, want false", delivered)
	}
}

// TestWaitForLiveStreamHead covers the live subscription's delivery guard:
// a record must not be notified until the response bridge has advanced the
// Chat Session stream head to cover its aggregate position. The canceled
// case uses the invocation-owned context's explicit completion signal rather
// than waiting for the guard's retry interval.
func TestWaitForLiveStreamHead(t *testing.T) {
	t.Run("returns current version once stream head covers record", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		chatSessions := server.chatSessions.(*fakeChatSessionsService)
		chatSessions.getSessionResult.Session.Version = 7
		chatSessions.getSessionResult.Session.StreamHead = 3

		version, ready := server.waitForLiveStreamHead(context.Background(), streamingTestSessionID, 3)

		if !ready || version != 7 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (7, true)", version, ready)
		}
	})

	t.Run("stops when session lookup fails", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		server.chatSessions.(*fakeChatSessionsService).getSessionErr = errors.New("get session failed")

		version, ready := server.waitForLiveStreamHead(context.Background(), streamingTestSessionID, 1)

		if ready || version != 0 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (0, false)", version, ready)
		}
	})

	t.Run("stops when turn completes before stream head advances", func(t *testing.T) {
		factoryTarget := &fakeFactoryTargetService{}
		server, _ := newStreamingTestServer(t, factoryTarget)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		version, ready := server.waitForLiveStreamHead(ctx, streamingTestSessionID, 1)

		if ready || version != 0 {
			t.Fatalf("waitForLiveStreamHead() = (%d, %v), want (0, false)", version, ready)
		}
	})
}

// TestLiveDrainTurnUpdatesVersionFailureStopsOnRecordDelivery proves a
// genuine GetSession failure while re-reading the session's current version
// for a delivered record stops the live drain rather than dispatching
// drainRecords against a stale/unknown version.
func TestLiveDrainTurnUpdatesVersionFailureStopsOnRecordDelivery(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify, _ := captureNotifier()

	// Prime the attachment and Subscribe registration before injecting the
	// GetSession failure, matching this fake's existing sequencing
	// convention elsewhere in this file (ensure Subscribe has already
	// observed the request before the drain can reach a delivery).
	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()
	eventsSvc.waitForSubscriber(t)
	chatSessions.getSessionErr = errors.New("get session failed")
	eventsSvc.seed(t, streamingTestSessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("hello"))

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(version failure) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a GetSession failure")
	}
}

// TestLiveDrainTurnUpdatesDeliversGapThenStopsOnCancel proves a retention
// gap observed mid-subscription is delivered as a gap notice (the same
// deliverReadTimeGap path streamTurnUpdates' own retained catch-up uses),
// and the drain loop continues rather than treating a successful gap
// delivery as terminal.
func TestLiveDrainTurnUpdatesDeliversGapThenStopsOnCancel(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)
	// The live record immediately after the gap must be acknowledgeable for
	// this test to prove the drain continues beyond the gap rather than merely
	// waiting for cancellation after one notification.
	chatSessions := server.chatSessions.(*fakeChatSessionsService)
	chatSessions.mu.Lock()
	chatSessions.getSessionResult.Session.StreamHead = 2
	chatSessions.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = contextWithAttachmentCache(ctx, &attachmentCache{})
	var mu sync.Mutex
	var notified []acpsdk.SessionNotification
	retainedDelivered := make(chan struct{})
	var retainedOnce sync.Once
	notify := func(notification acpsdk.SessionNotification) error {
		mu.Lock()
		notified = append(notified, notification)
		count := len(notified)
		mu.Unlock()
		if count == 2 {
			retainedOnce.Do(func() { close(retainedDelivered) })
		}
		return nil
	}

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()

	// This direct notification signal replaces the prior timing delay: the
	// gap must be delivered first and the retained record immediately after it
	// must also arrive before cancellation can end the subscription.
	<-retainedDelivered
	select {
	case <-drainDone:
		t.Fatal("liveDrainTurnUpdates returned after the gap instead of continuing to retained catch-up")
	default:
	}
	cancel()

	if delivered := <-drainDone; !delivered {
		t.Fatal("liveDrainTurnUpdates deliveredMessage = false, want the retained message delivered after the gap")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 2 {
		t.Fatalf("notify call count = %d, want exactly 2 (gap notice, then retained record)", len(notified))
	}
	if notified[0].Update.AgentThoughtChunk == nil {
		t.Fatalf("notification[0] = %+v, want the gap notice before retained catch-up", notified[0])
	}
	retained := notified[1].Update.AgentMessageChunk
	if retained == nil || retained.Content.Text == nil || retained.Content.Text.Text != "retained" {
		t.Fatalf("notification[1] = %+v, want retained message after the gap", notified[1])
	}
}

// TestLiveDrainTurnUpdatesVersionFailureStopsOnGap proves a genuine
// GetSession failure while re-reading the session's current version for an
// observed gap stops the live drain the same way it does for an ordinary
// record.
func TestLiveDrainTurnUpdatesVersionFailureStopsOnGap(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	chatSessions := server.chatSessions.(*fakeChatSessionsService)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify, _ := captureNotifier()

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()
	eventsSvc.waitForSubscriber(t)
	chatSessions.getSessionErr = errors.New("get session failed")
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(version failure on gap) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a GetSession failure")
	}
}

// TestLiveDrainTurnUpdatesGapAcknowledgeFailureStopsDrain proves a genuine
// deliverReadTimeGap failure (a notifier failure while delivering the gap
// notice) stops the live drain the same way it stops streamTurnUpdates.
func TestLiveDrainTurnUpdatesGapAcknowledgeFailureStopsDrain(t *testing.T) {
	factoryTarget := &fakeFactoryTargetService{}
	server, eventsSvc := newStreamingTestServer(t, factoryTarget)
	seedRetentionGap(t, eventsSvc, streamingTestSessionID)

	ctx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	notify := func(acpsdk.SessionNotification) error {
		return errors.New("notify failed")
	}

	drainDone := make(chan bool, 1)
	go func() {
		drainDone <- server.liveDrainTurnUpdates(ctx, "conn-a", streamingTestSessionID, 1, notify)
	}()

	select {
	case delivered := <-drainDone:
		if delivered {
			t.Fatalf("liveDrainTurnUpdates(gap notify failure) = %v, want false", delivered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("liveDrainTurnUpdates did not stop after a gap notifier failure")
	}
}
