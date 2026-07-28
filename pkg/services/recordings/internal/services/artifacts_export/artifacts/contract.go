// Package recording defines the portable, privacy-bounded JavaScript Factory
// Session recording contract. It contains no persistence or replay side effects.
package artifacts

import (
	"encoding/json"
	"time"
)

const (
	KindJavaScriptFactorySession = "you.factory-session.javascript.recording"
	CurrentSchemaVersion         = "2"
	ReplayCompatibilityVersion   = "1"
	// MaxSecretsRedacted bounds the aggregate count exposed by a recording.
	// Counts at or above this limit are reported as the limit.
	MaxSecretsRedacted int64 = 1_000_000
)

var supportedSchemaVersions = []string{"1", CurrentSchemaVersion}
var supportedReplayCompatibilityVersions = []string{ReplayCompatibilityVersion}

type Recording struct {
	RecordingKind              string             `json:"recordingKind"`
	SchemaVersion              string             `json:"schemaVersion"`
	ReplayCompatibilityVersion string             `json:"replayCompatibilityVersion"`
	Session                    SessionSummary     `json:"session"`
	Source                     SourceSummary      `json:"source"`
	ArgumentsDigest            string             `json:"argumentsDigest"`
	PolicyHash                 string             `json:"policyHash"`
	Artifacts                  []ArtifactSummary  `json:"artifacts"`
	Events                     []EventSummary     `json:"events"`
	Checkpoint                 *CheckpointSummary `json:"checkpoint,omitempty"`
	Result                     *ResultProjection  `json:"result,omitempty"`
	Redaction                  RedactionMetadata  `json:"redaction"`
}

type SessionSummary struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	OrchestratorKind string `json:"orchestratorKind"`
}

type SourceSummary struct {
	Ref  string `json:"ref"`
	Hash string `json:"hash"`
}

type ArtifactSummary struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Visibility  string    `json:"visibility"`
	Label       string    `json:"label,omitempty"`
	ContentHash string    `json:"contentHash"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
}

type EventSummary struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Sequence     int64     `json:"sequence"`
	Timestamp    time.Time `json:"timestamp"`
	ArtifactIDs  []string  `json:"artifactIds,omitempty"`
	CheckpointID string    `json:"checkpointId,omitempty"`
}

// CheckpointSummary exposes only the public checkpoint reference used for
// historical inspection. It never contains checkpoint state or dispatch data.
type CheckpointSummary struct {
	ID         string    `json:"id"`
	Label      string    `json:"label,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	ArtifactID string    `json:"artifactId,omitempty"`
}

// ResultProjection contains only the canonical public result read model. The
// digest protects inline public result data independently from artifact summaries.
type ResultProjection struct {
	Status        string              `json:"status"`
	Mode          string              `json:"mode"`
	PrimaryResult json.RawMessage     `json:"primaryResult,omitempty"`
	ContentHash   string              `json:"contentHash,omitempty"`
	ArtifactIDs   []string            `json:"artifactIds,omitempty"`
	Failure       *FailureSummary     `json:"failure,omitempty"`
	Availability  *AvailabilityDetail `json:"availability,omitempty"`
}

type FailureSummary struct {
	Reason                 string `json:"reason"`
	Message                string `json:"message,omitempty"`
	PartialResultAvailable bool   `json:"partialResultAvailable"`
}

type AvailabilityDetail struct {
	Reason    string `json:"reason"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable"`
}

type RedactionMetadata struct {
	RuntimeStateOmitted        bool  `json:"runtimeStateOmitted"`
	CheckpointBodiesOmitted    bool  `json:"checkpointBodiesOmitted"`
	ProviderTranscriptsOmitted bool  `json:"providerTranscriptsOmitted"`
	ChildDispatchesOmitted     bool  `json:"childDispatchesOmitted"`
	SecretsRedacted            int64 `json:"secretsRedacted"`
}
