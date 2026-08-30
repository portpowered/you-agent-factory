package factorycontracts

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// FactoryEventSchemaVersion identifies the serialized canonical event envelope.
// The value is owned by Factory even though OpenAPI publishes the same wire value.
type FactoryEventSchemaVersion string

const FactoryEventSchemaVersionV1 FactoryEventSchemaVersion = "agent-factory.event.v1"

// FactoryEventType is the canonical Factory event vocabulary. Transport
// adapters map these stable values to generated OpenAPI enums at the edge.
type FactoryEventType string

const (
	FactoryEventTypeAgentRunResponse              FactoryEventType = "AGENT_RUN_RESPONSE"
	FactoryEventTypeArtifactCreated               FactoryEventType = "ARTIFACT_CREATED"
	FactoryEventTypeDispatchInterrupted           FactoryEventType = "DISPATCH_INTERRUPTED"
	FactoryEventTypeDispatchQueued                FactoryEventType = "DISPATCH_QUEUED"
	FactoryEventTypeDispatchReconciled            FactoryEventType = "DISPATCH_RECONCILED"
	FactoryEventTypeDispatchRequest               FactoryEventType = "DISPATCH_REQUEST"
	FactoryEventTypeDispatchResponse              FactoryEventType = "DISPATCH_RESPONSE"
	FactoryEventTypeDispatchResultIgnored         FactoryEventType = "DISPATCH_RESULT_IGNORED"
	FactoryEventTypeDispatchWorkerSessionAssoc    FactoryEventType = "DISPATCH_WORKER_SESSION_ASSOCIATION"
	FactoryEventTypeHumanApprovalRequested        FactoryEventType = "HUMAN_APPROVAL_REQUESTED"
	FactoryEventTypeFactoryChange                 FactoryEventType = "FACTORY_CHANGE"
	FactoryEventTypeFactoryChangeRequest          FactoryEventType = "FACTORY_CHANGE_REQUEST"
	FactoryEventTypeFactoryChangeFailed           FactoryEventType = "FACTORY_CHANGE_FAILED"
	FactoryEventTypeFactoryStateResponse          FactoryEventType = "FACTORY_STATE_RESPONSE"
	FactoryEventTypeInferenceRequest              FactoryEventType = "INFERENCE_REQUEST"
	FactoryEventTypeInferenceResponse             FactoryEventType = "INFERENCE_RESPONSE"
	FactoryEventTypeInitialStructureRequest       FactoryEventType = "INITIAL_STRUCTURE_REQUEST"
	FactoryEventTypeJavaScriptCheckpointRef       FactoryEventType = "JAVASCRIPT_CHECKPOINT_REF"
	FactoryEventTypeJavaScriptPhaseChange         FactoryEventType = "JAVASCRIPT_PHASE_CHANGE"
	FactoryEventTypeModelRequest                  FactoryEventType = "MODEL_REQUEST"
	FactoryEventTypeModelResponse                 FactoryEventType = "MODEL_RESPONSE"
	FactoryEventTypeOrchestratorCheckpointWritten FactoryEventType = "ORCHESTRATOR_CHECKPOINT_WRITTEN"
	FactoryEventTypeOrchestratorPhaseChanged      FactoryEventType = "ORCHESTRATOR_PHASE_CHANGED"
	FactoryEventTypeRelationshipChangeRequest     FactoryEventType = "RELATIONSHIP_CHANGE_REQUEST"
	FactoryEventTypeRunRequest                    FactoryEventType = "RUN_REQUEST"
	FactoryEventTypeRunResponse                   FactoryEventType = "RUN_RESPONSE"
	FactoryEventTypeScriptRequest                 FactoryEventType = "SCRIPT_REQUEST"
	FactoryEventTypeScriptResponse                FactoryEventType = "SCRIPT_RESPONSE"
	FactoryEventTypeSessionCompleted              FactoryEventType = "SESSION_COMPLETED"
	FactoryEventTypeSessionLifecycleControl       FactoryEventType = "SESSION_LIFECYCLE_CONTROL"
	FactoryEventTypeSessionPaused                 FactoryEventType = "SESSION_PAUSED"
	FactoryEventTypeSessionResultUpdated          FactoryEventType = "SESSION_RESULT_UPDATED"
	FactoryEventTypeSessionResumed                FactoryEventType = "SESSION_RESUMED"
	FactoryEventTypeSessionStarted                FactoryEventType = "SESSION_STARTED"
	FactoryEventTypeWorkRequest                   FactoryEventType = "WORK_REQUEST"
	FactoryEventTypeWorkStateChange               FactoryEventType = "WORK_STATE_CHANGE"
)

// DispatchResultIgnoredReasonWorkAlreadyTerminal is the stable reason used
// when a correlated result arrives after an operator or another dispatch has
// already placed Work in a terminal or failed state.
const DispatchResultIgnoredReasonWorkAlreadyTerminal = "WORK_ALREADY_TERMINAL"

// FactoryEventReconnectCursor identifies the last acknowledged event for stream
// reconnect. Clients may supply AfterEventID or AfterSequence; when both are
// set, AfterEventID wins.
type FactoryEventReconnectCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// FactorySessionLogicalResolveHint carries persisted logical-session identity
// used to remap an unknown factorySessionID to the current live session.
type FactorySessionLogicalResolveHint struct {
	BackendScopeID      string
	LogicalSessionKeyID string
}

// FactorySessionSyncPreflightOptions carries reconnect and logical identity hints
// for session sync preflight resolution.
type FactorySessionSyncPreflightOptions struct {
	Reconnect           *FactoryEventReconnectCursor
	BackendScopeID      *string
	LogicalSessionKeyID *string
}

