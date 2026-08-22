package chatsessions

import "github.com/portpowered/infinite-you/pkg/services/events"

// AcknowledgeAttachmentOutcome distinguishes a newly advanced attachment
// delivery cursor from an idempotent reconciliation that left it unchanged
// because it had already reached or passed the requested position.
type AcknowledgeAttachmentOutcome int

const (
	AcknowledgeAttachmentOutcomeUnspecified AcknowledgeAttachmentOutcome = iota
	// AcknowledgeAttachmentOutcomeAdvanced reports that the named
	// Attachment's AfterSequence was newly advanced.
	AcknowledgeAttachmentOutcomeAdvanced
	// AcknowledgeAttachmentOutcomeAlreadyCurrent reports that AfterSequence
	// already stood at or beyond the requested position; the attachment was
	// left byte-for-byte unchanged.
	AcknowledgeAttachmentOutcomeAlreadyCurrent
)

// AcknowledgeAttachmentRequest asks one Attachment's delivery cursor to
// advance to AfterSequence -- a position within EventsTopic(SessionID) the
// caller has actually observed -- under an optimistic session-version
// guard. Advancing one attachment's cursor never reads or writes any other
// attachment, Session.StreamHead, or any ControlIntent: it is independent
// bookkeeping scoped to exactly the named AttachmentID.
type AcknowledgeAttachmentRequest struct {
	SessionID       string
	AttachmentID    string
	ExpectedVersion uint64
	// AfterSequence is the position within EventsTopic(SessionID) the
	// attachment has now observed through, in commit order.
	AfterSequence events.AggregateSequence
}

// Validate reports whether r is well-formed enough to attempt an
// acknowledgement: SessionID and AttachmentID are non-blank and
// AfterSequence is a valid already-assigned position.
func (r AcknowledgeAttachmentRequest) Validate() error {
	if r.SessionID == "" {
		return newValidationError("AcknowledgeAttachmentRequest", "SessionID", ErrRequiredValue)
	}
	if r.AttachmentID == "" {
		return newValidationError("AcknowledgeAttachmentRequest", "AttachmentID", ErrRequiredValue)
	}
	if err := r.AfterSequence.ValidateAssigned(); err != nil {
		return newValidationError("AcknowledgeAttachmentRequest", "AfterSequence", err)
	}
	return nil
}

// AcknowledgeAttachmentResult carries the Attachment after a successful (or
// idempotently reconciled) delivery-cursor advancement.
type AcknowledgeAttachmentResult struct {
	Attachment Attachment
	Outcome    AcknowledgeAttachmentOutcome
}
