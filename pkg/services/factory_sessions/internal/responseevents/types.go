// Package responseevents owns the published Factory Session response envelope
// and retains compatibility names for the Workers-owned provider draft
// vocabulary.
package responseevents

import (
	"encoding/json"
	"time"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

const SchemaVersionV1 = "agent-factory.response-event.v1"

type Kind = workers.Kind
type Phase = workers.Phase
type Delivery = workers.Delivery
type Representation = workers.Representation
type Fidelity = workers.Fidelity
type Provenance = workers.Provenance
type Capabilities = workers.Capabilities
type Draft = workers.Draft
type ContentBlockKind = workers.ContentBlockKind
type ContentBlock = workers.ContentBlock
type SessionPayload = workers.SessionPayload
type RunPayload = workers.RunPayload
type TurnPayload = workers.TurnPayload
type MessagePayload = workers.MessagePayload
type MessageDeltaPayload = workers.MessageDeltaPayload
type ReasoningPayload = workers.ReasoningPayload
type ToolPayload = workers.ToolPayload
type ToolDeltaPayload = workers.ToolDeltaPayload
type FileChangePayload = workers.FileChangePayload
type PlanStep = workers.PlanStep
type PlanPayload = workers.PlanPayload
type ProgressPayload = workers.ProgressPayload
type UsagePayload = workers.UsagePayload
type ErrorPayload = workers.ErrorPayload
type StreamGapPayload = workers.StreamGapPayload

const (
	KindSession    = workers.KindSession
	KindRun        = workers.KindRun
	KindTurn       = workers.KindTurn
	KindMessage    = workers.KindMessage
	KindReasoning  = workers.KindReasoning
	KindTool       = workers.KindTool
	KindFileChange = workers.KindFileChange
	KindPlan       = workers.KindPlan
	KindProgress   = workers.KindProgress
	KindUsage      = workers.KindUsage
	KindError      = workers.KindError
	KindStreamGap  = workers.KindStreamGap

	PhaseStarted   = workers.PhaseStarted
	PhaseDelta     = workers.PhaseDelta
	PhaseUpdated   = workers.PhaseUpdated
	PhaseCompleted = workers.PhaseCompleted
	PhaseFailed    = workers.PhaseFailed
	PhaseCanceled  = workers.PhaseCanceled

	DeliveryNativeStream = workers.DeliveryNativeStream
	DeliveryNativeFinal  = workers.DeliveryNativeFinal
	DeliverySynthesized  = workers.DeliverySynthesized
	DeliveryReplay       = workers.DeliveryReplay

	RepresentationDelta        = workers.RepresentationDelta
	RepresentationSnapshot     = workers.RepresentationSnapshot
	RepresentationNotification = workers.RepresentationNotification

	FidelityLossless      = workers.FidelityLossless
	FidelityNormalized    = workers.FidelityNormalized
	FidelityLossy         = workers.FidelityLossy
	FidelityFinalOnly     = workers.FidelityFinalOnly
	FidelityLifecycleOnly = workers.FidelityLifecycleOnly

	ContentBlockText             = workers.ContentBlockText
	ContentBlockReasoningSummary = workers.ContentBlockReasoningSummary
	ContentBlockToolRequest      = workers.ContentBlockToolRequest
	ContentBlockImageRef         = workers.ContentBlockImageRef
	ContentBlockResourceRef      = workers.ContentBlockResourceRef
	ContentBlockStructuredOutput = workers.ContentBlockStructuredOutput
)

var CloneDraft = workers.CloneDraft
var IsAuthoritativeMessageSnapshot = workers.IsAuthoritativeMessageSnapshot

// FactoryResponseEvent is the Factory Session-owned published envelope.
type FactoryResponseEvent struct {
	DispatchID         string          `json:"dispatchId,omitempty"`
	EventID            string          `json:"eventId"`
	FactorySessionID   string          `json:"factorySessionId"`
	ItemID             string          `json:"itemId,omitempty"`
	Kind               Kind            `json:"kind"`
	ParentItemID       string          `json:"parentItemId,omitempty"`
	Payload            json.RawMessage `json:"payload"`
	Phase              Phase           `json:"phase"`
	Provenance         Provenance      `json:"provenance"`
	ProviderSessionRef string          `json:"providerSessionRef,omitempty"`
	RecordedAt         time.Time       `json:"recordedAt"`
	RunID              string          `json:"runId"`
	SchemaVersion      string          `json:"schemaVersion"`
	Sequence           int64           `json:"sequence"`
	TurnID             string          `json:"turnId,omitempty"`
}
