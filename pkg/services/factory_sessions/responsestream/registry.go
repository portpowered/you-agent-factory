package responsestream

import (
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Registry owns the transient response-stream sets for all live sessions in
// one Factory Session service. Hosts no longer store response streams in their
// private runtime-state implementations.
type Registry struct {
	mu        sync.Mutex
	sets      map[string]*StreamSet
	newStream func() *SessionResponseStream
	clock     factory.Clock
}

// NewRegistry constructs an empty session response-stream registry.
func NewRegistry(newStream func() *SessionResponseStream, clock factory.Clock) *Registry {
	if newStream == nil || clock == nil {
		return nil
	}
	return &Registry{sets: make(map[string]*StreamSet), newStream: newStream, clock: clock}
}

// Streams returns the lazily allocated stream set for sessionID.
func (r *Registry) Streams(sessionID string) *StreamSet {
	if r == nil {
		return nil
	}
	key := strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if streams := r.sets[key]; streams != nil {
		return streams
	}
	streams := NewStreamSetWithFactory(r.newStream, r.clock)
	r.sets[key] = streams
	return streams
}

// Existing returns an already allocated stream set without creating one.
func (r *Registry) Existing(sessionID string) *StreamSet {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sets[strings.TrimSpace(sessionID)]
}

// Close closes every stream owned by sessionID and retains the closed set so
// late subscribers observe ErrSubscriptionClosed instead of silently opening
// a new stream window.
func (r *Registry) Close(sessionID string) {
	if r == nil {
		return
	}
	key := strings.TrimSpace(sessionID)
	r.mu.Lock()
	streams := r.sets[key]
	r.mu.Unlock()
	if streams != nil {
		streams.Close()
	}
}

// CloseDispatch completes one dispatch stream without allocating a session set.
func (r *Registry) CloseDispatch(sessionID, dispatchID string) bool {
	streams := r.Existing(sessionID)
	return streams != nil && streams.CloseDispatch(dispatchID)
}
