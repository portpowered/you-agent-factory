package events

// AttachMode selects how a source attachment consumes the source topic's
// history relative to Events' live head.
type AttachMode int

const (
	AttachModeUnspecified AttachMode = iota
	// AttachModeRetainedThenLive replays currently retained source records
	// starting at StartAt before delivering live records.
	AttachModeRetainedThenLive
	// AttachModeLiveOnly begins delivery at the source's live head and
	// never replays retained history.
	AttachModeLiveOnly
)

// Validate reports whether m is a supported attachment mode.
func (m AttachMode) Validate() error {
	switch m {
	case AttachModeRetainedThenLive, AttachModeLiveOnly:
		return nil
	default:
		return ErrUnsupportedAttachMode
	}
}

// AttachOutcome distinguishes a newly accepted attachment from an
// already-attached, idempotent outcome.
type AttachOutcome int

const (
	AttachOutcomeUnspecified AttachOutcome = iota
	// AttachOutcomeAccepted reports that the attachment was newly accepted.
	AttachOutcomeAccepted
	// AttachOutcomeAlreadyAttached reports that an equivalent attachment
	// already exists; the returned ID is the original, not a new one.
	AttachOutcomeAlreadyAttached
)

// AttachmentID stably correlates one Destination/Source attachment. Callers
// retain it to recognize a later idempotent AttachSource call for the same
// pair.
type AttachmentID struct {
	Destination Topic
	Source      Topic
}

// Validate reports whether id names a well-formed, non-self attachment.
func (id AttachmentID) Validate() error {
	if err := id.Destination.Validate(); err != nil {
		return err
	}
	if err := id.Source.Validate(); err != nil {
		return err
	}
	if id.Destination == id.Source {
		return ErrSelfAttachment
	}
	return nil
}

// AttachSourceRequest asks an aggregate-stream owner's later in-memory
// sequencer to follow Source from StartAt. AttachSourceRequest contains no
// embedded service, resolver, callback, channel, store, transport, or
// dependency bag, and makes no claim that Source's historical data is
// durable: Events retains no persistence responsibility (D1 in
// docs/internal/projects/acp-program/README.md).
type AttachSourceRequest struct {
	Destination Topic
	Source      Topic
	StartAt     Cursor
	Mode        AttachMode
}

// Validate reports whether r names a well-formed, compatible, non-self
// attachment with a starting position drawn from Source.
func (r AttachSourceRequest) Validate() error {
	if err := (AttachmentID{Destination: r.Destination, Source: r.Source}).Validate(); err != nil {
		return err
	}
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	if err := r.StartAt.Validate(); err != nil {
		return err
	}
	if !r.StartAt.BelongsTo(r.Source) {
		return ErrIncompatibleAttachmentCursor
	}
	return nil
}

// AttachSourceResult is the detached success outcome of one AttachSource
// call.
type AttachSourceResult struct {
	ID      AttachmentID
	Outcome AttachOutcome
	StartAt Cursor
}
