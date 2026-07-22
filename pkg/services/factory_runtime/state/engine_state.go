package state

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
)

// SnapshotHasActiveWork reports whether a runtime snapshot contains an active
// dispatch or a non-terminal, non-resource Work token.
func SnapshotHasActiveWork(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 {
		return true
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if snapshot.Topology == nil {
			return true
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category != StateCategoryTerminal && category != StateCategoryFailed {
			return true
		}
	}
	return false
}

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
		StreamGenerationID:   runtime.StreamGenerationID,
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
