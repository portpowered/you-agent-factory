package legacysnapshot

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Snapshot is the migration-only engine snapshot retained for stop-summary and
// other legacy projection paths inside Factory Sessions.
type runtimeNet = factory.RuntimeNet
type Snapshot = interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *runtimeNet]

// RuntimeTopology and RuntimeWorkType keep legacy session projection helpers
// behind this migration-only compatibility package. New peer contracts must
// not expose the concrete Factory Runtime graph types.
type RuntimeTopology = runtimeNet
type RuntimeWorkType = factory.WorkType

// Provider is the migration-only snapshot capability used by legacy session
// projections. It is private to Factory Sessions and is not a peer contract.
type Provider interface {
	GetEngineStateSnapshot(context.Context) (*Snapshot, error)
}
