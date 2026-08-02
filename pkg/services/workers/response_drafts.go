package workers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Kind string

const (
	KindSession    Kind = "SESSION"
	KindRun        Kind = "RUN"
	KindTurn       Kind = "TURN"
	KindMessage    Kind = "MESSAGE"
	KindReasoning  Kind = "REASONING"
	KindTool       Kind = "TOOL"
	KindFileChange Kind = "FILE_CHANGE"
	KindPlan       Kind = "PLAN"
	KindProgress   Kind = "PROGRESS"
	KindUsage      Kind = "USAGE"
	KindError      Kind = "ERROR"
	KindStreamGap  Kind = "STREAM_GAP"
)

// ErrUnknownDraftKind is the sentinel matched via errors.Is when a Kind value
// is the zero value or is not one of the twelve declared response draft
// kinds.
var ErrUnknownDraftKind = errors.New("workers: unknown draft kind")

// InvalidDraftKindError carries the invalid Kind value that failed Validate.
type InvalidDraftKindError struct {
	Kind Kind
}

func (e *InvalidDraftKindError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("draft kind %q is not a declared response draft kind", e.Kind)
}

func (e *InvalidDraftKindError) Unwrap() error {
	return ErrUnknownDraftKind
}

// Validate reports whether k is exactly one of the twelve declared response
// draft kinds. The zero value and any unknown value return a typed error
// matched via errors.Is(err, ErrUnknownDraftKind).
func (k Kind) Validate() error {
	switch k {
	case KindSession, KindRun, KindTurn, KindMessage, KindReasoning, KindTool,
		KindFileChange, KindPlan, KindProgress, KindUsage, KindError, KindStreamGap:
		return nil
	default:
		return &InvalidDraftKindError{Kind: k}
	}
}

type Phase string

const (
	PhaseStarted   Phase = "STARTED"
	PhaseDelta     Phase = "DELTA"
	PhaseUpdated   Phase = "UPDATED"
	PhaseCompleted Phase = "COMPLETED"
	PhaseFailed    Phase = "FAILED"
	PhaseCanceled  Phase = "CANCELED"
)

// ErrUnknownDraftPhase is the sentinel matched via errors.Is when a Phase
// value is the zero value or is not one of the six declared response draft
// phases.
var ErrUnknownDraftPhase = errors.New("workers: unknown draft phase")

// InvalidDraftPhaseError carries the invalid Phase value that failed
// Validate.
type InvalidDraftPhaseError struct {
	Phase Phase
}

func (e *InvalidDraftPhaseError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("draft phase %q is not a declared response draft phase", e.Phase)
}

func (e *InvalidDraftPhaseError) Unwrap() error {
	return ErrUnknownDraftPhase
}

// Validate reports whether p is exactly one of the six declared response
// draft phases. The zero value and any unknown value return a typed error
// matched via errors.Is(err, ErrUnknownDraftPhase).
func (p Phase) Validate() error {
	switch p {
	case PhaseStarted, PhaseDelta, PhaseUpdated, PhaseCompleted, PhaseFailed, PhaseCanceled:
		return nil
	default:
		return &InvalidDraftPhaseError{Phase: p}
	}
}

type Delivery string

const (
	DeliveryNativeStream Delivery = "NATIVE_STREAM"
	DeliveryNativeFinal  Delivery = "NATIVE_FINAL"
	DeliverySynthesized  Delivery = "SYNTHESIZED"
	DeliveryReplay       Delivery = "REPLAY"
)

type Representation string

const (
	RepresentationDelta        Representation = "DELTA"
	RepresentationSnapshot     Representation = "SNAPSHOT"
	RepresentationNotification Representation = "NOTIFICATION"
)

type Fidelity string

const (
	FidelityLossless      Fidelity = "LOSSLESS"
	FidelityNormalized    Fidelity = "NORMALIZED"
	FidelityLossy         Fidelity = "LOSSY"
	FidelityFinalOnly     Fidelity = "FINAL_ONLY"
	FidelityLifecycleOnly Fidelity = "LIFECYCLE_ONLY"
)

