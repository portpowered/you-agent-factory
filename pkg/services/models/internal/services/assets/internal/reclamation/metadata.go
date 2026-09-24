package reclamation

import models "github.com/portpowered/infinite-you/pkg/services/models"

// ManagedFile records the verified facts for one managed cache artifact.
type ManagedFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// BackendMetadata records the backend snapshot owned by a managed model.
type BackendMetadata struct {
	CachePath string        `json:"cachePath"`
	Revision  string        `json:"revision,omitempty"`
	Files     []ManagedFile `json:"files"`
}

// ManagedMetadata is the durable managed revision reference.
type ManagedMetadata struct {
	ModelName string           `json:"modelName"`
	Revision  string           `json:"revision"`
	Files     []ManagedFile    `json:"files"`
	Backend   *BackendMetadata `json:"backend,omitempty"`
}

// SnapshotMetadata records the ownership facts for a content-addressed tree.
type SnapshotMetadata struct {
	Kind      string                    `json:"kind"`
	Identity  string                    `json:"identity"`
	Source    string                    `json:"source"`
	SourceKey string                    `json:"sourceKey"`
	Artifacts []models.AssetRequirement `json:"artifacts"`
}
