package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	internalservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
)

type ObservationServiceCatalog = internalservice.ObservationServiceCatalog
type FleetObservationService = internalservice.FleetObservationService
type FleetHistory = internalservice.FleetHistory
type LogReader = internalservice.LogReader

func NewFleetObservationService(catalog ObservationServiceCatalog, history *FleetHistory) *FleetObservationService {
	return internalservice.NewFleetObservationService(catalog, history)
}

func NewFleetHistory(catalog ObservationServiceCatalog, logs *LogReader, clock platformclock.Source, logger logging.Logger, snapshots *HistorySnapshotBudget, continuation workersessions.Service) *FleetHistory {
	// Normalize the optional concrete role before storing it behind history's
	// private port; a typed nil would falsely advertise capture availability.
	if logs == nil {
		return internalservice.NewFleetHistory(catalog, nil, clock, logger, snapshots, continuation)
	}
	return internalservice.NewFleetHistory(catalog, logs, clock, logger, snapshots, continuation)
}

// NewLogReader preserves absent capture capability for legacy injected writers.
func NewLogReader(captured recordings.WorkerCapturedActivityReader, logger logging.Logger) *LogReader {
	if captured == nil {
		return nil
	}
	return internalservice.NewLogReader(captured, logger)
}
