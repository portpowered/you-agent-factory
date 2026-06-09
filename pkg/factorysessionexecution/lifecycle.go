package factorysessionexecution

import (
	"strings"
	"time"
)

// IsTerminalLifecycleStatus reports whether status is terminal and therefore
// immutable except for explicitly allowed inspection or retry behaviors.
func IsTerminalLifecycleStatus(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated:
		return true
	default:
		return false
	}
}

// AllowsPostTerminalInspection reports whether read/status/result/dispatch/artifact
// inspection remains allowed after the session reaches a terminal status.
func AllowsPostTerminalInspection(status LifecycleStatus) bool {
	return status != ""
}

// AllowsRetryDispatchOnTerminal reports whether retry-dispatch remains permitted
// after the session reaches a terminal status. Failed sessions may still accept
// retry-dispatch for failed child dispatches.
func AllowsRetryDispatchOnTerminal(status LifecycleStatus) bool {
	return status == LifecycleStatusFailed
}

// EvaluateLifecycleControl classifies one lifecycle control request against the
// current durable session status without runtime-specific dispatch context.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this transition classifier keeps durable lifecycle control outcomes explicit across terminal and active states.
func EvaluateLifecycleControl(operation LifecycleControlKind, status LifecycleStatus) LifecycleControlOutcome {
	if status == "" {
		return LifecycleControlOutcomeInvalidState
	}
	if IsTerminalLifecycleStatus(status) {
		switch operation {
		case LifecycleControlRetryDispatch:
			if status == LifecycleStatusFailed {
				return LifecycleControlOutcomeAccepted
			}
			return LifecycleControlOutcomeTerminalSession
		case LifecycleControlCancel, LifecycleControlTerminate:
			if status == LifecycleStatusCanceled && operation == LifecycleControlCancel {
				return LifecycleControlOutcomeNoOp
			}
			if status == LifecycleStatusTerminated && operation == LifecycleControlTerminate {
				return LifecycleControlOutcomeNoOp
			}
			return LifecycleControlOutcomeTerminalSession
		default:
			return LifecycleControlOutcomeTerminalSession
		}
	}

	switch operation {
	case LifecycleControlPause:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlResume:
		switch status {
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusResuming:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlCancel:
		switch status {
		case LifecycleStatusCanceling:
			return LifecycleControlOutcomeNoOp
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlTerminate:
		switch status {
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming,
			LifecycleStatusCanceling:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlApprove:
		if status == LifecycleStatusAwaitingApproval {
			return LifecycleControlOutcomeAccepted
		}
		return LifecycleControlOutcomeInvalidState
	case LifecycleControlRetryDispatch:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	default:
		return LifecycleControlOutcomeInvalidState
	}
}

// NormalizeLifecycleStatus trims and validates one durable lifecycle status value.
func NormalizeLifecycleStatus(status string) (LifecycleStatus, error) {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return "", NewValidationError("status", "status is required")
	}
	normalized := LifecycleStatus(trimmed)
	switch normalized {
	case LifecycleStatusQueued,
		LifecycleStatusAwaitingApproval,
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusResuming,
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceling,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated:
		return normalized, nil
	default:
		return "", NewValidationError("status", "status is invalid")
	}
}

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
