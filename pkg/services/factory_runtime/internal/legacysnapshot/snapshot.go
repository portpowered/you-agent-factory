package legacysnapshot

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

// Snapshot is the migration-only Petri engine snapshot retained for internal
// consumers that still require markings or topology.
type Snapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

// Provider is migration-only Petri snapshot access retained for hosted runtimes.
// It is not part of the published Service peer observation contract.
type Provider interface {
	GetEngineStateSnapshot(ctx context.Context) (*Snapshot, error)
}
