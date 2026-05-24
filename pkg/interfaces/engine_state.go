package interfaces

import "time"

// RuntimeStatus describes whether the runtime is actively processing work,
// intentionally idle but still available, or terminally finished.
type RuntimeStatus string

const (
	RuntimeStatusActive   RuntimeStatus = "ACTIVE"
	RuntimeStatusIdle     RuntimeStatus = "IDLE"
	RuntimeStatusFinished RuntimeStatus = "FINISHED"
)

// EngineStateSnapshot is a unified point-in-time snapshot of the full engine
// state: runtime state, factory lifecycle, session metrics, and uptime.
type EngineStateSnapshot[TMarking any, TTopology any] struct {
	RuntimeStatus   RuntimeStatus             `json:"runtime_status"`
	Marking         TMarking                  `json:"marking"`
	Dispatches      map[string]*DispatchEntry `json:"dispatches"`
	InFlightCount   int                       `json:"in_flight_count"`
	Results         []WorkResult              `json:"results"`
	DispatchHistory []CompletedDispatch       `json:"dispatch_history"`
	// ActiveThrottlePauses exposes active provider/model pause windows owned by
	// dispatcher policy for tests and observability reconstruction.
	ActiveThrottlePauses []ActiveThrottlePause `json:"active_throttle_pauses,omitempty"`
	TickCount            int                   `json:"tick_count"`

	// Factory lifecycle state.
	FactoryState string `json:"factory_state"`

	// Uptime since the factory started.
	Uptime time.Duration `json:"uptime"`

	// Topology is the workflow net used to interpret marking and dispatch
	// records for service-facing observability read models.
	Topology TTopology `json:"topology,omitempty"`
}
