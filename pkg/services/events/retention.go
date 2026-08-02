package events

import "fmt"

var (
	// ErrInvalidMaxRecords reports a non-positive RetentionLimits.MaxRecords.
	ErrInvalidMaxRecords = fmt.Errorf("events: retention max records must be positive")
	// ErrInvalidMaxBytes reports a non-positive RetentionLimits.MaxBytes.
	ErrInvalidMaxBytes = fmt.Errorf("events: retention max bytes must be positive")
)

// RetentionLimits express the positive bounded in-memory retention window
// Events enforces per Topic. RetentionLimits defines no durable retention
// window, persistence checkpoint, or recovery store: a Record evicted under
// these limits is gone for the remaining lifetime of the owning process.
type RetentionLimits struct {
	MaxRecords int64
	MaxBytes   int64
}

// Validate reports the first deterministic validation failure for l as a
// *ValidationError, or nil when both limits are positive.
func (l RetentionLimits) Validate() error {
	if l.MaxRecords <= 0 {
		return &ValidationError{Field: "max_records", Err: ErrInvalidMaxRecords}
	}
	if l.MaxBytes <= 0 {
		return &ValidationError{Field: "max_bytes", Err: ErrInvalidMaxBytes}
	}
	return nil
}

// RetainedRange reports the currently available bounded window of committed
// Records for one Topic: the earliest AggregateSequence still retained and
// the current live head. RetainedRange makes no durability claim: it
// describes in-memory state that exists only for the lifetime of the owning
// process.
type RetainedRange struct {
	Topic    TopicID
	Earliest AggregateSequence
	Head     AggregateSequence
}

// ErrInvalidGapRange reports a Gap whose From, To, or ResumeAt values are
// not a well-formed missing range.
var ErrInvalidGapRange = fmt.Errorf("events: invalid gap range")

// Gap reports that bounded in-memory retention evicted part of a requested
// aggregate range before a Read or Subscribe could deliver it. From and To
// are the inclusive bounds of the missing AggregateSequence range; ResumeAt
// is the earliest AggregateSequence a caller can still resume from through
// Read or a new subscription.
type Gap struct {
	Topic    TopicID
	From     AggregateSequence
	To       AggregateSequence
	ResumeAt AggregateSequence
}

// Validate reports the first deterministic validation failure for g as a
// *ValidationError, or nil when g describes a well-formed, resumable missing
// range.
func (g Gap) Validate() error {
	switch {
	case g.Topic == "":
		return &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	case g.From <= 0:
		return &ValidationError{Field: "from", Err: ErrInvalidGapRange}
	case g.To < g.From:
		return &ValidationError{Field: "to", Err: ErrInvalidGapRange}
	case g.ResumeAt <= g.To:
		return &ValidationError{Field: "resume_at", Err: ErrInvalidGapRange}
	default:
		return nil
	}
}
