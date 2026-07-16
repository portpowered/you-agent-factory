package interfaces

import (
	"encoding/json"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RequestValidationError reports a stable client-side validation failure.
type RequestValidationError struct {
	Message string
}

func (e *RequestValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// FactoryState represents the current lifecycle state of a Factory.
type FactoryState string

const (
	FactoryStateIdle      FactoryState = "IDLE"
	FactoryStateRunning   FactoryState = "RUNNING"
	FactoryStatePaused    FactoryState = "PAUSED"
	FactoryStateCompleted FactoryState = "COMPLETED"
	FactoryStateFailed    FactoryState = "FAILED"
)

// RuntimeMode determines whether the runtime exits on idle completion or stays
// available for future submissions until its context is canceled.
type RuntimeMode string

const (
	RuntimeModeBatch   RuntimeMode = "BATCH"
	RuntimeModeService RuntimeMode = "SERVICE"
)

// SubmitRequest is the internal normalized item used to create work tokens.
type SubmitRequest struct {
	RequestID                string            `json:"requestId,omitempty"`
	WorkID                   string            `json:"workId,omitempty"`
	Name                     string            `json:"name,omitempty"`
	WorkTypeID               string            `json:"workTypeName"`
	TargetState              string            `json:"targetState,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  []byte            `json:"payload"`
	Tags                     map[string]string `json:"tags"`
	Relations                []Relation        `json:"relations"`
	ExecutionID              string            `json:"executionId,omitempty"`
	InvocationArguments      *InvocationArguments
}

// WorkRequestType identifies the canonical request contract accepted by factory submit surfaces.
type WorkRequestType string

const (
	WorkRequestTypeFactoryRequestBatch WorkRequestType = "FACTORY_REQUEST_BATCH"
)

// WorkRequest is the factory-domain representation of the generated WorkRequest schema.
type WorkRequest struct {
	RequestID              string          `json:"requestId"`
	CurrentChainingTraceID string          `json:"currentChainingTraceId,omitempty"`
	Type                   WorkRequestType `json:"type"`
	Works                  []Work          `json:"works,omitempty"`
	Relations              []WorkRelation  `json:"relations,omitempty"`
}

// WorkRequestSubmittedWork identifies one accepted work item in a batch upsert.
type WorkRequestSubmittedWork struct {
	Name         string
	WorkTypeName string
	WorkID       string
}

// WorkRequestSubmitResult describes accepted request metadata.
type WorkRequestSubmitResult struct {
	RequestID    string
	TraceID      string
	WorkID       string
	Name         string
	WorkTypeName string
	Accepted     bool
	Works        []WorkRequestSubmittedWork
}

// FactoryInvocationResult carries the transport-independent outcome of one
// Factory Session invocation after input resolution and result selection.
type FactoryInvocationResult struct {
	RequestID     string
	TraceID       string
	Status        factoryapi.InvocationTerminalStatus
	PrimaryResult []WorkContentPart
	ErrorCode     string
	Message       string
	SessionID     string
	WorkID        string
	WorkName      string
	WorkState     string
}

// Work is one public item inside a WorkRequest batch.
type Work struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    string            `json:"state,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  any               `json:"payload,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	ExecutionID              string            `json:"-"`
	RuntimeRelations         []Relation        `json:"-"`
	// InvocationArguments are runtime-only normalized values used to resolve
	// invocation-interpolated worker and workstation fields. They are excluded
	// from the public request and event contracts because values can be sensitive.
	InvocationArguments *InvocationArguments `json:"-"`
}

// WorkContentPart is the backend-owned canonical work content shape mirrored
// from the public API contract.
type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	URL         string              `json:"url,omitempty"`
	File        string              `json:"file,omitempty"`
	JSON        json.RawMessage     `json:"json,omitempty"`
	Slot        string              `json:"slot,omitempty"`
	Label       string              `json:"label,omitempty"`
	Role        string              `json:"role,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	ArtifactID  string              `json:"artifactId,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

// WorkContentPartType identifies one canonical content part kind.
type WorkContentPartType string

const (
	WorkContentPartTypeText   WorkContentPartType = "text"
	WorkContentPartTypeImage  WorkContentPartType = "image"
	WorkContentPartTypeAudio  WorkContentPartType = "AUDIO"
	WorkContentPartTypeJSON   WorkContentPartType = "JSON"
	WorkContentPartTypeBinary WorkContentPartType = "BINARY"
)

// InvocationArguments carries transport-independent invocation parameter
// normalization data through runtime-only work and dispatch paths.
type InvocationArguments struct {
	Arguments map[string]InvocationArgument `json:"-"`
}

// InvocationArgument is one canonical invocation parameter value bundle keyed
// by authored internal parameter name.
type InvocationArgument struct {
	Values    []string                   `json:"-"`
	ValueMode string                     `json:"-"`
	Sensitive bool                       `json:"-"`
	Sources   []InvocationArgumentSource `json:"-"`
}

// InvocationArgumentSource records where one canonical invocation parameter was
// resolved from without exposing raw values.
type InvocationArgumentSource struct {
	Kind   string `json:"-"`
	Name   string `json:"-"`
	Redact bool   `json:"-"`
}

// CanonicalEventTime normalizes runtime event boundary timestamps to UTC while
// preserving zero values so optional/fallback handling remains explicit.
func CanonicalEventTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC()
}

// RuntimeWorkstationLookup resolves runtime workstation definitions by authored name.
type RuntimeWorkstationLookup interface {
	Workstation(name string) (*FactoryWorkstationConfig, bool)
}

// RuntimeDefinitionLookup resolves runtime worker and workstation definitions by authored name.
type RuntimeDefinitionLookup interface {
	RuntimeWorkstationLookup
	Worker(name string) (*WorkerConfig, bool)
}

// RuntimeFactoryConfigLookup resolves the effective runtime factory config when
// a consumer needs optional access to factory-level settings.
type RuntimeFactoryConfigLookup interface {
	FactoryConfig() *FactoryConfig
}

// RuntimeConfigLookup exposes the canonical public runtime-facing lookup
// contract for consumers that need runtime definitions plus path-aware
// execution lookups.
type RuntimeConfigLookup interface {
	RuntimeDefinitionLookup
	RuntimeFactoryConfigLookup
	FactoryDir() string
	RuntimeBaseDir() string
}

func firstNonNilLookup[T comparable](lookups ...T) T {
	var zero T
	for _, lookup := range lookups {
		if lookup != zero {
			return lookup
		}
	}
	return zero
}

// FirstRuntimeDefinitionLookup returns the first non-nil runtime definition
// lookup from the provided candidates.
func FirstRuntimeDefinitionLookup(lookups ...RuntimeDefinitionLookup) RuntimeDefinitionLookup {
	return firstNonNilLookup(lookups...)
}

// FirstRuntimeFactoryConfigLookup returns the first non-nil runtime factory
// config lookup from the provided candidates.
func FirstRuntimeFactoryConfigLookup(lookups ...RuntimeFactoryConfigLookup) RuntimeFactoryConfigLookup {
	return firstNonNilLookup(lookups...)
}

// FirstRuntimeWorkstationLookup returns the first non-nil runtime workstation
// lookup from the provided candidates.
func FirstRuntimeWorkstationLookup(lookups ...RuntimeWorkstationLookup) RuntimeWorkstationLookup {
	return firstNonNilLookup(lookups...)
}

// MutationType describes the kind of marking mutation.
type MutationType string

const (
	MutationMove    MutationType = "MOVE"
	MutationCreate  MutationType = "CREATE"
	MutationConsume MutationType = "CONSUME"
)

// MarkingMutation is a declarative description of a single token movement.
type MarkingMutation struct {
	Type           MutationType    `json:"type"`
	TokenID        string          `json:"token_id"`
	FromPlace      string          `json:"from_place"`
	ToPlace        string          `json:"to_place"`
	Reason         string          `json:"reason"`
	NewToken       *Token          `json:"-"`
	FailureRecords []FailureRecord `json:"-"`
}

// TokenMutationRecord stores the raw token mutation emitted while applying a
// worker result.
type TokenMutationRecord struct {
	DispatchID   string       `json:"dispatch_id"`
	TransitionID string       `json:"transition_id"`
	Outcome      WorkOutcome  `json:"outcome"`
	Type         MutationType `json:"type"`
	TokenID      string       `json:"token_id"`
	FromPlace    string       `json:"from_place"`
	ToPlace      string       `json:"to_place"`
	Reason       string       `json:"reason"`
	Token        *Token       `json:"token,omitempty"`
}

// DispatchEntry tracks an in-flight dispatch awaiting a worker result.
type DispatchEntry struct {
	DispatchID      string            `json:"dispatch_id"`
	TransitionID    string            `json:"transition_id"`
	WorkstationName string            `json:"workstation_name,omitempty"`
	StartTime       time.Time         `json:"start_time"`
	ConsumedTokens  []Token           `json:"consumed_tokens"`
	HeldMutations   []MarkingMutation `json:"held_mutations"`
}

// CompletedDispatch records a dispatch that has finished, with timing data.
type CompletedDispatch struct {
	DispatchID                  string                   `json:"dispatch_id"`
	TransitionID                string                   `json:"transition_id"`
	WorkstationName             string                   `json:"workstation_name,omitempty"`
	Outcome                     WorkOutcome              `json:"outcome"`
	SelectedClassificationLabel string                   `json:"selected_classification_label,omitempty"`
	Reason                      string                   `json:"reason,omitempty"`
	FailureMetadata             *WorkFailureMetadata     `json:"failure_metadata,omitempty"`
	ProviderSession             *ProviderSessionMetadata `json:"provider_session,omitempty"`
	StartTime                   time.Time                `json:"start_time"`
	EndTime                     time.Time                `json:"end_time"`
	Duration                    time.Duration            `json:"duration"`
	ConsumedTokens              []Token                  `json:"consumed_tokens,omitempty"`
	OutputMutations             []TokenMutationRecord    `json:"output_mutations,omitempty"`
}

// ActiveThrottlePause records an active provider/model dispatch pause window.
type ActiveThrottlePause struct {
	LaneID      string    `json:"lane_id"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	PausedAt    time.Time `json:"paused_at,omitempty"`
	PausedUntil time.Time `json:"paused_until"`
}

// EnabledTransition represents a transition that is ready to fire.
type EnabledTransition struct {
	TransitionID string             `json:"transition_id"`
	WorkerType   string             `json:"worker_type"`
	Bindings     map[string][]Token `json:"bindings"`
	ArcModes     map[string]ArcMode `json:"arc_modes"`
}

// ArcMode describes how an enabled transition uses an input arc.
type ArcMode int

const (
	ArcModeConsume ArcMode = iota
	ArcModeObserve
)

// FiringDecision represents a scheduler's decision to fire a transition.
type FiringDecision struct {
	TransitionID  string              `json:"transition_id"`
	ConsumeTokens []string            `json:"consume_tokens"`
	WorkerType    string              `json:"worker_type"`
	InputBindings map[string][]string `json:"input_bindings,omitempty"`
}

// TickResult is the output of a single subsystem execution.
type TickResult struct {
	Mutations              []MarkingMutation          `json:"mutations,omitempty"`
	GeneratedBatches       []GeneratedSubmissionBatch `json:"generated_batches,omitempty"`
	Dispatches             []DispatchRecord           `json:"dispatches,omitempty"`
	Histories              []TokenHistory             `json:"histories,omitempty"`
	CompletedDispatches    []CompletedDispatch        `json:"completed_dispatches,omitempty"`
	ActiveThrottlePauses   []ActiveThrottlePause      `json:"active_throttle_pauses,omitempty"`
	ThrottlePausesObserved bool                       `json:"throttle_pauses_observed,omitempty"`
	ShouldTerminate        bool                       `json:"should_terminate,omitempty"`
}

// DispatchRecord pairs a WorkDispatch with the marking mutations consumed to fire it.
type DispatchRecord struct {
	Dispatch  WorkDispatch      `json:"dispatch"`
	Mutations []MarkingMutation `json:"mutations"`
}

// RuntimeStatus describes whether the runtime is actively processing work,
// intentionally idle but still available, or terminally finished.
type RuntimeStatus string

const (
	RuntimeStatusActive   RuntimeStatus = "ACTIVE"
	RuntimeStatusIdle     RuntimeStatus = "IDLE"
	RuntimeStatusFinished RuntimeStatus = "FINISHED"
)

// EngineStateSnapshot is a unified point-in-time snapshot of the full engine
// state: runtime state, factory lifecycle, session metrics, and uptime.
type EngineStateSnapshot[TMarking any, TTopology any] struct {
	RuntimeStatus      RuntimeStatus             `json:"runtime_status"`
	StreamGenerationID string                    `json:"stream_generation_id,omitempty"`
	Marking            TMarking                  `json:"marking"`
	Dispatches         map[string]*DispatchEntry `json:"dispatches"`
	InFlightCount      int                       `json:"in_flight_count"`
	Results            []WorkResult              `json:"results"`
	DispatchHistory    []CompletedDispatch       `json:"dispatch_history"`
	// ActiveThrottlePauses exposes active provider/model pause windows owned by
	// dispatcher policy for tests and observability reconstruction.
	ActiveThrottlePauses []ActiveThrottlePause `json:"active_throttle_pauses,omitempty"`
	TickCount            int                   `json:"tick_count"`

	// Factory lifecycle state.
	FactoryState string `json:"factory_state"`

	// LifecycleControlStatus is the canonical pause/resume lifecycle status
	// reconstructed from SESSION_PAUSED and SESSION_RESUMED events when present.
	LifecycleControlStatus string `json:"lifecycle_control_status,omitempty"`

	// Uptime since the factory started.
	Uptime time.Duration `json:"uptime"`

	// Topology is the workflow net used to interpret marking and dispatch
	// records for service-facing observability read models.
	Topology TTopology `json:"topology,omitempty"`
}

// RuntimeStateSnapshot returns the raw runtime portion of the aggregate
// snapshot for reducers that intentionally operate on runtime records.
func (s EngineStateSnapshot[TMarking, TTopology]) RuntimeStateSnapshot() EngineStateSnapshot[TMarking, TTopology] {
	return EngineStateSnapshot[TMarking, TTopology]{
		RuntimeStatus:        s.RuntimeStatus,
		StreamGenerationID:   s.StreamGenerationID,
		Marking:              s.Marking,
		Dispatches:           s.Dispatches,
		InFlightCount:        s.InFlightCount,
		Results:              s.Results,
		DispatchHistory:      s.DispatchHistory,
		ActiveThrottlePauses: s.ActiveThrottlePauses,
		TickCount:            s.TickCount,
	}
}

// Normalized returns the stable backend-owned kind for supported public aliases.
func (t WorkContentPartType) Normalized() WorkContentPartType {
	switch t {
	case "TEXT":
		return WorkContentPartTypeText
	case "IMAGE":
		return WorkContentPartTypeImage
	default:
		return t
	}
}

// WorkRelationType identifies a relationship between work items in a WorkRequest.
type WorkRelationType string

const (
	WorkRelationDependsOn   WorkRelationType = "DEPENDS_ON"
	WorkRelationParentChild WorkRelationType = "PARENT_CHILD"
)

// WorkRelation describes a relation between named work items in a WorkRequest.
type WorkRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

// WorkRequestNormalizeOptions provides context inferred from a submit surface.
type WorkRequestNormalizeOptions struct {
	DefaultWorkTypeID string
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
}

// FactoryWorkItem describes a unit of work at a point in history.
type FactoryWorkItem struct {
	ID                       string            `json:"id"`
	WorkTypeID               string            `json:"workTypeId"`
	State                    string            `json:"state,omitempty"`
	DisplayName              string            `json:"displayName,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	ParentID                 string            `json:"parentId,omitempty"`
	PlaceID                  string            `json:"placeId,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
}

// FactoryRelation describes a typed relationship between work items.
type FactoryRelation struct {
	Type           string `json:"type"`
	SourceWorkID   string `json:"sourceWorkId,omitempty"`
	SourceWorkName string `json:"sourceWorkName,omitempty"`
	TargetWorkID   string `json:"targetWorkId"`
	TargetWorkName string `json:"targetWorkName,omitempty"`
	RequiredState  string `json:"requiredState,omitempty"`
	RequestID      string `json:"requestId,omitempty"`
	TraceID        string `json:"traceId,omitempty"`
}

// WorkRequestRecord stores the batch-level request observed before work token injection.
type WorkRequestRecord struct {
	RequestID     string
	Type          WorkRequestType
	TraceID       string
	Source        string
	ParentLineage []string
	WorkItems     []FactoryWorkItem
	Relations     []FactoryRelation
}

// GeneratedSubmissionBatchMetadata captures request-level metadata for generated work.
type GeneratedSubmissionBatchMetadata struct {
	Source        string   `json:"source"`
	ParentLineage []string `json:"parentLineage"`
}

// GeneratedSubmissionBatch carries a canonical generated request with runtime submissions.
type GeneratedSubmissionBatch struct {
	Request     WorkRequest                      `json:"request"`
	Metadata    GeneratedSubmissionBatchMetadata `json:"metadata"`
	Submissions []SubmitRequest                  `json:"submissions"`
}

// FactorySubmissionRecord stores the engine tick at which a submit request
// became visible to the runtime.
type FactorySubmissionRecord struct {
	SubmissionID string
	ObservedTick int
	Request      SubmitRequest
	Source       string
}

// FactoryDispatchRecord stores a raw WorkDispatch plus token mutations held
// while the worker is in flight.
type FactoryDispatchRecord struct {
	DispatchID     string
	CreatedTick    int
	Dispatch       WorkDispatch
	HeldMutations  []MarkingMutation
	ConsumedTokens []string
}

// FactoryCompletionRecord stores a worker result at the logical tick where the
// engine observed it.
type FactoryCompletionRecord struct {
	CompletionID string
	DispatchID   string
	ObservedTick int
	Result       WorkResult
}

// SubmissionHookContext is the input passed to engine-owned submission hooks
// once per logical tick.
type SubmissionHookContext[TSnapshot any] struct {
	Snapshot          TSnapshot
	ContinuationState map[string]string
}

// SubmissionHookResult contains all due hook output observed by the engine at
// one logical tick.
type SubmissionHookResult struct {
	GeneratedBatches  []GeneratedSubmissionBatch
	Results           []WorkResult
	MarkingMutations  []MarkingMutation
	ContinuationState map[string]string
	KeepAlive         bool
}

// FactorySessionJavaScriptCheckpointRef is a customer-visible JavaScript checkpoint
// reference without raw VM checkpoint payload bodies.
type FactorySessionJavaScriptCheckpointRef struct {
	ID                 string                           `json:"id"`
	Label              string                           `json:"label"`
	Summary            string                           `json:"summary"`
	Timestamp          time.Time                        `json:"timestamp,omitempty"`
	ArtifactRef        *JavaScriptCheckpointArtifactRef `json:"artifactRef,omitempty"`
	ResumabilityStatus string                           `json:"resumabilityStatus,omitempty"`
	Warnings           []FactorySessionDispatchWarning  `json:"warnings,omitempty"`
}

// FactorySessionJavaScriptRuntimeState carries JavaScript orchestrator runtime
// projection fields for one factory session.
type FactorySessionJavaScriptRuntimeState struct {
	Phase               string                                  `json:"phase"`
	Phases              []string                                `json:"phases"`
	ArgsDigest          string                                  `json:"argsDigest"`
	Checkpoints         []FactorySessionJavaScriptCheckpointRef `json:"checkpoints"`
	ScriptStatus        string                                  `json:"scriptStatus"`
	QueuedDispatches    int                                     `json:"queuedDispatches"`
	RunningDispatches   int                                     `json:"runningDispatches"`
	CompletedDispatches int                                     `json:"completedDispatches"`
	Dispatches          []FactorySessionDispatchState           `json:"dispatches,omitempty"`
	Artifacts           []FactorySessionArtifactState           `json:"artifacts,omitempty"`
	PrimaryResult       []WorkContentPart                       `json:"primaryResult,omitempty"`
	ResultStatus        string                                  `json:"resultStatus,omitempty"`
}
