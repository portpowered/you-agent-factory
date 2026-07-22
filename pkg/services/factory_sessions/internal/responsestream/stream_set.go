package responsestream

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// StreamSet keeps the dispatch-keyed internal response streams owned by one
// live Factory Session runtime.
type StreamSet struct {
	mu                         sync.RWMutex
	streams                    map[string]*SessionResponseStream
	closedDispatches           map[string]struct{}
	closed                     bool
	newStream                  func() *SessionResponseStream
	clock                      factory.Clock
	completedDispatchRetention time.Duration
}

// NewStreamSet allocates an empty response-stream set using the default stream
// constructor for newly observed dispatch identities.
func NewStreamSet(clock factory.Clock) *StreamSet {
	return NewStreamSetWithFactory(func() *SessionResponseStream {
		return NewSessionResponseStream(clock)
	}, clock)
}

// NewStreamSetWithFactory allocates an empty response-stream set using the
// supplied stream constructor for newly observed dispatch identities.
func NewStreamSetWithFactory(newStream func() *SessionResponseStream, clock factory.Clock) *StreamSet {
	return NewStreamSetWithFactoryAndRetention(
		newStream,
		DefaultCompletedDispatchRetention(),
		clock,
	)
}

// NewStreamSetWithFactoryAndRetention allocates a response-stream set with
// explicit completed-dispatch retention and eviction clock controls.
func NewStreamSetWithFactoryAndRetention(
	newStream func() *SessionResponseStream,
	completedDispatchRetention time.Duration,
	clock factory.Clock,
) *StreamSet {
	if clock == nil || newStream == nil || completedDispatchRetention <= 0 {
		return nil
	}
	return &StreamSet{
		streams:                    make(map[string]*SessionResponseStream),
		closedDispatches:           make(map[string]struct{}),
		newStream:                  newStream,
		clock:                      clock,
		completedDispatchRetention: completedDispatchRetention,
	}
}

// Stream returns the stream for one dispatch identity, allocating it on first
// use. Empty identities are retained under the empty-string key so providers
// without dispatch metadata still stay scoped to one session-owned stream.
func (s *StreamSet) Stream(dispatchID string) *SessionResponseStream {
	if s == nil {
		return nil
	}
	key := normalizeDispatchID(dispatchID)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredCompletedDispatchesLocked()
	if s.closed {
		return nil
	}
	if stream := s.streams[key]; stream != nil {
		return stream
	}
	if _, dispatchClosed := s.closedDispatches[key]; dispatchClosed {
		return nil
	}
	stream := s.newStream()
	s.streams[key] = stream
	return stream
}

// Subscribe attaches one internal subscriber to the dispatch-scoped stream and
// returns a retained-window cursor that can continue reading live events.
func (s *StreamSet) Subscribe(dispatchID string, afterSequence int64) (*Subscription, error) {
	stream := s.Stream(dispatchID)
	if stream == nil {
		return nil, ErrSubscriptionClosed
	}
	return stream.Subscribe(afterSequence)
}

// Count returns the number of dispatch-scoped streams currently allocated.
func (s *StreamSet) Count() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredCompletedDispatchesLocked()
	return len(s.streams)
}

// DispatchIDs reports the currently allocated dispatch identities in sorted
// order for deterministic tests and diagnostics.
func (s *StreamSet) DispatchIDs() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredCompletedDispatchesLocked()

	ids := make([]string, 0, len(s.streams))
	for dispatchID := range s.streams {
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	return ids
}

// CloseDispatch detaches live subscribers, stops further publication for one
// dispatch, and retains the buffered stream so late consumers can still
// discover and drain ordered progress until the session stream set closes.
func (s *StreamSet) CloseDispatch(dispatchID string) bool {
	if s == nil {
		return false
	}
	key := normalizeDispatchID(dispatchID)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	stream := s.streams[key]
	if stream == nil {
		s.mu.Unlock()
		return false
	}
	s.closedDispatches[key] = struct{}{}
	s.mu.Unlock()
	stream.CompleteDispatch()
	s.evictExpiredCompletedDispatches()
	return true
}

func (s *StreamSet) evictExpiredCompletedDispatches() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredCompletedDispatchesLocked()
}

func (s *StreamSet) evictExpiredCompletedDispatchesLocked() {
	if s == nil || s.closed || len(s.streams) == 0 {
		return
	}
	now := s.clock.Now().UTC()
	for key, stream := range s.streams {
		if _, completed := s.closedDispatches[key]; !completed {
			continue
		}
		stream.EnforceRetention()
		completedAt := stream.DispatchCompletedAt()
		if completedAt.IsZero() || now.Sub(completedAt) < s.completedDispatchRetention {
			continue
		}
		stream.Close()
		delete(s.streams, key)
	}
}

// Close detaches all dispatch-scoped subscribers and clears the set.
func (s *StreamSet) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	streams := make([]*SessionResponseStream, 0, len(s.streams))
	for _, stream := range s.streams {
		streams = append(streams, stream)
	}
	s.streams = make(map[string]*SessionResponseStream)
	s.mu.Unlock()
	for _, stream := range streams {
		stream.Close()
	}
}

// SubscriberCount reports the number of active subscribers for one dispatch.
func (s *StreamSet) SubscriberCount(dispatchID string) int {
	if s == nil {
		return 0
	}
	key := normalizeDispatchID(dispatchID)
	s.mu.RLock()
	stream := s.streams[key]
	s.mu.RUnlock()
	if stream == nil {
		return 0
	}
	return stream.SubscriberCount()
}

func normalizeDispatchID(dispatchID string) string {
	return strings.TrimSpace(dispatchID)
}
