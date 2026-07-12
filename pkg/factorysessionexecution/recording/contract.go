// Package recording defines the portable, privacy-bounded JavaScript Factory
// Session recording contract. It contains no persistence or replay side effects.
package recording

import "time"

const (
	KindJavaScriptFactorySession = "you.factory-session.javascript.recording"
	CurrentSchemaVersion         = "2"
	ReplayCompatibilityVersion   = "1"
)

var supportedSchemaVersions = []string{"1", CurrentSchemaVersion}
var supportedReplayCompatibilityVersions = []string{ReplayCompatibilityVersion}

type Recording struct {
	RecordingKind              string            `json:"recordingKind"`
	SchemaVersion              string            `json:"schemaVersion"`
	ReplayCompatibilityVersion string            `json:"replayCompatibilityVersion"`
	Session                    SessionSummary    `json:"session"`
	Source                     SourceSummary     `json:"source"`
	ArgumentsDigest            string            `json:"argumentsDigest"`
	PolicyHash                 string            `json:"policyHash"`
	Artifacts                  []ArtifactSummary `json:"artifacts"`
	Events                     []EventSummary    `json:"events"`
	Redaction                  RedactionMetadata `json:"redaction"`
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
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Sequence    int64     `json:"sequence"`
	Timestamp   time.Time `json:"timestamp"`
	ArtifactIDs []string  `json:"artifactIds,omitempty"`
}

type RedactionMetadata struct {
	RuntimeStateOmitted        bool  `json:"runtimeStateOmitted"`
	CheckpointBodiesOmitted    bool  `json:"checkpointBodiesOmitted"`
	ProviderTranscriptsOmitted bool  `json:"providerTranscriptsOmitted"`
	ChildDispatchesOmitted     bool  `json:"childDispatchesOmitted"`
	SecretsRedacted            int64 `json:"secretsRedacted"`
}
