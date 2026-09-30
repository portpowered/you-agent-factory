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

type fakeEventsService struct {
	events.Service
	mu             sync.Mutex
	records        map[events.Topic][]events.Record
	evictedThrough map[events.Topic]events.AggregateSequence
	cond           *sync.Cond
	subscribed     chan struct{}
	readErr        error
	subscribeErr   error
}

var _ events.Service = (*fakeEventsService)(nil)

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
func (f *fakeEventsService) markEvictedThrough(sessionID string, through events.AggregateSequence) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.evictedThrough == nil {
		f.evictedThrough = make(map[events.Topic]events.AggregateSequence)
	}
	f.evictedThrough[chatsessions.EventsTopic(sessionID)] = through
}
func (f *fakeEventsService) seed(t *testing.T, sessionID string, kind workers.Kind, phase workers.Phase, payload any) {
	t.Helper()
	f.seedItem(t, sessionID, "", "", kind, phase, payload)
}
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
func (f *fakeEventsService) ensureCond() {
	if f.cond == nil {
		f.cond = sync.NewCond(&f.mu)
	}
}
func (f *fakeEventsService) ensureSubscribedChan() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribed == nil {
		f.subscribed = make(chan struct{})
	}
	return f.subscribed
}
func (f *fakeEventsService) waitForSubscriber(t *testing.T) {
	t.Helper()
	select {
	case <-f.ensureSubscribedChan():
	case <-time.After(2 * time.Second):
		t.Fatal("fakeEventsService: no Subscribe call observed within timeout")
	}
}
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
	server := New(nil, chatSessions, catalog, factoryTarget, eventsSvc, resolveHomeDir, nil, nil, testStartResolver)
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
func captureNotifier() (promptNotifier, *[]acpsdk.SessionNotification) {
	var notified []acpsdk.SessionNotification
	return func(n acpsdk.SessionNotification) error {
		notified = append(notified, n)
		return nil
	}, &notified
}
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
func seedRetentionGap(t *testing.T, eventsSvc *fakeEventsService, sessionID string) {
	t.Helper()
	eventsSvc.seed(t, sessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("evicted"))
	eventsSvc.markEvictedThrough(sessionID, 1)
	eventsSvc.seed(t, sessionID, workers.KindMessage, workers.PhaseCompleted, assistantMessagePayload("retained"))
}
