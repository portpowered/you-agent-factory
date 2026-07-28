// Package processlocal implements the default process-local CheckpointStore
// adapter for checkpoint_recovery.
package processlocal

import (
	"strings"
	"sync"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

// Store keeps versioned opaque checkpoint envelopes in process memory.
type Store struct {
	mu        sync.RWMutex
	envelopes map[string]checkpointrecovery.Envelope
}

var _ checkpointrecovery.CheckpointStore = (*Store)(nil)

// New allocates an empty process-local checkpoint store.
func New() *Store {
	return &Store{
		envelopes: make(map[string]checkpointrecovery.Envelope),
	}
}

// Put stores or replaces one versioned opaque checkpoint envelope.
func (s *Store) Put(envelope checkpointrecovery.Envelope) error {
	if err := checkpointrecovery.ValidateEnvelope(envelope); err != nil {
		return err
	}
	id := strings.TrimSpace(envelope.CheckpointID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.envelopes == nil {
		s.envelopes = make(map[string]checkpointrecovery.Envelope)
	}
	s.envelopes[id] = cloneEnvelope(envelope, id)
	return nil
}

// Get returns one stored envelope when present and structurally valid.
func (s *Store) Get(checkpointID string) (checkpointrecovery.Envelope, error) {
	id := strings.TrimSpace(checkpointID)
	if id == "" {
		return checkpointrecovery.Envelope{}, checkpointrecovery.ErrCheckpointNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	envelope, ok := s.envelopes[id]
	if !ok {
		return checkpointrecovery.Envelope{}, checkpointrecovery.ErrCheckpointNotFound
	}
	if err := checkpointrecovery.ValidateEnvelope(envelope); err != nil {
		return checkpointrecovery.Envelope{}, err
	}
	return cloneEnvelope(envelope, id), nil
}

func cloneEnvelope(envelope checkpointrecovery.Envelope, checkpointID string) checkpointrecovery.Envelope {
	cloned := envelope
	cloned.CheckpointID = checkpointID
	if len(envelope.Payload) == 0 {
		cloned.Payload = nil
		return cloned
	}
	cloned.Payload = append([]byte(nil), envelope.Payload...)
	return cloned
}
