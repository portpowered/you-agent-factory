package events

import "fmt"

// AttachStartPositionMode names where an AttachSource relationship begins
// consuming its SourceTopic.
type AttachStartPositionMode string

const (
	// AttachStartBeginning starts consuming the source Topic from its
	// earliest retained record.
	AttachStartBeginning AttachStartPositionMode = "BEGINNING"
	// AttachStartLiveHead starts consuming the source Topic from records
	// committed after attachment, with no retained backfill.
	AttachStartLiveHead AttachStartPositionMode = "LIVE_HEAD"
	// AttachStartAt starts consuming the source Topic at the AggregateSequence
	// given by AttachStartPosition.At.
	AttachStartAt AttachStartPositionMode = "AT"
)

// AttachStartPosition is the explicit starting or correlation position an
// AttachSourceRequest begins consuming its SourceTopic from.
type AttachStartPosition struct {
	Mode AttachStartPositionMode
	// At is the source Topic AggregateSequence to start from. It is
	// meaningful only when Mode is AttachStartAt and MUST be positive; it
	// MUST be zero for every other Mode.
	At AggregateSequence
}

var (
	// ErrInvalidStartPositionMode reports an AttachStartPosition.Mode that is
	// none of the published AttachStartPositionMode values.
	ErrInvalidStartPositionMode = fmt.Errorf("events: invalid start position mode")
	// ErrAmbiguousStartPosition reports an AttachStartPosition whose At value
	// is inconsistent with its Mode: missing when Mode is AttachStartAt, or
	// present when Mode is not AttachStartAt.
	ErrAmbiguousStartPosition = fmt.Errorf("events: ambiguous start position")
)

// Validate reports the first deterministic validation failure for pos as a
// *ValidationError, or nil when pos is well-formed.
func (pos AttachStartPosition) Validate() error {
	switch pos.Mode {
	case AttachStartAt:
		if pos.At <= 0 {
			return &ValidationError{Field: "start.at", Err: ErrAmbiguousStartPosition}
		}
	case AttachStartBeginning, AttachStartLiveHead:
		if pos.At != 0 {
			return &ValidationError{Field: "start.at", Err: ErrAmbiguousStartPosition}
		}
	default:
		return &ValidationError{Field: "start.mode", Err: ErrInvalidStartPositionMode}
	}
	return nil
}

var (
	// ErrInvalidSourceTopic reports an empty or unparsable SourceTopic.
	ErrInvalidSourceTopic = fmt.Errorf("events: invalid source topic")
	// ErrSelfAttachment reports an AttachSourceRequest whose DestinationTopic
	// and SourceTopic are identical.
	ErrSelfAttachment = fmt.Errorf("events: a topic cannot attach itself as its own source")
	// ErrIncompatibleAttachment reports an AttachSourceRequest whose
	// DestinationTopic and SourceTopic session kinds cannot be attached. V0
	// permits only a factory-session SourceTopic attached into a
	// chat-session DestinationTopic.
	ErrIncompatibleAttachment = fmt.Errorf("events: incompatible destination and source topic combination")
)

// AttachSourceRequest asks the Service to attach a SourceTopic as an input to
// a DestinationTopic, so a committed destination Record can trace back to the
// source relationship it aggregated from. It carries no service, callback,
// transport, storage, or dependency-container value: SourceType and SourceID
// identify the attached source using the same opaque identity vocabulary
// Append uses.
type AttachSourceRequest struct {
	DestinationTopic TopicID
	SourceTopic      TopicID
	SourceType       SourceType
	SourceID         SourceID
	Start            AttachStartPosition
}

// Validate reports the first deterministic validation failure for req as a
// *ValidationError, or nil when req is well-formed.
func (req AttachSourceRequest) Validate() error {
	destParsed, err := ParseTopic(req.DestinationTopic)
	if err != nil {
		return &ValidationError{Field: "destination_topic", Err: ErrInvalidTopic}
	}

	srcParsed, err := ParseTopic(req.SourceTopic)
	if err != nil {
		return &ValidationError{Field: "source_topic", Err: ErrInvalidSourceTopic}
	}

	if req.DestinationTopic == req.SourceTopic {
		return &ValidationError{Field: "source_topic", Err: ErrSelfAttachment}
	}

	if srcParsed.Kind != SessionKindFactory || destParsed.Kind != SessionKindChat {
		return &ValidationError{Field: "source_topic", Err: ErrIncompatibleAttachment}
	}

	switch {
	case req.SourceType == "":
		return &ValidationError{Field: "source_type", Err: ErrInvalidSourceType}
	case req.SourceID == "":
		return &ValidationError{Field: "source_id", Err: ErrInvalidSourceID}
	}

	return req.Start.Validate()
}

// AttachmentID is the stable identity the Service assigns to one attached
// (DestinationTopic, SourceTopic, SourceType, SourceID) relationship.
type AttachmentID string

// AttachOutcome classifies the result of one AttachSource call.
type AttachOutcome string

const (
	// AttachOutcomeAttached reports that req described a new attachment
	// relationship, now active.
	AttachOutcomeAttached AttachOutcome = "ATTACHED"
	// AttachOutcomeAlreadyAttached reports that an identical
	// (DestinationTopic, SourceTopic, SourceType, SourceID, Start)
	// relationship was already active; AttachSourceResult.AttachmentID
	// identifies that same attachment rather than creating a second one.
	AttachOutcomeAlreadyAttached AttachOutcome = "ALREADY_ATTACHED"
	// AttachOutcomeConflict reports that (DestinationTopic, SourceTopic,
	// SourceType, SourceID) already has an active attachment with a
	// different Start position; callers do not retry with a differing Start
	// without first resolving the conflict.
	AttachOutcomeConflict AttachOutcome = "CONFLICT"
)

// AttachSourceResult is the detached outcome of one AttachSource call.
type AttachSourceResult struct {
	AttachmentID AttachmentID
	Outcome      AttachOutcome
}
