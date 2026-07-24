package factorysessions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Root-owned durable execution peer contracts. Nested internal/execution
// aliases these symbols so peers compile against the Sessions root only.

// ApproveRequest approves one durable session awaiting policy approval.
type ApproveRequest struct {
	ControlRequest
	ApprovalPreviewID string
	ApprovedPolicy    map[string]any
}

// ArtifactDetail is the shared durable artifact read projection.
type ArtifactDetail struct {
	ArtifactSummary
	SessionID       string
	Summary         string
	CaptureMetadata map[string]any
	Content         json.RawMessage
	ContentRef      *ArtifactRetrievalRef
}

// ArtifactRedactionCounts summarizes secret suppression for one artifact.
type ArtifactRedactionCounts struct {
	Paths   int32
	Secrets int32
	Tokens  int32
}

// ArtifactRefSummary is a customer-visible artifact reference on session reads.
type ArtifactRefSummary struct {
	ID          string
	Kind        string
	Visibility  string
	ContentHash string
	SizeBytes   int64
}

// ArtifactRetrievalRef is a safe API-relative artifact retrieval reference.
type ArtifactRetrievalRef struct {
	Href   string
	Method string
}

// ArtifactSummary is the shared durable artifact list projection.
type ArtifactSummary struct {
	ID              string
	Kind            string
	Visibility      string
	Label           string
	ContentHash     string
	SizeBytes       int64
	CreatedAt       *time.Time
	DispatchID      string
	AuditMode       string
	RedactionCounts *ArtifactRedactionCounts
	RetrievalRef    *ArtifactRetrievalRef
}

// AsyncStartResult is the shared async durable execution start outcome.
type AsyncStartResult struct {
	SessionID        string
	Status           string
	OrchestratorKind string
	Dialect          string
	ResolvedSource   ResolvedSource
	SourceHash       string
	Policy           PolicyProjection
	Links            InspectionLinks
}

// CheckpointRef identifies the latest durable orchestrator checkpoint without
// exposing its persisted runtime state.
type CheckpointRef struct {
	ID    string
	Label string
	Phase string
}

// ControlError carries a typed lifecycle-control outcome for invalid transitions
// and other actionable control failures surfaced by service implementations.
type ControlError struct {
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Status    LifecycleStatus
	Message   string
	Links     LifecycleControlLinks
}

// ControlRequest is optional metadata shared by pause, resume, cancel, and terminate.
type ControlRequest struct {
	RequestID string
	Reason    string
}

// DispatchDetail is the shared durable dispatch read projection.
type DispatchDetail struct {
	DispatchSummary
	SessionID         string
	OrchestratorKind  string
	ArtifactIDs       []string
	StatusTransitions []DispatchStatus
	Petri             *DispatchPetriProjection
	JavaScript        *DispatchJavaScriptProjection
}

// DispatchFailureDetail exposes one customer-visible dispatch failure.
type DispatchFailureDetail struct {
	Reason  string
	Message string
}

// DispatchFilters narrows a canonical Dispatch read by phase and status.
type DispatchFilters struct {
	Phase  string
	Status DispatchStatus
}

// DispatchJavaScriptProjection carries JavaScript-specific dispatch metadata.
type DispatchJavaScriptProjection struct {
	TaskKind      string
	TaskLabel     string
	ExecutionMode string
}

// DispatchPetriProjection carries Petri-specific dispatch metadata.
type DispatchPetriProjection struct {
	TransitionID    string
	WorkstationName string
	WorkerType      string
}

// DispatchQueryRequest is the complete owner request for one filtered
// Factory Session Dispatch inventory read.
type DispatchQueryRequest struct {
	SessionID string
	Filters   DispatchFilters
}

// DispatchStatus is the canonical dispatch lifecycle status shared across orchestrators.
type DispatchStatus string

