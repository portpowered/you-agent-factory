package root

import (
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// WorkerRecordingReaderFromProcess narrows the opaque read capability carried
// by the neutral application process to the Recordings-owned contract. The
// process still retains only one selected capability; it does not expose the
// application service graph or the capture writer.
func WorkerRecordingReaderFromProcess(
	process *initializerapplication.Process,
) recordings.WorkerRecordingReader {
	if process == nil {
		return nil
	}
	capability := process.WorkerRecordingReader()
	if capability == nil {
		return nil
	}
	reader, _ := capability.Value().(recordings.WorkerRecordingReader)
	return reader
}