// FactoryEventReconnectScope configures how reconnect cursors are interpreted
// and how a domain stream is scoped at the canonical ledger boundary.
type FactoryEventReconnectScope struct {
	// SessionID enables sessionSequence-based after_sequence matching for
	// session-scoped event streams.
	SessionID string
	// DispatchID limits a Worker Session stream to events for one dispatch.
	DispatchID string
	// Limit bounds the live subscription buffer. Zero uses the ledger default
	// buffer size.
	Limit int
	// HistoryLimit caps the retained history returned before live delivery.
	// Zero drains all retained history. Durable consumers that need a bounded
	// replay must opt into that bound without changing live backpressure.
	HistoryLimit int
}

// FactoryEventStream carries replayed history and then live canonical events.
type FactoryEventStream struct {
	BackendScopeID      string
	LogicalSessionKeyID string
	FactorySessionID    string
	StreamGenerationID  string
	History             []FactoryEvent
	Events              <-chan FactoryEvent
}

// FactoryEvent is the canonical, transport-independent event envelope. Payload
// interpretation belongs to the Factory reducer that handles Type; transports
// may decode the detached JSON at their boundary when a generated union is
// required.
type FactoryEvent struct {
	Context       FactoryEventContext       `json:"context"`
	Id            string                    `json:"id"`
	Payload       json.RawMessage           `json:"payload"`
	SchemaVersion FactoryEventSchemaVersion `json:"schemaVersion"`
	Type          FactoryEventType          `json:"type"`
}

// FactoryEventContext carries canonical ordering and correlation metadata.
type FactoryEventContext struct {
	CheckpointID             *string   `json:"checkpointId,omitempty"`
	CurrentChainingTraceID   *string   `json:"currentChainingTraceId,omitempty"`
	DispatchID               *string   `json:"dispatchId,omitempty"`
	EventTime                time.Time `json:"eventTime"`
	OrchestratorDialect      *string   `json:"orchestratorDialect,omitempty"`
	OrchestratorKind         *string   `json:"orchestratorKind,omitempty"`
	PhaseID                  *string   `json:"phaseId,omitempty"`
	PhaseName                *string   `json:"phaseName,omitempty"`
	PreviousChainingTraceIDs *[]string `json:"previousChainingTraceIds,omitempty"`
	RequestID                *string   `json:"requestId,omitempty"`
	Sequence                 int       `json:"sequence"`
	SessionID                *string   `json:"sessionId,omitempty"`
	SessionSequence          *int      `json:"sessionSequence,omitempty"`
	Source                   *string   `json:"source,omitempty"`
	Tick                     int       `json:"tick"`
	TraceIDs                 *[]string `json:"traceIds,omitempty"`
	WorkIDs                  *[]string `json:"workIds,omitempty"`
}

// OrchestratorPhaseStatus is the canonical lifecycle status for one
// orchestrator phase recorded in Factory event history.
type OrchestratorPhaseStatus string

const (
	OrchestratorPhaseStatusActive    OrchestratorPhaseStatus = "ACTIVE"
	OrchestratorPhaseStatusCompleted OrchestratorPhaseStatus = "COMPLETED"
	OrchestratorPhaseStatusSkipped   OrchestratorPhaseStatus = "SKIPPED"
)

// CheckpointResumabilityStatus describes whether an orchestrator checkpoint
// can resume Factory Session execution.
type CheckpointResumabilityStatus string

const (
	CheckpointResumabilityStatusResumable    CheckpointResumabilityStatus = "RESUMABLE"
	CheckpointResumabilityStatusNotResumable CheckpointResumabilityStatus = "NOT_RESUMABLE"
	CheckpointResumabilityStatusUnknown      CheckpointResumabilityStatus = "UNKNOWN"
)

// FactoryArtifactRef identifies one session-owned artifact from a canonical
// Factory event without exposing its storage implementation.
type FactoryArtifactRef struct {
	ContentHash *string `json:"contentHash,omitempty"`
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	SizeBytes   *int64  `json:"sizeBytes,omitempty"`
	Visibility  string  `json:"visibility"`
}

// FactoryDispatchWarning is one customer-visible warning retained by a
// canonical Factory event.
type FactoryDispatchWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// FactoryDispatchKind identifies the execution family of one Factory dispatch.
type FactoryDispatchKind string

const (
	FactoryDispatchKindJavaScriptAgent      FactoryDispatchKind = "JAVASCRIPT_AGENT"
	FactoryDispatchKindJavaScriptScript     FactoryDispatchKind = "JAVASCRIPT_SCRIPT"
	FactoryDispatchKindJavaScriptSynthesize FactoryDispatchKind = "JAVASCRIPT_SYNTHESIZE"
	FactoryDispatchKindJavaScriptSystem     FactoryDispatchKind = "JAVASCRIPT_SYSTEM"
	FactoryDispatchKindJavaScriptTool       FactoryDispatchKind = "JAVASCRIPT_TOOL"
	FactoryDispatchKindJavaScriptVerify     FactoryDispatchKind = "JAVASCRIPT_VERIFY"
	FactoryDispatchKindPetriTransition      FactoryDispatchKind = "PETRI_TRANSITION"
)

// FactoryDispatchStatus is the canonical lifecycle status recorded for a
// dispatch on the Factory event stream.
type FactoryDispatchStatus string

const (
	FactoryDispatchStatusCompleted   FactoryDispatchStatus = "COMPLETED"
	FactoryDispatchStatusFailed      FactoryDispatchStatus = "FAILED"
	FactoryDispatchStatusInterrupted FactoryDispatchStatus = "INTERRUPTED"
	FactoryDispatchStatusQueued      FactoryDispatchStatus = "QUEUED"
	FactoryDispatchStatusRunning     FactoryDispatchStatus = "RUNNING"
)

