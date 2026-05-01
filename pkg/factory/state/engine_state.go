package state

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// NewEngineStateSnapshot builds the canonical aggregate snapshot for
// service-facing consumers from a raw runtime snapshot plus service lifecycle
// metadata.
func NewEngineStateSnapshot(
	runtime interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net],
	factoryState string,
	uptime time.Duration,
	topology *Net,
) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net] {
	return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]{
		RuntimeStatus:        runtime.RuntimeStatus,
		Marking:              runtime.Marking,
		Dispatches:           runtime.Dispatches,
		InFlightCount:        runtime.InFlightCount,
		Results:              runtime.Results,
		DispatchHistory:      runtime.DispatchHistory,
		ActiveThrottlePauses: runtime.ActiveThrottlePauses,
		TickCount:            runtime.TickCount,
		FactoryState:         factoryState,
		Uptime:               uptime,
		Topology:             topology,
	}
}
