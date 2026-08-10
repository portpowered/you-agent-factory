// Shared fixtures for the focused ACP stdio behavioral suites.
//
// Keep these package-local definitions in one file so moving tests between
// command, lifecycle, cancellation, and streaming suites does not duplicate
// setup behavior.
package stdio

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeEventsService is a minimal events.Service test double, matching this
// package's existing fake convention (fakeChatSessionsService,
// fakeFactoryTargetService): Read and Subscribe are implemented (the two
// callers in this package that ever reach an events.Service --
// streamTurnUpdates uses only Read, liveDrainTurnUpdates uses only
// Subscribe). Embedding the interface unimplemented for the rest means a
// call to Append/AttachSource reaches a nil method value and panics, proving
// this package's streaming code never dispatches to either. It is backed by
// an in-memory, per-topic append-only record log with real
// events.AggregateSequence positions, so drainRecords' pagination and
// at-head/progress handling (via Read) and liveDrainTurnUpdates' delivery
// loop (via Subscribe) both run against real Cursor/ReadResult/Delivery
// semantics instead of a hand-simplified shortcut.
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

	mu             sync.Mutex
	records        map[events.Topic][]events.Record
	evictedThrough map[events.Topic]events.AggregateSequence
	// cond wakes every blocked Subscription.Next call once seedRaw appends a
	// new record or a caller's context is canceled (see the small watcher
	// goroutine Subscribe spawns per wait to bridge ctx.Done into a
	// Broadcast). Lazily initialized by ensureCond so a fakeEventsService
	// literal that never calls Subscribe (the overwhelming majority of this
	// package's tests) pays no cost for it.
	cond *sync.Cond
	// subscribed is closed the first time Subscribe is called, letting a test
	// deterministically wait for a live drain's Subscribe call to actually
	// register before seeding a record it expects that drain to observe.
	// Lazily initialized (see ensureSubscribedChan) so either Subscribe or a
	// test's own waitForSubscriber call can safely be first.
	subscribed chan struct{}
	// readErr, when set, fails every Read call with this exact error -- for
	// tests proving streamTurnUpdates propagates a genuine Events.Read
	// dependency failure instead of swallowing it.
	readErr error
	// subscribeErr, when set, fails every Subscribe call with this exact
	// error -- for tests proving liveDrainTurnUpdates' own best-effort
	// no-op convention on a Subscribe failure.
	subscribeErr error
}

var _ events.Service = (*fakeEventsService)(nil)

// Read mirrors the real Store.Read's own outcome decision
// (pkg/services/events/internal/service/read.go), including its "from+1 ==
// earliest is not a gap" boundary, so a test that calls markEvictedThrough
// exercises streamTurnUpdates against the same events.ReadOutcomeGap/
// events.GapFacts shape production Read would report, not a
// hand-simplified stand-in.
func (f *fakeEventsService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.readErr != nil {
		return events.ReadResult{}, f.readErr
	}

	all := f.records[req.Topic]
	head := events.AggregateSequence(len(all))
	earliest := f.evictedThrough[req.Topic] + 1
	retained := events.RetainedRange{Topic: req.Topic, Earliest: earliest, Head: head}

	from := req.From.Position
	switch {
	case from == head:
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: req.From, Retained: retained}, nil
	case from > head:
		return events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}, nil
	case earliest > 1 && from+1 < earliest:
		return events.ReadResult{
			Outcome: events.ReadOutcomeGap,
			Gap:     &events.GapFacts{Topic: req.Topic, Requested: from, EarliestRetained: earliest, Head: head},
		}, nil
	}

	start := int(from)
	end := min(start+req.Limit, len(all))
	page := all[start:end]
	next := events.Cursor{Topic: req.Topic, Position: page[len(page)-1].ID.Position}
	return events.ReadResult{Records: page, Next: next, Retained: retained, Outcome: events.ReadOutcomeProgress}, nil
}

