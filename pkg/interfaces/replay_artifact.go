package interfaces

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ReplayArtifact is the versioned, self-contained recording used to replay a
// factory run without requiring the original customer files or live side
// effects.
type ReplayArtifact struct {
	SchemaVersion string                    `json:"schemaVersion"`
	RecordedAt    time.Time                 `json:"recordedAt"`
	Events        []factoryapi.FactoryEvent `json:"events"`

	// The fields below are hydrated from Events for the current replay
	// implementation. They are intentionally excluded from artifact storage.
	Factory     factoryapi.Factory       `json:"-"`
	Diagnostics ReplayDiagnostics        `json:"-"`
	WallClock   *ReplayWallClockMetadata `json:"-"`
}

// ReplayDiagnostics stores artifact-level notes and optional nested execution
// details.
type ReplayDiagnostics struct {
	Notes   []string                       `json:"notes,omitempty"`
	Workers map[string]SafeWorkDiagnostics `json:"workers,omitempty"`
}

// ReplayWallClockMetadata retains wall-clock timing for investigation only.
// Replay behavior is driven by logical ticks, not these timestamps.
type ReplayWallClockMetadata struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}
