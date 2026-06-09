package factorysessionexecution

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// SessionListScope selects which factory sessions a list request returns.
type SessionListScope string

const (
	SessionListScopeLive      SessionListScope = "live"
	SessionListScopePersisted SessionListScope = "persisted"
	SessionListScopeAll       SessionListScope = "all"
)

// DefaultSessionListScope is live for backward-compatible live workspace session listing.
const DefaultSessionListScope = SessionListScopeLive

// SessionListFilters narrows scoped session listing without requiring clients to
// parse session internals.
type SessionListFilters struct {
	Statuses          []LifecycleStatus
	OrchestratorKinds []string
	SourceKind        workflowsource.Kind
	SourceRef         string
	ProjectBoundary   string
	Recoverable       *bool
	StaleLease        *bool
	CreatedAfter      *time.Time
	CreatedBefore     *time.Time
	UpdatedAfter      *time.Time
	UpdatedBefore     *time.Time
}

// ListSessionsRequest is the shared scoped session listing request consumed by API,
// CLI, MCP, and UI transports.
type ListSessionsRequest struct {
	Scope   SessionListScope
	Filters SessionListFilters
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

// SessionActionAvailability exposes which lifecycle controls are currently valid
// for one listed durable session.
type SessionActionAvailability struct {
	CanPause         bool
	CanResume        bool
	CanCancel        bool
	CanTerminate     bool
	CanApprove       bool
	CanRetryDispatch bool
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

// ListSessionsResult is the shared scoped session listing outcome.
type ListSessionsResult struct {
	Scope           SessionListScope
	LiveSessions    []LiveSessionSummary
	DurableSessions []DurableSessionListSummary
}
