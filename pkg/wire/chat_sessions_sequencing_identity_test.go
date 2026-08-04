package wire

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestChatSessionsSequencingIdentity_* proves story
// ACP-L1-V2-E03-chat-sequencer-005's acceptance criteria: assigned aggregate
// item identity (ItemID, ParentItemID, aggregate position, source identity,
// kind, schema, payload) survives unchanged across a retained Events Read, a
// retained-then-live Events Subscribe handoff (including a genuine
// concurrent race), a Chat Sessions attachment disconnect/reconnect, and an
// Events retention gap.
//
// These tests deliberately construct a real events.Service (via
// provideEventsService) and thread that exact instance into a real
// chatsessions.Service (via provideChatSessionsService), exactly as
// pkg/wire/chat_sessions_composition_test.go already does -- this is the one
// place both sibling services' /wire packages may legally be imported
// together (see the Codebase Patterns note in this repo's progress.txt).
//
// No customer-reachable transport path (CLI/HTTP/MCP/ACP) exists yet for
// Sequence, AdvanceStreamHead, or AcknowledgeAttachment -- they are brand
// new service operations with no handler wired in pkg/transports, and
// wiring one is explicitly out of this PRD's scope (T03). This composed
// two-service integration test is therefore the narrowest valid proof of
// cross-service identity stability available in this slice, matching this
// story's own acceptance criterion for when no customer-reachable execution
// path exists.
func newChatSessionsIdentityTestServices(t *testing.T) (chatsessions.Service, events.Service) {
	t.Helper()
	logger := logging.NoopLogger{}

	eventsService, err := provideEventsService(logger)
	if err != nil {
		t.Fatalf("provideEventsService() error = %v", err)
	}
	chatSessionsService, err := provideChatSessionsService(eventsService, logger)
	if err != nil {
		t.Fatalf("provideChatSessionsService() error = %v", err)
	}
	return chatSessionsService, eventsService
}

func createIdentityTestSession(t *testing.T, svc chatsessions.Service, connectionID string) chatsessions.Session {
	t.Helper()
	created, err := svc.CreateSession(context.Background(), chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: connectionID, JSONRPCStringID: "req-1"},
		WorkingRoot:   "/workspace/project",
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return created.Session
}

func identitySequenceRequest(sessionID string, sourceSeq int, parentItemID string) chatsessions.SequenceRequest {
	n := strconv.Itoa(sourceSeq)
	return chatsessions.SequenceRequest{
		SessionID:      sessionID,
		SourceType:     "worker",
		SourceID:       "worker-1",
		SourceSequence: events.SourceSequence(sourceSeq),
		SourceEventID:  events.SourceEventID("event-" + n),
		SchemaID:       "worker.output.v1",
		Kind:           workers.KindMessage,
		ParentItemID:   parentItemID,
		Payload:        json.RawMessage(`{"text":"message-` + n + `"}`),
	}
}

func mustSequence(t *testing.T, svc chatsessions.Service, req chatsessions.SequenceRequest) chatsessions.SequenceResult {
	t.Helper()
	result, err := svc.Sequence(context.Background(), req)
	if err != nil {
		t.Fatalf("Sequence() error = %v", err)
	}
	return result
}

func mustAttach(t *testing.T, svc chatsessions.Service, sessionID, connectionID string) string {
	t.Helper()
	result, err := svc.Attach(context.Background(), chatsessions.AttachRequest{
		SessionID:    sessionID,
		ConnectionID: connectionID,
		Interactive:  true,
	})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	return result.Attachment.ID
}

