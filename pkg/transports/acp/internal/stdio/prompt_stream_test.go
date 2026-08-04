package stdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
)

// fakeEventsService is a minimal events.Service test double, matching this
// package's existing fake convention (fakeChatSessionsService,
// fakeFactoryTargetService): only Read is implemented, since
// streamTurnUpdates is the one caller in this package that ever reaches an
// events.Service. Embedding the interface unimplemented means a call to any
// other method reaches a nil method value and panics, proving this
// package's streaming code never dispatches to Append/AttachSource/
// Subscribe. It is backed by an in-memory, per-topic append-only record
// log with real events.AggregateSequence positions, so drainRecords'
// pagination and at-head/progress handling run against real Cursor/
// ReadResult semantics instead of a hand-simplified shortcut.
//
// This package deliberately does not wire a real events.Service/
// chatsessions.Service pair here (pkg/boundary forbids a transport test
// constructing a product service through its own wire package -- see
// docs/internal/baselines/transport-behavior-baseline.json's deletion-only
// contract): proving this transport's streaming behavior against the real,
// fully composed service graph is story ACP-L1-V2-T03-message-projectors-005's
// job ("prove streaming through the canonical application graph" via
// root.BuildProcess), not this unit-level package's.
type fakeEventsService struct {
	events.Service

	mu      sync.Mutex
	records map[events.Topic][]events.Record
}

var _ events.Service = (*fakeEventsService)(nil)

func (f *fakeEventsService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	all := f.records[req.Topic]
	head := events.AggregateSequence(len(all))
	retained := events.RetainedRange{Topic: req.Topic, Earliest: 1, Head: head}

	if req.From.Position >= head {
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: req.From, Retained: retained}, nil
	}

	start := int(req.From.Position)
	end := min(start+req.Limit, len(all))
	page := all[start:end]
	next := events.Cursor{Topic: req.Topic, Position: page[len(page)-1].ID.Position}
	return events.ReadResult{Records: page, Next: next, Retained: retained, Outcome: events.ReadOutcomeProgress}, nil
}

// seed appends one (kind, phase, payload) record onto sessionID's
// chat-session topic, in the exact chatsessions.SequencedItem envelope
// shape a real chatsessions.Service.Sequence call commits (see
// prompt_stream.go's package doc for why no such production caller exists
// yet), so streamTurnUpdates/drainRecords decode and project it exactly as
// they would a genuine committed record.
func (f *fakeEventsService) seed(t *testing.T, sessionID string, kind workers.Kind, phase workers.Phase, payload any) {
	t.Helper()
	f.seedItem(t, sessionID, "", "", kind, phase, payload)
}

// seedItem is seed plus explicit sequencer-assigned itemID/parentItemID, for
// tests that assert item lineage survives projection (Chat Sessions assigns
// stable item identity at sequencing time -- see final-proposal.md §5.3 --
// so a record observed through Events already carries it).
func (f *fakeEventsService) seedItem(t *testing.T, sessionID, itemID, parentItemID string, kind workers.Kind, phase workers.Phase, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal seed payload: %v", err)
	}
	itemRaw, err := json.Marshal(chatsessions.SequencedItem{
		ItemID: itemID, ParentItemID: parentItemID, Kind: kind, Phase: phase, Payload: raw,
	})
	if err != nil {
		t.Fatalf("marshal seed envelope: %v", err)
	}
	f.seedRaw(sessionID, itemRaw)
}

// seedMalformed appends one record onto sessionID's topic whose payload does
// not decode as chatsessions.SequencedItem, standing in for a committed
// record this transport cannot make sense of -- proving drainRecords rejects
// it as errMalformedSequencedEnvelope instead of panicking or silently
// skipping it.
func (f *fakeEventsService) seedMalformed(sessionID string) {
	f.seedRaw(sessionID, json.RawMessage(`{"kind":`))
}

