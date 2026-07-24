package factorysessions

import (
	"errors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
)

// Factory Session response-event contracts are published at the service root.
type (
	ResponseEventCapabilities    = responseevents.Capabilities
	ResponseEventContentBlock    = responseevents.ContentBlock
	ResponseEventContentKind     = responseevents.ContentBlockKind
	ResponseEventDelivery        = responseevents.Delivery
	ResponseEventDraft           = responseevents.Draft
	ResponseEventErrorPayload    = responseevents.ErrorPayload
	FactoryResponseEvent         = responseevents.FactoryResponseEvent
	ResponseEventFidelity        = responseevents.Fidelity
	ResponseEventFileChange      = responseevents.FileChangePayload
	ResponseEventKind            = responseevents.Kind
	ResponseEventMessageDelta    = responseevents.MessageDeltaPayload
	ResponseEventMessage         = responseevents.MessagePayload
	ResponseEventPhase           = responseevents.Phase
	ResponseEventPlan            = responseevents.PlanPayload
	ResponseEventPlanStep        = responseevents.PlanStep
	ResponseEventProgress        = responseevents.ProgressPayload
	ResponseEventProvenance      = responseevents.Provenance
	ResponseEventReasoning       = responseevents.ReasoningPayload
	ResponseEventRepresentation  = responseevents.Representation
	ResponseEventRun             = responseevents.RunPayload
	ResponseEventSession         = responseevents.SessionPayload
	ResponseEventStreamGap       = responseevents.StreamGapPayload
	ResponseEventToolDelta       = responseevents.ToolDeltaPayload
	ResponseEventTool            = responseevents.ToolPayload
	ResponseEventTurn            = responseevents.TurnPayload
	ResponseEventUsage           = responseevents.UsagePayload
	ResponseEventValidationError = responseevents.ValidationError
)

const (
	ResponseEventSchemaVersionV1 = responseevents.SchemaVersionV1

	ResponseEventKindSession    = responseevents.KindSession
	ResponseEventKindRun        = responseevents.KindRun
	ResponseEventKindTurn       = responseevents.KindTurn
	ResponseEventKindMessage    = responseevents.KindMessage
	ResponseEventKindReasoning  = responseevents.KindReasoning
	ResponseEventKindTool       = responseevents.KindTool
	ResponseEventKindFileChange = responseevents.KindFileChange
	ResponseEventKindPlan       = responseevents.KindPlan
	ResponseEventKindProgress   = responseevents.KindProgress
	ResponseEventKindUsage      = responseevents.KindUsage
	ResponseEventKindError      = responseevents.KindError
	ResponseEventKindStreamGap  = responseevents.KindStreamGap

	ResponseEventPhaseStarted   = responseevents.PhaseStarted
	ResponseEventPhaseDelta     = responseevents.PhaseDelta
	ResponseEventPhaseUpdated   = responseevents.PhaseUpdated
	ResponseEventPhaseCompleted = responseevents.PhaseCompleted
	ResponseEventPhaseFailed    = responseevents.PhaseFailed
	ResponseEventPhaseCanceled  = responseevents.PhaseCanceled

	ResponseEventDeliveryNativeStream = responseevents.DeliveryNativeStream
	ResponseEventDeliveryNativeFinal  = responseevents.DeliveryNativeFinal
	ResponseEventDeliverySynthesized  = responseevents.DeliverySynthesized
	ResponseEventDeliveryReplay       = responseevents.DeliveryReplay

	ResponseEventRepresentationDelta        = responseevents.RepresentationDelta
	ResponseEventRepresentationSnapshot     = responseevents.RepresentationSnapshot
	ResponseEventRepresentationNotification = responseevents.RepresentationNotification

	ResponseEventFidelityLossless      = responseevents.FidelityLossless
	ResponseEventFidelityNormalized    = responseevents.FidelityNormalized
	ResponseEventFidelityLossy         = responseevents.FidelityLossy
	ResponseEventFidelityFinalOnly     = responseevents.FidelityFinalOnly
	ResponseEventFidelityLifecycleOnly = responseevents.FidelityLifecycleOnly

	ResponseEventContentBlockText             = responseevents.ContentBlockText
	ResponseEventContentBlockReasoningSummary = responseevents.ContentBlockReasoningSummary
	ResponseEventContentBlockToolRequest      = responseevents.ContentBlockToolRequest
	ResponseEventContentBlockImageRef         = responseevents.ContentBlockImageRef
	ResponseEventContentBlockResourceRef      = responseevents.ContentBlockResourceRef
	ResponseEventContentBlockStructuredOutput = responseevents.ContentBlockStructuredOutput
)

var CloneResponseEventDraft = responseevents.CloneDraft
var IsAuthoritativeResponseEventMessageSnapshot = responseevents.IsAuthoritativeMessageSnapshot
var ValidateResponseEventDraft = responseevents.ValidateDraft
var ValidateFactoryResponseEvent = responseevents.ValidateEvent

// ResponseEventSubscriptionRequest selects one Factory Session response-event
// cursor. Session lookup, reconnect position, dispatch selection, kind
// filtering, and retained-then-live ordering are Factory Sessions policy.
type ResponseEventSubscriptionRequest struct {
	SessionID     string
	AfterSequence int64
	DispatchID    string
	Kinds         []ResponseEventKind
}

var (
	// ErrInvalidResponseEventCursor reports an invalid reconnect position.
	ErrInvalidResponseEventCursor = errors.New("invalid factory response-event cursor")
	// ErrInvalidResponseEventFilter reports an unsupported response-event kind.
	ErrInvalidResponseEventFilter = errors.New("invalid factory response-event filter")
)

// --- merged from response_stream_contract.go ---

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
