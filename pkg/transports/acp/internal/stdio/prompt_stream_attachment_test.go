package stdio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

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

// TestStreamTurnUpdatesTwoIndependentAttachmentsObserveIdenticalRecordsWithOneExecution
// proves story ACP-L1-V2-T03-message-projectors-004's AC1: two attachments
// to one Chat Session -- connection A driving the actual "session/prompt"
// turn (the only Factory execution), and connection B independently
// draining the same already-sequenced records through its own attachment
// and cursor, never itself dispatching a Factory turn -- observe the exact
// same eligible records, in the same order, with the same sequencer-assigned
// ItemID, while each attachment's own delivery cursor advances
// independently of the other's.
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