// DispatchReconciliationSource identifies the authority that supplied a
// durable dispatch reconciliation fact.
type DispatchReconciliationSource string

const (
	DispatchReconciliationSourceDurableState      DispatchReconciliationSource = "DURABLE_STATE"
	DispatchReconciliationSourceProviderSession   DispatchReconciliationSource = "PROVIDER_SESSION"
	DispatchReconciliationSourceRuntimeReconciler DispatchReconciliationSource = "RUNTIME_RECONCILER"
	DispatchReconciliationSourceStreamReplay      DispatchReconciliationSource = "STREAM_REPLAY"
)

// FactoryDispatchUsage retains optional provider usage exactly as recorded.
type FactoryDispatchUsage struct {
	CostUSD        *float64 `json:"costUsd,omitempty"`
	DurationMillis *int64   `json:"durationMillis,omitempty"`
	InputTokens    *int64   `json:"inputTokens,omitempty"`
	OutputTokens   *int64   `json:"outputTokens,omitempty"`
	RetryCount     *int32   `json:"retryCount,omitempty"`
	TotalTokens    *int64   `json:"totalTokens,omitempty"`
}

// FactoryArtifact is the customer-visible metadata for a session-owned
// artifact. Artifact bodies and storage remain outside the Factory event.
type FactoryArtifact struct {
	AuditMode       *string                         `json:"auditMode,omitempty"`
	CaptureMetadata *FactoryArtifactCaptureMetadata `json:"captureMetadata,omitempty"`
	ContentHash     *string                         `json:"contentHash,omitempty"`
	ID              string                          `json:"id"`
	Kind            string                          `json:"kind"`
	Label           *string                         `json:"label,omitempty"`
	RedactionCounts *FactoryArtifactRedactionCounts `json:"redactionCounts,omitempty"`
	SizeBytes       *int64                          `json:"sizeBytes,omitempty"`
	Summary         *string                         `json:"summary,omitempty"`
	Visibility      string                          `json:"visibility"`
}

// FactoryArtifactCaptureMetadata records how an artifact was captured.
type FactoryArtifactCaptureMetadata struct {
	CapturedAt       *time.Time `json:"capturedAt,omitempty"`
	MIMEType         *string    `json:"mimeType,omitempty"`
	SourceDispatchID *string    `json:"sourceDispatchId,omitempty"`
}

// FactoryArtifactRedactionCounts records bounded redaction totals.
type FactoryArtifactRedactionCounts struct {
	Paths   *int32 `json:"paths,omitempty"`
	Secrets *int32 `json:"secrets,omitempty"`
	Tokens  *int32 `json:"tokens,omitempty"`
}

// FactorySessionJavaScriptCheckpointEventRef retains the customer-visible
// checkpoint reference carried by an interruption event.
type FactorySessionJavaScriptCheckpointEventRef struct {
	ArtifactRef *FactoryArtifactRef `json:"artifactRef,omitempty"`
	ID          string              `json:"id"`
	Label       *string             `json:"label,omitempty"`
	Summary     *string             `json:"summary,omitempty"`
	Timestamp   *time.Time          `json:"timestamp,omitempty"`
}

// DispatchQueuedEventPayload records a dispatch awaiting execution.
type DispatchQueuedEventPayload struct {
	CoordinationRef   *string             `json:"coordinationRef,omitempty"`
	DispatchKind      FactoryDispatchKind `json:"dispatchKind"`
	InputArtifactIDs  *[]string           `json:"inputArtifactIds,omitempty"`
	InputWorkIDs      *[]string           `json:"inputWorkIds,omitempty"`
	Label             *string             `json:"label,omitempty"`
	Model             *string             `json:"model,omitempty"`
	ModelProvider     *string             `json:"modelProvider,omitempty"`
	ParentDispatchID  *string             `json:"parentDispatchId,omitempty"`
	PresetID          *string             `json:"presetId,omitempty"`
	PromptDigest      *string             `json:"promptDigest,omitempty"`
	Provider          *string             `json:"provider,omitempty"`
	QueuePosition     *int                `json:"queuePosition,omitempty"`
	ReasoningEffort   *string             `json:"reasoningEffort,omitempty"`
	RetryOfDispatchID *string             `json:"retryOfDispatchId,omitempty"`
	RunnerID          *string             `json:"runnerId,omitempty"`
	SchemaDigest      *string             `json:"schemaDigest,omitempty"`
	SkipPermissions   *bool               `json:"skipPermissions,omitempty"`
}

// DispatchInterruptedEventPayload records an observed interruption.
type DispatchInterruptedEventPayload struct {
	CheckpointRef      *FactorySessionJavaScriptCheckpointEventRef `json:"checkpointRef,omitempty"`
	InterruptedAt      time.Time                                   `json:"interruptedAt"`
	ObservedStatus     FactoryDispatchStatus                       `json:"observedStatus"`
	ProviderSessionRef *providers.SessionMetadata                  `json:"providerSessionRef,omitempty"`
	Reason             string                                      `json:"reason"`
	RetryPlanned       bool                                        `json:"retryPlanned"`
}

// DispatchReconciledEventPayload records a durable dispatch reconciliation.
type DispatchReconciledEventPayload struct {
	ArtifactIDs          *[]string                      `json:"artifactIds,omitempty"`
	FailureDetail        *workerexecution.FailureDetail `json:"failureDetail,omitempty"`
	ReconciledStatus     FactoryDispatchStatus          `json:"reconciledStatus"`
	ReconciliationSource DispatchReconciliationSource   `json:"reconciliationSource"`
	Replayed             bool                           `json:"replayed"`
	ResultArtifactRef    *FactoryArtifactRef            `json:"resultArtifactRef,omitempty"`
	Usage                *FactoryDispatchUsage          `json:"usage,omitempty"`
}

