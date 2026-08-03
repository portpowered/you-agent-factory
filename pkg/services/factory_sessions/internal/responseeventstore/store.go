package responseeventstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
)

// SessionResponseEventStore retains immutable FactoryResponseEvent records for
// one live Factory Session runtime with session-monotonic sequencing.
type SessionResponseEventStore struct {
	mu               sync.RWMutex
	clock            factory.Clock
	generateEventID  ResponseEventIDGenerator
	factorySessionID string
	nextSequence     int64
	events           []responseevents.FactoryResponseEvent
	eventSizes       []int
	droppedSequences []sequenceSpan
	limits           RetentionLimits
	retainedBytes    int
	closed           bool
	completed        bool
	completedAt      time.Time
	nextSubID        int64
	subscribers      map[int64]*storeSubscriber

	// eventsService and eventsTopic are optional: when set (via
	// NewSessionResponseEventStoreWithEventsAuthority), Subscribe/Next/Drain
	// source each delivered record's content from this injected Events root
	// instead of trusting the locally retained copy, so the compatibility
	// surface's read path is genuinely backed by Events rather than merely
	// sharing write-order identity with it. The local retained copy in
	// events/eventSizes/droppedSequences is still authoritative for which
	// sequences the tiered retention policy still considers live (Events has
	// no equivalent importance-tiered eviction), but it is never used as a
	// fallback source of content: when Events cannot currently produce a
	// sequence's bytes (a failed/non-Progress Read, or a position Events'
	// own bounded window has already evicted even though this store's
	// tiered policy still retains it), substituteFromEvents (subscription.go)
	// replaces that record with a synthetic stream-gap marker instead of
	// this store's own bytes, so the compatibility surface can never emit
	// content a direct Events reader could no longer observe.
	eventsService events.Service
	eventsTopic   events.Topic
}

// ResponseEventIDGenerator supplies opaque identities for canonical Factory
// Session response events.
type ResponseEventIDGenerator = factorysessions.ResponseEventIDGenerator

// NewSessionResponseEventStore allocates an empty store for one session runtime
// using the explicitly supplied process clock.
func NewSessionResponseEventStore(factorySessionID string, clock factory.Clock, generateEventID ResponseEventIDGenerator) *SessionResponseEventStore {
	store, _ := NewSessionResponseEventStoreWithClockAndLimits(
		factorySessionID,
		clock,
		DefaultRetentionLimits(),
		generateEventID,
	)
	return store
}

// NewSessionResponseEventStoreWithClock allocates an empty store using the
// supplied clock.
func NewSessionResponseEventStoreWithClock(factorySessionID string, clock factory.Clock, generateEventID ResponseEventIDGenerator) *SessionResponseEventStore {
	store, _ := NewSessionResponseEventStoreWithClockAndLimits(
		factorySessionID,
		clock,
		DefaultRetentionLimits(),
		generateEventID,
	)
	return store
}

// NewSessionResponseEventStoreWithLimits allocates an empty store with an
// explicit clock and positive hard retention limits.
func NewSessionResponseEventStoreWithLimits(
	factorySessionID string,
	clock factory.Clock,
	limits RetentionLimits,
	generateEventID ResponseEventIDGenerator,
) (*SessionResponseEventStore, error) {
	return NewSessionResponseEventStoreWithClockAndLimits(factorySessionID, clock, limits, generateEventID)
}

// NewSessionResponseEventStoreWithClockAndLimits allocates an empty store with
// an explicit clock and positive hard retention limits.
func NewSessionResponseEventStoreWithClockAndLimits(
	factorySessionID string,
	clock factory.Clock,
	limits RetentionLimits,
	generateEventID ResponseEventIDGenerator,
) (*SessionResponseEventStore, error) {
	if clock == nil {
		return nil, errors.New("clock is required")
	}
	if err := validateRetentionLimits(limits); err != nil {
		return nil, err
	}
	if generateEventID == nil {
		return nil, errors.New("response event ID generator is required")
	}
	return &SessionResponseEventStore{
		clock:            clock,
		generateEventID:  generateEventID,
		factorySessionID: strings.TrimSpace(factorySessionID),
		limits:           limits,
		subscribers:      make(map[int64]*storeSubscriber),
	}, nil
}

