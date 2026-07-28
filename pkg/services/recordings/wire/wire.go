// Package wire is the Recordings service composition boundary.
//
// Wire performs construction only, returns the singular recordings.Service
// root interface, and starts no lifecycle components. Parent-private
// ledger/projection/lifecycle/replay/artifacts owner wiring stays inside the
// owner service assembly path; peers depend on Service rather than owner
// internals or construction ports.
package wire

import (
	"fmt"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
)

// NewService constructs an inert Recordings root from the runtime ledger seam
// and process-edge ports selected by the application graph. It composes the
// accepted root through parent-private ledger/projection/lifecycle/replay/
// artifacts owners without publishing owner types on the returned peer surface.
func NewService(
	ledger recordings.Ledger,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	writer := recordingsservice.NewReplayRecordingSnapshotWriter(writeFile)
	tickers := recordingsservice.NewRecordingFlushTickerFactory()
	publication, err := recordingsservice.NewPortableArtifactPublication()
	if err != nil {
		return nil, err
	}
	service := recordingsservice.NewServiceWithLifecycleEffects(
		ledger,
		recordingsservice.NewProjectionService(),
		targets,
		writer,
		tickers,
		publication,
		clocks...,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Recordings: implementation rejected its dependencies")
	}
	return service, nil
}