// DispatchResultIgnoredEventPayload records a stale dispatch result without
// retaining raw worker output, error text, or Work payload content. Dispatch
// and Work identity remain authoritative on FactoryEventContext.
type DispatchResultIgnoredEventPayload struct {
	Reason        string                      `json:"reason"`
	ResultOutcome workerexecution.WorkOutcome `json:"resultOutcome"`
	ObservedState ObservedWorkState           `json:"observedState"`
}

// ObservedWorkState is the authored name and lifecycle category observed when
// a stale dispatch result was retired.
type ObservedWorkState struct {
	Name string    `json:"name"`
	Type StateType `json:"type"`
}

// ArtifactCreatedEventPayload records customer-visible artifact metadata.
type ArtifactCreatedEventPayload struct {
	Artifact   FactoryArtifact `json:"artifact"`
	CapturedAt *time.Time      `json:"capturedAt,omitempty"`
}

// OrchestratorPhaseChangedEventPayload records a workflow phase transition.
// Current phase identity remains authoritative in FactoryEventContext.
type OrchestratorPhaseChangedEventPayload struct {
	CompletedAt       *time.Time              `json:"completedAt,omitempty"`
	PhaseStatus       OrchestratorPhaseStatus `json:"phaseStatus"`
	PreviousPhaseID   *string                 `json:"previousPhaseId,omitempty"`
	PreviousPhaseName *string                 `json:"previousPhaseName,omitempty"`
	ProgressSummary   *string                 `json:"progressSummary,omitempty"`
	StartedAt         *time.Time              `json:"startedAt,omitempty"`
}

// OrchestratorCheckpointWrittenEventPayload records a replay-safe checkpoint
// reference while raw checkpoint bodies remain orchestrator-owned.
type OrchestratorCheckpointWrittenEventPayload struct {
	ArtifactRef           *FactoryArtifactRef          `json:"artifactRef,omitempty"`
	Label                 string                       `json:"label"`
	ResumabilityStatus    CheckpointResumabilityStatus `json:"resumabilityStatus"`
	RuntimeSnapshotDigest *string                      `json:"runtimeSnapshotDigest,omitempty"`
	SourceHash            *string                      `json:"sourceHash,omitempty"`
	Timestamp             *time.Time                   `json:"timestamp,omitempty"`
	Warnings              []FactoryDispatchWarning     `json:"warnings,omitempty"`
}

// FactorySessionResultStatus describes customer-visible result availability
// retained by a canonical Factory Session lifecycle event.
type FactorySessionResultStatus string

const (
	FactorySessionResultStatusPartial           FactorySessionResultStatus = "PARTIAL"
	FactorySessionResultStatusFinal             FactorySessionResultStatus = "FINAL"
	FactorySessionResultStatusFailedWithPartial FactorySessionResultStatus = "FAILED_WITH_PARTIAL"
)

// FactorySessionLifecycleStatus is the durable status retained by canonical
// Factory Session lifecycle events.
type FactorySessionLifecycleStatus string

const (
	FactorySessionLifecycleStatusRunning   FactorySessionLifecycleStatus = "RUNNING"
	FactorySessionLifecycleStatusPaused    FactorySessionLifecycleStatus = "PAUSED"
	FactorySessionLifecycleStatusSucceeded FactorySessionLifecycleStatus = "SUCCEEDED"
	FactorySessionLifecycleStatusFailed    FactorySessionLifecycleStatus = "FAILED"
)

// FactorySessionLifecycleControlKind identifies a lifecycle operation recorded
// in canonical Factory history.
type FactorySessionLifecycleControlKind string

const (
	FactorySessionLifecycleControlPause  FactorySessionLifecycleControlKind = "PAUSE"
	FactorySessionLifecycleControlResume FactorySessionLifecycleControlKind = "RESUME"
)

// FactorySessionLifecycleControlOutcome describes the accepted control result
// retained by canonical Factory history.
type FactorySessionLifecycleControlOutcome string

const FactorySessionLifecycleControlOutcomeAccepted FactorySessionLifecycleControlOutcome = "ACCEPTED"

// FactorySessionChildDispatchCounts summarizes JavaScript child dispatch state.
type FactorySessionChildDispatchCounts struct {
	Completed int `json:"completed"`
	Queued    int `json:"queued"`
	Running   int `json:"running"`
}

// FactorySessionJavaScriptScriptStatus describes the observable state of a
// JavaScript orchestrator script within one Factory Session.
type FactorySessionJavaScriptScriptStatus string

const (
	FactorySessionJavaScriptScriptStatusFailed   FactorySessionJavaScriptScriptStatus = "FAILED"
	FactorySessionJavaScriptScriptStatusFinished FactorySessionJavaScriptScriptStatus = "FINISHED"
	FactorySessionJavaScriptScriptStatusIdle     FactorySessionJavaScriptScriptStatus = "IDLE"
	FactorySessionJavaScriptScriptStatusPaused   FactorySessionJavaScriptScriptStatus = "PAUSED"
	FactorySessionJavaScriptScriptStatusRunning  FactorySessionJavaScriptScriptStatus = "RUNNING"
)

// JavaScriptCheckpointRefEventPayload records a customer-visible checkpoint
// reference while raw VM checkpoint bodies remain orchestrator-owned.
type JavaScriptCheckpointRefEventPayload struct {
	ArtifactRef  FactoryArtifactRef `json:"artifactRef"`
	CheckpointID string             `json:"checkpointId"`
	Label        *string            `json:"label,omitempty"`
	Summary      *string            `json:"summary,omitempty"`
	Timestamp    *time.Time         `json:"timestamp,omitempty"`
}

