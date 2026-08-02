package events

import "fmt"

// CursorMode names where a Cursor resumes reading its Topic from.
type CursorMode string

const (
	// CursorBeginning resumes from the earliest currently retained Record.
	CursorBeginning CursorMode = "BEGINNING"
	// CursorAt resumes strictly after the AggregateSequence given by
	// Cursor.At.
	CursorAt CursorMode = "AT"
	// CursorLiveHead resumes from the Topic's current live head, with no
	// retained backfill.
	CursorLiveHead CursorMode = "LIVE_HEAD"
)

// Cursor is a resumable position within one Topic's committed Record
// sequence, scoped to the StreamGeneration it was issued against. A Cursor
// carries no meaning outside the exact Topic and StreamGeneration it names:
// ClassifyAgainst reports when a Cursor is foreign to another Topic or stale
// against a different StreamGeneration rather than letting a caller silently
// resume from the wrong place.
type Cursor struct {
	Topic      TopicID
	Generation StreamGeneration
	Mode       CursorMode
	// At is the AggregateSequence to resume after. It is meaningful only
	// when Mode is CursorAt and MUST be positive; it MUST be zero for every
	// other Mode.
	At AggregateSequence
}

var (
	// ErrInvalidCursorMode reports a Cursor.Mode that is none of the
	// published CursorMode values.
	ErrInvalidCursorMode = fmt.Errorf("events: invalid cursor mode")
	// ErrAmbiguousCursorPosition reports a Cursor whose At value is
	// inconsistent with its Mode: missing when Mode is CursorAt, or present
	// when Mode is not CursorAt.
	ErrAmbiguousCursorPosition = fmt.Errorf("events: ambiguous cursor position")
	// ErrInvalidCursorGeneration reports a non-positive Cursor.Generation.
	ErrInvalidCursorGeneration = fmt.Errorf("events: cursor generation must be positive")
)

// Validate reports the first deterministic validation failure for c as a
// *ValidationError, or nil when c is well-formed. Validate checks only c's
// own shape; it never inspects any Topic or StreamGeneration c is read
// against. Use ClassifyAgainst for that comparison.
func (c Cursor) Validate() error {
	if c.Topic == "" {
		return &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}
	if c.Generation <= 0 {
		return &ValidationError{Field: "generation", Err: ErrInvalidCursorGeneration}
	}
	switch c.Mode {
	case CursorAt:
		if c.At <= 0 {
			return &ValidationError{Field: "at", Err: ErrAmbiguousCursorPosition}
		}
	case CursorBeginning, CursorLiveHead:
		if c.At != 0 {
			return &ValidationError{Field: "at", Err: ErrAmbiguousCursorPosition}
		}
	default:
		return &ValidationError{Field: "mode", Err: ErrInvalidCursorMode}
	}
	return nil
}

// CursorStatus classifies c's relationship to a Topic and StreamGeneration
// it is being read or resumed against.
type CursorStatus string

const (
	// CursorStatusValid reports that c names the same Topic and
	// StreamGeneration it is being read against.
	CursorStatusValid CursorStatus = "VALID"
	// CursorStatusForeignTopic reports that c names a different Topic than
	// the one it is being read against.
	CursorStatusForeignTopic CursorStatus = "FOREIGN_TOPIC"
	// CursorStatusStaleGeneration reports that c names the same Topic but a
	// different StreamGeneration, for example because the owning process
	// restarted since c was issued.
	CursorStatusStaleGeneration CursorStatus = "STALE_GENERATION"
)

// ClassifyAgainst reports whether c is valid to resume reading topic at
// generation, or whether it is foreign to another Topic or stale against a
// different StreamGeneration. ClassifyAgainst assumes c already passed
// Validate(); callers MUST validate c's shape first.
func (c Cursor) ClassifyAgainst(topic TopicID, generation StreamGeneration) CursorStatus {
	if c.Topic != topic {
		return CursorStatusForeignTopic
	}
	if c.Generation != generation {
		return CursorStatusStaleGeneration
	}
	return CursorStatusValid
}

var (
	// ErrInvalidReadLimit reports a non-positive ReadRequest.Limit.
	ErrInvalidReadLimit = fmt.Errorf("events: read limit must be positive")
	// ErrCursorForeignTopic reports a ReadRequest whose After.Topic does not
	// match its own Topic.
	ErrCursorForeignTopic = fmt.Errorf("events: cursor topic does not match the requested topic")
	// ErrCursorStaleGeneration reports a ReadRequest whose After.Generation
	// does not match the Topic's current StreamGeneration, for example
	// because the owning process restarted since the cursor was issued.
	ErrCursorStaleGeneration = fmt.Errorf("events: cursor generation is stale for the current stream")
	// ErrTopicNotFound reports a ReadRequest.Topic the Service has no record
	// of.
	ErrTopicNotFound = fmt.Errorf("events: topic not found")
)

// ReadRequest asks the Service for a bounded, ordered read of committed
// Records on Topic strictly after After, returning at most Limit Records.
type ReadRequest struct {
	Topic TopicID
	After Cursor
	Limit int
}

// Validate reports the first deterministic validation failure for req as a
// *ValidationError, or nil when req is well-formed. Validate checks only
// req's own shape, including that After names the same Topic as req; it
// cannot classify a stale StreamGeneration or an unknown Topic, since both
// require the Service's own runtime state.
func (req ReadRequest) Validate() error {
	if req.Topic == "" {
		return &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}
	if err := req.After.Validate(); err != nil {
		return err
	}
	if req.After.Topic != req.Topic {
		return &ValidationError{Field: "after.topic", Err: ErrCursorForeignTopic}
	}
	if req.Limit <= 0 {
		return &ValidationError{Field: "limit", Err: ErrInvalidReadLimit}
	}
	return nil
}

// ReadOutcome classifies one successful ReadResult's completion, truncation,
// or gap state. ReadOutcome never represents silent record loss: a
// ReadOutcomeGap result always carries the exact missing range and a
// resumable position.
type ReadOutcome string

const (
	// ReadOutcomeComplete reports that Records reached the Topic's current
	// live head; Records may be empty when the caller was already caught up.
	ReadOutcomeComplete ReadOutcome = "COMPLETE"
	// ReadOutcomeTruncated reports that Limit was reached before the live
	// head; more committed Records remain to be read from NextCursor.
	ReadOutcomeTruncated ReadOutcome = "TRUNCATED"
	// ReadOutcomeGap reports that bounded retention evicted part of the
	// requested range before it could be delivered. Gap is non-nil and
	// identifies the missing range and a resumable position.
	ReadOutcomeGap ReadOutcome = "GAP"
)

// ReadResult is the detached outcome of one Read call.
type ReadResult struct {
	Records    []Record
	NextCursor Cursor
	Retained   RetainedRange
	Outcome    ReadOutcome
	// Gap is non-nil only when Outcome is ReadOutcomeGap.
	Gap *Gap
}
