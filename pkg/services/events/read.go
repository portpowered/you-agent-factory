package events

import (
	"context"
	"fmt"
)

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

// ErrInvalidSubscribeCapacity reports a non-positive SubscribeRequest.Capacity.
var ErrInvalidSubscribeCapacity = fmt.Errorf("events: subscribe capacity must be positive")

// SubscribeRequest asks the Service to open a bounded, ordered live
// Subscription to Topic starting at Start, buffering at most Capacity
// undelivered SubscriptionDelivery observations before the Service ends the
// subscription with SubscriptionTerminalBackpressure rather than blocking
// committed Append progress or buffering unboundedly.
type SubscribeRequest struct {
	Topic    TopicID
	Start    Cursor
	Capacity int
}

// Validate reports the first deterministic validation failure for req as a
// *ValidationError, or nil when req is well-formed. Validate checks only
// req's own shape, including that Start names the same Topic as req; it
// cannot classify a stale StreamGeneration or an unknown Topic, since both
// require the Service's own runtime state and are instead reported through
// the returned Subscription's SubscriptionTerminalInvalidCursor.
func (req SubscribeRequest) Validate() error {
	if req.Topic == "" {
		return &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}
	if err := req.Start.Validate(); err != nil {
		return err
	}
	if req.Start.Topic != req.Topic {
		return &ValidationError{Field: "start.topic", Err: ErrCursorForeignTopic}
	}
	if req.Capacity <= 0 {
		return &ValidationError{Field: "capacity", Err: ErrInvalidSubscribeCapacity}
	}
	return nil
}

// SubscriptionOutcome classifies one ordered observation a Subscription
// delivers: newly committed Records in commit order, or a terminal state
// ending the subscription. SubscriptionOutcome never implies silent record
// loss: a SubscriptionOutcomeTerminal delivery always carries the exact
// reason and the resumable position or missing range needed to recover.
type SubscriptionOutcome string

const (
	// SubscriptionOutcomeRecords reports zero or more newly committed
	// Records delivered in commit order; the subscription remains open.
	SubscriptionOutcomeRecords SubscriptionOutcome = "RECORDS"
	// SubscriptionOutcomeTerminal reports that the subscription has ended;
	// Terminal names the reason and no further delivery follows it.
	SubscriptionOutcomeTerminal SubscriptionOutcome = "TERMINAL"
)

// SubscriptionTerminalReason classifies why a Subscription ended.
type SubscriptionTerminalReason string

const (
	// SubscriptionTerminalCanceled reports that the caller canceled ctx or
	// otherwise closed the subscription.
	SubscriptionTerminalCanceled SubscriptionTerminalReason = "CANCELED"
	// SubscriptionTerminalCompleted reports that Topic reached a defined
	// completion and will deliver no further Records.
	SubscriptionTerminalCompleted SubscriptionTerminalReason = "COMPLETED"
	// SubscriptionTerminalGap reports that bounded in-memory retention
	// evicted part of the requested range before it could be delivered.
	SubscriptionTerminalGap SubscriptionTerminalReason = "GAP"
	// SubscriptionTerminalInvalidCursor reports that Start named a foreign,
	// stale-generation, or otherwise unresumable position — including an
	// unknown Topic — that the Service will not resume from.
	SubscriptionTerminalInvalidCursor SubscriptionTerminalReason = "INVALID_CURSOR"
	// SubscriptionTerminalBackpressure reports that the caller did not drain
	// deliveries within Capacity, so the Service ended the subscription
	// rather than pause committed Append progress or buffer unboundedly.
	SubscriptionTerminalBackpressure SubscriptionTerminalReason = "BACKPRESSURE"
)

// SubscriptionTerminal is the detached terminal state of one ended
// Subscription. Cursor is the last delivered or otherwise resumable
// position a caller can continue from through Read or a new Subscribe. Gap
// is non-nil only when Reason is SubscriptionTerminalGap.
type SubscriptionTerminal struct {
	Reason SubscriptionTerminalReason
	Cursor Cursor
	Gap    *Gap
}

// SubscriptionDelivery is one ordered observation a Subscription produces.
// Records is meaningful only when Outcome is SubscriptionOutcomeRecords;
// Terminal is non-nil only when Outcome is SubscriptionOutcomeTerminal.
type SubscriptionDelivery struct {
	Outcome    SubscriptionOutcome
	Records    []Record
	NextCursor Cursor
	Terminal   *SubscriptionTerminal
}

// Subscription is the implementation-neutral live handle a caller pulls
// ordered SubscriptionDelivery observations from until a terminal outcome
// ends it. Next may block until Records, a terminal outcome, or ctx
// cancellation; it prescribes no channel, buffer, goroutine, retention, or
// transport framing of its own.
type Subscription func(ctx context.Context) SubscriptionDelivery

// Next observes the next ordered SubscriptionDelivery. A nil Subscription
// reports SubscriptionTerminalCanceled rather than panicking.
func (s Subscription) Next(ctx context.Context) SubscriptionDelivery {
	if s == nil {
		return SubscriptionDelivery{
			Outcome:  SubscriptionOutcomeTerminal,
			Terminal: &SubscriptionTerminal{Reason: SubscriptionTerminalCanceled},
		}
	}
	return s(ctx)
}

// SubscribeResult is the detached success outcome of one Subscribe call.
type SubscribeResult struct {
	Subscription Subscription
}
