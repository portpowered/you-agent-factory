package factorysessionexecution

import (
	"encoding/json"
	"time"
)

// ResultStatus is the customer-visible durable session result availability.
type ResultStatus string

const (
	ResultStatusNotReady          ResultStatus = "NOT_READY"
	ResultStatusPartial           ResultStatus = "PARTIAL"
	ResultStatusFinal             ResultStatus = "FINAL"
	ResultStatusFailedWithPartial ResultStatus = "FAILED_WITH_PARTIAL"
	ResultStatusUnavailable       ResultStatus = "UNAVAILABLE"
)

// ResultMode selects final or partial durable result retrieval.
type ResultMode string

const (
	ResultModeFinal   ResultMode = "final"
	ResultModePartial ResultMode = "partial"
)

// ResultRequest normalizes durable session result read parameters.
type ResultRequest struct {
	Mode             ResultMode
	IncludeArtifacts bool
}

// ResultAvailabilityDetail explains why a durable result is not ready or unavailable.
type ResultAvailabilityDetail struct {
	Reason    string
	Message   string
	Retryable bool
}

// ResultReadResult is the shared durable session result projection consumed by API,
// CLI, MCP, and UI transports.
type ResultReadResult struct {
	SessionID        string
	ResultStatus       ResultStatus
	SessionStatus    LifecycleStatus
	Mode             ResultMode
	IncludeArtifacts bool
	PrimaryResult    json.RawMessage
	ArtifactIDs      []string
	ArtifactRefs     []ArtifactRefSummary
	Failure          *FailureSummary
	Availability     *ResultAvailabilityDetail
}

// DispatchStatus is the canonical dispatch lifecycle status shared across orchestrators.
type DispatchStatus string

const (
	DispatchStatusQueued    DispatchStatus = "QUEUED"
	DispatchStatusRunning   DispatchStatus = "RUNNING"
	DispatchStatusCompleted DispatchStatus = "COMPLETED"
	DispatchStatusFailed    DispatchStatus = "FAILED"
	DispatchStatusCanceled  DispatchStatus = "CANCELED"
	DispatchStatusTimedOut  DispatchStatus = "TIMED_OUT"
	DispatchStatusSkipped   DispatchStatus = "SKIPPED"
)

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

// DispatchFailureDetail exposes one customer-visible dispatch failure.
type DispatchFailureDetail struct {
	Reason     string
	Message    string
	ErrorClass string
}

// DispatchPetriProjection carries Petri-specific dispatch metadata.
type DispatchPetriProjection struct {
	TransitionID    string
	WorkstationName string
	WorkerType      string
}

// DispatchJavaScriptProjection carries JavaScript-specific dispatch metadata.
type DispatchJavaScriptProjection struct {
	TaskKind  string
	TaskLabel string
}

// DispatchSummary is the shared durable dispatch list projection.
type DispatchSummary struct {
	ID                string
	Status            DispatchStatus
	DispatchKind      string
	Phase             string
	Label             string
	Attempt           int
	RunnerID          string
	Model             string
	Provider          string
	OutputArtifactIDs []string
	Usage             *DispatchUsage
	Warnings          []DispatchWarning
	FailureDetail     *DispatchFailureDetail
}

// DispatchDetail is the shared durable dispatch read projection.
type DispatchDetail struct {
	DispatchSummary
	SessionID        string
	OrchestratorKind string
	ArtifactIDs      []string
	Petri            *DispatchPetriProjection
	JavaScript       *DispatchJavaScriptProjection
}

// ListDispatchesResult is the shared durable dispatch list outcome.
type ListDispatchesResult struct {
	SessionID  string
	Dispatches []DispatchSummary
}

// ArtifactRetrievalRef is a safe API-relative artifact retrieval reference.
type ArtifactRetrievalRef struct {
	Href   string
	Method string
}

// ArtifactRedactionCounts summarizes secret suppression for one artifact.
type ArtifactRedactionCounts struct {
	Paths   int32
	Secrets int32
	Tokens  int32
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

// ArtifactDetail is the shared durable artifact read projection.
type ArtifactDetail struct {
	ArtifactSummary
	SessionID       string
	Summary         string
	CaptureMetadata map[string]any
	Content         json.RawMessage
	ContentRef      *ArtifactRetrievalRef
}

// ListArtifactsResult is the shared durable artifact list outcome.
type ListArtifactsResult struct {
	SessionID string
	Artifacts []ArtifactSummary
}

// EventReconnectRequest identifies the last acknowledged durable session event.
type EventReconnectRequest struct {
	AfterEventID  string
	AfterSequence *int
}

// EventReadResult carries replayed canonical session events for one durable session.
type EventReadResult struct {
	SessionID string
	Events    []json.RawMessage
}
