package factorycontracts

import (
	"time"
)

// FactorySessionDispatchState carries one orchestrator-aware dispatch projection
// input for factory session runtime reads.
type FactorySessionDispatchState struct {
	ID             string                                 `json:"id"`
	DispatchKind   string                                 `json:"dispatchKind"`
	Status         string                                 `json:"status"`
	Phase          string                                 `json:"phase,omitempty"`
	Label          string                                 `json:"label,omitempty"`
	RunnerID       string                                 `json:"runnerId,omitempty"`
	Model          string                                 `json:"model,omitempty"`
	Provider       string                                 `json:"provider,omitempty"`
	PromptDigest   string                                 `json:"promptDigest,omitempty"`
	SchemaDigest   string                                 `json:"schemaDigest,omitempty"`
	RelatedWorkIDs []string                               `json:"relatedWorkIds,omitempty"`
	ArtifactIDs    []string                               `json:"artifactIds,omitempty"`
	Usage          *FactorySessionDispatchUsage           `json:"usage,omitempty"`
	Warnings       []FactorySessionDispatchWarning        `json:"warnings,omitempty"`
	FailureDetail  *FactorySessionDispatchFailureDetail   `json:"failureDetail,omitempty"`
	Petri          *FactorySessionDispatchPetriState      `json:"petri,omitempty"`
	JavaScript     *FactorySessionDispatchJavaScriptState `json:"javascript,omitempty"`
}

// FactorySessionDispatchUsage carries usage metadata for one dispatch projection.
type FactorySessionDispatchUsage struct {
	InputTokens    int64   `json:"inputTokens,omitempty"`
	OutputTokens   int64   `json:"outputTokens,omitempty"`
	TotalTokens    int64   `json:"totalTokens,omitempty"`
	CostUSD        float64 `json:"costUsd,omitempty"`
	DurationMillis int64   `json:"durationMillis,omitempty"`
	RetryCount     int     `json:"retryCount,omitempty"`
}

// FactorySessionDispatchWarning carries one customer-visible dispatch warning.
type FactorySessionDispatchWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// FactorySessionDispatchFailureDetail carries failure metadata for one dispatch.
type FactorySessionDispatchFailureDetail struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// FactorySessionDispatchPetriState carries Petri-specific dispatch projection fields.
type FactorySessionDispatchPetriState struct {
	TransitionID    string `json:"transitionId"`
	WorkstationName string `json:"workstationName,omitempty"`
	WorkerType      string `json:"workerType,omitempty"`
}

// FactorySessionDispatchJavaScriptState carries JavaScript-specific dispatch projection fields.
type FactorySessionDispatchJavaScriptState struct {
	TaskKind  string `json:"taskKind"`
	TaskLabel string `json:"taskLabel,omitempty"`
}

// FactorySessionArtifactState carries one orchestrator-aware artifact projection
// input for factory session runtime reads.
type FactorySessionArtifactState struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	Visibility      string            `json:"visibility"`
	Label           string            `json:"label,omitempty"`
	Summary         string            `json:"summary,omitempty"`
	AuditMode       string            `json:"auditMode,omitempty"`
	ContentHash     string            `json:"contentHash,omitempty"`
	SizeBytes       int64             `json:"sizeBytes,omitempty"`
	RedactionCounts map[string]int    `json:"redactionCounts,omitempty"`
	CaptureMetadata map[string]string `json:"captureMetadata,omitempty"`
	CapturedAt      time.Time         `json:"capturedAt,omitempty"`
}
