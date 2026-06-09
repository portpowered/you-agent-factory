package factorysessionexecution

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
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
