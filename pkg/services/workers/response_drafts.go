package workers

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
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

type Phase string

const (
	PhaseStarted   Phase = "STARTED"
	PhaseDelta     Phase = "DELTA"
	PhaseUpdated   Phase = "UPDATED"
	PhaseCompleted Phase = "COMPLETED"
	PhaseFailed    Phase = "FAILED"
	PhaseCanceled  Phase = "CANCELED"
)

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

// AttemptReason identifies why an attempt lifecycle record was created.
// Worker Sessions emits INITIAL for the first record of a new session, RETRY
// for a subsequent provider attempt, and RESUME for a caller-started exact
// provider continuation.
type AttemptReason string

const (
	AttemptReasonInitial AttemptReason = "INITIAL"
	AttemptReasonRetry   AttemptReason = "RETRY"
	AttemptReasonResume  AttemptReason = "RESUME"
)

// SessionProviderSelection carries only provider-selection facts explicitly
// resolved by the caller. Empty selection facts are omitted from the opening
// rather than replaced with a default provider or runner.
type SessionProviderSelection struct {
	RunnerID         string                `json:"runnerId,omitempty"`
	Source           RunnerSelectionSource `json:"source,omitempty"`
	ExecutorProvider string                `json:"executorProvider,omitempty"`
	ModelProvider    string                `json:"modelProvider,omitempty"`
}

// SessionContinuation is the detached exact continuation identity supplied to
// a resumed execution. It is not synthesized from model, runner, or provider
// defaults; a nil value means no continuation reference was supplied.
type SessionContinuation struct {
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
}

// SessionLineage carries the explicit relationship between one Worker
// Session attempt and the attempt or Worker Session that preceded or follows
// it. The current dispatch and attempt identities remain the surrounding
// SessionPayload.DispatchID and AttemptID fields; keeping the prior identity
// here makes a durable record self-describing without replay-time inference.
type SessionLineage struct {
	PredecessorWorkerSessionID string `json:"predecessorWorkerSessionId,omitempty"`
	SuccessorWorkerSessionID   string `json:"successorWorkerSessionId,omitempty"`
	PreviousDispatchID         string `json:"previousDispatchId,omitempty"`
	PreviousAttemptID          string `json:"previousAttemptId,omitempty"`
}

var (
	// ErrInvalidSessionContinuation reports a continuation payload that does
	// not carry the complete exact provider/kind/opaque-ID tuple.
	ErrInvalidSessionContinuation = errors.New("workers: invalid session continuation")
	// ErrInvalidSessionLineage reports missing, self-referential, or
	// contradictory durable Worker Session lineage.
	ErrInvalidSessionLineage = errors.New("workers: invalid session lineage")
)

// Validate reports whether value is a complete exact continuation tuple. It
// deliberately checks without trimming or replacing any field, so callers can
// retain the authoritative Providers reference byte-for-byte.
func (value SessionContinuation) Validate() error {
	if strings.TrimSpace(value.Provider) == "" || strings.TrimSpace(value.Kind) == "" || strings.TrimSpace(value.ID) == "" {
		return ErrInvalidSessionContinuation
	}
	return nil
}

// Validate reports whether value is a coherent relationship for the current
// Worker Session and attempt. Empty lineage is valid for an initial opening;
// once a relationship is present, both sides of every attempt correlation
// must be explicit and the relationship may not point back to itself.
func (value SessionLineage) Validate(currentWorkerSessionID, currentDispatchID, currentAttemptID string) error {
	if value.PredecessorWorkerSessionID == "" && value.SuccessorWorkerSessionID == "" &&
		value.PreviousDispatchID == "" && value.PreviousAttemptID == "" {
		return ErrInvalidSessionLineage
	}
	if err := validateLineageSessionID(value.PredecessorWorkerSessionID, currentWorkerSessionID); err != nil {
		return err
	}
	if err := validateLineageSessionID(value.SuccessorWorkerSessionID, currentWorkerSessionID); err != nil {
		return err
	}
	if value.PredecessorWorkerSessionID != "" && value.PredecessorWorkerSessionID == value.SuccessorWorkerSessionID {
		return ErrInvalidSessionLineage
	}
	if err := validateLineageAttemptIDs(value.PreviousDispatchID, value.PreviousAttemptID); err != nil {
		return err
	}
	if err := rejectCurrentLineageAttempt(value.PreviousDispatchID, currentDispatchID); err != nil {
		return err
	}
	return rejectCurrentLineageAttempt(value.PreviousAttemptID, currentAttemptID)
}

func validateLineageSessionID(value, current string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value || value == current {
		return ErrInvalidSessionLineage
	}
	return nil
}

func validateLineageAttemptIDs(dispatchID, attemptID string) error {
	if strings.TrimSpace(dispatchID) != dispatchID || strings.TrimSpace(attemptID) != attemptID {
		return ErrInvalidSessionLineage
	}
	if (dispatchID == "") != (attemptID == "") {
		return ErrInvalidSessionLineage
	}
	if dispatchID != "" && dispatchID != attemptID {
		return ErrInvalidSessionLineage
	}
	return nil
}

