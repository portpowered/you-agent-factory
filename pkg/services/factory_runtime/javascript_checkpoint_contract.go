package factory

import (
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type JavaScriptCheckpointStore interface {
	Put(factorydefinitions.JavaScriptCheckpointRecord)
	List() []factorydefinitions.JavaScriptCheckpointRecord
	Get(string) (factorydefinitions.JavaScriptCheckpointRecord, bool)
}

type JavaScriptCheckpointStoreFactory func() JavaScriptCheckpointStore

const (
	JavaScriptCheckpointSummaryKind          = "javascript_checkpoint_summary"
	JavaScriptCheckpointSummarySchemaVersion = 1
	JavaScriptResumeStrategy                 = "replay_completed_dispatches_then_continue"
)

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

type JavaScriptCheckpointSummaries interface {
	Build(JavaScriptCheckpointSummaryInput) *JavaScriptCheckpointSummary
	Latest(JavaScriptCheckpointSummaryInput) *JavaScriptCheckpointSummary
}
