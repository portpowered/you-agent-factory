package factorysessions

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkStopSummaryRequest carries the exact canonical state used to derive the
// stopped-state projection for one Work read.
type WorkStopSummaryRequest struct {
	SessionID          string
	Snapshot           *interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet]
	Token              *factorytoken.Token
	SessionStopSummary *StopSummary
}

// WorkStopSummaryProjector is the exact Factory Sessions-owned operation
// injected into Work-read transports.
type WorkStopSummaryProjector func(WorkStopSummaryRequest) *StopSummary

// Project derives one owner-defined Work stop summary.
func (project WorkStopSummaryProjector) Project(request WorkStopSummaryRequest) *StopSummary {
	if project == nil {
		return nil
	}
	return project(request)
}
