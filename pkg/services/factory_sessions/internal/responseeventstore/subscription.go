package responseeventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
)

type storeSubscriber struct {
	done chan struct{}
	once sync.Once
	wake chan struct{}
}

func newStoreSubscriber() *storeSubscriber {
	return &storeSubscriber{
		done: make(chan struct{}),
		wake: make(chan struct{}, 1),
	}
}

func (s *storeSubscriber) notify() {
	if s == nil {
		return
	}
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *storeSubscriber) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.done)
	})
}

// SubscribeOption configures one response-event store subscription.
type SubscribeOption func(*subscribeConfig)

type subscribeConfig struct {
	dispatchID        string
	hasDispatchFilter bool
}

// WithDispatchFilter limits delivery to events whose dispatchId matches the
// supplied identity. An empty dispatch ID is rejected.
func WithDispatchFilter(dispatchID string) SubscribeOption {
	trimmed := strings.TrimSpace(dispatchID)
	return func(config *subscribeConfig) {
		config.dispatchID = trimmed
		config.hasDispatchFilter = true
	}
}

// Subscription is a catch-up-then-live cursor over one session response-event store.
type Subscription struct {
	store        *SessionResponseEventStore
	subscriber   *storeSubscriber
	subscriberID int64

	mu            sync.Mutex
	afterSequence int64
	dispatchID    string
	detached      bool
}

// Subscribe registers one consumer starting after the supplied sequence. The
// subscriber can call Next to drain retained events with sequence greater than
// afterSequence, then continue receiving live publishes in ascending order.
// Optional dispatch filters omit non-matching events while preserving each
// delivered event's global session sequence and eventId.
func (s *SessionResponseEventStore) Subscribe(afterSequence int64, opts ...SubscribeOption) (*Subscription, error) {
	if s == nil {
		return nil, ErrStoreClosed
	}

	var config subscribeConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}
	if config.hasDispatchFilter && config.dispatchID == "" {
		return nil, errInvalidDispatchFilter
	}

	subscriber := newStoreSubscriber()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrStoreClosed
	}
	if s.completed && !s.storeNowLocked().Before(s.completedAt.Add(CompletedStreamRetentionWindow)) {
		return nil, ErrStoreExpired
	}
	if s.completed {
		return &Subscription{
			store:         s,
			subscriber:    subscriber,
			afterSequence: afterSequence,
			dispatchID:    config.dispatchID,
		}, nil
	}
	s.nextSubID++
	subscriberID := s.nextSubID
	s.subscribers[subscriberID] = subscriber
	return &Subscription{
		store:         s,
		subscriber:    subscriber,
		subscriberID:  subscriberID,
		afterSequence: afterSequence,
		dispatchID:    config.dispatchID,
	}, nil
}

// DispatchFilter returns the dispatch identity limiting this subscription, or
// empty when the subscription is unfiltered.
func (s *Subscription) DispatchFilter() string {
	if s == nil {
		return ""
	}
	return s.dispatchID
}

// SubscriberCount reports active subscription registrations on the store.
func (s *SessionResponseEventStore) SubscriberCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

// Complete stops further publication while retaining buffered events so
// existing and late subscribers can drain ordered progress until the store is
// fully closed.
func (s *SessionResponseEventStore) Complete() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed || s.completed {
		s.mu.Unlock()
		return
	}
	s.completed = true
	s.completedAt = s.storeNowLocked()
	subscribers := s.subscribersSnapshotLocked()
	s.subscribers = make(map[int64]*storeSubscriber)
	s.mu.Unlock()
	closeStoreSubscribers(subscribers)
}

// Close rejects new subscriptions and publishes, detaches active subscribers,
// and makes subsequent subscription reads return a closed outcome. Retained
// events remain readable through snapshot APIs.
func (s *SessionResponseEventStore) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	subscribers := s.subscribersSnapshotLocked()
	s.subscribers = make(map[int64]*storeSubscriber)
	s.mu.Unlock()
	closeStoreSubscribers(subscribers)
}

func (s *SessionResponseEventStore) storeNowLocked() time.Time {
	return s.clock.Now().UTC()
}

// Detach releases the store-owned subscriber registration.
func (s *Subscription) Detach() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.detached = true
	s.mu.Unlock()

	store := s.store
	if store != nil {
		store.detachSubscriber(s.subscriberID)
	}
	if s.subscriber != nil {
		s.subscriber.close()
	}
}

// Next returns the next retained or live events after the subscription cursor.
func (s *Subscription) Next(ctx context.Context) ([]responseevents.FactoryResponseEvent, error) {
	if s == nil || s.store == nil || s.subscriber == nil {
		return nil, ErrSubscriptionClosed
	}
	for {
		s.mu.Lock()
		detached := s.detached
		afterSequence := s.afterSequence
		dispatchID := s.dispatchID
		s.mu.Unlock()
		if detached {
			return nil, ErrSubscriptionClosed
		}

		result, closed := s.store.readForSubscriber(afterSequence, dispatchID)
		if closed {
			return nil, ErrSubscriptionClosed
		}
		if len(result.events) > 0 {
			s.advance(result.nextSequence)
			return result.events, nil
		}

		select {
		case <-ctx.Done():
			// A publish and cancellation can become ready together. Re-read the
			// retained store before honoring cancellation so callers can use
			// cancellation as a deterministic terminal drain boundary.
			result, closed = s.store.readForSubscriber(afterSequence, dispatchID)
			if closed {
				return nil, ErrSubscriptionClosed
			}
			if len(result.events) > 0 {
				s.advance(result.nextSequence)
				return result.events, nil
			}
			return nil, ctx.Err()
		case <-s.subscriber.done:
			s.mu.Lock()
			detached = s.detached
			s.mu.Unlock()
			if detached {
				return nil, ErrSubscriptionClosed
			}
			// Complete/Close may close done while a retained event is already
			// available; re-read before honoring completion.
			continue
		case <-s.subscriber.wake:
		}
	}
}

