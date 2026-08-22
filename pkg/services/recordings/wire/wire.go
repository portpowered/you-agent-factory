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
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
)

// NewService constructs an inert Recordings root from the runtime ledger seam
// and process-edge ports selected by the application graph. It composes the
// accepted root through parent-private ledger/projection/lifecycle/replay/
// artifacts owners without publishing owner types on the returned peer surface.
func NewService(
	ledger recordings.Ledger,
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
	clocks ...recordings.RecordingClock,
) (recordings.Service, error) {
	if ledger == nil {
		return nil, fmt.Errorf("construct Recordings: ledger is required")
	}
	if writeFile == nil {
		return nil, fmt.Errorf("construct Recordings: snapshot write function is required")
	}
	return NewServiceWithProjection(
		ledger,
		recordingsinternal.NewProjectionService(),
		targets,
		writeFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
		clocks...,
	)
}
