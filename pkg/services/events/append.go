package events

import "encoding/json"

// AppendOutcome distinguishes a newly accepted append from a duplicate,
// idempotent append. Both outcomes report the same stable Record.ID: a
// caller cannot distinguish the two by identity, only by Outcome.
type AppendOutcome int

const (
	AppendOutcomeUnspecified AppendOutcome = iota
	// AppendOutcomeAccepted reports that the append was newly accepted and
	// assigned a new aggregate position.
	AppendOutcomeAccepted
	// AppendOutcomeDuplicate reports that an equivalent append was already
	// accepted; the returned Record is the original, not a new one.
	AppendOutcomeDuplicate
)

// AppendIdentity is the explicit idempotency identity for one append. A
// duplicate append presenting the same tuple against the same topic
// resolves to the same accepted Record rather than creating a second one.
type AppendIdentity struct {
	SourceType     SourceType
	SourceID       SourceID
	SourceSequence SourceSequence
	SourceEventID  SourceEventID
}

// Validate reports whether id names a well-formed idempotency identity: all
// four tuple members are individually well-formed.
func (id AppendIdentity) Validate() error {
	if err := id.SourceType.Validate(); err != nil {
		return err
	}
	if err := id.SourceID.Validate(); err != nil {
		return err
	}
	if err := id.SourceSequence.Validate(); err != nil {
		return err
	}
	if err := id.SourceEventID.Validate(); err != nil {
		return err
	}
	return nil
}

// AppendRequest asks Events to append one source-native record to Topic.
// Payload stays source-native JSON; Events does not convert it into an
// Events-owned kind union.
type AppendRequest struct {
	Topic          Topic
	SourceType     SourceType
	SourceID       SourceID
	SourceSequence SourceSequence
	SourceEventID  SourceEventID
	SchemaID       SchemaID
	Payload        json.RawMessage
}

// Identity returns the explicit (sourceType, sourceID, sourceSequence,
// sourceEventID) idempotency identity for r.
func (r AppendRequest) Identity() AppendIdentity {
	return AppendIdentity{
		SourceType:     r.SourceType,
		SourceID:       r.SourceID,
		SourceSequence: r.SourceSequence,
		SourceEventID:  r.SourceEventID,
	}
}

// Validate reports whether r names a well-formed append envelope: every
// identity field is well-formed and Payload is non-empty, well-formed JSON.
func (r AppendRequest) Validate() error {
	if err := r.Topic.Validate(); err != nil {
		return err
	}
	if err := r.Identity().Validate(); err != nil {
		return err
	}
	if err := r.SchemaID.Validate(); err != nil {
		return err
	}
	return validatePayload(r.Payload)
}

// Detached returns a copy of r whose Payload is backed by its own array, so
// caller mutation of the original Payload slice after calling Detached
// cannot alter the value observed across the public contract.
func (r AppendRequest) Detached() AppendRequest {
	detached := r
	detached.Payload = clonePayload(r.Payload)
	return detached
}

// Record is one record Events accepted into a topic's aggregate ordering.
// Payload stays source-native JSON.
type Record struct {
	ID             RecordID
	SourceType     SourceType
	SourceID       SourceID
	SourceSequence SourceSequence
	SourceEventID  SourceEventID
	SchemaID       SchemaID
	Payload        json.RawMessage
}

// Identity returns the explicit idempotency identity recorded for rec.
func (rec Record) Identity() AppendIdentity {
	return AppendIdentity{
		SourceType:     rec.SourceType,
		SourceID:       rec.SourceID,
		SourceSequence: rec.SourceSequence,
		SourceEventID:  rec.SourceEventID,
	}
}

// Validate reports whether rec names a well-formed accepted record.
func (rec Record) Validate() error {
	if err := rec.ID.Validate(); err != nil {
		return err
	}
	if err := rec.Identity().Validate(); err != nil {
		return err
	}
	if err := rec.SchemaID.Validate(); err != nil {
		return err
	}
	return validatePayload(rec.Payload)
}

// Detached returns a copy of rec whose Payload is backed by its own array,
// so caller mutation of the original Payload slice after calling Detached
// cannot alter the value observed across the public contract.
func (rec Record) Detached() Record {
	detached := rec
	detached.Payload = clonePayload(rec.Payload)
	return detached
}

// AppendResult is the detached success outcome of one Append call.
type AppendResult struct {
	Record  Record
	Outcome AppendOutcome
}

func validatePayload(payload json.RawMessage) error {
	if len(payload) == 0 {
		return ErrEmptyPayload
	}
	if !json.Valid(payload) {
		return ErrMalformedPayloadJSON
	}
	return nil
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return nil
	}
	clone := make(json.RawMessage, len(payload))
	copy(clone, payload)
	return clone
}
