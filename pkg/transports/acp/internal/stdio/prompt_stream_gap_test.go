package stdio

import (
	"context"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

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