// JavaScriptPhaseChangeEventPayload records the current JavaScript workflow
// phase and its child-dispatch progress.
type JavaScriptPhaseChangeEventPayload struct {
	ArgsDigest          *string                              `json:"argsDigest,omitempty"`
	ChildDispatchCounts FactorySessionChildDispatchCounts    `json:"childDispatchCounts"`
	Phase               string                               `json:"phase"`
	Phases              []string                             `json:"phases"`
	ScriptStatus        FactorySessionJavaScriptScriptStatus `json:"scriptStatus"`
}

// FactorySessionStartedEventPayload records session execution start facts.
// Session and orchestrator identity remain authoritative in event context.
type FactorySessionStartedEventPayload struct {
	ArgsDigest *string   `json:"argsDigest,omitempty"`
	FactoryID  *string   `json:"factoryId,omitempty"`
	PolicyHash *string   `json:"policyHash,omitempty"`
	SourceHash *string   `json:"sourceHash,omitempty"`
	SourceRef  *string   `json:"sourceRef,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
}

// FactorySessionPausedEventPayload records a successful pause transition.
type FactorySessionPausedEventPayload struct {
	PausedAt time.Time                     `json:"pausedAt"`
	Status   FactorySessionLifecycleStatus `json:"status"`
}

// FactorySessionResumedEventPayload records a successful resume transition.
type FactorySessionResumedEventPayload struct {
	ResumedAt time.Time                     `json:"resumedAt"`
	Status    FactorySessionLifecycleStatus `json:"status"`
}

// FactorySessionResultUpdatedEventPayload records partial or final result
// availability in canonical Factory history.
type FactorySessionResultUpdatedEventPayload struct {
	ArtifactIDs   []string                   `json:"artifactIds,omitempty"`
	ResultStatus  FactorySessionResultStatus `json:"resultStatus"`
	ResultSummary []work.WorkContentPart     `json:"resultSummary,omitempty"`
}

// FactorySessionCompletedEventPayload records the authoritative terminal
// Factory Session marker.
type FactorySessionCompletedEventPayload struct {
	ArtifactIDs    []string                           `json:"artifactIds,omitempty"`
	CompletedAt    time.Time                          `json:"completedAt"`
	DispatchCounts *FactorySessionChildDispatchCounts `json:"dispatchCounts,omitempty"`
	DurationMillis *int64                             `json:"durationMillis,omitempty"`
	FailureDetail  *workerexecution.FailureDetail     `json:"failureDetail,omitempty"`
	FinalStatus    FactorySessionLifecycleStatus      `json:"finalStatus"`
	ResultStatus   *FactorySessionResultStatus        `json:"resultStatus,omitempty"`
}

// FactorySessionLifecycleControlEventPayload records replay-safe accepted
// lifecycle control facts.
type FactorySessionLifecycleControlEventPayload struct {
	NewStatus      FactorySessionLifecycleStatus         `json:"newStatus"`
	OccurredAt     time.Time                             `json:"occurredAt"`
	Operation      FactorySessionLifecycleControlKind    `json:"operation"`
	Outcome        FactorySessionLifecycleControlOutcome `json:"outcome"`
	PreviousStatus FactorySessionLifecycleStatus         `json:"previousStatus"`
	Reason         *string                               `json:"reason,omitempty"`
}

// Clone returns a detached event envelope so recorders and stream consumers
// cannot mutate canonical history through payload or context slice aliases.
func (e FactoryEvent) Clone() FactoryEvent {
	clone := e
	clone.Payload = append(json.RawMessage(nil), e.Payload...)
	clone.Context.CheckpointID = cloneStringPointer(e.Context.CheckpointID)
	clone.Context.CurrentChainingTraceID = cloneStringPointer(e.Context.CurrentChainingTraceID)
	clone.Context.DispatchID = cloneStringPointer(e.Context.DispatchID)
	clone.Context.OrchestratorDialect = cloneStringPointer(e.Context.OrchestratorDialect)
	clone.Context.OrchestratorKind = cloneStringPointer(e.Context.OrchestratorKind)
	clone.Context.PhaseID = cloneStringPointer(e.Context.PhaseID)
	clone.Context.PhaseName = cloneStringPointer(e.Context.PhaseName)
	clone.Context.PreviousChainingTraceIDs = cloneStringSlicePointer(e.Context.PreviousChainingTraceIDs)
	clone.Context.RequestID = cloneStringPointer(e.Context.RequestID)
	clone.Context.SessionID = cloneStringPointer(e.Context.SessionID)
	clone.Context.SessionSequence = cloneIntPointer(e.Context.SessionSequence)
	clone.Context.Source = cloneStringPointer(e.Context.Source)
	clone.Context.TraceIDs = cloneStringSlicePointer(e.Context.TraceIDs)
	clone.Context.WorkIDs = cloneStringSlicePointer(e.Context.WorkIDs)
	return clone
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneStringSlicePointer(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	clone := append([]string(nil), (*value)...)
	return &clone
}

// NewFactoryEvent converts an event-compatible value into the detached domain
// envelope. It is intended for temporary producer and transport adapters while
// event payload contracts migrate to their domain owners.
func NewFactoryEvent(value any) (FactoryEvent, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return FactoryEvent{}, fmt.Errorf("encode factory event: %w", err)
	}
	var event FactoryEvent
	if err := json.Unmarshal(encoded, &event); err != nil {
		return FactoryEvent{}, fmt.Errorf("decode factory event envelope: %w", err)
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	return event, nil
}

// Decode writes the domain envelope into an event-compatible destination.
func (e FactoryEvent) Decode(destination any) error {
	encoded, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode factory event envelope: %w", err)
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return fmt.Errorf("decode factory event: %w", err)
	}
	return nil
}

// DecodePayload decodes the detached payload into its domain or boundary
// representation. Callers select the payload contract from Type.
func (e FactoryEvent) DecodePayload(destination any) error {
	if err := json.Unmarshal(e.Payload, destination); err != nil {
		return fmt.Errorf("decode %s factory event payload: %w", e.Type, err)
	}
	return nil
}

// InitialStructurePayload describes the topology available before work moves.
type InitialStructurePayload struct {
	Name             string                          `json:"name,omitempty"`
	Version          *FactoryVersion                 `json:"version,omitempty"`
	Resources        []FactoryResource               `json:"resources,omitempty"`
	ResourceManifest *PortableResourceManifestConfig `json:"resource_manifest,omitempty"`
	Constraints      []FactoryConstraint             `json:"constraints,omitempty"`
	Layout           *FactoryLayoutConfig            `json:"layout,omitempty"`
	Workers          []FactoryWorker                 `json:"workers,omitempty"`
	WorkTypes        []FactoryWorkType               `json:"work_types,omitempty"`
	Workstations     []FactoryWorkstation            `json:"workstations,omitempty"`
	Places           []FactoryPlace                  `json:"places,omitempty"`
	Relations        []work.FactoryRelation          `json:"relations,omitempty"`
}

// InitialStructureRequestEventPayload carries the canonical Factory snapshot
// emitted before Work begins moving. Optional recording metadata remains
// detached from the snapshot for compatibility with historical artifacts.
type InitialStructureRequestEventPayload struct {
	Factory         *FactorySnapshot   `json:"factory"`
	Metadata        *map[string]string `json:"metadata,omitempty"`
	SourceDirectory *string            `json:"sourceDirectory,omitempty"`
}

// FactoryChangeEventPayload carries the replacement Factory snapshot after a
// live definition change becomes active.
type FactoryChangeEventPayload struct {
	Factory           *FactorySnapshot               `json:"factory"`
	ChangeID          string                         `json:"changeId,omitempty"`
	Operation         string                         `json:"operation,omitempty"`
	TargetID          string                         `json:"targetId,omitempty"`
	PreviousRevision  *int                           `json:"previousRevision,omitempty"`
	NewRevision       *int                           `json:"newRevision,omitempty"`
	EffectiveSequence *int                           `json:"effectiveSequence,omitempty"`
	ResourceCapacity  *FactoryResourceCapacityChange `json:"resourceCapacity,omitempty"`
	Metadata          *map[string]string             `json:"metadata,omitempty"`
	SourceDirectory   *string                        `json:"sourceDirectory,omitempty"`
}

// FactoryResourceCapacityChange retains the detached accounting needed to
// replay a resource-capacity change without inspecting mutable runtime state.
type FactoryResourceCapacityChange struct {
	ResourceID        string `json:"resourceId"`
	ResourceName      string `json:"resourceName,omitempty"`
	PreviousCapacity  int    `json:"previousCapacity"`
	RequestedCapacity int    `json:"requestedCapacity"`
	EffectiveCapacity int    `json:"effectiveCapacity"`
	InUseCount        int    `json:"inUseCount"`
	AvailableCount    int    `json:"availableCount"`
	MinimumCapacity   int    `json:"minimumCapacity"`
	Outcome           string `json:"outcome"`
}

// FactoryChangeRequestEventPayload carries the normalized operator intent
// before a live Factory Session change is applied. Request identity and
// session scope remain authoritative on FactoryEventContext.
type FactoryChangeRequestEventPayload struct {
	ChangeID         string          `json:"changeId"`
	ExpectedRevision int             `json:"expectedRevision"`
	Operation        string          `json:"operation"`
	TargetID         string          `json:"targetId"`
	RequestedValue   json.RawMessage `json:"requestedValue"`
	Actor            string          `json:"actor,omitempty"`
	Source           string          `json:"source,omitempty"`
	Reason           string          `json:"reason,omitempty"`
}

// FactoryChangeFailedEventPayload closes an admitted live change when its
// application cannot commit. Failure fields are deliberately safe and never
// carry provider payloads, commands, stack traces, or raw requested values.
type FactoryChangeFailedEventPayload struct {
	ChangeID         string `json:"changeId"`
	Operation        string `json:"operation"`
	TargetID         string `json:"targetId"`
	ExpectedRevision int    `json:"expectedRevision"`
	PreviousRevision int    `json:"previousRevision"`
	FailureCode      string `json:"failureCode"`
	FailureMessage   string `json:"failureMessage"`
}

// WorkInputPayload describes a work item submitted to the factory.
type WorkInputPayload struct {
	TokenID   string                 `json:"token_id"`
	WorkItem  work.FactoryWorkItem   `json:"work_item"`
	Relations []work.FactoryRelation `json:"relations,omitempty"`
}

// WorkRequestPayload describes a canonical work request batch submission.
type WorkRequestPayload struct {
	RequestID     string                 `json:"request_id"`
	Type          work.WorkRequestType   `json:"type"`
	TraceID       string                 `json:"trace_id,omitempty"`
	Source        string                 `json:"source,omitempty"`
	ParentLineage []string               `json:"parent_lineage,omitempty"`
	WorkItems     []work.FactoryWorkItem `json:"work_items,omitempty"`
}

// RelationshipChangePayload describes a relationship added by a request batch.
type RelationshipChangePayload struct {
	Relation  work.FactoryRelation `json:"relation"`
	RequestID string               `json:"request_id,omitempty"`
	TraceID   string               `json:"trace_id,omitempty"`
}

// WorkstationRequestPayload describes work and resources consumed by a dispatch.
type WorkstationRequestPayload struct {
	DispatchID            string                                `json:"dispatch_id"`
	TransitionID          string                                `json:"transition_id"`
	Workstation           FactoryWorkstationRef                 `json:"workstation"`
	RunnerID              string                                `json:"runner_id,omitempty"`
	RunnerSelectionSource workerexecution.RunnerSelectionSource `json:"runner_selection_source,omitempty"`
	Inputs                []WorkstationInput                    `json:"inputs,omitempty"`
	Resources             []FactoryResourceUnit                 `json:"resources,omitempty"`
}

// WorkstationResponsePayload describes the result and outputs of a dispatch.
type WorkstationResponsePayload struct {
	DispatchID      string                                 `json:"dispatch_id"`
	TransitionID    string                                 `json:"transition_id"`
	Workstation     FactoryWorkstationRef                  `json:"workstation"`
	Result          WorkstationResult                      `json:"result"`
	DurationMillis  int64                                  `json:"duration_millis"`
	Outputs         []WorkstationOutput                    `json:"outputs,omitempty"`
	OutputWork      []work.FactoryWorkItem                 `json:"output_work,omitempty"`
	OutputResources []FactoryResourceUnit                  `json:"output_resources,omitempty"`
	TraceData       *FactoryTraceData                      `json:"trace_data,omitempty"`
	ProviderSession *providers.SessionMetadata             `json:"provider_session,omitempty"`
	Diagnostics     *workerdiagnostics.SafeWorkDiagnostics `json:"diagnostics,omitempty"`
	TerminalWork    *FactoryTerminalWork                   `json:"terminal_work,omitempty"`
}

// DispatchConsumedWorkRef identifies one work item consumed by a dispatch.
// Work identity remains authoritative in FactoryEventContext; WorkID is kept
// for compatibility with recordings that predate context-owned work IDs.
type DispatchConsumedWorkRef struct {
	WorkID string `json:"workId,omitempty"`
}

// DispatchRequestEventMetadata carries non-identity replay metadata retained
// on a dispatch request event.
type DispatchRequestEventMetadata struct {
	ReplayKey             *string                                `json:"replayKey,omitempty"`
	RunnerID              *string                                `json:"runnerId,omitempty"`
	RunnerSelectionSource *workerexecution.RunnerSelectionSource `json:"runnerSelectionSource,omitempty"`
}

// DispatchResourceRef identifies a resource consumed by a dispatch. Capacity
// remains on the event for public compatibility; replay reconstruction uses
// the canonical resource name.
type DispatchResourceRef struct {
	Capacity int    `json:"capacity"`
	Name     string `json:"name"`
}

// DispatchRequestEventPayload describes a dispatch beginning execution.
// Correlation identity belongs to FactoryEventContext; the deprecated chaining
// fields remain readable for compatibility with historical recordings.
type DispatchRequestEventPayload struct {
	CurrentChainingTraceID   *string                               `json:"currentChainingTraceId,omitempty"`
	Inputs                   []DispatchConsumedWorkRef             `json:"inputs"`
	Metadata                 *DispatchRequestEventMetadata         `json:"metadata,omitempty"`
	PreviousChainingTraceIDs *[]string                             `json:"previousChainingTraceIds,omitempty"`
	Resources                *[]DispatchResourceRef                `json:"resources,omitempty"`
	ExpectedArtifactContext  *work.ExpectedArtifactTemplateContext `json:"expectedArtifactContext,omitempty"`
	TransitionID             string                                `json:"transitionId"`
}

// HumanApprovalDecision is one of the fixed operator decisions exposed by a
// pending HUMAN_APPROVAL workstation. The requested event deliberately
// records the decision vocabulary, not mutable prompt or Work content.
type HumanApprovalDecision string

const (
	HumanApprovalDecisionApprove HumanApprovalDecision = "APPROVE"
	HumanApprovalDecisionReject  HumanApprovalDecision = "REJECT"
)

// HumanApprovalStatus is the durable lifecycle state of an approval request.
// Resolution is intentionally owned by a later workflow; this lane records
// only the pending state.
type HumanApprovalStatus string

const HumanApprovalStatusPending HumanApprovalStatus = "PENDING"

// HumanApprovalRequestedEventPayload contains only stable approval identity
// and fixed decision vocabulary. Session, dispatch, Work, trace, and ordering
// identity remain authoritative on FactoryEventContext.
type HumanApprovalRequestedEventPayload struct {
	ApprovalID    string                  `json:"approvalId"`
	WorkstationID string                  `json:"workstationId"`
	Decisions     []HumanApprovalDecision `json:"decisions"`
	Status        HumanApprovalStatus     `json:"status"`
}

// DispatchWorkerSessionAssociationEventPayload records the canonical,
// stable dispatch-to-Worker-Session identity association. Runtime commits
// this record before invoking worker_sessions.Service.Start for the
// associated dispatch, so the association is always observable before any
// event that depends on it (the Worker Session's own opening or output
// records). DispatchID remains authoritative on FactoryEventContext.
type DispatchWorkerSessionAssociationEventPayload struct {
	WorkerSessionID string `json:"workerSessionId"`
}

// WorkStateChangeEventPayload describes a canonical Petri marking position
// change. Correlation and ordering remain authoritative in FactoryEventContext.
type WorkStateChangeEventPayload struct {
	FromPlaceID   string                     `json:"fromPlaceId"`
	FromState     string                     `json:"fromState"`
	Reason        *string                    `json:"reason,omitempty"`
	Source        work.WorkStateChangeSource `json:"source"`
	ToPlaceID     string                     `json:"toPlaceId"`
	ToState       string                     `json:"toState"`
	TriggerWorkID *string                    `json:"triggerWorkId,omitempty"`
	WorkID        string                     `json:"workId"`
	WorkTypeName  string                     `json:"workTypeName"`
}

// RunRequestEventPayload describes the Factory snapshot and optional replay
// metadata captured when a Factory run starts.
type RunRequestEventPayload struct {
	Diagnostics *ReplayDiagnostics `json:"diagnostics,omitempty"`
	Factory     *FactorySnapshot   `json:"factory"`
	RecordedAt  time.Time          `json:"recordedAt"`
	WallClock   *RunEventWallClock `json:"wallClock,omitempty"`
}

// RunResponseEventPayload describes the terminal state and optional replay
// metadata emitted when a Factory run finishes.
type RunResponseEventPayload struct {
	Diagnostics *ReplayDiagnostics `json:"diagnostics,omitempty"`
	Reason      *string            `json:"reason,omitempty"`
	State       *FactoryState      `json:"state,omitempty"`
	WallClock   *RunEventWallClock `json:"wallClock,omitempty"`
}

// RunEventWallClock records the observable wall-clock bounds of a Factory run.
type RunEventWallClock struct {
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
}

// FactoryStateResponseEventPayload describes one canonical Factory lifecycle
// transition. The generated public event union maps this owner-defined payload
// at the transport boundary.
type FactoryStateResponseEventPayload struct {
	PreviousState *FactoryState `json:"previousState,omitempty"`
	Reason        *string       `json:"reason,omitempty"`
	State         FactoryState  `json:"state"`
}

// FactoryStateChangePayload describes a lifecycle state change.
type FactoryStateChangePayload struct {
	PreviousState string `json:"previous_state,omitempty"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
}

