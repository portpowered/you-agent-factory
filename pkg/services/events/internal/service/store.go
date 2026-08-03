// Package service is the process-scoped, in-memory implementation of the
// events.Service contract. It has no durable write path: Recordings remains
// the canonical durable Factory Event ledger, and this package introduces no
// store, journal, database, WAL, or replay ledger of its own.
package service

import (
	"maps"
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
	closed              bool
}

var _ events.Service = (*Store)(nil)

// topicState holds one topic's aggregate ordering, retained records,
// idempotency index, live subscriber registrations, and outgoing attachment
// forwards. head is the position of the most recently accepted record (0
// when the topic has never accepted one) and advances only under mu, in
// commit order, independent of retention: eviction may drop entries from the
// front of records without ever resetting or renumbering head. records is
// always contiguous in Position: records[i].ID.Position equals the topic's
// earliest retained position plus i. identity maps each accepted append's
// (sourceType, sourceID, sourceSequence, sourceEventID) tuple to its
// originally accepted Record so a repeated append resolves to the same
// Record regardless of whether that position is still retained. subscribers
// holds every currently live registration keyed by an opaque per-topic id;
// attachments holds every topic currently attached to this one as a
// forwarding destination, keyed by that destination Topic (at most one
// attachment per distinct destination, which is what makes AttachSource
// idempotent per (Destination, Source) pair). Append offers each newly
// committed record to every subscriber and forwards it to every attached
// destination under the same lock that commits it, and closed marks that
// this topic has been shut down (directly via Store.Close, or pre-closed
// because the topic was first created after the Store was already closed).
type topicState struct {
	mu          sync.Mutex
	head        events.AggregateSequence
	records     []events.Record
	identity    map[events.AppendIdentity]events.Record
	subscribers map[uint64]*liveSubscriber
	attachments map[events.Topic]*attachmentForward
	nextSubID   uint64
	closed      bool
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
// to register a topic this Store has not seen before. A topic created after
// Close has already been called starts pre-closed, so a later Subscribe on
// it observes DeliveryClosed instead of registering a live subscriber that
// would never be shut down.
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
	ts = &topicState{
		identity:    make(map[events.AppendIdentity]events.Record),
		subscribers: make(map[uint64]*liveSubscriber),
		attachments: make(map[events.Topic]*attachmentForward),
		closed:      st.closed,
	}
	st.topics[t] = ts
	return ts
}

// Close idempotently shuts down every topic this Store has ever created:
// each active live subscriber observes DeliveryClosed (after draining any
// record already buffered ahead of it) exactly once, and repeated or
// concurrent calls to Close are safe no-ops once the first has taken effect.
// Close performs no durable write; it does not reject later Append or Read
// calls (that policy belongs to the canonical construction/shutdown wiring
// added when Events is injected into the application lifecycle).
func (st *Store) Close() {
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return
	}
	st.closed = true
	topics := make(map[events.Topic]*topicState, len(st.topics))
	maps.Copy(topics, st.topics)
	st.mu.Unlock()

	for topic, ts := range topics {
		ts.closeLocked(st, topic)
	}
	st.logStoreClosed()
}

// closeLocked marks ts closed, terminates every currently registered live
// subscriber with DeliveryClosed exactly once, and tears down every outgoing
// attachment forward registered on ts so no future commit (Close does not
// itself reject later Append calls) is ever forwarded to a topic that
// stopped being observed. Logs the topic-level closure once it has taken
// effect. Safe to call more than once; only the first call has any effect.
func (ts *topicState) closeLocked(st *Store, topic events.Topic) {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return
	}
	ts.closed = true
	subs := ts.subscribers
	ts.subscribers = make(map[uint64]*liveSubscriber)
	attachmentCount := len(ts.attachments)
	ts.attachments = make(map[events.Topic]*attachmentForward)
	ts.mu.Unlock()

	for _, sub := range subs {
		sub.terminate(events.DeliveryClosed)
	}
	st.logSubscribeTopicClosed(topic, len(subs))
	if attachmentCount > 0 {
		st.logAttachTopicClosed(topic, attachmentCount)
	}
}
