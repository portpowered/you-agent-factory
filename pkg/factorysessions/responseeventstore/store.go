package responseeventstore

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
)

// SessionResponseEventStore retains immutable FactoryResponseEvent records for
// one live Factory Session runtime with session-monotonic sequencing.
type SessionResponseEventStore struct {
	mu               sync.RWMutex
	clock            factory.Clock
	factorySessionID string
	nextSequence     int64
	events           []responseevents.FactoryResponseEvent
	closed           bool
	nextSubID        int64
	subscribers      map[int64]*storeSubscriber
}

// NewSessionResponseEventStore allocates an empty store for one session runtime.
func NewSessionResponseEventStore(factorySessionID string) *SessionResponseEventStore {
	return NewSessionResponseEventStoreWithClock(factorySessionID, factory.RealClock{})
}

// NewSessionResponseEventStoreWithClock allocates an empty store using the
// supplied clock.
func NewSessionResponseEventStoreWithClock(factorySessionID string, clock factory.Clock) *SessionResponseEventStore {
	return &SessionResponseEventStore{
		clock:            factory.EnsureClock(clock),
		factorySessionID: strings.TrimSpace(factorySessionID),
		subscribers:      make(map[int64]*storeSubscriber),
	}
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

// Publish validates input, assigns the next session-monotonic sequence and a
// stable event ID, and appends an immutable copy to the retained buffer.
func (s *SessionResponseEventStore) Publish(input responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	if s == nil {
		return responseevents.FactoryResponseEvent{}, errNilStore
	}

	prepared := s.preparePublishInput(input)
	if err := responseevents.ValidateEvent(prepared); err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}

	s.mu.Lock()
	stored := s.assignIdentityLocked(prepared)
	s.events = append(s.events, stored)
	subscribers := s.subscribersSnapshotLocked()
	s.mu.Unlock()

	notifyStoreSubscribers(subscribers)

	return cloneEvent(stored), nil
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

func (s *SessionResponseEventStore) assignIdentityLocked(event responseevents.FactoryResponseEvent) responseevents.FactoryResponseEvent {
	s.nextSequence++
	event.Sequence = s.nextSequence
	event.EventID = uuid.NewString()
	return cloneEvent(event)
}

func cloneEvent(event responseevents.FactoryResponseEvent) responseevents.FactoryResponseEvent {
	cloned := event
	if len(event.Payload) > 0 {
		cloned.Payload = append(json.RawMessage(nil), event.Payload...)
	}
	return cloned
}
