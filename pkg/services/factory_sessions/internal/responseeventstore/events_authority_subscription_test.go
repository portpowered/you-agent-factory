package responseeventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

// memoryEventsService is a minimal, hand-rolled events.Service double built
// only against the published pkg/services/events contract (no dependency on
// the owning service's wire/internal construction paths, which are reserved
// for pkg/wire and the owning service itself). It supports exactly what
// these tests need: Append accepts records in commit order per topic, and
// Read serves back a contiguous slice. AttachSource/Subscribe are not
// exercised by substituteFromEvents (only Read is) and are left
// unimplemented.
type memoryEventsService struct {
	mu      sync.Mutex
	records map[events.Topic][]events.Record
}

func newMemoryEventsService() *memoryEventsService {
	return &memoryEventsService{records: make(map[events.Topic][]events.Record)}
}

func (m *memoryEventsService) Append(_ context.Context, req events.AppendRequest) (events.AppendResult, error) {
	if err := req.Validate(); err != nil {
		return events.AppendResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	position := events.AggregateSequence(len(m.records[req.Topic]) + 1)
	record := events.Record{
		ID:             events.RecordID{Topic: req.Topic, Position: position},
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
		SchemaID:       req.SchemaID,
		Payload:        append(json.RawMessage(nil), req.Payload...),
	}
	m.records[req.Topic] = append(m.records[req.Topic], record)
	return events.AppendResult{Record: record.Detached(), Outcome: events.AppendOutcomeAccepted}, nil
}

func (m *memoryEventsService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	if err := req.Validate(); err != nil {
		return events.ReadResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.records[req.Topic]
	head := events.AggregateSequence(len(all))
	retained := events.RetainedRange{Topic: req.Topic, Earliest: 1, Head: head}
	if head == 0 || req.From.Position >= head {
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead, Next: events.Cursor{Topic: req.Topic, Position: head}, Retained: retained}, nil
	}
	start := int(req.From.Position)
	end := min(start+req.Limit, len(all))
	slice := all[start:end]
	out := make([]events.Record, len(slice))
	for i, rec := range slice {
		out[i] = rec.Detached()
	}
	last := out[len(out)-1]
	return events.ReadResult{
		Outcome:  events.ReadOutcomeProgress,
		Records:  out,
		Next:     events.Cursor{Topic: req.Topic, Position: last.ID.Position},
		Retained: retained,
	}, nil
}

func (m *memoryEventsService) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return nil, errors.New("memoryEventsService: Subscribe is not implemented")
}

func (m *memoryEventsService) AttachSource(context.Context, events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return events.AttachSourceResult{}, errors.New("memoryEventsService: AttachSource is not implemented")
}

var _ events.Service = (*memoryEventsService)(nil)

// newEventsAuthorityStore constructs a store bound to the given Events
// double and publishes through it exactly the way response_stream's
// ResponseStream.Publish does: append to Events first, deriving the store's
// own sequence from the aggregate position Events assigns, then commit that
// identical identity into the store. This proves substituteFromEvents'
// delivery behavior directly within the responseeventstore package's own
// test files, rather than only indirectly through a sibling package's tests
// (see the story 006 gotcha documented in progress.txt about unit coverage
// being measured per-package).
func newEventsAuthorityStore(t *testing.T, eventsService events.Service, factorySessionID string) *responseeventstore.SessionResponseEventStore {
	t.Helper()
	topic := events.Topic("factory-session/" + factorySessionID + "/response-events")
	store, err := responseeventstore.NewSessionResponseEventStoreWithEventsAuthority(
		factorySessionID,
		platformclock.Real{},
		responseeventstore.DefaultRetentionLimits(),
		testResponseEventID,
		eventsService,
		topic,
	)
	if err != nil {
		t.Fatalf("NewSessionResponseEventStoreWithEventsAuthority: %v", err)
	}
	return store
}

func publishThroughEventsAuthority(
	t *testing.T,
	eventsService events.Service,
	store *responseeventstore.SessionResponseEventStore,
	topic events.Topic,
) responseevents.FactoryResponseEvent {
	t.Helper()
	published, err := store.PublishThroughAuthority(samplePublishInput(), func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		eventID := testResponseEventID()
		prepared.Sequence = sequenceHint
		prepared.EventID = eventID
		payload, err := json.Marshal(prepared)
		if err != nil {
			t.Fatalf("marshal prepared event: %v", err)
		}
		result, err := eventsService.Append(context.Background(), events.AppendRequest{
			Topic:          topic,
			SourceType:     "factory-session-response-event",
			SourceID:       events.SourceID(store.FactorySessionID()),
			SourceSequence: events.SourceSequence(sequenceHint),
			SourceEventID:  events.SourceEventID(eventID),
			SchemaID:       "factory-response-event.v1",
			Payload:        payload,
		})
		if err != nil {
			return 0, "", err
		}
		return int64(result.Record.ID.Position), eventID, nil
	})
	if err != nil {
		t.Fatalf("PublishThroughAuthority: %v", err)
	}
	return published
}

// payloadSwappingEventsService wraps an events.Service and rewrites every
// Read's returned payload content, proving a subscriber's delivered content
// is genuinely read back from Events rather than only from the store's own
// retained copy.
type payloadSwappingEventsService struct {
	inner events.Service
}

