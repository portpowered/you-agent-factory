package responseeventstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
