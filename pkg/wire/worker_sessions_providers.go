package wire

import (
	"os"
	"path/filepath"
	runtime "runtime"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workerRecordingReaderCapability struct {
	reader recordings.WorkerRecordingReader
}

func (capability workerRecordingReaderCapability) Value() any {
	return capability.reader
}

func provideWorkerRecordingReaderCapability(
	writer recordings.WorkerRecordingWriter,
) initializerapplication.WorkerRecordingReaderCapability {
	reader, _ := writer.(recordings.WorkerRecordingReader)
	return workerRecordingReaderCapability{reader: reader}
}

// provideWorkerSessionsFactory constructs the one canonical per-session
// Worker Sessions (W4 Runtime dispatch cutover) construction path over the
// already-composed events.Service and logging.Logger singletons. Factory
// Runtime consumes only the returned factoryruntime.WorkerSessionsFactory
// function value, never worker_sessions/wire directly, so composition of the
// peer worker_sessions service stays inside pkg/wire.
func provideWorkerSessionsFactory(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	recorder recordings.WorkerSessionRecordingService,
) factoryruntime.WorkerSessionsFactory {
	return provideWorkerSessionsFactoryWithRecorder(eventsService, providerSessions, logger, recorder)
}

func provideWorkerSessionRecorder(
	eventsService events.Service,
	writer recordings.WorkerRecordingWriter,
	logger logging.Logger,
) (recordings.WorkerSessionRecordingService, error) {
	return recordingswire.NewWorkerSessionRecorder(eventsService, writer, logger)
}

func provideWorkerRecordingWriter(
	edges serviceedges.Edges,
) (recordings.WorkerRecordingWriter, error) {
	writer := edges.WorkerRecordingWriter
	if writer == nil {
		var err error
		writer, err = recordingswire.NewWorkerRecordingFileWriter(
			platformreplay.NewLocal(runtime.GOOS),
			filepath.Join(os.TempDir(), "you-worker-recordings"),
		)
		if err != nil {
			return nil, err
		}
	}
	return writer, nil
}

func provideWorkerSessionsFactoryWithRecorder(
	eventsService events.Service,
	providerSessions providersessions.Service,
	logger logging.Logger,
	recorder recordings.WorkerSessionRecordingService,
) factoryruntime.WorkerSessionsFactory {
	return func(boundary workers.WorkstationPoolBoundary, clock platformclock.Source) (workersessions.Service, error) {
		return workersessionswire.NewService(boundary, eventsService, logger, clock, providerSessions, recorder)
	}
}