// FactoryResource describes a bounded resource type.
type FactoryResource struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Capacity int    `json:"capacity"`
}

// FactoryResourceUnit identifies a concrete resource token.
type FactoryResourceUnit struct {
	ResourceID string `json:"resource_id"`
	TokenID    string `json:"token_id"`
	PlaceID    string `json:"place_id,omitempty"`
}

// FactoryConstraint describes a named runtime constraint or limit.
type FactoryConstraint struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Scope  string            `json:"scope,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

// FactoryWorker describes an executable worker type.
type FactoryWorker struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	ModelProvider string            `json:"model_provider,omitempty"`
	Model         string            `json:"model,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
}

// FactoryWorkType describes a work type and its possible states.
type FactoryWorkType struct {
	ID                string                             `json:"id"`
	Name              string                             `json:"name,omitempty"`
	States            []FactoryStateDefinition           `json:"states,omitempty"`
	ExpectedArtifacts []work.ExpectedArtifactDeclaration `json:"expected_artifacts,omitempty"`
}

// FactoryStateDefinition describes a named state in a work type lifecycle.
type FactoryStateDefinition struct {
	Value    string `json:"value"`
	Category string `json:"category"`
}

// FactoryWorkstation describes a transition that can execute work.
type FactoryWorkstation struct {
	ID                string                             `json:"id"`
	Name              string                             `json:"name"`
	Description       *NameValueConfig                   `json:"description,omitempty"`
	WorkerID          string                             `json:"worker_id,omitempty"`
	Kind              string                             `json:"kind,omitempty"`
	Config            map[string]string                  `json:"config,omitempty"`
	InputPlaceIDs     []string                           `json:"input_place_ids,omitempty"`
	OutputPlaceIDs    []string                           `json:"output_place_ids,omitempty"`
	ContinuePlaceIDs  []string                           `json:"continue_place_ids,omitempty"`
	RejectionPlaceIDs []string                           `json:"rejection_place_ids,omitempty"`
	FailurePlaceIDs   []string                           `json:"failure_place_ids,omitempty"`
	ExpectedArtifacts []work.ExpectedArtifactDeclaration `json:"expected_artifacts,omitempty"`
}

