package interfaces

import "time"

// FactoryWorldSessionBracketState reconstructs one durable factory session
// execution bracket from SESSION_STARTED, SESSION_RESULT_UPDATED, and
// SESSION_COMPLETED canonical events.
type FactoryWorldSessionBracketState struct {
	SessionID           string                              `json:"session_id,omitempty"`
	OrchestratorKind    string                              `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect string                              `json:"orchestrator_dialect,omitempty"`
	FactoryID           string                              `json:"factory_id,omitempty"`
	SourceRef           string                              `json:"source_ref,omitempty"`
	SourceHash          string                              `json:"source_hash,omitempty"`
	PolicyHash          string                              `json:"policy_hash,omitempty"`
	ArgsDigest          string                              `json:"args_digest,omitempty"`
	StartedAt           time.Time                           `json:"started_at,omitempty"`
	ResultStatus        string                              `json:"result_status,omitempty"`
	ResultSummary       []WorkContentPart                   `json:"result_summary,omitempty"`
	ArtifactIDs         []string                            `json:"artifact_ids,omitempty"`
	Terminal            bool                                `json:"terminal"`
	FinalStatus         string                              `json:"final_status,omitempty"`
	CompletedAt         time.Time                           `json:"completed_at,omitempty"`
	DurationMillis      int64                               `json:"duration_millis,omitempty"`
	DispatchCounts      *FactoryWorldJavaScriptChildDispatchCounts `json:"dispatch_counts,omitempty"`
	FailureReason       string                              `json:"failure_reason,omitempty"`
	FailureMessage      string                              `json:"failure_message,omitempty"`
	FailureErrorClass   string                              `json:"failure_error_class,omitempty"`
}

// FactoryWorldSessionBracketProjection is the customer-visible session bracket
// projection derived from reconstructed world state.
type FactoryWorldSessionBracketProjection struct {
	SessionID           string            `json:"session_id,omitempty"`
	OrchestratorKind    string            `json:"orchestrator_kind,omitempty"`
	OrchestratorDialect string            `json:"orchestrator_dialect,omitempty"`
	FactoryID           string            `json:"factory_id,omitempty"`
	SourceRef           string            `json:"source_ref,omitempty"`
	StartedAt           time.Time         `json:"started_at,omitempty"`
	ResultStatus        string            `json:"result_status,omitempty"`
	ResultSummary       []WorkContentPart `json:"result_summary,omitempty"`
	ArtifactIDs         []string          `json:"artifact_ids,omitempty"`
	Terminal            bool              `json:"terminal"`
	FinalStatus         string            `json:"final_status,omitempty"`
	CompletedAt         time.Time         `json:"completed_at,omitempty"`
	DurationMillis      int64             `json:"duration_millis,omitempty"`
	FailureReason       string            `json:"failure_reason,omitempty"`
	FailureMessage      string            `json:"failure_message,omitempty"`
}
