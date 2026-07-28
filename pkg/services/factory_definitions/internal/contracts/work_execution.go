package factorycontracts

import (
	"time"

	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ReplayArtifact is the Factory-owned in-memory view of one replay recording.
// Serialized events remain the compatibility source of the embedded Factory;
// Factory is hydrated as a detached snapshot for runtime consumers.
type ReplayArtifact struct {
	SchemaVersion string                   `json:"schemaVersion"`
	RecordedAt    time.Time                `json:"recordedAt"`
	Events        []FactoryEvent           `json:"events"`
	Factory       *FactorySnapshot         `json:"-"`
	Diagnostics   ReplayDiagnostics        `json:"-"`
	WallClock     *ReplayWallClockMetadata `json:"-"`
}

type ReplayDiagnostics struct {
	Notes   []string                                         `json:"notes,omitempty"`
	Workers map[string]workerdiagnostics.SafeWorkDiagnostics `json:"workers,omitempty"`
}

type ReplayWallClockMetadata struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}
