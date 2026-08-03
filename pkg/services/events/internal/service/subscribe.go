package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// liveSubscriber is one topic's live registration: records committed after
// the subscriber's registration point are offered into a bounded buffer in
// commit order. The buffer's capacity is the subscription's bounded live
// delivery pressure policy (SubscribeRequest.Limit): once it is full, the
// subscriber is terminated with DeliveryBackpressure instead of blocking the
// committing Append or silently dropping a record without telling the
// consumer. Closing records (rather than using a second signal channel)
// lets a consumer still drain every record that made it into the buffer
// before it or observes the fixed terminal kind: Go guarantees a closed
// channel's already-buffered values are read before the zero-value/!ok case.
type liveSubscriber struct {
	records chan events.Record

	mu     sync.Mutex
	closed bool
	kind   events.DeliveryKind
}

func newLiveSubscriber(capacity int) *liveSubscriber {
	return &liveSubscriber{records: make(chan events.Record, capacity)}
}

// deliver offers rec to s without blocking. It reports false when s's
// buffer is already full, in which case s is terminated with
// DeliveryBackpressure; the caller (Append, holding the owning topic's lock)
// must remove s from the topic's live registration so no further record is
// ever offered to it.
func (s *liveSubscriber) deliver(rec events.Record) bool {
	select {
	case s.records <- rec:
		return true
	default:
		s.terminate(events.DeliveryBackpressure)
		return false
	}
}

// terminate closes s's buffer with kind exactly once; later calls are
// no-ops. Any record already buffered before the first terminate call still
// drains normally through Next; only once the buffer is empty does a reader
// observe kind.
func (s *liveSubscriber) terminate(kind events.DeliveryKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.kind = kind
	close(s.records)
}

func (s *liveSubscriber) terminalKind() events.DeliveryKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kind
}

// notifySubscribersLocked offers rec (already detached by the caller) to
// every currently registered live subscriber, using an independently
// detached copy per subscriber so mutation observed through one
// subscription's Delivery can never alias another's or the store's own
// retained copy. Any subscriber whose buffer is already full is removed
// from ts.subscribers so Append never offers it another record, and the
// backpressure detection is logged exactly once, at the point of
// detection, rather than at each later Next() observation of the same
// fixed terminal outcome. Callers hold ts.mu.
func (ts *topicState) notifySubscribersLocked(st *Store, topic events.Topic, rec events.Record) {
	for id, sub := range ts.subscribers {
		if !sub.deliver(rec.Detached()) {
			delete(ts.subscribers, id)
			st.logSubscribeBackpressure(topic)
		}
	}
}

// catchupLocked returns the retained records after from (in commit order)
// that a new subscription must deliver before switching to live delivery,
// plus GapFacts when from names a position this topic no longer retains.
// This mirrors topicState.readLocked's outcome resolution exactly, except a
// gap does not stop the subscription: after reporting the gap once, the
// subscription resumes catch-up from the earliest still-retained position,
// since Subscribe's Delivery contract has no cursor-carrying InvalidCursor
// analogue and must instead recover deterministically on the caller's
// behalf. Callers hold ts.mu.
func (ts *topicState) catchupLocked(topic events.Topic, from events.AggregateSequence) ([]events.Record, *events.GapFacts) {
	head := ts.head
	earliest := ts.earliestLocked()

	var gap *events.GapFacts
	startAfter := from
	if earliest > 1 && from < earliest {
		gap = &events.GapFacts{Topic: topic, Requested: from, EarliestRetained: earliest, Head: head}
		startAfter = earliest - 1
	}
	if startAfter >= head {
		return nil, gap
	}

	startIndex := int(startAfter + 1 - earliest)
	src := ts.records[startIndex:]
	out := make([]events.Record, len(src))
	for i, rec := range src {
		out[i] = rec.Detached()
	}
	return out, gap
}

// Subscribe starts a subscription over req.Topic's aggregate ordering. A
// canceled context or malformed request is rejected before any topic state
// is touched or any log is emitted, matching Read. A starting cursor beyond
// the topic's live head is a well-formed but unresolvable position
// (ErrUnresolvableCursor): unlike Read, Subscribe's Delivery vocabulary has
// no successful "invalid cursor" outcome to report it as, so it must be an
// operation failure. Otherwise Subscribe always succeeds: the returned
// Subscription delivers any retained catch-up records (preceded by one
// DeliveryGap if the starting position was evicted), then live records in
// commit order with no missing or duplicate aggregate position across the
// handoff, because the live registration is captured under the same lock as
// the catch-up snapshot.
func (st *Store) Subscribe(ctx context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	ts := st.topic(req.Topic)
	ts.mu.Lock()
	if req.From.Position > ts.head {
		ts.mu.Unlock()
		st.logSubscribeRejected(req, events.ErrUnresolvableCursor)
		return nil, events.ErrUnresolvableCursor
	}

	catchup, gap := ts.catchupLocked(req.Topic, req.From.Position)
	sub := newLiveSubscriber(req.Limit)
	var subID uint64
	if ts.closed {
		sub.terminate(events.DeliveryClosed)
	} else {
		ts.nextSubID++
		subID = ts.nextSubID
		ts.subscribers[subID] = sub
	}
	ts.mu.Unlock()

	st.logSubscribeAccepted(req)
	if gap != nil {
		st.logSubscribeGap(req.Topic, gap)
	}

	state := &subscriptionState{
		store:   st,
		topic:   req.Topic,
		sub:     sub,
		subID:   subID,
		catchup: catchup,
		gap:     gap,
	}
	return events.Subscription(state.next), nil
}