// mustAdvanceStreamHead reads the session's current version and advances its
// StreamHead to aggregateSeq, using sourceSeq as both the originating
// SourceSequence and (stringified) SourceEventID for structured-log
// correlation only.
func mustAdvanceStreamHead(t *testing.T, svc chatsessions.Service, sessionID string, aggregateSeq events.AggregateSequence, sourceSeq int) chatsessions.AdvanceStreamHeadResult {
	t.Helper()
	current, err := svc.GetSession(context.Background(), chatsessions.GetSessionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	result, err := svc.AdvanceStreamHead(context.Background(), chatsessions.AdvanceStreamHeadRequest{
		SessionID:         sessionID,
		ExpectedVersion:   current.Session.Version,
		AggregateSequence: aggregateSeq,
		SourceType:        "worker",
		SourceID:          "worker-1",
		SourceSequence:    events.SourceSequence(sourceSeq),
		SourceEventID:     events.SourceEventID("event-" + strconv.Itoa(sourceSeq)),
	})
	if err != nil {
		t.Fatalf("AdvanceStreamHead() error = %v", err)
	}
	return result
}

func mustAcknowledgeAttachment(t *testing.T, svc chatsessions.Service, sessionID, attachmentID string, expectedVersion uint64, after events.AggregateSequence) chatsessions.AcknowledgeAttachmentResult {
	t.Helper()
	result, err := svc.AcknowledgeAttachment(context.Background(), chatsessions.AcknowledgeAttachmentRequest{
		SessionID:       sessionID,
		AttachmentID:    attachmentID,
		ExpectedVersion: expectedVersion,
		AfterSequence:   after,
	})
	if err != nil {
		t.Fatalf("AcknowledgeAttachment() error = %v", err)
	}
	return result
}

// TestChatSessionsSequencingIdentity_RetainedReadReturnsExactCommittedIdentity
// proves a retained Events Read against EventsTopic(SessionID) returns each
// record with the exact aggregate sequence, ItemID, ParentItemID, source
// identity, kind, schema, and payload the sequencer committed -- not a
// regenerated or reconstructed value.
func TestChatSessionsSequencingIdentity_RetainedReadReturnsExactCommittedIdentity(t *testing.T) {
	t.Parallel()
	chatSessionsService, eventsService := newChatSessionsIdentityTestServices(t)
	session := createIdentityTestSession(t, chatSessionsService, "conn-1")
	topic := chatsessions.EventsTopic(session.ID)

	parentReq := identitySequenceRequest(session.ID, 1, "")
	parent := mustSequence(t, chatSessionsService, parentReq)
	childReq := identitySequenceRequest(session.ID, 2, parent.ItemID)
	child := mustSequence(t, chatSessionsService, childReq)

	readResult, err := eventsService.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if readResult.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Read().Outcome = %v, want ReadOutcomeProgress", readResult.Outcome)
	}
	if len(readResult.Records) != 2 {
		t.Fatalf("Read() returned %d records, want 2", len(readResult.Records))
	}

	cases := []struct {
		record  events.Record
		want    chatsessions.SequenceResult
		wantReq chatsessions.SequenceRequest
	}{
		{readResult.Records[0], parent, parentReq},
		{readResult.Records[1], child, childReq},
	}
	for i, c := range cases {
		if c.record.ID.Position != c.want.AggregateSequence {
			t.Fatalf("record[%d] position = %d, want %d", i, c.record.ID.Position, c.want.AggregateSequence)
		}
		if c.record.SourceType != c.wantReq.SourceType || c.record.SourceID != c.wantReq.SourceID ||
			c.record.SourceSequence != c.wantReq.SourceSequence || c.record.SourceEventID != c.wantReq.SourceEventID ||
			c.record.SchemaID != c.wantReq.SchemaID {
			t.Fatalf("record[%d] source identity/schema = %+v, want it to match the originating SequenceRequest %+v", i, c.record, c.wantReq)
		}
		var envelope chatsessions.SequencedItem
		if err := json.Unmarshal(c.record.Payload, &envelope); err != nil {
			t.Fatalf("record[%d] unmarshal envelope: %v", i, err)
		}
		if envelope.ItemID != c.want.ItemID {
			t.Fatalf("record[%d] envelope ItemID = %q, want %q", i, envelope.ItemID, c.want.ItemID)
		}
		if envelope.ParentItemID != c.want.ParentItemID {
			t.Fatalf("record[%d] envelope ParentItemID = %q, want %q", i, envelope.ParentItemID, c.want.ParentItemID)
		}
		if envelope.Kind != c.wantReq.Kind {
			t.Fatalf("record[%d] envelope Kind = %q, want %q", i, envelope.Kind, c.wantReq.Kind)
		}
		if string(envelope.Payload) != string(c.wantReq.Payload) {
			t.Fatalf("record[%d] envelope Payload = %s, want %s", i, envelope.Payload, c.wantReq.Payload)
		}
	}
	if child.ParentItemID != parent.ItemID {
		t.Fatalf("child.ParentItemID = %q, want parent's ItemID %q", child.ParentItemID, parent.ItemID)
	}
}