// DispatchSummary is the shared durable dispatch list projection.
type DispatchSummary struct {
	ID                    string
	Status                DispatchStatus
	DispatchKind          string
	Phase                 string
	Label                 string
	Attempt               int
	Retryable             *bool
	FailureClassification string
	RunnerID              string
	PresetID              string
	ModelProvider         string
	Model                 string
	ReasoningEffort       string
	Provider              string
	ProviderSessionRefs   []ProviderSessionRef
	OutputArtifactIDs     []string
	Usage                 *DispatchUsage
	Warnings              []DispatchWarning
	FailureDetail         *DispatchFailureDetail
	JavaScript            *DispatchJavaScriptProjection
}

// DispatchUsage summarizes one dispatch execution.
type DispatchUsage struct {
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	DurationMillis int64
	CostUSD        float64
	RetryCount     int32
}

// DispatchWarning exposes one customer-visible dispatch warning.
type DispatchWarning struct {
	Code    string
	Message string
}

// DurableSessionListSummary is the shared durable session list row with enough
// summary data for API, CLI, MCP, and UI to show status, result readiness,
// dispatch counts, artifact counts, lease/recoverability, and action availability.
type DurableSessionListSummary struct {
	SessionID        string
	Status           LifecycleStatus
	OrchestratorKind string
	Dialect          string
	ResolvedSource   ResolvedSource
	SourceHash       string
	Policy           PolicyProjection
	Phase            string
	Progress         *ProgressCounts
	ResultSummary    *ResultSummary
	ArtifactCount    int
	StaleLease       bool
	Recoverable      bool
	Lifecycle        *LifecycleTimestamps
	Links            InspectionLinks
	Actions          SessionActionAvailability
}

// EventReadResult carries replayed canonical session events for one durable session.
type EventReadResult struct {
	SessionID string
	Events    []json.RawMessage
}

// EventReconnectRequest identifies the last acknowledged durable session event.
type EventReconnectRequest struct {
	AfterEventID  string
	AfterSequence *int
}

// ExecutionProvider selects which durable Factory Session execution backend serves
// start and inspection calls at the shared service boundary.
type ExecutionProvider string

// FailureSummary exposes customer-visible durable session failure details.
type FailureSummary struct {
	Reason                 string
	Message                string
	PartialResultAvailable bool
}

// InlineWorkflowSource carries inline workflow source from a durable execution request.
type InlineWorkflowSource struct {
	Dialect      string
	InlineSource string
	Entrypoint   string
	Metadata     map[string]string
	// Factory declaration fields retain constraints when a caller has already
	// resolved a JavaScript factory asset and supplies its source directly.
	Agents        map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	ArgsSchema    json.RawMessage
	DefaultPolicy json.RawMessage
}

// InspectionLinks are API-relative links for polling and inspecting one session.
type InspectionLinks struct {
	Session    string
	Status     string
	Events     string
	Results    string
	Dispatches string
	Artifacts  string
}

// InterruptDispatchRequest interrupts one active durable session dispatch.
type InterruptDispatchRequest struct {
	ControlRequest
	DispatchID string
}

// LifecycleControlKind identifies one durable session lifecycle control operation.
type LifecycleControlKind string

// LifecycleControlLinks are API-relative links for post-control inspection.
type LifecycleControlLinks struct {
	Session    string
	Results    string
	Dispatches string
	Artifacts  string
	Events     string
	Status     string
}

// LifecycleControlOutcome reports how one lifecycle control request was evaluated.
type LifecycleControlOutcome string

// LifecycleControlResult is the shared durable lifecycle control outcome.
type LifecycleControlResult struct {
	SessionID           string
	Operation           LifecycleControlKind
	Outcome             LifecycleControlOutcome
	Status              LifecycleStatus
	Session             *SessionReadResult
	EffectivePolicyHash string
	ApprovalPreviewID   string
	DispatchID          string
	RetryDispatchID     string
	Detail              string
	Links               LifecycleControlLinks
}

