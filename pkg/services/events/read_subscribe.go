package events

import "context"

// ReadRequest asks Events for a bounded slice of a topic's aggregate
// ordering, starting after From.
type ReadRequest struct {
	Topic Topic
	From  Cursor
	Limit int
}

// Validate reports whether r names a well-formed, topic-bound read.
func (r ReadRequest) Validate() error {
	if err := r.Topic.Validate(); err != nil {
		return err
	}
	if err := r.From.Validate(); err != nil {
		return err
	}
	if !r.From.BelongsTo(r.Topic) {
		return ErrCursorTopicMismatch
	}
	if r.Limit <= 0 {
		return ErrInvalidReadLimit
	}
	return nil
}

// ReadOutcome is one explicit Read observation. A caller never infers
// missing history from an empty Records slice: Outcome states whether the
// read progressed, was already at the live head, used an invalid or
// mismatched cursor, or lost history to retention.
type ReadOutcome int

const (
	ReadOutcomeUnspecified ReadOutcome = iota
	// ReadOutcomeProgress reports one or more records read past From.
	ReadOutcomeProgress
	// ReadOutcomeAtHead reports that From already names the live head; no
	// records were available to read.
	ReadOutcomeAtHead
	// ReadOutcomeInvalidCursor reports that From does not name a position
	// Events can resolve.
	ReadOutcomeInvalidCursor
	// ReadOutcomeGap reports that From names a position no longer
	// retained; Gap describes the resumable position.
	ReadOutcomeGap
)

// GapFacts identifies a retention gap: the position a caller requested, the
// earliest position Events still retains, and the topic's live head where
// known. A caller uses GapFacts to surface or recover from loss without
// fabricating records or parsing an error string.
type GapFacts struct {
	Topic            Topic
	Requested        AggregateSequence
	EarliestRetained AggregateSequence
	Head             AggregateSequence
}

// Validate reports whether g names an internally consistent retention gap: a
// well-formed Topic, a nonzero Head (a gap cannot exist on a topic that has
// never retained a record), an EarliestRetained between 1 and Head
// inclusive, and a Requested position strictly before EarliestRetained (the
// position must actually have been evicted to be a gap).
func (g GapFacts) Validate() error {
	if err := g.Topic.Validate(); err != nil {
		return err
	}
	if g.Head == 0 {
		return ErrInvalidGapFacts
	}
	if g.EarliestRetained == 0 || g.EarliestRetained > g.Head {
		return ErrInvalidGapFacts
	}
	if g.Requested >= g.EarliestRetained {
		return ErrInvalidGapFacts
	}
	return nil
}

// ReadResult is the detached outcome of one Read call.
type ReadResult struct {
	Records  []Record
	Next     Cursor
	Retained RetainedRange
	Outcome  ReadOutcome
	Gap      *GapFacts
}

// Validate reports whether res is internally consistent: Records, Gap, and
// Outcome agree with each other so a caller cannot infer missing history
// from an empty success.
func (res ReadResult) Validate() error {
	switch res.Outcome {
	case ReadOutcomeProgress:
		if len(res.Records) == 0 || res.Gap != nil {
			return ErrInconsistentReadOutcome
		}
	case ReadOutcomeAtHead, ReadOutcomeInvalidCursor:
		if len(res.Records) != 0 || res.Gap != nil {
			return ErrInconsistentReadOutcome
		}
	case ReadOutcomeGap:
		if len(res.Records) != 0 || res.Gap == nil {
			return ErrInconsistentReadOutcome
		}
		if err := res.Gap.Validate(); err != nil {
			return err
		}
	default:
		return ErrInconsistentReadOutcome
	}
	return nil
}

// SubscribeRequest asks Events for ongoing delivery of a topic's aggregate
// ordering starting after From, bounded per delivery by Limit.
type SubscribeRequest struct {
	Topic Topic
	From  Cursor
	Limit int
}

// Validate reports whether r names a well-formed, topic-bound subscription.
func (r SubscribeRequest) Validate() error {
	if err := r.Topic.Validate(); err != nil {
		return err
	}
	if err := r.From.Validate(); err != nil {
		return err
	}
	if !r.From.BelongsTo(r.Topic) {
		return ErrCursorTopicMismatch
	}
	if r.Limit <= 0 {
		return ErrInvalidReadLimit
	}
	return nil
}

// DeliveryKind is one explicit subscription observation.
type DeliveryKind int

const (
	DeliveryUnspecified DeliveryKind = iota
	// DeliveryRecord reports one delivered Record and the Cursor the
	// subscription advanced to.
	DeliveryRecord
	// DeliveryGap reports a retention gap; Gap describes the resumable
	// position and no record is fabricated in its place.
	DeliveryGap
	// DeliveryClosed reports a normal, expected end of delivery.
	DeliveryClosed
	// DeliveryCanceled reports that delivery ended because its context was
	// canceled.
	DeliveryCanceled
	// DeliveryBackpressure reports that delivery ended because the
	// consumer could not keep up; it is an explicit terminal outcome, never
	// silent loss.
	DeliveryBackpressure
)

// Delivery is one observation from a Subscription. Record and Cursor are
// set only for DeliveryRecord; Gap is set only for DeliveryGap.
type Delivery struct {
	Kind   DeliveryKind
	Record Record
	Cursor Cursor
	Gap    *GapFacts
}

// Validate reports whether d is internally consistent with its Kind: a
// caller can never observe an impossible combination such as DeliveryGap
// with a nil or malformed Gap, DeliveryRecord with an unset Record/Cursor or
// a Cursor naming a different topic than the delivered Record, or a
// terminal/gap Delivery carrying a leftover Record or Cursor.
func (d Delivery) Validate() error {
	recordSet := d.Record.ID != (RecordID{})
	cursorSet := d.Cursor != (Cursor{})
	switch d.Kind {
	case DeliveryRecord:
		if !recordSet || !cursorSet || d.Gap != nil {
			return ErrInconsistentDelivery
		}
		if err := d.Record.Validate(); err != nil {
			return err
		}
		if err := d.Cursor.Validate(); err != nil {
			return err
		}
		if !d.Cursor.BelongsTo(d.Record.ID.Topic) {
			return ErrCursorTopicMismatch
		}
	case DeliveryGap:
		if recordSet || cursorSet || d.Gap == nil {
			return ErrInconsistentDelivery
		}
		if err := d.Gap.Validate(); err != nil {
			return err
		}
	case DeliveryClosed, DeliveryCanceled, DeliveryBackpressure:
		if recordSet || cursorSet || d.Gap != nil {
			return ErrInconsistentDelivery
		}
	default:
		return ErrInconsistentDelivery
	}
	return nil
}

// Subscription observes the next Delivery for one Subscribe call. Next may
// block until a record, gap, close, or context cancellation is available; it
// does not prescribe channels, buffers, goroutines, or transport framing.
type Subscription func(context.Context) Delivery

// Next observes the next Delivery. A nil Subscription reports
// DeliveryClosed.
func (s Subscription) Next(ctx context.Context) Delivery {
	if s == nil {
		return Delivery{Kind: DeliveryClosed}
	}
	return s(ctx)
}
