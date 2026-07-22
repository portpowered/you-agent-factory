package responsestream

import (
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// SessionResponseStream keeps ordered internal provider progress for one live
// Factory Session runtime. It is intentionally separate from canonical
// factory event history and from service-coordinator state.
type SessionResponseStream struct {
	mu     sync.RWMutex
	clock  factory.Clock
	limits RetentionLimits

	nextSequence      int64
	events            []Event
	totalBytes        int
	closed            bool
	dispatchCompleted bool
	completedAt       time.Time
	nextSubID         int64
	subscribers       map[int64]*streamSubscriber
}

// NewSessionResponseStream allocates an empty internal response stream with
// documented default retention limits and an explicitly supplied process clock.
func NewSessionResponseStream(clock factory.Clock) *SessionResponseStream {
	return NewSessionResponseStreamWithClock(clock, DefaultRetentionLimits())
}

// NewSessionResponseStreamWithClock allocates an empty stream using the
// supplied clock and retention limits.
func NewSessionResponseStreamWithClock(clock factory.Clock, limits RetentionLimits) *SessionResponseStream {
	if clock == nil {
		return nil
	}
	return &SessionResponseStream{
		clock:       clock,
		limits:      limits,
		subscribers: make(map[int64]*streamSubscriber),
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

// EnforceRetention re-applies bounded retention controls to the retained event
// window without appending new events. Completed dispatch streams rely on this
// path because Append is disabled after dispatch completion.
func (s *SessionResponseStream) EnforceRetention() *CompactionSummary {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.enforceRetentionLocked()
}

// DispatchCompleted reports whether the dispatch has finished publication.
func (s *SessionResponseStream) DispatchCompleted() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dispatchCompleted
}

// DispatchCompletedAt returns when the dispatch completed, or zero when the
// dispatch is still live.
func (s *SessionResponseStream) DispatchCompletedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completedAt
}

// Append records one internal response-stream event, enforces bounded
// retention, and returns the stored envelope with assigned ordering metadata.
// When retention pressure drops retained events, the second return value
// summarizes the compaction for downstream diagnostics.
func (s *SessionResponseStream) Append(event Event) (Event, *CompactionSummary) {
	if s == nil {
		return event, nil
	}
	s.mu.Lock()
	if s.closed || s.dispatchCompleted {
		s.mu.Unlock()
		return event, nil
	}
	stored, compaction := s.appendLocked(event, true)
	subscribers := s.subscribersSnapshotLocked()
	s.mu.Unlock()
	notifySubscribers(subscribers)
	return stored, compaction
}

func (s *SessionResponseStream) appendLocked(event Event, enforceRetention bool) (Event, *CompactionSummary) {
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

	if !enforceRetention {
		return event, nil
	}
	return event, s.enforceRetentionLocked()
}

func (s *SessionResponseStream) enforceRetentionLocked() *CompactionSummary {
	var summary *CompactionSummary

	now := s.clock.Now().UTC()
	for s.limits.MaxAge > 0 && len(s.events) > 0 && now.Sub(s.events[0].RecordedAt) > s.limits.MaxAge {
		summary = mergeCompactionSummary(summary, s.dropFrontLocked(CompactionReasonAgeEvicted))
	}

	for s.limits.MaxEvents > 0 && s.retentionEventCountLocked() > s.limits.MaxEvents {
		if coalesced, droppedSequence := s.tryCoalesceLocked(); coalesced {
			summary = mergeCompactionSummary(summary, &CompactionSummary{
				Reason:                CompactionReasonCoalesced,
				DroppedSequenceCount:  1,
				FirstRetainedSequence: s.firstRetainedSequenceLocked(),
				LastDroppedSequence:   droppedSequence,
			})
			continue
		}
		if dropped := s.dropOldestRetentionEventLocked(CompactionReasonTruncated); dropped == nil {
			break
		} else {
			summary = mergeCompactionSummary(summary, dropped)
		}
	}

	for s.limits.MaxBytes > 0 && s.retentionPayloadBytesLocked() > s.limits.MaxBytes {
		if coalesced, droppedSequence := s.tryCoalesceLocked(); coalesced {
			summary = mergeCompactionSummary(summary, &CompactionSummary{
				Reason:                CompactionReasonCoalesced,
				DroppedSequenceCount:  1,
				FirstRetainedSequence: s.firstRetainedSequenceLocked(),
				LastDroppedSequence:   droppedSequence,
			})
			continue
		}
		if dropped := s.dropOldestRetentionEventLocked(CompactionReasonTruncated); dropped == nil {
			break
		} else {
			summary = mergeCompactionSummary(summary, dropped)
		}
	}

	if summary != nil {
		summary.FirstRetainedSequence = s.firstRetainedSequenceLocked()
	}
	return summary
}

func (s *SessionResponseStream) retentionEventCountLocked() int {
	count := 0
	for _, event := range s.events {
		if event.Kind == EventKindCompactionSignal {
			continue
		}
		count++
	}
	return count
}

func (s *SessionResponseStream) retentionPayloadBytesLocked() int {
	total := 0
	for _, event := range s.events {
		if event.Kind == EventKindCompactionSignal {
			continue
		}
		total += event.PayloadBytes
	}
	return total
}

func (s *SessionResponseStream) dropOldestRetentionEventLocked(reason CompactionReason) *CompactionSummary {
	for i, event := range s.events {
		if event.Kind == EventKindCompactionSignal {
			continue
		}
		return s.dropIndexLocked(i, reason)
	}
	return nil
}

func (s *SessionResponseStream) dropIndexLocked(index int, reason CompactionReason) *CompactionSummary {
	if index < 0 || index >= len(s.events) {
		return nil
	}
	dropped := s.events[index]
	s.events = append(s.events[:index], s.events[index+1:]...)
	s.totalBytes -= dropped.PayloadBytes
	return &CompactionSummary{
		Reason:                reason,
		DroppedSequenceCount:  1,
		FirstRetainedSequence: s.firstRetainedSequenceLocked(),
		LastDroppedSequence:   dropped.Sequence,
	}
}

func (s *SessionResponseStream) dropFrontLocked(reason CompactionReason) *CompactionSummary {
	if len(s.events) == 0 {
		return nil
	}
	dropped := s.events[0]
	s.events = s.events[1:]
	s.totalBytes -= dropped.PayloadBytes
	return &CompactionSummary{
		Reason:                reason,
		DroppedSequenceCount:  1,
		FirstRetainedSequence: s.firstRetainedSequenceLocked(),
		LastDroppedSequence:   dropped.Sequence,
	}
}

func (s *SessionResponseStream) tryCoalesceLocked() (bool, int64) {
	for i := 0; i < len(s.events)-1; i++ {
		left := &s.events[i]
		right := s.events[i+1]
		if !canCoalesceEvents(*left, right) {
			continue
		}
		droppedSequence := right.Sequence
		left.Payload += right.Payload
		left.PayloadBytes = len([]byte(left.Payload))
		s.totalBytes += left.PayloadBytes - right.PayloadBytes
		s.events = append(s.events[:i+1], s.events[i+2:]...)
		return true, droppedSequence
	}
	return false, 0
}

func (s *SessionResponseStream) firstRetainedSequenceLocked() int64 {
	for _, event := range s.events {
		if event.Kind == EventKindCompactionSignal {
			continue
		}
		return event.Sequence
	}
	return s.nextSequence + 1
}

func canCoalesceEvents(left, right Event) bool {
	switch left.Kind {
	case EventKindProgressFragment, EventKindResponseFragment:
	default:
		return false
	}
	if left.Kind != right.Kind || left.DispatchID != right.DispatchID {
		return false
	}
	if left.Type != right.Type || left.ExternalEventType != right.ExternalEventType {
		return false
	}
	if !stringMapEqual(left.Metadata, right.Metadata) {
		return false
	}
	return providerSessionRefEqual(left.ProviderSessionRef, right.ProviderSessionRef)
}

func providerSessionRefEqual(left, right *workerexecution.ProviderSessionMetadata) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.Provider == right.Provider && left.Kind == right.Kind && left.ID == right.ID
}

func stringMapEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}

func mergeCompactionSummary(left, right *CompactionSummary) *CompactionSummary {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	merged := &CompactionSummary{
		Reason:                right.Reason,
		DroppedSequenceCount:  left.DroppedSequenceCount + right.DroppedSequenceCount,
		FirstRetainedSequence: right.FirstRetainedSequence,
		LastDroppedSequence:   right.LastDroppedSequence,
	}
	if merged.FirstRetainedSequence == 0 {
		merged.FirstRetainedSequence = left.FirstRetainedSequence
	}
	if merged.LastDroppedSequence < left.LastDroppedSequence {
		merged.LastDroppedSequence = left.LastDroppedSequence
	}
	return merged
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

// EventsAfter returns retained events with Sequence greater than afterSequence.
// When afterSequence falls before the retained window, BehindRetainedWindow is
// true and Compaction summarizes the dropped prefix so slow consumers can
// recover without blocking producers.
func (s *SessionResponseStream) EventsAfter(afterSequence int64) ReadResult {
	if s == nil {
		return ReadResult{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventsAfterLocked(afterSequence)
}

func (s *SessionResponseStream) eventsAfterLocked(afterSequence int64) ReadResult {
	firstRetained := s.firstRetainedSequenceLocked()
	if len(s.events) == 0 {
		return ReadResult{FirstRetainedSequence: firstRetained}
	}

	if afterSequence > 0 && afterSequence < firstRetained {
		out := make([]Event, len(s.events))
		copy(out, s.events)
		return ReadResult{
			Events:                out,
			BehindRetainedWindow:  true,
			FirstRetainedSequence: firstRetained,
			Compaction: &CompactionSummary{
				Reason:                CompactionReasonTruncated,
				DroppedSequenceCount:  int(firstRetained - afterSequence),
				FirstRetainedSequence: firstRetained,
				LastDroppedSequence:   firstRetained - 1,
			},
		}
	}

	var out []Event
	for _, event := range s.events {
		if event.Sequence > afterSequence {
			out = append(out, event)
		}
	}
	return ReadResult{
		Events:                out,
		FirstRetainedSequence: firstRetained,
	}
}

func (s *SessionResponseStream) subscribersSnapshotLocked() []*streamSubscriber {
	if len(s.subscribers) == 0 {
		return nil
	}
	subscribers := make([]*streamSubscriber, 0, len(s.subscribers))
	for _, subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	return subscribers
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