func (f *fakeEventsService) seedRaw(sessionID string, payload json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	topic := chatsessions.EventsTopic(sessionID)
	if f.records == nil {
		f.records = make(map[events.Topic][]events.Record)
	}
	position := events.AggregateSequence(len(f.records[topic]) + 1)
	f.records[topic] = append(f.records[topic], events.Record{
		ID:      events.RecordID{Topic: topic, Position: position},
		Payload: payload,
	})
}

const streamingTestSessionID = "session-1"

// newStreamingTestServer builds a Server against fakeChatSessionsService and
// fakeEventsService test doubles (not a real, wired chatsessions.Service/
// events.Service pair -- see fakeEventsService's own doc comment for why),
// with a session whose target episode is already bound to factorySessionID:
// every admitted turn in these tests reaches invokeFactorySessionForEpisode,
// not the start/bind branch, since that branch's own behavior is already
// covered by session_prompt_test.go and is orthogonal to what this file
// proves about streaming. turnIDs queues one chatsessions.StartTurnResult
// per call this test drives handleSessionPrompt through, letting a
// multi-turn test observe a fresh admitted Turn identity each time.
func newStreamingTestServer(t *testing.T, factoryTarget *fakeFactoryTargetService, turnIDs ...string) (*Server, *fakeEventsService) {
	t.Helper()

	session := chatsessions.Session{ID: streamingTestSessionID, Version: 1, WorkingRoot: "/work/project", TargetEpisode: 1}
	episode := chatsessions.TargetEpisode{
		Number:           1,
		Target:           chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/review"},
		FactorySessionID: "fs-1",
	}
	startTurnResults := make([]chatsessions.StartTurnResult, len(turnIDs))
	for i, turnID := range turnIDs {
		startTurnResults[i] = chatsessions.StartTurnResult{
			Session: session,
			Turn:    chatsessions.Turn{ID: turnID, State: chatsessions.TurnStateAdmitted},
			Episode: episode,
		}
	}

	chatSessions := &fakeChatSessionsService{
		getSessionResult: chatsessions.GetSessionResult{Session: session},
		startTurnResults: startTurnResults,
	}
	eventsSvc := &fakeEventsService{}
	catalog := &fakeFactoryTargetCatalogService{result: catalogResultWithCurrent("factory:@you/review")}
	resolveHomeDir := func() (string, error) { return "/home/operator", nil }
	server := New(nil, chatSessions, catalog, factoryTarget, eventsSvc, resolveHomeDir)
	return server, eventsSvc
}

func assistantMessagePayload(text string) workers.MessagePayload {
	return workers.MessagePayload{
		Role:          "assistant",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: text}},
	}
}

func fallbackInvokeResult(text string) factorysessions.InvocationResult {
	return factorysessions.InvocationResult{
		Status:        factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
	}
}

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

// captureNotifier returns a promptNotifier that appends every notification
// it receives, and the slice it appends into.
func captureNotifier() (promptNotifier, *[]acpsdk.SessionNotification) {
	var notified []acpsdk.SessionNotification
	return func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}, &notified
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
func TestStreamTurnUpdatesReconnectAfterDetachReplaysRetainedHistoryOnce(t *testing.T) {
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

	server.detachAttachments(context.Background(), firstCache)
	if len(chatSessions.detachCalls) != 1 {
		t.Fatalf("detach call count = %d, want exactly 1 after the first connection disconnects", len(chatSessions.detachCalls))
	}

	secondNotify, secondNotified := captureNotifier()
	secondCtx := contextWithAttachmentCache(context.Background(), &attachmentCache{})
	if _, err := server.streamTurnUpdates(secondCtx, "conn-second", streamingTestSessionID, 1, secondNotify); err != nil {
		t.Fatalf("reconnecting connection streamTurnUpdates() error = %v, want success", err)
	}
	if len(*secondNotified) != 1 {
		t.Fatalf("reconnecting connection notify count = %d, want exactly 1 (the retained record, replayed once)", len(*secondNotified))
	}
	if got := notificationItemID(t, (*secondNotified)[0]); got != "item-1" {
		t.Fatalf("reconnecting connection notification item id = %q, want the original %q", got, "item-1")
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
