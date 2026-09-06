package wire

import (
	"io/fs"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture/internal/service"
)

// New constructs the Recordings-owned Worker capture capability over Events
// and an explicit durable writer.
func New(
	eventService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
) (recordings.WorkerSessionRecordingService, error) {
	return workerrecording.New(eventService, writer, logger)
}

// NewFileWriter selects the durable local sidecar writer used by production
// composition when no external Worker-recording writer override is supplied.
func NewFileWriter(storage platformreplay.Storage, root string) (recordings.WorkerRecordingWriter, error) {
	return workerrecording.NewFileWriter(storage, root)
}

// NewFileWriterWithDirectoryReader binds the catalog to the exact directory
// listing effect selected by the composition boundary.
func NewFileWriterWithDirectoryReader(
	storage platformreplay.Storage,
	root string,
	readDir func(string) ([]fs.DirEntry, error),
) (recordings.WorkerRecordingWriter, error) {
	return workerrecording.NewFileWriterWithDirectoryReader(storage, root, readDir)
}