type Provenance struct {
	Delivery           Delivery       `json:"delivery"`
	Fidelity           Fidelity       `json:"fidelity"`
	NativeEventSubtype string         `json:"nativeEventSubtype,omitempty"`
	NativeEventType    string         `json:"nativeEventType"`
	Provider           string         `json:"provider"`
	Representation     Representation `json:"representation"`
}

type Capabilities struct {
	NativeStreaming    bool `json:"nativeStreaming"`
	MessageDeltas      bool `json:"messageDeltas"`
	MessageSnapshots   bool `json:"messageSnapshots"`
	ReasoningSummaries bool `json:"reasoningSummaries"`
	ToolLifecycle      bool `json:"toolLifecycle"`
	ToolOutputDeltas   bool `json:"toolOutputDeltas"`
	FileChanges        bool `json:"fileChanges"`
	Plans              bool `json:"plans"`
	Usage              bool `json:"usage"`
	StableItemIDs      bool `json:"stableItemIds"`
	ProviderReconnect  bool `json:"providerReconnect"`
}

type Draft struct {
	RunID              string          `json:"runId,omitempty"`
	Kind               Kind            `json:"kind"`
	Phase              Phase           `json:"phase"`
	Provenance         Provenance      `json:"provenance"`
	Payload            json.RawMessage `json:"payload"`
	DispatchID         string          `json:"dispatchId,omitempty"`
	TurnID             string          `json:"turnId,omitempty"`
	ItemID             string          `json:"itemId,omitempty"`
	ParentItemID       string          `json:"parentItemId,omitempty"`
	ProviderSessionRef string          `json:"providerSessionRef,omitempty"`
}

// CloneDraft returns a draft whose mutable payload bytes are independent from
// the source so publication cannot mutate adapter-owned state.
func CloneDraft(draft Draft) Draft {
	cloned := draft
	cloned.Payload = append([]byte(nil), draft.Payload...)
	return cloned
}

type ContentBlockKind string

const (
	ContentBlockText             ContentBlockKind = "TEXT"
	ContentBlockReasoningSummary ContentBlockKind = "REASONING_SUMMARY"
	ContentBlockToolRequest      ContentBlockKind = "TOOL_REQUEST"
	ContentBlockImageRef         ContentBlockKind = "IMAGE_REF"
	ContentBlockResourceRef      ContentBlockKind = "RESOURCE_REF"
	ContentBlockStructuredOutput ContentBlockKind = "STRUCTURED_OUTPUT"
)

type ContentBlock struct {
	Kind             ContentBlockKind `json:"kind"`
	Text             string           `json:"text,omitempty"`
	ToolCallID       string           `json:"toolCallId,omitempty"`
	ToolName         string           `json:"toolName,omitempty"`
	ArgumentsSummary json.RawMessage  `json:"argumentsSummary,omitempty"`
	ImageRef         string           `json:"imageRef,omitempty"`
	ResourceRef      string           `json:"resourceRef,omitempty"`
	StructuredOutput json.RawMessage  `json:"structuredOutput,omitempty"`
}

