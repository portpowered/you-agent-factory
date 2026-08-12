package root

import (
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// WorkerRecordingReaderFromProcess returns the Recordings-owned read
// capability carried by the process. The process retains only this selected
// capability; it does not expose the application service graph or capture
// writer.
func WorkerRecordingReaderFromProcess(
	process *initializerapplication.Process,
) recordings.WorkerRecordingReader {
	if process == nil {
		return nil
	}
	return process.WorkerRecordingReader()
}
