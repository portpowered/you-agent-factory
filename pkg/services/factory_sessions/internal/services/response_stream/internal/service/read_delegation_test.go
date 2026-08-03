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
// normally but always failing Read, to prove a transient Events read
// failure degrades gracefully to the store's own already-guaranteed-identical
// retained copy instead of failing or hanging a subscriber's delivery.
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

// TestService_SubscribeFallsBackToStoreCopyWhenEventsReadFails proves a
// transient failure reading back from the injected Events root never fails
// or hangs a subscriber: delivery falls back to the store's own retained
// copy, which PublishThroughAuthority already guarantees is identical to
// what Events accepted (see TestService_ConcurrentPublishAndCompletionNeverDivergesFromEvents).
func TestService_SubscribeFallsBackToStoreCopyWhenEventsReadFails(t *testing.T) {
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
	if delivered[0].DispatchID != "dispatch-1" {
		t.Fatalf("delivered[0].DispatchID = %q, want %q (a failed Events read must fall back to the store's own retained copy, not drop the delivery)", delivered[0].DispatchID, "dispatch-1")
	}
}
