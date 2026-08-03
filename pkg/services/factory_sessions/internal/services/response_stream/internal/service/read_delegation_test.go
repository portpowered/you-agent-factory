package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// payloadSwappingEventsService wraps a real events.Service and rewrites
// every Read's returned payload content before returning it, leaving Append
// untouched. It is used to prove that SubscribeFactoryResponseEvents'
// delivered content is genuinely sourced from the injected Events root's own
// Read, not merely from the session-owned store's independently retained
// copy: if delivery ever reverted to reading the local copy directly, the
// swap this double performs would never be observed by a subscriber.
type payloadSwappingEventsService struct {
	inner events.Service
}

const swappedDispatchID = "swapped-by-events-read"

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
			decoded.DispatchID = swappedDispatchID
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

// TestService_SubscribeDeliversContentReadBackFromEventsNotTheLocalStoreCopy
// is the decisive delegation proof the PR review asked for: a test that
// would fail if SubscribeFactoryResponseEvents were ever switched back to
// reading only the session-owned store's own retained copy. The store still
// retains its own copy of every published event (used for the tiered
// retention/gap decisions Events has no equivalent for), but the content a
// subscriber actually observes is fetched back from the injected Events
// root's Read -- proven here by a wrapper that rewrites Read's returned
// content and asserting the subscriber observes the rewritten value, not the
// store's original.
func TestService_SubscribeDeliversContentReadBackFromEventsNotTheLocalStoreCopy(t *testing.T) {
	t.Parallel()

	swapping := &payloadSwappingEventsService{inner: newTestEventsService(t)}
	service, err := serviceWithEvents(t, swapping)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	published := publishThroughService(t, service, store, responseevents.KindMessage, "original-dispatch")
	if published.DispatchID != "original-dispatch" {
		t.Fatalf("published.DispatchID = %q, want %q", published.DispatchID, "original-dispatch")
	}

	// The store's own retained copy holds the original, unmodified content:
	// PublishThroughAuthority commits exactly what it sent to Events, before
	// the swapping wrapper's Read ever runs against it.
	storeEvents := store.Events()
	if len(storeEvents) != 1 || storeEvents[0].DispatchID != "original-dispatch" {
		t.Fatalf("store.Events() = %#v, want exactly one retained event with DispatchID %q", storeEvents, "original-dispatch")
	}

	subscribed, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscribed.Detach()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	delivered, err := subscribed.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(delivered) != 1 {
		t.Fatalf("Next() delivered %d events, want 1", len(delivered))
	}
	if delivered[0].DispatchID != swappedDispatchID {
		t.Fatalf("delivered[0].DispatchID = %q, want %q (delivered content must be read back from the injected Events root, not the store's own retained copy)", delivered[0].DispatchID, swappedDispatchID)
	}
}

// readFailingEventsService wraps a real events.Service, delegating Append
// normally but always failing Read, to prove a failed Events read never
// fails or hangs a subscriber's delivery and never falls back to the
// store's own retained copy -- it must surface an honest gap instead.
type readFailingEventsService struct {
	inner events.Service
}

var errReadFailingEventsServiceRead = errors.New("readFailingEventsService: Read always fails")

func (r *readFailingEventsService) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return r.inner.Append(ctx, req)
}

func (r *readFailingEventsService) AttachSource(ctx context.Context, req events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return r.inner.AttachSource(ctx, req)
}

func (r *readFailingEventsService) Read(context.Context, events.ReadRequest) (events.ReadResult, error) {
	return events.ReadResult{}, errReadFailingEventsServiceRead
}

func (r *readFailingEventsService) Subscribe(ctx context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	return r.inner.Subscribe(ctx, req)
}

var _ events.Service = (*readFailingEventsService)(nil)

