package legacysnapshot

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Snapshot is the migration-only engine snapshot retained for stop-summary and
// other legacy projection paths inside Factory Sessions.
type Snapshot = interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet]
