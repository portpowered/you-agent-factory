package responsestream

import (
	"sync"

	"github.com/portpowered/infinite-you/pkg/factory"
)

// SessionResponseStream keeps ordered internal provider progress for one live
// Factory Session runtime. It is intentionally separate from canonical
// factory event history and from service-coordinator state.
type SessionResponseStream struct {
	mu     sync.RWMutex
	clock  factory.Clock
	limits RetentionLimits

	nextSequence int64
	events       []Event
	totalBytes   int
}

// NewSessionResponseStream allocates an empty internal response stream with
// documented default retention limits.
func NewSessionResponseStream() *SessionResponseStream {
	return NewSessionResponseStreamWithClock(factory.RealClock{}, DefaultRetentionLimits())
}

// NewSessionResponseStreamWithClock allocates an empty stream using the
// supplied clock and retention limits.
func NewSessionResponseStreamWithClock(clock factory.Clock, limits RetentionLimits) *SessionResponseStream {
	return &SessionResponseStream{
		clock:  factory.EnsureClock(clock),
		limits: limits,
	}
}

// RetentionLimits returns the configured bounded-retention controls.
func (s *SessionResponseStream) RetentionLimits() RetentionLimits {
	if s == nil {
		return RetentionLimits{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.limits
}

// RetentionAccounting summarizes the retained event window for byte-size,
// event-count, and age-based eviction decisions.
func (s *SessionResponseStream) RetentionAccounting() RetentionAccounting {
	if s == nil {
		return RetentionAccounting{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.retentionAccountingLocked()
}

func (s *SessionResponseStream) retentionAccountingLocked() RetentionAccounting {
	accounting := RetentionAccounting{
		EventCount:        len(s.events),
		TotalPayloadBytes: s.totalBytes,
	}
	if len(s.events) > 0 {
		accounting.OldestRecordedAt = s.events[0].RecordedAt
	}
	return accounting
}

// Append records one internal response-stream event and returns the stored
// envelope with assigned ordering and retention-accounting metadata.
func (s *SessionResponseStream) Append(event Event) Event {
	if s == nil {
		return event
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextSequence++
	event.Sequence = s.nextSequence
	if event.RecordedAt.IsZero() {
		event.RecordedAt = s.clock.Now().UTC()
	} else {
		event.RecordedAt = event.RecordedAt.UTC()
	}
	if event.PayloadBytes <= 0 {
		event.PayloadBytes = len([]byte(event.Payload))
	}

	s.events = append(s.events, event)
	s.totalBytes += event.PayloadBytes
	return event
}

// Events returns a snapshot of retained events in ascending Sequence order.
func (s *SessionResponseStream) Events() []Event {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.events) == 0 {
		return nil
	}
	out := make([]Event, len(s.events))
	copy(out, s.events)
	return out
}

// LatestSequence returns the highest assigned stream sequence, or zero when the
// stream is empty.
func (s *SessionResponseStream) LatestSequence() int64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextSequence
}
