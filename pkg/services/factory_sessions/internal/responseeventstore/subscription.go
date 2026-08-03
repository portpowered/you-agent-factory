package responseeventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	events "github.com/portpowered/infinite-you/pkg/services/events"
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
	if s.completed && !s.storeNowLocked().Before(s.completedAt.Add(s.completedRetentionWindowLocked())) {
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

func (s *SessionResponseEventStore) completedRetentionWindowLocked() time.Duration {
	if s == nil {
		return CompletedStreamRetentionWindow
	}
	if s.limits.CompletedRetentionWindow > 0 {
		return s.limits.CompletedRetentionWindow
	}
	return CompletedStreamRetentionWindow
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
			result.events = s.store.substituteFromEvents(result.events)
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
				result.events = s.store.substituteFromEvents(result.events)
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
		result.events = s.store.substituteFromEvents(result.events)
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

// DecodeFactoryResponseEvent is the single authoritative way to interpret a
// Factory Session response event carried by an Events record on a session's
// response-event topic (see responseEventTopic in the owning response_stream
// service). events.Service.Append is one round trip: the publisher must
// marshal a self-describing payload before Append assigns the record's real
// aggregate position, so it can only ever predict that position beforehand
// (see ResponseStream.Publish's sequenceHint) -- and an already-accepted
// payload can never be rewritten afterward to correct a stale prediction on
// a non-aligned topic (one whose Events aggregate positions and this store's
// own session-local sequence numbering diverge, for example a second store
// instance bound to a session topic that already carries history from a
// prior instance). This function resolves that one-round-trip constraint
// explicitly rather than masking it at any one caller's read boundary: it
// never trusts the payload's own embedded Sequence field, and instead always
// derives Sequence from the Events record's own assigned aggregate Position.
// Every reader of this topic -- this store's own compatibility surface, and
// any future direct Events consumer of the same topic -- must decode through
// this function so they always observe one consistent identity for the same
// accepted record, instead of each independently reconciling (or failing to
// reconcile) the payload's stale predicted value.
func DecodeFactoryResponseEvent(record events.Record) (responseevents.FactoryResponseEvent, error) {
	var decoded responseevents.FactoryResponseEvent
	if err := json.Unmarshal(record.Payload, &decoded); err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}
	decoded.Sequence = int64(record.ID.Position)
	return decoded, nil
}

// substituteFromEvents replaces each real (non-gap) delivered event's
// content with the matching record read back from the injected Events root,
// so a subscriber's delivered content is genuinely sourced from Events
// rather than only from this store's own retained copy. It is a no-op when
// no Events root is bound (see NewSessionResponseEventStoreWithEventsAuthority).
//
// When bound, this store's own retained copy still decides *which* sequences
// remain deliverable (tiered importance retention, dispatch filtering, gap
// bookkeeping -- Events has no equivalent for any of these). For a sequence
// Events can currently produce, its content always wins over this store's
// own retained bytes, proving delivery is genuinely read back from Events
// (see TestSessionResponseEventStoreSubscription_DeliversContentReadBackFromEventsAuthority).
// For a sequence Events cannot currently produce, this method distinguishes
// two causes:
//
//   - Events' own bounded FIFO retention window has evicted the position
//     (a confirmed events.ReadOutcomeGap for it) even though this store's
//     own tiered policy still retains it. This is expected, not a
//     divergence: PublishThroughAuthority already proved Events accepted
//     this exact content at commit time, before this store's own tiered
//     eviction and Events' own FIFO eviction inevitably diverge on *which*
//     older records they each keep. Falling back to this store's own
//     retained copy here is what preserves the compatibility surface's
//     existing tiered retention/gap behavior instead of manufacturing a new
//     gap cause for a record the compatibility policy still promises.
//   - Any other reason (a failed Read, a non-Progress/non-Gap outcome, or a
//     payload Events returned but this store could not decode) is a real
//     operational failure, not an expected retention-policy difference: it
//     is replaced with the same synthetic stream-gap marker this store
//     already publishes for its own retention gaps
//     (eventsAuthorityGapEvent), never with this store's local bytes, so a
//     broken or misbehaving Events integration cannot be masked by silently
//     falling back to local content.
//
// This read always runs against context.Background(), independent of any
// caller-supplied context: by the time delivered is non-empty, the local
// store has already decided these sequences are deliverable, and a caller's
// cancellation racing with that decision must not turn real retained content
// into a false authority gap (a canceled context is guaranteed to make
// events.Service.Read fail).
func (s *SessionResponseEventStore) substituteFromEvents(delivered []responseevents.FactoryResponseEvent) []responseevents.FactoryResponseEvent {
	if s == nil || s.eventsService == nil || len(delivered) == 0 {
		return delivered
	}

	var minSeq, maxSeq int64
	realCount := 0
	for _, event := range delivered {
		if event.Sequence <= 0 {
			continue
		}
		if realCount == 0 || event.Sequence < minSeq {
			minSeq = event.Sequence
		}
		if event.Sequence > maxSeq {
			maxSeq = event.Sequence
		}
		realCount++
	}
	if realCount == 0 {
		return delivered
	}

	byPosition, hardFailure := s.readEventsAuthorityRange(context.Background(), minSeq, maxSeq)

	substituted := make([]responseevents.FactoryResponseEvent, len(delivered))
	for i, event := range delivered {
		if event.Sequence <= 0 {
			substituted[i] = event
			continue
		}
		record, ok := byPosition[event.Sequence]
		if !ok {
			if hardFailure {
				substituted[i] = s.eventsAuthorityGapEvent(event.Sequence)
			} else {
				// Events' own bounded retention has evicted this position,
				// but this store's tiered policy still retains it: adopt
				// the store's own copy instead of a synthetic gap (see the
				// doc comment above).
				substituted[i] = event
			}
			continue
		}
		fromEvents, err := DecodeFactoryResponseEvent(record)
		if err != nil {
			substituted[i] = s.eventsAuthorityGapEvent(event.Sequence)
			continue
		}
		substituted[i] = fromEvents
	}
	return substituted
}

// readEventsAuthorityRange reads every Events-authority position in
// [minSeq, maxSeq] it can currently produce, resuming past any evicted
// prefix instead of treating one evicted position as unavailability for the
// entire requested range. events.Service.Read's Gap outcome is all-or-nothing
// for the exact call it was made on (it reports Gap for the whole requested
// range whenever the starting cursor itself is behind the earliest retained
// position, even when later positions in that range remain retained), so a
// single Read spanning an evicted low position would otherwise mark every
// still-available newer position as unavailable too. This loop instead
// re-issues Read starting exactly from one position before the gap's own
// EarliestRetained (From is exclusive: it reports records strictly after the
// cursor), so the position named by EarliestRetained itself is recovered
// along with everything after it -- events.Service.Read's own boundary
// contract (established by story 002) treats a From naming exactly
// EarliestRetained-1 as a valid, non-gap cursor whose first returned record
// is EarliestRetained, so only the positions Events has genuinely evicted
// end up missing from the returned map.
//
// The returned hardFailure reports whether the loop stopped because of a
// real operational failure (a Read error, or an outcome other than Progress
// or Gap) rather than because it fully resolved the requested range through
// zero or more expected Gap recoveries: substituteFromEvents uses this to
// tell "Events genuinely evicted this position from its own bounded window"
// (not a hard failure -- every position in range was accounted for) apart
// from "Events could not be read at all" (a hard failure), since only the
// latter must never be masked by falling back to local content.
func (s *SessionResponseEventStore) readEventsAuthorityRange(ctx context.Context, minSeq, maxSeq int64) (byPosition map[int64]events.Record, hardFailure bool) {
	byPosition = make(map[int64]events.Record, maxSeq-minSeq+1)
	from := events.AggregateSequence(minSeq - 1)
	for int64(from) < maxSeq {
		limit := int(maxSeq) - int(from)
		result, err := s.eventsService.Read(ctx, events.ReadRequest{
			Topic: s.eventsTopic,
			From:  events.Cursor{Topic: s.eventsTopic, Position: from},
			Limit: limit,
		})
		if err != nil {
			return byPosition, true
		}
		switch result.Outcome {
		case events.ReadOutcomeProgress:
			for _, record := range result.Records {
				position := int64(record.ID.Position)
				if position > maxSeq {
					continue
				}
				byPosition[position] = record
			}
			if result.Next.Position <= from {
				return byPosition, false
			}
			from = result.Next.Position
		case events.ReadOutcomeGap:
			next := events.AggregateSequence(result.Gap.EarliestRetained) - 1
			if next <= from {
				return byPosition, false
			}
			from = next
		default:
			return byPosition, true
		}
	}
	return byPosition, false
}

// eventsAuthorityGapEventReason marks a synthetic stream-gap event produced
// because the injected Events root could not currently produce content for a
// sequence this store's own tiered retention still considers deliverable --
// distinct from retentionGapReason, which marks a gap this store's own
// retention policy created directly, so the two causes remain
// distinguishable in the published gap payload.
const eventsAuthorityGapEventReason = "events_authority_unavailable"

// eventsAuthorityGapEvent synthesizes the same published stream-gap
// vocabulary retentionGapEventLocked uses for this store's own retention
// gaps, covering exactly one sequence the Events root could not produce
// content for. It does not require s.mu: it reads only s.factorySessionID
// (immutable after construction) and s.clock (safe for concurrent use), and
// writes nothing.
func (s *SessionResponseEventStore) eventsAuthorityGapEvent(sequence int64) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.StreamGapPayload{
		FromSequence:           sequence,
		ToSequence:             sequence,
		FirstAvailableSequence: sequence + 1,
		Reason:                 eventsAuthorityGapEventReason,
	})
	identity := fmt.Sprintf("%s:%d:%d:%s", s.factorySessionID, sequence, sequence, eventsAuthorityGapEventReason)
	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String(),
		Sequence:         0,
		RecordedAt:       s.clock.Now().UTC(),
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
