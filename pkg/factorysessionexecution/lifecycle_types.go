package factorysessionexecution

import "time"

// LifecycleStatus is the durable factory-session lifecycle status shared by read
// and control surfaces.
type LifecycleStatus string

const (
	LifecycleStatusQueued            LifecycleStatus = "QUEUED"
	LifecycleStatusAwaitingApproval  LifecycleStatus = "AWAITING_APPROVAL"
	LifecycleStatusRunning           LifecycleStatus = "RUNNING"
	LifecycleStatusPaused            LifecycleStatus = "PAUSED"
	LifecycleStatusResuming          LifecycleStatus = "RESUMING"
	LifecycleStatusSucceeded         LifecycleStatus = "SUCCEEDED"
	LifecycleStatusFailed            LifecycleStatus = "FAILED"
	LifecycleStatusCanceling         LifecycleStatus = "CANCELING"
	LifecycleStatusCanceled          LifecycleStatus = "CANCELED"
	LifecycleStatusTimedOut          LifecycleStatus = "TIMED_OUT"
	LifecycleStatusInterrupted       LifecycleStatus = "INTERRUPTED"
	LifecycleStatusTerminated        LifecycleStatus = "TERMINATED"
)

// LifecycleControlKind identifies one durable session lifecycle control operation.
type LifecycleControlKind string

const (
	LifecycleControlPause         LifecycleControlKind = "PAUSE"
	LifecycleControlResume        LifecycleControlKind = "RESUME"
	LifecycleControlCancel          LifecycleControlKind = "CANCEL"
	LifecycleControlTerminate       LifecycleControlKind = "TERMINATE"
	LifecycleControlApprove         LifecycleControlKind = "APPROVE"
	LifecycleControlRetryDispatch   LifecycleControlKind = "RETRY_DISPATCH"
)

// LifecycleControlOutcome reports how one lifecycle control request was evaluated.
type LifecycleControlOutcome string

const (
	LifecycleControlOutcomeAccepted        LifecycleControlOutcome = "ACCEPTED"
	LifecycleControlOutcomeNoOp            LifecycleControlOutcome = "NO_OP"
	LifecycleControlOutcomeInvalidState    LifecycleControlOutcome = "INVALID_STATE"
	LifecycleControlOutcomeTerminalSession LifecycleControlOutcome = "TERMINAL_SESSION"
	LifecycleControlOutcomeConflict        LifecycleControlOutcome = "CONFLICT"
)

// ControlRequest is optional metadata shared by pause, resume, cancel, and terminate.
type ControlRequest struct {
	RequestID string
	Reason    string
}

// ApproveRequest approves one durable session awaiting policy approval.
type ApproveRequest struct {
	ControlRequest
	ApprovalPreviewID string
	ApprovedPolicy    map[string]any
}

// RetryDispatchRequest retries one durable session dispatch.
type RetryDispatchRequest struct {
	ControlRequest
	DispatchID        string
	ForceNewAttempt   bool
	ResetAttemptCount bool
}

// PhaseSummary summarizes dispatch progress for one workflow phase.
type PhaseSummary struct {
	Phase                  string
	Label                  string
	DispatchCount          int
	CompletedDispatchCount int
	FailedDispatchCount    int
}

// ProgressCounts summarizes durable dispatch progress for one session.
type ProgressCounts struct {
	TotalDispatches     int
	CompletedDispatches int
	FailedDispatches    int
	InFlightDispatches  int
	PhaseCount          int
}

// ResultSummary exposes customer-visible result readiness for one session read.
type ResultSummary struct {
	ResultStatus string
	Summary      string
}

// FailureSummary exposes customer-visible durable session failure details.
type FailureSummary struct {
	Reason                 string
	Message                string
	ErrorClass             string
	PartialResultAvailable bool
}

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

// ArtifactRefSummary is a customer-visible artifact reference on session reads.
type ArtifactRefSummary struct {
	ID          string
	Kind        string
	Visibility  string
	ContentHash string
	SizeBytes   int64
}

// SessionReadResult is the shared durable session read projection consumed by API,
// CLI, MCP, and UI transports.
type SessionReadResult struct {
	SessionID         string
	Status            LifecycleStatus
	OrchestratorKind  string
	Dialect           string
	ResolvedSource    ResolvedSource
	SourceHash        string
	Policy            PolicyProjection
	Phase             string
	PhaseSummaries    []PhaseSummary
	Progress          *ProgressCounts
	ResultSummary     *ResultSummary
	ArtifactRefs      []ArtifactRefSummary
	ArtifactCount     int
	Failure           *FailureSummary
	Lifecycle         *LifecycleTimestamps
	StaleLease        bool
	Links             InspectionLinks
}

// LifecycleControlLinks are API-relative links for post-control inspection.
type LifecycleControlLinks struct {
	Session    string
	Results    string
	Dispatches string
	Artifacts  string
	Events     string
	Status     string
}

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