// TestService_SubscribeSurfacesGapWhenEventsReadFails proves a failure
// reading back from the injected Events root never silently falls back to
// the store's own retained copy: the compatibility surface must never emit
// content a direct Events reader could not also observe (PR #1753 review
// finding, 2026-08-03T16:05:34Z), so a failed Events read instead surfaces
// the same published stream-gap vocabulary this store already uses for its
// own retention gaps, and the failed sequence is never delivered as if it
// had succeeded.
func TestService_SubscribeSurfacesGapWhenEventsReadFails(t *testing.T) {
	t.Parallel()

	failing := &readFailingEventsService{inner: newTestEventsService(t)}
	service, err := serviceWithEvents(t, failing)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")

	subscribed, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscribed.Detach()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	delivered, err := subscribed.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(delivered) != 1 {
		t.Fatalf("Next() delivered %d events, want 1", len(delivered))
	}
	if delivered[0].Kind != responseevents.KindStreamGap {
		t.Fatalf("delivered[0].Kind = %q, want %q (a failed Events read must surface an honest gap, not the store's own local content)", delivered[0].Kind, responseevents.KindStreamGap)
	}
	if delivered[0].DispatchID == "dispatch-1" {
		t.Fatalf("delivered[0].DispatchID = %q: the original record's content must never be delivered when Events could not confirm it", delivered[0].DispatchID)
	}
}

// gapReportingEventsService wraps a real events.Service, delegating Append
// normally but always answering Read with ReadOutcomeGap, to simulate Events'
// own bounded FIFO retention having evicted a position that this store's
// separate, tiered (importance-based, not purely recency-based) local
// retention policy still retains -- the scenario a real session exceeding
// Events' fixed per-topic retention cap after a long enough mix of
// high-priority and low-priority events would eventually reach. Simulating
// the terminal Gap outcome directly (rather than actually publishing beyond
// the cap) proves the exact same code path deterministically without an
// impractically large/slow test.
type gapReportingEventsService struct {
	inner events.Service
}

func (g *gapReportingEventsService) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return g.inner.Append(ctx, req)
}

func (g *gapReportingEventsService) AttachSource(ctx context.Context, req events.AttachSourceRequest) (events.AttachSourceResult, error) {
	return g.inner.AttachSource(ctx, req)
}

func (g *gapReportingEventsService) Read(ctx context.Context, req events.ReadRequest) (events.ReadResult, error) {
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

// TestService_SubscribeSurfacesGapWhenEventsHasEvictedAStillLocallyRetainedRecord
// proves the mixed-tier retention divergence the PR review flagged: this
// store's own tiered retention can keep a record (for example a
// final-semantic completed message) long after Events' fixed, purely-FIFO
// per-topic window has evicted the same position. When that happens, the
// compatibility surface must surface the same published gap vocabulary a
// direct Events reader would observe, never the store's own stale bytes --
// otherwise the two surfaces would deterministically disagree about what
// exists, which is exactly what "delegates to Events" must rule out.
func TestService_SubscribeSurfacesGapWhenEventsHasEvictedAStillLocallyRetainedRecord(t *testing.T) {
	t.Parallel()

	gapping := &gapReportingEventsService{inner: newTestEventsService(t)}
	service, err := serviceWithEvents(t, gapping)
	if err != nil {
		t.Fatalf("construct response-stream service: %v", err)
	}
	store := newStore(t, service)

	// A final-semantic completed message: exactly the retention tier this
	// store's own local policy would keep even after many more (lower
	// priority) events than Events' own bounded window retains.
	published, err := store.Publish(responseevents.FactoryResponseEvent{
		DispatchID: "dispatch-completed",
		RunID:      "run-1",
		Kind:       responseevents.KindMessage,
		Phase:      responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider: "test", NativeEventType: "message.completed",
			Delivery:       responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationSnapshot,
			Fidelity:       responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"final"}]}`),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The store's own retained copy genuinely still has the record: only
	// Events (via the double) reports it gone.
	storeEvents := store.Events()
	if len(storeEvents) != 1 || storeEvents[0].EventID != published.EventID {
		t.Fatalf("store.Events() = %#v, want exactly the published completed-message record still retained locally", storeEvents)
	}

	subscribed, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscribed.Detach()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	delivered, err := subscribed.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(delivered) != 1 {
		t.Fatalf("Next() delivered %d events, want 1", len(delivered))
	}
	if delivered[0].Kind != responseevents.KindStreamGap {
		t.Fatalf("delivered[0].Kind = %q, want %q (a record Events has evicted must surface as a gap even when this store's own tiered policy still retains it)", delivered[0].Kind, responseevents.KindStreamGap)
	}
	if delivered[0].EventID == published.EventID {
		t.Fatalf("delivered[0].EventID = %q: a record Events cannot confirm must never be delivered under its original identity", delivered[0].EventID)
	}
}