// TestChatSessionsSequencingIdentity_RetainedThenLiveHandoffDeliversEachPositionOnce
// proves a subscription started while records are already retained delivers
// every eligible position exactly once, in increasing order, including
// records sequenced after the subscription started (during handoff) but
// before any retained record was drained -- with no regenerated or
// connection-specific item identity: the delivered envelope's ItemID and
// ParentItemID must be byte-identical to what Sequence originally assigned.
func TestChatSessionsSequencingIdentity_RetainedThenLiveHandoffDeliversEachPositionOnce(t *testing.T) {
	t.Parallel()
	chatSessionsService, eventsService := newChatSessionsIdentityTestServices(t)
	session := createIdentityTestSession(t, chatSessionsService, "conn-1")
	topic := chatsessions.EventsTopic(session.ID)
	ctx := context.Background()

	first := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 1, ""))
	second := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 2, ""))

	subscription, err := eventsService.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	// Committed during the handoff window, before any retained record has
	// been drained by the subscriber.
	third := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 3, ""))
	fourth := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 4, ""))

	want := []chatsessions.SequenceResult{first, second, third, fourth}
	var previous events.AggregateSequence
	for i, expect := range want {
		delivery := subscription.Next(ctx)
		if delivery.Kind != events.DeliveryRecord {
			t.Fatalf("Next()[%d].Kind = %v, want DeliveryRecord", i, delivery.Kind)
		}
		if delivery.Record.ID.Position != expect.AggregateSequence {
			t.Fatalf("Next()[%d] position = %d, want %d (increasing order, no missing/duplicate position)", i, delivery.Record.ID.Position, expect.AggregateSequence)
		}
		if i > 0 && delivery.Record.ID.Position-previous != 1 {
			t.Fatalf("Next()[%d] position jumped from %d to %d, want contiguous", i, previous, delivery.Record.ID.Position)
		}
		previous = delivery.Record.ID.Position

		var envelope chatsessions.SequencedItem
		if err := json.Unmarshal(delivery.Record.Payload, &envelope); err != nil {
			t.Fatalf("Next()[%d] unmarshal envelope: %v", i, err)
		}
		if envelope.ItemID != expect.ItemID {
			t.Fatalf("Next()[%d] envelope ItemID = %q, want %q (identity must survive the retained-then-live handoff unchanged)", i, envelope.ItemID, expect.ItemID)
		}
	}
}

// TestChatSessionsSequencingIdentity_ConcurrentHandoffRaceDeliversEachPositionOnceIncreasing
// races several concurrent Sequence calls against an already-active
// subscription's Next() reader, released through a shared start barrier
// (never a sleep), and proves the subscription still delivers exactly the
// contiguous position set with each delivered ItemID matching the identity
// its corresponding Sequence call was assigned -- regardless of goroutine
// scheduling order.
func TestChatSessionsSequencingIdentity_ConcurrentHandoffRaceDeliversEachPositionOnceIncreasing(t *testing.T) {
	t.Parallel()
	chatSessionsService, eventsService := newChatSessionsIdentityTestServices(t)
	session := createIdentityTestSession(t, chatSessionsService, "conn-1")
	topic := chatsessions.EventsTopic(session.ID)
	ctx := context.Background()

	const writers = 16
	subscription, err := eventsService.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: writers + 1,
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	delivered := make(chan events.Delivery, writers)
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for range writers {
			delivered <- subscription.Next(ctx)
		}
		close(delivered)
	}()

	start := make(chan struct{})
	var writerWG sync.WaitGroup
	results := make([]chatsessions.SequenceResult, writers)
	for i := range writers {
		writerWG.Add(1)
		go func(idx int) {
			defer writerWG.Done()
			<-start
			results[idx] = mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, idx+1, ""))
		}(i)
	}
	close(start)
	writerWG.Wait()
	readerWG.Wait()

	wantItemIDByPosition := make(map[events.AggregateSequence]string, writers)
	for _, r := range results {
		wantItemIDByPosition[r.AggregateSequence] = r.ItemID
	}
	if len(wantItemIDByPosition) != writers {
		t.Fatalf("distinct committed positions = %d, want %d (no missing/duplicate position among concurrent writers)", len(wantItemIDByPosition), writers)
	}

	seenPositions := make(map[events.AggregateSequence]bool, writers)
	var previous events.AggregateSequence
	for delivery := range delivered {
		if delivery.Kind != events.DeliveryRecord {
			t.Fatalf("delivery.Kind = %v, want DeliveryRecord", delivery.Kind)
		}
		pos := delivery.Record.ID.Position
		if seenPositions[pos] {
			t.Fatalf("position %d delivered more than once", pos)
		}
		seenPositions[pos] = true
		if pos <= previous {
			t.Fatalf("position %d delivered out of increasing order after %d", pos, previous)
		}
		previous = pos

		var envelope chatsessions.SequencedItem
		if err := json.Unmarshal(delivery.Record.Payload, &envelope); err != nil {
			t.Fatalf("unmarshal envelope at position %d: %v", pos, err)
		}
		want, ok := wantItemIDByPosition[pos]
		if !ok {
			t.Fatalf("delivered position %d was never committed by any writer", pos)
		}
		if envelope.ItemID != want {
			t.Fatalf("delivered ItemID at position %d = %q, want %q", pos, envelope.ItemID, want)
		}
	}
	if len(seenPositions) != writers {
		t.Fatalf("delivered %d distinct positions, want %d", len(seenPositions), writers)
	}
}

