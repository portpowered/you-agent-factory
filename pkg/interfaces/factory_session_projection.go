package interfaces

import "time"

// FactorySessionJavaScriptCheckpointRef is a customer-visible JavaScript checkpoint
// reference without raw VM checkpoint payload bodies.
type FactorySessionJavaScriptCheckpointRef struct {
	ID          string                           `json:"id"`
	Label       string                           `json:"label"`
	Summary     string                           `json:"summary"`
	Timestamp   time.Time                        `json:"timestamp,omitempty"`
	ArtifactRef *JavaScriptCheckpointArtifactRef `json:"artifactRef,omitempty"`
}

// FactorySessionJavaScriptRuntimeState carries JavaScript orchestrator runtime
// projection fields for one factory session.
type FactorySessionJavaScriptRuntimeState struct {
	Phase               string                                `json:"phase"`
	Phases              []string                              `json:"phases"`
	ArgsDigest          string                                `json:"argsDigest"`
	Checkpoints         []FactorySessionJavaScriptCheckpointRef `json:"checkpoints"`
	ScriptStatus        string                                `json:"scriptStatus"`
	QueuedDispatches    int                                   `json:"queuedDispatches"`
	RunningDispatches   int                                   `json:"runningDispatches"`
	CompletedDispatches int                                   `json:"completedDispatches"`
}