// LifecycleStatus is the durable factory-session lifecycle status shared by read
// and control surfaces.
type LifecycleStatus string

// LifecycleTimestamps exposes durable session lifecycle timestamps.
type LifecycleTimestamps struct {
	QueuedAt           *time.Time
	AwaitingApprovalAt *time.Time
	StartedAt          *time.Time
	PausedAt           *time.Time
	ResumedAt          *time.Time
	FinishedAt         *time.Time
	InterruptedAt      *time.Time
	TerminatedAt       *time.Time
	UpdatedAt          *time.Time
}

// ListArtifactsResult is the shared durable artifact list outcome.
type ListArtifactsResult struct {
	SessionID string
	Artifacts []ArtifactSummary
}

// ListDispatchesResult is the shared durable dispatch list outcome.
type ListDispatchesResult struct {
	SessionID  string
	Dispatches []DispatchSummary
}

// ListSessionsRequest is the shared scoped session listing request consumed by API,
// CLI, MCP, and UI transports.
type ListSessionsRequest struct {
	Scope   SessionListScope
	Filters SessionListFilters
}

// ListSessionsResult is the shared scoped session listing outcome.
type ListSessionsResult struct {
	Scope           SessionListScope
	LiveSessions    []LiveSessionSummary
	DurableSessions []DurableSessionListSummary
}

// LiveSessionSummary is the shared live workspace session row for scope=live and
// scope=all responses. Live-session open and invocation remain separate from
// durable execution listing.
type LiveSessionSummary struct {
	ID         string
	FactoryDir string
	FolderPath string
	Project    string
	IsDefault  bool
}

// OrchestratorOverride is an optional orchestrator override on a start request.
type OrchestratorOverride struct {
	Kind string
	Raw  json.RawMessage
}

// PersistencePolicy is the application-level durable snapshot policy. The
// zero value preserves production persistence; callers must select Disabled
// explicitly when durable snapshots are not wanted.
type PersistencePolicy string

// PhaseSummary summarizes dispatch progress for one workflow phase.
type PhaseSummary struct {
	Phase                  string
	Label                  string
	DispatchCount          int
	CompletedDispatchCount int
	FailedDispatchCount    int
}

// PolicyProjection carries requested and effective policy hashes for start responses.
type PolicyProjection struct {
	Requested     map[string]any
	Effective     map[string]any
	EffectiveHash string
}

// ProgressCounts summarizes durable dispatch progress for one session.
type ProgressCounts struct {
	TotalDispatches       int
	CompletedDispatches   int
	FailedDispatches      int
	InFlightDispatches    int
	QueuedDispatches      int
	RunningDispatches     int
	CanceledDispatches    int
	TimedOutDispatches    int
	SkippedDispatches     int
	InterruptedDispatches int
	PhaseCount            int
}

// ProviderSessionRef correlates one dispatch with a provider session identity.
type ProviderSessionRef struct {
	Provider string
	Kind     string
	ID       string
}

// ResolvedSource is the customer-visible resolved source identity for one session.
type ResolvedSource struct {
	Kind            workflowsource.WorkflowSourceKind
	SourceRef       string
	SourceHash      string
	Dialect         string
	ResolutionOrder []string
	Metadata        map[string]string
	Agents          map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	ArgsSchema      json.RawMessage `json:"argsSchema,omitempty"`
	DefaultPolicy   json.RawMessage `json:"defaultPolicy,omitempty"`
}

// ResourceUsage summarizes one named resource pool for session runtime usage.
type ResourceUsage struct {
	Name      string
	Available int
	Total     int
}

// ResultAvailabilityDetail explains why a durable result is not ready or unavailable.
type ResultAvailabilityDetail struct {
	Reason    string
	Message   string
	Retryable bool
}

// ResultMode selects final or partial durable result retrieval.
type ResultMode string