// NewSessionResponseEventStoreWithEventsAuthority allocates an empty store
// exactly like NewSessionResponseEventStoreWithClockAndLimits, additionally
// binding the injected Events root and the topic this session's response
// events are published to. Once bound, Subscribe/Next/Drain fetch each
// delivered record's content from eventsService rather than only trusting
// the store's own retained copy, so the compatibility surface's read path is
// genuinely backed by the same Events root Publish writes through.
func NewSessionResponseEventStoreWithEventsAuthority(
	factorySessionID string,
	clock factory.Clock,
	limits RetentionLimits,
	generateEventID ResponseEventIDGenerator,
	eventsService events.Service,
	eventsTopic events.Topic,
) (*SessionResponseEventStore, error) {
	if eventsService == nil {
		return nil, errors.New("Events root is required")
	}
	if err := eventsTopic.Validate(); err != nil {
		return nil, fmt.Errorf("Events topic: %w", err)
	}
	store, err := NewSessionResponseEventStoreWithClockAndLimits(factorySessionID, clock, limits, generateEventID)
	if err != nil {
		return nil, err
	}
	store.eventsService = eventsService
	store.eventsTopic = eventsTopic
	return store, nil
}

// RetentionLimits returns the active session-wide hard limits.
func (s *SessionResponseEventStore) RetentionLimits() RetentionLimits {
	if s == nil {
		return RetentionLimits{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limits
}

// SetRetentionLimits applies positive hard limits and immediately evicts until
// both are satisfied. Invalid limits leave the current policy unchanged.
func (s *SessionResponseEventStore) SetRetentionLimits(limits RetentionLimits) error {
	if s == nil {
		return errNilStore
	}
	if err := validateRetentionLimits(limits); err != nil {
		return err
	}
	s.mu.Lock()
	s.limits = limits
	s.enforceRetentionLocked()
	subscribers := s.subscribersSnapshotLocked()
	s.mu.Unlock()
	notifyStoreSubscribers(subscribers)
	return nil
}

// RetentionAccounting returns exact counters for the immutable retained events.
func (s *SessionResponseEventStore) RetentionAccounting() RetentionAccounting {
	if s == nil {
		return RetentionAccounting{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return RetentionAccounting{EventCount: len(s.events), TotalBytes: s.retainedBytes}
}

// FactorySessionID returns the session identity bound to this store.
func (s *SessionResponseEventStore) FactorySessionID() string {
	if s == nil {
		return ""
	}
	return s.factorySessionID
}

// LatestSequence returns the highest assigned sequence, or zero when empty.
func (s *SessionResponseEventStore) LatestSequence() int64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextSequence
}

// Events returns a snapshot of retained events in ascending sequence order.
func (s *SessionResponseEventStore) Events() []responseevents.FactoryResponseEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.events) == 0 {
		return nil
	}
	out := make([]responseevents.FactoryResponseEvent, len(s.events))
	for index, event := range s.events {
		out[index] = cloneEvent(event)
	}
	return out
}

// EventAtSequence returns the retained event for one sequence when present.
func (s *SessionResponseEventStore) EventAtSequence(sequence int64) (responseevents.FactoryResponseEvent, bool) {
	if s == nil || sequence <= 0 {
		return responseevents.FactoryResponseEvent{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, event := range s.events {
		if event.Sequence == sequence {
			return cloneEvent(event), true
		}
	}
	return responseevents.FactoryResponseEvent{}, false
}

// Completed reports whether publication has finished while retained events
// remain available for catch-up subscribers.
func (s *SessionResponseEventStore) Completed() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completed
}

// CompletedAt returns when publication completed, or zero when still live.
func (s *SessionResponseEventStore) CompletedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completedAt
}

// Publish validates input, assigns the next session-monotonic sequence and a
// stable event ID, and appends an immutable copy to the retained buffer.
func (s *SessionResponseEventStore) Publish(input responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	if s == nil {
		return responseevents.FactoryResponseEvent{}, errNilStore
	}

	prepared := s.preparePublishInput(input)
	if prepared.FactorySessionID != s.factorySessionID {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf(
			"%w: got %q, want %q",
			ErrFactorySessionMismatch,
			prepared.FactorySessionID,
			s.factorySessionID,
		)
	}
	if err := responseevents.ValidateEvent(prepared); err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, ErrStoreClosed
	}
	if s.completed {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, ErrStoreCompleted
	}
	stored, err := s.assignIdentityLocked(prepared)
	if err != nil {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, err
	}
	storedBytes, err := SerializedEventSize(stored)
	if err != nil {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, err
	}
	s.events = append(s.events, stored)
	s.eventSizes = append(s.eventSizes, storedBytes)
	s.retainedBytes += storedBytes
	s.enforceRetentionLocked()
	subscribers := s.subscribersSnapshotLocked()
	s.mu.Unlock()

	notifyStoreSubscribers(subscribers)

	return cloneEvent(stored), nil
}

