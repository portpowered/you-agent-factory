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

// defaultMaxRetainedPerTopic bounds retained records per topic when New is
// constructed without an explicit policy.
const defaultMaxRetainedPerTopic = 10_000

// Store is the concurrency-safe, in-memory implementation of events.Service.
// It grows one topicState per distinct events.Topic on first use; every
// topic-scoped read/write goes through that topic's own mutex so unrelated
// topics never contend with each other. The zero value is not usable;
// construct with New or NewWithRetention.
type Store struct {
	mu                  sync.RWMutex
	topics              map[events.Topic]*topicState
	maxRetainedPerTopic int
	logger              logging.Logger
}

// topicState holds one topic's aggregate ordering, retained records, and
// idempotency index. head is the position of the most recently accepted
// record (0 when the topic has never accepted one) and advances only under
// mu, in commit order, independent of retention: eviction may drop entries
// from the front of records without ever resetting or renumbering head.
// records is always contiguous in Position: records[i].ID.Position equals
// the topic's earliest retained position plus i. identity maps each
// accepted append's (sourceType, sourceID, sourceSequence, sourceEventID)
// tuple to its originally accepted Record so a repeated append resolves to
// the same Record regardless of whether that position is still retained.
type topicState struct {
	mu       sync.Mutex
	head     events.AggregateSequence
	records  []events.Record
	identity map[events.AppendIdentity]events.Record
}

// earliestLocked returns the oldest retained position for ts, or 0 when the
// topic has never retained a record. Callers hold ts.mu.
func (ts *topicState) earliestLocked() events.AggregateSequence {
	if len(ts.records) == 0 {
		return 0
	}
	return ts.records[0].ID.Position
}

// New constructs an empty Store using the default bounded-retention policy
// (defaultMaxRetainedPerTopic records per topic). logger is optional and
// defaults to a no-op logger when omitted, matching the repository's
// optional-logger construction convention.
func New(logger ...logging.Logger) *Store {
	return NewWithRetention(defaultMaxRetainedPerTopic, logger...)
}

// NewWithRetention constructs an empty Store bounded to at most
// maxRetainedPerTopic records per topic; a non-positive value falls back to
// defaultMaxRetainedPerTopic. logger is optional and defaults to a no-op
// logger when omitted.
func NewWithRetention(maxRetainedPerTopic int, logger ...logging.Logger) *Store {
	if maxRetainedPerTopic <= 0 {
		maxRetainedPerTopic = defaultMaxRetainedPerTopic
	}
	var provided logging.Logger
	if len(logger) > 0 {
		provided = logger[0]
	}
	return &Store{
		topics:              make(map[events.Topic]*topicState),
		maxRetainedPerTopic: maxRetainedPerTopic,
		logger:              logging.EnsureLogger(provided),
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
