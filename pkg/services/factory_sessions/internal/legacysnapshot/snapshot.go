package legacysnapshot

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Snapshot is the migration-only engine snapshot retained for stop-summary and
// other legacy projection paths inside Factory Sessions.
type Snapshot = interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet]

// Provider is the migration-only snapshot capability used by legacy session
// projections. It is private to Factory Sessions and is not a peer contract.
type Provider interface {
	GetEngineStateSnapshot(context.Context) (*Snapshot, error)
}
