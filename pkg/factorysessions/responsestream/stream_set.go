package responsestream

import (
	"sort"
	"strings"
	"sync"
)

// StreamSet keeps the dispatch-keyed internal response streams owned by one
// live Factory Session runtime.
type StreamSet struct {
	mu        sync.RWMutex
	streams   map[string]*SessionResponseStream
	newStream func() *SessionResponseStream
}

// NewStreamSet allocates an empty response-stream set using the default stream
// constructor for newly observed dispatch identities.
func NewStreamSet() *StreamSet {
	return NewStreamSetWithFactory(NewSessionResponseStream)
}

// NewStreamSetWithFactory allocates an empty response-stream set using the
// supplied stream constructor for newly observed dispatch identities.
func NewStreamSetWithFactory(newStream func() *SessionResponseStream) *StreamSet {
	if newStream == nil {
		newStream = NewSessionResponseStream
	}
	return &StreamSet{
		streams:   make(map[string]*SessionResponseStream),
		newStream: newStream,
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

	s.mu.RLock()
	stream := s.streams[key]
	s.mu.RUnlock()
	if stream != nil {
		return stream
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if stream = s.streams[key]; stream != nil {
		return stream
	}
	stream = s.newStream()
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.streams)
}

// DispatchIDs reports the currently allocated dispatch identities in sorted
// order for deterministic tests and diagnostics.
func (s *StreamSet) DispatchIDs() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.streams))
	for dispatchID := range s.streams {
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	return ids
}

// CloseDispatch detaches subscribers and removes one dispatch-scoped stream
// from the set.
func (s *StreamSet) CloseDispatch(dispatchID string) bool {
	if s == nil {
		return false
	}
	key := normalizeDispatchID(dispatchID)
	s.mu.Lock()
	stream := s.streams[key]
	delete(s.streams, key)
	s.mu.Unlock()
	if stream == nil {
		return false
	}
	stream.Close()
	return true
}

// Close detaches all dispatch-scoped subscribers and clears the set.
func (s *StreamSet) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
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
