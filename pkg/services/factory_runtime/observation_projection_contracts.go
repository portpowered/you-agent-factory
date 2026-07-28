package factory

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Runtime-owned observation and projection shape aliases published at the
// Factory Runtime root. Recordings projection and observation edges consume
// these aliases rather than nested Runtime implementation packages.
type (
	RuntimeObservationRequest = ObserveRequest
	RuntimeObservationResult  = ObserveResult
	RuntimeObservation        = Observation
)

// RuntimeEngineStateSnapshot carries orchestration-neutral dashboard header
// facts peers derive from Service.Observe or event replay.
type RuntimeEngineStateSnapshot struct {
	FactoryState  string
	RuntimeStatus interfaces.RuntimeStatus
	TickCount     int
	Uptime        time.Duration
}

// DashboardEngineStateSnapshot constructs the Runtime-owned engine-state
// snapshot vocabulary Recordings dashboard seams use when correlating selected
// tick world-state projections with live Runtime uptime and tick metadata.
func DashboardEngineStateSnapshot(
	factoryState string,
	runtimeStatus interfaces.RuntimeStatus,
	tickCount int,
	uptime time.Duration,
) RuntimeEngineStateSnapshot {
	return RuntimeEngineStateSnapshot{
		FactoryState:  factoryState,
		RuntimeStatus: runtimeStatus,
		TickCount:     tickCount,
		Uptime:        uptime,
	}
}
