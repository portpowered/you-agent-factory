package factorysessions

// Response-stream root slice freezes subscription, cursor, event, gap, and
// completion vocabulary on the singular Service. Peers consume these plain root
// contracts without importing private response-stream store or manager types:
//
//   - Subscription: ResponseStreamSubscriptionRequest
//   - Cursor: ResponseStreamCursor
//   - Event: ResponseStreamEvent
//   - Gap: ResponseStreamGap / ResponseStreamKindGap
//   - Completion: ResponseStreamCompletionKind + ResponseStreamCompletionPhase
//
// Typed failures peers distinguish with errors.Is:
//   - ErrResponseStreamStaleCursor for stale/invalid reconnect positions
//   - ResponseStreamKindGap events for retained-window / compaction gaps
//   - ErrResponseStreamSubscriptionClosed for cancelled/detached subscriptions
//
// Response-stream operations remain methods on Service (notably
// SubscribeFactoryResponseEvents); this file does not publish a nested stream
// interface for peer import.

// ResponseStreamSubscriptionRequest is the plain root subscribe request for
// retained-then-live response-event delivery.
type ResponseStreamSubscriptionRequest = ResponseEventSubscriptionRequest

// ResponseStreamCursor is the plain root detached retained-then-live cursor
// returned by Service.SubscribeFactoryResponseEvents.
type ResponseStreamCursor = ResponseEventCursor

// ResponseStreamEvent is the plain root response-event envelope delivered
// through a ResponseStreamCursor.
type ResponseStreamEvent = FactoryResponseEvent

// ResponseStreamGap is the plain root gap payload describing compacted or
// otherwise unavailable sequence ranges (and item-scoped gaps).
type ResponseStreamGap = ResponseEventStreamGap

const (
	// ResponseStreamKindGap is the published event kind for stream-gap outcomes.
	ResponseStreamKindGap = ResponseEventKindStreamGap
	// ResponseStreamCompletionKind is the published event kind used for terminal
	// run completion on the response-stream slice.
	ResponseStreamCompletionKind = ResponseEventKindRun
	// ResponseStreamCompletionPhase is the published phase used for terminal
	// completion on the response-stream slice.
	ResponseStreamCompletionPhase = ResponseEventPhaseCompleted
)

var (
	// ErrResponseStreamStaleCursor reports an invalid or stale reconnect cursor
	// on the response-stream root slice.
	ErrResponseStreamStaleCursor = ErrInvalidResponseEventCursor
	// ErrResponseStreamSubscriptionClosed reports that a response-stream cursor
	// was detached/cancelled or its owning store closed.
	ErrResponseStreamSubscriptionClosed = ErrResponseEventSubscriptionClosed
)
