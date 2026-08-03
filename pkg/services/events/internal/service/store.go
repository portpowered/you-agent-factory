// Package service is the process-scoped, in-memory implementation of the
// events.Service contract. It has no durable write path: Recordings remains
// the canonical durable Factory Event ledger, and this package introduces no
// store, journal, database, WAL, or replay ledger of its own.
package service

import (
	"context"
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

	// attachMu serializes AttachSource calls store-wide so two concurrent
	// requests can never each observe an individually-acyclic graph and
	// register complementary edges that together form a cycle. It guards
	// only attachment-graph structural changes; Append/Read/Subscribe never
	// take it and stay on their existing per-topic ts.mu fast path.
	attachMu sync.Mutex
}

var _ events.Service = (*Store)(nil)

// closer is the exact Close(context.Context) error shutdown role Store
// satisfies for canonical application construction. It is asserted
// structurally rather than published as a second pkg/services/events
// interface, keeping the service root to its one published Service contract.
type closer interface {
	Close(context.Context) error
}

var _ closer = (*Store)(nil)

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
// Record; identity is pruned in lockstep with records eviction (see
// topicState.commitLocked), so idempotency detection is bounded by the same
// retention policy as retained records and does not grow without bound --
// once a position has been evicted, repeating its identity is accepted as a
// new record rather than resolved as a duplicate. subscribers
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
// Close performs no durable write. Once Close has taken effect, every later
// Append or Read call is rejected with events.ErrClosed (checked per-topic,
// so a topic created after Close observes the same rejection as one that
// existed before it); Subscribe and AttachSource keep their own existing
// per-topic-closed answers (an immediately DeliveryClosed subscription, or
// events.ErrOperationFailed for a closed attachment source) unchanged. Close
// gives canonical application construction a Close(context.Context) error
// shutdown role it can fold into the process lifecycle without widening the
// published events.Service contract; it never returns a non-nil error.
func (st *Store) Close(context.Context) error {
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return nil
	}
	st.closed = true
	topics := make(map[events.Topic]*topicState, len(st.topics))
	maps.Copy(topics, st.topics)
	st.mu.Unlock()

	for topic, ts := range topics {
		ts.closeLocked(st, topic)
	}
	st.logStoreClosed()
	return nil
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