func (p *payloadSwappingEventsService) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return p.inner.Append(ctx, req)
}

func (p *payloadSwappingEventsService) AttachSource(ctx context.Context, req events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return p.inner.AttachSource(ctx, req)
}

func (p *payloadSwappingEventsService) Read(ctx context.Context, req events.ReadRequest) (events.ReadResult, error) {
	result, err := p.inner.Read(ctx, req)
	if err != nil || result.Outcome != events.ReadOutcomeProgress {
		return result, err
	}
	swapped := make([]events.Record, len(result.Records))
	for i, record := range result.Records {
		var decoded responseevents.FactoryResponseEvent
		if jsonErr := json.Unmarshal(record.Payload, &decoded); jsonErr == nil {
			decoded.RunID = "swapped-by-events-read"
			if reencoded, encErr := json.Marshal(decoded); encErr == nil {
				record.Payload = reencoded
			}
		}
		swapped[i] = record
	}
	result.Records = swapped
	return result, nil
}

func (p *payloadSwappingEventsService) Subscribe(ctx context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	return p.inner.Subscribe(ctx, req)
}

var _ events.Service = (*payloadSwappingEventsService)(nil)

func TestSessionResponseEventStoreSubscription_DeliversContentReadBackFromEventsAuthority(t *testing.T) {
	t.Parallel()

	swapping := &payloadSwappingEventsService{inner: newMemoryEventsService()}
	topic := events.Topic("factory-session/session-events-authority/response-events")
	store := newEventsAuthorityStore(t, swapping, "session-events-authority")

	publishThroughEventsAuthority(t, swapping, store, topic)

	// The store's own retained copy still holds the original content:
	// PublishThroughAuthority commits exactly what it sent to Events, before
	// the swapping wrapper's Read ever runs against it.
	storeEvents := store.Events()
	if len(storeEvents) != 1 || storeEvents[0].RunID != "run-test" {
		t.Fatalf("store.Events() = %#v, want exactly one retained event with the original RunID", storeEvents)
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	delivered, err := subscription.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(delivered) != 1 {
		t.Fatalf("Next() delivered %d events, want 1", len(delivered))
	}
	if delivered[0].RunID != "swapped-by-events-read" {
		t.Fatalf("delivered[0].RunID = %q, want content read back from the injected Events root, not the store's own retained copy", delivered[0].RunID)
	}
}

// gapReportingEventsService always answers Read with ReadOutcomeGap,
// simulating Events having evicted a position this store's own tiered
// retention still retains.
type gapReportingEventsService struct {
	inner events.Service
}

func (g *gapReportingEventsService) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return g.inner.Append(ctx, req)
}

func (g *gapReportingEventsService) AttachSource(ctx context.Context, req events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return g.inner.AttachSource(ctx, req)
}

func (g *gapReportingEventsService) Read(_ context.Context, req events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{
		Outcome: events.ReadOutcomeGap,
		Gap: &events.GapFacts{
			Topic:            req.Topic,
			Requested:        req.From.Position,
			EarliestRetained: req.From.Position + events.AggregateSequence(req.Limit) + 1,
			Head:             req.From.Position + events.AggregateSequence(req.Limit) + 1,
		},
	}, nil
}

func (g *gapReportingEventsService) Subscribe(ctx context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	return g.inner.Subscribe(ctx, req)
}

var _ events.Service = (*gapReportingEventsService)(nil)

// TestSessionResponseEventStoreSubscription_SurfacesGapWhenEventsAuthorityCannotProduceContent
// proves substituteFromEvents never falls back to the store's own locally
// retained bytes when the injected Events root cannot currently produce
// content for an otherwise-deliverable record: it must surface the same
// published stream-gap vocabulary the store already uses for its own
// retention gaps instead (PR #1753 review finding, 2026-08-03T16:05:34Z).
func TestSessionResponseEventStoreSubscription_SurfacesGapWhenEventsAuthorityCannotProduceContent(t *testing.T) {
	t.Parallel()

	gapping := &gapReportingEventsService{inner: newMemoryEventsService()}
	topic := events.Topic("factory-session/session-events-authority-gap/response-events")
	store := newEventsAuthorityStore(t, gapping, "session-events-authority-gap")

	published := publishThroughEventsAuthority(t, gapping, store, topic)

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	delivered, err := subscription.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(delivered) != 1 {
		t.Fatalf("Next() delivered %d events, want 1", len(delivered))
	}
	if delivered[0].Kind != responseevents.KindStreamGap {
		t.Fatalf("delivered[0].Kind = %q, want %q (Events being unable to confirm a record must surface as a gap, never the store's own local bytes)", delivered[0].Kind, responseevents.KindStreamGap)
	}
	if delivered[0].EventID == published.EventID {
		t.Fatalf("delivered[0].EventID = %q: a record Events cannot confirm must never be delivered under its original identity", delivered[0].EventID)
	}

	// The store's own retained copy is untouched by the substitution: it
	// still holds the original content, proving the gap is a delivery-time
	// decision, not a mutation of retained state.
	storeEvents := store.Events()
	if len(storeEvents) != 1 || storeEvents[0].EventID != published.EventID {
		t.Fatalf("store.Events() = %#v, want the original published record still retained locally", storeEvents)
	}
}
