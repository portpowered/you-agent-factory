package events

import "context"

// Service is the singular Events root contract: append, attach a source,
// read, and subscribe over one process-local, in-memory event stream (D1,
// D2 in docs/internal/projects/acp-program/README.md). Peers depend on
// Service rather than a store, journal, or transport type.
//
// Read and Subscribe return a non-nil error for a malformed request (see
// ReadRequest.Validate/SubscribeRequest.Validate) and for an operation
// failure distinct from an expected ReadResult/Delivery outcome: an unknown
// topic (errors.Is(err, ErrUnknownTopic)), a well-formed but unresolvable
// cursor (errors.Is(err, ErrUnresolvableCursor)), or any other operation
// failure (errors.Is(err, ErrOperationFailed)). A nil error with
// ReadOutcomeAtHead/InvalidCursor/Gap or DeliveryGap/Closed/Canceled/
// Backpressure is an expected, successful observation, not a failure.
type Service interface {
	Append(context.Context, AppendRequest) (AppendResult, error)
	AttachSource(context.Context, AttachSourceRequest) (AttachSourceResult, error)
	Read(context.Context, ReadRequest) (ReadResult, error)
	Subscribe(context.Context, SubscribeRequest) (Subscription, error)
}