// ResultReadResult is the shared durable session result projection consumed by API,
// CLI, MCP, and UI transports.
type ResultReadResult struct {
	SessionID        string
	ResultStatus     ResultStatus
	SessionStatus    LifecycleStatus
	Mode             ResultMode
	IncludeArtifacts bool
	PrimaryResult    json.RawMessage
	ArtifactIDs      []string
	ArtifactRefs     []ArtifactRefSummary
	Failure          *FailureSummary
	Availability     *ResultAvailabilityDetail
}

// ResultRequest normalizes durable session result read parameters.
type ResultRequest struct {
	Mode             ResultMode
	IncludeArtifacts bool
}

// ResultStatus is the customer-visible durable session result availability.
type ResultStatus string

// ResultSummary exposes customer-visible result readiness for one session read.
type ResultSummary struct {
	ResultStatus string
	Summary      string
}

// ResumeError carries a typed restart-resume failure surfaced by durable session
// execution when checkpoint summaries or persisted resume state are missing or invalid.
type ResumeError struct {
	Outcome   ResumeOutcome
	Status    LifecycleStatus
	Field     string
	Message   string
	SessionID string
}

// ResumeOutcome identifies one typed restart-resume failure class.
type ResumeOutcome string

// ResumeSessionRequest resumes one interrupted durable session from persisted
// checkpoint summaries and shared session state.
type ResumeSessionRequest struct {
	RequestID string
}

// RetryDispatchRequest retries one durable session dispatch.
type RetryDispatchRequest struct {
	ControlRequest
	DispatchID        string
	ForceNewAttempt   bool
	ResetAttemptCount bool
}

// RuntimeOptions selects durable JavaScript runtime execution behavior without
// changing workflow source syntax.
type RuntimeOptions struct {
	ChildExecutorMode string
}

// ExecutionService is the shared durable factory-session execution contract consumed by
// API, CLI, MCP, and UI transports. Live-session open and invocation remain on
// the separate factorysessions compatibility surface. All methods are
// cancellation-aware; transports must not mutate runtime state directly.
type ExecutionService interface {
	StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error)
	StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error)
	ResumeInterruptedSession(ctx context.Context, sessionID string, req ResumeSessionRequest) (AsyncStartResult, error)
	GetSession(ctx context.Context, sessionID string) (SessionReadResult, error)
	Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error)
	RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error)
	InterruptDispatch(ctx context.Context, sessionID string, req InterruptDispatchRequest) (LifecycleControlResult, error)
	GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error)
	ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error)
	QueryDispatches(ctx context.Context, request DispatchQueryRequest) (ListDispatchesResult, error)
	GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error)
	ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error)
	GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error)
	ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
}

// SessionActionAvailability exposes which lifecycle controls are currently valid
// for one listed durable session.
type SessionActionAvailability struct {
	CanPause             bool
	CanResume            bool
	CanCancel            bool
	CanTerminate         bool
	CanApprove           bool
	CanRetryDispatch     bool
	CanInterruptDispatch bool
}

// SessionBudgets exposes effective orchestrator policy budgets for one session read.
type SessionBudgets struct {
	MaxAgents int
}

// SessionListFilters narrows scoped session listing without requiring clients to
// parse session internals.
type SessionListFilters struct {
	Statuses          []LifecycleStatus
	OrchestratorKinds []string
	SourceKind        workflowsource.WorkflowSourceKind
	SourceRef         string
	ProjectBoundary   string
	Recoverable       *bool
	StaleLease        *bool
	CreatedAfter      *time.Time
	CreatedBefore     *time.Time
	UpdatedAfter      *time.Time
	UpdatedBefore     *time.Time
}

// SessionListScope selects which factory sessions a list request returns.
type SessionListScope string

