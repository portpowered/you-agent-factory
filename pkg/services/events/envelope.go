package events

import "fmt"

// TopicID identifies one session-scoped Events stream destination, such as
// factory-session/<id>/response-events or chat-session/<id>/events. This
// contract-only iteration validates only that a TopicID is non-empty; exact
// topic construction, parsing, and session-scope rules belong to the session
// topic and source-attachment contract published on this same root.
type TopicID string

// SourceType names the producer vocabulary a delivered envelope originates
// from (for example, a Factory Session dispatch stream or a Chat Session
// aggregator). Events treats SourceType as an opaque identity component; it
// does not define or own a source-type taxonomy.
type SourceType string

// SourceID identifies one producer instance within a SourceType.
type SourceID string

// SourceSequence is the source-assigned position of one envelope within its
// own producer stream. It MUST be positive. Events uses SourceSequence only
// as one component of the exact append idempotency tuple; it never reorders
// or reinterprets source delivery from it.
type SourceSequence int64

// SourceEventID is the source-assigned stable identity for one envelope. It
// MUST be non-empty.
type SourceEventID string

// SchemaID identifies a payload's source-owned schema. Events validates only
// that a SchemaID is present; it does not interpret, version, or own payload
// schemas itself.
type SchemaID string

// AggregateSequence is the position Events assigns to a committed Record
// within its Topic, strictly increasing in commit order. AggregateSequence is
// never supplied by a caller and is never derived from a timestamp: two
// envelopes appended out of source-timestamp order still receive
// AggregateSequence values in the order Append committed them.
type AggregateSequence int64

// StreamGeneration identifies one process lifetime of the Events stream. It
// changes only when the owning process restarts. Events keeps no state
// across a StreamGeneration boundary, and a cursor or subscription carrying a
// foreign StreamGeneration is never silently accepted.
type StreamGeneration int64

// Payload is the opaque, source-native content of one delivered envelope.
// Append takes ownership of the Payload a caller supplies: once appended,
// the caller MUST NOT mutate it, and Events makes no independent defensive
// copy. Events never inspects, transforms, or reinterprets Payload; it is
// opaque source-native data, not an Events-owned event kind.
type Payload any

// IdempotencyKey is the exact tuple Events uses to detect duplicate source
// delivery: (SourceType, SourceID, SourceSequence, SourceEventID). Two
// AppendRequest values carrying an identical IdempotencyKey MUST resolve to
// the same committed Record identity rather than a second Record.
type IdempotencyKey struct {
	SourceType     SourceType
	SourceID       SourceID
	SourceSequence SourceSequence
	SourceEventID  SourceEventID
}

// Record is one committed, validated delivery envelope: a source-native
// Payload plus the identity and ordering metadata Events assigns and
// preserves. Record carries no Events-owned event-kind classification.
type Record struct {
	Topic             TopicID
	SourceType        SourceType
	SourceID          SourceID
	SourceSequence    SourceSequence
	SourceEventID     SourceEventID
	Schema            SchemaID
	AggregateSequence AggregateSequence
	Generation        StreamGeneration
	Payload           Payload
}

// Key returns the exact append idempotency tuple carried by r.
func (r Record) Key() IdempotencyKey {
	return IdempotencyKey{
		SourceType:     r.SourceType,
		SourceID:       r.SourceID,
		SourceSequence: r.SourceSequence,
		SourceEventID:  r.SourceEventID,
	}
}

var (
	// ErrInvalidTopic reports an empty destination TopicID.
	ErrInvalidTopic = fmt.Errorf("events: invalid topic")
	// ErrInvalidSourceType reports an empty SourceType.
	ErrInvalidSourceType = fmt.Errorf("events: invalid source type")
	// ErrInvalidSourceID reports an empty SourceID.
	ErrInvalidSourceID = fmt.Errorf("events: invalid source id")
	// ErrInvalidSourceSequence reports a non-positive SourceSequence.
	ErrInvalidSourceSequence = fmt.Errorf("events: source sequence must be positive")
	// ErrInvalidSourceEventID reports an empty SourceEventID.
	ErrInvalidSourceEventID = fmt.Errorf("events: invalid source event id")
	// ErrInvalidSchemaID reports an empty SchemaID.
	ErrInvalidSchemaID = fmt.Errorf("events: invalid schema id")
)

// ValidationError reports one deterministic, typed envelope validation
// failure. Callers distinguish the failed field with errors.Is against the
// package Err* sentinels rather than parsing Error() text.
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("events: invalid %s: %v", e.Field, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
