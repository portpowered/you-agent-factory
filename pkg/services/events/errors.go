package events

import "errors"

// Identity, sequence, cursor, and retained-position validation failures.
// Callers classify a failure with errors.Is against these sentinels rather
// than matching error strings.
var (
	ErrEmptyTopic               = errors.New("events: topic is empty")
	ErrMalformedTopic           = errors.New("events: topic is malformed")
	ErrEmptySourceType          = errors.New("events: source type is empty")
	ErrMalformedSourceType      = errors.New("events: source type is malformed")
	ErrEmptySourceID            = errors.New("events: source id is empty")
	ErrMalformedSourceID        = errors.New("events: source id is malformed")
	ErrInvalidSourceSequence    = errors.New("events: source sequence is invalid")
	ErrEmptySourceEventID       = errors.New("events: source event id is empty")
	ErrMalformedSourceEventID   = errors.New("events: source event id is malformed")
	ErrEmptySchemaID            = errors.New("events: schema id is empty")
	ErrMalformedSchemaID        = errors.New("events: schema id is malformed")
	ErrInvalidAggregateSequence = errors.New("events: aggregate sequence is invalid")
	ErrInvalidRetainedRange     = errors.New("events: retained range is internally inconsistent")

	ErrEmptyPayload         = errors.New("events: payload is empty")
	ErrMalformedPayloadJSON = errors.New("events: payload is not well-formed JSON")

	ErrSelfAttachment               = errors.New("events: source cannot attach to itself")
	ErrUnsupportedAttachMode        = errors.New("events: attach mode is unsupported")
	ErrIncompatibleAttachmentCursor = errors.New("events: starting cursor does not belong to the attachment source")

	ErrCursorTopicMismatch     = errors.New("events: cursor does not belong to the requested topic")
	ErrInvalidReadLimit        = errors.New("events: read/subscribe limit must be positive")
	ErrInconsistentReadOutcome = errors.New("events: read result outcome is inconsistent with its records/gap")
)