// SessionReadResult is the shared durable session read projection consumed by API,
// CLI, MCP, and UI transports.
type SessionReadResult struct {
	SessionID        string
	Status           LifecycleStatus
	OrchestratorKind string
	Dialect          string
	ResolvedSource   ResolvedSource
	SourceHash       string
	Policy           PolicyProjection
	Phase            string
	PhaseSummaries   []PhaseSummary
	LatestCheckpoint *CheckpointRef
	Progress         *ProgressCounts
	Budgets          *SessionBudgets
	Usage            SessionUsage
	ResultSummary    *ResultSummary
	ArtifactRefs     []ArtifactRefSummary
	ArtifactCount    int
	Failure          *FailureSummary
	Lifecycle        *LifecycleTimestamps
	StaleLease       bool
	Links            InspectionLinks
}

// SessionUsage exposes resource availability and consumption for one session read.
type SessionUsage struct {
	Resources []ResourceUsage
}

// Source is the normalized durable execution source selector.
type Source struct {
	Kind           workflowsource.WorkflowSourceKind
	FactoryID      string
	FactoryInline  json.RawMessage
	WorkflowFile   string
	WorkflowName   string
	InlineWorkflow *InlineWorkflowSource
}

// StartRequest is the normalized durable session execution request shared by async
// and sync start across API, CLI, MCP, and UI.
type StartRequest struct {
	RequestID       string
	Source          Source
	Args            map[string]any
	Orchestrator    *OrchestratorOverride
	RequestedPolicy map[string]any
	Runtime         *RuntimeOptions
	Wait            *WaitOptions
	EventConsumer   FactoryEventConsumer `json:"-"`
}

// SyncOutcome reports how a sync start wait ended.
type SyncOutcome string

// SyncStartResult is the shared sync durable execution start outcome.
type SyncStartResult struct {
	AsyncStartResult
	SyncOutcome              SyncOutcome
	Result                   json.RawMessage
	TimedOut                 bool
	SessionCanceledByTimeout bool
}

// ValidationError reports a stable client-side normalization failure.
type ValidationError struct {
	Message string
	Field   string
}

// WaitOptions bounds sync execution waits.
type WaitOptions struct {
	TimeoutMillis   *int64
	CancelOnTimeout bool
}

// ExecutionValidationError is the published root name for durable start/input validation failures.
type ExecutionValidationError = ValidationError

// ExecutionFactoryEventConsumer is the published root name for durable sync event observation.
type ExecutionFactoryEventConsumer = FactoryEventConsumer

const (
	LifecycleStatusQueued           LifecycleStatus = "QUEUED"
	LifecycleStatusAwaitingApproval LifecycleStatus = "AWAITING_APPROVAL"
	LifecycleStatusRunning          LifecycleStatus = "RUNNING"
	LifecycleStatusPaused           LifecycleStatus = "PAUSED"
	LifecycleStatusResuming         LifecycleStatus = "RESUMING"
	LifecycleStatusSucceeded        LifecycleStatus = "SUCCEEDED"
	LifecycleStatusFailed           LifecycleStatus = "FAILED"
	LifecycleStatusCanceling        LifecycleStatus = "CANCELING"
	LifecycleStatusCanceled         LifecycleStatus = "CANCELED"
	LifecycleStatusTimedOut         LifecycleStatus = "TIMED_OUT"
	LifecycleStatusInterrupted      LifecycleStatus = "INTERRUPTED"
	LifecycleStatusTerminated       LifecycleStatus = "TERMINATED"
)

const (
	LifecycleControlPause             LifecycleControlKind = "PAUSE"
	LifecycleControlResume            LifecycleControlKind = "RESUME"
	LifecycleControlCancel            LifecycleControlKind = "CANCEL"
	LifecycleControlTerminate         LifecycleControlKind = "TERMINATE"
	LifecycleControlApprove           LifecycleControlKind = "APPROVE"
	LifecycleControlRetryDispatch     LifecycleControlKind = "RETRY_DISPATCH"
	LifecycleControlInterruptDispatch LifecycleControlKind = "INTERRUPT_DISPATCH"
)

