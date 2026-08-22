package events

import "strings"

// Conservative length ceilings for detached identity tokens. These bound
// pathological inputs; they are not a claim about any storage limit since
// Events has no storage responsibility.
const (
	maxTopicLength         = 256
	maxSourceTypeLength    = 128
	maxSourceIDLength      = 256
	maxSourceEventIDLength = 256
	maxSchemaIDLength      = 128
)

// Topic identifies one session-scoped Events stream, such as
// "factory-session/<id>/response-events" or "chat-session/<id>/events".
// Events does not hard-code these as the only valid topic families: any
// caller-defined identity that satisfies Validate names a valid stream.
//
// The zero value ("") never validates.
type Topic string

// Validate reports whether t is a well-formed topic identity. It rejects the
// empty value and any value containing leading/trailing whitespace, control
// characters, or characters outside the conservative path-segment charset
// (letters, digits, '-', '_', '.', '/', ':').
func (t Topic) Validate() error {
	switch validateIdentityToken(string(t), maxTopicLength) {
	case identityTokenEmpty:
		return ErrEmptyTopic
	case identityTokenMalformed:
		return ErrMalformedTopic
	default:
		return nil
	}
}

// SourceType names the source-native producer family for an appended
// record, such as a worker kind or a chat control source. Events treats
// SourceType as an opaque identity; it does not own or restrict the set of
// valid source types.
//
// The zero value ("") never validates.
type SourceType string

// Validate reports whether s is a well-formed source type identity.
func (s SourceType) Validate() error {
	switch validateIdentityToken(string(s), maxSourceTypeLength) {
	case identityTokenEmpty:
		return ErrEmptySourceType
	case identityTokenMalformed:
		return ErrMalformedSourceType
	default:
		return nil
	}
}

// SourceID names one source instance within a SourceType, such as one
// worker's stable identity.
//
// The zero value ("") never validates.
type SourceID string

// Validate reports whether s is a well-formed source id identity.
func (s SourceID) Validate() error {
	switch validateIdentityToken(string(s), maxSourceIDLength) {
	case identityTokenEmpty:
		return ErrEmptySourceID
	case identityTokenMalformed:
		return ErrMalformedSourceID
	default:
		return nil
	}
}

// SourceSequence is a source-native monotonic sequence number scoped to one
// (Topic, SourceType, SourceID). Together with SourceEventID it forms the
// idempotency identity for an appended record.
//
// The zero value means "unset" and never validates; source sequences begin
// at 1.
type SourceSequence uint64

// Validate reports whether s is a well-formed source sequence.
func (s SourceSequence) Validate() error {
	if s == 0 {
		return ErrInvalidSourceSequence
	}
	return nil
}

// SourceEventID is the source-native unique identifier for one produced
// event, used together with SourceSequence as part of the append idempotency
// identity.
//
// The zero value ("") never validates.
type SourceEventID string

// Validate reports whether s is a well-formed source event id identity.
func (s SourceEventID) Validate() error {
	switch validateIdentityToken(string(s), maxSourceEventIDLength) {
	case identityTokenEmpty:
		return ErrEmptySourceEventID
	case identityTokenMalformed:
		return ErrMalformedSourceEventID
	default:
		return nil
	}
}

// SchemaID identifies the shape of a record's source-native JSON payload,
// such as "worker.output.v1". Events does not interpret the payload against
// this identity; SchemaID is caller-defined metadata that crosses the
// contract unchanged.
//
// The zero value ("") never validates.
type SchemaID string

// Validate reports whether s is a well-formed schema id identity.
func (s SchemaID) Validate() error {
	switch validateIdentityToken(string(s), maxSchemaIDLength) {
	case identityTokenEmpty:
		return ErrEmptySchemaID
	case identityTokenMalformed:
		return ErrMalformedSchemaID
	default:
		return nil
	}
}

// AggregateSequence is the position Events assigns to an accepted record
// within one topic's aggregate ordering, in commit order. The zero value
// means "before the first record": it is valid only as a starting or
// retained-boundary position, never as an already-assigned record's own
// position.
type AggregateSequence uint64

// ValidateAssigned reports whether the sequence is a valid position for an
// already-accepted record. Assigned positions begin at 1; the zero value is
// reserved for "before the first record" and is rejected here.
func (s AggregateSequence) ValidateAssigned() error {
	if s == 0 {
		return ErrInvalidAggregateSequence
	}
	return nil
}

// RecordID names one accepted record within a topic's aggregate ordering. A
// RecordID is only meaningful together with the AggregateSequence Events
// assigned to it at commit time.
//
// The zero value is invalid.
type RecordID struct {
	Topic    Topic
	Position AggregateSequence
}

// Validate reports whether id names a specific, already-accepted record.
func (id RecordID) Validate() error {
	if err := id.Topic.Validate(); err != nil {
		return err
	}
	return id.Position.ValidateAssigned()
}

type identityTokenShape int

const (
	identityTokenValid identityTokenShape = iota
	identityTokenEmpty
	identityTokenMalformed
)

// validateIdentityToken applies the shared conservative shape rules used by
// every detached identity string in this package: non-empty, no
// leading/trailing whitespace, within maxLen, and restricted to a
// path-segment-safe charset.
func validateIdentityToken(s string, maxLen int) identityTokenShape {
	if s == "" {
		return identityTokenEmpty
	}
	if len(s) > maxLen {
		return identityTokenMalformed
	}
	if strings.TrimSpace(s) != s {
		return identityTokenMalformed
	}
	for _, r := range s {
		if !isIdentityTokenRune(r) {
			return identityTokenMalformed
		}
	}
	return identityTokenValid
}

func isIdentityTokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-' || r == '_' || r == '.' || r == '/' || r == ':':
		return true
	default:
		return false
	}
}
