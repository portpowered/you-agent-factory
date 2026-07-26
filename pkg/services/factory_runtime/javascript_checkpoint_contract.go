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

// CheckpointOutcome is the plain success vocabulary for Factory Runtime root
// checkpoint operations. Peers branch on these values without Petri marking
// snapshots or JavaScript checkpoint strategy types.
type CheckpointOutcome string

const (
	// CheckpointOutcomeCaptured indicates a versioned checkpoint was captured.
	CheckpointOutcomeCaptured CheckpointOutcome = "CAPTURED"
	// CheckpointOutcomeLoaded indicates a checkpoint was loaded for inspect or
	// compatibility checking.
	CheckpointOutcomeLoaded CheckpointOutcome = "LOADED"
	// CheckpointOutcomeRestored indicates a compatible opaque checkpoint was
	// restored into mutable Runtime execution state.
	CheckpointOutcomeRestored CheckpointOutcome = "RESTORED"
)

// Checkpoint is the plain versioned checkpoint value published at the Runtime
// root. Payload is opaque strategy checkpoint bytes; peers must not interpret
// it as Petri marking snapshots or JavaScript checkpoint strategy records.
// Recordings remains the owner of immutable history; this value is Runtime's
// mutable execution-checkpoint vocabulary only.
type Checkpoint struct {
	CheckpointID  string
	SchemaVersion int
	// StrategyKind is an opaque strategy discriminator string. It is not a
	// peer-facing Petri or JavaScript type; nested IMP-RUN packets own codec
	// meaning.
	StrategyKind string
	Payload      []byte
}

// CaptureCheckpointRequest is the plain capture input published at the Runtime
// root.
type CaptureCheckpointRequest struct {
	CheckpointID string
}

// CaptureCheckpointResult is the plain capture success shape published at the
// Runtime root.
type CaptureCheckpointResult struct {
	Outcome    CheckpointOutcome
	Checkpoint Checkpoint
}

// LoadCheckpointRequest is the plain load/inspect-compatibility input published
// at the Runtime root.
type LoadCheckpointRequest struct {
	CheckpointID          string
	ExpectedSchemaVersion int
}

// LoadCheckpointResult is the plain load/inspect success shape published at the
// Runtime root. Compatible reports whether the loaded checkpoint matches the
// expected schema version when ExpectedSchemaVersion is non-zero.
type LoadCheckpointResult struct {
	Outcome    CheckpointOutcome
	Checkpoint Checkpoint
	Compatible bool
}

// RestoreCheckpointRequest is the plain restore input published at the Runtime
// root. Checkpoint.Payload remains opaque strategy bytes.
type RestoreCheckpointRequest struct {
	Checkpoint Checkpoint
}

// RestoreCheckpointResult is the plain restore success shape published at the
// Runtime root.
type RestoreCheckpointResult struct {
	Outcome      CheckpointOutcome
	CheckpointID string
}
