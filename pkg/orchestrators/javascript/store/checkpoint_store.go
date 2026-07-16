package store

import (
	"sort"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// CheckpointStore keeps orchestrator-owned checkpoint bundles for one JavaScript
// workflow session.
type CheckpointStore struct {
	mu      sync.RWMutex
	records map[string]interfaces.JavaScriptCheckpointRecord
}

// NewCheckpointStore allocates an empty checkpoint store.
func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{
		records: make(map[string]interfaces.JavaScriptCheckpointRecord),
	}
}

// Put stores or replaces one checkpoint record for the session.
func (s *CheckpointStore) Put(record interfaces.JavaScriptCheckpointRecord) {
	if s == nil || strings.TrimSpace(record.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]interfaces.JavaScriptCheckpointRecord)
	}
	s.records[record.ID] = record
}

// List returns checkpoint records in stable id order.
func (s *CheckpointStore) List() []interfaces.JavaScriptCheckpointRecord {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.records))
	for id := range s.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	records := make([]interfaces.JavaScriptCheckpointRecord, 0, len(ids))
	for _, id := range ids {
		records = append(records, s.records[id])
	}
	return records
}

// Get returns one checkpoint record when present.
func (s *CheckpointStore) Get(id string) (interfaces.JavaScriptCheckpointRecord, bool) {
	if s == nil {
		return interfaces.JavaScriptCheckpointRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	return record, ok
}