// Drain returns all events currently retained after the subscription cursor
// without waiting for another publish. It is intended for terminal handoffs
// that have already stopped their live consumer.
func (s *Subscription) Drain() ([]responseevents.FactoryResponseEvent, error) {
	if s == nil || s.store == nil || s.subscriber == nil {
		return nil, ErrSubscriptionClosed
	}
	s.mu.Lock()
	detached := s.detached
	afterSequence := s.afterSequence
	dispatchID := s.dispatchID
	s.mu.Unlock()
	if detached {
		return nil, ErrSubscriptionClosed
	}

	result, closed := s.store.readForSubscriber(afterSequence, dispatchID)
	if closed {
		return nil, ErrSubscriptionClosed
	}
	if len(result.events) > 0 {
		s.advance(result.nextSequence)
	}
	return result.events, nil
}

func (s *Subscription) advance(sequence int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sequence > s.afterSequence {
		s.afterSequence = sequence
	}
}

type subscriberRead struct {
	events       []responseevents.FactoryResponseEvent
	nextSequence int64
}

func (s *SessionResponseEventStore) readForSubscriber(afterSequence int64, dispatchID string) (subscriberRead, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return subscriberRead{}, true
	}
	result := s.eventsAfterLocked(afterSequence, dispatchID)
	if s.completed && len(result.events) == 0 {
		return subscriberRead{}, true
	}
	return result, false
}

func (s *SessionResponseEventStore) eventsAfterLocked(afterSequence int64, dispatchID string) subscriberRead {
	result := subscriberRead{nextSequence: afterSequence}
	if from, to, ok := droppedBoundsAfter(s.droppedSequences, afterSequence); ok {
		firstAvailable := s.firstAvailableSequenceLocked(afterSequence, dispatchID)
		result.events = append(result.events, s.retentionGapEventLocked(from, to, firstAvailable))
		result.nextSequence = to
	}
	for _, event := range s.events {
		if event.Sequence <= afterSequence {
			continue
		}
		if dispatchID != "" && !dispatchMatches(event.DispatchID, dispatchID) {
			continue
		}
		result.events = append(result.events, cloneEvent(event))
		if event.Sequence > result.nextSequence {
			result.nextSequence = event.Sequence
		}
	}
	return result
}

func (s *SessionResponseEventStore) firstAvailableSequenceLocked(afterSequence int64, dispatchID string) int64 {
	for _, event := range s.events {
		if event.Sequence > afterSequence && (dispatchID == "" || dispatchMatches(event.DispatchID, dispatchID)) {
			return event.Sequence
		}
	}
	return s.nextSequence + 1
}

// retentionGapEventLocked creates an out-of-band marker. Sequence zero is
// reserved for this synthetic read result: the marker is never stored, never
// participates in retention, and never consumes or impersonates a published
// sequence. Its deterministic event ID identifies the same cursor-relative gap
// consistently without entering the session's published identity space.
func (s *SessionResponseEventStore) retentionGapEventLocked(from, to, firstAvailable int64) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.StreamGapPayload{
		FromSequence:           from,
		ToSequence:             to,
		FirstAvailableSequence: firstAvailable,
		Reason:                 retentionGapReason,
	})
	identity := fmt.Sprintf("%s:%d:%d:%d:%s", s.factorySessionID, from, to, firstAvailable, retentionGapReason)
	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String(),
		Sequence:         0,
		RecordedAt:       s.storeNowLocked(),
		FactorySessionID: s.factorySessionID,
		Kind:             responseevents.KindStreamGap,
		Phase:            responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider:        "you-agent-factory",
			NativeEventType: "response.retention_gap",
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossy,
		},
		Payload: payload,
	}
}

func dispatchMatches(eventDispatchID, filterDispatchID string) bool {
	return strings.TrimSpace(eventDispatchID) == strings.TrimSpace(filterDispatchID)
}

func (s *SessionResponseEventStore) detachSubscriber(id int64) {
	if s == nil || id == 0 {
		return
	}
	s.mu.Lock()
	subscriber := s.subscribers[id]
	delete(s.subscribers, id)
	s.mu.Unlock()
	if subscriber != nil {
		subscriber.close()
	}
}

func (s *SessionResponseEventStore) subscribersSnapshotLocked() []*storeSubscriber {
	if len(s.subscribers) == 0 {
		return nil
	}
	subscribers := make([]*storeSubscriber, 0, len(s.subscribers))
	for _, subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	return subscribers
}

func notifyStoreSubscribers(subscribers []*storeSubscriber) {
	for _, subscriber := range subscribers {
		subscriber.notify()
	}
}

func closeStoreSubscribers(subscribers []*storeSubscriber) {
	for _, subscriber := range subscribers {
		subscriber.close()
	}
}
