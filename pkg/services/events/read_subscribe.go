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

// Validate reports whether res is internally consistent: Records, Next,
// Retained, Gap, and Outcome agree with each other so a caller cannot infer
// missing history from an empty success, accept a malformed or
// mixed-topic record, or resume from a Next cursor that skips or replays
// history. ReadOutcomeProgress and ReadOutcomeAtHead additionally require a
// well-formed Next cursor and Retained range naming the same topic:
// Progress requires each Record to validate, to be contiguous in Position
// (each successive record's Position is exactly one more than the last, so
// no aggregate position is silently skipped), to stay within Retained, and
// Next to name exactly the last delivered Record's position; AtHead
// requires Next to name exactly the Retained head, since being at head
// means there is nothing to advance past. ReadOutcomeInvalidCursor and
// ReadOutcomeGap carry no resume/retention state of their own: Next and
// Retained must be their zero values, since an invalid cursor names no
// resolvable position and a gap's resumable position is already carried by
// Gap; a leftover Next or Retained on either outcome would let a caller
// observe two contradictory resume states for the same read.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (res ReadResult) Validate() error {
	switch res.Outcome {
	case ReadOutcomeProgress:
		if len(res.Records) == 0 || res.Gap != nil {
			return ErrInconsistentReadOutcome
		}
		if err := res.validateNextAndRetained(); err != nil {
			return err
		}
		var previous AggregateSequence
		for i, rec := range res.Records {
			if err := rec.Validate(); err != nil {
				return err
			}
			if rec.ID.Topic != res.Next.Topic {
				return ErrCursorTopicMismatch
			}
			if i > 0 && (rec.ID.Position <= previous || rec.ID.Position-previous != 1) {
				return ErrInconsistentReadOutcome
			}
			previous = rec.ID.Position
		}
		first := res.Records[0]
		last := res.Records[len(res.Records)-1]
		if first.ID.Position < res.Retained.Earliest || last.ID.Position > res.Retained.Head {
			return ErrInconsistentReadOutcome
		}
		if res.Next.Position != last.ID.Position {
			return ErrInconsistentReadOutcome
		}
	case ReadOutcomeAtHead:
		if len(res.Records) != 0 || res.Gap != nil {
			return ErrInconsistentReadOutcome
		}
		if err := res.validateNextAndRetained(); err != nil {
			return err
		}
		if res.Next.Position != res.Retained.Head {
			return ErrInconsistentReadOutcome
		}
	case ReadOutcomeInvalidCursor:
		if len(res.Records) != 0 || res.Gap != nil {
			return ErrInconsistentReadOutcome
		}
		if res.Next != (Cursor{}) || res.Retained != (RetainedRange{}) {
			return ErrInconsistentReadOutcome
		}
	case ReadOutcomeGap:
		if len(res.Records) != 0 || res.Gap == nil {
			return ErrInconsistentReadOutcome
		}
		if err := res.Gap.Validate(); err != nil {
			return err
		}
		if res.Next != (Cursor{}) || res.Retained != (RetainedRange{}) {
			return ErrInconsistentReadOutcome
		}
	default:
		return ErrInconsistentReadOutcome
	}
	return nil
}

// validateNextAndRetained validates Next and Retained individually and
// checks that they name the same topic. Callers use this for the outcomes
// (Progress, AtHead) that report real resume/retention state.
func (res ReadResult) validateNextAndRetained() error {
	if err := res.Next.Validate(); err != nil {
		return err
	}
	if err := res.Retained.Validate(); err != nil {
		return err
	}
	if res.Retained.Topic != res.Next.Topic {
		return ErrCursorTopicMismatch
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
// with a nil or malformed Gap, DeliveryRecord with an unset Record/Cursor, a
// Cursor naming a different topic than the delivered Record, a Cursor whose
// Position does not name that exact Record (which would let a resumed read
// skip or replay it), or a terminal/gap Delivery carrying a leftover Record
// or Cursor. A Record with only some fields set (for example a stray
// Payload on a gap or terminal delivery) is rejected the same as a
// fully-set one: Record.IsZero checks every field, not just ID.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (d Delivery) Validate() error {
	recordZero := d.Record.IsZero()
	cursorSet := d.Cursor != (Cursor{})
	switch d.Kind {
	case DeliveryRecord:
		if recordZero || !cursorSet || d.Gap != nil {
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
		if d.Cursor.Position != d.Record.ID.Position {
			return ErrInconsistentDelivery
		}
	case DeliveryGap:
		if !recordZero || cursorSet || d.Gap == nil {
			return ErrInconsistentDelivery
		}
		if err := d.Gap.Validate(); err != nil {
			return err
		}
	case DeliveryClosed, DeliveryCanceled, DeliveryBackpressure:
		if !recordZero || cursorSet || d.Gap != nil {
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