const (
	LifecycleControlOutcomeAccepted        LifecycleControlOutcome = "ACCEPTED"
	LifecycleControlOutcomeNoOp            LifecycleControlOutcome = "NO_OP"
	LifecycleControlOutcomeInvalidState    LifecycleControlOutcome = "INVALID_STATE"
	LifecycleControlOutcomeTerminalSession LifecycleControlOutcome = "TERMINAL_SESSION"
	LifecycleControlOutcomeConflict        LifecycleControlOutcome = "CONFLICT"
)

const (
	SessionListScopeLive      SessionListScope = "live"
	SessionListScopePersisted SessionListScope = "persisted"
	SessionListScopeAll       SessionListScope = "all"
)

// DefaultSessionListScope is live for backward-compatible live workspace session listing.
const DefaultSessionListScope = SessionListScopeLive

const LifecycleControlOutcomeClassNotFound = "NOT_FOUND"

const (
	ResultStatusNotReady          ResultStatus = "NOT_READY"
	ResultStatusPartial           ResultStatus = "PARTIAL"
	ResultStatusFinal             ResultStatus = "FINAL"
	ResultStatusFailedWithPartial ResultStatus = "FAILED_WITH_PARTIAL"
	ResultStatusUnavailable       ResultStatus = "UNAVAILABLE"
)

const (
	ResultModeFinal   ResultMode = "final"
	ResultModePartial ResultMode = "partial"
)

const (
	// ExecutionProviderFake selects the deterministic in-memory fake session path.
	ExecutionProviderFake ExecutionProvider = "fake"
	// ExecutionProviderJavaScriptRuntime selects the real simple JavaScript runtime path.
	ExecutionProviderJavaScriptRuntime ExecutionProvider = "javascript-runtime"
)

const (
	// ChildExecutorModeFake selects deterministic in-process child execution.
	ChildExecutorModeFake = workflowsource.JavaScriptChildExecutionModeFake
	// ChildExecutorModeLive selects provider-backed child execution.
	ChildExecutorModeLive = workflowsource.JavaScriptChildExecutionModeLive
)

const (
	// ResumeOutcomeMissingCheckpoint reports that no persisted checkpoint summary
	// exists for the interrupted session.
	ResumeOutcomeMissingCheckpoint ResumeOutcome = "MISSING_CHECKPOINT"
	// ResumeOutcomeInvalidState reports corrupted resume metadata or a session that
	// is not eligible for restart-resume reconstruction.
	ResumeOutcomeInvalidState ResumeOutcome = "INVALID_RESUME_STATE"
	// ResumeOutcomeCorruptedPersistence reports unreadable or invalid persisted
	// session snapshots required for restart-resume.
	ResumeOutcomeCorruptedPersistence ResumeOutcome = "CORRUPTED_PERSISTENCE"
)

// ErrArtifactNotFound reports that no artifact matched the requested id within
// the targeted durable session.
var ErrArtifactNotFound = errors.New("artifact not found")

// ErrDispatchNotFound reports that no dispatch matched the requested id within
// the targeted durable session.
var ErrDispatchNotFound = errors.New("dispatch not found")

// ErrExecutionRequestIDConflict reports that requestId was reused with a different
// normalized execution tuple or a different start operation (async vs sync).
var ErrExecutionRequestIDConflict = errors.New("execution request id conflict")

// ErrReconnectCursorNotFound reports that the reconnect cursor did not match any
// recorded durable session event.
var ErrReconnectCursorNotFound = errors.New("reconnect cursor not found in event history")

// ErrExecutionServiceNotConfigured reports an application composition graph that did
// not supply its required durable Factory Session execution collaborator.
var ErrExecutionServiceNotConfigured = errors.New("durable factory session execution service is not configured")

// ErrDurableSessionNotFound reports that no durable session matched the requested id.
var ErrDurableSessionNotFound = errors.New("factory session not found")

func (e *ControlError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *ResumeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

