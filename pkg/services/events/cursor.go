package events

// Cursor names a position within one Topic's aggregate ordering. A Cursor is
// only meaningful relative to the Topic it names: callers must not present a
// Cursor obtained for one topic to a Read or Subscribe operation on another.
// BelongsTo lets a caller reject that mismatch explicitly instead of
// accidentally reading from the wrong stream.
//
// The zero value is invalid: a valid Cursor always names its Topic, even
// when it names the start-of-stream position, Cursor{Topic: topic} at the
// zero Position.
type Cursor struct {
	Topic    Topic
	Position AggregateSequence
}

// Validate reports whether c names a well-formed cursor. A cursor at
// Position 0 (the start-of-topic position) is valid; Validate only requires
// a well-formed Topic, since Position 0 is a meaningful starting position
// rather than an assigned record position.
func (c Cursor) Validate() error {
	return c.Topic.Validate()
}

// BelongsTo reports whether c was issued against topic. Read and Subscribe
// operations use this to reject a cursor's use against a different stream
// rather than silently reading from the wrong topic.
func (c Cursor) BelongsTo(topic Topic) bool {
	return c.Topic == topic
}

// RetainedRange describes the span of AggregateSequence positions Events
// currently retains for a topic, plus the topic's live head position.
//
// Earliest is the oldest retained record position; Head is the position of
// the most recently accepted record. Both are 0 when the topic has never
// accepted a record. When Head is nonzero, Earliest must be at least 1 and
// no greater than Head: an Earliest of 0 alongside a nonzero Head describes
// a retained record at the reserved "before the first record" position,
// which Validate rejects as internally inconsistent.
type RetainedRange struct {
	Topic    Topic
	Earliest AggregateSequence
	Head     AggregateSequence
}

// Validate reports whether r describes an internally consistent retained
// range for a well-formed topic.
func (r RetainedRange) Validate() error {
	if err := r.Topic.Validate(); err != nil {
		return err
	}
	if r.Head == 0 {
		if r.Earliest != 0 {
			return ErrInvalidRetainedRange
		}
		return nil
	}
	if r.Earliest == 0 || r.Earliest > r.Head {
		return ErrInvalidRetainedRange
	}
	return nil
}
