package factory

import (
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// JavaScriptCheckpointStore is the Factory Runtime capability for retaining
// checkpoint records belonging to one JavaScript workflow session.
type JavaScriptCheckpointStore interface {
	Put(factorydefinitions.JavaScriptCheckpointRecord)
	List() []factorydefinitions.JavaScriptCheckpointRecord
	Get(string) (factorydefinitions.JavaScriptCheckpointRecord, bool)
}

// JavaScriptCheckpointStoreFactory constructs an isolated checkpoint store for
// one live JavaScript workflow session.
type JavaScriptCheckpointStoreFactory func() JavaScriptCheckpointStore

const (
	JavaScriptCheckpointSummaryKind          = "javascript_checkpoint_summary"
	JavaScriptCheckpointSummarySchemaVersion = 1
	JavaScriptResumeStrategy                 = "replay_completed_dispatches_then_continue"
)

// JavaScriptCheckpointSummary is the durable resume contract for one
// JavaScript workflow session.
type JavaScriptCheckpointSummary struct {
	SchemaVersion        int            `json:"schemaVersion"`
	Kind                 string         `json:"kind"`
	SessionID            string         `json:"sessionId"`
	CheckpointID         string         `json:"checkpointId"`
	Label                string         `json:"label,omitempty"`
	Phase                string         `json:"phase,omitempty"`
	SourceHash           string         `json:"sourceHash,omitempty"`
	ArgsDigest           string         `json:"argsDigest,omitempty"`
	PolicyHash           string         `json:"policyHash,omitempty"`
	CreatedAt            time.Time      `json:"createdAt,omitempty"`
	CompletedDispatchIDs []string       `json:"completedDispatchIds"`
	PendingDispatchIDs   []string       `json:"pendingDispatchIds,omitempty"`
	ArtifactIDs          []string       `json:"artifactIds,omitempty"`
	ResumeStrategy       string         `json:"resumeStrategy"`
	CheckpointState      map[string]any `json:"checkpointState,omitempty"`
}

// JavaScriptCheckpointSummaryInput carries the facts needed to build one
// durable checkpoint summary.
type JavaScriptCheckpointSummaryInput struct {
	SessionID       string
	CheckpointID    string
	Label           string
	Phase           string
	SourceHash      string
	ArgsDigest      string
	PolicyHash      string
	CreatedAt       time.Time
	CheckpointState map[string]any
	Records         []JavaScriptRuntimeRecord
}

// JavaScriptCheckpointSummaries projects durable checkpoint summaries from
// JavaScript runtime records.
type JavaScriptCheckpointSummaries interface {
	Build(JavaScriptCheckpointSummaryInput) *JavaScriptCheckpointSummary
	Latest(JavaScriptCheckpointSummaryInput) *JavaScriptCheckpointSummary
}