// markEvictedThrough simulates retention having evicted every position up
// to and including through on sessionID's topic, so a later Read from a
// cursor at or before through observes events.ReadOutcomeGap the same way a
// real Events retention policy would report it.
func (f *fakeEventsService) markEvictedThrough(sessionID string, through events.AggregateSequence) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.evictedThrough == nil {
		f.evictedThrough = make(map[events.Topic]events.AggregateSequence)
	}
	f.evictedThrough[chatsessions.EventsTopic(sessionID)] = through
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
	if f.cond != nil {
		f.cond.Broadcast()
	}
}

// ensureCond lazily initializes cond under f.mu, safe to call from Subscribe
// or seedRaw regardless of call order.
func (f *fakeEventsService) ensureCond() {
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
}

// ensureSubscribedChan lazily initializes and returns subscribed under f.mu,
// safe to call from Subscribe or waitForSubscriber regardless of call order.
func (f *fakeEventsService) ensureSubscribedChan() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribed == nil {
		f.subscribed = make(chan struct{})
	}
	return f.subscribed
}

// waitForSubscriber blocks until Subscribe has been called at least once,
// letting a test deterministically know a live drain is already listening
// before it seeds a record the drain is expected to observe -- without a
// fixed sleep.
func (f *fakeEventsService) waitForSubscriber(t *testing.T) {
	t.Helper()
	select {
	case <-f.ensureSubscribedChan():
	case <-time.After(2 * time.Second):
		t.Fatal("fakeEventsService: no Subscribe call observed within timeout")
	}
}

// Subscribe returns a Subscription (a plain, blocking func(ctx) Delivery --
// see events.Subscription's own doc comment) that replays req.Topic's
// already-recorded events strictly in order starting after req.From, then
// blocks until seedRaw appends a new one or ctx is done. It mirrors Read's
// own retention-gap boundary (earliest > 1 && position+1 < earliest) so a
// test can exercise DeliveryGap the same way it exercises
// events.ReadOutcomeGap via markEvictedThrough.
func (f *fakeEventsService) Subscribe(_ context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	f.mu.Lock()
	if f.subscribeErr != nil {
		f.mu.Unlock()
		return nil, f.subscribeErr
	}
	f.ensureCond()
	f.mu.Unlock()

	subscribed := f.ensureSubscribedChan()
	select {
	case <-subscribed:
	default:
		close(subscribed)
	}

	topic := req.Topic
	position := req.From.Position
	return events.Subscription(func(ctx context.Context) events.Delivery {
		f.mu.Lock()
		defer f.mu.Unlock()
		for {
			all := f.records[topic]
			head := events.AggregateSequence(len(all))
			earliest := f.evictedThrough[topic] + 1
			switch {
			case earliest > 1 && position+1 < earliest:
				gap := &events.GapFacts{Topic: topic, Requested: position, EarliestRetained: earliest, Head: head}
				position = earliest - 1
				return events.Delivery{Kind: events.DeliveryGap, Gap: gap}
			case position < head:
				rec := all[int(position)]
				position = rec.ID.Position
				return events.Delivery{Kind: events.DeliveryRecord, Record: rec, Cursor: events.Cursor{Topic: topic, Position: rec.ID.Position}}
			}

			if ctx.Err() != nil {
				return events.Delivery{Kind: events.DeliveryCanceled}
			}
			waitDone := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					f.mu.Lock()
					f.cond.Broadcast()
					f.mu.Unlock()
				case <-waitDone:
				}
			}()
			f.cond.Wait()
			close(waitDone)
			if ctx.Err() != nil {
				return events.Delivery{Kind: events.DeliveryCanceled}
			}
		}
	}), nil
}

const streamingTestSessionID = "session-1"

// newStreamingTestServer builds a Server against fakeChatSessionsService and
// fakeEventsService test doubles (not a real, wired chatsessions.Service/
// events.Service pair -- see fakeEventsService's own doc comment for why),
// with a session whose target episode is already bound to factorySessionID:
// every admitted turn in these tests reaches invokeFactorySessionForEpisode,
// not the start/bind branch, since that branch's own behavior is already
// covered by session_prompt_dispatch_test.go and is orthogonal to what this file
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
	server := New(nil, chatSessions, catalog, factoryTarget, eventsSvc, resolveHomeDir, nil, nil)
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