// PublishThroughAuthority normalizes input exactly as Publish does, then --
// while holding this store's write lock so no concurrent Close, Complete, or
// other PublishThroughAuthority call can interleave -- calls commit with the
// normalized event and the sequence commit must be given to be accepted as
// this store's next record. commit is expected to submit the record to an
// external identity/order authority (the injected Events root) and, only on
// that authority's acceptance, return the exact sequence and event ID it
// assigned; PublishThroughAuthority then retains the identical event under
// that assigned identity before releasing the lock. Because the authority
// call and this store's own retained-append happen inside one critical
// section, the authority's decision and this store's retained state can
// never diverge: the authority can never accept a record this store then
// fails to retain (commit's returned sequence is validated but the retained
// append itself cannot fail for a normalized, already-validated event), and
// this store never retains a record the authority rejected (a commit error
// leaves state completely untouched). A store that is closed or has
// completed publication rejects the call before commit is ever invoked, so
// the authority is never asked to accept a record this store has already
// decided it will not retain.
func (s *SessionResponseEventStore) PublishThroughAuthority(
	input responseevents.FactoryResponseEvent,
	commit func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (sequence int64, eventID string, err error),
) (responseevents.FactoryResponseEvent, error) {
	if s == nil {
		return responseevents.FactoryResponseEvent{}, errNilStore
	}
	if commit == nil {
		return responseevents.FactoryResponseEvent{}, errors.New("publish authority is required")
	}

	prepared := s.preparePublishInput(input)
	if prepared.FactorySessionID != s.factorySessionID {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf(
			"%w: got %q, want %q",
			ErrFactorySessionMismatch,
			prepared.FactorySessionID,
			s.factorySessionID,
		)
	}
	if err := responseevents.ValidateEvent(prepared); err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, ErrStoreClosed
	}
	if s.completed {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, ErrStoreCompleted
	}

	sequenceHint := s.nextSequence + 1
	sequence, eventID, err := commit(prepared, sequenceHint)
	if err != nil {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, err
	}
	if sequence != sequenceHint {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("%w: got %d, want %d", ErrSequenceMismatch, sequence, sequenceHint)
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, errors.New("response event ID is required")
	}

	prepared.Sequence = sequence
	prepared.EventID = eventID
	stored := cloneEvent(prepared)
	storedBytes, err := SerializedEventSize(stored)
	if err != nil {
		s.mu.Unlock()
		return responseevents.FactoryResponseEvent{}, err
	}
	s.nextSequence = sequence
	s.events = append(s.events, stored)
	s.eventSizes = append(s.eventSizes, storedBytes)
	s.retainedBytes += storedBytes
	s.enforceRetentionLocked()
	subscribers := s.subscribersSnapshotLocked()
	s.mu.Unlock()

	notifyStoreSubscribers(subscribers)

	return cloneEvent(stored), nil
}

func (s *SessionResponseEventStore) enforceRetentionLocked() {
	for len(s.events) > s.limits.MaxEvents || s.retainedBytes > s.limits.MaxBytes {
		index := s.evictionIndexLocked()
		if index < 0 {
			return
		}
		s.retainedBytes -= s.eventSizes[index]
		s.droppedSequences = addDroppedSequence(s.droppedSequences, s.events[index].Sequence)
		copy(s.events[index:], s.events[index+1:])
		s.events = s.events[:len(s.events)-1]
		copy(s.eventSizes[index:], s.eventSizes[index+1:])
		s.eventSizes = s.eventSizes[:len(s.eventSizes)-1]
	}
}

func (s *SessionResponseEventStore) evictionIndexLocked() int {
	if len(s.events) == 0 {
		return -1
	}
	lowestTier := eventRetentionTier(s.events[0])
	oldestIndex := 0
	for index := 1; index < len(s.events); index++ {
		tier := eventRetentionTier(s.events[index])
		if tier < lowestTier {
			lowestTier = tier
			oldestIndex = index
		}
	}
	return oldestIndex
}

func (s *SessionResponseEventStore) preparePublishInput(input responseevents.FactoryResponseEvent) responseevents.FactoryResponseEvent {
	event := input
	if strings.TrimSpace(event.SchemaVersion) == "" {
		event.SchemaVersion = responseevents.SchemaVersionV1
	}
	if strings.TrimSpace(event.FactorySessionID) == "" {
		event.FactorySessionID = s.factorySessionID
	}
	if event.RecordedAt.IsZero() {
		event.RecordedAt = s.clock.Now().UTC()
	} else {
		event.RecordedAt = event.RecordedAt.UTC()
	}
	event.Sequence = 0
	event.EventID = ""
	return event
}

func (s *SessionResponseEventStore) assignIdentityLocked(event responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	eventID := strings.TrimSpace(s.generateEventID())
	if eventID == "" {
		return responseevents.FactoryResponseEvent{}, errors.New("response event ID generator returned an empty identity")
	}
	s.nextSequence++
	event.Sequence = s.nextSequence
	event.EventID = eventID
	return cloneEvent(event), nil
}

func cloneEvent(event responseevents.FactoryResponseEvent) responseevents.FactoryResponseEvent {
	cloned := event
	if len(event.Payload) > 0 {
		cloned.Payload = append(json.RawMessage(nil), event.Payload...)
	}
	return cloned
}
