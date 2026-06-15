package factorysessionexecution

import (
	"encoding/json"
	"errors"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// SyncOutcome reports how a sync start wait ended.
type SyncOutcome string

const (
	SyncOutcomeCompleted    SyncOutcome = "COMPLETED"
	SyncOutcomeTimedOut     SyncOutcome = "TIMED_OUT"
	SyncOutcomeStillRunning SyncOutcome = "STILL_RUNNING"
)

// InlineWorkflowSource carries inline workflow source from a durable execution request.
type InlineWorkflowSource struct {
	Dialect      string
	InlineSource string
	Entrypoint   string
	Metadata     map[string]string
}

// Source is the normalized durable execution source selector.
type Source struct {
	Kind            workflowsource.Kind
	FactoryID       string
	FactoryInline   json.RawMessage
	WorkflowFile    string
	WorkflowName    string
	InlineWorkflow  *InlineWorkflowSource
}

// OrchestratorOverride is an optional orchestrator override on a start request.
type OrchestratorOverride struct {
	Kind string
	Raw  json.RawMessage
}

// WaitOptions bounds sync execution waits.
type WaitOptions struct {
	TimeoutMillis   *int64
	CancelOnTimeout bool
}

// StartRequest is the normalized durable session execution request shared by async
// and sync start across API, CLI, MCP, and UI.
type StartRequest struct {
	RequestID       string
	Source          Source
	Args            map[string]any
	Orchestrator    *OrchestratorOverride
	RequestedPolicy map[string]any
	Wait            *WaitOptions
}

// ResolvedSource is the customer-visible resolved source identity for one session.
type ResolvedSource struct {
	Kind            workflowsource.Kind
	SourceRef       string
	SourceHash      string
	Dialect         string
	ResolutionOrder []string
	Metadata        map[string]string
}

// InspectionLinks are API-relative links for polling and inspecting one session.
type InspectionLinks struct {
	Session  string
	Status   string
	Events   string
	Results  string
	Dispatches string
	Artifacts  string
}

// PolicyProjection carries requested and effective policy hashes for start responses.
type PolicyProjection struct {
	Requested     map[string]any
	Effective     map[string]any
	EffectiveHash string
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

// SyncStartResult is the shared sync durable execution start outcome.
type SyncStartResult struct {
	AsyncStartResult
	SyncOutcome              SyncOutcome
	Result                   json.RawMessage
	TimedOut                 bool
	SessionCanceledByTimeout bool
}

// ErrExecutionRequestIDConflict reports that requestId was reused with a different
// normalized execution tuple or a different start operation (async vs sync).
var ErrExecutionRequestIDConflict = errors.New("execution request id conflict")

// ErrControlRequestIDConflict reports that requestId was reused with a different
// normalized lifecycle-control tuple.
var ErrControlRequestIDConflict = errors.New("control request id conflict")

// ErrSessionNotFound reports that no durable session matched the requested id.
var ErrSessionNotFound = errors.New("factory session not found")

// ErrDispatchNotFound reports that no dispatch matched the requested id within
// the targeted durable session.
var ErrDispatchNotFound = errors.New("dispatch not found")

// ErrArtifactNotFound reports that no artifact matched the requested id within
// the targeted durable session.
var ErrArtifactNotFound = errors.New("artifact not found")

// ErrReconnectCursorNotFound reports that the reconnect cursor did not match any
// recorded durable session event.
var ErrReconnectCursorNotFound = errors.New("reconnect cursor not found in event history")

// ErrUnsupportedControl reports that the requested control is not supported by
// the active durable session runtime.
var ErrUnsupportedControl = errors.New("unsupported lifecycle control")

// ControlError carries a typed lifecycle-control outcome for invalid transitions
// and other actionable control failures surfaced by service implementations.
type ControlError struct {
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Status    LifecycleStatus
	Message   string
}

func (e *ControlError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Outcome)
}

// ValidationError reports a stable client-side normalization failure.
type ValidationError struct {
	Message string
	Field   string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewValidationError constructs one field-scoped validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}