// subscriptionState is the mutable cursor behind one Subscribe call's
// returned events.Subscription closure. subID is the topic-scoped
// registration key used to unregister sub from ts.subscribers on
// cancellation; it is zero when the subscription started already closed
// (never registered in the first place, so there is nothing to unregister).
type subscriptionState struct {
	store *Store
	topic events.Topic
	sub   *liveSubscriber
	subID uint64

	mu       sync.Mutex
	gap      *events.GapFacts
	catchup  []events.Record
	canceled bool
}

// next observes the next Delivery: once this subscription has ever observed
// a canceled context, it is permanently terminal -- every later call, even
// with a fresh, uncanceled context, deterministically returns the same
// DeliveryCanceled outcome rather than resuming catch-up/gap/live delivery.
// Otherwise a canceled context is checked first (this can fire on every call
// since a Subscription is long-lived), then any pending gap notice, then any
// remaining catch-up record, then a live record or the fixed terminal kind
// once the live buffer is closed and drained.
func (s *subscriptionState) next(ctx context.Context) events.Delivery {
	s.mu.Lock()
	alreadyCanceled := s.canceled
	s.mu.Unlock()
	if alreadyCanceled {
		return s.cancel()
	}

	if err := ctx.Err(); err != nil {
		return s.cancel()
	}

	s.mu.Lock()
	if s.gap != nil {
		gap := s.gap
		s.gap = nil
		s.mu.Unlock()
		return events.Delivery{Kind: events.DeliveryGap, Gap: gap}
	}
	if len(s.catchup) > 0 {
		rec := s.catchup[0]
		s.catchup = s.catchup[1:]
		s.mu.Unlock()
		return events.Delivery{Kind: events.DeliveryRecord, Record: rec, Cursor: events.Cursor{Topic: s.topic, Position: rec.ID.Position}}
	}
	s.mu.Unlock()

	select {
	case rec, ok := <-s.sub.records:
		if !ok {
			return events.Delivery{Kind: s.sub.terminalKind()}
		}
		return events.Delivery{Kind: events.DeliveryRecord, Record: rec, Cursor: events.Cursor{Topic: s.topic, Position: rec.ID.Position}}
	case <-ctx.Done():
		return s.cancel()
	}
}

// cancel atomically terminalizes s with DeliveryCanceled: it discards any
// pending gap/catch-up so a later call (even with a fresh context) cannot
// resume delivering them, unregisters the underlying live subscriber from
// its topic so no future commit is ever offered to it again (proving
// append-after-cancel cannot deliver), and closes the live buffer so every
// later Next observes the same terminal kind. Only the first call has any
// side effect (unregistering, terminating, logging); later calls just
// re-report the already-persisted terminal kind.
//
// Unregistration happens strictly before termination (closing s.sub.records).
// unregisterSubscriber takes the owning topicState's mutex, the same mutex
// Append holds for the entire duration of notifySubscribersLocked's send to
// s.sub.records: acquiring it here therefore cannot return until any
// concurrently in-flight deliver() call for this subscriber has already
// completed, and once it returns the subscriber is no longer in
// ts.subscribers so no future Append can start a new one. Only after that
// happens is it safe to close s.sub.records -- reversing this order (closing
// first, unregistering after, as an earlier revision did) leaves a window
// where Append's notifySubscribersLocked can still be mid-send on a channel
// this call is concurrently closing, which is a send-on-closed-channel
// panic, not just a lost delivery.
func (s *subscriptionState) cancel() events.Delivery {
	s.mu.Lock()
	first := !s.canceled
	s.canceled = true
	s.gap = nil
	s.catchup = nil
	s.mu.Unlock()

	if first {
		s.store.unregisterSubscriber(s.topic, s.subID)
	}
	s.sub.terminate(events.DeliveryCanceled)
	if first {
		s.store.logSubscribeCanceled(s.topic)
	}
	return events.Delivery{Kind: s.sub.terminalKind()}
}

// unregisterSubscriber removes id from topic's live registration when still
// present. id == 0 means the subscription started already closed (never
// registered), and a missing id is a harmless no-op (for example, backpressure
// eviction or topic closure already removed it).
func (st *Store) unregisterSubscriber(topic events.Topic, id uint64) {
	if id == 0 {
		return
	}
	ts := st.topic(topic)
	ts.mu.Lock()
	delete(ts.subscribers, id)
	ts.mu.Unlock()
}