// TestChatSessionsSequencingIdentity_ReconnectFromAcknowledgedAttachmentPositionPreservesIdentity
// proves disconnecting (simply ceasing to read) and reconnecting from an
// attachment's last-acknowledged position preserves every retained record's
// identity and lets that same attachment continue to advance independently
// afterward.
func TestChatSessionsSequencingIdentity_ReconnectFromAcknowledgedAttachmentPositionPreservesIdentity(t *testing.T) {
	t.Parallel()
	chatSessionsService, eventsService := newChatSessionsIdentityTestServices(t)
	ctx := context.Background()
	session := createIdentityTestSession(t, chatSessionsService, "conn-1")
	topic := chatsessions.EventsTopic(session.ID)
	attachmentID := mustAttach(t, chatSessionsService, session.ID, "conn-1")

	mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 1, ""))
	second := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 2, ""))
	third := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 3, ""))

	advanceResult := mustAdvanceStreamHead(t, chatSessionsService, session.ID, third.AggregateSequence, 3)
	if advanceResult.Session.StreamHead != uint64(third.AggregateSequence) {
		t.Fatalf("StreamHead = %d, want %d", advanceResult.Session.StreamHead, third.AggregateSequence)
	}

	// The attachment observes through position 2, then "disconnects" (simply
	// stops reading).
	ackResult := mustAcknowledgeAttachment(t, chatSessionsService, session.ID, attachmentID, advanceResult.Session.Version, second.AggregateSequence)
	if ackResult.Attachment.AfterSequence != uint64(second.AggregateSequence) {
		t.Fatalf("Attachment.AfterSequence = %d, want %d", ackResult.Attachment.AfterSequence, second.AggregateSequence)
	}

	// "Reconnect": read from the attachment's own last-acknowledged position,
	// exactly as a resumed consumer would, and confirm the still-unobserved
	// tail's identity is unchanged.
	readResult, err := eventsService.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic, Position: events.AggregateSequence(ackResult.Attachment.AfterSequence)},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if readResult.Outcome != events.ReadOutcomeProgress {
		t.Fatalf("Read().Outcome = %v, want ReadOutcomeProgress", readResult.Outcome)
	}
	if len(readResult.Records) != 1 {
		t.Fatalf("Read() after reconnect returned %d records, want 1 (only the still-unobserved tail)", len(readResult.Records))
	}
	var envelope chatsessions.SequencedItem
	if err := json.Unmarshal(readResult.Records[0].Payload, &envelope); err != nil {
		t.Fatalf("unmarshal reconnected envelope: %v", err)
	}
	if envelope.ItemID != third.ItemID {
		t.Fatalf("reconnected envelope ItemID = %q, want %q (original identity preserved across reconnect)", envelope.ItemID, third.ItemID)
	}
	if readResult.Records[0].ID.Position != third.AggregateSequence {
		t.Fatalf("reconnected record position = %d, want %d", readResult.Records[0].ID.Position, third.AggregateSequence)
	}

	// The attachment now advances independently past what it just observed
	// on reconnect, with no interference from StreamHead or any other state.
	secondAck := mustAcknowledgeAttachment(t, chatSessionsService, session.ID, attachmentID, advanceResult.Session.Version, third.AggregateSequence)
	if secondAck.Attachment.AfterSequence != uint64(third.AggregateSequence) {
		t.Fatalf("Attachment.AfterSequence after independent advance = %d, want %d", secondAck.Attachment.AfterSequence, third.AggregateSequence)
	}

	finalSession, err := chatSessionsService.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession() (final) error = %v", err)
	}
	if finalSession.Session.StreamHead != uint64(third.AggregateSequence) {
		t.Fatalf("final StreamHead = %d, want unchanged %d (attachment advancement must never move StreamHead)", finalSession.Session.StreamHead, third.AggregateSequence)
	}
}

