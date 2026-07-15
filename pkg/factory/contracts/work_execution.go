package factorycontracts

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
)

// ReplayArtifact remains a Factory replay compatibility contract until the
// Factory ownership migration moves the remaining event and projection types.
type ReplayArtifact struct {
	SchemaVersion string                    `json:"schemaVersion"`
	RecordedAt    time.Time                 `json:"recordedAt"`
	Events        []factoryapi.FactoryEvent `json:"events"`
	Factory       factoryapi.Factory        `json:"-"`
	Diagnostics   ReplayDiagnostics         `json:"-"`
	WallClock     *ReplayWallClockMetadata  `json:"-"`
}

type ReplayDiagnostics struct {
	Notes   []string                                         `json:"notes,omitempty"`
	Workers map[string]workerdiagnostics.SafeWorkDiagnostics `json:"workers,omitempty"`
}

type ReplayWallClockMetadata struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}
