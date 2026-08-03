// Package service is the process-scoped, in-memory implementation of the
// events.Service contract. It has no durable write path: Recordings remains
// the canonical durable Factory Event ledger, and this package introduces no
// store, journal, database, WAL, or replay ledger of its own.
package service

import (
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Store is the concurrency-safe, in-memory implementation of events.Service.
// It grows one topicState per distinct events.Topic on first use; every
// topic-scoped read/write goes through that topic's own mutex so unrelated
// topics never contend with each other. The zero value is not usable;
// construct with New.
type Store struct {
	mu     sync.RWMutex
	topics map[events.Topic]*topicState
	logger logging.Logger
}

// topicState holds one topic's aggregate ordering, retained records, and
// idempotency index. head is the position of the most recently accepted
// record (0 when the topic has never accepted one) and advances only under
// mu, in commit order, independent of retention: retention (added by a later
// slice of this package) may evict entries from records without ever
// resetting or renumbering head. identity maps each accepted append's
// (sourceType, sourceID, sourceSequence, sourceEventID) tuple to its
// originally accepted Record so a repeated append resolves to the same
// Record regardless of whether that position is still retained.
type topicState struct {
	mu       sync.Mutex
	head     events.AggregateSequence
	records  []events.Record
	identity map[events.AppendIdentity]events.Record
}

// New constructs an empty Store. logger is optional and defaults to a no-op
// logger when omitted, matching the repository's optional-logger
// construction convention.
func New(logger ...logging.Logger) *Store {
	var provided logging.Logger
	if len(logger) > 0 {
		provided = logger[0]
	}
	return &Store{
		topics: make(map[events.Topic]*topicState),
		logger: logging.EnsureLogger(provided),
	}
}

// topic returns the topicState for t, creating it on first use. It takes the
// store's read lock first so concurrent operations against already-known
// topics never contend on the store-level lock; the write lock is only taken
// to register a topic this Store has not seen before.
func (st *Store) topic(t events.Topic) *topicState {
	st.mu.RLock()
	ts, ok := st.topics[t]
	st.mu.RUnlock()
	if ok {
		return ts
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	if ts, ok = st.topics[t]; ok {
		return ts
	}
	ts = &topicState{identity: make(map[events.AppendIdentity]events.Record)}
	st.topics[t] = ts
	return ts
}
