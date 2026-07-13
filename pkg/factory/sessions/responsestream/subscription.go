package responsestream

import (
	"context"
	"errors"
	"sync"
)

var ErrSubscriptionClosed = errors.New("session response stream subscription is closed")

type streamSubscriber struct {
	done chan struct{}
	once sync.Once
	wake chan struct{}
}

func newStreamSubscriber() *streamSubscriber {
	return &streamSubscriber{
		done: make(chan struct{}),
		wake: make(chan struct{}, 1),
	}
}

func (s *streamSubscriber) notify() {
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

func (s *streamSubscriber) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.done)
	})
}

// Subscription is an internal live-session response-stream cursor.
type Subscription struct {
	stream       *SessionResponseStream
	subscriber   *streamSubscriber
	subscriberID int64

	mu            sync.Mutex
	afterSequence int64
}

// Subscribe registers one internal consumer starting after the supplied
// sequence. The subscriber can immediately call Next to read the retained
// window, then continue reading live updates in order.
func (s *SessionResponseStream) Subscribe(afterSequence int64) (*Subscription, error) {
	if s == nil {
		return nil, ErrSubscriptionClosed
	}
	subscriber := newStreamSubscriber()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrSubscriptionClosed
	}
	s.nextSubID++
	subscriberID := s.nextSubID
	s.subscribers[subscriberID] = subscriber
	return &Subscription{
		stream:        s,
		subscriber:    subscriber,
		subscriberID:  subscriberID,
		afterSequence: afterSequence,
	}, nil
}

// SubscriberCount reports the number of active internal subscribers currently
// attached to the stream.
func (s *SessionResponseStream) SubscriberCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

// CompleteDispatch detaches live subscribers and stops further publication
// while retaining the buffered window so late consumers can still subscribe
// and drain ordered progress until the stream is fully closed.
func (s *SessionResponseStream) CompleteDispatch() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed || s.dispatchCompleted {
		s.mu.Unlock()
		return
	}
	s.dispatchCompleted = true
	s.completedAt = s.clock.Now().UTC()
	subscribers := s.subscribersSnapshotLocked()
	s.subscribers = make(map[int64]*streamSubscriber)
	s.mu.Unlock()
	closeSubscribers(subscribers)
}

// Close detaches all subscribers and prevents further publication or
// subscription on the stream.
func (s *SessionResponseStream) Close() {
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
	s.subscribers = make(map[int64]*streamSubscriber)
	s.mu.Unlock()
	closeSubscribers(subscribers)
}

// Detach releases the runtime-owned subscriber registration.
func (s *Subscription) Detach() {
	if s == nil {
		return
	}
	stream := s.stream
	if stream != nil {
		stream.detachSubscriber(s.subscriberID)
	}
	if s.subscriber != nil {
		s.subscriber.close()
	}
}

// Next returns the next retained or live stream segment after the subscriber's
// current cursor.
func (s *Subscription) Next(ctx context.Context) (ReadResult, error) {
	if s == nil || s.stream == nil || s.subscriber == nil {
		return ReadResult{}, ErrSubscriptionClosed
	}
	for {
		s.mu.Lock()
		afterSequence := s.afterSequence
		s.mu.Unlock()

		result, closed := s.stream.readForSubscriber(afterSequence)
		if closed {
			return ReadResult{}, ErrSubscriptionClosed
		}
		if len(result.Events) > 0 || result.BehindRetainedWindow || result.Compaction != nil {
			s.advance(result)
			return result, nil
		}

		select {
		case <-ctx.Done():
			return ReadResult{}, ctx.Err()
		case <-s.subscriber.done:
			return ReadResult{}, ErrSubscriptionClosed
		case <-s.subscriber.wake:
		}
	}
}

func (s *Subscription) advance(result ReadResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(result.Events) > 0 {
		s.afterSequence = result.Events[len(result.Events)-1].Sequence
		return
	}
	if result.FirstRetainedSequence > 0 {
		s.afterSequence = result.FirstRetainedSequence - 1
	}
}

func (s *SessionResponseStream) readForSubscriber(afterSequence int64) (ReadResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ReadResult{}, true
	}
	result := s.eventsAfterLocked(afterSequence)
	if s.dispatchCompleted &&
		len(result.Events) == 0 &&
		!result.BehindRetainedWindow &&
		result.Compaction == nil {
		return ReadResult{}, true
	}
	return result, false
}

func (s *SessionResponseStream) detachSubscriber(id int64) {
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

func notifySubscribers(subscribers []*streamSubscriber) {
	for _, subscriber := range subscribers {
		subscriber.notify()
	}
}

func closeSubscribers(subscribers []*streamSubscriber) {
	for _, subscriber := range subscribers {
		subscriber.close()
	}
}