// TestChatSessionsSequencingIdentity_EvictedPositionProducesGapWithoutFabrication
// proves that once Events has genuinely evicted a Chat Session's earliest
// sequenced records under its real, unmodified default retention policy,
// both a direct Read and an attachment reconnect through Chat Sessions
// observe the existing Events gap outcome (with an accurate retained range)
// instead of any fabricated record or silently reconstructed identity, and
// that AcknowledgeAttachment never jumps the attachment cursor forward
// through the unobserved, evicted range on its behalf.
func TestChatSessionsSequencingIdentity_EvictedPositionProducesGapWithoutFabrication(t *testing.T) {
	t.Parallel()
	chatSessionsService, eventsService := newChatSessionsIdentityTestServices(t)
	ctx := context.Background()
	session := createIdentityTestSession(t, chatSessionsService, "conn-1")
	topic := chatsessions.EventsTopic(session.ID)
	attachmentID := mustAttach(t, chatSessionsService, session.ID, "conn-1")

	// The attachment observes and acknowledges only the very first record,
	// before the rest of a very long history is committed and evicts it out
	// of Events' retained window.
	first := mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, 1, ""))
	advanceResult := mustAdvanceStreamHead(t, chatSessionsService, session.ID, first.AggregateSequence, 1)
	mustAcknowledgeAttachment(t, chatSessionsService, session.ID, attachmentID, advanceResult.Session.Version, first.AggregateSequence)

	// Commit enough further records to genuinely evict position 1 under
	// Events' real, unmodified default per-topic retention bound -- no
	// retention override, no fake: this is the production policy.
	const defaultMaxRetainedPerTopic = 10_000
	last := first
	for i := 2; i <= defaultMaxRetainedPerTopic+5; i++ {
		last = mustSequence(t, chatSessionsService, identitySequenceRequest(session.ID, i, ""))
	}
	mustAdvanceStreamHead(t, chatSessionsService, session.ID, last.AggregateSequence, defaultMaxRetainedPerTopic+5)

	// A direct Read from the now-evicted position 1 observes the Events gap
	// outcome, not a fabricated record.
	readResult, err := eventsService.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic, Position: first.AggregateSequence},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if readResult.Outcome != events.ReadOutcomeGap {
		t.Fatalf("Read().Outcome = %v, want ReadOutcomeGap (position 1 must have been evicted by now)", readResult.Outcome)
	}
	if readResult.Gap == nil || readResult.Gap.Requested != first.AggregateSequence {
		t.Fatalf("Read().Gap = %+v, want a gap naming Requested=%d", readResult.Gap, first.AggregateSequence)
	}
	if readResult.Gap.EarliestRetained <= first.AggregateSequence {
		t.Fatalf("Read().Gap.EarliestRetained = %d, want strictly after the evicted position %d", readResult.Gap.EarliestRetained, first.AggregateSequence)
	}
	if len(readResult.Records) != 0 {
		t.Fatalf("Read() on a gap returned %d fabricated records, want 0", len(readResult.Records))
	}

	// The attachment, still parked at the now-evicted position 1, cannot be
	// silently jumped forward across the gap by an acknowledgement request
	// that spans it: Chat Sessions must surface the retention gap rather
	// than reconstruct or skip the lost identity.
	finalSession, err := chatSessionsService.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession() (final) error = %v", err)
	}
	_, err = chatSessionsService.AcknowledgeAttachment(ctx, chatsessions.AcknowledgeAttachmentRequest{
		SessionID:       session.ID,
		AttachmentID:    attachmentID,
		ExpectedVersion: finalSession.Session.Version,
		AfterSequence:   last.AggregateSequence,
	})
	var gapErr *chatsessions.AttachmentRetentionGapError
	if err == nil {
		t.Fatal("AcknowledgeAttachment() across an evicted range = nil error, want *AttachmentRetentionGapError")
	}
	if !errors.As(err, &gapErr) {
		t.Fatalf("AcknowledgeAttachment() error = %v, want *AttachmentRetentionGapError", err)
	}

	afterRejected, err := chatSessionsService.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: session.ID})
	if err != nil {
		t.Fatalf("GetSession() (after rejected ack) error = %v", err)
	}
	if afterRejected.Session.Version != finalSession.Session.Version {
		t.Fatalf("Session.Version changed after a rejected acknowledgement: got %d, want unchanged %d", afterRejected.Session.Version, finalSession.Session.Version)
	}
}
