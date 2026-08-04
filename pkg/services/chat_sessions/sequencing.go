package chatsessions

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// EventsTopic names the Events topic Sequence commits SessionID's aggregate
// records to: chat-session/<session-id>/events. Any caller reading or
// subscribing to a Chat Session's sequenced record stream (including a later
// attachment-delivery or retained-read consumer) derives the same topic
// identity through this one function rather than hand-formatting the string.
func EventsTopic(sessionID string) events.Topic {
	return events.Topic("chat-session/" + sessionID + "/events")
}

// SequenceOutcome distinguishes a newly committed aggregate record from a
// duplicate resolved to its originally assigned identity.
type SequenceOutcome int

const (
	SequenceOutcomeUnspecified SequenceOutcome = iota
	// SequenceOutcomeAccepted reports that the record was newly committed and
	// assigned a new ItemID and aggregate position.
	SequenceOutcomeAccepted
	// SequenceOutcomeDuplicate reports that an equivalent source tuple was
	// already committed; the returned identity and position are the
	// original, not a new one.
	SequenceOutcomeDuplicate
)

// SequenceRequest asks a Chat Session's sequencer to commit one source-native
// record onto EventsTopic(SessionID). SourceType, SourceID, SourceSequence,
// and SourceEventID form the same explicit idempotency identity Events
// itself defines (events.AppendIdentity): repeating that exact tuple on the
// same session returns the original aggregate identity instead of a second
// record. ParentItemID, when non-blank, must already identify an ItemID this
// exact session has sequenced -- a child can never be sequenced before its
// parent, since the sequencer assigns both.
type SequenceRequest struct {
	SessionID      string
	SourceType     events.SourceType
	SourceID       events.SourceID
	SourceSequence events.SourceSequence
	SourceEventID  events.SourceEventID
	SchemaID       events.SchemaID
	Kind           workers.Kind
	// Phase names the source-native workers.Phase this record was produced
	// in. It travels alongside Kind as its own envelope field (not folded
	// into Payload) for the same reason Kind does: a later reader must be
	// able to route the record -- reconstructing the workers.Draft a
	// downstream projector like the ACP transport's mapping.Project expects
	// -- without parsing Payload's source-native shape first.
	Phase workers.Phase
	// ParentItemID names the already-sequenced aggregate item this record is
	// a child of, or is blank for a record with no parent.
	ParentItemID string
	// Payload stays source-native JSON; Sequence does not convert it into a
	// new normalized event taxonomy.
	Payload json.RawMessage
}

// Validate reports whether r is well-formed enough to attempt sequencing:
// every identity field is well-formed, Kind is a declared workers.Kind
// member, and Payload is non-empty, well-formed JSON. Validate does not (and
// cannot) check that ParentItemID names an item already sequenced in this
// session -- that is a stateful check Sequence itself performs.
func (r SequenceRequest) Validate() error {
	if r.SessionID == "" {
		return newValidationError("SequenceRequest", "SessionID", ErrRequiredValue)
	}
	if err := r.SourceType.Validate(); err != nil {
		return err
	}
	if err := r.SourceID.Validate(); err != nil {
		return err
	}
	if err := r.SourceSequence.Validate(); err != nil {
		return err
	}
	if err := r.SourceEventID.Validate(); err != nil {
		return err
	}
	if err := r.SchemaID.Validate(); err != nil {
		return err
	}
	if err := r.Kind.Validate(); err != nil {
		return newValidationError("SequenceRequest", "Kind", ErrUnknownEnumValue)
	}
	if err := r.Phase.Validate(); err != nil {
		return newValidationError("SequenceRequest", "Phase", ErrUnknownEnumValue)
	}
	if len(r.Payload) == 0 {
		return newValidationError("SequenceRequest", "Payload", ErrRequiredValue)
	}
	if !json.Valid(r.Payload) {
		return newValidationError("SequenceRequest", "Payload", ErrMalformedValue)
	}
	return nil
}

// SequencedItem is the complete envelope Sequence assigns and commits as the
// Events record Payload for one aggregate record. Assigning ItemID and
// ParentItemID at sequencing time (rather than at transport projection time)
// is what lets a later reader observe the exact same identity across a
// retained read, a live subscription, and a reconnect, with no
// transport-owned or connection-local identity map.
type SequencedItem struct {
	ItemID       string          `json:"itemId"`
	ParentItemID string          `json:"parentItemId,omitempty"`
	Kind         workers.Kind    `json:"kind"`
	Phase        workers.Phase   `json:"phase"`
	Payload      json.RawMessage `json:"payload"`
}

// Validate reports whether i is an internally consistent, already-assigned
// item envelope.
func (i SequencedItem) Validate() error {
	if i.ItemID == "" {
		return newValidationError("SequencedItem", "ItemID", ErrRequiredValue)
	}
	if err := i.Kind.Validate(); err != nil {
		return newValidationError("SequencedItem", "Kind", ErrUnknownEnumValue)
	}
	if err := i.Phase.Validate(); err != nil {
		return newValidationError("SequencedItem", "Phase", ErrUnknownEnumValue)
	}
	if len(i.Payload) == 0 {
		return newValidationError("SequencedItem", "Payload", ErrRequiredValue)
	}
	if !json.Valid(i.Payload) {
		return newValidationError("SequencedItem", "Payload", ErrMalformedValue)
	}
	return nil
}

// SequenceResult carries the aggregate identity Sequence assigned (or, for a
// duplicate, previously assigned) to one committed record.
type SequenceResult struct {
	SessionID    string
	ItemID       string
	ParentItemID string
	// AggregateSequence is the committed record's position within
	// EventsTopic(SessionID), in commit order.
	AggregateSequence events.AggregateSequence
	Outcome           SequenceOutcome
}
