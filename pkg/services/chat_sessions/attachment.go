package chatsessions

import "strings"

// Attachment is the public, transport-neutral identity and delivery-cursor
// fact set for one client connection's subscription to a Chat Session. Its
// AfterSequence cursor is independent of the Session's stream head and of any
// control-intent target: it defines no leader election, permission handling,
// subscription delivery, or mutable attachment storage.
type Attachment struct {
	ID            string
	SessionID     string
	ConnectionID  string
	AfterSequence uint64
	Interactive   bool
}

// Validate reports whether attachment carries every required identity. A
// zero AfterSequence is valid: it means delivery has not yet started. Missing
// ID, SessionID, or ConnectionID returns a typed InvalidAttachmentError; the
// error never carries the supplied identity values.
func (attachment Attachment) Validate() error {
	hasID := strings.TrimSpace(attachment.ID) != ""
	hasSessionID := strings.TrimSpace(attachment.SessionID) != ""
	hasConnectionID := strings.TrimSpace(attachment.ConnectionID) != ""

	switch {
	case !hasID:
		return &InvalidAttachmentError{Reason: AttachmentInvalidMissingID}
	case !hasSessionID:
		return &InvalidAttachmentError{Reason: AttachmentInvalidMissingSessionID}
	case !hasConnectionID:
		return &InvalidAttachmentError{Reason: AttachmentInvalidMissingConnectionID}
	default:
		return nil
	}
}
