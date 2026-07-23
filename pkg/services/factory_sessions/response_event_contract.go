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
