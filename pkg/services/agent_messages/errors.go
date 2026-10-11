package agentmessages

import "errors"

// Errors carry stable codes only. Raw requests, credentials, storage paths and
// collaborator diagnostics must never be attached to a message failure.
var (
	ErrBadRequest           = errors.New("BAD_REQUEST")
	ErrNotPermitted         = errors.New("MESSAGE_NOT_PERMITTED")
	ErrRecipientNotFound    = errors.New("MESSAGE_RECIPIENT_NOT_FOUND")
	ErrInterruptUnsupported = errors.New("MESSAGE_INTERRUPT_UNSUPPORTED")
	ErrLimitExceeded        = errors.New("MESSAGE_LIMIT_EXCEEDED")
	ErrRequestConflict      = errors.New("MESSAGE_REQUEST_CONFLICT")
	ErrCursorInvalid        = errors.New("MESSAGE_CURSOR_INVALID")
	ErrMessageNotFound      = errors.New("MESSAGE_NOT_FOUND")
	ErrDisabled             = errors.New("MESSAGING_DISABLED")
)

// LimitError reports the exhausted dimension without exposing request content.
// RetryAfterSeconds is meaningful only for a rolling sender window.
type LimitError struct {
	Dimension         string
	RetryAfterSeconds int
}

func (e *LimitError) Error() string { return ErrLimitExceeded.Error() }
func (e *LimitError) Unwrap() error { return ErrLimitExceeded }
