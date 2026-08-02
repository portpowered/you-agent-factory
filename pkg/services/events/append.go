package events

// AppendRequest asks the Service to commit one source-native delivery
// envelope to a destination Topic. SourceType, SourceID, SourceSequence, and
// SourceEventID together form the exact append idempotency tuple.
type AppendRequest struct {
	Topic          TopicID
	SourceType     SourceType
	SourceID       SourceID
	SourceSequence SourceSequence
	SourceEventID  SourceEventID
	Schema         SchemaID
	Payload        Payload
}

// Validate reports the first deterministic validation failure for req as a
// *ValidationError, or nil when every value is well-formed. Validate never
// inspects Payload content: an empty, nil, or arbitrarily shaped Payload
// from any source vocabulary is a valid opaque value.
func (req AppendRequest) Validate() error {
	switch {
	case req.Topic == "":
		return &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	case req.SourceType == "":
		return &ValidationError{Field: "source_type", Err: ErrInvalidSourceType}
	case req.SourceID == "":
		return &ValidationError{Field: "source_id", Err: ErrInvalidSourceID}
	case req.SourceSequence <= 0:
		return &ValidationError{Field: "source_sequence", Err: ErrInvalidSourceSequence}
	case req.SourceEventID == "":
		return &ValidationError{Field: "source_event_id", Err: ErrInvalidSourceEventID}
	case req.Schema == "":
		return &ValidationError{Field: "schema_id", Err: ErrInvalidSchemaID}
	default:
		return nil
	}
}

// Key returns the exact append idempotency tuple carried by req.
func (req AppendRequest) Key() IdempotencyKey {
	return IdempotencyKey{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
}

// AppendOutcome classifies whether an AppendResult committed a new Record or
// resolved to a previously committed duplicate.
type AppendOutcome string

const (
	// AppendOutcomeCommitted reports that a request produced a newly
	// committed Record at the next commit-order AggregateSequence for its
	// Topic.
	AppendOutcomeCommitted AppendOutcome = "COMMITTED"
	// AppendOutcomeDuplicate reports that a request's idempotency tuple
	// matched an already-committed Record; AppendResult.Record identifies
	// that same Record rather than creating a second one.
	AppendOutcomeDuplicate AppendOutcome = "DUPLICATE"
)

// AppendResult is the detached outcome of one Append call. When Outcome is
// AppendOutcomeDuplicate, Record is the previously committed Record that
// matched the request's idempotency tuple, unchanged.
type AppendResult struct {
	Record  Record
	Outcome AppendOutcome
}
