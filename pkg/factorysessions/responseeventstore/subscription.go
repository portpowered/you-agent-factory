package responseeventstore

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
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

// Subscription is a catch-up-then-live cursor over one session response-event store.
type Subscription struct {
	store        *SessionResponseEventStore
	subscriber   *storeSubscriber
	subscriberID int64

	mu            sync.Mutex
	afterSequence int64
}

// Subscribe registers one consumer starting after the supplied sequence. The
// subscriber can call Next to drain retained events with sequence greater than
// afterSequence, then continue receiving live publishes in ascending order.
func (s *SessionResponseEventStore) Subscribe(afterSequence int64) (*Subscription, error) {
	if s == nil {
		return nil, ErrStoreClosed
	}

	subscriber := newStoreSubscriber()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrStoreClosed
	}
	s.nextSubID++
	subscriberID := s.nextSubID
	s.subscribers[subscriberID] = subscriber
	return &Subscription{
		store:         s,
		subscriber:    subscriber,
		subscriberID:  subscriberID,
		afterSequence: afterSequence,
	}, nil
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

// Close rejects new subscriptions and detaches active subscribers. Retained
// events remain readable through snapshot APIs until a later story adds
// completion semantics.
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

// Detach releases the store-owned subscriber registration.
func (s *Subscription) Detach() {
	if s == nil {
		return
	}
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
		afterSequence := s.afterSequence
		s.mu.Unlock()

		events, closed := s.store.readForSubscriber(afterSequence)
		if closed {
			return nil, ErrSubscriptionClosed
		}
		if len(events) > 0 {
			s.advance(events)
			return events, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.subscriber.done:
			return nil, ErrSubscriptionClosed
		case <-s.subscriber.wake:
		}
	}
}

func (s *Subscription) advance(events []responseevents.FactoryResponseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterSequence = events[len(events)-1].Sequence
}

func (s *SessionResponseEventStore) readForSubscriber(afterSequence int64) ([]responseevents.FactoryResponseEvent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, true
	}
	return s.eventsAfterLocked(afterSequence), false
}

func (s *SessionResponseEventStore) eventsAfterLocked(afterSequence int64) []responseevents.FactoryResponseEvent {
	if len(s.events) == 0 {
		return nil
	}
	out := make([]responseevents.FactoryResponseEvent, 0)
	for _, event := range s.events {
		if event.Sequence > afterSequence {
			out = append(out, cloneEvent(event))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