// FactoryWorkstationRef identifies a workstation in a runtime event.
type FactoryWorkstationRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// FactoryPlace describes a topology place for work or resource tokens.
type FactoryPlace struct {
	ID       string `json:"id"`
	TypeID   string `json:"type_id"`
	State    string `json:"state"`
	Category string `json:"category,omitempty"`
}

// WorkstationInput describes an input token consumed by a dispatch.
type WorkstationInput struct {
	TokenID  string                `json:"token_id"`
	PlaceID  string                `json:"place_id"`
	WorkItem *work.FactoryWorkItem `json:"work_item,omitempty"`
	Resource *FactoryResourceUnit  `json:"resource,omitempty"`
}

// WorkstationOutput describes a token produced or moved by a dispatch.
type WorkstationOutput struct {
	Type      string                `json:"type"`
	TokenID   string                `json:"token_id"`
	FromPlace string                `json:"from_place,omitempty"`
	ToPlace   string                `json:"to_place,omitempty"`
	WorkItem  *work.FactoryWorkItem `json:"work_item,omitempty"`
	Resource  *FactoryResourceUnit  `json:"resource,omitempty"`
}

// WorkstationResult is owned by pkg/services/workers; the contracts mega-barrel
// retains a temporary alias until CLN-DEF-CONTRACTS story 007 deletes it.
type WorkstationResult = workerexecution.WorkstationResult

// FactoryTraceData carries trace identifiers attached to a runtime event.
type FactoryTraceData struct {
	TraceID string   `json:"trace_id,omitempty"`
	WorkIDs []string `json:"work_ids,omitempty"`
}

// FactoryTerminalWork describes work that reached a terminal outcome.
type FactoryTerminalWork struct {
	WorkItem work.FactoryWorkItem `json:"work_item"`
	Status   string               `json:"status"`
}
