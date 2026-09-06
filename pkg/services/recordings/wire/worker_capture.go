package wire

import (
	"io/fs"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerrecordingwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture/wire"
)

// NewWorkerSessionRecorder constructs the Recordings-owned capture capability
// over the process Events stream and an explicit durable writer. It performs
// no subscription until a Worker Session asks to start recording.
func NewWorkerSessionRecorder(
	eventService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
) (recordings.WorkerSessionRecordingService, error) {
	return workerrecordingwire.New(eventService, writer, logger)
}

// NewWorkerRecordingFileWriter constructs the default durable Worker sidecar
// writer from the policy-free replay storage effect selected by Wire.
func NewWorkerRecordingFileWriter(
	storage platformreplay.Storage,
	root string,
) (recordings.WorkerRecordingWriter, error) {
	return workerrecordingwire.NewFileWriter(storage, root)
}

// NewWorkerRecordingFileWriterWithDirectoryReader constructs the default
// durable Worker sidecar with the directory-listing effect selected by Wire.
func NewWorkerRecordingFileWriterWithDirectoryReader(
	storage platformreplay.Storage,
	root string,
	readDir func(string) ([]fs.DirEntry, error),
) (recordings.WorkerRecordingWriter, error) {
	return workerrecordingwire.NewFileWriterWithDirectoryReader(storage, root, readDir)
}
