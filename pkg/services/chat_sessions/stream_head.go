package chatsessions

import "github.com/portpowered/infinite-you/pkg/services/events"

// AdvanceStreamHeadOutcome distinguishes a newly committed StreamHead
// advancement from an idempotent reconciliation that left the session
// unchanged because it had already reached or passed the requested position.
type AdvanceStreamHeadOutcome int

const (
	AdvanceStreamHeadOutcomeUnspecified AdvanceStreamHeadOutcome = iota
	// AdvanceStreamHeadOutcomeAdvanced reports that Session.StreamHead and
	// Session.Version were newly advanced.
	AdvanceStreamHeadOutcomeAdvanced
	// AdvanceStreamHeadOutcomeAlreadyCurrent reports that Session.StreamHead
	// already stood at or beyond AggregateSequence; the session was left
	// byte-for-byte unchanged, including its Version.
	AdvanceStreamHeadOutcomeAlreadyCurrent
)

// AdvanceStreamHeadRequest asks a Chat Session's stream head to advance to
// AggregateSequence -- the position Sequence's accepted commit was assigned
// within EventsTopic(SessionID) -- under an optimistic-version guard.
// SourceType, SourceID, SourceSequence, and SourceEventID are the same
// identity the originating Sequence call used; AdvanceStreamHead requires
// them to match the exact identity Sequence recorded when it committed
// AggregateSequence, rejecting a mismatch with *UncommittedStreamPositionError
// rather than trusting a caller-supplied position, and also carries them
// through so structured operation logs can identify which source delivery
// drove one StreamHead advancement without logging its payload.
type AdvanceStreamHeadRequest struct {
	SessionID       string
	ExpectedVersion uint64
	// AggregateSequence is the committed record's position within
	// EventsTopic(SessionID) -- SequenceResult.AggregateSequence from the
	// accepted Sequence call this advancement commits.
	AggregateSequence events.AggregateSequence
	SourceType        events.SourceType
	SourceID          events.SourceID
	SourceSequence    events.SourceSequence
	SourceEventID     events.SourceEventID
}

// Validate reports whether r is well-formed enough to attempt a StreamHead
// advancement: SessionID is non-blank, AggregateSequence is a valid
// already-assigned position, and every source identity field is well-formed.
func (r AdvanceStreamHeadRequest) Validate() error {
	if r.SessionID == "" {
		return newValidationError("AdvanceStreamHeadRequest", "SessionID", ErrRequiredValue)
	}
	if err := r.AggregateSequence.ValidateAssigned(); err != nil {
		return newValidationError("AdvanceStreamHeadRequest", "AggregateSequence", err)
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
	return nil
}

// AdvanceStreamHeadResult carries the Session after a successful (or
// idempotently reconciled) StreamHead advancement.
type AdvanceStreamHeadResult struct {
	Session Session
	Outcome AdvanceStreamHeadOutcome
}
