package interfaces

import (
	"encoding/json"
	"time"
)

const (
	JavaScriptCheckpointArtifactVisibility = "INTERNAL_CHECKPOINT"
	JavaScriptCheckpointArtifactKind       = "CHECKPOINT"
)

// JavaScriptCheckpointRecord is an orchestrator-owned checkpoint bundle kept out
// of public session, world, and event projections.
type JavaScriptCheckpointRecord struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	Summary     string          `json:"summary"`
	Timestamp   time.Time       `json:"timestamp,omitempty"`
	ArtifactID  string          `json:"artifactId"`
	ContentHash string          `json:"contentHash"`
	SizeBytes   int64           `json:"sizeBytes"`
	RawBody     json.RawMessage `json:"rawBody"`
	StoragePath string          `json:"storagePath"`
}

// JavaScriptCheckpointArtifactRef is customer-visible artifact metadata for one
// orchestrator-owned checkpoint bundle.
type JavaScriptCheckpointArtifactRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Visibility  string `json:"visibility"`
	ContentHash string `json:"contentHash,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}