func rejectCurrentLineageAttempt(previous, current string) error {
	if previous != "" && current != "" && previous == current {
		return ErrInvalidSessionLineage
	}
	return nil
}

// Clone returns a detached lineage value.
func (value SessionLineage) Clone() SessionLineage {
	return value
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
	Status            string                    `json:"status,omitempty"`
	StartedAt         *time.Time                `json:"startedAt,omitempty"`
	WorkerSessionID   string                    `json:"workerSessionId,omitempty"`
	WorkerType        string                    `json:"workerType,omitempty"`
	FactorySessionID  string                    `json:"factorySessionId,omitempty"`
	RecordingID       string                    `json:"recordingId,omitempty"`
	ProjectID         string                    `json:"projectId,omitempty"`
	DispatchID        string                    `json:"dispatchId,omitempty"`
	TransitionID      string                    `json:"transitionId,omitempty"`
	WorkstationName   string                    `json:"workstationName,omitempty"`
	TurnID            string                    `json:"turnId,omitempty"`
	TraceID           string                    `json:"traceId,omitempty"`
	ReplayKey         string                    `json:"replayKey,omitempty"`
	WorkIDs           []string                  `json:"workIds,omitempty"`
	AttemptID         string                    `json:"attemptId,omitempty"`
	Attempt           int                       `json:"attempt,omitempty"`
	AttemptReason     AttemptReason             `json:"attemptReason,omitempty"`
	Continuation      *SessionContinuation      `json:"continuation,omitempty"`
	Lineage           *SessionLineage           `json:"lineage,omitempty"`
	ProviderSelection *SessionProviderSelection `json:"providerSelection,omitempty"`
	Model             string                    `json:"model,omitempty"`
	ReasoningEffort   string                    `json:"reasoningEffort,omitempty"`
	// WorkingDirectory is the resolved execution directory retained for public
	// Worker Session stream correlation.
	WorkingDirectory string        `json:"workingDirectory,omitempty"`
	Capabilities     *Capabilities `json:"capabilities,omitempty"`
	// Title carries a mid-lifecycle Chat Session display-title change (only
	// meaningful with Phase == PhaseUpdated; lifecycle phases leave it nil).
	// A nil Title declares no title change, matching acp-go-sdk's own
	// "set to null to clear" nullable-Title convention for session_info_update.
	Title *string `json:"title,omitempty"`
}

// ValidateLineage checks the optional durable continuation facts on a Session
// payload. Legacy initial and terminal payloads with no lineage remain valid;
// any explicit continuation or lineage is checked rather than normalized.
func (payload SessionPayload) ValidateLineage() error {
	if err := payload.validateLineageValues(); err != nil {
		return err
	}
	return validateAttemptReasonLineage(payload)
}

func (payload SessionPayload) validateLineageValues() error {
	if payload.Continuation != nil {
		if err := payload.Continuation.Validate(); err != nil {
			return err
		}
	}
	if payload.Lineage == nil {
		return nil
	}
	if strings.TrimSpace(payload.WorkerSessionID) == "" ||
		strings.TrimSpace(payload.DispatchID) == "" ||
		strings.TrimSpace(payload.AttemptID) == "" {
		return ErrInvalidSessionLineage
	}
	return payload.Lineage.Validate(payload.WorkerSessionID, payload.DispatchID, payload.AttemptID)
}

func validateAttemptReasonLineage(payload SessionPayload) error {
	switch payload.AttemptReason {
	case "":
		return validateUnattributedLineage(payload.Lineage)
	case AttemptReasonInitial:
		return validateInitialLineage(payload)
	case AttemptReasonRetry:
		return validateRetryLineage(payload)
	case AttemptReasonResume:
		return validateResumeLineage(payload)
	default:
		return ErrInvalidSessionLineage
	}
}

func validateUnattributedLineage(lineage *SessionLineage) error {
	if lineage != nil && (lineage.PredecessorWorkerSessionID != "" || lineage.PreviousDispatchID != "") {
		return ErrInvalidSessionLineage
	}
	return nil
}

func validateInitialLineage(payload SessionPayload) error {
	if payload.Continuation != nil || payload.Lineage != nil {
		return ErrInvalidSessionLineage
	}
	return nil
}

func validateRetryLineage(payload SessionPayload) error {
	if payload.Lineage == nil || payload.Lineage.PreviousDispatchID == "" || payload.Continuation != nil ||
		payload.Lineage.PredecessorWorkerSessionID != "" || payload.Lineage.SuccessorWorkerSessionID != "" {
		return ErrInvalidSessionLineage
	}
	return nil
}

func validateResumeLineage(payload SessionPayload) error {
	if payload.Continuation == nil || payload.Lineage == nil || payload.Lineage.PreviousDispatchID == "" {
		return ErrInvalidSessionLineage
	}
	return nil
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
