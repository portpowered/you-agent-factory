package factorysessions

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
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

// SubscribeFactoryResponseEvents opens the exact session-owned response-event
// cursor selected by request. It never falls back to the default session.
func SubscribeFactoryResponseEvents(
	ctx context.Context,
	session *LiveSession,
	request ResponseEventSubscriptionRequest,
) (ResponseEventCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.AfterSequence < 0 {
		return nil, ErrInvalidResponseEventCursor
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	if session.ResponseEvents == nil {
		return nil, ErrRuntimeNotAvailable
	}
	selected := make(map[ResponseEventKind]struct{}, len(request.Kinds))
	for _, kind := range request.Kinds {
		if !validResponseEventSubscriptionKind(kind) {
			return nil, ErrInvalidResponseEventFilter
		}
		selected[kind] = struct{}{}
	}
	options := make([]ResponseEventSubscribeOption, 0, 1)
	if dispatchID := strings.TrimSpace(request.DispatchID); dispatchID != "" {
		options = append(options, WithResponseEventDispatchFilter(dispatchID))
	}
	subscription, err := session.ResponseEvents.Subscribe(request.AfterSequence, options...)
	if err != nil {
		return nil, err
	}
	if len(request.Kinds) == 0 {
		return subscription, nil
	}
	return &filteredResponseEventCursor{cursor: subscription, selected: selected}, nil
}

func validResponseEventSubscriptionKind(kind ResponseEventKind) bool {
	switch kind {
	case ResponseEventKindSession, ResponseEventKindRun, ResponseEventKindTurn,
		ResponseEventKindMessage, ResponseEventKindReasoning, ResponseEventKindTool,
		ResponseEventKindFileChange, ResponseEventKindPlan, ResponseEventKindProgress,
		ResponseEventKindUsage, ResponseEventKindError, ResponseEventKindStreamGap:
		return true
	default:
		return false
	}
}

type filteredResponseEventCursor struct {
	cursor   ResponseEventCursor
	selected map[ResponseEventKind]struct{}
}

func (c *filteredResponseEventCursor) Next(ctx context.Context) ([]FactoryResponseEvent, error) {
	for {
		events, err := c.cursor.Next(ctx)
		if err != nil {
			return nil, err
		}
		if filtered := c.filter(events); len(filtered) > 0 {
			return filtered, nil
		}
	}
}

func (c *filteredResponseEventCursor) Drain() ([]FactoryResponseEvent, error) {
	events, err := c.cursor.Drain()
	if err != nil {
		return nil, err
	}
	return c.filter(events), nil
}

func (c *filteredResponseEventCursor) Detach() {
	c.cursor.Detach()
}

func (c *filteredResponseEventCursor) filter(events []FactoryResponseEvent) []FactoryResponseEvent {
	filtered := make([]FactoryResponseEvent, 0, len(events))
	for _, event := range events {
		if event.Kind == ResponseEventKindStreamGap {
			filtered = append(filtered, event)
			continue
		}
		if _, ok := c.selected[event.Kind]; ok {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
