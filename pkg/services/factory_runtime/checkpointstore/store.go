// Package checkpointstore implements Factory Runtime's JavaScript checkpoint
// storage contract. The composition root selects this implementation.
package checkpointstore

import (
	"sort"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

var _ factoryruntime.JavaScriptCheckpointStore = (*Store)(nil)

// Store keeps checkpoint records for one JavaScript workflow session.
type Store struct {
	mu      sync.RWMutex
	records map[string]factorydefinitions.JavaScriptCheckpointRecord
}

// New allocates an empty checkpoint store.
func New() *Store {
	return &Store{
		records: make(map[string]factorydefinitions.JavaScriptCheckpointRecord),
	}
}

// Put stores or replaces one checkpoint record for the session.
func (s *Store) Put(record factorydefinitions.JavaScriptCheckpointRecord) {
	if s == nil || strings.TrimSpace(record.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]factorydefinitions.JavaScriptCheckpointRecord)
	}
	s.records[record.ID] = record
}

// List returns checkpoint records in stable ID order.
func (s *Store) List() []factorydefinitions.JavaScriptCheckpointRecord {
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
	records := make([]factorydefinitions.JavaScriptCheckpointRecord, 0, len(ids))
	for _, id := range ids {
		records = append(records, s.records[id])
	}
	return records
}

// Get returns one checkpoint record when present.
func (s *Store) Get(id string) (factorydefinitions.JavaScriptCheckpointRecord, bool) {
	if s == nil {
		return factorydefinitions.JavaScriptCheckpointRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	return record, ok
}