type SessionPayload struct {
	Status       string        `json:"status,omitempty"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}
type RunPayload struct {
	Status string `json:"status,omitempty"`
}
type TurnPayload struct {
	TurnIndex int    `json:"turnIndex,omitempty"`
	Status    string `json:"status,omitempty"`
}
type MessagePayload struct {
	Role          string         `json:"role"`
	ContentBlocks []ContentBlock `json:"contentBlocks"`
	// Partial marks timeout- or cancellation-captured assistant snapshots that
	// must not be treated as authoritative final responses.
	Partial bool `json:"partial,omitempty"`
}

// IsAuthoritativeMessageSnapshot reports whether a completed MESSAGE snapshot
// may be treated as an authoritative final response for invocation primary-result
// selection or final-only provider parsing. Timeout- or cancellation-captured
// snapshots with partial=true must return false.
func IsAuthoritativeMessageSnapshot(payload MessagePayload) bool {
	return !payload.Partial
}

type MessageDeltaPayload struct {
	ContentBlockIndex int              `json:"contentBlockIndex"`
	ContentBlockKind  ContentBlockKind `json:"contentBlockKind"`
	TextDelta         string           `json:"textDelta,omitempty"`
}
type ReasoningPayload struct {
	Summary      string `json:"summary,omitempty"`
	SummaryDelta string `json:"summaryDelta,omitempty"`
}
type ToolPayload struct {
	ToolCallID       string          `json:"toolCallId"`
	ToolName         string          `json:"toolName"`
	Status           string          `json:"status,omitempty"`
	ArgumentsSummary json.RawMessage `json:"argumentsSummary,omitempty"`
	ResultSummary    json.RawMessage `json:"resultSummary,omitempty"`
}
type ToolDeltaPayload struct {
	ToolCallID  string `json:"toolCallId"`
	OutputDelta string `json:"outputDelta"`
}
type FileChangePayload struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Summary   string `json:"summary,omitempty"`
}
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status,omitempty"`
}
type PlanPayload struct {
	Steps   []PlanStep `json:"steps,omitempty"`
	Summary string     `json:"summary,omitempty"`
}
type ProgressPayload struct {
	Label           string   `json:"label"`
	Message         string   `json:"message,omitempty"`
	PercentComplete *float64 `json:"percentComplete,omitempty"`
}
type UsagePayload struct {
	InputTokens           int64  `json:"inputTokens,omitempty"`
	CachedInputTokens     int64  `json:"cachedInputTokens,omitempty"`
	OutputTokens          int64  `json:"outputTokens,omitempty"`
	ReasoningOutputTokens int64  `json:"reasoningOutputTokens,omitempty"`
	TotalTokens           int64  `json:"totalTokens,omitempty"`
	Model                 string `json:"model,omitempty"`
}
type ErrorPayload struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable,omitempty"`
	RetryAfterSeconds *int64 `json:"retryAfterSeconds,omitempty"`
	RetryAttempt      *int   `json:"retryAttempt,omitempty"`
}
type StreamGapPayload struct {
	FromSequence           int64  `json:"fromSequence,omitempty"`
	ToSequence             int64  `json:"toSequence,omitempty"`
	FirstAvailableSequence int64  `json:"firstAvailableSequence,omitempty"`
	AffectedItemID         string `json:"affectedItemId,omitempty"`
	ToolCallID             string `json:"toolCallId,omitempty"`
	Reason                 string `json:"reason,omitempty"`
}

// MarshalJSON preserves the two exclusive public gap shapes. Retention
// sequence fields remain present even when a valid bound is zero, while item
// gaps never acquire synthetic zero-valued retention fields.
func (p StreamGapPayload) MarshalJSON() ([]byte, error) {
	if strings.TrimSpace(p.AffectedItemID) != "" {
		return json.Marshal(struct {
			AffectedItemID string `json:"affectedItemId"`
			ToolCallID     string `json:"toolCallId,omitempty"`
			Reason         string `json:"reason"`
		}{
			AffectedItemID: p.AffectedItemID,
			ToolCallID:     p.ToolCallID,
			Reason:         p.Reason,
		})
	}
	return json.Marshal(struct {
		FromSequence           int64  `json:"fromSequence"`
		ToSequence             int64  `json:"toSequence"`
		FirstAvailableSequence int64  `json:"firstAvailableSequence"`
		Reason                 string `json:"reason,omitempty"`
	}{
		FromSequence:           p.FromSequence,
		ToSequence:             p.ToSequence,
		FirstAvailableSequence: p.FirstAvailableSequence,
		Reason:                 p.Reason,
	})
}
